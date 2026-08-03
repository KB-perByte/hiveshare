#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/smoke-helpers.sh" "$@"

echo "=== Service Account Smoke Test ==="

# Setup: owner + hiveshare
REG=$(smoke_register "sa-owner" "SA Owner")
KEY=$(echo "$REG" | jq -r '.api_key')
AUTH="Authorization: Bearer $KEY"

HS_ID=$(curl -sf -X POST "$SMOKE_BASE/hiveshares" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"sa-test-hs"}' | jq -r '.id')

smoke_section "Create service account"

# Create SA with view role
CREATE_CODE=$(curl -s -o "$SMOKE_TMPDIR/sa_create.json" -w "%{http_code}" \
    -X POST "$SMOKE_BASE/hiveshares/$HS_ID/service-accounts" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"ci-bot","role":"view"}')
smoke_check "$CREATE_CODE" "201" "create SA returns 201"

SA_KEY=$(jq -r '.key' < "$SMOKE_TMPDIR/sa_create.json")
SA_ID=$(jq -r '.id' < "$SMOKE_TMPDIR/sa_create.json")
SA_ROLE=$(jq -r '.role' < "$SMOKE_TMPDIR/sa_create.json")

echo "$SA_KEY" | grep -q "^hvsa_" && smoke_ok "SA key has hvsa_ prefix" || smoke_fail "SA key missing hvsa_ prefix"
[ "$SA_ROLE" = "view" ] && smoke_ok "SA role is view" || smoke_fail "SA role expected view, got $SA_ROLE"

# Key shown only once — second create gets a different SA, not the same key again
smoke_section "Create SA with default role (should be view)"
DEFAULT_ROLE=$(curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/service-accounts" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"ci-bot-2","role":""}' | jq -r '.role')
[ "$DEFAULT_ROLE" = "view" ] && smoke_ok "empty role defaults to view" || smoke_fail "empty role expected view, got $DEFAULT_ROLE"

smoke_section "List service accounts"
LIST_CODE=$(curl -s -o "$SMOKE_TMPDIR/sa_list.json" -w "%{http_code}" \
    "$SMOKE_BASE/hiveshares/$HS_ID/service-accounts" \
    -H "$AUTH")
smoke_check "$LIST_CODE" "200" "list SAs returns 200"
COUNT=$(jq 'length' < "$SMOKE_TMPDIR/sa_list.json")
[ "$COUNT" -ge 2 ] && smoke_ok "list returns $COUNT service accounts" || smoke_fail "expected >=2 SAs, got $COUNT"

smoke_section "Mint JWT token"
TOKEN_CODE=$(curl -s -o "$SMOKE_TMPDIR/sa_token.json" -w "%{http_code}" \
    -X POST "$SMOKE_BASE/auth/service-accounts/token" \
    -H "Authorization: Bearer $SA_KEY")
smoke_check "$TOKEN_CODE" "200" "mint token returns 200"

JWT=$(jq -r '.token' < "$SMOKE_TMPDIR/sa_token.json")
EXPIRES_IN=$(jq -r '.expires_in' < "$SMOKE_TMPDIR/sa_token.json")

# JWT has three dot-separated parts
DOT_COUNT=$(echo "$JWT" | tr -cd '.' | wc -c)
[ "$DOT_COUNT" -eq 2 ] && smoke_ok "token is a valid JWT format (3 parts)" || smoke_fail "token doesn't look like a JWT"
[ "$EXPIRES_IN" -gt 0 ] && smoke_ok "expires_in=$EXPIRES_IN" || smoke_fail "expires_in missing or zero"

smoke_section "Use JWT to call API"
SEARCH_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives/search" \
    -H "Authorization: Bearer $JWT" \
    -H "Content-Type: application/json" \
    -d '{"query":"test"}')
smoke_check "$SEARCH_CODE" "200" "JWT bearer accepted for search"

smoke_section "JWT rejected for write (view role)"
# Add a hive entry as owner first so there's something to test against
curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"source_type":"manual","source_ref":"sa-test-1","content":"test content"}' > /dev/null

WRITE_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives" \
    -H "Authorization: Bearer $JWT" \
    -H "Content-Type: application/json" \
    -d '{"source_type":"manual","source_ref":"sa-write-attempt","content":"should be blocked"}')
smoke_check "$WRITE_CODE" "403" "view-role JWT blocked from writing"

smoke_section "Invalid SA key rejected"
INVALID_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$SMOKE_BASE/auth/service-accounts/token" \
    -H "Authorization: Bearer hvsa_invalidkey00000000000000000000000000000000000000")
smoke_check "$INVALID_CODE" "401" "invalid SA key returns 401"

smoke_section "List requires all role (view user blocked)"
REG_V=$(smoke_register "sa-viewer" "Viewer")
KEY_V=$(echo "$REG_V" | jq -r '.api_key')
EMAIL_V=$(echo "$REG_V" | jq -r '.email')

TOKEN_V=$(curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/invite" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d "{\"email\":\"$EMAIL_V\",\"role\":\"view\"}" | jq -r '.token')
curl -sf -X POST "$SMOKE_BASE/invitations/$TOKEN_V/accept" \
    -H "Content-Type: application/json" -d '{}' > /dev/null

VIEW_LIST_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$SMOKE_BASE/hiveshares/$HS_ID/service-accounts" \
    -H "Authorization: Bearer $KEY_V")
smoke_check "$VIEW_LIST_CODE" "403" "view member cannot list service accounts"

smoke_section "Delete service account"
DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X DELETE "$SMOKE_BASE/hiveshares/$HS_ID/service-accounts/$SA_ID" \
    -H "$AUTH")
smoke_check "$DEL_CODE" "204" "delete SA returns 204"

# Deleted SA key should no longer mint tokens
DEAD_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$SMOKE_BASE/auth/service-accounts/token" \
    -H "Authorization: Bearer $SA_KEY")
smoke_check "$DEAD_CODE" "401" "deleted SA key rejected"

smoke_summary "Service accounts"
