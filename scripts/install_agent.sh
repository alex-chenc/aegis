#!/bin/bash
# Aegis Agent 安装脚本

set -e

INSTALL_DIR="/opt/aegis-agent"
MINIO_URL="${MINIO_URL:-http://localhost:9000}"
AGENT_PACKAGE="agent-artifacts/aegis-agent.tar.gz"

echo "=== Aegis Agent 安装脚本 ==="
echo ""

# 创建安装目录
echo "[1/4] 创建安装目录..."
mkdir -p ${INSTALL_DIR}

# 下载agent包
echo "[2/4] 下载Agent程序..."
curl --noproxy localhost -sSL ${MINIO_URL}/${AGENT_PACKAGE} -o /tmp/aegis-agent.tar.gz

# 解压
echo "[3/4] 解压安装包..."
tar -xzf /tmp/aegis-agent.tar.gz -C ${INSTALL_DIR} --strip-components=1

# 设置权限
echo "[4/4] 设置权限..."
chmod +x ${INSTALL_DIR}/aegis-agent

# 创建必要目录
mkdir -p /etc/aegis-agent/rules
mkdir -p /var/quarantine
mkdir -p /var/lib/aegis-agent

# 清理
rm -f /tmp/aegis-agent.tar.gz

echo ""
echo "=== 安装完成 ==="
echo "安装目录: ${INSTALL_DIR}"
echo ""
echo "目录结构:"
ls -la ${INSTALL_DIR}/
echo ""
echo "运行方式:"
echo "  cd ${INSTALL_DIR} && ./aegis-agent"