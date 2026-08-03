#!/usr/bin/env bash
# Shared helpers for smoke test scripts. Source this, don't run it.

SMOKE_BASE="${1:-${HIVESHARE_TEST_URL:-http://localhost:8080}}/api/v1"
SMOKE_PASS=0
SMOKE_FAIL=0
SMOKE_TMPDIR=$(mktemp -d "${TMPDIR:-/tmp}/hiveshare-smoke.XXXXXX")
trap 'rm -rf "$SMOKE_TMPDIR"' EXIT

smoke_ok()   { SMOKE_PASS=$((SMOKE_PASS+1)); echo "  PASS: $1"; }
smoke_fail() { SMOKE_FAIL=$((SMOKE_FAIL+1)); echo "  FAIL: $1"; }
smoke_check() {
    if [ "$1" = "$2" ]; then smoke_ok "$3"; else smoke_fail "$3 (expected '$2', got '$1')"; fi
}
smoke_section() { echo ""; echo "── $1 ──"; }
smoke_summary() {
    echo ""
    echo "=== $1: $SMOKE_PASS passed, $SMOKE_FAIL failed ==="
    [ "$SMOKE_FAIL" -eq 0 ] && return 0 || return 1
}

smoke_register() {
    local suffix="$1"
    local name="$2"
    local ts result code
    ts=$(date +%s%N)
    # Retry up to 3 times with a short backoff in case the public rate limit
    # (10 req/min by IP) is transiently hit during rapid test runs.
    for i in 1 2 3; do
        code=$(curl -s -o "$SMOKE_TMPDIR/reg_${suffix}.json" -w "%{http_code}" \
            -X POST "$SMOKE_BASE/auth/register" \
            -H "Content-Type: application/json" \
            -d "{\"email\":\"${suffix}-${ts}@test.local\",\"name\":\"${name}\"}")
        if [ "$code" = "201" ]; then
            cat "$SMOKE_TMPDIR/reg_${suffix}.json"
            return
        fi
        [ "$code" = "429" ] && sleep 7 || { echo "register failed ($code)"; return 1; }
    done
    echo "register failed after retries"
    return 1
}
