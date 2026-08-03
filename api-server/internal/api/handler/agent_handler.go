package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/storage"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	serverClient *grpcclient.ServerClient
	minio        *storage.MinIOClient
	serverIP     string
	httpPort     int
	grpcPort     int
}

func NewAgentHandler(serverClient *grpcclient.ServerClient, minio *storage.MinIOClient, serverIP string, httpPort, grpcPort int) *AgentHandler {
	return &AgentHandler{
		serverClient: serverClient,
		minio:        minio,
		serverIP:     serverIP,
		httpPort:     httpPort,
		grpcPort:     grpcPort,
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

echo "=== Aegis Agent 安装脚本 ==="
echo "安装目录：${INSTALL_DIR}"
echo "服务器地址：${SERVER_ADDR}"
echo ""

# 检查是否以root权限运行
if [ "$EUID" -ne 0 ]; then
    echo "请使用 sudo 运行此脚本"
    exit 1
fi

# 检查是否已安装
if [ -f ${INSTALL_DIR}/aegis-agent ]; then
    echo "检测到已安装的Agent，正在停止并卸载..."
    systemctl stop aegis-agent 2>/dev/null || true
    systemctl disable aegis-agent 2>/dev/null || true
    rm -f /etc/systemd/system/aegis-agent.service
    systemctl daemon-reload
fi

# 创建所有目录
echo "[1/6] 创建目录..."
mkdir -p ${INSTALL_DIR}/logs
mkdir -p ${INSTALL_DIR}/bpf
mkdir -p ${INSTALL_DIR}/quarantine
mkdir -p ${INSTALL_DIR}/rules
mkdir -p ${INSTALL_DIR}/config
chmod 755 ${INSTALL_DIR}
chmod 755 ${INSTALL_DIR}/logs
chmod 700 ${INSTALL_DIR}/quarantine

# 下载 Agent 安装包
echo "[2/6] 下载 Agent..."
cd /tmp
curl -sSL "http://${SERVER_ADDR}/api/v1/agent/download?os=linux&arch=amd64" -o aegis-agent.tar.gz
if [ ! -f aegis-agent.tar.gz ]; then
    echo "下载失败！"
    exit 1
fi
tar -xzf aegis-agent.tar.gz -C ${INSTALL_DIR}
rm -f /tmp/aegis-agent.tar.gz

# 重命名二进制文件
if [ -f ${INSTALL_DIR}/aegis-agent-linux-amd64 ]; then
    mv ${INSTALL_DIR}/aegis-agent-linux-amd64 ${INSTALL_DIR}/aegis-agent
elif [ -f ${INSTALL_DIR}/aegis-agent-linux-arm64 ]; then
    mv ${INSTALL_DIR}/aegis-agent-linux-arm64 ${INSTALL_DIR}/aegis-agent
fi

if [ ! -f ${INSTALL_DIR}/aegis-agent ]; then
    echo "解压失败，未找到 aegis-agent 文件！"
    echo "目录内容："
    ls -la ${INSTALL_DIR}/
    exit 1
fi
chmod +x ${INSTALL_DIR}/aegis-agent

# 创建配置文件
echo "[3/6] 创建配置文件..."
mkdir -p /etc/aegis-agent
cat > /etc/aegis-agent/config.toml <<EOF
ServerAddr = "${GRPC_ADDR}"
AuthToken = "a_very_secret_agent_token"
HostID = ""
AgentGuardEnabled = true
AgentGuardBehaviorMonitorEnabled = true
AgentGuardToolAdapterEnabled = false
AgentGuardToolSourceManifest = ""
AgentGuardToolHookSocket = ""
AgentGuardEnforcementEnabled = false
AgentGuardFreezeEnabled = false
AgentGuardStateDir = "/var/lib/aegis/agent-guard"
AgentGuardSpoolCapacity = 4096
AgentGuardReconcileSeconds = 30
EOF
chmod 600 /etc/aegis-agent/config.toml
# 安装目录只保留兼容入口，避免修改未生效的配置副本。
ln -sfn /etc/aegis-agent/config.toml ${INSTALL_DIR}/config/config.toml

# 创建卸载脚本
echo "[4/6] 创建卸载脚本..."
cat > ${INSTALL_DIR}/uninstall.sh <<'UNINSTALL'
#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "请使用 sudo 运行此脚本"
    exit 1
fi

echo "=== Aegis Agent 卸载脚本 ==="
echo "[1/4] 停止 Agent 服务..."
systemctl stop aegis-agent 2>/dev/null || echo "服务未运行"
systemctl disable aegis-agent 2>/dev/null || echo "服务未启用"

echo "[2/4] 删除 systemd 服务..."
rm -f /etc/systemd/system/aegis-agent.service
systemctl daemon-reload

echo "[3/4] 删除安装目录..."
rm -rf /opt/aegis-agent

echo "[4/4] 删除配置目录..."
rm -rf /etc/aegis-agent

echo ""
echo "=== 卸载完成 ==="
echo "Aegis Agent 已完全卸载"
UNINSTALL
chmod +x ${INSTALL_DIR}/uninstall.sh

# 创建 systemd 服务
echo "[5/6] 创建 systemd 服务..."
cat > /etc/systemd/system/aegis-agent.service <<EOF
[Unit]
Description=Aegis Check Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/aegis-agent
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=10
Environment="TZ=Asia/Shanghai"

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
echo "[6/6] 启动 Agent 服务..."
systemctl daemon-reload
systemctl enable aegis-agent
systemctl start aegis-agent
sleep 2
systemctl status aegis-agent --no-pager

echo ""
echo "=== 安装完成 ==="
echo "Agent 状态：systemctl status aegis-agent"
echo "Agent 日志：tail -f ${INSTALL_DIR}/logs/agent.log"
echo "卸载命令：sudo ${INSTALL_DIR}/uninstall.sh"
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

	// Download the tar.gz package which contains binary and BPF files
	objectName := "aegis-agent.tar.gz"

	reader, err := h.minio.DownloadFile("agent-artifacts", objectName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": fmt.Sprintf("agent package not found: %v", err),
		})
		return
	}
	defer reader.Close()

	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", objectName))
	c.Header("Content-Transfer-Encoding", "binary")

	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error streaming agent package: %v\n", err)
	}
}

func (h *AgentHandler) GetUninstallScript(c *gin.Context) {
	script := `#!/bin/bash
# Aegis Agent 卸载脚本

set -e

echo "=== Aegis Agent 卸载脚本 ==="

# 检查是否以root权限运行
if [ "$EUID" -ne 0 ]; then
    echo "请使用 sudo 运行此脚本"
    exit 1
fi

# 停止并禁用服务
echo "[1/5] 停止 Agent 服务..."
systemctl stop aegis-agent 2>/dev/null || echo "服务未运行"
systemctl disable aegis-agent 2>/dev/null || echo "服务未启用"

# 删除 systemd 服务
echo "[2/5] 删除 systemd 服务..."
rm -f /etc/systemd/system/aegis-agent.service
systemctl daemon-reload

# 删除安装目录
echo "[3/5] 删除安装目录..."
rm -rf /opt/aegis-agent

# 删除配置目录
echo "[4/5] 删除配置目录..."
rm -rf /etc/aegis-agent

# 询问是否删除隔离文件
read -p "是否删除隔离目录 /var/quarantine 中的文件？(y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /var/quarantine
    echo "隔离目录已删除"
else
    echo "隔离目录保留"
fi

# 删除数据目录
echo "[5/5] 删除数据目录..."
rm -rf /var/lib/aegis-agent

echo ""
echo "=== 卸载完成 ==="
echo "Aegis Agent 已完全卸载"
`

	c.Header("Content-Type", "text/x-shellscript")
	c.String(http.StatusOK, script)
}
