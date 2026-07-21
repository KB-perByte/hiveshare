# HiveShare

**Collaborative AI memory for engineering teams.**

When you and your teammates use Claude Code or Cursor on the same Jira ticket or GitHub issue, every session re-crunches the same context from scratch. HiveShare fixes that: anything one person's AI agent processes gets stored as searchable memory in a shared **hiveshare**, so everyone else's agent reuses it immediately — no re-reading, no re-summarising.

```
Alice crunches PROJ-42 with Claude  →  memory saved to hiveshare
Bob opens PROJ-42 with Cursor       →  MCP tool loads Alice's memory automatically
                                        Bob's agent starts with full context
```

---

## Features

- **Isolated hiveshares** — one per project, story, or sprint; fully separate memory spaces
- **Invite by email** — token-based, no SMTP required; teammates get their own API key on accept
- **Live sync** — `hshare stream` shows new entries from any teammate in real time (SSE)
- **Semantic search** — OpenAI or Ollama embeddings; falls back to PostgreSQL full-text if not configured
- **MCP integration** — Claude Code and Cursor can search and save memory automatically without you doing anything
- **Metrics** — reuse rate, top contributors, source coverage, 7-day activity

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
go install github.com/KB-perByte/hiveshare/cmd/hshare@latest
```

### 3 — Register and create a hiveshare

```bash
hshare auth register --email you@example.com --name "Alice"
# Saves your API key to ~/.config/hiveshare/config.json

hshare headspace create "PROJ-42 Sprint"
hshare headspace list          # note the ID
hshare headspace use <uuid>
```

### 4 — Invite a teammate

```bash
hshare invite bob@example.com
# Prints an invite link:
#   https://your-server/api/v1/invitations/<token>/accept
```

Bob opens the link (or POSTs to it), gets his API key, and runs step 2–3 with the same server URL.

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

Restart Claude Code. Claude now has five tools: `search_memory`, `add_memory`, `list_hiveshares`, `get_context`, `get_metrics`.

### 6 — Test it

**Alice's terminal:**
```bash
hshare stream          # live tail — keep this open
```

**Bob adds memory:**
```bash
echo "PROJ-42: the JWT middleware needs to validate tokens before forwarding..." | \
  hshare memory add --source-type jira --source-ref PROJ-42 --tool claude
```

Alice's terminal immediately shows:
```
[10:41:02] + memory added: jira/PROJ-42 by Bob
```

**Alice searches:**
```bash
hshare memory search "JWT validation"
```

---

## CLI reference

```
hshare auth register --email EMAIL --name NAME [--server URL]
hshare auth status

hshare headspace create NAME [--description TEXT]
hshare headspace list
hshare headspace use ID

hshare memory add --source-ref REF [--source-type TYPE] [--tool TOOL] [< file]
hshare memory search QUERY [--limit N] [--source-type TYPE]
hshare memory list [--source-type TYPE] [--limit N]

hshare invite EMAIL [--role member|viewer]
hshare members list

hshare stream
hshare metrics [--me]
```

Source types: `jira`, `github_issue`, `github_pr`, `file`, `url`, `manual`  
Tools: `claude`, `cursor`, `manual`

---

## MCP tools

| Tool | What it does |
|---|---|
| `search_memory` | Semantic or full-text search across the hiveshare |
| `add_memory` | Save crunched context (call after processing any ticket or PR) |
| `list_hiveshares` | List all spaces you belong to |
| `get_context` | All memory for a specific source ref (e.g. `PROJ-42`) |
| `get_metrics` | Collaboration stats for the hiveshare |

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

---

## Architecture

```
hshare CLI  ──┐
               ├──  REST + SSE  ──  hiveshare-server (Go)
MCP sidecar ──┘                          │
                                  ┌──────┴──────┐
                            PostgreSQL      Redis
                           + pgvector      pub/sub
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for a deep review, scale analysis, Mermaid diagrams, and a prioritised fix list. See [`docs/INFRA_SETUP.md`](docs/INFRA_SETUP.md) for ngrok, AWS EC2, and OpenShift deployment guides.

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
#   hshare            — CLI
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

Issues and PRs welcome. See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) section 7 for the prioritised list of known issues to fix.

---

## License

MIT — see [LICENSE](LICENSE).
