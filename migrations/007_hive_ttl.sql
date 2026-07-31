-- 007: per-record expiry for volatile hives (CI status, PR state, etc.)
-- NULL = never expires; set expires_at or pass ttl_seconds at write time.
ALTER TABLE hives ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
