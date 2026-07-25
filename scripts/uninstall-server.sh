#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; CYAN='\033[0;36m'; NC='\033[0m'

ok()   { echo -e "${GREEN}  ✓${NC} $*"; }
warn() { echo -e "${YELLOW}  !${NC} $*"; }
die()  { echo -e "${RED}  ✗ $*${NC}" >&2; exit 1; }
step() { echo -e "\n${CYAN}── $* ${NC}"; }

ask_yn() {
    local prompt="$1" default="${2:-n}" var
    read -rp "  $prompt (y/N): " var
    [[ "${var:-$default}" =~ ^[Yy]$ ]]
}

NUKE=0
for arg in "$@"; do
    [[ "$arg" == "--nuke" ]] && NUKE=1
done

echo
if [[ "$NUKE" -eq 1 ]]; then
echo -e "${BOLD}╔══════════════════════════════════╗${NC}"
echo -e "${RED}${BOLD}║  HiveShare SERVER NUKE           ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════╝${NC}"
else
echo -e "${BOLD}╔══════════════════════════════════╗${NC}"
echo -e "${BOLD}║  HiveShare Server Uninstaller    ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════╝${NC}"
fi
echo

# ── locate install paths ──────────────────────────────────────────────────────

if [[ "$EUID" -eq 0 ]]; then
    INSTALL_DIR="/usr/local/bin"
    CONFIG_DIR="/etc/hiveshare"
else
    INSTALL_DIR="${HOME}/.local/bin"
    CONFIG_DIR="${HOME}/.config/hiveshare"
fi

BINARY="$INSTALL_DIR/hiveshare-server"
VERSION_FILE="$INSTALL_DIR/hiveshare-server.version"
ENV_FILE="$CONFIG_DIR/server.env"

# ── check anything is installed ───────────────────────────────────────────────

if [[ ! -f "$BINARY" ]]; then
    warn "hiveshare-server not found at $BINARY — nothing to remove."
    exit 0
fi

VERSION="unknown"
[[ -f "$VERSION_FILE" ]] && VERSION="$(grep '^commit=' "$VERSION_FILE" | cut -d= -f2)"
echo "  Found: $BINARY  (commit: $VERSION)"
echo

ask_yn "Uninstall hiveshare-server?" "n" || { echo "Aborted."; exit 0; }

# ── stop systemd service if running ──────────────────────────────────────────

step "Checking for running service"

STOPPED_UNIT=""
if command -v systemctl >/dev/null 2>&1; then
    for unit in hiveshare hiveshare-server; do
        if systemctl is-active --quiet "$unit" 2>/dev/null; then
            systemctl stop "$unit" && ok "Stopped $unit"
            systemctl disable "$unit" 2>/dev/null && ok "Disabled $unit"
            STOPPED_UNIT="$unit"
            break
        fi
    done
fi

if [[ -z "$STOPPED_UNIT" ]] && pgrep -x hiveshare-server >/dev/null 2>&1; then
    pkill -x hiveshare-server && ok "Killed hiveshare-server process"
fi

[[ -z "$STOPPED_UNIT" ]] && ok "No running service found"

# ── remove binary ─────────────────────────────────────────────────────────────

step "Removing binary"

rm -f "$BINARY"      && ok "Removed $BINARY"
rm -f "$VERSION_FILE" && ok "Removed $VERSION_FILE"

# ── remove config ─────────────────────────────────────────────────────────────

step "Config"

if [[ -f "$ENV_FILE" ]]; then
    if ask_yn "Remove config file $ENV_FILE? (contains DB/Redis credentials)" "n"; then
        rm -f "$ENV_FILE" && ok "Removed $ENV_FILE"
        # Remove the dir only if it's now empty
        rmdir "$CONFIG_DIR" 2>/dev/null && ok "Removed $CONFIG_DIR" || true
    else
        warn "Config kept at $ENV_FILE"
    fi
else
    ok "No config file found"
fi

# ── systemd unit file ─────────────────────────────────────────────────────────

UNIT_FILE="/etc/systemd/system/hiveshare.service"
if [[ -f "$UNIT_FILE" ]]; then
    if ask_yn "Remove systemd unit file $UNIT_FILE?" "y"; then
        rm -f "$UNIT_FILE"
        systemctl daemon-reload 2>/dev/null || true
        ok "Removed $UNIT_FILE"
    fi
fi

# ── nuke: drop postgres database ─────────────────────────────────────────────

if [[ "$NUKE" -eq 1 ]]; then
    step "Database nuke"

    # Resolve DATABASE_URL: prefer env, then env file, then prompt
    DB_URL="${DATABASE_URL:-}"
    if [[ -z "$DB_URL" && -f "$ENV_FILE" ]]; then
        DB_URL="$(grep '^DATABASE_URL=' "$ENV_FILE" | cut -d= -f2-)"
    fi
    if [[ -z "$DB_URL" ]]; then
        read -rp "  DATABASE_URL: " DB_URL
    fi

    [[ -z "$DB_URL" ]] && die "No DATABASE_URL — cannot drop database."

    # Extract db name for display
    DB_NAME="$(basename "${DB_URL%%\?*}")"

    echo
    echo -e "  ${RED}${BOLD}This will permanently destroy all data in: ${DB_NAME}${NC}"
    echo -e "  ${RED}There is no undo.${NC}"
    echo
    read -rp "  Type the database name to confirm: " CONFIRM
    [[ "$CONFIRM" == "$DB_NAME" ]] || die "Name did not match — aborted."

    # Terminate active connections first, then drop
    psql "$DB_URL" -c "
        SELECT pg_terminate_backend(pid)
        FROM pg_stat_activity
        WHERE datname = '$DB_NAME' AND pid <> pg_backend_pid();
    " -q 2>/dev/null || true

    # Connect to postgres maintenance db to drop
    BASE_URL="${DB_URL%/$DB_NAME*}/postgres"
    psql "$BASE_URL" -c "DROP DATABASE IF EXISTS \"${DB_NAME}\";" \
        && ok "Dropped database: $DB_NAME" \
        || die "DROP DATABASE failed. You may need superuser privileges."
fi

# ── done ──────────────────────────────────────────────────────────────────────

echo
if [[ "$NUKE" -eq 1 ]]; then
    echo -e "${RED}${BOLD}Server nuked. Binary, config, service, and database removed.${NC}"
else
    echo -e "${GREEN}${BOLD}Server uninstalled.${NC}"
    echo
    echo "  Database and Redis data were NOT removed."
    echo "  For a full wipe, re-run with --nuke:"
    echo "    $0 --nuke"
fi
echo
