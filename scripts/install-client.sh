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
echo -e "${BOLD}║   HiveShare Client Installer     ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════╝${NC}"
echo

# ── paths ─────────────────────────────────────────────────────────────────────

INSTALL_DIR="${HOME}/.local/bin"
VERSION_FILE="${INSTALL_DIR}/hiveshare.version"
CONFIG_DIR="${HOME}/.config/hiveshare"
CONFIG_FILE="${CONFIG_DIR}/config.json"
BINARY="${INSTALL_DIR}/hiveshare"
BACKUP_BINARY="${BINARY}.bak"
BACKUP_CONFIG="${CONFIG_FILE}.bak"
ROLLBACK_NEEDED=0

cleanup() {
    local exit_code=$?
    if [[ "$exit_code" -ne 0 && "$ROLLBACK_NEEDED" -eq 1 ]]; then
        warn "Installation failed — rolling back..."
        [[ -f "$BACKUP_BINARY" ]] && mv -f "$BACKUP_BINARY" "$BINARY" && warn "Restored previous binary."
        [[ -f "$BACKUP_CONFIG" ]] && mv -f "$BACKUP_CONFIG" "$CONFIG_FILE" && warn "Restored previous config."
    fi
    rm -f "$BACKUP_BINARY" "$BACKUP_CONFIG" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"

# ── prerequisites ─────────────────────────────────────────────────────────────

step "Checking prerequisites"

command -v go >/dev/null 2>&1 || die "Go is not installed. See https://go.dev/doc/install"
ok "Go $(go version | awk '{print $3}')"

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

# ── PATH setup ────────────────────────────────────────────────────────────────

step "Checking PATH"

if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    warn "$INSTALL_DIR is not in your PATH."

    SHELL_PROFILE=""
    case "${SHELL:-}" in
        */zsh)  SHELL_PROFILE="${HOME}/.zshrc" ;;
        */fish) SHELL_PROFILE="${HOME}/.config/fish/config.fish" ;;
        *)      SHELL_PROFILE="${HOME}/.bashrc" ;;
    esac

    if ask_yn "Add $INSTALL_DIR to PATH in $SHELL_PROFILE?" "y"; then
        if [[ "${SHELL:-}" == */fish ]]; then
            echo "fish_add_path $INSTALL_DIR" >> "$SHELL_PROFILE"
        else
            echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> "$SHELL_PROFILE"
        fi
        ok "Added to $SHELL_PROFILE — restart your shell or run: source $SHELL_PROFILE"
        export PATH="$INSTALL_DIR:$PATH"
    else
        warn "Not added. Run manually: export PATH=\"\$HOME/.local/bin:\$PATH\""
    fi
else
    ok "$INSTALL_DIR is in PATH"
fi

# ── backup existing installation ──────────────────────────────────────────────

if [[ "$UPGRADING" -eq 1 ]]; then
    step "Backing up existing installation"
    cp -f "$BINARY" "$BACKUP_BINARY" && ok "Binary backed up"
    [[ -f "$CONFIG_FILE" ]] && cp -f "$CONFIG_FILE" "$BACKUP_CONFIG" && ok "Config backed up"
    ROLLBACK_NEEDED=1
fi

# ── build ─────────────────────────────────────────────────────────────────────

step "Building"

BUILD_TMP="$(mktemp)"
trap 'rm -f "$BUILD_TMP"; cleanup' EXIT

go build -ldflags="$LDFLAGS" -o "$BUILD_TMP" ./cmd/hiveshare \
    || die "Build failed."

mv -f "$BUILD_TMP" "$BINARY"
chmod +x "$BINARY"
ok "Binary installed: $BINARY"

printf 'commit=%s\nbuild_time=%s\ninstalled_at=%s\n' \
    "$NEW_COMMIT" "$NEW_BUILDTIME" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$VERSION_FILE"

"$BINARY" --version >/dev/null 2>&1 && ok "Binary runs: $("$BINARY" --version 2>&1 | head -1)"

# ── config setup ──────────────────────────────────────────────────────────────

step "Configuration"

KEEP_CONFIG=0
EXISTING_API_KEY=""
EXISTING_SERVER_URL=""

if [[ -f "$CONFIG_FILE" && "$UPGRADING" -eq 1 ]]; then
    EXISTING_API_KEY="$(grep -o '"api_key"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | cut -d'"' -f4 || true)"
    EXISTING_SERVER_URL="$(grep -o '"server_url"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | cut -d'"' -f4 || true)"
    warn "Existing config found: $CONFIG_FILE"
    [[ -n "$EXISTING_SERVER_URL" ]] && warn "  server_url : $EXISTING_SERVER_URL"
    [[ -n "$EXISTING_API_KEY"    ]] && warn "  api_key    : ${EXISTING_API_KEY:0:8}…"
    echo
    if ask_yn "Keep existing config?" "y"; then
        KEEP_CONFIG=1
        ok "Keeping existing config."
    fi
fi

if [[ "$KEEP_CONFIG" -eq 0 ]]; then
    echo
    DEFAULT_URL="${EXISTING_SERVER_URL:-http://localhost:8080}"
    SERVER_URL=$(ask "HiveShare server URL" "$DEFAULT_URL")

    OFFLINE=0
    if ! curl -sf "${SERVER_URL}/health" >/dev/null 2>&1; then
        warn "Cannot reach ${SERVER_URL}/health"
        if ask_yn "Continue in offline mode? (skip account setup and verification)" "n"; then
            OFFLINE=1
        else
            die "Aborted. Start the server or provide the correct URL."
        fi
    else
        ok "Server is reachable"
    fi

    API_KEY=""

    if [[ "$OFFLINE" -eq 0 ]]; then
        echo
        echo "  Choose one:"
        echo "    1) Register a new account"
        echo "    2) Use an existing API key"
        read -rp "  Choice [1/2]: " CHOICE

        case "${CHOICE:-1}" in
            1)
                NAME=$(ask "Your name")
                EMAIL=$(ask "Email address")
                REGISTER_RESP="$(curl -sf -X POST "${SERVER_URL}/api/v1/auth/register" \
                    -H "Content-Type: application/json" \
                    -d "{\"name\":\"${NAME}\",\"email\":\"${EMAIL}\"}" 2>&1)" || {
                    HTTP_CODE="${PIPESTATUS[0]:-1}"
                    # Handle already-registered email
                    if echo "$REGISTER_RESP" | grep -qi "already\|conflict\|exists"; then
                        warn "That email is already registered."
                        if ask_yn "Enter your existing API key instead?" "y"; then
                            API_KEY=$(ask_secret "API key")
                        else
                            die "Aborted."
                        fi
                    else
                        die "Registration failed: $REGISTER_RESP"
                    fi
                }
                if [[ -z "$API_KEY" ]]; then
                    API_KEY="$(echo "$REGISTER_RESP" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)"
                    [[ -n "$API_KEY" ]] || die "Could not parse API key from response: $REGISTER_RESP"
                    ok "Registered as $NAME ($EMAIL)"
                fi
                ;;
            2)
                DEFAULT_KEY="${EXISTING_API_KEY:-}"
                if [[ -n "$DEFAULT_KEY" ]]; then
                    API_KEY=$(ask_secret "API key (press Enter to keep existing ${DEFAULT_KEY:0:8}…)")
                    API_KEY="${API_KEY:-$DEFAULT_KEY}"
                else
                    API_KEY=$(ask_secret "API key")
                fi
                [[ -n "$API_KEY" ]] || die "API key cannot be empty."
                ;;
            *)
                die "Invalid choice."
                ;;
        esac
    else
        # Offline: preserve existing key or leave blank
        API_KEY="${EXISTING_API_KEY:-}"
        [[ -z "$API_KEY" ]] && warn "No API key set — run 'hiveshare auth register' once the server is available."
    fi

    cat > "$CONFIG_FILE" <<EOF
{
  "server_url": "${SERVER_URL}",
  "api_key": "${API_KEY}",
  "default_hiveshare": "",
  "default_hiveshare_name": ""
}
EOF
    chmod 600 "$CONFIG_FILE"
    ok "Config written: $CONFIG_FILE"
fi

# ── verify API access ─────────────────────────────────────────────────────────

step "Verifying"

# Re-read config in case we kept existing
FINAL_URL="$(grep -o '"server_url"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | cut -d'"' -f4 || true)"
FINAL_KEY="$(grep -o '"api_key"[[:space:]]*:[[:space:]]*"[^"]*"'    "$CONFIG_FILE" | cut -d'"' -f4 || true)"

if [[ -n "$FINAL_KEY" && -n "$FINAL_URL" ]]; then
    WHOAMI="$(curl -sf "${FINAL_URL}/api/v1/auth/whoami" \
        -H "Authorization: Bearer ${FINAL_KEY}" 2>&1)" && {
        NAME_OUT="$(echo "$WHOAMI" | grep -o '"name":"[^"]*"' | cut -d'"' -f4 || true)"
        ok "Authenticated as: ${NAME_OUT:-<unknown>}"
        ROLLBACK_NEEDED=0
    } || {
        HTTP_STATUS="$(curl -o /dev/null -sw '%{http_code}' "${FINAL_URL}/api/v1/auth/whoami" \
            -H "Authorization: Bearer ${FINAL_KEY}" 2>/dev/null || echo "000")"
        if [[ "$HTTP_STATUS" == "401" ]]; then
            warn "API key rejected (401). The key in $CONFIG_FILE may be wrong."
            warn "Update it with: hiveshare auth register"
        elif [[ "$HTTP_STATUS" == "000" ]]; then
            warn "Server unreachable — verification skipped. Run 'hiveshare auth status' when the server is up."
        else
            warn "Unexpected response (HTTP $HTTP_STATUS). Check server logs."
        fi
        ROLLBACK_NEEDED=0  # don't roll back just because server is down
    }
else
    warn "No API key configured — skipping API verification."
    ROLLBACK_NEEDED=0
fi

# ── done ──────────────────────────────────────────────────────────────────────

echo
echo -e "${GREEN}${BOLD}Installation complete.${NC}"
echo
echo "  Binary  : $BINARY  (commit: $NEW_COMMIT)"
echo "  Config  : $CONFIG_FILE"
echo
echo "  Quick start:"
echo "    hiveshare list           # list your hiveshares"
echo "    hiveshare create <name>  # create a new hiveshare"
echo "    hiveshare use <id>       # set active hiveshare"
echo "    hiveshare hive add --content '…'   # save a hive"
echo "    hiveshare hive search '<query>'    # search hives"
echo "    hiveshare hive history <id>        # version history"
echo "    hiveshare snapshot list  # list snapshots"
echo
