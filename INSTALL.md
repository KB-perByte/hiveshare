# Installation

## Prerequisites

| Dependency | Required for | Install |
|---|---|---|
| [Go 1.22+](https://go.dev/doc/install) | Building binaries | `go.dev/doc/install` |
| PostgreSQL 16+ with pgvector | Server | see below |
| Redis 7+ | Server | `apt install redis` / `brew install redis` |
| `psql` client | Running migrations | `apt install postgresql-client` |
| `curl` | Installer health checks | usually pre-installed |

### PostgreSQL with pgvector

The server requires the `vector` extension. Use the official pgvector image for Docker:

```bash
docker run -d \
  --name hiveshare-pg \
  -e POSTGRES_USER=hiveshare \
  -e POSTGRES_PASSWORD=hiveshare \
  -e POSTGRES_DB=hiveshare \
  -p 5432:5432 \
  pgvector/pgvector:pg16
```

Or install pgvector into an existing PostgreSQL instance: https://github.com/pgvector/pgvector#installation

---

## Server installation

Run from the repository root:

```bash
chmod +x scripts/install-server.sh
./scripts/install-server.sh
```

The script will:

1. Check Go and `psql` are installed
2. Prompt for configuration (database, Redis, listen address, embedding provider)
3. Build the binary to `/usr/local/bin/hiveshare-server` (or `~/.local/bin/` if not root)
4. Write the config to `/etc/hiveshare/server.env` (or `~/.config/hiveshare/server.env`)
5. Run all database migrations
6. Start the server briefly and hit `/health` to confirm it works

### Configuration reference

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | Full postgres DSN. If blank, set individual `POSTGRES_*` vars |
| `POSTGRES_HOST` | `localhost` | — |
| `POSTGRES_PORT` | `5432` | — |
| `POSTGRES_USER` | `hiveshare` | — |
| `POSTGRES_PASSWORD` | `hiveshare` | — |
| `POSTGRES_DB` | `hiveshare` | — |
| `REDIS_URL` | `redis://localhost:6379` | — |
| `LISTEN_ADDR` | `:8080` | Host and port to bind |
| `EMBED_PROVIDER` | _(none)_ | `openai` or `ollama`. Leave blank to use full-text search only |
| `OPENAI_API_KEY` | — | Required when `EMBED_PROVIDER=openai` |
| `OPENAI_EMBED_MODEL` | `text-embedding-3-small` | — |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Required when `EMBED_PROVIDER=ollama` |
| `OLLAMA_EMBED_MODEL` | `nomic-embed-text` | — |
| `HISTORY_TTL_DAYS` | `0` | Purge history rows older than N days (`0` = keep forever) |
| `HISTORY_MAX_VERSIONS` | `0` | Keep at most N versions per hive (`0` = unlimited) |

Migrations `001`–`006` are applied by the installer (`005` renames `memory_entries` → `hives`; `006` adds `hives_history` + snapshots).

### Running the server

```bash
# Load the config written by the installer and start:
source /etc/hiveshare/server.env && hiveshare-server

# Or export individually:
export DATABASE_URL="postgresql://hiveshare:pass@localhost:5432/hiveshare"
export REDIS_URL="redis://localhost:6379"
hiveshare-server
```

### systemd service (optional)

Create `/etc/systemd/system/hiveshare.service`:

```ini
[Unit]
Description=HiveShare Server
After=network.target postgresql.service redis.service

[Service]
EnvironmentFile=/etc/hiveshare/server.env
ExecStart=/usr/local/bin/hiveshare-server
Restart=on-failure
RestartSec=5
User=hiveshare

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now hiveshare
systemctl status hiveshare
```

### Verifying the server

```bash
curl http://localhost:8080/health
# expected: {"status":"ok","db":"ok","redis":"ok",...}
```

---

## Client installation

Run from the repository root on the machine where you want to use the CLI:

```bash
chmod +x scripts/install-client.sh
./scripts/install-client.sh
```

The script will:

1. Build the `hiveshare` binary to `~/.local/bin/hiveshare`
2. Prompt for the server URL
3. Register a new account **or** accept an existing API key
4. Write `~/.config/hiveshare/config.json`
5. Call `/api/v1/auth/whoami` to confirm the credentials work

### PATH note

If `~/.local/bin` is not in your PATH, add this to `~/.bashrc` or `~/.zshrc`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Config file

`~/.config/hiveshare/config.json`:

```json
{
  "server_url": "https://your-server",
  "api_key": "hvs_...",
  "default_hiveshare": "",
  "default_hiveshare_name": ""
}
```

Fields can also be set via environment variables:

| Variable | Description |
|---|---|
| `HIVESHARE_SERVER_URL` | Overrides `server_url` |
| `HIVESHARE_API_KEY` | Overrides `api_key` |
| `HIVESHARE_CONFIG_DIR` | Use a different config directory |

### Verifying the client

```bash
hiveshare --help
hiveshare auth status
hiveshare list
```

---

## Uninstalling

### Server

```bash
chmod +x scripts/uninstall-server.sh
sudo ./scripts/uninstall-server.sh   # or without sudo if installed as non-root
```

The script will:
1. Stop and disable the systemd service (if present)
2. Remove the binary and version file
3. Ask whether to remove the config file (contains DB/Redis credentials)
4. Ask whether to remove the systemd unit file

The PostgreSQL database and Redis data are **not** touched. To drop the database too, pass `--nuke`:

```bash
sudo ./scripts/uninstall-server.sh --nuke
```

`--nuke` terminates active DB connections, then drops the database entirely. You must type the database name at the confirmation prompt — there is no undo.

### Client

```bash
chmod +x scripts/uninstall-client.sh
./scripts/uninstall-client.sh
```

The script will:
1. Remove the `hiveshare` binary and version file
2. Ask whether to remove the config file (`~/.config/hiveshare/config.json`, contains your API key)
3. Ask whether to remove the `~/.local/bin` PATH entry from your shell profile

---

## Upgrading

Pull the latest code and re-run the installer — it will rebuild and overwrite the binary. The config and migrations are not touched (migrations are idempotent).

```bash
git pull
./scripts/install-server.sh   # server
./scripts/install-client.sh   # client
```

---

## Troubleshooting

**Server won't start — DB connection refused**
- Confirm postgres is running: `pg_isready -h localhost -U hiveshare`
- Check the `DATABASE_URL` in the env file

**Server won't start — Redis connection refused**
- Confirm Redis is running: `redis-cli ping` (should return `PONG`)

**Migration fails with "extension vector does not exist"**
- You are not using the `pgvector/pgvector:pg16` image or pgvector is not installed. See [pgvector installation](https://github.com/pgvector/pgvector#installation).

**`hiveshare: command not found`**
- `~/.local/bin` is not in PATH. See the PATH note above.

**API key rejected (401)**
- Confirm the key in `~/.config/hiveshare/config.json` matches what the server issued.
- Re-register: `hiveshare auth register`
