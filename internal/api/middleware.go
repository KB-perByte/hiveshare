package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/sagpaul/hiveshare/internal/models"
	"github.com/sagpaul/hiveshare/internal/store"
)

type contextKey string

const ctxUser contextKey = "user"

func AuthMiddleware(us *store.UserStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if token == "" || token == auth {
				writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
				return
			}
			user, err := us.GetByAPIKey(r.Context(), token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			ctx := context.WithValue(r.Context(), ctxUser, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func currentUser(r *http.Request) *models.User {
	u, _ := r.Context().Value(ctxUser).(*models.User)
	return u
}
