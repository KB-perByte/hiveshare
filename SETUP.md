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

## CLI Quick Start

```bash
# Build and optionally install
make cli
# or: make install  (copies to /usr/local/bin)

# Register
./bin/hshare auth register --email you@example.com --name "Alice"

# Create a hiveshare
./bin/hshare hiveshare create "Auth Refactor Sprint"
./bin/hshare hiveshare list

# Set it as active (saves to ~/.config/hiveshare/config.json)
./bin/hshare hiveshare use <hiveshare-id>

# Add memory from a tool session
echo "PROJ-123 is about refactoring the JWT middleware..." | \
  ./bin/hshare memory add --source-type jira --source-ref PROJ-123 --tool claude

# Search
./bin/hshare memory search "JWT authentication"

# Invite a colleague
./bin/hshare invite alice@example.com

# Metrics
./bin/hshare metrics
./bin/hshare metrics --me

# Stream live updates (open in another terminal)
./bin/hshare stream
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
        "HIVESHARE_DEFAULT_HEADSPACE": "your-hiveshare-uuid"
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
| `search_memory` | Semantic/full-text search across hiveshare memory |
| `add_memory` | Save processed context (auto-called after you crunch a ticket) |
| `list_hiveshares` | List all hiveshares you belong to |
| `get_context` | All memory for a specific source ref (e.g. `PROJ-123`) |
| `get_metrics` | Collaboration and reuse metrics |

### Example Claude Code workflow

Claude will automatically call `search_memory` when you mention a Jira ticket or GitHub issue. After processing, it will call `add_memory` to save the context so teammates benefit instantly.

You can also prompt it directly:
```
Search the hiveshare for anything about authentication middleware.
Save this context about PROJ-123 to the hiveshare.
Show me metrics for this hiveshare.
```

## Invite Flow

1. Team member A runs: `hshare invite bob@example.com`
2. An invite link is printed: `http://localhost:8080/api/v1/invitations/<token>/accept`
3. Bob opens the link (or POSTs to it with `{"name": "Bob"}`)
4. Bob gets a new account and is added to the hiveshare
5. Bob registers their API key with the CLI/MCP and starts sharing memory

## Metrics

`GET /api/v1/hiveshares/:id/metrics` returns:
- **Memory stats**: entry count, source type breakdown, tool breakdown
- **Collaboration**: total views, reuses, reuse rate, top contributors
- **Coverage**: how many Jira refs / GitHub refs have memory entries
- **Activity**: last 7-day adds, searches, active users

## Project Structure

```
hiveshare/
├── cmd/server/     API server
├── cmd/mcp/        MCP sidecar (connect to Claude/Cursor)
├── cmd/hshare/     CLI
├── internal/
│   ├── api/        HTTP handlers + router
│   ├── embed/      Embedding backends (OpenAI, Ollama, no-op)
│   ├── mcp/        MCP protocol + tool definitions
│   ├── models/     Domain types
│   ├── realtime/   Redis pub/sub + SSE hub
│   └── store/      PostgreSQL queries
└── migrations/     SQL schema
```
