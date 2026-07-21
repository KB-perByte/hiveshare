package embed

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// Job is an async embedding request for a memory entry.
type Job struct {
	EntryID uuid.UUID
	Content string
}

// EmbeddingUpdater persists a computed embedding for an entry.
type EmbeddingUpdater interface {
	UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error
}

// Worker pools embedding work so HTTP handlers are not blocked on the embed API.
type Worker struct {
	embedder Embedder
	updater  EmbeddingUpdater
	jobs     chan Job
}

// NewWorker creates a buffered worker pool. Call Start to begin processing.
func NewWorker(embedder Embedder, updater EmbeddingUpdater, workers, queueSize int) *Worker {
	if workers < 1 {
		workers = 2
	}
	if queueSize < 1 {
		queueSize = 64
	}
	return &Worker{
		embedder: embedder,
		updater:  updater,
		jobs:     make(chan Job, queueSize),
	}
}

// Start launches worker goroutines. They exit when ctx is cancelled.
func (w *Worker) Start(ctx context.Context, n int) {
	if n < 1 {
		n = 2
	}
	for i := 0; i < n; i++ {
		go w.loop(ctx)
	}
}

// Enqueue submits embedding work. If the queue is full the job is dropped
// and search falls back to full-text until a later re-embed.
func (w *Worker) Enqueue(job Job) {
	if w == nil {
		return
	}
	select {
	case w.jobs <- job:
	default:
		slog.Warn("embed worker: queue full, dropping job", "entry_id", job.EntryID)
	}
}

func (w *Worker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.jobs:
			w.process(ctx, job)
		}
	}
}

func (w *Worker) process(ctx context.Context, job Job) {
	vec, err := w.embedder.Embed(ctx, job.Content)
	if err != nil || len(vec) == 0 {
		if err != nil {
			slog.Warn("embed worker: embed failed", "entry_id", job.EntryID, "err", err)
		}
		return
	}
	if err := w.updater.UpdateEmbedding(ctx, job.EntryID, vec); err != nil {
		slog.Warn("embed worker: update failed", "entry_id", job.EntryID, "err", err)
	}
}
