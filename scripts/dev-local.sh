#!/usr/bin/env bash
set -euo pipefail

# dev-local.sh — one-command local development setup
#
# Starts Postgres + Redis, runs migrations, and launches the server.
# No Docker image required — builds and runs from source.
#
# Usage:
#   ./scripts/dev-local.sh           # start everything
#   ./scripts/dev-local.sh --reset   # wipe DB volume first, then start fresh
#   ./scripts/dev-local.sh --stop    # stop containers and exit

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; CYAN='\033[0;36m'; NC='\033[0m'

info()  { echo -e "${BOLD}[hiveshare]${NC} $*"; }
ok()    { echo -e "  ${GREEN}✓${NC} $*"; }
warn()  { echo -e "  ${YELLOW}!${NC} $*"; }
die()   { echo -e "  ${RED}✗ $*${NC}" >&2; exit 1; }
step()  { echo -e "\n${CYAN}── $* ──${NC}"; }

# ── Detect container runtime ──────────────────────────────────────────────────

RUNTIME=""
if command -v docker &>/dev/null && docker compose version &>/dev/null 2>&1; then
    RUNTIME="docker"
elif command -v podman &>/dev/null && podman compose version &>/dev/null 2>&1; then
    RUNTIME="podman"
else
    die "docker compose or podman compose is required. Install Docker Desktop or Podman."
fi

COMPOSE="$RUNTIME compose"

# ── Args ──────────────────────────────────────────────────────────────────────

RESET=0
STOP=0
for arg in "$@"; do
    case "$arg" in
        --reset) RESET=1 ;;
        --stop)  STOP=1 ;;
        --help|-h)
            echo "Usage: $0 [--reset] [--stop]"
            echo "  --reset  Wipe the database volume and start fresh"
            echo "  --stop   Stop containers and exit"
            exit 0 ;;
        *) die "Unknown argument: $arg" ;;
    esac
done

# ── Prerequisites ─────────────────────────────────────────────────────────────

step "Checking prerequisites"

command -v go &>/dev/null || die "Go is not installed. https://go.dev/doc/install"
GO_VERSION=$(go version | awk '{print $3}' | tr -d 'go')
ok "Go $GO_VERSION found"

command -v jq &>/dev/null || warn "jq not found — smoke tests will not run (brew install jq / apt install jq)"
command -v curl &>/dev/null || warn "curl not found — smoke tests will not run"

ok "$RUNTIME compose found"

# ── Stop mode ─────────────────────────────────────────────────────────────────

if [ "$STOP" -eq 1 ]; then
    step "Stopping containers"
    $COMPOSE down
    ok "Stopped"
    exit 0
fi

# ── Reset ─────────────────────────────────────────────────────────────────────

if [ "$RESET" -eq 1 ]; then
    step "Wiping database volume"
    warn "This will delete ALL local hiveshare data"
    read -rp "  Type 'yes' to confirm: " confirm
    [ "$confirm" = "yes" ] || { info "Aborted."; exit 0; }
    $COMPOSE down -v
    ok "Volume wiped"
fi

# ── Start containers ──────────────────────────────────────────────────────────

step "Starting Postgres + Redis"
$COMPOSE up -d

# Wait for Postgres
info "Waiting for Postgres to be healthy..."
for i in $(seq 1 30); do
    if $COMPOSE exec -T postgres pg_isready -U hiveshare -q 2>/dev/null; then
        ok "Postgres ready"
        break
    fi
    if [ "$i" -eq 30 ]; then
        die "Postgres did not become healthy in 30s. Check: $RUNTIME compose logs postgres"
    fi
    sleep 1
done

# Wait for Redis
info "Waiting for Redis..."
for i in $(seq 1 15); do
    if $COMPOSE exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; then
        ok "Redis ready"
        break
    fi
    if [ "$i" -eq 15 ]; then
        die "Redis did not become healthy in 15s. Check: $RUNTIME compose logs redis"
    fi
    sleep 1
done

# ── Migrations ────────────────────────────────────────────────────────────────

step "Applying migrations"
POSTGRES_URL="postgres://hiveshare:hiveshare@localhost:5432/hiveshare?sslmode=disable"

for f in migrations/*.sql; do
    name=$(basename "$f")
    if $COMPOSE exec -T postgres psql -U hiveshare -d hiveshare -f - < "$f" &>/dev/null; then
        ok "$name"
    else
        die "Migration failed: $name"
    fi
done

# ── Build ─────────────────────────────────────────────────────────────────────

step "Building server"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILDTIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w -X github.com/KB-perByte/hiveshare/internal/version.Commit=${COMMIT} -X github.com/KB-perByte/hiveshare/internal/version.BuildTime=${BUILDTIME}"
go build -ldflags="$LDFLAGS" -o bin/hiveshare-server ./cmd/server
ok "bin/hiveshare-server built (commit=$COMMIT)"

# ── Env ───────────────────────────────────────────────────────────────────────

export DATABASE_URL="${DATABASE_URL:-$POSTGRES_URL}"
export REDIS_URL="${REDIS_URL:-redis://localhost:6379}"
export LISTEN_ADDR="${LISTEN_ADDR:-:8080}"
export EMBED_PROVIDER="${EMBED_PROVIDER:-}"
export JWT_SECRET="${JWT_SECRET:-dev-secret-change-in-production}"

if [ "$JWT_SECRET" = "dev-secret-change-in-production" ]; then
    warn "JWT_SECRET is using the insecure dev default. Set JWT_SECRET in your environment for production."
fi

# ── Launch ────────────────────────────────────────────────────────────────────

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  HiveShare dev server starting                       ║${NC}"
echo -e "${BOLD}║                                                      ║${NC}"
echo -e "${BOLD}║  API:    http://localhost:8080                       ║${NC}"
echo -e "${BOLD}║  Health: http://localhost:8080/health                ║${NC}"
echo -e "${BOLD}║  Docs:   http://localhost:8080/docs                  ║${NC}"
echo -e "${BOLD}║                                                      ║${NC}"
echo -e "${BOLD}║  Quick start:                                        ║${NC}"
echo -e "${BOLD}║    curl http://localhost:8080/health                 ║${NC}"
echo -e "${BOLD}║    curl -X POST http://localhost:8080/api/v1/auth/register \\${NC}"
echo -e "${BOLD}║      -H 'Content-Type: application/json' \\           ║${NC}"
echo -e "${BOLD}║      -d '{\"email\":\"you@dev.local\",\"name\":\"You\"}'     ║${NC}"
echo -e "${BOLD}║                                                      ║${NC}"
echo -e "${BOLD}║  Stop containers: ./scripts/dev-local.sh --stop     ║${NC}"
echo -e "${BOLD}║  Wipe and reset:  ./scripts/dev-local.sh --reset    ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════╝${NC}"
echo ""

exec bin/hiveshare-server
