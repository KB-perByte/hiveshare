# HiveShare — Setup Guide

## Prerequisites

- Go 1.22+ (`sudo dnf install golang` on Fedora)
- Docker + Docker Compose
- (Optional) OpenAI API key for semantic search, or Ollama for local embeddings

## Quick Start

```bash
# 1. Start infrastructure
make docker-up

# 2. Install Go dependencies and run migrations
make deps migrate

# 3. Start the server
make server && ./bin/hiveshare-server
# or for development:
make dev
```

The server listens on `:8080` by default. Override with `LISTEN_ADDR=:9000`.

Health check (Postgres + Redis):

```bash
curl -s http://localhost:8080/health
# {"status":"ok","db":"ok","redis":"ok"}
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | Full postgres DSN (overrides individual vars) |
| `POSTGRES_HOST` | `localhost` | |
| `POSTGRES_PORT` | `5432` | |
| `POSTGRES_USER` | `hiveshare` | |
| `POSTGRES_PASSWORD` | `hiveshare` | |
| `POSTGRES_DB` | `hiveshare` | |
| `REDIS_URL` | `redis://localhost:6379` | |
| `LISTEN_ADDR` | `:8080` | Server listen address |
| `BASE_URL` | `http://localhost:8080` | Used to generate invite links |
| `EMBED_PROVIDER` | — | `openai` or `ollama` (no embeddings if unset) |
| `OPENAI_API_KEY` | — | Required if `EMBED_PROVIDER=openai` |
| `OPENAI_EMBED_MODEL` | `text-embedding-3-small` | |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Required if `EMBED_PROVIDER=ollama` |
| `OLLAMA_EMBED_MODEL` | `nomic-embed-text` | |
| `HISTORY_TTL_DAYS` | `0` | Purge history older than N days (`0` = forever) |
| `HISTORY_MAX_VERSIONS` | `0` | Max versions kept per hive (`0` = unlimited) |

## CLI Quick Start

```bash
# Build and optionally install
make cli
# or: make install  (copies to /usr/local/bin)

# Register (API key is shown once — server stores SHA-256 only)
./bin/hiveshare auth register --email you@example.com --name "Maverick"

# Create a hiveshare
./bin/hiveshare create "Auth Refactor Sprint"
./bin/hiveshare list

# Set it as active (saves to ~/.config/hiveshare/config.json)
./bin/hiveshare use <hiveshare-id>

# Add a hive from a tool session
echo "PROJ-123 is about refactoring the JWT middleware..." | \
  ./bin/hiveshare hive add --source-type jira --source-ref PROJ-123 --tool claude

# Search
./bin/hiveshare hive search "JWT authentication"

# History / rollback
./bin/hiveshare hive history <entry-id>
./bin/hiveshare hive rollback <entry-id> --version <history-id>

# Snapshots
./bin/hiveshare snapshot create --name "before-refactor"
./bin/hiveshare snapshot list
./bin/hiveshare snapshot restore <snapshot-id> --name "restored copy"

# Invite a colleague
./bin/hiveshare invite alice@example.com

# Metrics
./bin/hiveshare metrics
./bin/hiveshare metrics --me

# Stream live updates (open in another terminal)
./bin/hiveshare stream
```

## MCP Setup (Claude Code)

Build and install the MCP server:
```bash
make mcp
make install-mcp
```

Add to your Claude Code MCP config (`~/.claude/claude_desktop_config.json` or project `.mcp.json`):
```json
{
  "mcpServers": {
    "hiveshare": {
      "command": "hiveshare-mcp",
      "env": {
        "HIVESHARE_API_KEY": "hvs_your_key_here",
        "HIVESHARE_SERVER_URL": "http://localhost:8080",
        "HIVESHARE_DEFAULT_HIVESHARE": "your-hiveshare-uuid"
      }
    }
  }
}
```

Alternatively, configure via file at `~/.config/hiveshare/config.json`:
```json
{
  "server_url": "http://localhost:8080",
  "api_key": "hvs_your_key_here",
  "default_hiveshare": "your-hiveshare-uuid"
}
```

### Available MCP Tools

| Tool | What it does |
|---|---|
| `search_hives` | Semantic/full-text search across hiveshare hives |
| `add_hive` | Save processed context (auto-called after you crunch a ticket) |
| `list_hiveshares` | List all hiveshares you belong to |
| `get_context` | All hives for a specific source ref (e.g. `PROJ-123`) |
| `get_metrics` | Collaboration and reuse metrics |

### Example Claude Code workflow

Claude will automatically call `search_hives` when you mention a Jira ticket or GitHub issue. After processing, it will call `add_hive` to save the context so teammates benefit instantly.

You can also prompt it directly:
```
Search the hiveshare for anything about authentication middleware.
Save this hive about PROJ-123 to the hiveshare.
Show me metrics for this hiveshare.
```

## Invite Flow

1. Team member A runs: `hiveshare invite bob@example.com`
2. An invite link is printed: `http://localhost:8080/api/v1/invitations/<token>/accept`
3. Rooster opens the link (or POSTs to it with `{"name": "Rooster"}`)
4. Rooster gets a new account (API key returned once) and is added to the hiveshare
5. Rooster saves the key in CLI/MCP config and starts sharing

Invites are rejected in SQL when `status != 'pending'` or `expires_at` has passed (default 7 days).

## Metrics

`GET /api/v1/hiveshares/:id/metrics` returns:
- **Hive stats**: entry count, source type breakdown, tool breakdown
- **Collaboration**: total views, reuses, reuse rate, top contributors
- **Coverage**: how many Jira refs / GitHub refs have hive entries
- **Activity**: last 7-day adds, searches, active users

## Project Structure

```
hiveshare/
├── cmd/server/     API server (embed workers, view flusher, history purge, event TTL)
├── cmd/mcp/        MCP sidecar (connect to Claude/Cursor)
├── cmd/hiveshare/     CLI
├── internal/
│   ├── api/        HTTP handlers + router (rate limit, timeout, /health)
│   ├── embed/      Embedding backends + async worker pool
│   ├── mcp/        MCP protocol + tool definitions
│   ├── models/     Domain types (Hive, HistoryEntry, Snapshot, …)
│   ├── realtime/   Redis pub/sub + shared SSE hub
│   └── store/      PostgreSQL + Redis view counters + history store
├── migrations/     001–006 (HNSW, hive rename, history/snapshots)
└── API.md          Full HTTP reference with curl examples
```

Hive **list** returns metadata without full `content` (keeps MCP/CLI payloads small). Use get or search for body text. Embeddings are written asynchronously after add/update; until then, search falls back to full-text for those rows. `source_ref` is unique per hiveshare — the API auto-suffixes duplicates (e.g. `PROJ-123-2`). Write-access members can delete any hive; deletion also cleans up the Redis view counter and fires a `hive_deleted` SSE event. Content mutations are recorded in `hives_history` (embedding-only updates are excluded); snapshots restore into a new hiveshare.
