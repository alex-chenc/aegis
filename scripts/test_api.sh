#!/bin/bash

API_BASE="http://localhost:8080/api/v1"
PASS=0
FAIL=0

echo "========================================"
echo "AI基线检查系统 - API 接口测试"
echo "========================================"
echo ""

test_api() {
    local name="$1"
    local method="$2"
    local endpoint="$3"
    local data="$4"
    
    echo -n "测试 $name... "
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" "$API_BASE$endpoint" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$API_BASE$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" 2>/dev/null)
    fi
    
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        code=$(echo "$body" | grep -o '"code":[0-9]*' | head -1 | cut -d: -f2)
        if [ "$code" = "0" ]; then
            echo "✅ PASS"
            ((PASS++))
        else
            echo "❌ FAIL (code: $code)"
            ((FAIL++))
        fi
    else
        echo "❌ FAIL (HTTP $http_code)"
        ((FAIL++))
    fi
}

echo "=== 配置相关接口 ==="
test_api "获取LLM配置" "GET" "/config/llm"
echo ""

echo "=== 主机相关接口 ==="
test_api "获取主机列表" "GET" "/hosts"
echo ""

echo "=== 模板相关接口 ==="
test_api "获取模板列表" "GET" "/templates"
echo ""

echo "=== 任务相关接口 ==="
test_api "获取任务列表" "GET" "/tasks"
test_api "获取任务列表(带过滤)" "GET" "/tasks?status=success&task_type=check&page=1&page_size=10"
echo ""

echo "=== Agent相关接口 ==="
test_api "获取Agent安装命令" "GET" "/agent/install-command"
echo ""

echo "========================================"
echo "测试结果: 通过 $PASS, 失败 $FAIL"
echo "========================================"

if [ $FAIL -gt 0 ]; then
    exit 1
fi