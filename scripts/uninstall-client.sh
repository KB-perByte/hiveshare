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
BINARY="${INSTALL_DIR}/hiveshare"
VERSION_FILE="${INSTALL_DIR}/hiveshare.version"
MCP_BINARY="${INSTALL_DIR}/hiveshare-mcp"
MCP_VERSION_FILE="${INSTALL_DIR}/hiveshare-mcp.version"
CONFIG_FILE="${HOME}/.config/hiveshare/config.json"
CONFIG_DIR="${HOME}/.config/hiveshare"

HAS_JQ=0
command -v jq >/dev/null 2>&1 && HAS_JQ=1

# ── check anything is installed ───────────────────────────────────────────────

if [[ ! -f "$BINARY" && ! -f "$MCP_BINARY" ]]; then
    warn "Neither hiveshare nor hiveshare-mcp found in $INSTALL_DIR — nothing to remove."
    exit 0
fi

VERSION="unknown"
[[ -f "$VERSION_FILE" ]] && VERSION="$(grep '^commit=' "$VERSION_FILE" | cut -d= -f2)"
[[ -f "$BINARY"     ]] && echo "  Found: $BINARY  (commit: $VERSION)"
[[ -f "$MCP_BINARY" ]] && echo "  Found: $MCP_BINARY"
echo

ask_yn "Uninstall hiveshare CLI + MCP sidecar?" "n" || { echo "Aborted."; exit 0; }

# ── remove binary ─────────────────────────────────────────────────────────────

step "Removing binaries"

[[ -f "$BINARY"          ]] && rm -f "$BINARY"          && ok "Removed $BINARY"
[[ -f "$VERSION_FILE"    ]] && rm -f "$VERSION_FILE"    && ok "Removed $VERSION_FILE"
[[ -f "$MCP_BINARY"      ]] && rm -f "$MCP_BINARY"      && ok "Removed $MCP_BINARY"
[[ -f "$MCP_VERSION_FILE" ]] && rm -f "$MCP_VERSION_FILE" && ok "Removed $MCP_VERSION_FILE"

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

# ── remove hiveshare from AI tool MCP configs ─────────────────────────────────

step "AI tool config cleanup"

dewire_ai_config() {
    local cfg_file="$1" label="$2"
    [[ ! -f "$cfg_file" ]] && return
    if ! grep -q '"hiveshare"' "$cfg_file" 2>/dev/null; then
        ok "$label: no hiveshare entry found"
        return
    fi
    if [[ "$HAS_JQ" -eq 1 ]]; then
        local tmp; tmp="$(mktemp)"
        jq 'del(.mcpServers.hiveshare)' "$cfg_file" > "$tmp" && mv "$tmp" "$cfg_file"
        ok "Removed hiveshare from $label ($cfg_file)"
    else
        warn "jq not found — remove the hiveshare entry manually from $cfg_file"
    fi
}

dewire_ai_config "${HOME}/.claude/claude_desktop_config.json" "Claude Code"
dewire_ai_config "${HOME}/.cursor/mcp.json"                   "Cursor"

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
echo -e "${GREEN}${BOLD}Client and MCP sidecar uninstalled.${NC}"
echo
