#!/bin/bash

BASE_URL="${BASE_URL:-http://localhost:8080/api/v1}"
PASSED=0
FAILED=0

echo "======================================"
echo "  API Negative Tests"
echo "======================================"
echo ""

test_negative() {
    local name="$1"
    local url="$2"
    local method="$3"
    local data="$4"
    local expected_codes="$5"
    
    echo -n "Testing: $name ... "
    
    if [ "$method" == "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" "$url" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" -H "Content-Type: application/json" -d "$data" "$url" 2>/dev/null)
    fi
    
    http_code=$(echo "$response" | tail -n1)
    
    if [[ " $expected_codes " =~ " $http_code " ]]; then
        echo "✓ PASSED (HTTP $http_code as expected)"
        ((PASSED++))
    else
        echo "✗ FAILED (Expected [$expected_codes], got HTTP $http_code)"
        ((FAILED++))
    fi
}

echo "--- Invalid Endpoints ---"
test_negative "Non-existent endpoint" "$BASE_URL/nonexistent" "GET" "" "404"
test_negative "Invalid API version" "$BASE_URL/../api/v2/hosts" "GET" "" "404"

echo ""
echo "--- Invalid Methods ---"
test_negative "POST to GET-only endpoint" "$BASE_URL/hosts" "POST" "{}" "405 404"
test_negative "DELETE on read-only endpoint" "$BASE_URL/../health" "DELETE" "" "405 404"

echo ""
echo "--- Invalid Parameters ---"
test_negative "Invalid host ID format" "$BASE_URL/hosts/not-a-uuid" "GET" "" "400 404"
test_negative "Invalid task ID format" "$BASE_URL/tasks/invalid-id" "GET" "" "400 404"
test_negative "Invalid pagination params" "$BASE_URL/tasks?page=-1" "GET" "" "400 200"

echo ""
echo "--- Invalid Request Bodies ---"
test_negative "Invalid JSON for LLM config" "$BASE_URL/config/llm" "POST" "not json" "400"
test_negative "Empty JSON for LLM test" "$BASE_URL/config/llm/test" "POST" "{}" "400 500"
test_negative "Missing required fields" "$BASE_URL/detection/alerts/invalid/resolve" "POST" "{}" "400 404"

echo ""
echo "--- Edge Cases ---"
test_negative "Empty alert ID" "$BASE_URL/detection/alerts/" "GET" "" "404"
test_negative "SQL injection attempt" "$BASE_URL/hosts?id=1';DROP TABLE hosts;--" "GET" "" "400 404 200"

echo ""
echo "======================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "======================================"

if [ $FAILED -gt 0 ]; then
    exit 1
fi