// Package store provides PostgreSQL-backed persistence for all hiveshare domain
// objects. Each Store type is constructed with a pgxpool.Pool and is safe for
// concurrent use.
package store

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens and validates a connection pool using DATABASE_URL or individual
// POSTGRES_* environment variables. Returns a ready-to-use pool or an error if
// the database is unreachable.
func NewPool(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := getenv("POSTGRES_HOST", "localhost")
		port := getenv("POSTGRES_PORT", "5432")
		user := getenv("POSTGRES_USER", "hiveshare")
		pass := getenv("POSTGRES_PASSWORD", "hiveshare")
		dbname := getenv("POSTGRES_DB", "hiveshare")
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, dbname)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}

	cfg.MaxConns = 20
	cfg.MinConns = 4
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	// Improve ivfflat recall when that index is in use (harmless no-op failure with HNSW-only setups).
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, _ = conn.Exec(ctx, "SET ivfflat.probes = 10")
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
