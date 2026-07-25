package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/KB-perByte/hiveshare/internal/embed"
	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/KB-perByte/hiveshare/internal/realtime"
	"github.com/KB-perByte/hiveshare/internal/store"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type HiveHandler struct {
	mem      *store.HiveStore
	hs       *store.HiveshareStore
	metrics  *store.MetricsStore
	embedder embed.Embedder
	hub      *realtime.Hub
	worker   *embed.Worker
	views    *store.ViewCounter
}

func NewHiveHandler(
	mem *store.HiveStore,
	hs *store.HiveshareStore,
	metrics *store.MetricsStore,
	embedder embed.Embedder,
	hub *realtime.Hub,
	worker *embed.Worker,
	views *store.ViewCounter,
) *HiveHandler {
	return &HiveHandler{
		mem: mem, hs: hs, metrics: metrics, embedder: embedder,
		hub: hub, worker: worker, views: views,
	}
}

func (h *HiveHandler) requireAccess(r *http.Request, w http.ResponseWriter, writeRequired bool) (uuid.UUID, bool) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hiveshare id")
		return uuid.Nil, false
	}
	role, _ := h.hs.IsMember(r.Context(), id, u.ID)
	if !models.CanView(role) {
		writeError(w, http.StatusForbidden, "not a member of this hiveshare")
		return uuid.Nil, false
	}
	if writeRequired && !models.CanWrite(role) {
		writeError(w, http.StatusForbidden, "view access cannot write memory")
		return uuid.Nil, false
	}
	return id, true
}

func (h *HiveHandler) List(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 50
	}
	entries, err := h.mem.List(r.Context(), hsID, store.ListHiveFilter{
		SourceType: q.Get("source_type"),
		SourceRef:  q.Get("source_ref"),
		Tag:        q.Get("tag"),
		Tool:       q.Get("tool"),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []*models.Hive{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *HiveHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	var req struct {
		SourceType string                 `json:"source_type"`
		SourceRef  string                 `json:"source_ref"`
		SourceURL  string                 `json:"source_url"`
		Tool       string                 `json:"tool"`
		Content    string                 `json:"content"`
		Summary    string                 `json:"summary"`
		Tags       []string               `json:"tags"`
		Metadata   map[string]interface{} `json:"metadata"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" || req.SourceType == "" || req.SourceRef == "" {
		writeError(w, http.StatusBadRequest, "content, source_type, and source_ref are required")
		return
	}
	if req.Tool == "" {
		req.Tool = "manual"
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}

	entry := &models.Hive{
		HiveshareID: hsID,
		UserID:      u.ID,
		SourceType:  req.SourceType,
		SourceRef:   req.SourceRef,
		SourceURL:   req.SourceURL,
		Tool:        req.Tool,
		Content:     req.Content,
		Summary:     req.Summary,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
	}

	// Store immediately with embedding = NULL; embed async so HTTP isn't blocked.
	// Auto-suffix source_ref (e.g. PROJ-123-2) if not unique within this hiveshare.
	baseRef := entry.SourceRef
	var created *models.Hive
	var err error
	for n := 0; ; n++ {
		if n > 0 {
			entry.SourceRef = fmt.Sprintf("%s-%d", baseRef, n+1)
		}
		created, err = h.mem.Create(r.Context(), entry, nil)
		if err == nil {
			break
		}
		if n >= 9 || !isUniqueViolation(err) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	created.UserName = u.Name

	h.worker.Enqueue(embed.Job{EntryID: created.ID, Content: req.Content})

	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EntryID:     &created.ID,
		EventType:   "add",
		Metadata:    map[string]interface{}{"source_ref": req.SourceRef, "tool": req.Tool},
	})

	_ = h.hub.Publish(r.Context(), models.StreamEvent{
		Type:        "hive_added",
		HiveshareID: hsID,
		Payload:     created,
	})

	writeJSON(w, http.StatusCreated, created)
}

func (h *HiveHandler) Get(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	u := currentUser(r)
	entryID, err := parseUUID(r, "entryId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entry id")
		return
	}
	entry, err := h.mem.Get(r.Context(), entryID, hsID)
	if err != nil {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}

	if pending, err := h.views.Increment(r.Context(), entryID); err == nil {
		// DB views are the flushed base; Redis holds the unflushed delta (incl. this view).
		entry.Views += int(pending)
	} else {
		entry.Views++ // best-effort local bump if Redis is down
	}

	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EntryID:     &entryID,
		EventType:   "view",
	})
	writeJSON(w, http.StatusOK, entry)
}

func (h *HiveHandler) Update(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	entryID, err := parseUUID(r, "entryId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entry id")
		return
	}
	var req struct {
		Content string   `json:"content"`
		Summary string   `json:"summary"`
		Tags    []string `json:"tags"`
	}
	decodeJSON(r, &req)
	entry, err := h.mem.Update(r.Context(), entryID, hsID, req.Content, req.Summary, req.Tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.Content != "" {
		h.worker.Enqueue(embed.Job{EntryID: entry.ID, Content: req.Content})
	}
	_ = h.hub.Publish(r.Context(), models.StreamEvent{
		Type:        "hive_updated",
		HiveshareID: hsID,
		Payload:     entry,
	})
	writeJSON(w, http.StatusOK, entry)
}

// Delete removes a hive entry. Only write-access members may delete. Cleans up
// the Redis view counter and publishes a hive_deleted SSE event on success.
func (h *HiveHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	entryID, err := parseUUID(r, "entryId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entry id")
		return
	}
	found, err := h.mem.Delete(r.Context(), entryID, hsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "hive not found")
		return
	}

	h.views.Delete(r.Context(), entryID)

	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EntryID:     &entryID,
		EventType:   "delete",
		Metadata:    map[string]interface{}{},
	})

	_ = h.hub.Publish(r.Context(), models.StreamEvent{
		Type:        "hive_deleted",
		HiveshareID: hsID,
		Payload:     map[string]string{"id": entryID.String()},
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *HiveHandler) Search(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	var req struct {
		Query      string `json:"query"`
		SourceType string `json:"source_type"`
		Limit      int    `json:"limit"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if req.Limit == 0 {
		req.Limit = 10
	}

	var entries []*models.Hive
	var err error

	// try vector search first
	if emb, embErr := h.embedder.Embed(r.Context(), req.Query); embErr == nil && len(emb) > 0 {
		entries, err = h.mem.SearchVector(r.Context(), hsID, emb, req.SourceType, req.Limit)
	} else {
		entries, err = h.mem.SearchFullText(r.Context(), hsID, req.Query, req.SourceType, req.Limit)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []*models.Hive{}
	}

	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EventType:   "search",
		Metadata:    map[string]interface{}{"query": req.Query},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": entries,
		"count":   len(entries),
		"query":   req.Query,
	})
}

func (h *HiveHandler) Stream(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	h.hub.ServeSSE(w, r, hsID)
}
