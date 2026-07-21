CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "vector";

CREATE TABLE users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT UNIQUE NOT NULL,
    name       TEXT NOT NULL,
    api_key    TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE hiveshares (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT,
    owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    settings    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE hiveshare_members (
    hiveshare_id UUID NOT NULL REFERENCES hiveshares(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT 'all',
    invited_by   UUID REFERENCES users(id),
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (hiveshare_id, user_id),
    CONSTRAINT valid_role CHECK (role IN ('all', 'view'))
);

CREATE TABLE invitations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hiveshare_id UUID NOT NULL REFERENCES hiveshares(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    invited_by   UUID NOT NULL REFERENCES users(id),
    token        TEXT UNIQUE NOT NULL,
    role         TEXT NOT NULL DEFAULT 'all',
    status       TEXT NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
    CONSTRAINT valid_status CHECK (status IN ('pending', 'accepted', 'expired'))
);

CREATE TABLE memory_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hiveshare_id UUID NOT NULL REFERENCES hiveshares(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type  TEXT NOT NULL,
    source_ref   TEXT NOT NULL,
    source_url   TEXT,
    tool         TEXT NOT NULL DEFAULT 'manual',
    content      TEXT NOT NULL,
    summary      TEXT,
    embedding    vector(1536),
    tags         TEXT[] NOT NULL DEFAULT '{}',
    metadata     JSONB NOT NULL DEFAULT '{}',
    views        INTEGER NOT NULL DEFAULT 0,
    reuses       INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT valid_source_type CHECK (source_type IN ('jira', 'github_issue', 'github_pr', 'file', 'url', 'manual')),
    CONSTRAINT valid_tool CHECK (tool IN ('claude', 'cursor', 'manual'))
);

CREATE TABLE usage_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hiveshare_id UUID REFERENCES hiveshares(id) ON DELETE CASCADE,
    entry_id     UUID REFERENCES memory_entries(id) ON DELETE SET NULL,
    event_type   TEXT NOT NULL,
    metadata     JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
