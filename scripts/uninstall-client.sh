#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; CYAN='\033[0;36m'; NC='\033[0m'

ok()   { echo -e "${GREEN}  ✓${NC} $*"; }
warn() { echo -e "${YELLOW}  !${NC} $*"; }
step() { echo -e "\n${CYAN}── $* ${NC}"; }

ask_yn() {
    local prompt="$1" default="${2:-n}" var
    read -rp "  $prompt (y/N): " var
    [[ "${var:-$default}" =~ ^[Yy]$ ]]
}

echo
echo -e "${BOLD}╔══════════════════════════════════╗${NC}"
echo -e "${BOLD}║  HiveShare Client Uninstaller    ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════╝${NC}"
echo

# ── paths ─────────────────────────────────────────────────────────────────────

INSTALL_DIR="${HOME}/.local/bin"
BINARY="${INSTALL_DIR}/hshare"
VERSION_FILE="${INSTALL_DIR}/hshare.version"
CONFIG_FILE="${HOME}/.config/hiveshare/config.json"
CONFIG_DIR="${HOME}/.config/hiveshare"

# ── check anything is installed ───────────────────────────────────────────────

if [[ ! -f "$BINARY" ]]; then
    warn "hshare not found at $BINARY — nothing to remove."
    exit 0
fi

VERSION="unknown"
[[ -f "$VERSION_FILE" ]] && VERSION="$(grep '^commit=' "$VERSION_FILE" | cut -d= -f2)"
echo "  Found: $BINARY  (commit: $VERSION)"
echo

ask_yn "Uninstall hshare?" "n" || { echo "Aborted."; exit 0; }

# ── remove binary ─────────────────────────────────────────────────────────────

step "Removing binary"

rm -f "$BINARY"       && ok "Removed $BINARY"
rm -f "$VERSION_FILE"  && ok "Removed $VERSION_FILE"

# ── remove config ─────────────────────────────────────────────────────────────

step "Config"

if [[ -f "$CONFIG_FILE" ]]; then
    if ask_yn "Remove config file $CONFIG_FILE? (contains your API key)" "n"; then
        rm -f "$CONFIG_FILE" && ok "Removed $CONFIG_FILE"
        # Remove config dir only if empty (server.env may still be there)
        rmdir "$CONFIG_DIR" 2>/dev/null && ok "Removed $CONFIG_DIR" || true
    else
        warn "Config kept at $CONFIG_FILE"
    fi
else
    ok "No config file found"
fi

# ── remove PATH entry from shell profile ──────────────────────────────────────

step "PATH cleanup"

PATH_LINE="export PATH=\"\$HOME/.local/bin:\$PATH\""
FISH_LINE="fish_add_path $INSTALL_DIR"
CLEANED=0

for profile in "${HOME}/.bashrc" "${HOME}/.zshrc"; do
    if [[ -f "$profile" ]] && grep -qF "$PATH_LINE" "$profile"; then
        if ask_yn "Remove PATH entry from $profile?" "y"; then
            # Use a temp file to avoid in-place sed issues on all platforms
            grep -vF "$PATH_LINE" "$profile" > "${profile}.tmp" && mv "${profile}.tmp" "$profile"
            ok "Removed from $profile"
            CLEANED=1
        fi
    fi
done

FISH_CONFIG="${HOME}/.config/fish/config.fish"
if [[ -f "$FISH_CONFIG" ]] && grep -qF "$FISH_LINE" "$FISH_CONFIG"; then
    if ask_yn "Remove PATH entry from $FISH_CONFIG?" "y"; then
        grep -vF "$FISH_LINE" "$FISH_CONFIG" > "${FISH_CONFIG}.tmp" && mv "${FISH_CONFIG}.tmp" "$FISH_CONFIG"
        ok "Removed from $FISH_CONFIG"
        CLEANED=1
    fi
fi

[[ "$CLEANED" -eq 0 ]] && ok "No shell profile entries found"

# ── done ──────────────────────────────────────────────────────────────────────

echo
echo -e "${GREEN}${BOLD}Client uninstalled.${NC}"
echo
