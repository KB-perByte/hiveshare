-- Rename memory_entries table to hives.
-- Table rename is metadata-only in PostgreSQL — no data movement, safe on a live DB.
-- The FK from usage_events.entry_id follows the OID automatically.

ALTER TABLE memory_entries RENAME TO hives;

-- Rename indexes for consistency
ALTER INDEX memory_entries_embedding_idx RENAME TO hives_embedding_idx;
ALTER INDEX memory_entries_source_idx    RENAME TO hives_source_idx;
ALTER INDEX memory_entries_time_idx      RENAME TO hives_time_idx;
ALTER INDEX memory_entries_fts_idx       RENAME TO hives_fts_idx;

-- Enforce one hive per (hiveshare, source_ref).
-- On conflict the API auto-suffixes the ref (e.g. PROJ-123-2).
ALTER TABLE hives
    ADD CONSTRAINT hives_hiveshare_source_ref_key UNIQUE (hiveshare_id, source_ref);
