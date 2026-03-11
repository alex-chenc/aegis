#!/bin/bash
# API Endpoint Testing Script
# Tests all REST API endpoints

BASE_URL="${BASE_URL:-http://localhost:8080}"
PASS=0
FAIL=0

echo "========================================="
echo "API Endpoint Testing"
echo "Base URL: $BASE_URL"
echo "========================================="

# Helper function
test_endpoint() {
    local method=$1
    local endpoint=$2
    local description=$3
    local expected_code=${4:-200}
    local data=${5:-'{}'}
    
    echo -n "Testing $description... "
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$BASE_URL$endpoint" 2>/dev/null || echo "000")
    else
        response=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL$endpoint" -H "Content-Type: application/json" -d "$data" 2>/dev/null || echo "000")
    fi
    
    if [ "$response" = "$expected_code" ]; then
        echo "✅ PASS (HTTP $response)"
        PASS=$((PASS+1))
    else
        echo "❌ FAIL (Expected HTTP $expected_code, got HTTP $response)"
        FAIL=$((FAIL+1))
    fi
}

# Health check
test_endpoint "GET" "/health" "Health Check"

# Config endpoints
test_endpoint "GET" "/api/v1/config/llm" "Get LLM Config"
test_endpoint "POST" "/api/v1/config/llm/test" "Test LLM Connection" 400

# Hosts endpoints
test_endpoint "GET" "/api/v1/hosts" "List Hosts"

# Templates endpoints
test_endpoint "GET" "/api/v1/templates" "List Templates"

# Agent endpoints
test_endpoint "GET" "/api/v1/agent/install-command" "Get Install Command"

echo ""
echo "========================================="
echo "Results: $PASS passed, $FAIL failed"
echo "========================================="

if [ $FAIL -gt 0 ]; then
    exit 1
fi
