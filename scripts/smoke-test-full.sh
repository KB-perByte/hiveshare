#!/usr/bin/env bash
set -euo pipefail

# Smoke test harness — runs all smoke-test-*.sh scripts in order.
# Usage: ./scripts/smoke-test-full.sh [base_url]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
URL="${1:-${HIVESHARE_TEST_URL:-http://localhost:8080}}"
TOTAL_PASS=0
TOTAL_FAIL=0
FAILED_SUITES=""

run_suite() {
    local script="$1"
    local name
    name=$(basename "$script" .sh | sed 's/smoke-test-//')
    echo ""
    echo "================================================================"
    echo "  Running: $name"
    echo "================================================================"
    if "$script" "$URL"; then
        TOTAL_PASS=$((TOTAL_PASS+1))
    else
        TOTAL_FAIL=$((TOTAL_FAIL+1))
        FAILED_SUITES="$FAILED_SUITES $name"
    fi
}

# Connectivity first — bail if server is unreachable
run_suite "$SCRIPT_DIR/smoke-test.sh"
if [ "$TOTAL_FAIL" -gt 0 ]; then
    echo ""
    echo "=== Connectivity failed — skipping remaining suites ==="
    exit 1
fi

# Run all smoke-test-*.sh scripts except the harness itself and the base connectivity test
for script in "$SCRIPT_DIR"/smoke-test-*.sh; do
    base=$(basename "$script")
    case "$base" in
        smoke-test-full.sh|smoke-test.sh) continue ;;
    esac
    run_suite "$script"
done

echo ""
echo "================================================================"
echo "  Final: $TOTAL_PASS suites passed, $TOTAL_FAIL failed"
if [ -n "$FAILED_SUITES" ]; then
    echo "  Failed:$FAILED_SUITES"
fi
echo "================================================================"
[ "$TOTAL_FAIL" -eq 0 ] && exit 0 || exit 1
