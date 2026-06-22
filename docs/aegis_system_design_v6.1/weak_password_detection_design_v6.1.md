# Aegis V6.1 弱密码检测能力设计

## 1. 背景与问题

Aegis V6.0 已经把主要功能接入智能体模式。V6.1 的弱密码检测能力需要继续沿用 AI 原生路线：系统不只提供一个固定扫描入口，而是让服务端基于资产采集结果识别哪些主机、应用和配置文件可能存在可采集的账号密码材料，再由 Agent 上的标准弱密码工具读取目标文件字段并回传账号、加密密码或明文密码，最后由服务端和大模型共同完成字典、混合、模糊匹配和结果解释。

本设计要解决的问题：

1. Agent 如何实现弱密码采集工具，而不是执行任意 shell 脚本。
2. 服务端如何根据资产采集结果让大模型分析可采集应用和配置位置。
3. server 如何把弱密码工具调用下发到指定在线 Agent。
4. Agent 如何读取文件中指定字段，标准化输出账号、明文密码、hash、salt 或认证字符串。
5. 服务端如何对明文密码做字典匹配，对加密密码/hash 让大模型参与匹配分析。
6. 当读取配置文件失败、路径不准、字段不准时，如何给大模型提供受控辅助工具继续定位配置密码文件。
7. 明确禁止使用 `find`、全盘搜索、任意命令执行等不可控方式。

## 2. 设计目标

### 2.1 主目标

- 新增 Agent 弱密码工具集，支持按服务端下发的文件路径、字段选择器和应用 Profile 读取账号密码材料。
- 服务端基于资产采集结果和大模型分析生成弱密码采集任务。
- server 复用现有 `ExecuteTool` 链路，把弱密码工具调用下发给对应 Agent。
- Agent 返回统一 `CredentialRecord`，让服务端不依赖各应用的原始文件格式。
- 明文密码由服务端直接按字典、混合规则和模糊规则匹配。
- 加密密码/hash 场景由服务端构造 LLM 匹配任务，把应用类型、账号、加密密码材料、salt、算法提示和字典批次明确标注后交给大模型分析，并由服务端做可复现校验。
- 读文件失败时，大模型只能调用受控辅助工具定位配置路径，不能调用 `find` 或任意 shell。

### 2.2 用户价值

- 不需要用户手动知道每个应用密码放在哪里，系统可根据资产和 AI 自动推断。
- 不需要为每类应用写一次页面流程，智能体可通过统一工具完成采集、匹配和解释。
- 结果能展示到弱密码页面、主机详情和智能体结果卡片中，形成风险闭环。
- 弱密码采集失败时，系统能解释失败原因，并尝试用受控工具修复采集路径或字段选择。

## 3. 范围和边界

### 3.1 V6.1 范围内

- Linux 主机本地账号 `/etc/passwd` 和 `/etc/shadow` 材料采集。
- 基于资产采集结果识别 Redis、MySQL、PostgreSQL、Nginx/Apache Basic Auth、AI Agent、MCP Server、LLM 服务网关等可采集应用。
- 读取配置文件、认证文件、环境文件、systemd unit 引用文件中的账号和密码字段。
- 采集明文密码、hash、salt、加密认证字符串和算法提示。
- 服务端直接匹配明文密码。
- 大模型参与加密密码/hash 的应用识别、字段解释、候选字典批次匹配、混合规则和模糊规则匹配。
- 服务端对大模型返回的命中候选做二次校验；无法二次校验的专有算法结果标记为 `ai_inferred_needs_confirm`。
- 智能体模式下支持自然语言发起、查看、复测和解释弱密码任务。
- 普通页面支持任务列表、主机明细、命中弱密码、失败原因、AI 分析依据和整改建议。

### 3.2 范围外

- 不做互联网未授权目标扫描。
- 不做暴力破解和高频在线登录尝试。
- 不允许 Agent 执行 `find`、`locate`、`grep -R /`、任意 shell 或递归全盘扫描来找密码文件。
- 不自动修改主机密码，不自动禁用账号。
- 不把未脱敏密码写入日志、普通审计摘要或 LLM 长期记忆。
- Windows AD、LDAP、Kerberos 弱密码检测作为后续版本扩展。

## 4. 总体架构

```text
资产采集结果
  ↓
api-server WeakPasswordService
  ↓
LLM AnalyzeCollectableApplications
  ↓
生成 CredentialCollectionPlan
  ↓
api-server serverClient.ExecuteTool
  ↓
server APIServerToServer.ExecuteTool
  ↓
server GRPCServer.ExecuteTool
  ↓
Agent CallbackClient.ExecuteTool
  ↓
Agent WeakPassword 工具集
  ↓
读取指定文件和字段，输出 CredentialRecord[]
  ↓
api-server WeakPasswordMatchService
  ├─ 明文密码：服务端字典/混合/模糊匹配
  └─ 加密密码/hash：LLM 匹配任务 + 服务端校验
  ↓
weak_password_findings 入库
  ↓
前端页面 / 智能体结果卡片 / 攻击研判上下文
```

关键分工：

| 组件 | 职责 |
|:---|:---|
| api-server | 任务编排、资产查询、LLM 调用、字典管理、匹配调度、结果入库 |
| server | 不理解弱密码业务，只负责 Agent 在线连接查找和工具调用转发 |
| Agent | 实现弱密码采集工具，按下发计划读取文件和字段，标准化输出账号密码材料 |
| LLM | 分析资产中的可采集应用、生成采集计划、处理采集失败修复、参与加密密码/hash 和模糊匹配分析 |
| frontend | 展示任务、主机结果、命中密码、失败原因、AI 解释和整改建议 |

## 5. 端到端流程

### 5.1 任务发起

用户可以通过两种方式发起：

1. 普通页面选择主机、主机组、应用类型和字典策略。
2. 智能体模式输入自然语言，例如“检查生产环境 Redis 和 MySQL 是否存在弱密码”。

api-server 创建 `weak_password_scan_tasks` 后进入异步执行。

### 5.2 基于资产的大模型分析

api-server 查询资产采集结果：

- 主机基础信息：OS、发行版、Agent 版本、在线状态。
- 进程资产：进程名、命令行、运行用户、工作目录。
- 端口资产：监听端口、协议、绑定地址。
- 软件资产：包名、版本、安装路径。
- 应用资产：应用类型、配置路径、数据目录、服务名。
- AI 资产：AI Agent、MCP Server、LLM 网关、工具配置文件。

api-server 将脱敏资产上下文发给 LLM，让 LLM 输出可采集应用：

```json
{
  "host_id": "host-uuid",
  "asset_context": {
    "os": "linux",
    "processes": [
      {
        "name": "redis-server",
        "cmdline": "/usr/bin/redis-server /etc/redis/redis.conf",
        "run_user": "redis"
      }
    ],
    "ports": [
      {
        "port": 6379,
        "process_name": "redis-server"
      }
    ],
    "known_config_paths": [
      "/etc/redis/redis.conf"
    ]
  },
  "task": "analyze_collectable_credential_sources"
}
```

LLM 输出：

```json
{
  "collectable_applications": [
    {
      "application": "redis",
      "confidence": 0.96,
      "reason": "redis-server command line references /etc/redis/redis.conf",
      "profile_id": "redis_config_v1",
      "candidate_files": [
        "/etc/redis/redis.conf"
      ],
      "credential_fields": [
        {
          "account_field": null,
          "password_field": "requirepass",
          "credential_format": "plaintext_or_acl_secret"
        }
      ]
    }
  ]
}
```

### 5.3 服务端生成采集计划

api-server 把 LLM 输出转换为 `CredentialCollectionPlan`：

```json
{
  "task_id": "scan-task-uuid",
  "host_id": "host-uuid",
  "plan_id": "plan-uuid",
  "applications": [
    {
      "application": "redis",
      "profile_id": "redis_config_v1",
      "paths": [
        "/etc/redis/redis.conf"
      ],
      "extractors": [
        {
          "type": "line_key_value",
          "account_selector": null,
          "password_selector": "requirepass",
          "format_hint": "plaintext"
        }
      ]
    }
  ],
  "collection_policy": {
    "max_file_bytes": 1048576,
    "allow_auxiliary_locator_tools": true,
    "forbid_recursive_search": true,
    "forbid_find_command": true
  }
}
```

### 5.4 server 下发

api-server 对每台主机调用现有 gRPC：

```json
{
  "call_id": "weakpass:scan-task-uuid:host-uuid:collect:0",
  "host_id": "host-uuid",
  "tool": "WeakPassword.CollectCredentials",
  "arguments": {
    "task_id": "scan-task-uuid",
    "plan_id": "plan-uuid",
    "applications": [],
    "collection_policy": {}
  },
  "timeout_seconds": 180
}
```

server 处理方式：

1. `APIServerToServerImpl.ExecuteTool` 接收 `ToolExecuteRequest`。
2. 转换为 `agent_comm.ToolRequest`：
   - `CallId = req.CallId`
   - `HostId = req.HostId`
   - `Tool = req.Tool`
   - `ParamsJson = req.Arguments`
3. `GRPCServer.ExecuteTool` 根据 `host_id` 查找 `agentConnections`。
4. 如果连接不存在，返回 `agent not connected`。
5. 如果 `CallbackClient` 为空，返回 `agent callback client not available`。
6. 调用 `agentConn.CallbackClient.ExecuteTool(ctx, req)` 下发到 Agent。
7. server 将 Agent 响应原样返回 api-server。

server 不解析弱密码业务参数，不读取密码，不做匹配。

### 5.5 Agent 采集

Agent 收到 `WeakPassword.CollectCredentials` 后：

1. 校验工具名在 allowlist。
2. 校验路径来自服务端计划，不接受任意路径追加。
3. 校验禁止递归扫描和禁止 `find`。
4. 按应用 Profile 打开文件。
5. 按 extractor 读取账号字段和密码字段。
6. 判断密码材料类型：`plaintext`、`hash`、`salted_hash`、`encrypted_blob`、`auth_string`、`unknown`。
7. 输出统一 `CredentialRecord[]`。
8. 对读文件失败、权限不足、字段不存在、格式不支持分别输出结构化错误。

### 5.6 服务端匹配

服务端收到 `CredentialRecord[]` 后按类型处理：

- `plaintext`：服务端直接和字典、混合规则、模糊候选做比较。
- `hash`、`salted_hash`、`encrypted_blob`、`auth_string`：服务端构造 LLM 匹配任务，把应用类型、账号、加密密码材料、salt、算法提示、字典块明确分区发送给大模型，让大模型分析可能命中的候选。
- LLM 返回候选后，服务端用本地 verifier 做二次校验；可校验通过才入库为 `confirmed`。
- 无法校验的专有格式可入库为 `ai_inferred_needs_confirm`，页面必须标识为 AI 推断待确认。

### 5.7 失败修复循环

如果 Agent 返回读文件失败或字段提取失败：

1. api-server 把错误、资产上下文和已尝试路径发给 LLM。
2. LLM 只能选择受控辅助工具继续定位。
3. Agent 执行辅助工具，例如非递归列目录、检查 systemd unit、检查进程启动参数、读取环境文件引用。
4. LLM 基于辅助工具结果生成新的候选路径或字段选择器。
5. api-server 再次下发 `WeakPassword.CollectCredentials`。
6. 如果大模型累计调用 10 次 Agent 辅助工具仍无法获取有效配置文件，标记为 `config_discovery_failed` 并在前端展示失败原因和已尝试路径。

## 6. Agent 弱密码工具设计

### 6.1 工具列表

| 工具名 | 用途 | 是否可由 LLM 间接触发 |
|:---|:---|:---|
| `WeakPassword.CollectCredentials` | 按计划读取账号密码字段并返回标准凭据记录 | 是 |
| `WeakPassword.ProbePath` | 检查指定路径是否存在、类型、大小、权限、owner | 是 |
| `WeakPassword.ListConfigDir` | 非递归列出指定配置目录下的文件名 | 是 |
| `WeakPassword.ReadConfigSlice` | 读取指定文件小片段，用于判断格式和字段位置 | 是 |
| `WeakPassword.ServiceUnitInspect` | 读取指定 systemd service 的 ExecStart、EnvironmentFile、WorkingDirectory | 是 |
| `WeakPassword.ProcessConfigHints` | 根据指定 pid 获取启动参数、cwd、有限 open config files | 是 |
| `WeakPassword.PurgeCredentialCache` | 清除本次任务的临时缓存 | 否 |

禁止工具和行为：

- 禁止 `find`。
- 禁止 `locate`。
- 禁止递归目录遍历。
- 禁止任意 shell。
- 禁止读取未在计划、资产路径、Profile 默认路径或辅助工具候选中的文件。
- 禁止把文件全文无边界回传给 LLM。

### 6.2 Agent 入口

在 `agent/internal/tools/tool_manager.go` 增加弱密码工具分支：

```go
case "WeakPassword.CollectCredentials":
    return m.weakPasswordCollector.CollectCredentials(context.Background(), params)
case "WeakPassword.ProbePath":
    return m.weakPasswordCollector.ProbePath(context.Background(), params)
case "WeakPassword.ListConfigDir":
    return m.weakPasswordCollector.ListConfigDir(context.Background(), params)
case "WeakPassword.ReadConfigSlice":
    return m.weakPasswordCollector.ReadConfigSlice(context.Background(), params)
case "WeakPassword.ServiceUnitInspect":
    return m.weakPasswordCollector.ServiceUnitInspect(context.Background(), params)
case "WeakPassword.ProcessConfigHints":
    return m.weakPasswordCollector.ProcessConfigHints(context.Background(), params)
case "WeakPassword.PurgeCredentialCache":
    return m.weakPasswordCollector.PurgeCredentialCache(context.Background(), params)
```

`agent/internal/client.HandleToolCall` 保持现有逻辑：解析 `ToolRequest.ParamsJson`，调用 `toolManager.Execute`，把结果序列化为 `ToolResponse.ResultJson`。

### 6.3 CollectCredentials 请求

```json
{
  "task_id": "scan-task-uuid",
  "plan_id": "plan-uuid",
  "host_id": "host-uuid",
  "applications": [
    {
      "application": "mysql",
      "asset_id": "asset-uuid",
      "profile_id": "mysql_config_v1",
      "paths": [
        "/etc/mysql/mysql.conf.d/mysqld.cnf",
        "/etc/mysql/debian.cnf"
      ],
      "extractors": [
        {
          "type": "ini",
          "section": "client",
          "account_selector": "user",
          "password_selector": "password",
          "format_hint": "plaintext"
        }
      ]
    }
  ],
  "collection_policy": {
    "max_file_bytes": 1048576,
    "max_records": 500,
    "redact_context_values": true,
    "forbid_find_command": true,
    "forbid_recursive_search": true
  }
}
```

### 6.4 CredentialRecord 标准输出

Agent 返回：

```json
{
  "task_id": "scan-task-uuid",
  "plan_id": "plan-uuid",
  "host_id": "host-uuid",
  "records": [
    {
      "record_id": "record-uuid",
      "application": "mysql",
      "asset_id": "asset-uuid",
      "source_path": "/etc/mysql/debian.cnf",
      "source_kind": "config_file",
      "account": "debian-sys-maint",
      "credential_type": "plaintext",
      "credential_value": "password-value",
      "salt": "",
      "algorithm_hint": "",
      "field_path": "client.password",
      "parser": "ini",
      "confidence": 0.94
    },
    {
      "record_id": "record-uuid-2",
      "application": "linux_shadow",
      "source_path": "/etc/shadow",
      "source_kind": "system_account",
      "account": "root",
      "credential_type": "salted_hash",
      "credential_value": "$6$salt$hash",
      "salt": "salt",
      "algorithm_hint": "sha512-crypt",
      "field_path": "shadow.password",
      "parser": "shadow",
      "confidence": 1.0
    }
  ],
  "errors": []
}
```

说明：

- `credential_value` 可以是明文密码、hash、完整 shadow 字段、加密 blob 或认证字符串。
- 传输层走现有 gRPC TLS/内网通道；如果部署要求更高，可在 `credential_value` 外再加任务级 envelope 加密。
- Agent 日志不得打印 `credential_value`、`salt`、认证字符串或明文密码。
- 服务端普通业务表默认不保存原始 `credential_value`，只保存命中结果、脱敏值和必要 evidence；原值仅在匹配任务内存中短时存在。

### 6.5 采集实现规则

Agent 采集时按以下顺序执行：

1. 校验 `task_id`、`plan_id`、`host_id`。
2. 校验每个 path 是否来自服务端计划。
3. 调用 `os.Stat` 判断文件存在性、类型和大小。
4. 文件超过 `max_file_bytes` 直接返回 `file_too_large`。
5. 根据 extractor 类型解析：
   - `shadow`：解析 `/etc/passwd` 与 `/etc/shadow`。
   - `ini`：解析 section/key。
   - `yaml`：解析 path selector。
   - `json`：解析 JSONPath。
   - `properties`：解析 key/value。
   - `line_key_value`：解析 `key value` 或 `key=value`。
   - `htpasswd`：解析 Basic Auth 账号和 hash。
6. 生成 `CredentialRecord`。
7. 对每个失败项生成 `CredentialCollectionError`，不中断其它路径采集。

错误结构：

```json
{
  "application": "redis",
  "source_path": "/etc/redis/redis.conf",
  "error_code": "permission_denied",
  "message": "permission denied when reading config file",
  "retryable": true,
  "suggested_auxiliary_tools": [
    "WeakPassword.ServiceUnitInspect",
    "WeakPassword.ProcessConfigHints"
  ]
}
```

## 7. 服务端 AI 编排设计

### 7.1 资产到采集计划

新增 `WeakPasswordAssetPlanner`：

1. 查询在线主机和资产采集结果。
2. 过滤没有 Agent、Agent 离线或权限不足的主机。
3. 把资产上下文脱敏后发送给 LLM。
4. LLM 输出可采集应用、候选文件、字段选择器和置信度。
5. 服务端使用内置 Profile 校验 LLM 结果，剔除不允许路径。
6. 生成 `CredentialCollectionPlan`。

### 7.2 LLM 提示词要求

LLM 分析资产时必须输出 JSON，不输出自然语言自由文本：

```json
{
  "collectable_applications": [
    {
      "application": "redis",
      "profile_id": "redis_config_v1",
      "confidence": 0.96,
      "candidate_files": [
        "/etc/redis/redis.conf"
      ],
      "extractors": [
        {
          "type": "line_key_value",
          "password_selector": "requirepass",
          "format_hint": "plaintext"
        }
      ],
      "reason": "process command line references redis.conf"
    }
  ],
  "rejected_applications": [
    {
      "application": "unknown",
      "reason": "no credential-bearing config path found"
    }
  ]
}
```

服务端校验规则：

- `candidate_files` 必须在资产采集路径、Profile 默认路径或受控辅助工具返回路径中。
- `profile_id` 必须存在。
- extractor 类型必须在 allowlist 中。
- 不接受包含通配符的任意路径，如 `/var/**`。
- 不接受 `find`、shell、管道命令或命令字符串。

### 7.3 采集失败修复

新增 `WeakPasswordCollectionRepairService`。当 Agent 返回错误时，服务端构造修复上下文：

```json
{
  "application": "redis",
  "failed_path": "/etc/redis/redis.conf",
  "error_code": "file_not_found",
  "asset_context": {
    "process_cmdline": "/usr/bin/redis-server /opt/redis/conf/redis.conf",
    "service_name": "redis-server",
    "run_user": "redis"
  },
  "allowed_auxiliary_tools": [
    "WeakPassword.ProbePath",
    "WeakPassword.ListConfigDir",
    "WeakPassword.ServiceUnitInspect",
    "WeakPassword.ProcessConfigHints"
  ],
  "forbidden": [
    "find",
    "locate",
    "recursive_scan",
    "arbitrary_shell"
  ]
}
```

LLM 只能返回下一步工具调用：

```json
{
  "tool": "WeakPassword.ProcessConfigHints",
  "arguments": {
    "pid": 1234,
    "include_open_files": true,
    "file_suffix_allowlist": [
      ".conf",
      ".cnf",
      ".yml",
      ".yaml",
      ".json",
      ".properties"
    ],
    "max_files": 20
  },
  "reason": "process command line may reveal actual config file path"
}
```

修复循环限制：

- 每个应用最多 10 次 Agent 辅助工具调用。
- 每轮只能调用一个辅助工具。
- `ListConfigDir` 只能列服务端允许的目录，且非递归。
- `ReadConfigSlice` 最多读取前后各 80 行或最多 16KB。
- 如果错误是权限不足，不尝试绕过权限，只返回 `permission_denied` 和整改建议。

## 8. 应用 Profile 设计

### 8.1 Profile 结构

```json
{
  "profile_id": "redis_config_v1",
  "application": "redis",
  "default_paths": [
    "/etc/redis/redis.conf",
    "/etc/redis/redis-server.conf"
  ],
  "asset_path_hints": [
    "process_cmdline",
    "systemd_exec_start",
    "systemd_environment_file"
  ],
  "extractors": [
    {
      "type": "line_key_value",
      "password_selector": "requirepass",
      "format_hint": "plaintext"
    },
    {
      "type": "line_key_value",
      "account_selector": "user",
      "password_selector": "password",
      "format_hint": "plaintext"
    }
  ],
  "dictionary_context": [
    "redis",
    "cache",
    "company",
    "hostname"
  ]
}
```

### 8.2 V6.1 内置 Profile

| 应用 | 常见来源 | 凭据格式 |
|:---|:---|:---|
| Linux local account | `/etc/passwd`、`/etc/shadow` | salted hash |
| Redis | `redis.conf`、ACL 文件 | plaintext、ACL secret、hash-like auth string |
| MySQL/MariaDB | `my.cnf`、`debian.cnf`、应用连接配置 | plaintext、mysql auth string |
| PostgreSQL | `pg_hba.conf`、`.pgpass`、应用连接配置 | plaintext、md5/scram auth string |
| Nginx/Apache Basic Auth | `.htpasswd`、vhost 配置引用 | bcrypt、apr1、sha1、crypt |
| AI Agent | agent config、tool config、env file | plaintext token、API key、hashed secret |
| MCP Server | MCP config、env file、stdio server config | plaintext token、API key |
| LLM Gateway | gateway yaml/json/env | plaintext token、hashed API key |

## 9. 服务端匹配设计

### 9.1 字典来源

服务端支持：

- 内置 1000 条默认弱密码字典，覆盖常见弱口令、默认口令、应用默认口令和企业常见组合模式。
- 用户上传字典。
- 按应用类型的专用字典。
- 基于主机名、应用名、账号名、公司名、环境名生成的 AI 候选词。
- 混合规则：大小写变换、年份后缀、特殊字符后缀、账号名拼接、应用名拼接。
- 模糊规则：相似字符替换、拼音/英文近似、键盘邻近、Levenshtein 距离限制。
- 前端提供“AI 一键生成字典”入口，用户输入业务场景、应用、账号命名、组织关键词和数量上限后，由 LLM 生成候选弱口令字典，服务端去重、风险标记、保存为任务字典或个人字典。

### 9.2 明文密码匹配

对于 `credential_type=plaintext`：

1. 服务端读取 `credential_value`。
2. 对字典、混合候选和模糊候选做直接比较。
3. 命中后生成 `WeakPasswordFinding`。
4. 原始明文只在匹配内存中使用，入库保存脱敏值和必要加密 evidence。

命中结果：

```json
{
  "record_id": "record-uuid",
  "account": "admin",
  "application": "redis",
  "match_status": "confirmed",
  "credential_type": "plaintext",
  "matched_password": "Admin@123",
  "matched_password_mask": "A***3",
  "match_source": "builtin_dictionary",
  "match_rule": "exact"
}
```

### 9.3 加密密码/hash 的 AI 匹配

对于 `credential_type=hash`、`salted_hash`、`encrypted_blob`、`auth_string`，服务端构造 `LLMPasswordMatchJob`。大模型输入必须清楚标注哪部分是字典、哪部分是加密密码、是什么应用：

```json
{
  "job_type": "encrypted_password_dictionary_match",
  "application": "linux_shadow",
  "algorithm_hint": "sha512-crypt",
  "credential_block": [
    {
      "record_id": "record-uuid",
      "account": "root",
      "credential_type": "salted_hash",
      "encrypted_password": "$6$salt$hash",
      "salt": "salt",
      "field_path": "shadow.password"
    }
  ],
  "dictionary_block": [
    {
      "candidate_id": "dict-1",
      "candidate": "admin"
    },
    {
      "candidate_id": "dict-2",
      "candidate": "Admin@123"
    }
  ],
  "matching_instruction": {
    "task": "find whether any dictionary candidate matches encrypted_password",
    "return_only_candidate_ids": false,
    "include_reason": true,
    "application_context": "Linux /etc/shadow sha512-crypt"
  }
}
```

LLM 返回：

```json
{
  "matches": [
    {
      "record_id": "record-uuid",
      "account": "root",
      "candidate_id": "dict-2",
      "candidate": "Admin@123",
      "confidence": 0.98,
      "reason": "candidate matches sha512-crypt material with provided salt",
      "requires_server_verify": true
    }
  ],
  "unsupported_records": []
}
```

可靠性要求：

- 大模型负责识别格式、选择候选、处理模糊和混合语义。
- 服务端必须对可支持算法执行本地 verifier 二次校验。
- 二次校验通过：`match_status=confirmed`。
- 二次校验失败：丢弃该命中，记录 `llm_match_verify_failed`。
- 算法未知或专有加密无法校验：`match_status=ai_inferred_needs_confirm`，页面标记为“AI 推断，待人工确认”。

这样既满足“加密密码和字典交给大模型分析匹配”的产品要求，也避免把模型不可复现的计算结果直接当作最终事实。

### 9.4 混合和模糊匹配

混合和模糊匹配分两段：

1. LLM 根据应用、账号、主机名、组织关键词和字典生成候选扩展。
2. 服务端对明文直接比较；对加密密码/hash 构造新的 `LLMPasswordMatchJob` 并执行二次校验。

示例：

```json
{
  "seed_dictionary": [
    "admin",
    "redis",
    "aegis"
  ],
  "context": {
    "application": "redis",
    "account": "default",
    "hostname": "prod-cache-01",
    "environment": "prod"
  },
  "rules": [
    "append_year",
    "append_special_char",
    "capitalize",
    "leet_replace",
    "application_account_mix"
  ]
}
```

LLM 输出候选：

```json
{
  "candidates": [
    {
      "candidate": "Redis@2026",
      "rule": "capitalize+append_special_char+append_year"
    },
    {
      "candidate": "prod-cache-01@123",
      "rule": "hostname_mix"
    }
  ]
}
```

### 9.5 LLM 批次控制

为避免上下文过大：

- 每个 LLM 匹配批次最多 200 条候选。
- 每个 `CredentialRecord` 最多匹配 5000 条候选。
- 高成本算法如 bcrypt 默认限制更低。
- LLM 匹配任务必须有 `task_id`、`batch_id`、`record_id`。
- 超过限制的任务拆分批次执行。

## 10. 辅助定位工具设计

### 10.1 为什么需要辅助工具

实际采集中会遇到：

- 资产记录的配置路径不存在。
- 应用启动参数中引用了另一份配置。
- systemd unit 使用 `EnvironmentFile`。
- 密码字段在 include 文件中。
- 文件权限不足。
- 配置格式和 Profile 不一致。

因此需要让大模型在失败后继续调用受控工具定位文件，但工具必须有限制。

### 10.2 允许的辅助工具

| 工具 | 参数限制 | 返回内容 |
|:---|:---|:---|
| `WeakPassword.ProbePath` | 只能检查一个指定路径 | exists、type、size、mode、owner |
| `WeakPassword.ListConfigDir` | 只能列 allowlist 目录，非递归，最多 200 项 | 文件名、类型、大小、mtime |
| `WeakPassword.ReadConfigSlice` | 只能读指定 offset/line range，最多 16KB | 片段内容，敏感值可按策略脱敏 |
| `WeakPassword.ServiceUnitInspect` | 只能查指定 service | ExecStart、EnvironmentFile、WorkingDirectory |
| `WeakPassword.ProcessConfigHints` | 只能查资产中已知 pid | cmdline、cwd、有限 config open files |

### 10.3 禁止项

- 不允许 `find / -name "*pass*"`。
- 不允许 `grep -R password /`。
- 不允许读取 `/proc/*/environ` 全量内容并回传。
- 不允许递归列目录。
- 不允许 LLM 生成 shell 命令。
- 不允许扫描用户 home 目录，除非资产 Profile 明确给出应用配置路径。

### 10.4 修复示例

Agent 返回：

```json
{
  "error_code": "file_not_found",
  "application": "redis",
  "source_path": "/etc/redis/redis.conf"
}
```

LLM 选择：

```json
{
  "tool": "WeakPassword.ServiceUnitInspect",
  "arguments": {
    "service_name": "redis-server"
  }
}
```

Agent 返回：

```json
{
  "service_name": "redis-server",
  "exec_start": "/usr/bin/redis-server /opt/redis/conf/redis.conf",
  "environment_files": []
}
```

LLM 生成新的采集计划：

```json
{
  "application": "redis",
  "paths": [
    "/opt/redis/conf/redis.conf"
  ],
  "extractors": [
    {
      "type": "line_key_value",
      "password_selector": "requirepass",
      "format_hint": "plaintext"
    }
  ]
}
```

## 11. 数据库设计

### 11.1 weak_password_scan_tasks

```sql
CREATE TABLE weak_password_scan_tasks (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    trigger_source TEXT NOT NULL,
    status TEXT NOT NULL,
    scope_json JSONB NOT NULL DEFAULT '{}',
    dictionary_policy_json JSONB NOT NULL DEFAULT '{}',
    ai_policy_json JSONB NOT NULL DEFAULT '{}',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 11.2 weak_password_collection_plans

```sql
CREATE TABLE weak_password_collection_plans (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id),
    host_id UUID NOT NULL,
    plan_json JSONB NOT NULL,
    llm_analysis_json JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 11.3 weak_password_scan_hosts

```sql
CREATE TABLE weak_password_scan_hosts (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id),
    host_id UUID NOT NULL,
    status TEXT NOT NULL,
    agent_status TEXT NOT NULL,
    collected_records INT NOT NULL DEFAULT 0,
    matched_findings INT NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
```

### 11.4 weak_password_findings

```sql
CREATE TABLE weak_password_findings (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id),
    host_id UUID NOT NULL,
    asset_id UUID,
    application TEXT NOT NULL,
    account TEXT NOT NULL,
    credential_type TEXT NOT NULL,
    match_status TEXT NOT NULL,
    matched_password_mask TEXT,
    matched_password_encrypted BYTEA,
    match_source TEXT NOT NULL,
    match_rule TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    evidence_json JSONB NOT NULL DEFAULT '{}',
    ai_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 11.5 weak_password_collection_errors

```sql
CREATE TABLE weak_password_collection_errors (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id),
    host_id UUID NOT NULL,
    application TEXT,
    source_path TEXT,
    error_code TEXT NOT NULL,
    error_message TEXT,
    repair_attempts INT NOT NULL DEFAULT 0,
    final_status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## 12. HTTP API 设计

| 方法 | 路径 | 说明 |
|:---|:---|:---|
| `POST` | `/api/v1/weak-password/asset-applications/analyze` | 一键分析应用资产，只分析资产采集中的应用资产，返回可能存在密码的应用列表 |
| `GET` | `/api/v1/weak-password/asset-applications` | 查询最近一次应用资产分析结果 |
| `POST` | `/api/v1/weak-password/tasks` | 创建弱密码任务 |
| `POST` | `/api/v1/weak-password/tasks/by-application` | 针对单个应用创建弱密码检查任务 |
| `GET` | `/api/v1/weak-password/tasks` | 查询任务列表 |
| `GET` | `/api/v1/weak-password/tasks/:id` | 查询任务详情 |
| `GET` | `/api/v1/weak-password/tasks/:id/progress` | 查询任务进度、当前阶段、Agent 工具调用次数和错误信息 |
| `GET` | `/api/v1/weak-password/tasks/:id/hosts` | 查询主机执行明细 |
| `GET` | `/api/v1/weak-password/tasks/:id/findings` | 查询命中结果 |
| `POST` | `/api/v1/weak-password/tasks/:id/retry-failed` | 重试失败主机 |
| `GET` | `/api/v1/weak-password/dictionaries/default` | 查询内置 1000 条弱密码默认字典的摘要、分类和启用状态 |
| `POST` | `/api/v1/weak-password/dictionaries/ai-generate` | AI 一键生成弱密码字典 |
| `POST` | `/api/v1/weak-password/dictionaries` | 保存上传字典或 AI 生成字典 |
| `POST` | `/api/v1/weak-password/findings/:id/reveal` | 审批后查看完整命中弱密码 |
| `POST` | `/api/v1/assistant/tools/weak-password.scan` | 智能体发起扫描 |
| `POST` | `/api/v1/assistant/tools/weak-password.explain` | 智能体解释结果 |

## 13. 智能体工具设计

### 13.1 `Credential.WeakPassword.Scan`

输入：

```json
{
  "scope": {
    "host_ids": [],
    "host_group_ids": [],
    "application_types": [
      "redis",
      "mysql",
      "linux_shadow"
    ]
  },
  "dictionary_policy": {
    "builtin": true,
    "uploaded_dictionary_ids": [],
    "hybrid": true,
    "fuzzy": true,
    "ai_generate_candidates": true
  },
  "ai_policy": {
    "analyze_collectable_applications": true,
    "repair_collection_errors": true,
    "encrypted_password_llm_match": true
  }
}
```

输出：

```json
{
  "task_id": "scan-task-uuid",
  "status": "running",
  "message": "弱密码检测任务已创建，正在分析资产并下发 Agent 采集工具"
}
```

### 13.2 `Credential.WeakPassword.QueryFindings`

支持按主机、应用、账号、风险等级、确认状态查询。

### 13.3 `Credential.WeakPassword.Explain`

智能体解释：

- 哪些应用存在弱密码。
- 密码从哪个文件字段采集。
- 是明文直接命中还是加密/hash 经 AI 匹配命中。
- 是否通过服务端二次校验。
- 如何修复。

## 14. 前端页面 PRD

### 14.1 产品定位

弱密码检测前端页面面向安全运营人员，核心目标是把“资产应用分析 -> 选择应用 -> 获取配置 -> 字典匹配 -> 查看结果和修复建议”串成一个可操作闭环。页面不是先让用户填写配置路径，而是先基于资产采集中的应用资产做一键分析，列出所有可能存在账号密码配置的应用，再允许用户按单个应用发起弱密码检查。

### 14.2 页面入口

菜单位置建议：

- 一级菜单：风险管理。
- 二级菜单：弱密码检测。

页面分为 4 个主 Tab：

| Tab | 目标 |
|:---|:---|
| 应用资产分析 | 一键分析资产中的应用资产，列出可能存在密码的应用 |
| 弱密码检查 | 对单个应用或多个应用发起检查，展示获取配置和匹配进度 |
| 字典管理 | 管理默认 1000 字典、上传字典、AI 生成字典 |
| 检测结果 | 查看命中弱密码、失败任务、AI 分析和整改建议 |

### 14.3 应用资产分析页

#### 14.3.1 页面目标

用户点击“一键分析资产应用”后，系统只分析资产采集结果中的应用资产，不扫描主机文件系统。分析完成后列出所有可能存在密码的应用，作为后续弱密码检查入口。

#### 14.3.2 空资产状态

如果当前范围内没有应用资产，页面展示空状态：

- 标题：暂无可分析的应用资产。
- 说明：请先执行资产采集，系统将基于采集到的应用资产分析可能存在密码的配置位置。
- 主按钮：去采集资产。
- 次按钮：刷新资产状态。

“去采集资产”跳转到现有资产采集入口，或打开资产采集任务创建抽屉。

#### 14.3.3 操作区

顶部操作：

- 主机范围选择：全部主机、主机组、指定主机。
- 应用类型过滤：全部、数据库、中间件、Web 服务、AI 应用、MCP 服务、LLM 网关。
- 在线 Agent 过滤：默认只看在线 Agent。
- 按钮：一键分析资产应用。
- 按钮：刷新。

#### 14.3.4 分析结果表

字段：

| 字段 | 说明 |
|:---|:---|
| 主机 | 主机名、IP、在线状态 |
| 应用 | 应用名称、版本、类型 |
| 资产来源 | 应用资产记录及其关联的进程、端口、软件信息 |
| 可能密码位置 | 配置路径、systemd unit、环境文件、认证文件 |
| 凭据类型 | 明文、hash、salted hash、auth string、unknown |
| AI 置信度 | 高、中、低 |
| 风险说明 | 为什么认为该应用可能存在密码 |
| 操作 | 检查弱密码、查看分析依据、忽略 |

#### 14.3.5 单应用弱密码检查入口

每行应用提供“检查弱密码”按钮。点击后打开检查确认抽屉：

- 展示目标主机和应用。
- 展示预计读取的配置路径和字段。
- 展示使用的字典策略。
- 可选择默认 1000 字典、上传字典、AI 生成字典、混合规则、模糊规则。
- 点击“开始检查”后创建单应用弱密码任务。

### 14.4 弱密码检查页

#### 14.4.1 主流程

单应用检查流程：

```text
选择应用
  ↓
确认配置采集计划
  ↓
Agent 获取配置
  ↓
服务端明文/加密密码匹配
  ↓
结果展示
```

#### 14.4.2 进度条

任务详情页顶部展示阶段进度条：

| 阶段 | 说明 |
|:---|:---|
| 资产应用分析 | LLM 基于应用资产生成可采集应用和配置位置 |
| 下发 Agent 工具 | server 通过 `ExecuteTool` 下发 `WeakPassword.CollectCredentials` |
| 获取配置文件 | Agent 读取指定文件和字段 |
| AI 修复定位 | 配置读取失败时，LLM 调用受控辅助工具继续定位 |
| 密码匹配 | 明文由服务端匹配，加密/hash 由 LLM 匹配并服务端校验 |
| 结果入库 | 写入 finding、错误和 AI 分析摘要 |

进度展示要求：

- 显示总进度百分比。
- 显示当前阶段名称。
- 显示当前正在处理的应用和主机。
- 显示已调用 Agent 工具次数，例如 `4/10`。
- 显示最近一次工具调用名称。
- 显示最近一次失败原因。

#### 14.4.3 10 次 Agent 工具调用上限

当大模型为了获取配置文件而调用 Agent 辅助工具时，前端必须展示调用次数。若累计调用 10 次仍未获得有效配置文件：

- 任务状态变为失败。
- 错误码显示为 `config_discovery_failed`。
- 错误说明：AI 已尝试 10 次受控 Agent 工具调用，仍未定位到有效配置文件。
- 展示已尝试路径、工具调用记录和失败原因。
- 提供操作：重试、修改范围、查看资产采集建议。

### 14.5 字典管理页

#### 14.5.1 默认 1000 弱密码字典

系统内置 1000 条默认弱密码字典。前端展示：

- 字典名称：默认弱密码字典。
- 条数：1000。
- 分类：通用弱口令、默认口令、数据库默认口令、中间件默认口令、AI 应用常见弱口令、企业组合模式。
- 状态：启用、停用。
- 操作：查看摘要、启用、停用、复制为自定义字典。

默认字典详情页只展示摘要、分类和样例数量，不在普通列表中直接展开全部明文条目。查看完整字典需要具备字典管理权限。

#### 14.5.2 AI 一键生成字典

字典管理页提供“AI 一键生成字典”按钮。点击后打开生成抽屉：

输入项：

| 字段 | 说明 |
|:---|:---|
| 生成目标 | 通用、指定应用、指定主机、指定账号模式 |
| 应用类型 | Redis、MySQL、PostgreSQL、Nginx Basic Auth、AI Agent、MCP Server、LLM Gateway |
| 组织关键词 | 公司简称、系统名、环境名、业务线 |
| 账号关键词 | admin、root、service、应用账号名 |
| 生成数量 | 默认 200，最大 1000 |
| 规则 | 年份后缀、特殊字符、大小写、拼音、leet、应用名组合、主机名组合 |
| 去重策略 | 与默认字典去重、与上传字典去重 |

生成结果页：

- 展示候选数量。
- 展示规则分布。
- 展示风险等级分布。
- 支持搜索、删除、批量删除。
- 支持保存为“任务临时字典”或“自定义字典”。

生成限制：

- 不允许生成超过管理员配置的最大条数。
- 不允许把完整字典写入普通日志。
- 保存字典需要记录创建人、提示词摘要、模型、生成时间和规则配置。

### 14.6 检测结果页

#### 14.6.1 命中结果表

字段：

| 字段 | 说明 |
|:---|:---|
| 主机 | 主机名、IP |
| 应用 | 应用名称、类型 |
| 账号 | 采集到的账号 |
| 凭据类型 | plaintext、hash、salted_hash、auth_string |
| 命中密码 | 默认展示脱敏值 |
| 匹配方式 | 默认字典、上传字典、AI 生成字典、混合、模糊 |
| 状态 | confirmed、ai_inferred_needs_confirm、verify_failed |
| 置信度 | LLM 或服务端匹配置信度 |
| 配置来源 | 文件路径和字段名 |
| AI 解释 | 命中依据和风险说明 |
| 操作 | 查看详情、申请明文、复测、标记误报 |

展示规则：

- 默认展示 `matched_password_mask`。
- 用户有权限并通过审批后可查看完整 `matched_password`。
- `ai_inferred_needs_confirm` 必须显示醒目标识，不能和 confirmed 混淆。

#### 14.6.2 采集失败表

字段：

- 主机。
- 应用。
- 失败阶段。
- 错误码。
- 最近一次 Agent 工具。
- Agent 工具调用次数。
- 已尝试配置路径。
- AI 修复建议。
- 操作。

常见错误：

| 错误码 | 页面说明 |
|:---|:---|
| `no_application_assets` | 当前范围没有应用资产，请先采集资产 |
| `agent_not_connected` | Agent 不在线，无法下发工具 |
| `permission_denied` | Agent 无权限读取配置文件 |
| `field_not_found` | 文件存在，但未找到密码字段 |
| `config_discovery_failed` | AI 已调用 10 次 Agent 工具仍未定位有效配置 |
| `llm_match_verify_failed` | AI 返回候选未通过服务端校验 |

### 14.7 交互状态

| 状态 | 页面表现 |
|:---|:---|
| 无应用资产 | 空状态 + 去采集资产按钮 |
| 正在分析应用资产 | 表格 loading + 顶部进度提示 |
| 分析完成但无可采集应用 | 空状态 + 展示分析依据 |
| 可检查应用存在 | 展示应用表 + 单应用检查按钮 |
| Agent 工具执行中 | 进度条 + 当前工具 + 次数 |
| 10 次工具调用失败 | 失败态 + 已尝试工具记录 |
| 匹配完成 | 命中结果表和失败表刷新 |

### 14.8 权限和审计

- 一键分析资产应用需要弱密码查看权限。
- 发起弱密码检查需要弱密码扫描权限。
- AI 一键生成字典需要字典管理权限。
- 查看完整命中密码需要 reveal 审批。
- 所有扫描、AI 字典生成、明文查看都写审计。

## 15. 安全影响

### 15.1 敏感数据保护

- Agent、server、api-server 日志禁止打印账号密码原值、hash、salt、token。
- `CredentialRecord.credential_value` 只在工具响应、匹配内存和必要的短期任务上下文中存在。
- 入库默认保存脱敏密码；完整命中密码如需保存，必须加密保存并受 reveal 审批控制。
- LLM 匹配任务必须受策略控制，记录 batch_id、模型、调用人、任务 ID 和摘要，不记录完整 prompt 到普通日志。

### 15.2 LLM 使用边界

- 推荐使用私有化或内网受控大模型处理加密密码材料和字典。
- 如果使用公网模型，默认关闭 `encrypted_password_llm_match`，除非管理员显式开启并接受风险。
- 对可校验算法，LLM 返回结果必须经过服务端二次校验。
- 对不可校验算法，结果只能是 AI 推断待确认。

### 15.3 权限控制

- 弱密码扫描属于高风险工具，智能体模式下需要审批。
- 查看完整命中密码需要二次审批。
- 读文件权限不足时不尝试绕过权限，只提示 Agent 权限或应用文件权限问题。

## 16. 配置项

```env
WEAK_PASSWORD_ENABLED=true
WEAK_PASSWORD_MAX_FILE_BYTES=1048576
WEAK_PASSWORD_MAX_RECORDS_PER_HOST=500
WEAK_PASSWORD_MAX_AGENT_TOOL_CALLS_PER_APP=10
WEAK_PASSWORD_FORBID_FIND=true
WEAK_PASSWORD_FORBID_RECURSIVE_SEARCH=true
WEAK_PASSWORD_DEFAULT_DICTIONARY_SIZE=1000
WEAK_PASSWORD_AI_DICTIONARY_GENERATE=true
WEAK_PASSWORD_AI_DICTIONARY_MAX_SIZE=1000
WEAK_PASSWORD_LLM_APP_ANALYSIS=true
WEAK_PASSWORD_LLM_REPAIR=true
WEAK_PASSWORD_LLM_ENCRYPTED_MATCH=true
WEAK_PASSWORD_LLM_MATCH_BATCH_SIZE=200
WEAK_PASSWORD_REQUIRE_SERVER_VERIFY=true
WEAK_PASSWORD_REVEAL_APPROVAL_REQUIRED=true
```

## 17. 测试用例设计

### 17.1 Agent 单元测试

- `CollectCredentials` 能解析 `/etc/shadow` 样例并输出账号、hash、salt、算法提示。
- `CollectCredentials` 能解析 Redis `requirepass`。
- `CollectCredentials` 能解析 MySQL ini 的账号和密码。
- `CollectCredentials` 对权限不足返回 `permission_denied`。
- `CollectCredentials` 对字段不存在返回 `field_not_found`。
- `ListConfigDir` 不允许递归。
- 工具层拒绝包含 `find` 或 shell 命令的参数。
- Agent 日志不包含 `credential_value`。

### 17.2 api-server 单元测试

- `WeakPasswordAssetPlanner` 能把资产转换为 LLM 输入。
- LLM 输出非法路径时服务端拒绝生成计划。
- 明文密码直接字典命中。
- 混合候选命中。
- 模糊候选命中。
- hash 命中必须经过 verifier 校验。
- verifier 失败时丢弃 LLM 命中。
- 采集失败时最多允许大模型调用 10 次 Agent 辅助工具。
- 无应用资产时返回 `no_application_assets`，不创建弱密码检查任务。
- 默认 1000 弱密码字典可被任务引用。
- AI 生成字典会执行去重、数量上限和审计记录。

### 17.3 server 转发测试

- Agent 在线时 `ExecuteTool` 正确转发。
- Agent 不在线时返回 `agent not connected`。
- `CallbackClient` 为空时返回 callback 不可用。

### 17.4 智能体工具测试

- 自然语言能创建弱密码扫描任务。
- 高风险扫描触发审批。
- 查询结果时能解释明文命中和 hash/AI 命中的区别。
- reveal 完整密码必须经过审批。

### 17.5 前端测试

- 应用资产分析页无资产时展示“先采集资产”提示和跳转按钮。
- 一键分析资产应用只展示应用资产中的可疑应用。
- 用户能从单个应用行发起弱密码检查。
- 任务进度条能展示阶段、百分比、当前应用和 Agent 工具调用次数。
- 大模型调用 10 次 Agent 工具仍未获取配置文件时展示 `config_discovery_failed`。
- 字典管理页展示默认 1000 弱密码字典摘要。
- AI 一键生成字典能配置应用、关键词、数量、规则并保存。
- 创建任务页面能选择字典、混合、模糊、AI 匹配策略。
- 结果表能显示 confirmed 与 AI 推断待确认状态。
- 采集失败列表能显示失败原因和 AI 修复过程。
- reveal 操作能触发审批并记录审计。

## 18. 验收标准

- 可以从智能体模式发起弱密码检测任务。
- 服务端能基于资产调用 LLM 分析可采集应用。
- 前端能一键分析应用资产，且只分析资产采集中的应用资产。
- 如果没有应用资产，前端提示用户先采集资产。
- 前端能列出所有可能存在密码的应用，并支持按单个应用发起弱密码检查。
- server 能通过现有 `ExecuteTool` 把 `WeakPassword.CollectCredentials` 下发给指定 Agent。
- Agent 能按指定文件和字段采集账号密码材料并标准化输出。
- 任务详情能展示获取配置、AI 修复定位、密码匹配和结果入库进度条。
- 大模型累计调用 10 次 Agent 辅助工具仍未获取配置文件时，任务失败并展示错误。
- 系统内置 1000 条默认弱密码字典。
- 前端提供 AI 一键生成字典能力，并支持保存为任务字典或自定义字典。
- 明文密码能在服务端直接字典匹配。
- 加密密码/hash 能构造 LLM 匹配任务，并明确标注字典、加密密码和应用类型。
- 混合和模糊匹配能产生候选并完成匹配。
- 读文件失败时能调用受控辅助工具修复路径或字段。
- 任意流程都不能调用 `find`、全盘递归搜索或任意 shell。
- 页面能展示命中弱密码、匹配方式、AI 解释、失败原因和整改建议。

## 19. 分期落地建议

### V6.1-alpha

- 实现 Agent `WeakPassword.CollectCredentials`。
- 支持 Linux shadow、Redis、MySQL 配置解析。
- 服务端实现明文直接匹配。
- server 复用现有工具下发链路。

### V6.1-beta

- 接入 LLM 资产分析生成采集计划。
- 接入 LLM 加密密码/hash 匹配任务。
- 实现混合和模糊候选生成。
- 实现采集失败修复循环。

### V6.1-rc

- 完成前端页面。
- 完成智能体工具。
- 完成 reveal 审批和审计。
- 完成安全日志检查。

### V6.1

- 全链路验收。
- 离线部署包更新。
- README 和 release 文档更新。

## 20. 风险与回滚

### 20.1 风险

- LLM 对 hash 计算或专有加密判断可能不准确。
- 采集配置文件可能遇到权限不足。
- 大字典和模糊候选会增加 LLM 和服务端计算成本。
- 如果策略配置不当，可能把敏感材料发送到不可信模型。

### 20.2 缓解

- 默认要求服务端 verifier 二次校验。
- 使用私有化或内网受控模型作为推荐部署。
- 限制 LLM 批次大小、候选数量和修复轮次。
- 所有辅助工具受 allowlist 和非递归限制。
- 权限不足只提示整改，不绕过权限。

### 20.3 回滚

- 关闭 `WEAK_PASSWORD_ENABLED` 可隐藏入口并停止新任务。
- 关闭 `WEAK_PASSWORD_LLM_ENCRYPTED_MATCH` 可禁用加密密码 LLM 匹配。
- 关闭 `WEAK_PASSWORD_LLM_REPAIR` 可禁用 AI 修复循环。
- Agent 未升级时服务端不下发弱密码工具，任务主机标记为 `agent_capability_missing`。
