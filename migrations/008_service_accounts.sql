-- 008: service accounts for zero-interaction / CI agents (WIF bridge)
-- Long-lived key mints short-lived JWTs; no invite flow required.
CREATE TABLE IF NOT EXISTS service_accounts (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    hiveshare_id UUID        NOT NULL REFERENCES hiveshares(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL UNIQUE,
    role         TEXT        NOT NULL DEFAULT 'view' CHECK (role IN ('all','view')),
    created_by   UUID        NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);
