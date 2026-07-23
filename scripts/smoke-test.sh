#!/usr/bin/env bash
set -euo pipefail

# Basic connectivity smoke test — no user required.
# Tests: health endpoint, response format, DB/Redis status.
# Usage: ./scripts/smoke-test.sh [base_url]

BASE="${1:-${HIVESHARE_TEST_URL:-http://localhost:8080}}"
PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); echo "  FAIL: $1"; }

echo "=== HiveShare Basic Smoke Test ==="
echo "Target: $BASE"
echo ""

# ── Health endpoint ───────────────────────────────────────────────────────────
echo "1. Health endpoint reachable"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/health" 2>/dev/null || echo "000")
if [ "$HTTP_CODE" = "200" ]; then
    ok "GET /health returned 200"
else
    fail "GET /health returned $HTTP_CODE (is the server running?)"
    echo ""
    echo "=== Results: $PASS passed, $FAIL failed ==="
    exit 1
fi

echo "2. Health response format"
HEALTH=$(curl -sf "$BASE/health")
STATUS=$(echo "$HEALTH" | jq -r '.status' 2>/dev/null || echo "parse_error")
DB=$(echo "$HEALTH" | jq -r '.db' 2>/dev/null || echo "parse_error")
REDIS=$(echo "$HEALTH" | jq -r '.redis' 2>/dev/null || echo "parse_error")
COMMIT=$(echo "$HEALTH" | jq -r '.commit' 2>/dev/null || echo "parse_error")

[ "$STATUS" = "ok" ] && ok "status: ok" || fail "status: $STATUS"
[ "$DB" = "ok" ] && ok "db: ok" || fail "db: $DB"
[ "$REDIS" = "ok" ] && ok "redis: ok" || fail "redis: $REDIS"
[ "$COMMIT" != "parse_error" ] && ok "commit: $COMMIT" || fail "commit field missing"
BUILD_TIME=$(echo "$HEALTH" | jq -r '.build_time' 2>/dev/null || echo "parse_error")
[ "$BUILD_TIME" != "parse_error" ] && ok "build_time: $BUILD_TIME" || fail "build_time field missing"

# ── Auth required ─────────────────────────────────────────────────────────────
echo "3. Auth enforcement"
AUTH_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/v1/hiveshares" 2>/dev/null)
[ "$AUTH_CODE" = "401" ] && ok "GET /hiveshares without auth returns 401" || fail "expected 401, got $AUTH_CODE"

AUTH_CODE2=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/v1/auth/whoami" \
    -H "Authorization: Bearer hvs_invalid" 2>/dev/null)
[ "$AUTH_CODE2" = "401" ] && ok "invalid API key returns 401" || fail "expected 401, got $AUTH_CODE2"

# ── 404 for unknown routes ───────────────────────────────────────────────────
echo "4. Unknown routes"
NOT_FOUND=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/v1/nonexistent" 2>/dev/null)
[ "$NOT_FOUND" = "404" ] || [ "$NOT_FOUND" = "405" ] && ok "unknown route returns $NOT_FOUND" || fail "expected 404/405, got $NOT_FOUND"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
