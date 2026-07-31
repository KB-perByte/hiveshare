package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/KB-perByte/hiveshare/internal/store"
)

// ServiceAccountHandler handles creation and token minting for service accounts.
type ServiceAccountHandler struct {
	sa *store.ServiceAccountStore
	hs *store.HiveshareStore
}

func NewServiceAccountHandler(sa *store.ServiceAccountStore, hs *store.HiveshareStore) *ServiceAccountHandler {
	return &ServiceAccountHandler{sa: sa, hs: hs}
}

// Create creates a new service account for a hiveshare. Requires all role.
func (h *ServiceAccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hiveshare id")
		return
	}
	role, _ := h.hs.IsMember(r.Context(), id, u.ID)
	if !models.CanWrite(role) {
		writeError(w, http.StatusForbidden, "all access required to create service accounts")
		return
	}
	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	req.Role = models.NormalizeRole(req.Role)
	if req.Role != models.RoleAll && req.Role != models.RoleView {
		req.Role = models.RoleView
	}
	sa, err := h.sa.Create(r.Context(), id, u.ID, req.Name, req.Role)
	if err != nil {
		writeDBError(w, "could not create service account", err)
		return
	}
	writeJSON(w, http.StatusCreated, sa)
}

// List returns all service accounts for a hiveshare. Requires all role.
func (h *ServiceAccountHandler) List(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hiveshare id")
		return
	}
	role, _ := h.hs.IsMember(r.Context(), id, u.ID)
	if !models.CanWrite(role) {
		writeError(w, http.StatusForbidden, "all access required")
		return
	}
	accounts, err := h.sa.List(r.Context(), id)
	if err != nil {
		writeDBError(w, "could not list service accounts", err)
		return
	}
	if accounts == nil {
		accounts = []*models.ServiceAccount{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

// Delete removes a service account. Requires all role.
func (h *ServiceAccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	hsID, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hiveshare id")
		return
	}
	saID, err := parseUUID(r, "saId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service account id")
		return
	}
	role, _ := h.hs.IsMember(r.Context(), hsID, u.ID)
	if !models.CanWrite(role) {
		writeError(w, http.StatusForbidden, "all access required")
		return
	}
	found, err := h.sa.Delete(r.Context(), saID, hsID)
	if err != nil {
		writeDBError(w, "could not delete service account", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "service account not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MintToken exchanges a service account key for a short-lived JWT.
// The key is passed as Authorization: Bearer hvsa_... .
// No user auth middleware wraps this route — the SA key is the credential.
func (h *ServiceAccountHandler) MintToken(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	key := strings.TrimPrefix(auth, "Bearer ")
	if key == "" || key == auth {
		writeError(w, http.StatusUnauthorized, "service account key required")
		return
	}
	sa, err := h.sa.GetByKey(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid service account key")
		return
	}
	go h.sa.TouchLastUsed(r.Context(), sa.ID)

	ttl := saTokenTTL()
	claims := jwt.MapClaims{
		"sub":          sa.ID.String(),
		"hiveshare_id": sa.HiveshareID.String(),
		"role":         sa.Role,
		"exp":          time.Now().Add(ttl).Unix(),
		"iat":          time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not sign token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      signed,
		"expires_in": int(ttl.Seconds()),
		"role":       sa.Role,
	})
}

// ValidateServiceAccountJWT validates a JWT and returns (hiveshareID, role, error).
func ValidateServiceAccountJWT(tokenStr string) (string, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return jwtSecret(), nil
	})
	if err != nil || !token.Valid {
		return "", "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", jwt.ErrTokenInvalidClaims
	}
	hsID, _ := claims["hiveshare_id"].(string)
	role, _ := claims["role"].(string)
	return hsID, role, nil
}

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "change-me-in-production"
	}
	return []byte(s)
}

func saTokenTTL() time.Duration {
	if v := os.Getenv("SA_TOKEN_TTL_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 15 * time.Minute
}
