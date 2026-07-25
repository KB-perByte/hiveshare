-- Hive history: trigger-based audit trail for all content mutations.
-- Enables per-hive rollback/undelete and hiveshare-level snapshots.
-- Runs after 005_rename_memory_to_hive.sql (table is already named hives).

-- ── History table ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS hives_history (
    history_id   BIGSERIAL PRIMARY KEY,
    entry_id     UUID NOT NULL,
    hiveshare_id UUID NOT NULL,
    user_id      UUID NOT NULL,
    action       TEXT NOT NULL,
    content      TEXT,
    summary      TEXT,
    embedding    vector(1536),
    tags         TEXT[] NOT NULL DEFAULT '{}',
    metadata     JSONB NOT NULL DEFAULT '{}',
    source_type  TEXT,
    source_ref   TEXT,
    source_url   TEXT,
    tool         TEXT,
    recorded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT valid_history_action CHECK (action IN ('insert', 'update', 'delete'))
);

CREATE INDEX IF NOT EXISTS hives_history_entry_idx
    ON hives_history (entry_id, recorded_at DESC);

CREATE INDEX IF NOT EXISTS hives_history_hiveshare_idx
    ON hives_history (hiveshare_id, recorded_at DESC);

-- ── Trigger function ─────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION record_hive_history() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        INSERT INTO hives_history
            (entry_id, hiveshare_id, user_id, action, content, summary, embedding,
             tags, metadata, source_type, source_ref, source_url, tool)
        VALUES
            (OLD.id, OLD.hiveshare_id, OLD.user_id, 'delete', OLD.content, OLD.summary,
             OLD.embedding, OLD.tags, OLD.metadata, OLD.source_type, OLD.source_ref,
             OLD.source_url, OLD.tool);
        RETURN OLD;
    ELSE
        INSERT INTO hives_history
            (entry_id, hiveshare_id, user_id, action, content, summary, embedding,
             tags, metadata, source_type, source_ref, source_url, tool)
        VALUES
            (NEW.id, NEW.hiveshare_id, NEW.user_id, lower(TG_OP), NEW.content, NEW.summary,
             NEW.embedding, NEW.tags, NEW.metadata, NEW.source_type, NEW.source_ref,
             NEW.source_url, NEW.tool);
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- embedding is intentionally omitted from UPDATE OF: the async embed worker
-- fills it after insert and must not create a spurious "update" history row.
DROP TRIGGER IF EXISTS hive_history_trigger ON hives;
CREATE TRIGGER hive_history_trigger
    AFTER INSERT OR UPDATE OF content, summary, tags, metadata OR DELETE
    ON hives
    FOR EACH ROW EXECUTE FUNCTION record_hive_history();

-- ── Snapshot tables ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS hiveshare_snapshots (
    snapshot_id  BIGSERIAL PRIMARY KEY,
    hiveshare_id UUID NOT NULL REFERENCES hiveshares(id) ON DELETE CASCADE,
    created_by   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS hiveshare_snapshot_entries (
    snapshot_id  BIGINT NOT NULL REFERENCES hiveshare_snapshots(snapshot_id) ON DELETE CASCADE,
    entry_id     UUID NOT NULL,
    content      TEXT,
    summary      TEXT,
    embedding    vector(1536),
    tags         TEXT[] NOT NULL DEFAULT '{}',
    metadata     JSONB NOT NULL DEFAULT '{}',
    source_type  TEXT,
    source_ref   TEXT,
    source_url   TEXT,
    tool         TEXT,
    PRIMARY KEY (snapshot_id, entry_id)
);

CREATE INDEX IF NOT EXISTS hiveshare_snapshots_hs_idx
    ON hiveshare_snapshots (hiveshare_id, created_at DESC);

-- ── Backfill existing entries ────────────────────────────────────────────────

INSERT INTO hives_history
    (entry_id, hiveshare_id, user_id, action, content, summary, embedding,
     tags, metadata, source_type, source_ref, source_url, tool)
SELECT id, hiveshare_id, user_id, 'insert', content, summary, embedding,
       tags, metadata, source_type, source_ref, source_url, tool
FROM hives
WHERE NOT EXISTS (
    SELECT 1 FROM hives_history WHERE entry_id = hives.id
);
