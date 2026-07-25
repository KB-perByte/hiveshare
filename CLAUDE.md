# HiveShare — CLAUDE.md

Collaborative AI memory server for engineering teams. Teammates' AI agents store and search crunched context in shared "hiveshares" so nobody re-reads the same ticket twice.

## Repo layout

```
cmd/
  server/     → API server binary (main.go)
  mcp/        → MCP sidecar binary (used by Claude Code / Cursor)
  hshare/     → CLI binary (main.go + client.go + config.go)
internal/
  api/        → HTTP handlers (router.go, auth.go, memory.go, headspaces.go, …)
  mcp/        → MCP protocol + server + client
  models/     → shared Go types (Hive, Hiveshare, User, roles)
  store/      → Postgres queries (db.go, memory.go, headspaces.go, users.go, …)
  realtime/   → SSE hub (Redis pub/sub)
  embed/      → async embedding worker (OpenAI / Ollama)
migrations/   → numbered SQL files applied in order
deploy/       → OpenShift manifests
```

## Build & run

```bash
make deps build          # build all three binaries into ./bin/
make dev                 # docker-up + migrate + run server (EMBED_PROVIDER= for no embeddings)
make migrate             # apply migrations/*.sql in order
make docker-up / docker-down
make release             # cross-compile linux+darwin amd64+arm64 → dist/
```

Binaries: `bin/hiveshare-server`, `bin/hiveshare-mcp`, `bin/hshare`

## Key env vars (server)

| Var | Notes |
|---|---|
| `DATABASE_URL` | Full Postgres DSN |
| `REDIS_URL` | Default `redis://localhost:6379` |
| `LISTEN_ADDR` | Default `:8080` |
| `BASE_URL` | Used in invite links |
| `EMBED_PROVIDER` | `openai` \| `ollama` \| empty (falls back to full-text) |
| `OPENAI_API_KEY` | Required when `EMBED_PROVIDER=openai` |

## Naming: hive vs memory

The entity was renamed from `memory_entry` → `hive` in migration 005. The Go type is `Hive`, the DB table is `hives`, API routes use `/hives`, SSE events are `hive_added` / `hive_updated` / `hive_deleted`. Old names (`memory`, `MemoryEntry`) are gone — don't reintroduce them.

## Auth

API keys are prefixed `hvs_`, SHA-256 hashed at rest. Cleartext returned only at registration. All API endpoints require `Authorization: Bearer hvs_…`.

## Writes / search flow

1. `POST /hiveshares/{id}/hives` inserts immediately (embedding = NULL).
2. Async embed worker picks it up and fills the embedding.
3. Search uses HNSW cosine similarity if embedding present, else PostgreSQL full-text.
4. `source_ref` is unique per hiveshare; duplicates get auto-suffixed (`PROJ-42-2`).

## MCP tools (what Claude sees)

`search_hives`, `add_hive`, `list_hiveshares`, `get_context`, `get_metrics`

## Migrations

Apply with `make migrate` or `psql -f migrations/NNN_*.sql` in order. Files are idempotent-safe only if you run them in sequence from 001 onward on a fresh DB; re-running on an existing DB may error — check each file.

## Go module

`github.com/KB-perByte/hiveshare`, Go 1.22+. Key deps: chi (router), pgx/v5 (postgres), pgvector-go, go-redis/v9, cobra (CLI).
