# HiveShare

**Collaborative AI memory for engineering teams.**

When you and your teammates use Claude Code or Cursor on the same Jira ticket or GitHub issue, every session re-crunches the same context from scratch. HiveShare fixes that: anything one person's AI agent processes gets stored as searchable memory in a shared **hiveshare**, so everyone else's agent reuses it immediately — no re-reading, no re-summarising.

```mermaid
sequenceDiagram
    actor Maverick as 🧑‍💻 Maverick · Claude Code
    participant MCP as hiveshare MCP
    participant API as hiveshare server
    participant DB as PostgreSQL + pgvector
    participant Redis as Redis
    actor Rooster as 👨‍💻 Rooster · Cursor

    Maverick->>MCP: add_hive(PROJ-42, crunched context)
    MCP->>API: POST /hiveshares/{id}/hives
    API->>DB: INSERT hive · embedding = NULL
    API-->>Redis: PUBLISH hive_added
    API-->>Maverick: 201 · hive saved
    DB-->>DB: async embed worker · UPDATE embedding

    Note over DB: indexed · searchable · team-visible

    Rooster->>MCP: search_hives("PROJ-42 auth flow")
    MCP->>API: POST /hives/search
    API->>DB: HNSW cosine similarity scan
    DB-->>API: Maverick's hive · score 0.97
    API-->>Rooster: full context · zero re-reading
```

---

## Features

- **Isolated hiveshares** — one per project, story, or sprint; fully separate hive stores
- **Invite by email** — token-based, no SMTP required; teammates get their own API key on accept
- **Live sync** — `hiveshare stream` shows new entries from any teammate in real time (SSE; one Redis sub per hiveshare)
- **Semantic search** — OpenAI or Ollama embeddings (async on write, HNSW index); falls back to PostgreSQL full-text if not configured
- **MCP integration** — Claude Code and Cursor can search and save memory automatically without you doing anything
- **Metrics** — reuse rate, top contributors, source coverage, 7-day activity
- **Unique source refs** — one hive per `source_ref` per hiveshare; duplicates are auto-suffixed (`PROJ-42-2`) so saves never conflict
- **Hive deletion** — write-access members can delete any hive; Redis view counters are cleaned up and a `hive_deleted` event is broadcast over SSE
- **Version history** — every content change is recorded; roll back a hive or undelete after removal
- **Snapshots** — capture a hiveshare point-in-time and restore into a **new** hiveshare; copy hives between spaces
- **Hardened API** — hashed API keys, rate limits, body size cap, request timeouts, `GET /health`

---

## Quick start

### 1 — Start the server (Docker)

```bash
curl -O https://raw.githubusercontent.com/KB-perByte/hiveshare/main/docker-compose.full.yml
curl -O https://raw.githubusercontent.com/KB-perByte/hiveshare/main/.env.example
cp .env.example .env
# Edit .env: set a strong POSTGRES_PASSWORD and your BASE_URL
nano .env

docker compose -f docker-compose.full.yml up -d
```

The server is now running at `http://localhost:8080` (or your `BASE_URL`).

### 2 — Install the CLI

```bash
curl -sSL https://raw.githubusercontent.com/KB-perByte/hiveshare/main/install.sh | bash
```

Or with Go installed:

```bash
go install github.com/KB-perByte/hiveshare/cmd/hiveshare@latest
```

### 3 — Register and create a hiveshare

```bash
hiveshare auth register --email you@example.com --name "Maverick"
# Prints your API key once and saves it to ~/.config/hiveshare/config.json
# (server stores only SHA-256 of the key — save it now)

hiveshare create "PROJ-42 Sprint"
hiveshare list          # note the ID
hiveshare use <uuid>
```

### 4 — Invite a teammate

```bash
hiveshare invite bob@example.com
# Prints an invite link:
#   https://your-server/api/v1/invitations/<token>/accept
```

Rooster opens the link (or POSTs to it), gets his API key, and runs step 2–3 with the same server URL.

### 5 — Connect Claude Code

Install the MCP binary:
```bash
go install github.com/KB-perByte/hiveshare/cmd/hiveshare-mcp@latest
# or: the install.sh above already placed it in ~/.local/bin/
```

Add to `~/.claude/claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "hiveshare": {
      "command": "hiveshare-mcp",
      "env": {
        "HIVESHARE_API_KEY": "hvs_your_key",
        "HIVESHARE_SERVER_URL": "https://your-server",
        "HIVESHARE_DEFAULT_HIVESHARE": "your-hiveshare-uuid"
      }
    }
  }
}
```

Restart Claude Code. Claude now has nine tools: `search_hives`, `add_hive`, `list_hiveshares`, `get_context`, `get_metrics`, `list_hives`, `update_hive`, `delete_hive`, `batch_add`.

### 6 — Pin a hiveshare to each project

The binary and credentials installed above are global — one install covers all your projects. What changes per-project is *which hiveshare* Claude connects to.

Inside each project repo:
```bash
hiveshare use <uuid> --project
```

This writes `HIVESHARE_DEFAULT_HIVESHARE` into `.claude/settings.json` at the repo root:

```json
{
  "mcpServers": {
    "hiveshare": {
      "env": {
        "HIVESHARE_DEFAULT_HIVESHARE": "a3f1c9d2-..."
      }
    }
  }
}
```

Commit that file. Every teammate who has `hiveshare-mcp` installed will automatically connect to the right hiveshare when they open this repo — no manual configuration.

Without `--project`, `hiveshare use <uuid>` updates the global default in `~/.config/hiveshare/config.json` instead.

### 7 — Test it

**Maverick's terminal:**
```bash
hiveshare stream          # live tail — keep this open
```

**Rooster saves a hive:**
```bash
echo "PROJ-42: the JWT middleware needs to validate tokens before forwarding..." | \
  hiveshare hive add --source-type jira --source-ref PROJ-42 --tool claude
```

Maverick's terminal immediately shows:
```
[10:41:02] + hive added: jira/PROJ-42 by Rooster
```

**Maverick searches:**
```bash
hiveshare hive search "JWT validation"
```

---

## CLI reference

```
hiveshare auth register --email EMAIL --name NAME [--server URL]
hiveshare auth status

hiveshare create NAME [--description TEXT]
hiveshare list [--json]
hiveshare use ID                              # set global default
hiveshare use ID --project                    # write to .claude/settings.json (commit this)

hiveshare hive add --source-ref REF [--source-type TYPE] [--tool TOOL] [< file]
hiveshare hive get ENTRY_ID [--hiveshare ID] [--json]
hiveshare hive search QUERY [--limit N] [--source-type TYPE] [--alpha 0.7] [--json]
hiveshare hive list [--source-type TYPE] [--limit N] [--json]
hiveshare hive history ENTRY_ID [--limit N]
hiveshare hive rollback ENTRY_ID --version HISTORY_ID
hiveshare hive undelete --version HISTORY_ID
hiveshare hive copy --to TARGET_HS --entries ID1,ID2

hiveshare snapshot create [--name NAME]
hiveshare snapshot list
hiveshare snapshot show ID
hiveshare snapshot restore ID [--name NAME]
hiveshare snapshot delete ID

hiveshare invite EMAIL [--role all|view]
hiveshare members list

hiveshare stream
hiveshare metrics [--me]

hiveshare completion bash|zsh|fish|powershell
```

Global flag: `--json` outputs raw JSON instead of formatted tables (supported by `hive get`, `hive list`, `hive search`).

Source types: `jira`, `github_issue`, `github_pr`, `file`, `url`, `manual`  
Tools: `claude`, `cursor`, `manual`  
Roles: `all` (invite + read + write) · `view` (read-only)

---

## MCP tools

| Tool | What it does |
|---|---|
| `search_hives` | Hybrid semantic + full-text search; `alpha` controls the blend |
| `add_hive` | Save crunched context (call after processing any ticket or PR) |
| `list_hiveshares` | List all spaces you belong to |
| `get_context` | All hives for a specific source ref (e.g. `PROJ-42`) |
| `get_metrics` | Collaboration stats for the hiveshare |
| `list_hives` | Browse hives with optional source_type filter and pagination |
| `update_hive` | Refine or correct a hive you previously added |
| `delete_hive` | Remove a stale or incorrect entry |
| `batch_add` | Add multiple hives in one call instead of calling `add_hive` repeatedly |

---

## Server configuration

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | Full Postgres DSN (overrides individual vars) |
| `POSTGRES_*` | see `.env.example` | Individual DB connection settings |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `BASE_URL` | `http://localhost:8080` | Public URL (used in invite links) |
| `EMBED_PROVIDER` | — | `openai` or `ollama`; empty = full-text search only |
| `OPENAI_API_KEY` | — | Required when `EMBED_PROVIDER=openai` |
| `OPENAI_EMBED_MODEL` | `text-embedding-3-small` | |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Required when `EMBED_PROVIDER=ollama` |
| `OLLAMA_EMBED_MODEL` | `nomic-embed-text` | |
| `HISTORY_TTL_DAYS` | `0` | Purge history older than N days (`0` = forever) |
| `HISTORY_MAX_VERSIONS` | `0` | Max versions kept per hive (`0` = unlimited) |

---

## Architecture

```
hiveshare CLI  ──┐
               ├──  REST + SSE  ──  hiveshare-server (Go)
MCP sidecar ──┘                    │  embed workers, health
                            ┌──────┴──────┐
                      PostgreSQL      Redis
                     + pgvector      pub/sub + view counters
                        HNSW
```

- Auth: Bearer `hvs_…` key; **SHA-256 at rest**, cleartext returned only at registration
- Writes: hive inserts immediately; embeddings filled async (search uses full-text until ready)
- History: trigger on content/summary/tags/metadata (not embedding fills); rollback / undelete / copy
- Snapshots: full hiveshare capture; restore creates a new hiveshare (capped at 10k entries)
- `source_ref` is unique per hiveshare — the API auto-suffixes duplicates (`PROJ-42-2`)
- Delete: removes the DB row, Redis view counter, logs a usage event, and publishes `hive_deleted` over SSE
- List endpoints omit full `content` (use Get/Search for body text)
- Ops: `GET /health` checks Postgres + Redis

See [`docs/GETTING_STARTED.md`](docs/GETTING_STARTED.md) for the full onboarding guide covering all user types (team owner, CLI developer, MCP-only, server setup). See [`docs/TUTORIAL.md`](docs/TUTORIAL.md) for a hands-on walkthrough of every flow with curl, CLI, and MCP agent prompts. See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for diagrams, scale analysis, and the V2/V3 roadmap. See [`API.md`](API.md) for the full HTTP reference. See [`docs/INFRA_SETUP.md`](docs/INFRA_SETUP.md) for ngrok, AWS EC2, and OpenShift deployment guides.

---

## Building from source

Requires Go 1.22+.

```bash
git clone https://github.com/KB-perByte/hiveshare
cd hiveshare
make deps build

# Binaries land in ./bin/
#   hiveshare-server  — API server
#   hiveshare-mcp     — MCP sidecar for Claude Code / Cursor
#   hiveshare            — CLI
```

### Development

```bash
# Start postgres + redis, run migrations, start server
make dev

# Or individually:
make docker-up
make migrate
./bin/hiveshare-server
```

---

## Infrastructure

Three deployment guides in [`docs/INFRA_SETUP.md`](docs/INFRA_SETUP.md):

| | ngrok | AWS EC2 | OpenShift |
|---|---|---|---|
| Setup | 5 min | 30 min | 45 min |
| Cost | Free | ~$8–10/mo | cluster cost |
| Best for | Quick two-person test | Ongoing team use | Enterprise / existing OCP |

OpenShift manifests are in [`deploy/openshift/`](deploy/openshift/).

---

## Contributing

Issues and PRs welcome. P0–P2 hardening is done; see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) section 7 for remaining V2/V3 scale work.

---

## License

MIT — see [LICENSE](LICENSE).
