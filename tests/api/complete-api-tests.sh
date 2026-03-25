#!/bin/bash

BASE_URL=${BASE_URL:-http://localhost:8080}
FAILED=0
PASSED=0

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

echo "=========================================="
echo "Aegis Complete API Test Suite"
echo "=========================================="
echo "Testing against: $BASE_URL"
echo ""

test_api() {
    local name="$1"
    local method="$2"
    local endpoint="$3"
    local expected="$4"
    local data="$5"
    local check_field="$6"
    
    echo -n "Testing $name... "
    
    local response
    if [ -n "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X $method \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$BASE_URL$endpoint" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X $method \
            "$BASE_URL$endpoint" 2>/dev/null)
    fi
    
    local http_code=$(echo "$response" | tail -1)
    local body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "$expected" ]; then
        if [ -n "$check_field" ]; then
            local field_value=$(echo "$body" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data', {}).get('$check_field', 'NOT_FOUND'))" 2>/dev/null)
            if [ "$field_value" = "NOT_FOUND" ]; then
                echo -e "${RED}FAIL${NC} (field '$check_field' not found)"
                ((FAILED++))
                return
            fi
        fi
        echo -e "${GREEN}PASS${NC} ($http_code)"
        ((PASSED++))
    else
        echo -e "${RED}FAIL${NC} (expected $expected, got $http_code)"
        echo "  Response: $body"
        ((FAILED++))
    fi
}

echo "=== 1. Health Check ==="
test_api "Health Check" GET "/health" 200

echo ""
echo "=== 2. Hosts API ==="
test_api "List Hosts" GET "/api/v1/hosts" 200
test_api "Get Host - Invalid ID" GET "/api/v1/hosts/invalid-uuid" 400

echo ""
echo "=== 3. Detection API ==="
test_api "List Alerts" GET "/api/v1/detection/alerts" 200
test_api "List Alerts with Severity" GET "/api/v1/detection/alerts?severity=critical" 200
test_api "List Alerts with Status" GET "/api/v1/detection/alerts?status=pending" 200
test_api "Get Alert - Invalid ID" GET "/api/v1/detection/alerts/invalid" 404
test_api "List Block Policies" GET "/api/v1/detection/block-policies" 200
test_api "List Detection Rules" GET "/api/v1/detection/rules" 200
test_api "Get Attack Matrix" GET "/api/v1/detection/attack-matrix" 200
test_api "Get Threat Statistics" GET "/api/v1/detection/statistics/threats" 200
test_api "Get Alert Trend" GET "/api/v1/detection/statistics/alert-trend" 200
test_api "List Block Records" GET "/api/v1/detection/blocks" 200

echo ""
echo "=== 4. Alert Operations ==="
ALERT_ID=$(curl -s "$BASE_URL/api/v1/detection/alerts?severity=high" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('data',[{}])[0].get('alert_id',''))" 2>/dev/null)
if [ -n "$ALERT_ID" ]; then
    echo "Using Alert ID: $ALERT_ID"
    test_api "Get Alert Detail" GET "/api/v1/detection/alerts/$ALERT_ID" 200
    test_api "Resolve Alert" POST "/api/v1/detection/alerts/$ALERT_ID/resolve" 200
else
    echo "No alerts found for testing"
fi

echo ""
echo "=== 5. Batch Delete Alerts ==="
ALERT_IDS=$(curl -s "$BASE_URL/api/v1/detection/alerts?severity=medium" | python3 -c "import json,sys; d=json.load(sys.stdin); alerts=d.get('data',{}).get('data',[]); print(','.join([a['alert_id'] for a in alerts[:2]]))" 2>/dev/null)
if [ -n "$ALERT_IDS" ]; then
    echo "Deleting: $ALERT_IDS"
    test_api "Batch Delete Alerts" DELETE "/api/v1/detection/alerts" 200 "{\"alert_ids\": [\"$(echo $ALERT_IDS | sed 's/,/","/g')\"]}"
else
    echo "No alerts for batch delete test"
fi

echo ""
echo "=== 6. Agent API ==="
test_api "Get Install Command" GET "/api/v1/agent/install-command" 200
test_api "Get Install Script" GET "/api/v1/agent/install.sh" 200
test_api "Get Uninstall Script" GET "/api/v1/agent/uninstall.sh" 200

echo ""
echo "=== 7. Config API ==="
test_api "Get LLM Config" GET "/api/v1/config/llm" 200

echo ""
echo "=== 8. Templates API ==="
test_api "List Templates" GET "/api/v1/templates" 200

echo ""
echo "=== 9. Rule Status Update ==="
RULE_ID=$(curl -s "$BASE_URL/api/v1/detection/rules?status=active" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('data',[{}])[0].get('rule_id',''))" 2>/dev/null)
if [ -n "$RULE_ID" ]; then
    echo "Using Rule ID: $RULE_ID"
    test_api "Disable Rule" PUT "/api/v1/detection/rules/$RULE_ID/status" 200 '{"status":"disabled"}'
    test_api "Enable Rule" PUT "/api/v1/detection/rules/$RULE_ID/status" 200 '{"status":"active"}'
else
    echo "No rules found for testing"
fi

echo ""
echo "=== 10. Block Policy Update ==="
test_api "Update Block Policy" PUT "/api/v1/detection/block-policies/t1059.004" 200 '{"enabled":true,"auto_block":false}'

echo ""
echo "=== 11. Negative Tests ==="
test_api "Invalid Endpoint" GET "/api/v1/nonexistent" 404
test_api "Invalid Method" POST "/api/v1/health" 404

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