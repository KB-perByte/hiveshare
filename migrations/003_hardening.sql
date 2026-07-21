-- Migrate embedding index from ivfflat → HNSW (better recall, no probes tuning).
-- Safe to re-run: drops then recreates.
DROP INDEX IF EXISTS memory_entries_embedding_idx;
CREATE INDEX IF NOT EXISTS memory_entries_embedding_idx
    ON memory_entries USING hnsw (embedding vector_cosine_ops);

-- Supports rolling TTL deletes on usage_events
CREATE INDEX IF NOT EXISTS usage_events_created_at_idx
    ON usage_events (created_at);
