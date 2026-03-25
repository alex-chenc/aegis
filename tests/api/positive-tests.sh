#!/bin/bash

BASE_URL="${BASE_URL:-http://localhost:8080/api/v1}"
PASSED=0
FAILED=0

echo "======================================"
echo "  API Positive Tests"
echo "======================================"
echo ""

test_api() {
    local name="$1"
    local url="$2"
    local expected="$3"
    
    echo -n "Testing: $name ... "
    
    response=$(curl -s -w "\n%{http_code}" "$url" 2>/dev/null)
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')
    
    if [[ "$http_code" == "$expected" ]]; then
        echo "✓ PASSED (HTTP $http_code)"
        ((PASSED++))
    else
        echo "✗ FAILED (Expected HTTP $expected, got HTTP $http_code)"
        ((FAILED++))
    fi
}

test_api_post() {
    local name="$1"
    local url="$2"
    local data="$3"
    local expected="$4"
    
    echo -n "Testing: $name ... "
    
    response=$(curl -s -w "\n%{http_code}" -X POST -H "Content-Type: application/json" -d "$data" "$url" 2>/dev/null)
    http_code=$(echo "$response" | tail -n1)
    
    if [[ "$http_code" == "$expected" ]]; then
        echo "✓ PASSED (HTTP $http_code)"
        ((PASSED++))
    else
        echo "✗ FAILED (Expected HTTP $expected, got HTTP $http_code)"
        ((FAILED++))
    fi
}

echo "--- Health Check ---"
test_api "Health check" "http://localhost:8080/health" "200"

echo ""
echo "--- Config Endpoints ---"
test_api "Get LLM config" "$BASE_URL/config/llm" "200"

echo ""
echo "--- Host Endpoints ---"
test_api "List hosts" "$BASE_URL/hosts" "200"

echo ""
echo "--- Template Endpoints ---"
test_api "List templates" "$BASE_URL/templates" "200"

echo ""
echo "--- Detection Endpoints ---"
test_api "List alerts" "$BASE_URL/detection/alerts" "200"
test_api "List block policies" "$BASE_URL/detection/block-policies" "200"
test_api "List block records" "$BASE_URL/detection/blocks" "200"
test_api "Get attack matrix" "$BASE_URL/detection/attack-matrix" "200"
test_api "List detection rules" "$BASE_URL/detection/rules" "200"
test_api "List tool calls" "$BASE_URL/detection/tool-calls" "200"

echo ""
echo "--- Task Endpoints ---"
test_api "List tasks" "$BASE_URL/tasks" "200"

echo ""
echo "--- Agent Endpoints ---"
test_api "Get install command" "$BASE_URL/agent/install-command" "200"

echo ""
echo "======================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "======================================"

if [ $FAILED -gt 0 ]; then
    exit 1
fi