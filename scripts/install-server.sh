#!/usr/bin/env bash
set -euo pipefail

# ── helpers ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; CYAN='\033[0;36m'; NC='\033[0m'

info()  { echo -e "${BOLD}[hiveshare]${NC} $*"; }
ok()    { echo -e "${GREEN}  ✓${NC} $*"; }
warn()  { echo -e "${YELLOW}  !${NC} $*"; }
die()   { echo -e "${RED}  ✗ $*${NC}" >&2; exit 1; }
step()  { echo -e "\n${CYAN}── $* ${NC}"; }

ask() {
    local prompt="$1" default="${2:-}" var
    if [[ -n "$default" ]]; then
        read -rp "  $prompt [$default]: " var
        echo "${var:-$default}"
    else
        read -rp "  $prompt: " var
        echo "$var"
    fi
}

ask_secret() {
    local prompt="$1" var
    read -rsp "  $prompt: " var; echo >&2
    echo "$var"
}

ask_yn() {
    local prompt="$1" default="${2:-n}" var
    read -rp "  $prompt (y/N): " var
    [[ "${var:-$default}" =~ ^[Yy]$ ]]
}

# ── locate repo root ──────────────────────────────────────────────────────────

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ -f "$REPO_ROOT/go.mod" ]] || die "Must be run from the hiveshare repository."
cd "$REPO_ROOT"

NEW_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
NEW_BUILDTIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w \
  -X github.com/KB-perByte/hiveshare/internal/version.Commit=${NEW_COMMIT} \
  -X github.com/KB-perByte/hiveshare/internal/version.BuildTime=${NEW_BUILDTIME}"

echo
echo -e "${BOLD}╔══════════════════════════════════╗${NC}"
echo -e "${BOLD}║   HiveShare Server Installer     ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════╝${NC}"
echo

# ── install paths ─────────────────────────────────────────────────────────────

if [[ "$EUID" -eq 0 ]]; then
    INSTALL_DIR="/usr/local/bin"
    CONFIG_DIR="/etc/hiveshare"
else
    INSTALL_DIR="${HOME}/.local/bin"
    CONFIG_DIR="${HOME}/.config/hiveshare"
    warn "Not running as root — installing to $INSTALL_DIR"
fi

BINARY="$INSTALL_DIR/hiveshare-server"
VERSION_FILE="$INSTALL_DIR/hiveshare-server.version"
ENV_FILE="$CONFIG_DIR/server.env"
BACKUP_BINARY="$BINARY.bak"
BACKUP_ENV="$ENV_FILE.bak"
ROLLBACK_NEEDED=0

# Cleanup/rollback on unexpected exit
cleanup() {
    local exit_code=$?
    if [[ "$exit_code" -ne 0 && "$ROLLBACK_NEEDED" -eq 1 ]]; then
        warn "Installation failed — rolling back..."
        [[ -f "$BACKUP_BINARY" ]] && mv -f "$BACKUP_BINARY" "$BINARY" && warn "Restored previous binary."
        [[ -f "$BACKUP_ENV" ]]    && mv -f "$BACKUP_ENV"    "$ENV_FILE" && warn "Restored previous config."
    fi
    rm -f "$BACKUP_BINARY" "$BACKUP_ENV" 2>/dev/null || true
}
trap cleanup EXIT

# ── prerequisites ─────────────────────────────────────────────────────────────

step "Checking prerequisites"

command -v go   >/dev/null 2>&1 || die "Go is not installed. See https://go.dev/doc/install"
ok "Go $(go version | awk '{print $3}')"

command -v psql >/dev/null 2>&1 || die "psql is not installed (apt install postgresql-client)."
ok "psql $(psql --version | awk '{print $3}')"

mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"

# ── detect existing installation ──────────────────────────────────────────────

UPGRADING=0
if [[ -f "$BINARY" ]]; then
    UPGRADING=1
    CURRENT_COMMIT="unknown"
    CURRENT_BUILD="unknown"
    if [[ -f "$VERSION_FILE" ]]; then
        CURRENT_COMMIT="$(grep '^commit='    "$VERSION_FILE" | cut -d= -f2)"
        CURRENT_BUILD="$(grep  '^build_time=' "$VERSION_FILE" | cut -d= -f2)"
    fi
    echo
    echo -e "  Installed : ${YELLOW}${CURRENT_COMMIT}${NC} (${CURRENT_BUILD})"
    echo -e "  New       : ${GREEN}${NEW_COMMIT}${NC} (${NEW_BUILDTIME})"
    echo
    if [[ "$CURRENT_COMMIT" == "$NEW_COMMIT" ]]; then
        ask_yn "Same version already installed. Re-install anyway?" "n" || { echo "Aborted."; exit 0; }
    else
        ask_yn "Upgrade from ${CURRENT_COMMIT} to ${NEW_COMMIT}?" "y" || { echo "Aborted."; exit 0; }
    fi
fi

# ── detect running server ─────────────────────────────────────────────────────

SYSTEMD_UNIT=""
WAS_RUNNING=0

if command -v systemctl >/dev/null 2>&1; then
    for unit in hiveshare hiveshare-server; do
        if systemctl is-active --quiet "$unit" 2>/dev/null; then
            SYSTEMD_UNIT="$unit"
            WAS_RUNNING=1
            break
        fi
    done
fi

# Fall back to process check if no systemd unit found
if [[ "$WAS_RUNNING" -eq 0 ]] && pgrep -x hiveshare-server >/dev/null 2>&1; then
    WAS_RUNNING=1
fi

if [[ "$UPGRADING" -eq 1 && "$WAS_RUNNING" -eq 1 ]]; then
    step "Stopping running server"
    if [[ -n "$SYSTEMD_UNIT" ]]; then
        systemctl stop "$SYSTEMD_UNIT" && ok "Stopped systemd unit: $SYSTEMD_UNIT"
    else
        pkill -x hiveshare-server && ok "Stopped hiveshare-server process"
    fi
    sleep 1
fi

# ── configuration ─────────────────────────────────────────────────────────────

step "Configuration"

KEEP_CONFIG=0
if [[ -f "$ENV_FILE" && "$UPGRADING" -eq 1 ]]; then
    warn "Existing config found at $ENV_FILE"
    if ask_yn "Keep existing config? (No = reconfigure)" "y"; then
        KEEP_CONFIG=1
        ok "Keeping existing config."
        # shellcheck disable=SC1090
        source "$ENV_FILE"
    fi
fi

if [[ "$KEEP_CONFIG" -eq 0 ]]; then
    echo
    echo "  Database — leave DATABASE_URL blank to enter individual fields."
    echo
    DB_URL=$(ask "DATABASE_URL" "")

    if [[ -z "$DB_URL" ]]; then
        PG_HOST=$(ask "Postgres host"     "localhost")
        PG_PORT=$(ask "Postgres port"     "5432")
        PG_USER=$(ask "Postgres user"     "hiveshare")
        PG_PASS=$(ask_secret "Postgres password")
        PG_DB=$(ask   "Postgres database" "hiveshare")
        DB_URL="postgresql://${PG_USER}:${PG_PASS}@${PG_HOST}:${PG_PORT}/${PG_DB}"
    fi

    echo
    REDIS_URL=$(ask "Redis URL"     "redis://localhost:6379")
    LISTEN_ADDR=$(ask "Listen address" ":8080")

    echo
    echo "  Embedding provider — enables semantic search (optional)."
    echo "  Options: none, openai, ollama"
    EMBED_PROVIDER=$(ask "Provider" "none")
    EMBED_PROVIDER="${EMBED_PROVIDER,,}"

    OPENAI_API_KEY=""; OPENAI_EMBED_MODEL=""
    OLLAMA_BASE_URL=""; OLLAMA_EMBED_MODEL=""

    case "$EMBED_PROVIDER" in
        openai)
            OPENAI_API_KEY=$(ask_secret "OpenAI API key")
            OPENAI_EMBED_MODEL=$(ask "Embedding model" "text-embedding-3-small")
            ;;
        ollama)
            OLLAMA_BASE_URL=$(ask "Ollama base URL"  "http://localhost:11434")
            OLLAMA_EMBED_MODEL=$(ask "Embedding model" "nomic-embed-text")
            ;;
        none|"")
            EMBED_PROVIDER=""
            warn "No embedding — search uses full-text fallback."
            ;;
        *)
            die "Unknown provider '$EMBED_PROVIDER'. Choose none, openai, or ollama."
            ;;
    esac
fi

# ── validate connectivity before touching anything ────────────────────────────

step "Validating connectivity"

PSQL_ERR="$(psql "$DB_URL" -c '\q' 2>&1)" || {
    echo "  Connection string: $DB_URL"
    echo "  psql error: $PSQL_ERR"
    die "Cannot connect to PostgreSQL. Fix DATABASE_URL and re-run."
}
ok "PostgreSQL reachable"

REDIS_HOST="${REDIS_URL#redis://}"
REDIS_HOST="${REDIS_HOST%%/*}"
REDIS_ADDR="${REDIS_HOST%:*}"
REDIS_PORT="${REDIS_HOST##*:}"
REDIS_PORT="${REDIS_PORT:-6379}"

if command -v redis-cli >/dev/null 2>&1; then
    redis-cli -h "$REDIS_ADDR" -p "$REDIS_PORT" ping >/dev/null 2>&1 \
        || { warn "Redis not reachable at $REDIS_URL — server requires Redis for SSE and view counts."; \
             ask_yn "Continue anyway?" "n" || die "Aborted. Start Redis and re-run."; }
    ok "Redis reachable"
else
    warn "redis-cli not found — skipping Redis check."
fi

# ── backup existing binary + config ───────────────────────────────────────────

if [[ "$UPGRADING" -eq 1 ]]; then
    step "Backing up existing installation"
    cp -f "$BINARY"   "$BACKUP_BINARY" && ok "Binary backed up"
    [[ -f "$ENV_FILE" ]] && cp -f "$ENV_FILE" "$BACKUP_ENV" && ok "Config backed up"
    ROLLBACK_NEEDED=1
fi

# ── build ─────────────────────────────────────────────────────────────────────

step "Building"

BUILD_TMP="$(mktemp)"
trap 'rm -f "$BUILD_TMP"; cleanup' EXIT

go build -ldflags="$LDFLAGS" -o "$BUILD_TMP" ./cmd/server \
    || die "Build failed."

# Atomic replace — avoids "text file busy" if the binary is somehow still open
mv -f "$BUILD_TMP" "$BINARY"
chmod +x "$BINARY"
ok "Binary installed: $BINARY"

printf 'commit=%s\nbuild_time=%s\ninstalled_at=%s\n' \
    "$NEW_COMMIT" "$NEW_BUILDTIME" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$VERSION_FILE"
ok "Version file written: $VERSION_FILE"

# ── write env file (only if reconfigured) ─────────────────────────────────────

if [[ "$KEEP_CONFIG" -eq 0 ]]; then
    cat > "$ENV_FILE" <<EOF
DATABASE_URL=$DB_URL
REDIS_URL=$REDIS_URL
LISTEN_ADDR=$LISTEN_ADDR
EMBED_PROVIDER=$EMBED_PROVIDER
OPENAI_API_KEY=$OPENAI_API_KEY
OPENAI_EMBED_MODEL=$OPENAI_EMBED_MODEL
OLLAMA_BASE_URL=$OLLAMA_BASE_URL
OLLAMA_EMBED_MODEL=$OLLAMA_EMBED_MODEL
EOF
    chmod 600 "$ENV_FILE"
    ok "Config written: $ENV_FILE"
fi

# ── migrations ────────────────────────────────────────────────────────────────

step "Running migrations"

FAILED_MIGRATIONS=()
for f in migrations/*.sql; do
    OUT="$(psql "$DB_URL" -f "$f" 2>&1)" && ok "$f" || {
        warn "Failed: $f"
        echo "  $OUT"
        FAILED_MIGRATIONS+=("$f")
    }
done

if [[ "${#FAILED_MIGRATIONS[@]}" -gt 0 ]]; then
    echo
    warn "Failed migrations: ${FAILED_MIGRATIONS[*]}"
    ask_yn "Continue anyway? (server may not function correctly)" "n" \
        || die "Aborted. Fix migration errors and re-run."
fi

# ── health check ──────────────────────────────────────────────────────────────

step "Verifying installation"

PORT="${LISTEN_ADDR##*:}"

# Check port isn't already in use by something else
if command -v ss >/dev/null 2>&1; then
    ss -tlnp "sport = :${PORT}" 2>/dev/null | grep -q ":${PORT}" \
        && die "Port ${PORT} is already in use. Set a different LISTEN_ADDR."
elif command -v lsof >/dev/null 2>&1; then
    lsof -i ":${PORT}" -sTCP:LISTEN >/dev/null 2>&1 \
        && die "Port ${PORT} is already in use. Set a different LISTEN_ADDR."
fi

DATABASE_URL="$DB_URL" \
REDIS_URL="$REDIS_URL" \
LISTEN_ADDR="$LISTEN_ADDR" \
EMBED_PROVIDER="${EMBED_PROVIDER:-}" \
"$BINARY" &> /tmp/hiveshare-server-smoke.log &
SERVER_PID=$!

HEALTHY=0
for i in $(seq 1 20); do
    sleep 1
    if curl -sf "http://localhost:${PORT}/health" >/dev/null 2>&1; then
        HEALTHY=1; break
    fi
    # Bail early if process already died
    kill -0 "$SERVER_PID" 2>/dev/null || { warn "Server process exited. Log:"; cat /tmp/hiveshare-server-smoke.log; break; }
done

kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true

if [[ "$HEALTHY" -eq 1 ]]; then
    ok "Server responded healthy on :${PORT}"
    ROLLBACK_NEEDED=0
else
    echo
    warn "Server did not respond. Last log output:"
    cat /tmp/hiveshare-server-smoke.log
    die "Health check failed. Installation rolled back."
fi

# ── restart systemd unit if it was running ────────────────────────────────────

if [[ "$WAS_RUNNING" -eq 1 && -n "$SYSTEMD_UNIT" ]]; then
    step "Restarting service"
    systemctl start "$SYSTEMD_UNIT" && ok "Restarted $SYSTEMD_UNIT" \
        || warn "Could not restart $SYSTEMD_UNIT — start it manually."
fi

# ── done ──────────────────────────────────────────────────────────────────────

echo
echo -e "${GREEN}${BOLD}Installation complete.${NC}"
echo
echo "  Binary  : $BINARY  (commit: $NEW_COMMIT)"
echo "  Config  : $ENV_FILE"
echo
if [[ -z "${SYSTEMD_UNIT:-}" ]]; then
    echo "  To start:"
    echo "    source $ENV_FILE && hiveshare-server"
    echo
    echo "  For a persistent service, see INSTALL.md (systemd section)."
fi
echo
