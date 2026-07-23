#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/smoke-helpers.sh" "$@"

echo "=== Auth Smoke Test ==="

smoke_section "Register"
TS=$(date +%s%N)
REG_CODE=$(curl -s -o "$SMOKE_TMPDIR/auth_reg.json" -w "%{http_code}" -X POST "$SMOKE_BASE/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"auth-${TS}@test.local\",\"name\":\"Auth Test\"}")
REG=$(cat "$SMOKE_TMPDIR/auth_reg.json")
smoke_check "$REG_CODE" "201" "register returns 201"
KEY=$(echo "$REG" | jq -r '.api_key')
[ -n "$KEY" ] && [ "$KEY" != "null" ] && smoke_ok "registered" || smoke_fail "register"
echo "$KEY" | grep -q "^hvs_" && smoke_ok "api_key has hvs_ prefix" || smoke_fail "api_key missing hvs_ prefix"
ID=$(echo "$REG" | jq -r '.id')
[ -n "$ID" ] && [ "$ID" != "null" ] && smoke_ok "response has id" || smoke_fail "response missing id"
AUTH="Authorization: Bearer $KEY"

smoke_section "Whoami"
WHOAMI_CODE=$(curl -s -o "$SMOKE_TMPDIR/auth_whoami.json" -w "%{http_code}" "$SMOKE_BASE/auth/whoami" -H "$AUTH")
WHOAMI=$(cat "$SMOKE_TMPDIR/auth_whoami.json")
smoke_check "$WHOAMI_CODE" "200" "whoami returns 200"
WHOAMI_EMAIL=$(echo "$WHOAMI" | jq -r '.email')
REG_EMAIL=$(echo "$REG" | jq -r '.email')
smoke_check "$WHOAMI_EMAIL" "$REG_EMAIL" "whoami returns correct email"
smoke_check "$(echo "$WHOAMI" | jq -r '.name')" "Auth Test" "whoami returns correct name"

smoke_section "Duplicate email"
EMAIL=$(echo "$REG" | jq -r '.email')
DUP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SMOKE_BASE/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$EMAIL\",\"name\":\"Dup\"}")
smoke_check "$DUP_CODE" "409" "duplicate email rejected"

smoke_section "Missing fields"
BAD_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SMOKE_BASE/auth/register" \
    -H "Content-Type: application/json" -d '{"email":"only-email@test.local"}')
smoke_check "$BAD_CODE" "400" "missing name returns 400"

smoke_section "Auth enforcement"
NO_AUTH=$(curl -s -o /dev/null -w "%{http_code}" "$SMOKE_BASE/auth/whoami")
smoke_check "$NO_AUTH" "401" "no auth returns 401"

BAD_KEY=$(curl -s -o /dev/null -w "%{http_code}" "$SMOKE_BASE/auth/whoami" \
    -H "Authorization: Bearer hvs_invalid")
smoke_check "$BAD_KEY" "401" "bad key returns 401"

smoke_summary "Auth"
