# 通讯层设计文档 - V2.0 完整版

**版本**: 2.0
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 2.0 | 2026-03-05 | Manus AI | **全面更新**。在 V1.6 基础上补充模板解析状态查询 API、脚本生成状态 API、修复任务下发 API、自愈流程状态 API、模板删除 API，完善所有接口的请求/响应 JSON Schema，补充错误码定义。 |
| 1.6 | 2026-03-05 | Manus AI | 完整重写，包含 gRPC Protobuf 定义和 RESTful API 详细 JSON Schema。 |

## 2. 概述

本文档定义了系统内所有组件间的通讯协议。V2.0 在 V1.6 的基础上，补充了后端详细设计中涉及的所有新增 API 接口，使通讯层设计与后端业务逻辑完全对齐。

## 3. gRPC 通讯协议 (后端 <-> Agent)

gRPC 通讯协议与 V1.6 完全一致，不做修改。完整的 Protobuf 定义请参见 V1.6 通讯层设计文档第 3 节。

## 4. HTTP/RESTful API (前端 <-> 后端)

### 4.1 通用约定

所有 API 遵循以下通用约定。

**Base URL**: `/api/v1`

**请求头**: `Content-Type: application/json`（文件上传除外）

**成功响应**: `200 OK`、`201 Created` 或 `202 Accepted`

**错误响应格式**:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "人类可读的错误描述信息"
  }
}
```

### 4.2 错误码定义

V2.0 新增了完整的错误码定义，便于前端进行精确的错误处理。

| 错误码 | HTTP 状态码 | 描述 |
|:---|:---|:---|
| `VALIDATION_ERROR` | 400 | 请求参数校验失败 |
| `UNAUTHORIZED` | 401 | 认证失败 |
| `NOT_FOUND` | 404 | 请求的资源不存在 |
| `FILE_TOO_LARGE` | 413 | 上传文件超过大小限制 |
| `UNSUPPORTED_FILE_TYPE` | 415 | 不支持的文件类型 |
| `LLM_CONFIG_NOT_SET` | 422 | LLM 配置未设置，无法执行需要 LLM 的操作 |
| `LLM_CONNECTION_FAILED` | 502 | LLM 服务连接失败 |
| `LLM_PARSE_FAILED` | 502 | LLM 返回结果解析失败 |
| `SCRIPT_NOT_READY` | 422 | 脚本尚未生成完成 |
| `HOST_OFFLINE` | 422 | 目标主机离线 |
| `TEMPLATE_PARSE_FAILED` | 500 | 模板解析失败 |
| `INTERNAL_ERROR` | 500 | 服务器内部错误 |

### 4.3 API 接口详述

#### 4.3.1 系统配置 (Settings)

**`GET /api/v1/config/llm`**

获取当前生效的大模型配置。API Key 以脱敏形式返回。

响应体 (200 OK):
```json
{
  "api_key": "sk-xxxx...1234",
  "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
  "model_name": "qwen-plus",
  "last_test_status": "success",
  "last_test_at": "2026-03-05T14:00:00Z"
}
```

**`POST /api/v1/config/llm`**

更新大模型配置。仅在连通性测试通过后才允许保存。

请求体:
```json
{
  "api_key": "sk-real-api-key",
  "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
  "model_name": "qwen-plus"
}
```

响应体 (200 OK): 同 `GET` 响应体格式，API Key 脱敏。

**`POST /api/v1/config/llm/test`**

测试大模型配置的连通性。执行三层校验（格式校验、网络连通性校验、模型可用性校验）。

请求体:
```json
{
  "api_key": "sk-real-api-key",
  "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
  "model_name": "qwen-plus"
}
```

响应体 (200 OK):
```json
{
  "status": "ok",
  "message": "连接成功，模型可用",
  "details": {
    "format_check": "passed",
    "connectivity_check": "passed",
    "model_check": "passed"
  }
}
```

响应体 (200 OK, 测试失败):
```json
{
  "status": "failed",
  "message": "认证失败，请检查 API Key",
  "details": {
    "format_check": "passed",
    "connectivity_check": "failed",
    "model_check": "skipped"
  }
}
```

#### 4.3.2 Agent 安装与分发

**`GET /api/v1/agent/install-command`**

获取 Agent 一键安装命令（用于前端展示）。后端在启动时自动检测自身对外可达的 IP 地址（优先公网 IP），并将其填入到安装命令中。前端用户可以直接复制此命令并在目标主机上粘贴执行，无需手动修改 IP 地址。

响应体 (200 OK):
```json
{
  "command": "curl -sSL http://203.0.113.10:8080/api/v1/agent/install.sh | sudo bash",
  "server_ip": "203.0.113.10",
  "http_port": 8080,
  "grpc_port": 9090
}
```

字段说明：
- `command`：完整的一键安装命令，前端可以直接使用此命令。
- `server_ip`：后端检测到的对外可达 IP 地址（优先公网）。
- `http_port`：后端 HTTP API 服务端口。
- `grpc_port`：后端 gRPC 服务端口（Agent 需要此端口连接）。

**`GET /api/v1/agent/install.sh`**

动态生成并返回一键安装脚本。Content-Type 为 `text/plain`。脚本中的后端地址和 gRPC 端口均由后端自动填充，使脚本可以直接执行不需手动修改。

响应示例（仅月核心部分）：
```bash
#!/bin/bash
set -e

SERVER_ADDR="203.0.113.10:8080"
GRPC_ADDR="203.0.113.10:9090"
INSTALL_DIR="/opt/baseline-agent"
SERVICE_NAME="baseline-agent"

# 检测系统架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH_SUFFIX="amd64" ;;
    aarch64) ARCH_SUFFIX="arm64" ;;
    *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac

DOWNLOAD_URL="http://$SERVER_ADDR/api/v1/agent/download?os=linux&arch=$ARCH_SUFFIX"

echo "[INFO] 正在从 $SERVER_ADDR 下载 Agent..."
mkdir -p $INSTALL_DIR
curl -sSL -o $INSTALL_DIR/baseline-agent "$DOWNLOAD_URL"
chmod +x $INSTALL_DIR/baseline-agent

echo "[INFO] 正在创建 systemd 服务..."
cat > /etc/systemd/system/$SERVICE_NAME.service <<EOF
[Unit]
Description=Baseline Check Agent
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/baseline-agent --server $GRPC_ADDR
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable $SERVICE_NAME
systemctl start $SERVICE_NAME

echo "[INFO] Agent 安装完成！服务已启动。"
systemctl status $SERVICE_NAME
```

**`GET /api/v1/agent/download`**

获取 Agent 二进制文件的预签名下载链接。

Query Parameters: `os` (string, required), `arch` (string, required)

响应体 (200 OK):
```json
{
  "download_url": "http://minio:9000/agent-artifacts/baseline-agent-linux-amd64?X-Amz-Algorithm=..."
}
```

#### 4.3.3 资产管理 (Hosts)

**`GET /api/v1/hosts`**

获取主机列表，支持搜索和分页。在线状态通过 Redis 心跳 Key 判断。

Query Parameters: `page` (int, default 1), `pageSize` (int, default 20), `query` (string, optional)

响应体 (200 OK):
```json
{
  "items": [
    {
      "id": "uuid-1",
      "status": "online",
      "ip_address": "192.168.1.10",
      "hostname": "web-server-01",
      "os_type": "linux",
      "agent_version": "v2.0.0",
      "last_heartbeat_at": "2026-03-05T14:30:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

#### 4.3.4 模板与规则 (Templates & Rules)

**`POST /api/v1/templates/upload`**

上传基线模板文件。支持 PDF、DOCX、YAML、XLSX、TXT 格式，最大 50MB。

请求体: `Content-Type: multipart/form-data`，字段名 `file`。

响应体 (201 Created):
```json
{
  "id": "uuid-template-1",
  "name": "CIS_Ubuntu_Linux_22.04_LTS_Benchmark_v1.0.0.pdf",
  "file_type": "PDF",
  "status": "parsing",
  "created_at": "2026-03-05T14:00:00Z"
}
```

**`GET /api/v1/templates`**

获取所有已上传的模板列表。

响应体 (200 OK):
```json
{
  "items": [
    {
      "id": "uuid-template-1",
      "name": "CIS_Ubuntu_Linux_22.04_LTS_Benchmark_v1.0.0.pdf",
      "file_type": "PDF",
      "status": "completed",
      "rule_count": 45,
      "created_at": "2026-03-05T14:00:00Z"
    }
  ]
}
```

**`GET /api/v1/templates/{id}/status`** — V2.0 新增

获取模板解析的实时状态（从 Redis 读取）。前端轮询此接口获取解析进度。

响应体 (200 OK):
```json
{
  "template_id": "uuid-template-1",
  "status": "parsing",
  "progress": 65,
  "message": "正在调用 AI 解析规则..."
}
```

响应体 (200 OK, 解析完成):
```json
{
  "template_id": "uuid-template-1",
  "status": "completed",
  "progress": 100,
  "message": "解析完成，共提取 45 条规则",
  "rule_count": 45
}
```

**`GET /api/v1/templates/{id}/rules`**

获取指定模板解析出的所有规则。

响应体 (200 OK):
```json
{
  "items": [
    {
      "id": "uuid-rule-1",
      "title": "确保密码创建要求已配置",
      "check_content": "检查 /etc/security/pwquality.conf 文件中 minlen 参数是否 >= 14...",
      "fix_content": "编辑 /etc/security/pwquality.conf 文件，设置 minlen = 14...",
      "script_status": "ready",
      "check_script_version": 1,
      "fix_script_version": 1
    }
  ]
}
```

**`DELETE /api/v1/templates/{id}`** — V2.0 新增

删除指定模板及其关联的所有规则和脚本。

响应体 (200 OK):
```json
{
  "message": "模板及关联数据已删除"
}
```

#### 4.3.5 脚本管理 — V2.0 新增

**`GET /api/v1/rules/{id}/scripts`**

获取指定规则的检查脚本和修复脚本内容。

响应体 (200 OK):
```json
{
  "rule_id": "uuid-rule-1",
  "check_script": {
    "content": "#!/bin/bash\nset -e\n...",
    "version": 2,
    "status": "ready",
    "last_updated": "2026-03-05T14:30:00Z"
  },
  "fix_script": {
    "content": "#!/bin/bash\nset -e\n...",
    "version": 1,
    "status": "ready",
    "last_updated": "2026-03-05T14:00:00Z"
  }
}
```

**`POST /api/v1/rules/{id}/scripts/regenerate`** — V2.0 新增

手动触发重新生成指定规则的脚本（当自动生成的脚本不满意时使用）。

请求体:
```json
{
  "script_type": "CHECK"
}
```

响应体 (202 Accepted):
```json
{
  "message": "脚本重新生成任务已提交",
  "rule_id": "uuid-rule-1",
  "script_type": "CHECK"
}
```

**`GET /api/v1/rules/{id}/scripts/versions`** — V2.0 新增

获取指定规则的脚本版本历史。

Query Parameters: `script_type` (string, required, "CHECK" 或 "FIX")

响应体 (200 OK):
```json
{
  "items": [
    {
      "id": "uuid-sv-1",
      "version": 1,
      "generation_source": "initial",
      "is_current": false,
      "created_at": "2026-03-05T14:00:00Z"
    },
    {
      "id": "uuid-sv-2",
      "version": 2,
      "generation_source": "self_healing",
      "is_current": true,
      "created_at": "2026-03-05T14:30:00Z"
    }
  ]
}
```

#### 4.3.6 任务执行 (Tasks)

**`POST /api/v1/tasks/run-check`**

下发检查任务。

请求体:
```json
{
  "rule_id": "uuid-rule-1",
  "host_ids": ["uuid-host-1", "uuid-host-2"]
}
```

响应体 (202 Accepted):
```json
{
  "task_group_id": "uuid-task-group-1",
  "tasks": [
    {"task_id": "uuid-task-1", "host_id": "uuid-host-1", "status": "PENDING"},
    {"task_id": "uuid-task-2", "host_id": "uuid-host-2", "status": "PENDING"}
  ]
}
```

**`POST /api/v1/tasks/run-fix`** — V2.0 新增

下发修复任务。请求和响应格式与 `run-check` 相同。

请求体:
```json
{
  "rule_id": "uuid-rule-1",
  "host_ids": ["uuid-host-1"]
}
```

响应体 (202 Accepted): 同 `run-check` 格式。

**`GET /api/v1/tasks/{group_id}/status`** — V2.0 新增

获取任务组的整体执行状态。

响应体 (200 OK):
```json
{
  "task_group_id": "uuid-task-group-1",
  "overall_status": "running",
  "tasks": [
    {
      "task_id": "uuid-task-1",
      "host_id": "uuid-host-1",
      "hostname": "web-server-01",
      "status": "SUCCESS",
      "exit_code": 0,
      "is_healing": false
    },
    {
      "task_id": "uuid-task-2",
      "host_id": "uuid-host-2",
      "hostname": "web-server-02",
      "status": "HEALING",
      "exit_code": 2,
      "is_healing": true,
      "healing_attempt": 1,
      "healing_max_attempts": 3
    }
  ]
}
```

**`GET /api/v1/tasks/{task_id}/logs`**

获取单个任务的实时执行日志（从 Redis LIST 增量拉取）。

Query Parameters: `offset` (int, default 0) — 从第几行开始拉取

响应体 (200 OK):
```json
{
  "task_id": "uuid-task-1",
  "status": "RUNNING",
  "logs": [
    {"timestamp": "2026-03-05T14:30:01Z", "stream": "stdout", "line": "Running check..."},
    {"timestamp": "2026-03-05T14:30:02Z", "stream": "stdout", "line": "Checking /etc/ssh/sshd_config..."},
    {"timestamp": "2026-03-05T14:30:03Z", "stream": "stderr", "line": "Warning: PermitRootLogin is set to yes"}
  ],
  "total_lines": 3,
  "is_finished": false
}
```

#### 4.3.7 自愈流程 — V2.0 新增

**`GET /api/v1/healing/{healing_id}`**

获取自愈流程的详细信息。

响应体 (200 OK):
```json
{
  "id": "uuid-healing-1",
  "original_task_id": "uuid-task-1",
  "rule_id": "uuid-rule-1",
  "host_id": "uuid-host-1",
  "script_type": "FIX",
  "status": "healed",
  "total_attempts": 2,
  "max_attempts": 3,
  "trigger_error": "bash: line 5: sshd_config: command not found",
  "attempts": [
    {
      "attempt": 1,
      "error_input": "bash: line 5: sshd_config: command not found",
      "fix_summary": "将 sshd_config 替换为完整路径 /etc/ssh/sshd_config",
      "result_exit_code": 1,
      "result_stderr": "Permission denied",
      "timestamp": "2026-03-05T14:30:00Z"
    },
    {
      "attempt": 2,
      "error_input": "Permission denied",
      "fix_summary": "在关键命令前添加 sudo",
      "result_exit_code": 0,
      "result_stderr": "",
      "timestamp": "2026-03-05T14:30:30Z"
    }
  ],
  "started_at": "2026-03-05T14:29:55Z",
  "finished_at": "2026-03-05T14:30:35Z"
}
```

#### 4.3.8 健康检查

**`GET /health`**

用于 Docker 和负载均衡器的健康检查端点。

响应体 (200 OK):
```json
{
  "status": "healthy",
  "components": {
    "database": "connected",
    "redis": "connected",
    "minio": "connected"
  },
  "version": "2.0.0",
  "uptime": "2h30m15s"
}
```

## 5. API 接口汇总

以下是 V2.0 版本所有 API 接口的完整汇总。

| 方法 | 路径 | 描述 | 版本 |
|:---|:---|:---|:---|
| GET | `/health` | 健康检查 | V1.6 更新 |
| GET | `/api/v1/config/llm` | 获取 LLM 配置 | V1.6 |
| POST | `/api/v1/config/llm` | 更新 LLM 配置 | V1.6 |
| POST | `/api/v1/config/llm/test` | 测试 LLM 连通性 | V1.6 更新 |
| GET | `/api/v1/agent/install-command` | 获取安装命令 | V1.6 |
| GET | `/api/v1/agent/install.sh` | 获取安装脚本 | V1.6 |
| GET | `/api/v1/agent/download` | 获取 Agent 下载链接 | V1.6 |
| GET | `/api/v1/hosts` | 获取主机列表 | V1.6 |
| POST | `/api/v1/templates/upload` | 上传模板文件 | V1.6 |
| GET | `/api/v1/templates` | 获取模板列表 | V1.6 |
| GET | `/api/v1/templates/{id}/status` | 获取模板解析状态 | **V2.0 新增** |
| GET | `/api/v1/templates/{id}/rules` | 获取模板规则列表 | V1.6 |
| DELETE | `/api/v1/templates/{id}` | 删除模板 | **V2.0 新增** |
| GET | `/api/v1/rules/{id}/scripts` | 获取规则脚本内容 | **V2.0 新增** |
| POST | `/api/v1/rules/{id}/scripts/regenerate` | 重新生成脚本 | **V2.0 新增** |
| GET | `/api/v1/rules/{id}/scripts/versions` | 获取脚本版本历史 | **V2.0 新增** |
| POST | `/api/v1/tasks/run-check` | 下发检查任务 | V1.6 |
| POST | `/api/v1/tasks/run-fix` | 下发修复任务 | **V2.0 新增** |
| GET | `/api/v1/tasks/{group_id}/status` | 获取任务组状态 | **V2.0 新增** |
| GET | `/api/v1/tasks/{task_id}/logs` | 获取任务日志 | V1.6 更新 |
| GET | `/api/v1/healing/{healing_id}` | 获取自愈详情 | **V2.0 新增** |
