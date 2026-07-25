// Package api implements the JSON HTTP API handlers and middleware for the
// hiveshare service. All handlers read the authenticated user from context
// (set by AuthMiddleware) and write JSON responses.
package api

import (
	"net/http"

	"github.com/KB-perByte/hiveshare/internal/store"
)

// AuthHandler handles user registration and identity endpoints.
type AuthHandler struct {
	users *store.UserStore
}

// NewAuthHandler returns an AuthHandler backed by the given user store.
func NewAuthHandler(users *store.UserStore) *AuthHandler {
	return &AuthHandler{users: users}
}

// Register creates a new user account and returns the cleartext API key.
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

// Whoami returns the authenticated user's profile.
func (h *AuthHandler) Whoami(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	writeJSON(w, http.StatusOK, u)
}
