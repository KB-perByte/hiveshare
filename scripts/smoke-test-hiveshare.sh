#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/smoke-helpers.sh" "$@"

echo "=== Hiveshare Smoke Test ==="

REG_A=$(smoke_register "hs-a" "User A")
KEY_A=$(echo "$REG_A" | jq -r '.api_key')
AUTH_A="Authorization: Bearer $KEY_A"

REG_B=$(smoke_register "hs-b" "User B")
KEY_B=$(echo "$REG_B" | jq -r '.api_key')
AUTH_B="Authorization: Bearer $KEY_B"

smoke_section "Create"
HS_RESP=$(curl -s -o $SMOKE_TMPDIR/hs_create.json -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d '{"name":"hs-test","description":"smoke"}')
HS=$(cat $SMOKE_TMPDIR/hs_create.json)
HS_ID=$(echo "$HS" | jq -r '.id')
smoke_check "$HS_RESP" "201" "create returns 201"
smoke_check "$(echo "$HS" | jq -r '.name')" "hs-test" "create returns correct name"
smoke_check "$(echo "$HS" | jq -r '.role')" "all" "creator gets all role"
smoke_check "$(echo "$HS" | jq '.member_count')" "1" "starts with 1 member"

smoke_section "List"
LIST_CODE=$(curl -s -o $SMOKE_TMPDIR/hs_list.json -w "%{http_code}" "$SMOKE_BASE/hiveshares" -H "$AUTH_A")
LIST=$(cat $SMOKE_TMPDIR/hs_list.json)
smoke_check "$LIST_CODE" "200" "list returns 200"
echo "$LIST" | jq -r '.[].id' | grep -q "$HS_ID" && smoke_ok "in list" || smoke_fail "not in list"

smoke_section "Get"
GET_CODE=$(curl -s -o $SMOKE_TMPDIR/hs_get.json -w "%{http_code}" "$SMOKE_BASE/hiveshares/$HS_ID" -H "$AUTH_A")
GET=$(cat $SMOKE_TMPDIR/hs_get.json)
smoke_check "$GET_CODE" "200" "get returns 200"
smoke_check "$(echo "$GET" | jq -r '.name')" "hs-test" "get returns correct name"
smoke_check "$(echo "$GET" | jq -r '.id')" "$HS_ID" "get returns correct id"

FORBID=$(curl -s -o /dev/null -w "%{http_code}" "$SMOKE_BASE/hiveshares/$HS_ID" -H "$AUTH_B")
smoke_check "$FORBID" "404" "non-member gets 404"

smoke_section "Update"
UPD_CODE=$(curl -s -o $SMOKE_TMPDIR/hs_upd.json -w "%{http_code}" -X PUT "$SMOKE_BASE/hiveshares/$HS_ID" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d '{"name":"updated","description":"updated"}')
smoke_check "$UPD_CODE" "200" "update returns 200"
smoke_check "$(cat $SMOKE_TMPDIR/hs_upd.json | jq -r '.name')" "updated" "update changes name"

smoke_section "Invite & Members"
EMAIL_B=$(echo "$REG_B" | jq -r '.email')
INV_CODE=$(curl -s -o $SMOKE_TMPDIR/hs_inv.json -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/invite" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d "{\"email\":\"$EMAIL_B\",\"role\":\"view\"}")
INV=$(cat $SMOKE_TMPDIR/hs_inv.json)
smoke_check "$INV_CODE" "201" "invite returns 201"
TOKEN=$(echo "$INV" | jq -r '.token')
[ -n "$TOKEN" ] && smoke_ok "invitation has token" || smoke_fail "invite missing token"

ACCEPT_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SMOKE_BASE/invitations/$TOKEN/accept" \
    -H "Content-Type: application/json" -d '{}')
smoke_check "$ACCEPT_CODE" "200" "accept returns 200"

MEM_CODE=$(curl -s -o $SMOKE_TMPDIR/hs_mem.json -w "%{http_code}" "$SMOKE_BASE/hiveshares/$HS_ID/members" -H "$AUTH_A")
MEMBERS=$(cat $SMOKE_TMPDIR/hs_mem.json)
smoke_check "$MEM_CODE" "200" "members returns 200"
MC=$(echo "$MEMBERS" | jq 'length')
[ "$MC" -ge 2 ] && smoke_ok "$MC members after invite" || smoke_fail "expected >= 2 members"

B_READ=$(curl -s -o /dev/null -w "%{http_code}" "$SMOKE_BASE/hiveshares/$HS_ID" -H "$AUTH_B")
smoke_check "$B_READ" "200" "invited user can read"

smoke_section "Delete"
DEL_HS=$(curl -sf -X POST "$SMOKE_BASE/hiveshares" \
    -H "$AUTH_A" -H "Content-Type: application/json" -d '{"name":"del-me"}')
DEL_ID=$(echo "$DEL_HS" | jq -r '.id')
DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$SMOKE_BASE/hiveshares/$DEL_ID" -H "$AUTH_A")
smoke_check "$DEL_CODE" "204" "delete returns 204"

smoke_section "Cleanup"
curl -sf -X DELETE "$SMOKE_BASE/hiveshares/$HS_ID" -H "$AUTH_A" > /dev/null
smoke_ok "cleaned up"

smoke_summary "Hiveshare"
