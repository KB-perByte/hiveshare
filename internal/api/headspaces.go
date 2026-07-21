package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/KB-perByte/hiveshare/internal/store"
)

type HiveshareHandler struct {
	hs      *store.HiveshareStore
	metrics *store.MetricsStore
}

func NewHiveshareHandler(hs *store.HiveshareStore, metrics *store.MetricsStore) *HiveshareHandler {
	return &HiveshareHandler{hs: hs, metrics: metrics}
}

func (h *HiveshareHandler) List(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	list, err := h.hs.ListForUser(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*models.Hiveshare{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *HiveshareHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	hs, err := h.hs.Create(r.Context(), req.Name, req.Description, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, hs)
}

func (h *HiveshareHandler) Get(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	hs, err := h.hs.Get(r.Context(), id, u.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "hiveshare not found")
		return
	}
	writeJSON(w, http.StatusOK, hs)
}

func (h *HiveshareHandler) Update(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	// only owner/member can edit
	role, err := h.hs.IsMember(r.Context(), id, u.ID)
	if err != nil || role == "" || role == "viewer" {
		writeError(w, http.StatusForbidden, "not authorized")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	decodeJSON(r, &req)
	hs, err := h.hs.Update(r.Context(), id, req.Name, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hs)
}

func (h *HiveshareHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	role, _ := h.hs.IsMember(r.Context(), id, u.ID)
	if role != "owner" {
		writeError(w, http.StatusForbidden, "only owner can delete")
		return
	}
	if err := h.hs.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HiveshareHandler) Invite(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	role, _ := h.hs.IsMember(r.Context(), id, u.ID)
	if role == "" || role == "viewer" {
		writeError(w, http.StatusForbidden, "not authorized to invite")
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}

	tokenBytes := make([]byte, 24)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	inv := &models.Invitation{
		HiveshareID: id,
		Email:       req.Email,
		InvitedBy:   u.ID,
		Token:       token,
		Role:        req.Role,
	}
	if err := h.hs.CreateInvitation(r.Context(), inv); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	inv.InviteURL = baseURL + "/api/v1/invitations/" + token + "/accept"

	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      u.ID,
		HiveshareID: &id,
		EventType:   "invite_sent",
		Metadata:    map[string]interface{}{"email": req.Email},
	})

	writeJSON(w, http.StatusCreated, inv)
}

func (h *HiveshareHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, err := h.hs.GetInvitation(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "invitation not found")
		return
	}
	if inv.Status != "pending" || time.Now().After(inv.ExpiresAt) {
		writeError(w, http.StatusGone, "invitation expired or already used")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	decodeJSON(r, &req)
	if req.Name == "" {
		req.Name = inv.Email
	}

	// find or create user
	us, _ := r.Context().Value(userStoreKey{}).(*store.UserStore)
	if us == nil {
		writeError(w, http.StatusInternalServerError, "user store unavailable")
		return
	}
	user, err := us.GetByEmail(r.Context(), inv.Email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && !isNotFound(err) {
			writeError(w, http.StatusInternalServerError, "lookup failed: "+err.Error())
			return
		}
		user, err = us.Create(r.Context(), inv.Email, req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not create user: "+err.Error())
			return
		}
	}

	if err := h.hs.AddMember(r.Context(), inv.HiveshareID, user.ID, inv.InvitedBy, inv.Role); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.hs.AcceptInvitation(r.Context(), token)

	_ = h.metrics.RecordEvent(r.Context(), &models.UsageEvent{
		UserID:      user.ID,
		HiveshareID: &inv.HiveshareID,
		EventType:   "invite_accepted",
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":      "Welcome to " + inv.HiveShareName,
		"hiveshare_id": inv.HiveshareID,
		"user":         user,
	})
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, pgx.ErrNoRows)
}

func (h *HiveshareHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if role, _ := h.hs.IsMember(r.Context(), id, u.ID); role == "" {
		writeError(w, http.StatusForbidden, "not a member")
		return
	}
	members, err := h.hs.ListMembers(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *HiveshareHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	targetID, err := parseUUID(r, "userId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid userId")
		return
	}
	role, _ := h.hs.IsMember(r.Context(), id, u.ID)
	if role != "owner" && u.ID != targetID {
		writeError(w, http.StatusForbidden, "not authorized")
		return
	}
	_ = h.hs.RemoveMember(r.Context(), id, targetID)
	w.WriteHeader(http.StatusNoContent)
}

// Metrics delegates to the metrics handler.
func (h *HiveshareHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if role, _ := h.hs.IsMember(r.Context(), id, u.ID); role == "" {
		writeError(w, http.StatusForbidden, "not a member")
		return
	}
	m, err := h.metrics.HiveshareMetrics(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

