package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sagpaul/hiveshare/internal/embed"
	"github.com/sagpaul/hiveshare/internal/realtime"
	"github.com/sagpaul/hiveshare/internal/store"
)

// userStoreKey is used to pass the user store through context for invite acceptance.
type userStoreKey struct{}

func NewRouter(
	userStore *store.UserStore,
	hsStore *store.HiveshareStore,
	memStore *store.MemoryStore,
	metricsStore *store.MetricsStore,
	embedder embed.Embedder,
	hub *realtime.Hub,
) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// inject user store for invite acceptance
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), userStoreKey{}, userStore)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	auth := NewAuthHandler(userStore)
	hs := NewHiveshareHandler(hsStore, metricsStore)
	mem := NewMemoryHandler(memStore, hsStore, metricsStore, embedder, hub)
	met := NewMetricsHandler(metricsStore)

	r.Route("/api/v1", func(r chi.Router) {
		// public
		r.Post("/auth/register", auth.Register)
		r.Post("/invitations/{token}/accept", func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), "userStore", userStore)
			hs.AcceptInvite(w, req.WithContext(ctx))
		})

		// authenticated
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(userStore))

			r.Get("/auth/whoami", auth.Whoami)

			r.Get("/hiveshares", hs.List)
			r.Post("/hiveshares", hs.Create)
			r.Get("/hiveshares/{id}", hs.Get)
			r.Put("/hiveshares/{id}", hs.Update)
			r.Delete("/hiveshares/{id}", hs.Delete)

			r.Post("/hiveshares/{id}/invite", hs.Invite)
			r.Get("/hiveshares/{id}/members", hs.ListMembers)
			r.Delete("/hiveshares/{id}/members/{userId}", hs.RemoveMember)

			r.Get("/hiveshares/{id}/memory", mem.List)
			r.Post("/hiveshares/{id}/memory", mem.Create)
			r.Get("/hiveshares/{id}/memory/{entryId}", mem.Get)
			r.Put("/hiveshares/{id}/memory/{entryId}", mem.Update)
			r.Delete("/hiveshares/{id}/memory/{entryId}", mem.Delete)
			r.Post("/hiveshares/{id}/memory/search", mem.Search)

			r.Get("/hiveshares/{id}/metrics", hs.Metrics)
			r.Get("/hiveshares/{id}/stream", mem.Stream)

			r.Get("/metrics/me", met.UserMetrics)
		})
	})

	return r
}
