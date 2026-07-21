package api

import (
	"net/http"

	"github.com/KB-perByte/hiveshare/internal/store"
)

type AuthHandler struct {
	users *store.UserStore
}

func NewAuthHandler(users *store.UserStore) *AuthHandler {
	return &AuthHandler{users: users}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Email == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "email and name are required")
		return
	}
	user, err := h.users.Create(r.Context(), req.Email, req.Name)
	if err != nil {
		writeError(w, http.StatusConflict, "email already registered or internal error")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *AuthHandler) Whoami(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	u.APIKey = "" // don't echo the key back on whoami
	writeJSON(w, http.StatusOK, u)
}
