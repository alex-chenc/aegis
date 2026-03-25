#!/bin/bash

echo "======================================"
echo "  API Test Suite Runner"
echo "======================================"
echo ""

BASE_URL="${BASE_URL:-http://localhost:8080/api/v1}"
echo "Base URL: $BASE_URL"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "======================================"
echo "  Running Positive Tests"
echo "======================================"
bash "$SCRIPT_DIR/positive-tests.sh"
POS_RESULT=$?

echo ""
echo "======================================"
echo "  Running Negative Tests"
echo "======================================"
bash "$SCRIPT_DIR/negative-tests.sh"
NEG_RESULT=$?

echo ""
echo "======================================"
echo "  Final Summary"
echo "======================================"

if [ $POS_RESULT -eq 0 ] && [ $NEG_RESULT -eq 0 ]; then
    echo "✓ All tests passed!"
    exit 0
else
    echo "✗ Some tests failed"
    exit 1
fi