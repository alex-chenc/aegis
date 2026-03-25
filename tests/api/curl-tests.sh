#!/bin/bash

BASE_URL=${BASE_URL:-http://localhost:8080}
FAILED=0
PASSED=0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=========================================="
echo "Aegis API Test Suite"
echo "=========================================="
echo "Testing against: $BASE_URL"
echo ""

test_endpoint() {
    local name="$1"
    local method="$2"
    local endpoint="$3"
    local expected="$4"
    local data="$5"
    
    echo -n "Testing $name... "
    
    if [ -n "$data" ]; then
        response=$(curl -s -w "%{http_code}" -X $method \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$BASE_URL$endpoint" 2>/dev/null)
    else
        response=$(curl -s -w "%{http_code}" -X $method \
            "$BASE_URL$endpoint" 2>/dev/null)
    fi
    
    http_code=${response: -3}
    body=${response%???}
    
    if [ "$http_code" = "$expected" ]; then
        echo -e "${GREEN}PASS${NC} ($http_code)"
        ((PASSED++))
    else
        echo -e "${RED}FAIL${NC} (expected $expected, got $http_code)"
        echo "  Response: $body"
        ((FAILED++))
    fi
}

echo "=== Health Check ==="
test_endpoint "Health Check" GET "/health" 200

echo ""
echo "=== Hosts API ==="
test_endpoint "List Hosts" GET "/api/v1/hosts" 200
test_endpoint "Get Host - Invalid ID" GET "/api/v1/hosts/invalid-uuid" 400

echo ""
echo "=== Detection API ==="
test_endpoint "List Alerts" GET "/api/v1/detection/alerts" 200
test_endpoint "List Alerts with Filter" GET "/api/v1/detection/alerts?severity=high" 200
test_endpoint "Get Alert - Invalid ID" GET "/api/v1/detection/alerts/invalid" 404
test_endpoint "List Block Policies" GET "/api/v1/detection/block-policies" 200
test_endpoint "List Detection Rules" GET "/api/v1/detection/rules" 200
test_endpoint "Get Attack Matrix" GET "/api/v1/detection/attack-matrix" 200
test_endpoint "Get Threat Statistics" GET "/api/v1/detection/statistics/threats" 200
test_endpoint "Get Alert Trend" GET "/api/v1/detection/statistics/alert-trend" 200

echo ""
echo "=== Agent API ==="
test_endpoint "Get Install Command" GET "/api/v1/agent/install-command" 200
test_endpoint "Get Install Script" GET "/api/v1/agent/install.sh" 200
test_endpoint "Get Uninstall Script" GET "/api/v1/agent/uninstall.sh" 200

echo ""
echo "=== Config API ==="
test_endpoint "Get LLM Config" GET "/api/v1/config/llm" 200

echo ""
echo "=== Templates API ==="
test_endpoint "List Templates" GET "/api/v1/templates" 200

echo ""
echo "=== Negative Tests ==="
test_endpoint "Invalid Endpoint" GET "/api/v1/nonexistent" 404

echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "Passed: ${GREEN}$PASSED${NC}"
echo -e "Failed: ${RED}$FAILED${NC}"
echo ""

if [ $FAILED -gt 0 ]; then
    exit 1
fi

echo "All tests passed!"