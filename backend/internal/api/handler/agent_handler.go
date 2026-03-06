package handler

import (
	"baseline-system/internal/grpc_server"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AgentHandler Agent Handler
type AgentHandler struct {
	grpcServer *grpc_server.GRPCServer
	serverIP   string
	httpPort   int
	grpcPort   int
}

// NewAgentHandler 创建 Agent Handler
func NewAgentHandler(grpcServer *grpc_server.GRPCServer, serverIP string, httpPort, grpcPort int) *AgentHandler {
	return &AgentHandler{
		grpcServer: grpcServer,
		serverIP:   serverIP,
		httpPort:   httpPort,
		grpcPort:   grpcPort,
	}
}

// GetInstallCommand 获取安装命令
func (h *AgentHandler) GetInstallCommand(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"command":   "curl -sSL http://" + h.serverIP + ":" + string(rune(h.httpPort)) + "/api/v1/agent/install.sh | sudo bash",
			"server_ip": h.serverIP,
			"http_port": h.httpPort,
			"grpc_port": h.grpcPort,
		},
	})
}

// GetInstallScript 获取安装脚本
func (h *AgentHandler) GetInstallScript(c *gin.Context) {
	script := `#!/bin/bash
# Baseline Agent 安装脚本

set -e

SERVER_ADDR="` + h.serverIP + `:8080"
GRPC_ADDR="` + h.serverIP + `:9090"

echo "Downloading agent..."
curl -sSL "http://${SERVER_ADDR}/api/v1/agent/download" -o /usr/local/bin/baseline-agent

echo "Creating configuration..."
cat > /etc/baseline-agent/config.toml <<EOF
server_addr = "${SERVER_ADDR}"
grpc_addr = "${GRPC_ADDR}"
EOF

echo "Starting agent service..."
systemctl daemon-reload
systemctl enable baseline-agent
systemctl start baseline-agent

echo "Agent installed successfully!"
`

	c.Header("Content-Type", "text/x-shellscript")
	c.String(http.StatusOK, script)
}

// DownloadAgent 下载 Agent 二进制
func (h *AgentHandler) DownloadAgent(c *gin.Context) {
	// TODO: 从 MinIO 下载 Agent 二进制
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    501,
		"message": "agent download not implemented yet",
	})
}
