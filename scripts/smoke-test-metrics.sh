#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/smoke-helpers.sh" "$@"

echo "=== Metrics Smoke Test ==="

REG=$(smoke_register "met" "Metrics User")
KEY=$(echo "$REG" | jq -r '.api_key')
AUTH="Authorization: Bearer $KEY"

HS=$(curl -sf -X POST "$SMOKE_BASE/hiveshares" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"metrics-test"}')
HS_ID=$(echo "$HS" | jq -r '.id')

curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/hives" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"source_type":"jira","source_ref":"MET-1","content":"For metrics","tool":"claude","tags":[]}' > /dev/null

smoke_section "Hiveshare metrics"
HS_MET_CODE=$(curl -s -o $SMOKE_TMPDIR/met_hs.json -w "%{http_code}" "$SMOKE_BASE/hiveshares/$HS_ID/metrics" -H "$AUTH")
HS_MET=$(cat $SMOKE_TMPDIR/met_hs.json)
smoke_check "$HS_MET_CODE" "200" "hiveshare metrics returns 200"
smoke_check "$(echo "$HS_MET" | jq 'has("hiveshare")')" "true" "has hiveshare summary"
smoke_check "$(echo "$HS_MET" | jq 'has("memory")')" "true" "has memory stats"
smoke_check "$(echo "$HS_MET" | jq 'has("collaboration")')" "true" "has collaboration stats"
smoke_check "$(echo "$HS_MET" | jq 'has("coverage")')" "true" "has coverage stats"
smoke_check "$(echo "$HS_MET" | jq 'has("activity")')" "true" "has activity stats"
TOTAL=$(echo "$HS_MET" | jq '.memory.total_entries')
[ "$TOTAL" -ge 1 ] && smoke_ok "total_entries >= 1" || smoke_fail "total_entries is $TOTAL"

smoke_section "User metrics"
USER_MET_CODE=$(curl -s -o $SMOKE_TMPDIR/met_user.json -w "%{http_code}" "$SMOKE_BASE/metrics/me" -H "$AUTH")
USER_MET=$(cat $SMOKE_TMPDIR/met_user.json)
smoke_check "$USER_MET_CODE" "200" "user metrics returns 200"
smoke_check "$(echo "$USER_MET" | jq 'has("total_entries")')" "true" "has total_entries"
smoke_check "$(echo "$USER_MET" | jq 'has("total_searches")')" "true" "has total_searches"
smoke_check "$(echo "$USER_MET" | jq 'has("hiveshares_owned")')" "true" "has hiveshares_owned"

smoke_section "Cleanup"
curl -sf -X DELETE "$SMOKE_BASE/hiveshares/$HS_ID" -H "$AUTH" > /dev/null
smoke_ok "cleaned up"

smoke_summary "Metrics"
