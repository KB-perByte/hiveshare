package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/KB-perByte/hiveshare/internal/embed"
	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/KB-perByte/hiveshare/internal/realtime"
	"github.com/KB-perByte/hiveshare/internal/store"
)

type MemoryHandler struct {
	mem      *store.MemoryStore
	hs       *store.HiveshareStore
	metrics  *store.MetricsStore
	embedder embed.Embedder
	hub      *realtime.Hub
	worker   *embed.Worker
	views    *store.ViewCounter
}

func NewMemoryHandler(
	mem *store.MemoryStore,
	hs *store.HiveshareStore,
	metrics *store.MetricsStore,
	embedder embed.Embedder,
	hub *realtime.Hub,
	worker *embed.Worker,
	views *store.ViewCounter,
) *MemoryHandler {
	return &MemoryHandler{
		mem: mem, hs: hs, metrics: metrics, embedder: embedder,
		hub: hub, worker: worker, views: views,
	}
}

func (h *MemoryHandler) requireAccess(r *http.Request, w http.ResponseWriter, minRole string) (uuid.UUID, bool) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hiveshare id")
		return uuid.Nil, false
	}
	role, _ := h.hs.IsMember(r.Context(), id, u.ID)
	if role == "" {
		writeError(w, http.StatusForbidden, "not a member of this hiveshare")
		return uuid.Nil, false
	}
	if minRole == "member" && role == "viewer" {
		writeError(w, http.StatusForbidden, "viewer role cannot write")
		return uuid.Nil, false
	}
	return id, true
}

func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, "viewer")
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 50
	}
	entries, err := h.mem.List(r.Context(), hsID, store.ListMemoryFilter{
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
		entries = []*models.MemoryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *MemoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, "member")
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

	entry := &models.MemoryEntry{
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
	created, err := h.mem.Create(r.Context(), entry, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
		Type:        "memory_added",
		HiveshareID: hsID,
		Payload:     created,
	})

	writeJSON(w, http.StatusCreated, created)
}

func (h *MemoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, "viewer")
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

func (h *MemoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, "member")
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
		Type:        "memory_updated",
		HiveshareID: hsID,
		Payload:     entry,
	})
	writeJSON(w, http.StatusOK, entry)
}

func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, "member")
	if !ok {
		return
	}
	entryID, err := parseUUID(r, "entryId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entry id")
		return
	}
	_ = h.mem.Delete(r.Context(), entryID, hsID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *MemoryHandler) Search(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, "viewer")
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

	var entries []*models.MemoryEntry
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
		entries = []*models.MemoryEntry{}
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

func (h *MemoryHandler) Stream(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, "viewer")
	if !ok {
		return
	}
	h.hub.ServeSSE(w, r, hsID)
}
