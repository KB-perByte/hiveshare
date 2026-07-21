package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/KB-perByte/hiveshare/internal/api"
	"github.com/KB-perByte/hiveshare/internal/embed"
	"github.com/KB-perByte/hiveshare/internal/realtime"
	"github.com/KB-perByte/hiveshare/internal/store"
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
	memStore := store.NewMemoryStore(pool)
	metricsStore := store.NewMetricsStore(pool)
	embedder := embed.New()
	hub := realtime.NewHub(rdb)
	views := store.NewViewCounter(rdb, pool)
	views.StartFlusher(ctx, 60*time.Second)

	worker := embed.NewWorker(embedder, memStore, 2, 64)
	worker.Start(ctx, 2)

	// rolling TTL for usage_events (retain 90 days)
	go purgeUsageEvents(ctx, metricsStore)

	router := api.NewRouter(userStore, hsStore, memStore, metricsStore, embedder, hub, worker, views, pool, rdb)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	slog.Info("hiveshare server listening", "addr", addr)
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
