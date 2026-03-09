package handler

import (
	"baseline-system/internal/grpc_server"
	"baseline-system/internal/storage"
	"fmt"
	"net/http"
	"strings"
	"time"

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
# Baseline Agent 安装脚本

set -e

SERVER_ADDR="%s:%d"
GRPC_ADDR="%s:%d"

echo "Downloading agent..."
curl -sSL "http://${SERVER_ADDR}/api/v1/agent/download?os=linux&arch=amd64" -o /usr/local/bin/baseline-agent
chmod +x /usr/local/bin/baseline-agent

echo "Creating configuration..."
mkdir -p /etc/baseline-agent
cat > /etc/baseline-agent/config.toml <<EOF
server_addr = "${GRPC_ADDR}"
auth_token = "a_very_secret_agent_token"
host_id = ""
EOF

echo "Creating systemd service..."
cat > /etc/systemd/system/baseline-agent.service <<EOF
[Unit]
Description=Baseline Check Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/baseline-agent --config /etc/baseline-agent/config.toml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

echo "Starting agent service..."
systemctl daemon-reload
systemctl enable baseline-agent
systemctl start baseline-agent

echo "Agent installed successfully!"
`, h.serverIP, h.httpPort, h.serverIP, h.grpcPort)

	c.Header("Content-Type", "text/x-shellscript")
	c.String(http.StatusOK, script)
}

func (h *AgentHandler) DownloadAgent(c *gin.Context) {
	os := c.Query("os")
	arch := c.Query("arch")

	if os == "" || arch == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "os and arch parameters are required",
		})
		return
	}

	objectName := fmt.Sprintf("baseline-agent-linux-%s", arch)

	presignedURL, err := h.minio.GetPresignedURL("agent-artifacts", objectName, time.Hour)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": fmt.Sprintf("agent binary not found: %v", err),
		})
		return
	}

	externalURL := strings.Replace(presignedURL, "http://minio:9000", fmt.Sprintf("http://%s:9000", h.serverIP), 1)

	c.Redirect(http.StatusFound, externalURL)
}
