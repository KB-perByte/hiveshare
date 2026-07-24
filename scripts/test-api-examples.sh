#!/usr/bin/env bash
set -euo pipefail

# Tests every curl example from API.md against the running server.
# Usage: ./scripts/test-api-examples.sh [base_url]

BASE="${1:-${HIVESHARE_TEST_URL:-http://localhost:8080}}"
API="$BASE/api/v1"
PASS=0
FAIL=0
TMPDIR=$(mktemp -d "${TMPDIR:-/tmp}/hiveshare-api-examples.XXXXXX")
trap 'rm -rf "$TMPDIR"' EXIT

ok()   { PASS=$((PASS+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); echo "  FAIL: $1"; }
check_code() {
    if [ "$1" = "$2" ]; then ok "$3"; else fail "$3 (expected $2, got $1)"; fi
}
section() { echo ""; echo "── $1 ──"; }

TS=$(date +%s%N)

echo "=== API.md Curl Example Verification ==="
echo "Target: $BASE"

# ── Health ────────────────────────────────────────────────────────────────────
section "Health"

CODE=$(curl -s -o "$TMPDIR/health.json" -w "%{http_code}" "$BASE/health")
check_code "$CODE" "200" "GET /health"
jq -e '.status' "$TMPDIR/health.json" > /dev/null && ok "has status field" || fail "missing status"
jq -e '.db' "$TMPDIR/health.json" > /dev/null && ok "has db field" || fail "missing db"
jq -e '.redis' "$TMPDIR/health.json" > /dev/null && ok "has redis field" || fail "missing redis"
jq -e '.commit' "$TMPDIR/health.json" > /dev/null && ok "has commit field" || fail "missing commit"
jq -e '.build_time' "$TMPDIR/health.json" > /dev/null && ok "has build_time field" || fail "missing build_time"

# ── Auth: Register ────────────────────────────────────────────────────────────
section "Auth: Register"

CODE=$(curl -s -o "$TMPDIR/reg.json" -w "%{http_code}" -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"apidoc-a-${TS}@test.local\",\"name\":\"Alice\"}")
check_code "$CODE" "201" "POST /auth/register"
KEY_A=$(jq -r '.api_key' "$TMPDIR/reg.json")
[ -n "$KEY_A" ] && [ "$KEY_A" != "null" ] && ok "api_key present" || fail "api_key missing"
echo "$KEY_A" | grep -q "^hvs_" && ok "api_key has hvs_ prefix" || fail "bad prefix"
jq -e '.id' "$TMPDIR/reg.json" > /dev/null && ok "has id" || fail "missing id"
AUTH_A="Authorization: Bearer $KEY_A"

CODE=$(curl -s -o "$TMPDIR/reg_b.json" -w "%{http_code}" -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"apidoc-b-${TS}@test.local\",\"name\":\"Bob\"}")
check_code "$CODE" "201" "register user B"
KEY_B=$(jq -r '.api_key' "$TMPDIR/reg_b.json")
AUTH_B="Authorization: Bearer $KEY_B"
EMAIL_B=$(jq -r '.email' "$TMPDIR/reg_b.json")

# ── Auth: Duplicate ───────────────────────────────────────────────────────────
section "Auth: Duplicate"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"apidoc-a-${TS}@test.local\",\"name\":\"Dup\"}")
check_code "$CODE" "409" "duplicate email rejected"

# ── Auth: Missing fields ─────────────────────────────────────────────────────
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"only-email@test.local"}')
check_code "$CODE" "400" "missing name returns 400"

# ── Auth: Whoami ──────────────────────────────────────────────────────────────
section "Auth: Whoami"

CODE=$(curl -s -o "$TMPDIR/whoami.json" -w "%{http_code}" "$API/auth/whoami" -H "$AUTH_A")
check_code "$CODE" "200" "GET /auth/whoami"
NAME=$(jq -r '.name' "$TMPDIR/whoami.json")
[ "$NAME" = "Alice" ] && ok "name matches" || fail "name: $NAME"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/auth/whoami")
check_code "$CODE" "401" "no auth returns 401"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/auth/whoami" \
  -H "Authorization: Bearer hvs_bogus")
check_code "$CODE" "401" "bad key returns 401"

# ── Hiveshares: Create ────────────────────────────────────────────────────────
section "Hiveshares: Create"

CODE=$(curl -s -o "$TMPDIR/hs_create.json" -w "%{http_code}" -X POST "$API/hiveshares" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"name":"Sprint 42","description":"Shared context"}')
check_code "$CODE" "201" "POST /hiveshares"
HS_ID=$(jq -r '.id' "$TMPDIR/hs_create.json")
[ "$(jq -r '.name' "$TMPDIR/hs_create.json")" = "Sprint 42" ] && ok "name matches" || fail "name mismatch"
[ "$(jq -r '.role' "$TMPDIR/hs_create.json")" = "all" ] && ok "role is all" || fail "wrong role"
[ "$(jq '.member_count' "$TMPDIR/hs_create.json")" = "1" ] && ok "member_count is 1" || fail "wrong member_count"

# ── Hiveshares: List ──────────────────────────────────────────────────────────
section "Hiveshares: List"

CODE=$(curl -s -o "$TMPDIR/hs_list.json" -w "%{http_code}" "$API/hiveshares" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares"
jq -r '.[].id' "$TMPDIR/hs_list.json" | grep -q "$HS_ID" && ok "hiveshare in list" || fail "not in list"

# ── Hiveshares: Get ───────────────────────────────────────────────────────────
section "Hiveshares: Get"

CODE=$(curl -s -o "$TMPDIR/hs_get.json" -w "%{http_code}" "$API/hiveshares/$HS_ID" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares/{id}"
[ "$(jq -r '.id' "$TMPDIR/hs_get.json")" = "$HS_ID" ] && ok "id matches" || fail "id mismatch"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/hiveshares/$HS_ID" -H "$AUTH_B")
check_code "$CODE" "404" "non-member gets 404"

# ── Hiveshares: Update ────────────────────────────────────────────────────────
section "Hiveshares: Update"

CODE=$(curl -s -o "$TMPDIR/hs_upd.json" -w "%{http_code}" -X PUT "$API/hiveshares/$HS_ID" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"name":"Renamed","description":"Updated"}')
check_code "$CODE" "200" "PUT /hiveshares/{id}"
[ "$(jq -r '.name' "$TMPDIR/hs_upd.json")" = "Renamed" ] && ok "name updated" || fail "name not updated"

# ── Hiveshares: Invite ────────────────────────────────────────────────────────
section "Hiveshares: Invite"

CODE=$(curl -s -o "$TMPDIR/invite.json" -w "%{http_code}" -X POST "$API/hiveshares/$HS_ID/invite" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL_B\",\"role\":\"view\"}")
check_code "$CODE" "201" "POST /hiveshares/{id}/invite"
TOKEN=$(jq -r '.token' "$TMPDIR/invite.json")
[ -n "$TOKEN" ] && ok "has token" || fail "missing token"
jq -e '.invite_url' "$TMPDIR/invite.json" > /dev/null && ok "has invite_url" || fail "missing invite_url"

# ── Hiveshares: Accept Invite ─────────────────────────────────────────────────
section "Hiveshares: Accept Invite"

CODE=$(curl -s -o "$TMPDIR/accept.json" -w "%{http_code}" -X POST "$API/invitations/$TOKEN/accept" \
  -H "Content-Type: application/json" -d '{"name":"Bob"}')
check_code "$CODE" "200" "POST /invitations/{token}/accept"
jq -e '.hiveshare_id' "$TMPDIR/accept.json" > /dev/null && ok "has hiveshare_id" || fail "missing hiveshare_id"

# ── Hiveshares: Members ───────────────────────────────────────────────────────
section "Hiveshares: Members"

CODE=$(curl -s -o "$TMPDIR/members.json" -w "%{http_code}" "$API/hiveshares/$HS_ID/members" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares/{id}/members"
MC=$(jq 'length' "$TMPDIR/members.json")
[ "$MC" -ge 2 ] && ok "$MC members" || fail "expected >= 2"

# ── Memory: Create ────────────────────────────────────────────────────────────
section "Memory: Create"

CODE=$(curl -s -o "$TMPDIR/mem_create.json" -w "%{http_code}" -X POST "$API/hiveshares/$HS_ID/memory" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"source_type":"jira","source_ref":"PROJ-123","content":"Analysis of auth refactor","tool":"claude","tags":["auth"]}')
check_code "$CODE" "201" "POST /hiveshares/{id}/memory"
ENTRY_ID=$(jq -r '.id' "$TMPDIR/mem_create.json")
[ "$(jq -r '.source_type' "$TMPDIR/mem_create.json")" = "jira" ] && ok "source_type" || fail "wrong source_type"
[ "$(jq -r '.source_ref' "$TMPDIR/mem_create.json")" = "PROJ-123" ] && ok "source_ref" || fail "wrong source_ref"
[ "$(jq -r '.tool' "$TMPDIR/mem_create.json")" = "claude" ] && ok "tool" || fail "wrong tool"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/hiveshares/$HS_ID/memory" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"content":"no source"}')
check_code "$CODE" "400" "missing fields returns 400"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/hiveshares/$HS_ID/memory" \
  -H "$AUTH_B" -H "Content-Type: application/json" \
  -d '{"source_type":"manual","source_ref":"x","content":"x","tool":"manual"}')
check_code "$CODE" "403" "view-only cannot write"

# ── Memory: List ──────────────────────────────────────────────────────────────
section "Memory: List"

CODE=$(curl -s -o "$TMPDIR/mem_list.json" -w "%{http_code}" \
  "$API/hiveshares/$HS_ID/memory?source_type=jira&limit=10" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares/{id}/memory"
[ "$(jq 'length' "$TMPDIR/mem_list.json")" -ge 1 ] && ok "has entries" || fail "empty list"

# ── Memory: Get ───────────────────────────────────────────────────────────────
section "Memory: Get"

CODE=$(curl -s -o "$TMPDIR/mem_get.json" -w "%{http_code}" \
  "$API/hiveshares/$HS_ID/memory/$ENTRY_ID" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares/{id}/memory/{entryId}"
[ "$(jq -r '.id' "$TMPDIR/mem_get.json")" = "$ENTRY_ID" ] && ok "id matches" || fail "id mismatch"
jq -e '.content' "$TMPDIR/mem_get.json" > /dev/null && ok "has content" || fail "missing content"

# ── Memory: Update ────────────────────────────────────────────────────────────
section "Memory: Update"

CODE=$(curl -s -o "$TMPDIR/mem_upd.json" -w "%{http_code}" -X PUT \
  "$API/hiveshares/$HS_ID/memory/$ENTRY_ID" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"content":"Updated analysis...","tags":["auth","updated"]}')
check_code "$CODE" "200" "PUT /hiveshares/{id}/memory/{entryId}"
[ "$(jq -r '.content' "$TMPDIR/mem_upd.json")" = "Updated analysis..." ] && ok "content updated" || fail "content not updated"

# ── Memory: Search ────────────────────────────────────────────────────────────
section "Memory: Search"

CODE=$(curl -s -o "$TMPDIR/search.json" -w "%{http_code}" -X POST \
  "$API/hiveshares/$HS_ID/memory/search" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"query":"auth refactor","limit":5}')
check_code "$CODE" "200" "POST /hiveshares/{id}/memory/search"
jq -e '.results' "$TMPDIR/search.json" > /dev/null && ok "has results" || fail "missing results"
jq -e '.count' "$TMPDIR/search.json" > /dev/null && ok "has count" || fail "missing count"
jq -e '.query' "$TMPDIR/search.json" > /dev/null && ok "has query" || fail "missing query"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
  "$API/hiveshares/$HS_ID/memory/search" \
  -H "$AUTH_A" -H "Content-Type: application/json" -d '{"limit":5}')
check_code "$CODE" "400" "search missing query returns 400"

# ── History: List ─────────────────────────────────────────────────────────────
section "History: List"

CODE=$(curl -s -o "$TMPDIR/hist.json" -w "%{http_code}" \
  "$API/hiveshares/$HS_ID/memory/$ENTRY_ID/history?limit=10" -H "$AUTH_A")
check_code "$CODE" "200" "GET /memory/{entryId}/history"
HIST_LEN=$(jq 'length' "$TMPDIR/hist.json")
[ "$HIST_LEN" -ge 1 ] && ok "$HIST_LEN versions" || fail "no history"
HIST_ID=$(jq '.[- 1].history_id' "$TMPDIR/hist.json")
jq -e '.[0].action' "$TMPDIR/hist.json" > /dev/null && ok "has action field" || fail "missing action"
jq -e '.[0] | has("has_embedding")' "$TMPDIR/hist.json" > /dev/null && ok "has has_embedding field" || fail "missing has_embedding"

# ── History: Rollback ─────────────────────────────────────────────────────────
section "History: Rollback"

CODE=$(curl -s -o "$TMPDIR/rollback.json" -w "%{http_code}" -X POST \
  "$API/hiveshares/$HS_ID/memory/$ENTRY_ID/rollback" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d "{\"history_id\":$HIST_ID}")
check_code "$CODE" "200" "POST /memory/{entryId}/rollback"
[ "$(jq -r '.content' "$TMPDIR/rollback.json")" = "Analysis of auth refactor" ] && ok "content restored" || fail "content not restored"

# ── History: Delete + Undelete ────────────────────────────────────────────────
section "History: Undelete"

ENTRY2=$(curl -s -X POST "$API/hiveshares/$HS_ID/memory" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"source_type":"manual","source_ref":"del-test","content":"Delete me","tool":"manual","tags":[]}')
ENTRY2_ID=$(echo "$ENTRY2" | jq -r '.id')

curl -s -X DELETE "$API/hiveshares/$HS_ID/memory/$ENTRY2_ID" -H "$AUTH_A" > /dev/null

CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/hiveshares/$HS_ID/memory/$ENTRY2_ID" -H "$AUTH_A")
check_code "$CODE" "404" "deleted entry returns 404"

DEL_HIST=$(curl -s "$API/hiveshares/$HS_ID/memory/$ENTRY2_ID/history" -H "$AUTH_A")
DEL_HIST_ID=$(echo "$DEL_HIST" | jq '[.[] | select(.action=="delete")][0].history_id')

CODE=$(curl -s -o "$TMPDIR/undel.json" -w "%{http_code}" -X POST \
  "$API/hiveshares/$HS_ID/memory/undelete" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d "{\"history_id\":$DEL_HIST_ID}")
check_code "$CODE" "201" "POST /memory/undelete"
[ "$(jq -r '.id' "$TMPDIR/undel.json")" = "$ENTRY2_ID" ] && ok "same id" || fail "id mismatch"
[ "$(jq -r '.content' "$TMPDIR/undel.json")" = "Delete me" ] && ok "content restored" || fail "content mismatch"

# ── Snapshots: Create ─────────────────────────────────────────────────────────
section "Snapshots: Create"

CODE=$(curl -s -o "$TMPDIR/snap.json" -w "%{http_code}" -X POST \
  "$API/hiveshares/$HS_ID/snapshots" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"name":"before-cleanup","description":"Snapshot before removing stale entries"}')
check_code "$CODE" "201" "POST /hiveshares/{id}/snapshots"
SNAP_ID=$(jq '.snapshot_id' "$TMPDIR/snap.json")
[ "$(jq -r '.name' "$TMPDIR/snap.json")" = "before-cleanup" ] && ok "name matches" || fail "name mismatch"
[ "$(jq '.entry_count' "$TMPDIR/snap.json")" -ge 1 ] && ok "entry_count >= 1" || fail "no entries"

# ── Snapshots: List ───────────────────────────────────────────────────────────
section "Snapshots: List"

CODE=$(curl -s -o "$TMPDIR/snap_list.json" -w "%{http_code}" \
  "$API/hiveshares/$HS_ID/snapshots" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares/{id}/snapshots"
[ "$(jq 'length' "$TMPDIR/snap_list.json")" -ge 1 ] && ok "has snapshots" || fail "empty list"

# ── Snapshots: Get ────────────────────────────────────────────────────────────
section "Snapshots: Get"

CODE=$(curl -s -o "$TMPDIR/snap_get.json" -w "%{http_code}" \
  "$API/hiveshares/$HS_ID/snapshots/$SNAP_ID" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares/{id}/snapshots/{snapshotId}"
jq -e '.snapshot' "$TMPDIR/snap_get.json" > /dev/null && ok "has snapshot key" || fail "missing snapshot"
jq -e '.entries' "$TMPDIR/snap_get.json" > /dev/null && ok "has entries key" || fail "missing entries"
[ "$(jq '.entries | length' "$TMPDIR/snap_get.json")" -ge 1 ] && ok "has entries" || fail "no entries"

# ── Snapshots: Restore ────────────────────────────────────────────────────────
section "Snapshots: Restore"

CODE=$(curl -s -o "$TMPDIR/restore.json" -w "%{http_code}" -X POST \
  "$API/hiveshares/$HS_ID/snapshots/$SNAP_ID/restore" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d '{"name":"Sprint 42 (restored)"}')
check_code "$CODE" "201" "POST /snapshots/{snapshotId}/restore"
NEW_HS_ID=$(jq -r '.hiveshare.id' "$TMPDIR/restore.json")
[ "$NEW_HS_ID" != "$HS_ID" ] && ok "new hiveshare id" || fail "same id"
[ "$(jq -r '.hiveshare.name' "$TMPDIR/restore.json")" = "Sprint 42 (restored)" ] && ok "name matches" || fail "name mismatch"
[ "$(jq '.entries_restored' "$TMPDIR/restore.json")" -ge 1 ] && ok "entries restored" || fail "no entries"

# ── Snapshots: Delete ─────────────────────────────────────────────────────────
section "Snapshots: Delete"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
  "$API/hiveshares/$HS_ID/snapshots/$SNAP_ID" -H "$AUTH_A")
check_code "$CODE" "204" "DELETE /snapshots/{snapshotId}"

# ── Memory: Copy ──────────────────────────────────────────────────────────────
section "Memory: Copy"

CODE=$(curl -s -o "$TMPDIR/copy.json" -w "%{http_code}" -X POST \
  "$API/hiveshares/$NEW_HS_ID/memory/copy" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d "{\"entry_ids\":[\"$ENTRY_ID\"]}")
check_code "$CODE" "201" "POST /hiveshares/{id}/memory/copy"
[ "$(jq 'length' "$TMPDIR/copy.json")" = "1" ] && ok "copied 1 entry" || fail "wrong count"
[ "$(jq -r '.[0].hiveshare_id' "$TMPDIR/copy.json")" = "$NEW_HS_ID" ] && ok "in target hiveshare" || fail "wrong hiveshare"

# ── Memory: Delete ────────────────────────────────────────────────────────────
section "Memory: Delete"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
  "$API/hiveshares/$HS_ID/memory/$ENTRY2_ID" -H "$AUTH_A")
check_code "$CODE" "204" "DELETE /hiveshares/{id}/memory/{entryId}"

# ── Metrics ───────────────────────────────────────────────────────────────────
section "Metrics"

CODE=$(curl -s -o "$TMPDIR/hs_met.json" -w "%{http_code}" \
  "$API/hiveshares/$HS_ID/metrics" -H "$AUTH_A")
check_code "$CODE" "200" "GET /hiveshares/{id}/metrics"
jq -e '.hiveshare' "$TMPDIR/hs_met.json" > /dev/null && ok "has hiveshare" || fail "missing"
jq -e '.memory' "$TMPDIR/hs_met.json" > /dev/null && ok "has memory" || fail "missing"
jq -e '.collaboration' "$TMPDIR/hs_met.json" > /dev/null && ok "has collaboration" || fail "missing"
jq -e '.coverage' "$TMPDIR/hs_met.json" > /dev/null && ok "has coverage" || fail "missing"
jq -e '.activity' "$TMPDIR/hs_met.json" > /dev/null && ok "has activity" || fail "missing"

CODE=$(curl -s -o "$TMPDIR/user_met.json" -w "%{http_code}" "$API/metrics/me" -H "$AUTH_A")
check_code "$CODE" "200" "GET /metrics/me"
jq -e '.total_entries' "$TMPDIR/user_met.json" > /dev/null && ok "has total_entries" || fail "missing"

# ── Hiveshares: Delete (owner only) ──────────────────────────────────────────
section "Hiveshares: Delete"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$API/hiveshares/$HS_ID" -H "$AUTH_B")
check_code "$CODE" "403" "non-owner cannot delete"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$API/hiveshares/$HS_ID" -H "$AUTH_A")
check_code "$CODE" "204" "DELETE /hiveshares/{id}"

# ── Remove member (tested via self-leave before delete) ───────────────────────
section "Members: Remove"

HS3=$(curl -s -X POST "$API/hiveshares" \
  -H "$AUTH_A" -H "Content-Type: application/json" -d '{"name":"remove-test"}')
HS3_ID=$(echo "$HS3" | jq -r '.id')
INV3=$(curl -s -X POST "$API/hiveshares/$HS3_ID/invite" \
  -H "$AUTH_A" -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL_B\",\"role\":\"view\"}")
TOK3=$(echo "$INV3" | jq -r '.token')
curl -s -X POST "$API/invitations/$TOK3/accept" -H "Content-Type: application/json" -d '{}' > /dev/null
USER_B_ID=$(jq -r '.id' "$TMPDIR/reg_b.json")

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
  "$API/hiveshares/$HS3_ID/members/$USER_B_ID" -H "$AUTH_A")
check_code "$CODE" "204" "DELETE /hiveshares/{id}/members/{userId}"

# cleanup
curl -s -X DELETE "$API/hiveshares/$HS3_ID" -H "$AUTH_A" > /dev/null
curl -s -X DELETE "$API/hiveshares/$NEW_HS_ID" -H "$AUTH_A" > /dev/null

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
