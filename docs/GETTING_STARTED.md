# Getting Started with HiveShare

HiveShare is a shared memory server for engineering teams. When your AI agent (Claude, Cursor) reads a Jira ticket or GitHub PR, the processed context gets saved to a hiveshare. Every teammate's agent can reuse it instantly — no re-reading, no re-summarising.

> **Want to test interactions?** See [TUTORIAL.md](TUTORIAL.md) for a hands-on walkthrough — curl commands, MCP agent prompts, collaboration round-trips, rate limit tests, and a quick checklist to confirm everything is wired up.

---

## How the pieces fit together

```
Your machine                     Teammate's machine
──────────────                   ──────────────────
hiveshare CLI  ──┐               ┌── Cursor (MCP)
                 ├── REST API ───┤
Cursor (MCP)  ──┘               └── Claude Code (MCP)
                      │
              hiveshare-server
                 │         │
            PostgreSQL    Redis
           + pgvector    pub/sub
```

**Three independent layers of config** — understand these and everything else makes sense:

| Layer | What it is | Where it lives | Set once per |
|---|---|---|---|
| Binary | `hiveshare-mcp` executable | `~/.local/bin/` | Machine |
| Credentials | API key + server URL | `~/.config/hiveshare/config.json` | Person |
| Project context | Which hiveshare to use | `.claude/settings.json` in repo | Repo |

---

## Choose your path

- **[A — I'm the team owner](#path-a-team-owner)** — server is mine, I create hiveshares and invite people
- **[B — I'm a developer joining a team (CLI + MCP)](#path-b-developer-cli--mcp)** — I got an invite, I want CLI and MCP
- **[C — I only use Claude Code or Cursor](#path-c-mcp-only-no-cli)** — no terminal, just my AI assistant
- **[D — I'm setting up the server](#path-d-server-setup)** — infrastructure first

---

## Path A: Team Owner

You run the server (or someone did for you) and you're creating the shared spaces.

### 1. Install the client

```bash
git clone https://github.com/KB-perByte/hiveshare
cd hiveshare
./scripts/install-client.sh
```

The script builds and installs the CLI (`hiveshare`) and MCP sidecar (`hiveshare-mcp`), registers your account, and offers to wire Claude Code / Cursor automatically.

### 2. Create a hiveshare

```bash
hiveshare create "Sprint 42"
# → Created hiveshare: Sprint 42 (a3f1c9d2-...)

hiveshare use a3f1c9d2-...
# → Active hiveshare: Sprint 42 (a3f1c9d2-...)
```

### 3. Pin it to your project repo

Run this inside the repo your team works in:

```bash
cd /path/to/your-project
hiveshare use a3f1c9d2-... --project
# → Written to: /path/to/your-project/.claude/settings.json
```

Commit that file. Every teammate who installs `hiveshare-mcp` and pulls will automatically connect to the right hiveshare.

```bash
git add .claude/settings.json
git commit -m "chore: pin hiveshare context for Claude/Cursor"
```

### 4. Invite teammates

```bash
# CLI + MCP users
hiveshare invite alice@example.com --role all

# MCP-only users — share the token, they accept via their agent
hiveshare invite bob@example.com --role view
# → Invite link: https://your-server/api/v1/invitations/abc123.../accept
# → Share just the token "abc123..." with Bob over Slack or email
```

### 5. Start saving context

```bash
echo "PROJ-42: JWT middleware must validate tokens before forwarding to services..." | \
  hiveshare hive add --source-type jira --source-ref PROJ-42

hiveshare hive search "JWT validation"
hiveshare stream   # live tail of teammate activity
```

---

## Path B: Developer (CLI + MCP)

You received an invite. You want both the CLI for daily use and MCP so your agent works automatically.

### 1. Install

```bash
git clone https://github.com/KB-perByte/hiveshare
cd hiveshare
./scripts/install-client.sh
```

When prompted for an API key, choose **"Register a new account"** — the invite will add you to the hiveshare once your account exists.

### 2. Accept the invite

You should have received an invite token (e.g. `abc123def456...`) from the team owner via Slack or email.

```bash
curl -X POST https://your-server/api/v1/invitations/abc123def456.../accept \
  -H "Content-Type: application/json" \
  -d '{"name": "Your Name"}'
```

Or if you already have the CLI configured:

```bash
# The server will add your account to the hiveshare
# (the token contains the target hiveshare)
curl ... (same as above)
```

### 3. Set the active hiveshare

```bash
hiveshare list          # see all hiveshares you now belong to
hiveshare use <uuid>    # set the project hiveshare as active
```

If the repo already has `.claude/settings.json` committed (the team owner should have done this), you're done — your MCP will pick it up automatically on the next Claude/Cursor restart.

### 4. Verify

```bash
hiveshare auth status
hiveshare hive list
```

---

## Path C: MCP-Only (No CLI)

You only use Claude Code or Cursor. No terminal required after the one-time config.

### Step 1 — Install `hiveshare-mcp`

You need Go installed. Then:

```bash
git clone https://github.com/KB-perByte/hiveshare
cd hiveshare
go build -o ~/.local/bin/hiveshare-mcp ./cmd/mcp
```

Or ask whoever runs the server to share a pre-built binary.

### Step 2 — Add the server URL to your AI tool config

**Claude Code** — add to `~/.claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "hiveshare": {
      "command": "/home/yourname/.local/bin/hiveshare-mcp",
      "env": {
        "HIVESHARE_SERVER_URL": "https://your-server"
      }
    }
  }
}
```

Note: no `HIVESHARE_API_KEY` yet — that's fine. The server starts in limited mode with only `accept_invite` available.

**Cursor** — add to `~/.cursor/mcp.json` (same format).

### Step 3 — Accept the invite via your agent

Restart Claude/Cursor. Then tell your agent:

> "Accept my hiveshare invite. The token is `abc123def456...` and my name is Alice."

Your agent calls `accept_invite` and gets back:

```
New account created.
1) Set HIVESHARE_API_KEY=hvs_abc... in your MCP config.
2) Set HIVESHARE_DEFAULT_HIVESHARE=a3f1c9d2-... in your MCP config.
3) Restart Claude/Cursor. You will then have full access.
```

### Step 4 — Update your config and restart

```json
{
  "mcpServers": {
    "hiveshare": {
      "command": "/home/yourname/.local/bin/hiveshare-mcp",
      "env": {
        "HIVESHARE_SERVER_URL": "https://your-server",
        "HIVESHARE_API_KEY": "hvs_abc...",
        "HIVESHARE_DEFAULT_HIVESHARE": "a3f1c9d2-..."
      }
    }
  }
}
```

Restart Claude/Cursor. All nine tools are now available.

> **Note:** If the repo you're working in has `.claude/settings.json` committed with a `HIVESHARE_DEFAULT_HIVESHARE` value, that overrides the global default automatically — you don't need to set it in your global config.

### What the agent can do

Once configured your agent works automatically. It will:
- Search context before asking you questions (`search_hives`)
- Save summaries after processing tickets or PRs (`add_hive`)
- Update notes when new information arrives (`update_hive`)
- Retrieve all context for a specific ticket (`get_context`)

You never need to tell it to — the MCP tools run in the background.

---

## Path D: Server Setup

### Prerequisites

- Docker + Docker Compose
- Go 1.22+ (to build from source)
- A domain name or IP with port 8080 accessible to teammates

### Quick start

```bash
git clone https://github.com/KB-perByte/hiveshare
cd hiveshare

# Start Postgres + Redis, run migrations, start server
make dev
```

### Production setup

```bash
./scripts/install-server.sh
```

The installer will:
- Build the server binary
- Prompt for database and Redis config
- Run all migrations
- Verify the server responds healthy

**Key env vars:**

| Var | Notes |
|---|---|
| `DATABASE_URL` | Full Postgres DSN |
| `REDIS_URL` | Default `redis://localhost:6379` |
| `LISTEN_ADDR` | Default `:8080` |
| `BASE_URL` | Public URL — used in invite links |
| `EMBED_PROVIDER` | `openai` or `ollama` — enables semantic search |
| `OPENAI_API_KEY` | Required when `EMBED_PROVIDER=openai` |

Verify the server is running:

```bash
curl https://your-server/health
# → {"status":"ok","db":"ok","redis":"ok",...}
```

Share `https://your-server` with teammates — they need it for their MCP config.

---

## Per-project configuration

The binary and API key are global (one install covers all projects). Only the *which hiveshare* part is project-specific.

Inside any project repo:

```bash
hiveshare use <uuid> --project
```

This writes to `.claude/settings.json` at the git root:

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

**Commit this file.** Every teammate who has `hiveshare-mcp` installed connects to the correct hiveshare automatically when they open the repo — no manual configuration.

Without `--project`, `hiveshare use <uuid>` updates `~/.config/hiveshare/config.json` (your personal global default) instead.

---

## Day-to-day workflow

### CLI users

```bash
# Save context after processing a ticket
hiveshare hive add \
  --source-type jira \
  --source-ref PROJ-123 \
  --summary "Auth service refactor: JWT validation must happen before forwarding" \
  < analysis.txt

# Search before starting work
hiveshare hive search "payment service timeout"

# See what teammates have been adding
hiveshare stream

# Check a specific ticket's full context
hiveshare hive list --source-type jira
hiveshare hive get <entry-id>
hiveshare hive search "PROJ-123" --source-type jira

# Version history
hiveshare hive history <entry-id>
hiveshare hive rollback <entry-id> --version <history-id>
```

### MCP / agent workflow

Your agent calls these automatically. You can also instruct it explicitly:

> "Before you answer, search the hiveshare for context on PROJ-123"
> "Save what you just learned about the payment service to the hiveshare"
> "Update the hive for PROJ-42 with the new approach we decided on"

---

## Roles

| Role | Can do |
|---|---|
| `all` | Read, write, invite members, manage hiveshare |
| `view` | Read and search only — no writes, no invites |

Assign roles when inviting:

```bash
hiveshare invite alice@example.com --role all   # full access
hiveshare invite bob@example.com   --role view  # read-only
```

---

## Troubleshooting

### "no hiveshare set"

```bash
hiveshare list          # see your hiveshares
hiveshare use <uuid>    # pick one
```

### "invalid api key" / 401

Your key may have been issued when you registered. It's only shown once. If lost:

```bash
# Re-register with the same email is blocked (409 Conflict).
# Ask the team owner to check if your account exists and share a new key.
# (API key rotation endpoint coming — see plan.md Phase 1)
```

### MCP tools not appearing in Claude/Cursor

1. Check the binary path is correct and executable: `ls -la ~/.local/bin/hiveshare-mcp`
2. Restart Claude/Cursor — MCP tool lists are cached at startup
3. Check the MCP server log (Claude Code: Help → MCP Logs)
4. Verify your API key works: `curl https://your-server/api/v1/auth/whoami -H "Authorization: Bearer hvs_..."`

### `accept_invite` returned an error

- `api 404` — token is wrong or already used
- `api 410` — invite expired (7 days) or already accepted; ask for a new invite
- `api 409` — you're already a member of this hiveshare

### Semantic search not working (returning 0 results)

Search falls back to full-text automatically when no embedder is configured or the embedding is missing. The response `"type"` field tells you which path ran:

```json
{ "type": "hybrid" }   // vector + full-text blend
{ "type": "fulltext" } // full-text only
```

To enable vector search, set `EMBED_PROVIDER=openai` (and `OPENAI_API_KEY`) or `EMBED_PROVIDER=ollama` on the server. Existing hives get embeddings asynchronously — give it a few minutes after enabling.

### Server health check fails

```bash
curl https://your-server/health
# "db": "unavailable" → PostgreSQL connection issue — check DATABASE_URL
# "redis": "unavailable" → Redis not running — check REDIS_URL
```

---

## Quick reference

```bash
# Auth
hiveshare auth register --email EMAIL --name NAME
hiveshare auth status

# Hiveshares
hiveshare create NAME
hiveshare list
hiveshare use ID                  # global default
hiveshare use ID --project        # project-level (.claude/settings.json)

# Hives
hiveshare hive add --source-ref REF [--source-type TYPE] [< file]
hiveshare hive get ENTRY_ID [--json]
hiveshare hive search QUERY [--alpha 0.7] [--json]
hiveshare hive list [--source-type TYPE] [--json]
hiveshare hive history ENTRY_ID
hiveshare hive rollback ENTRY_ID --version HISTORY_ID
hiveshare hive undelete --version HISTORY_ID

# Snapshots
hiveshare snapshot create [--name NAME]
hiveshare snapshot list
hiveshare snapshot restore ID [--name NAME]

# Team
hiveshare invite EMAIL [--role all|view]
hiveshare members list
hiveshare stream

# Shell completion
hiveshare completion bash | zsh | fish | powershell
```

MCP tools (available in Claude Code / Cursor):
`search_hives` · `add_hive` · `list_hiveshares` · `get_context` · `get_metrics`
`list_hives` · `update_hive` · `delete_hive` · `batch_add` · `accept_invite`
