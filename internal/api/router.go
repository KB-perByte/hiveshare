package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/KB-perByte/hiveshare/internal/embed"
	"github.com/KB-perByte/hiveshare/internal/realtime"
	"github.com/KB-perByte/hiveshare/internal/store"
	"github.com/KB-perByte/hiveshare/internal/version"
)

// userStoreKey is used to pass the user store through context for invite acceptance.
type userStoreKey struct{}

func NewRouter(
	userStore *store.UserStore,
	hsStore *store.HiveshareStore,
	memStore *store.MemoryStore,
	metricsStore *store.MetricsStore,
	historyStore *store.HistoryStore,
	embedder embed.Embedder,
	hub *realtime.Hub,
	worker *embed.Worker,
	views *store.ViewCounter,
	pool *pgxpool.Pool,
	rdb *redis.Client,
) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestSize(1 << 20)) // 1MB body cap
	r.Use(httprate.Limit(60, time.Minute,
		httprate.WithKeyFuncs(apiKeyOrIP),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		}),
	))

	// inject user store for invite acceptance
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), userStoreKey{}, userStore)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	auth := NewAuthHandler(userStore)
	hs := NewHiveshareHandler(hsStore, metricsStore)
	mem := NewMemoryHandler(memStore, hsStore, metricsStore, historyStore, embedder, hub, worker, views)
	met := NewMetricsHandler(metricsStore)

	r.Get("/health", healthHandler(pool, rdb))

	r.Route("/api/v1", func(r chi.Router) {
		// public
		r.With(middleware.Timeout(30*time.Second)).Post("/auth/register", auth.Register)
		r.With(middleware.Timeout(30*time.Second)).Post("/invitations/{token}/accept", hs.AcceptInvite)

		// authenticated
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(userStore))

			r.Group(func(r chi.Router) {
				r.Use(middleware.Timeout(30 * time.Second))

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
				r.Get("/hiveshares/{id}/memory/{entryId}/history", mem.ListHistory)
				r.Post("/hiveshares/{id}/memory/{entryId}/rollback", mem.Rollback)
				r.Post("/hiveshares/{id}/memory/undelete", mem.Undelete)
				r.Post("/hiveshares/{id}/memory/copy", mem.CopyEntries)

				r.Post("/hiveshares/{id}/snapshots", mem.CreateSnapshot)
				r.Get("/hiveshares/{id}/snapshots", mem.ListSnapshots)
				r.Get("/hiveshares/{id}/snapshots/{snapshotId}", mem.GetSnapshot)
				r.Post("/hiveshares/{id}/snapshots/{snapshotId}/restore", mem.RestoreSnapshot)
				r.Delete("/hiveshares/{id}/snapshots/{snapshotId}", mem.DeleteSnapshot)

				r.Get("/hiveshares/{id}/metrics", hs.Metrics)
				r.Get("/metrics/me", met.UserMetrics)
			})

			// SSE must not use the request timeout middleware
			r.Get("/hiveshares/{id}/stream", mem.Stream)
		})
	})

	return r
}

func apiKeyOrIP(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != "" {
			return "key:" + token, nil
		}
	}
	return httprate.KeyByRealIP(r)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration", time.Since(start),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

func healthHandler(pool *pgxpool.Pool, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := http.StatusOK
		body := map[string]string{
			"status":     "ok",
			"db":         "ok",
			"redis":      "ok",
			"commit":     version.Commit,
			"build_time": version.BuildTime,
		}

		if err := pool.Ping(ctx); err != nil {
			status = http.StatusServiceUnavailable
			body["status"] = "degraded"
			body["db"] = "unavailable"
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			status = http.StatusServiceUnavailable
			body["status"] = "degraded"
			body["redis"] = "unavailable"
		}

		writeJSON(w, status, body)
	}
}
