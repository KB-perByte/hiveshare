package store

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const viewsKeyPrefix = "hiveshare:views:"

// ViewCounter increments views in Redis and flushes deltas to Postgres on a ticker,
// avoiding row-level lock contention on hot memory entries.
type ViewCounter struct {
	rdb *redis.Client
	db  *pgxpool.Pool
}

func NewViewCounter(rdb *redis.Client, db *pgxpool.Pool) *ViewCounter {
	return &ViewCounter{rdb: rdb, db: db}
}

// Increment records a view in Redis. Returns the pending delta (including this view).
func (v *ViewCounter) Increment(ctx context.Context, entryID uuid.UUID) (int64, error) {
	return v.rdb.Incr(ctx, viewsKeyPrefix+entryID.String()).Result()
}

// Pending returns the unflushed view delta for an entry.
func (v *ViewCounter) Pending(ctx context.Context, entryID uuid.UUID) int64 {
	n, err := v.rdb.Get(ctx, viewsKeyPrefix+entryID.String()).Int64()
	if err != nil {
		return 0
	}
	return n
}

// StartFlusher periodically writes Redis view deltas into Postgres.
func (v *ViewCounter) StartFlusher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				v.flush(context.Background())
				return
			case <-ticker.C:
				v.flush(ctx)
			}
		}
	}()
}

func (v *ViewCounter) flush(ctx context.Context) {
	var cursor uint64
	for {
		keys, next, err := v.rdb.Scan(ctx, cursor, viewsKeyPrefix+"*", 100).Result()
		if err != nil {
			slog.Warn("views flush: scan failed", "err", err)
			return
		}
		for _, key := range keys {
			v.flushKey(ctx, key)
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

func (v *ViewCounter) flushKey(ctx context.Context, key string) {
	n, err := v.rdb.GetDel(ctx, key).Int64()
	if err != nil || n == 0 {
		return
	}
	idStr := strings.TrimPrefix(key, viewsKeyPrefix)
	id, err := uuid.Parse(idStr)
	if err != nil {
		slog.Warn("views flush: bad key", "key", key)
		return
	}
	_, err = v.db.Exec(ctx,
		`UPDATE memory_entries SET views = views + $2 WHERE id = $1`, id, n,
	)
	if err != nil {
		// put the delta back so we don't lose counts
		_ = v.rdb.IncrBy(ctx, key, n).Err()
		slog.Warn("views flush: postgres update failed", "entry_id", id, "delta", n, "err", err)
		return
	}
	slog.Debug("views flush", "entry_id", id, "delta", strconv.FormatInt(n, 10))
}
