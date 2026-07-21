-- vector similarity index (HNSW: no probe tuning, better recall than ivfflat)
CREATE INDEX IF NOT EXISTS memory_entries_embedding_idx
    ON memory_entries USING hnsw (embedding vector_cosine_ops);

-- source lookup index
CREATE INDEX IF NOT EXISTS memory_entries_source_idx
    ON memory_entries (hiveshare_id, source_type, source_ref);

-- chronological listing
CREATE INDEX IF NOT EXISTS memory_entries_time_idx
    ON memory_entries (hiveshare_id, created_at DESC);

-- full-text search fallback
CREATE INDEX IF NOT EXISTS memory_entries_fts_idx
    ON memory_entries USING gin(to_tsvector('english', content));

-- fast membership checks
CREATE INDEX IF NOT EXISTS hiveshare_members_user_idx
    ON hiveshare_members (user_id);

-- invite lookup by token
CREATE INDEX IF NOT EXISTS invitations_token_idx
    ON invitations (token);

-- metrics queries
CREATE INDEX IF NOT EXISTS usage_events_hiveshare_time_idx
    ON usage_events (hiveshare_id, created_at DESC);
CREATE INDEX IF NOT EXISTS usage_events_user_time_idx
    ON usage_events (user_id, created_at DESC);

-- supports rolling TTL deletes on usage_events
CREATE INDEX IF NOT EXISTS usage_events_created_at_idx
    ON usage_events (created_at);
