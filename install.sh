#!/usr/bin/env bash
# HiveShare CLI + MCP installer
# Usage: curl -sSL https://raw.githubusercontent.com/KB-perByte/hiveshare/main/install.sh | bash
set -euo pipefail

REPO="KB-perByte/hiveshare"
INSTALL_DIR="${HIVESHARE_INSTALL_DIR:-$HOME/.local/bin}"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"

# ── helpers ──────────────────────────────────────────────────────────────────

info()  { printf '\033[0;34m  =>\033[0m %s\n' "$*"; }
ok()    { printf '\033[0;32m  ✓\033[0m  %s\n' "$*"; }
die()   { printf '\033[0;31m  ✗\033[0m  %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" &>/dev/null || die "Required tool not found: $1"; }

# ── detect OS / arch ─────────────────────────────────────────────────────────

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) die "Unsupported OS: $OS" ;;
esac

PLATFORM="${OS}_${ARCH}"

# ── find latest release ───────────────────────────────────────────────────────

need curl
need tar

info "Fetching latest release from GitHub..."
RELEASE_JSON=$(curl -fsSL "$GITHUB_API")
TAG=$(printf '%s' "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

[ -n "$TAG" ] || die "Could not determine latest release tag"
info "Latest release: $TAG"

# ── download & install ────────────────────────────────────────────────────────

BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
TARBALL="hiveshare_${PLATFORM}.tar.gz"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading ${TARBALL}..."
curl -fsSL "${BASE_URL}/${TARBALL}" -o "${TMP}/${TARBALL}"

info "Extracting..."
tar -xzf "${TMP}/${TARBALL}" -C "$TMP"

mkdir -p "$INSTALL_DIR"

for BIN in hshare hiveshare-mcp hiveshare-server; do
  SRC="${TMP}/${BIN}"
  if [ -f "$SRC" ]; then
    install -m 755 "$SRC" "${INSTALL_DIR}/${BIN}"
    ok "Installed ${BIN} → ${INSTALL_DIR}/${BIN}"
  fi
done

# ── PATH check ────────────────────────────────────────────────────────────────

if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
  printf '\n\033[0;33m  ! %s is not in your PATH.\033[0m\n' "$INSTALL_DIR"
  printf '    Add this to your shell profile:\n'
  printf '    \033[0;36mexport PATH="%s:$PATH"\033[0m\n\n' "$INSTALL_DIR"
fi

# ── done ─────────────────────────────────────────────────────────────────────

printf '\n\033[0;32mHiveShare %s installed.\033[0m\n\n' "$TAG"
printf 'Next steps:\n'
printf '  1. Start the server:  see https://github.com/%s#quick-start\n' "$REPO"
printf '  2. Register:          hshare auth register --email you@example.com --name "You"\n'
printf '  3. Create a space:    hshare hiveshare create "My Project"\n\n'
