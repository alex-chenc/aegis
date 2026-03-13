package handler

import (
	"aegis-system/internal/grpc_server"
	"aegis-system/internal/storage"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	grpcServer *grpc_server.GRPCServer
	minio      *storage.MinIOClient
	serverIP   string
	httpPort   int
	grpcPort   int
}

func NewAgentHandler(grpcServer *grpc_server.GRPCServer, minio *storage.MinIOClient, serverIP string, httpPort, grpcPort int) *AgentHandler {
	return &AgentHandler{
		grpcServer: grpcServer,
		minio:      minio,
		serverIP:   serverIP,
		httpPort:   httpPort,
		grpcPort:   grpcPort,
	}
}

func (h *AgentHandler) GetInstallCommand(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"command":   fmt.Sprintf("curl -sSL http://%s:%d/api/v1/agent/install.sh | sudo bash", h.serverIP, h.httpPort),
			"server_ip": h.serverIP,
			"http_port": h.httpPort,
			"grpc_port": h.grpcPort,
		},
	})
}

func (h *AgentHandler) GetInstallScript(c *gin.Context) {
	script := fmt.Sprintf(`#!/bin/bash
# Aegis Agent 安装脚本
# Agent 将安装在 /opt/aegis-agent/ 目录
# 日志文件位于 /opt/aegis-agent/logs/agent.log

set -e

SERVER_ADDR="%s:%d"
GRPC_ADDR="%s:%d"
INSTALL_DIR="/opt/aegis-agent"
LOG_DIR="/opt/aegis-agent/logs"

echo "=== Aegis Agent 安装脚本 ==="
echo "安装目录：${INSTALL_DIR}"
echo "日志目录：${LOG_DIR}"
echo "服务器地址：${SERVER_ADDR}"
echo ""

# 创建安装和日志目录
echo "[1/5] 创建目录..."
mkdir -p ${INSTALL_DIR}
mkdir -p ${LOG_DIR}
chmod 755 ${INSTALL_DIR}
chmod 755 ${LOG_DIR}

# 下载 Agent 二进制
echo "[2/5] 下载 Agent 二进制..."
curl -sSL "http://${SERVER_ADDR}/api/v1/agent/download?os=linux&arch=amd64" -o ${INSTALL_DIR}/aegis-agent
chmod +x ${INSTALL_DIR}/aegis-agent

# 创建配置文件
echo "[3/5] 创建配置文件..."
mkdir -p /etc/aegis-agent
cat > /etc/aegis-agent/config.toml <<EOF
server_addr = "${GRPC_ADDR}"
auth_token = "a_very_secret_agent_token"
host_id = ""
EOF

# 创建 systemd 服务
echo "[4/5] 创建 systemd 服务..."
cat > /etc/systemd/system/aegis-agent.service <<EOF
[Unit]
Description=Aegis Check Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/aegis-agent
Restart=always
RestartSec=10
StandardOutput=append:${LOG_DIR}/agent.log
StandardError=append:${LOG_DIR}/agent.log
Environment="TZ=Asia/Shanghai"

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
echo "[5/5] 启动 Agent 服务..."
systemctl daemon-reload
systemctl enable aegis-agent
systemctl start aegis-agent
systemctl status aegis-agent --no-pager

echo ""
echo "=== 安装完成 ==="
echo "Agent 状态：systemctl status aegis-agent"
echo "Agent 日志：journalctl -u aegis-agent -f"
echo "日志文件：${LOG_DIR}/agent.log"
`, h.serverIP, h.httpPort, h.serverIP, h.grpcPort)

	c.Header("Content-Type", "text/x-shellscript")
	c.String(http.StatusOK, script)
}

func (h *AgentHandler) DownloadAgent(c *gin.Context) {
	osParam := c.Query("os")
	arch := c.Query("arch")

	if osParam == "" || arch == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "os and arch parameters are required",
		})
		return
	}

	objectName := fmt.Sprintf("aegis-agent-linux-%s", arch)

	reader, err := h.minio.DownloadFile("agent-artifacts", objectName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": fmt.Sprintf("agent binary not found: %v", err),
		})
		return
	}
	defer reader.Close()

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", objectName))
	c.Header("Content-Transfer-Encoding", "binary")

	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error streaming agent binary: %v\n", err)
	}
}
