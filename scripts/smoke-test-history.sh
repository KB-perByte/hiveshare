#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/smoke-helpers.sh" "$@"

echo "=== History Smoke Test ==="

REG=$(smoke_register "hist" "History User")
KEY=$(echo "$REG" | jq -r '.api_key')
AUTH="Authorization: Bearer $KEY"

HS=$(curl -sf -X POST "$SMOKE_BASE/hiveshares" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"hist-test"}')
HS_ID=$(echo "$HS" | jq -r '.id')

# ── Create + verify history ───────────────────────────────────────────────────
smoke_section "Entry history"

ENTRY=$(curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/memory" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"source_type":"manual","source_ref":"hist-1","content":"Original content","tool":"manual","tags":["test"]}')
ENTRY_ID=$(echo "$ENTRY" | jq -r '.id')
smoke_ok "created entry"

HIST=$(curl -sf "$SMOKE_BASE/hiveshares/$HS_ID/memory/$ENTRY_ID/history" -H "$AUTH")
HIST_LEN=$(echo "$HIST" | jq 'length')
[ "$HIST_LEN" -ge 1 ] && smoke_ok "history has $HIST_LEN versions" || smoke_fail "no history"
INSERT_COUNT=$(echo "$HIST" | jq '[.[] | select(.action=="insert")] | length')
smoke_check "$INSERT_COUNT" "1" "insert history row exists"
LAST_ACTION=$(echo "$HIST" | jq -r '.[-1].action')
smoke_check "$LAST_ACTION" "insert" "last history action is insert"
HIST_ID=$(echo "$HIST" | jq '.[0].history_id')

# ── Update + verify history ───────────────────────────────────────────────────
curl -sf -X PUT "$SMOKE_BASE/hiveshares/$HS_ID/memory/$ENTRY_ID" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"content":"Updated content","summary":"updated","tags":["updated"]}' > /dev/null

HIST2=$(curl -sf "$SMOKE_BASE/hiveshares/$HS_ID/memory/$ENTRY_ID/history" -H "$AUTH")
UPDATE_COUNT=$(echo "$HIST2" | jq '[.[] | select(.action=="update")] | length')
[ "$UPDATE_COUNT" -ge 1 ] && smoke_ok "update history row exists" || smoke_fail "no update history"

# ── Rollback ──────────────────────────────────────────────────────────────────
smoke_section "Rollback"

ROLLED=$(curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/memory/$ENTRY_ID/rollback" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d "{\"history_id\":$HIST_ID}")
ROLL_CONTENT=$(echo "$ROLLED" | jq -r '.content')
smoke_check "$ROLL_CONTENT" "Original content" "rollback restored original"

# ── Delete + Undelete ─────────────────────────────────────────────────────────
smoke_section "Delete + Undelete"

ENTRY2=$(curl -sf -X POST "$SMOKE_BASE/hiveshares/$HS_ID/memory" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"source_type":"manual","source_ref":"del-test","content":"Delete me","tool":"manual","tags":[]}')
ENTRY2_ID=$(echo "$ENTRY2" | jq -r '.id')

curl -sf -X DELETE "$SMOKE_BASE/hiveshares/$HS_ID/memory/$ENTRY2_ID" -H "$AUTH" > /dev/null
smoke_ok "deleted entry"

GET_AFTER_DEL=$(curl -s -o /dev/null -w "%{http_code}" \
    "$SMOKE_BASE/hiveshares/$HS_ID/memory/$ENTRY2_ID" -H "$AUTH")
smoke_check "$GET_AFTER_DEL" "404" "deleted entry returns 404"

DEL_HIST=$(curl -sf "$SMOKE_BASE/hiveshares/$HS_ID/memory/$ENTRY2_ID/history" -H "$AUTH")
DEL_HIST_ID=$(echo "$DEL_HIST" | jq '[.[] | select(.action=="delete")][0].history_id')
[ "$DEL_HIST_ID" != "null" ] && smoke_ok "delete history row found" || smoke_fail "no delete history"

UNDEL_CODE=$(curl -s -o $SMOKE_TMPDIR/hist_undel.json -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/memory/undelete" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d "{\"history_id\":$DEL_HIST_ID}")
UNDEL=$(cat $SMOKE_TMPDIR/hist_undel.json)
smoke_check "$UNDEL_CODE" "201" "undelete returns 201"
UNDEL_ID=$(echo "$UNDEL" | jq -r '.id')
smoke_check "$UNDEL_ID" "$ENTRY2_ID" "undeleted with same ID"
UNDEL_CONTENT=$(echo "$UNDEL" | jq -r '.content')
smoke_check "$UNDEL_CONTENT" "Delete me" "undeleted content matches original"

# ── Snapshots ─────────────────────────────────────────────────────────────────
smoke_section "Snapshots"

SNAP_CODE=$(curl -s -o $SMOKE_TMPDIR/hist_snap.json -w "%{http_code}" -X POST "$SMOKE_BASE/hiveshares/$HS_ID/snapshots" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"test-snap","description":"smoke test"}')
SNAP=$(cat $SMOKE_TMPDIR/hist_snap.json)
smoke_check "$SNAP_CODE" "201" "snapshot create returns 201"
SNAP_ID=$(echo "$SNAP" | jq '.snapshot_id')
smoke_check "$(echo "$SNAP" | jq -r '.name')" "test-snap" "snapshot has correct name"
SNAP_EC=$(echo "$SNAP" | jq '.entry_count')
[ "$SNAP_EC" -ge 1 ] && smoke_ok "snapshot has $SNAP_EC entries" || smoke_fail "snapshot empty"

SNAP_LIST=$(curl -sf "$SMOKE_BASE/hiveshares/$HS_ID/snapshots" -H "$AUTH")
SL=$(echo "$SNAP_LIST" | jq 'length')
[ "$SL" -ge 1 ] && smoke_ok "listed $SL snapshots" || smoke_fail "no snapshots"

SNAP_DETAIL=$(curl -sf "$SMOKE_BASE/hiveshares/$HS_ID/snapshots/$SNAP_ID" -H "$AUTH")
smoke_check "$(echo "$SNAP_DETAIL" | jq 'has("snapshot")')" "true" "detail has snapshot key"
smoke_check "$(echo "$SNAP_DETAIL" | jq 'has("entries")')" "true" "detail has entries key"
SE=$(echo "$SNAP_DETAIL" | jq '.entries | length')
[ "$SE" -ge 1 ] && smoke_ok "snapshot detail has $SE entries" || smoke_fail "detail empty"

RESTORE_CODE=$(curl -s -o "$SMOKE_TMPDIR/hist_restore.json" -w "%{http_code}" -X POST \
    "$SMOKE_BASE/hiveshares/$HS_ID/snapshots/$SNAP_ID/restore" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"restored-hs"}')
RESTORED=$(cat "$SMOKE_TMPDIR/hist_restore.json")
smoke_check "$RESTORE_CODE" "201" "restore returns 201"
NEW_HS_ID=$(echo "$RESTORED" | jq -r '.hiveshare.id')
smoke_check "$(echo "$RESTORED" | jq -r '.hiveshare.name')" "restored-hs" "restored hiveshare has correct name"
[ "$NEW_HS_ID" != "$HS_ID" ] && smoke_ok "restored to new hiveshare" || smoke_fail "same ID"
RESTORED_EC=$(echo "$RESTORED" | jq '.entries_restored')
[ "$RESTORED_EC" -ge 1 ] && smoke_ok "$RESTORED_EC entries restored" || smoke_fail "no entries"

DEL_SNAP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "$SMOKE_BASE/hiveshares/$HS_ID/snapshots/$SNAP_ID" -H "$AUTH")
smoke_check "$DEL_SNAP_CODE" "204" "snapshot deleted"

# ── Copy ──────────────────────────────────────────────────────────────────────
smoke_section "Copy"

COPY_HS=$(curl -sf -X POST "$SMOKE_BASE/hiveshares" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"name":"copy-target"}')
COPY_HS_ID=$(echo "$COPY_HS" | jq -r '.id')

COPY_CODE=$(curl -s -o "$SMOKE_TMPDIR/hist_copy.json" -w "%{http_code}" -X POST \
    "$SMOKE_BASE/hiveshares/$COPY_HS_ID/memory/copy" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d "{\"entry_ids\":[\"$ENTRY_ID\"]}")
COPIED=$(cat "$SMOKE_TMPDIR/hist_copy.json")
smoke_check "$COPY_CODE" "201" "copy returns 201"
COPY_LEN=$(echo "$COPIED" | jq 'length')
smoke_check "$COPY_LEN" "1" "copied 1 entry"
COPY_HS_CHECK=$(echo "$COPIED" | jq -r '.[0].hiveshare_id')
smoke_check "$COPY_HS_CHECK" "$COPY_HS_ID" "entry in target hiveshare"
COPY_CONTENT=$(echo "$COPIED" | jq -r '.[0].content')
smoke_check "$COPY_CONTENT" "Original content" "copied content matches source"

# ── Cleanup ───────────────────────────────────────────────────────────────────
smoke_section "Cleanup"
curl -sf -X DELETE "$SMOKE_BASE/hiveshares/$COPY_HS_ID" -H "$AUTH" > /dev/null
curl -sf -X DELETE "$SMOKE_BASE/hiveshares/$NEW_HS_ID" -H "$AUTH" > /dev/null
curl -sf -X DELETE "$SMOKE_BASE/hiveshares/$HS_ID" -H "$AUTH" > /dev/null
smoke_ok "cleaned up"

smoke_summary "History"
