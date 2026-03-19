#!/bin/bash
BASE_URL="http://localhost:8080/api/v1"
AGENT_TOKEN="a_shared_secret_token_for_agents"

echo "=========================================="
echo "测试自定义CVE功能 API接口"
echo "=========================================="

# 测试1: 启动自定义CVE查询（无认证，测试基本响应）
echo ""
echo "--- 测试1: POST /vulnerability/custom-query ---"
echo "请求: CVE-2021-44228"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/vulnerability/custom-query" \
  -H "Content-Type: application/json" \
  -d '{"cve_id": "CVE-2021-44228"}')
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | head -1)
echo "HTTP状态码: $HTTP_CODE"
echo "响应: $BODY" | jq .

# 测试2: 获取当前查询状态
echo ""
echo "--- 测试2: GET /vulnerability/custom-query/current ---"
RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/vulnerability/custom-query/current")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | head -1)
echo "HTTP状态码: $HTTP_CODE"
echo "响应: $BODY" | jq .

# 测试3: 测试无效CVE格式
echo ""
echo "--- 测试3: POST /vulnerability/custom-query (无效CVE格式) ---"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/vulnerability/custom-query" \
  -H "Content-Type: application/json" \
  -d '{"cve_id": "INVALID-CVE-FORMAT"}')
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | head -1)
echo "HTTP状态码: $HTTP_CODE"
echo "响应: $BODY" | jq .

# 测试4: 生成主机脚本（需要CVE和主机ID）
echo ""
echo "--- 测试4: POST /vulnerability/:cve_id/scripts/generate ---"
CVE_ID="CVE-2021-44228"
HOST_ID=$(docker exec aegis-postgres psql -U aegis_user -d aegis_db -t -c "SELECT id FROM hosts LIMIT 1;" | tr -d ' ')
if [ -z "$HOST_ID" ]; then
    echo "跳过: 没有主机数据"
else
    echo "请求: CVE=$CVE_ID, Host=$HOST_ID, Type=poc"
    RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/vulnerability/$CVE_ID/scripts/generate" \
      -H "Content-Type: application/json" \
      -d "{\"host_ids\": [\"$HOST_ID\"], \"script_type\": \"poc\"}")
    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | head -1)
    echo "HTTP状态码: $HTTP_CODE"
    echo "响应: $BODY" | jq .
fi

# 测试5: 获取主机脚本状态
echo ""
echo "--- 测试5: GET /vulnerability/:cve_id/host-scripts ---"
if [ -z "$HOST_ID" ]; then
    echo "跳过: 没有主机数据"
else
    RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/vulnerability/$CVE_ID/host-scripts?script_type=poc")
    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | head -1)
    echo "HTTP状态码: $HTTP_CODE"
    echo "响应: $BODY" | jq .
fi

# 测试6: 获取漏洞列表（验证自定义CVE是否显示）
echo ""
echo "--- 测试6: GET /vulnerability (漏洞列表) ---"
RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/vulnerability?page=1&page_size=10")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | head -1)
echo "HTTP状态码: $HTTP_CODE"
echo "响应: $BODY" | jq .

echo ""
echo "=========================================="
echo "测试完成!"
echo "=========================================="
