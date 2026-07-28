# HiveShare — CLAUDE.md

Collaborative AI memory server for engineering teams. Teammates' AI agents store and search crunched context in shared "hiveshares" so nobody re-reads the same ticket twice.

## Repo layout

```
cmd/
  server/     → API server binary (main.go)
  mcp/        → MCP sidecar binary (used by Claude Code / Cursor)
  hiveshare/     → CLI binary (main.go + client.go + config.go)
internal/
  api/        → HTTP handlers (router.go, auth.go, memory.go=HiveHandler, …)
  mcp/        → MCP protocol + server + client
  models/     → shared Go types (Hive, HistoryEntry, Snapshot, …)
  store/      → Postgres (db.go, memory.go=HiveStore, history.go, …)
  realtime/   → SSE hub (Redis pub/sub)
  embed/      → async embedding worker (OpenAI / Ollama)
migrations/   → 001–006 applied in order
API.md        → HTTP reference (verified by scripts/test-api-examples.sh)
deploy/       → OpenShift manifests
```

## Build & run

```bash
make deps build          # build all three binaries into ./bin/
make dev                 # docker-up + migrate + run server (EMBED_PROVIDER= for no embeddings)
make migrate             # apply migrations/*.sql in order
make docker-up / docker-down
make smoke-test / smoke-test-full / integration-test
make release             # cross-compile linux+darwin amd64+arm64 → dist/
```

Binaries: `bin/hiveshare-server`, `bin/hiveshare-mcp`, `bin/hiveshare`

## Key env vars (server)

| Var | Notes |
|---|---|
| `DATABASE_URL` | Full Postgres DSN |
| `REDIS_URL` | Default `redis://localhost:6379` |
| `LISTEN_ADDR` | Default `:8080` |
| `BASE_URL` | Used in invite links |
| `EMBED_PROVIDER` | `openai` \| `ollama` \| empty (falls back to full-text) |
| `OPENAI_API_KEY` | Required when `EMBED_PROVIDER=openai` |
| `HISTORY_TTL_DAYS` | Optional purge age (`0` = forever) |
| `HISTORY_MAX_VERSIONS` | Optional per-hive version cap (`0` = unlimited) |

## Naming: hive vs memory

The entity was renamed from `memory_entry` → `hive` in migration 005. The Go type is `Hive`, the DB table is `hives`, API routes use `/hives`, SSE events are `hive_added` / `hive_updated` / `hive_deleted` / `hive_rolled_back` / `hive_undeleted`. Old names (`memory`, `MemoryEntry`) are gone — don't reintroduce them. Handler file is still `internal/api/memory.go` (HiveHandler).

## Auth

API keys are prefixed `hvs_`, SHA-256 hashed at rest. Cleartext returned only at registration. All API endpoints require `Authorization: Bearer hvs_…`.

## Writes / search / history flow

1. `POST /hiveshares/{id}/hives` inserts immediately (embedding = NULL); trigger writes `hives_history` insert row.
2. Async embed worker fills embedding — **no** history row (trigger excludes embedding column).
3. Search uses HNSW cosine similarity if embedding present, else PostgreSQL full-text.
4. `source_ref` is unique per hiveshare; duplicates get auto-suffixed (`PROJ-42-2`).
5. Rollback / undelete / copy / snapshots live under `/hives/…` and `/snapshots/…` (see `API.md`).

## MCP tools (what Claude sees)

`search_hives`, `add_hive`, `list_hiveshares`, `get_context`, `get_metrics`,
`list_hives`, `update_hive`, `delete_hive`, `batch_add`

Default hiveshare env: `HIVESHARE_DEFAULT_HIVESHARE` (legacy alias: `HIVESHARE_DEFAULT_HEADSPACE`).

## MCP install scope

The MCP sidecar (`hiveshare-mcp`) has three independent layers of config:

| Layer | Where | How set |
|---|---|---|
| Binary | `~/.local/bin/hiveshare-mcp` | `scripts/install-client.sh` |
| Personal credentials (API key, server URL) | `~/.config/hiveshare/config.json` | `hiveshare auth register` or install script |
| Per-project hiveshare | `.claude/settings.json` in repo root | `hiveshare use <id> --project` |

Run `scripts/install-client.sh` once per machine. Run `hiveshare use <id> --project` once per project repo and commit the resulting `.claude/settings.json` — teammates get the right hiveshare context automatically when they open the repo.

## Migrations

Apply with `make migrate`, `scripts/install-server.sh`, or `psql -f migrations/NNN_*.sql` in order through **006**. Files are mostly idempotent (`IF NOT EXISTS` / `OR REPLACE`).

## Go module

`github.com/KB-perByte/hiveshare`, Go 1.22+. Key deps: chi (router), pgx/v5 (postgres), pgvector-go, go-redis/v9, cobra (CLI).
