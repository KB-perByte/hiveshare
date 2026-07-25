#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/smoke-helpers.sh" "$@"

echo "=== Memory Smoke Test ==="

REG_A=$(smoke_register "mem-a" "Writer")
KEY_A=$(echo "$REG_A" | jq -r '.api_key')
AUTH_A="Authorization: Bearer $KEY_A"

REG_B=$(smoke_register "mem-b" "Viewer")
KEY_B=$(echo "$REG_B" | jq -r '.api_key')
AUTH_B="Authorization: Bearer $KEY_B"

HS=$(curl -sf -X POST "$SMOKE_BASE/hiveshares" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d '{"name":"mem-test"}')
HS_ID=$(echo "$HS" | jq -r '.id')

EMAIL_B=$(echo "$REG_B" | jq -r '.email')
TOKEN=$(curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/invite" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d "{\"email\":\"$EMAIL_B\",\"role\":\"view\"}" | jq -r '.token')
curl -sf -X POST "$SMOKE_BASE/invitations/$TOKEN/accept" \
    -H "Content-Type: application/json" -d '{}' > /dev/null

smoke_section "Create"
CREATE_CODE=$(curl -s -o $SMOKE_TMPDIR/mem_create.json -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d '{"source_type":"jira","source_ref":"MEM-1","content":"Test content","tool":"claude","tags":["test"]}')
ENTRY=$(cat $SMOKE_TMPDIR/mem_create.json)
ENTRY_ID=$(echo "$ENTRY" | jq -r '.id')
smoke_check "$CREATE_CODE" "201" "create returns 201"
[ -n "$ENTRY_ID" ] && smoke_ok "created entry has id" || smoke_fail "create missing id"
smoke_check "$(echo "$ENTRY" | jq -r '.source_type')" "jira" "create returns source_type"
smoke_check "$(echo "$ENTRY" | jq -r '.source_ref')" "MEM-1" "create returns source_ref"
smoke_check "$(echo "$ENTRY" | jq -r '.tool')" "claude" "create returns tool"

BAD_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives" \
    -H "$AUTH_A" -H "Content-Type: application/json" -d '{"content":"no source"}')
smoke_check "$BAD_CODE" "400" "missing fields returns 400"

VIEW_WRITE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives" \
    -H "$AUTH_B" -H "Content-Type: application/json" \
    -d '{"source_type":"manual","source_ref":"x","content":"x","tool":"manual"}')
smoke_check "$VIEW_WRITE" "403" "view-only cannot write"

smoke_section "List"
LIST_CODE=$(curl -s -o $SMOKE_TMPDIR/mem_list.json -w "%{http_code}" "$SMOKE_BASE/hiveshares/$HS_ID/hives" -H "$AUTH_A")
MEM_LIST=$(cat $SMOKE_TMPDIR/mem_list.json)
smoke_check "$LIST_CODE" "200" "list returns 200"
MC=$(echo "$MEM_LIST" | jq 'length')
[ "$MC" -ge 1 ] && smoke_ok "listed $MC entries" || smoke_fail "list empty"

smoke_section "List filter by source_type"
FILTERED=$(curl -sf "$SMOKE_BASE/hiveshares/$HS_ID/hives?source_type=jira" -H "$AUTH_A")
FILTERED_LEN=$(echo "$FILTERED" | jq 'length')
[ "$FILTERED_LEN" -ge 1 ] && smoke_ok "filtered list has $FILTERED_LEN entries" || smoke_fail "filter returned 0"
FILTERED_TYPES=$(echo "$FILTERED" | jq -r '.[].source_type' | sort -u)
smoke_check "$FILTERED_TYPES" "jira" "all filtered entries are jira"

smoke_section "Get"
GET_CODE=$(curl -s -o $SMOKE_TMPDIR/mem_get.json -w "%{http_code}" "$SMOKE_BASE/hiveshares/$HS_ID/hives/$ENTRY_ID" -H "$AUTH_A")
GET_ENTRY=$(cat $SMOKE_TMPDIR/mem_get.json)
smoke_check "$GET_CODE" "200" "get returns 200"
smoke_check "$(echo "$GET_ENTRY" | jq -r '.id')" "$ENTRY_ID" "get returns correct id"
smoke_check "$(echo "$GET_ENTRY" | jq -r '.content')" "Test content" "get returns content"
smoke_check "$(echo "$GET_ENTRY" | jq 'has("content")')" "true" "get has content key"

smoke_section "Search"
SEARCH_CODE=$(curl -s -o $SMOKE_TMPDIR/mem_search.json -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives/search" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d '{"query":"test content","limit":5}')
SEARCH=$(cat $SMOKE_TMPDIR/mem_search.json)
smoke_check "$SEARCH_CODE" "200" "search returns 200"
SC=$(echo "$SEARCH" | jq '.count')
[ "$SC" -ge 1 ] && smoke_ok "found $SC results" || smoke_fail "search returned 0"
smoke_check "$(echo "$SEARCH" | jq 'has("results")')" "true" "search has results key"
smoke_check "$(echo "$SEARCH" | jq 'has("count")')" "true" "search has count key"
smoke_check "$(echo "$SEARCH" | jq 'has("query")')" "true" "search has query key"

SEARCH_400=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives/search" \
    -H "$AUTH_A" -H "Content-Type: application/json" -d '{"limit":5}')
smoke_check "$SEARCH_400" "400" "search without query returns 400"

smoke_section "Update"
UPD_CODE=$(curl -s -o $SMOKE_TMPDIR/mem_upd.json -w "%{http_code}" -X PUT "$SMOKE_BASE/hiveshares/$HS_ID/hives/$ENTRY_ID" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d '{"content":"Updated","summary":"upd","tags":["updated"]}')
smoke_check "$UPD_CODE" "200" "update returns 200"
smoke_check "$(cat $SMOKE_TMPDIR/mem_upd.json | jq -r '.content')" "Updated" "update changes content"

smoke_section "Delete"
DEL_ENTRY=$(curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives" \
    -H "$AUTH_A" -H "Content-Type: application/json" \
    -d '{"source_type":"manual","source_ref":"del","content":"delete me","tool":"manual","tags":[]}')
DEL_ID=$(echo "$DEL_ENTRY" | jq -r '.id')
DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "$SMOKE_BASE/hiveshares/$HS_ID/hives/$DEL_ID" -H "$AUTH_A")
smoke_check "$DEL_CODE" "204" "delete returns 204"

smoke_section "Cleanup"
curl -sf -X DELETE "$SMOKE_BASE/hiveshares/$HS_ID" -H "$AUTH_A" > /dev/null
smoke_ok "cleaned up"

smoke_summary "Memory"
