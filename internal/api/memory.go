package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/KB-perByte/hiveshare/internal/embed"
	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/KB-perByte/hiveshare/internal/realtime"
	"github.com/KB-perByte/hiveshare/internal/store"
)

type HiveHandler struct {
	mem      *store.HiveStore
	hs       *store.HiveshareStore
	metrics  *store.MetricsStore
	history  *store.HistoryStore
	embedder embed.Embedder
	hub      *realtime.Hub
	worker   *embed.Worker
	views    *store.ViewCounter
}

func NewHiveHandler(
	mem *store.HiveStore,
	hs *store.HiveshareStore,
	metrics *store.MetricsStore,
	history *store.HistoryStore,
	embedder embed.Embedder,
	hub *realtime.Hub,
	worker *embed.Worker,
	views *store.ViewCounter,
) *HiveHandler {
	return &HiveHandler{
		mem: mem, hs: hs, metrics: metrics, history: history, embedder: embedder,
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
	// Service account tokens carry their role in the JWT; skip the DB membership check.
	role := u.SARole
	if role == "" {
		role, _ = h.hs.IsMember(r.Context(), id, u.ID)
	}
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
		writeDBError(w, "could not list hives", err)
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
		ExpiresAt  *time.Time             `json:"expires_at"`
		TTLSeconds int                    `json:"ttl_seconds"`
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

	if req.TTLSeconds > 0 {
		t := time.Now().Add(time.Duration(req.TTLSeconds) * time.Second)
		req.ExpiresAt = &t
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
		ExpiresAt:   req.ExpiresAt,
	}

	// Semantic dedup: opt-in via ?dedup_threshold=0.95 (default 0 = disabled).
	// Disabled by default because the synchronous embed round-trip adds 200–500ms
	// latency to every create when an embedding provider is configured.
	var cachedEmbedding []float32
	dedupThreshold := 0.0
	if t := r.URL.Query().Get("dedup_threshold"); t != "" {
		if v, parseErr := strconv.ParseFloat(t, 64); parseErr == nil {
			dedupThreshold = v
		}
	}
	if dedupThreshold > 0 {
		if emb, embErr := h.embedder.Embed(r.Context(), req.Content); embErr == nil && len(emb) > 0 {
			cachedEmbedding = emb
			if existing, findErr := h.mem.FindSimilar(r.Context(), hsID, req.SourceRef, emb, dedupThreshold); findErr == nil && existing != nil {
				writeJSON(w, http.StatusConflict, map[string]interface{}{
					"error":      "similar hive already exists",
					"existing":   existing,
					"similarity": existing.Score,
				})
				return
			}
		}
	}

	// Store immediately; pass cached embedding if available to skip async embed.
	// Auto-suffix source_ref (e.g. PROJ-123-2) if not unique within this hiveshare.
	baseRef := entry.SourceRef
	var created *models.Hive
	var err error
	for n := 0; ; n++ {
		if n > 0 {
			entry.SourceRef = fmt.Sprintf("%s-%d", baseRef, n+1)
		}
		created, err = h.mem.Create(r.Context(), entry, cachedEmbedding)
		if err == nil {
			break
		}
		if n >= 9 || !store.IsUniqueViolation(err) {
			writeDBError(w, "could not create hive", err)
			return
		}
	}
	created.UserName = u.Name

	if len(cachedEmbedding) == 0 {
		h.worker.Enqueue(embed.Job{EntryID: created.ID, Content: req.Content})
	}

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
		writeDBError(w, "hive not found", err)
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
		Content    string     `json:"content"`
		Summary    string     `json:"summary"`
		Tags       []string   `json:"tags"`
		ExpiresAt  *time.Time `json:"expires_at"`
		TTLSeconds int        `json:"ttl_seconds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.TTLSeconds > 0 {
		t := time.Now().Add(time.Duration(req.TTLSeconds) * time.Second)
		req.ExpiresAt = &t
	}
	entry, err := h.mem.Update(r.Context(), entryID, hsID, req.Content, req.Summary, req.Tags, req.ExpiresAt)
	if err != nil {
		writeDBError(w, "could not update hive", err)
		return
	}
	h.worker.Enqueue(embed.Job{EntryID: entry.ID, Content: req.Content})
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
		writeDBError(w, "could not delete hive", err)
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
		Query       string  `json:"query"`
		SourceType  string  `json:"source_type"`
		Limit       int     `json:"limit"`
		Alpha       float64 `json:"alpha"` // 0=full-text only, 1=vector only, default 0.7
		MaxAgeSecs  int     `json:"max_age_seconds"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if req.Limit == 0 {
		req.Limit = 10
	}
	if req.Alpha == 0 {
		req.Alpha = 0.7
	}

	var entries []*models.Hive
	var err error
	searchType := "fulltext"

	if emb, embErr := h.embedder.Embed(r.Context(), req.Query); embErr == nil && len(emb) > 0 {
		searchType = "hybrid"
		entries, err = h.mem.SearchHybrid(r.Context(), hsID, emb, req.Query, req.SourceType, req.Alpha, req.Limit, req.MaxAgeSecs)
	} else {
		entries, err = h.mem.SearchFullText(r.Context(), hsID, req.Query, req.SourceType, req.Limit, req.MaxAgeSecs)
	}
	if err != nil {
		writeDBError(w, "search failed", err)
		return
	}
	if entries == nil {
		entries = []*models.Hive{}
	}

	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EventType:   "search",
		Metadata:    map[string]interface{}{"query": req.Query, "type": searchType},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": entries,
		"count":   len(entries),
		"query":   req.Query,
		"type":    searchType,
	})
}

func (h *HiveHandler) Stream(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	h.hub.ServeSSE(w, r, hsID)
}

// ── Per-entry history ────────────────────────────────────────────────────────

func (h *HiveHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	entryID, err := parseUUID(r, "entryId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entry id")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	versions, err := h.history.ListVersions(r.Context(), entryID, hsID, limit, offset)
	if err != nil {
		writeDBError(w, "could not load history", err)
		return
	}
	if versions == nil {
		versions = []*models.HistoryEntry{}
	}
	writeJSON(w, http.StatusOK, versions)
}

func (h *HiveHandler) Rollback(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		HistoryID int64 `json:"history_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.HistoryID == 0 {
		writeError(w, http.StatusBadRequest, "history_id is required")
		return
	}
	entry, hasEmb, err := h.history.Rollback(r.Context(), entryID, hsID, req.HistoryID)
	if err != nil {
		writeDBError(w, "rollback failed", err)
		return
	}
	if !hasEmb {
		h.worker.Enqueue(embed.Job{EntryID: entry.ID, Content: entry.Content})
	}
	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EntryID:     &entry.ID,
		EventType:   "rollback",
	})
	_ = h.hub.Publish(r.Context(), models.StreamEvent{
		Type:        "hive_rolled_back",
		HiveshareID: hsID,
		Payload:     entry,
	})
	writeJSON(w, http.StatusOK, entry)
}

func (h *HiveHandler) Undelete(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	var req struct {
		HistoryID int64 `json:"history_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.HistoryID == 0 {
		writeError(w, http.StatusBadRequest, "history_id is required")
		return
	}
	entry, hasEmb, err := h.history.Undelete(r.Context(), req.HistoryID, hsID)
	if err != nil {
		writeDBError(w, "undelete failed", err)
		return
	}
	if !hasEmb {
		h.worker.Enqueue(embed.Job{EntryID: entry.ID, Content: entry.Content})
	}
	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EntryID:     &entry.ID,
		EventType:   "undelete",
	})
	_ = h.hub.Publish(r.Context(), models.StreamEvent{
		Type:        "hive_undeleted",
		HiveshareID: hsID,
		Payload:     entry,
	})
	writeJSON(w, http.StatusCreated, entry)
}

// ── Snapshots ────────────────────────────────────────────────────────────────

func (h *HiveHandler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	snap, err := h.history.CreateSnapshot(r.Context(), hsID, u.ID, req.Name, req.Description)
	if err != nil {
		if errors.Is(err, store.ErrSnapshotTooLarge) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeDBError(w, "could not create snapshot", err)
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

func (h *HiveHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	snaps, err := h.history.ListSnapshots(r.Context(), hsID)
	if err != nil {
		writeDBError(w, "could not list snapshots", err)
		return
	}
	if snaps == nil {
		snaps = []*models.Snapshot{}
	}
	writeJSON(w, http.StatusOK, snaps)
}

func (h *HiveHandler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, false)
	if !ok {
		return
	}
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "snapshotId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid snapshot id")
		return
	}
	snap, entries, err := h.history.GetSnapshot(r.Context(), snapshotID, hsID)
	if err != nil {
		writeDBError(w, "snapshot not found", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"snapshot": snap,
		"entries":  entries,
	})
}

func (h *HiveHandler) RestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "snapshotId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid snapshot id")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		req.Name = "(restored)"
	}
	result, err := h.history.RestoreSnapshot(r.Context(), snapshotID, hsID, u.ID, req.Name)
	if err != nil {
		writeDBError(w, "restore failed", err)
		return
	}
	for _, id := range result.NullEmbeddings {
		entry, getErr := h.mem.Get(r.Context(), id, result.Hiveshare.ID)
		if getErr == nil {
			h.worker.Enqueue(embed.Job{EntryID: id, Content: entry.Content})
		}
	}
	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:    u.ID,
		EventType: "snapshot_restore",
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"hiveshare":        result.Hiveshare,
		"entries_restored": result.EntriesCreated,
	})
}

func (h *HiveHandler) DeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "snapshotId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid snapshot id")
		return
	}
	deleted, err := h.history.DeleteSnapshot(r.Context(), snapshotID, hsID)
	if err != nil {
		writeDBError(w, "could not delete snapshot", err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "snapshot not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Copy entries ─────────────────────────────────────────────────────────────

func (h *HiveHandler) CopyEntries(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, ok := h.requireAccess(r, w, true)
	if !ok {
		return
	}
	var req struct {
		EntryIDs []uuid.UUID `json:"entry_ids"`
	}
	if err := decodeJSON(r, &req); err != nil || len(req.EntryIDs) == 0 {
		writeError(w, http.StatusBadRequest, "entry_ids is required")
		return
	}
	results, err := h.history.CopyEntries(r.Context(), hsID, u.ID, req.EntryIDs)
	if err != nil {
		if errors.Is(err, store.ErrForbidden) {
			writeError(w, http.StatusForbidden, "not a member of source hiveshare")
			return
		}
		writeDBError(w, "could not copy hives", err)
		return
	}
	var entries []*models.Hive
	for _, cr := range results {
		entries = append(entries, cr.Entry)
		if !cr.HasEmbedding {
			h.worker.Enqueue(embed.Job{EntryID: cr.Entry.ID, Content: cr.Entry.Content})
		}
	}
	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &hsID,
		EventType:   "copy",
	})
	writeJSON(w, http.StatusCreated, entries)
}

