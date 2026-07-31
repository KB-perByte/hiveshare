package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/KB-perByte/hiveshare/internal/api"
	"github.com/KB-perByte/hiveshare/internal/embed"
	"github.com/KB-perByte/hiveshare/internal/realtime"
	"github.com/KB-perByte/hiveshare/internal/store"
	"github.com/KB-perByte/hiveshare/internal/version"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// database
	pool, err := store.NewPool(ctx)
	if err != nil {
		slog.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// redis
	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "redis://localhost:6379"
	}
	opt, err := redis.ParseURL(redisAddr)
	if err != nil {
		slog.Error("redis url", "err", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(opt)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("redis ping", "err", err)
		os.Exit(1)
	}

	// layers
	userStore := store.NewUserStore(pool)
	hsStore := store.NewHiveshareStore(pool)
	hiveStore := store.NewHiveStore(pool)
	metricsStore := store.NewMetricsStore(pool)
	embedder := embed.New()
	hub := realtime.NewHub(rdb)
	views := store.NewViewCounter(rdb, pool)
	views.StartFlusher(ctx, 60*time.Second)

	historyStore := store.NewHistoryStore(pool)
	saStore := store.NewServiceAccountStore(pool)

	worker := embed.NewWorker(embedder, hiveStore, 2, 64)
	worker.Start(ctx, 2)

	// rolling TTL for usage_events (retain 90 days)
	go purgeUsageEvents(ctx, metricsStore)

	// optional history purge
	historyTTLDays := envInt("HISTORY_TTL_DAYS", 0)
	historyMaxVersions := envInt("HISTORY_MAX_VERSIONS", 0)
	if historyTTLDays > 0 || historyMaxVersions > 0 {
		go purgeHistory(ctx, historyStore, historyTTLDays, historyMaxVersions)
	}

	router := api.NewRouter(userStore, hsStore, hiveStore, metricsStore, historyStore, embedder, hub, worker, views, pool, rdb, saStore)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{
		Addr: addr,
		Handler: router,
		// ReadHeaderTimeout bounds slowloris; do NOT set WriteTimeout globally —
		// it kills long-lived SSE (/stream) after N seconds with unexpected EOF.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	slog.Info("hiveshare server listening",
		"addr", addr,
		"commit", version.Commit,
		"build_time", version.BuildTime,
	)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func purgeHistory(ctx context.Context, hs *store.HistoryStore, ttlDays, maxVersions int) {
	run := func() {
		if ttlDays > 0 {
			n, err := hs.PurgeByAge(ctx, time.Duration(ttlDays)*24*time.Hour)
			if err != nil {
				slog.Warn("history age purge failed", "err", err)
			} else if n > 0 {
				slog.Info("history purged by age", "rows", n)
			}
		}
		if maxVersions > 0 {
			n, err := hs.PurgeByCount(ctx, maxVersions)
			if err != nil {
				slog.Warn("history count purge failed", "err", err)
			} else if n > 0 {
				slog.Info("history purged by count", "rows", n)
			}
		}
	}
	run()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func purgeUsageEvents(ctx context.Context, metrics *store.MetricsStore) {
	run := func() {
		n, err := metrics.PurgeOldUsageEvents(ctx, 90*24*time.Hour)
		if err != nil {
			slog.Warn("usage_events purge failed", "err", err)
			return
		}
		if n > 0 {
			slog.Info("usage_events purged", "rows", n)
		}
	}
	run()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
