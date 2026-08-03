#!/usr/bin/env bash
set -euo pipefail

# dev-local.sh — one-command local development setup
#
# Builds the server image, starts all three containers (Postgres, Redis,
# hiveshare-server), runs migrations, and tails logs.
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
            echo "  --stop   Stop all containers and exit"
            exit 0 ;;
        *) die "Unknown argument: $arg" ;;
    esac
done

# ── Prerequisites ─────────────────────────────────────────────────────────────

step "Checking prerequisites"

command -v jq &>/dev/null || warn "jq not found — smoke tests will not run (brew install jq / apt install jq)"
command -v curl &>/dev/null || warn "curl not found — smoke tests will not run"

ok "$RUNTIME compose found"

if [ "${JWT_SECRET:-}" = "" ]; then
    warn "JWT_SECRET not set — using insecure dev default. Set it for production."
    export JWT_SECRET="dev-secret-change-in-production"
fi

# ── Stop mode ─────────────────────────────────────────────────────────────────

if [ "$STOP" -eq 1 ]; then
    step "Stopping containers"
    $COMPOSE --profile server down
    ok "Stopped"
    exit 0
fi

# ── Reset ─────────────────────────────────────────────────────────────────────

if [ "$RESET" -eq 1 ]; then
    step "Wiping database volume"
    warn "This will delete ALL local hiveshare data"
    read -rp "  Type 'yes' to confirm: " confirm
    [ "$confirm" = "yes" ] || { info "Aborted."; exit 0; }
    $COMPOSE --profile server down -v
    ok "Volume wiped"
fi

# ── Start infra (Postgres + Redis) ────────────────────────────────────────────

step "Starting Postgres + Redis"
$COMPOSE up -d  # no --profile server yet — infra only first

info "Waiting for Postgres to be healthy..."
for i in $(seq 1 30); do
    if $COMPOSE exec -T postgres pg_isready -U hiveshare -q 2>/dev/null; then
        ok "Postgres ready"
        break
    fi
    [ "$i" -eq 30 ] && die "Postgres did not become healthy in 30s. Check: $RUNTIME compose logs postgres"
    sleep 1
done

info "Waiting for Redis..."
for i in $(seq 1 15); do
    if $COMPOSE exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; then
        ok "Redis ready"
        break
    fi
    [ "$i" -eq 15 ] && die "Redis did not become healthy in 15s. Check: $RUNTIME compose logs redis"
    sleep 1
done

# ── Migrations ────────────────────────────────────────────────────────────────

step "Applying migrations"
for f in migrations/*.sql; do
    name=$(basename "$f")
    if $COMPOSE exec -T postgres psql -U hiveshare -d hiveshare -f - < "$f" &>/dev/null; then
        ok "$name"
    else
        die "Migration failed: $name — check: $RUNTIME compose logs postgres"
    fi
done

# ── Build + start server container ────────────────────────────────────────────

step "Building server image"
$COMPOSE --profile server build hiveshare
ok "Image built"

step "Starting server container"
$COMPOSE --profile server up -d hiveshare

info "Waiting for server to be healthy..."
for i in $(seq 1 30); do
    STATUS=$(curl -s http://localhost:8080/health 2>/dev/null | grep -o '"status":"ok"' || true)
    if [ "$STATUS" = '"status":"ok"' ]; then
        ok "Server healthy"
        break
    fi
    [ "$i" -eq 30 ] && die "Server did not become healthy. Check: $RUNTIME compose logs hiveshare"
    sleep 2
done

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  HiveShare is running                                ║${NC}"
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
echo -e "${BOLD}║  Logs:   $RUNTIME compose logs -f hiveshare          ║${NC}"
echo -e "${BOLD}║  Stop:   ./scripts/dev-local.sh --stop               ║${NC}"
echo -e "${BOLD}║  Reset:  ./scripts/dev-local.sh --reset              ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════╝${NC}"
echo ""
info "Tailing server logs (Ctrl-C to detach — containers keep running):"
$COMPOSE logs -f hiveshare
