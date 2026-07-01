# Aegis V6.1 弱密码检测 Agent 程序设计

## 1. 文档目标

本文定义 V6.1 弱密码检测在 Agent 侧的程序设计，包括工具入口、包结构、参数模型、文件读取策略、解析器、辅助定位工具、安全限制、日志和测试。Agent 只负责按 api-server 下发的计划读取文件和字段并标准化输出，不做服务端业务编排。

## 2. 设计原则

1. Agent 实现标准弱密码工具，不执行任意 shell。
2. Agent 只读取服务端计划允许的路径。
3. Agent 禁止 `find`、`locate`、递归全盘搜索和任意 shell。
4. Agent 返回账号、明文密码、hash、salt、auth string 等标准 `CredentialRecord`。
5. Agent 日志禁止打印密码、hash、salt、token。
6. 读取失败返回结构化错误，不自行绕过权限。

## 3. 推荐目录

```text
agent/internal/weakpass/
  collector.go
  types.go
  policy.go
  parsers.go
  parser_shadow.go
  parser_ini.go
  parser_yaml.go
  parser_json.go
  parser_properties.go
  parser_line_kv.go
  parser_htpasswd.go
  tools_probe.go
  tools_config.go
  redactor.go
  collector_test.go

agent/internal/tools/tool_manager.go
```

## 4. Agent 工具列表

| 工具名 | 用途 | 返回 |
|:---|:---|:---|
| `WeakPassword.CollectCredentials` | 按计划读取账号密码字段 | `CredentialCollectionResult` |
| `WeakPassword.ProbePath` | 检查指定路径是否存在、类型、大小、权限 | `PathProbeResult` |
| `WeakPassword.ListConfigDir` | 非递归列出指定配置目录下的文件 | `ConfigDirListResult` |
| `WeakPassword.ReadConfigSlice` | 读取指定文件小片段 | `ConfigSliceResult` |
| `WeakPassword.ServiceUnitInspect` | 读取指定 systemd service 的启动信息 | `ServiceUnitInspectResult` |
| `WeakPassword.ProcessConfigHints` | 根据指定 pid 获取启动参数、cwd、有限配置文件提示 | `ProcessConfigHintsResult` |
| `WeakPassword.PurgeCredentialCache` | 清理本任务临时缓存 | `PurgeResult` |

## 5. ToolManager 接入

在 `agent/internal/tools/tool_manager.go` 增加分支：

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

`agent/internal/client.HandleToolCall` 保持现有模式：解析 `ToolRequest.ParamsJson`，调用 `toolManager.Execute`，把结果序列化为 `ToolResponse.ResultJson`。

## 6. 数据结构

### 6.1 CollectCredentials 请求

```go
type CredentialCollectionRequest struct {
    TaskID           string                    `json:"task_id"`
    PlanID           string                    `json:"plan_id"`
    HostID           string                    `json:"host_id"`
    Applications     []ApplicationCollectPlan  `json:"applications"`
    CollectionPolicy CollectionPolicy          `json:"collection_policy"`
}

type ApplicationCollectPlan struct {
    Application  string              `json:"application"`
    AssetID      string              `json:"asset_id"`
    ProfileID    string              `json:"profile_id"`
    Paths        []string            `json:"paths"`
    Extractors   []CredentialExtractor `json:"extractors"`
}

type CollectionPolicy struct {
    MaxFileBytes         int64 `json:"max_file_bytes"`
    MaxRecords           int   `json:"max_records"`
    RedactContextValues  bool  `json:"redact_context_values"`
    ForbidFindCommand    bool  `json:"forbid_find_command"`
    ForbidRecursiveSearch bool `json:"forbid_recursive_search"`
}
```

### 6.2 CredentialRecord 输出

```go
type CredentialRecord struct {
    RecordID        string  `json:"record_id"`
    Application     string  `json:"application"`
    AssetID         string  `json:"asset_id"`
    SourcePath      string  `json:"source_path"`
    SourceKind      string  `json:"source_kind"`
    Account         string  `json:"account"`
    CredentialType  string  `json:"credential_type"`
    CredentialValue string  `json:"credential_value"`
    Salt            string  `json:"salt"`
    AlgorithmHint   string  `json:"algorithm_hint"`
    FieldPath       string  `json:"field_path"`
    Parser          string  `json:"parser"`
    Confidence      float64 `json:"confidence"`
}
```

`credential_type` 可选值：

| 值 | 说明 |
|:---|:---|
| `plaintext` | 明文密码 |
| `hash` | 无 salt hash |
| `salted_hash` | 带 salt hash |
| `encrypted_blob` | 应用加密 blob |
| `auth_string` | 应用认证字符串 |
| `unknown` | 未知格式 |

### 6.3 错误输出

```go
type CredentialCollectionError struct {
    Application             string   `json:"application"`
    SourcePath              string   `json:"source_path"`
    ErrorCode               string   `json:"error_code"`
    Message                 string   `json:"message"`
    Retryable               bool     `json:"retryable"`
    SuggestedAuxiliaryTools []string `json:"suggested_auxiliary_tools"`
}
```

## 7. CollectCredentials 流程

```text
解析请求
  ↓
校验 policy 和路径
  ↓
遍历应用计划
  ↓
遍历路径和 extractor
  ↓
读取文件
  ↓
按 parser 提取账号和密码字段
  ↓
识别凭据类型和算法提示
  ↓
生成 CredentialRecord 或 CredentialCollectionError
  ↓
返回 CredentialCollectionResult
```

执行规则：

1. 校验 `task_id`、`plan_id`、`host_id` 不为空。
2. 校验 `forbid_find_command=true`。
3. 校验 `forbid_recursive_search=true`。
4. 校验 path 不包含通配符和命令字符。
5. 调用 `os.Stat` 获取文件信息。
6. 文件超过 `max_file_bytes` 返回 `file_too_large`。
7. 读取文件内容并按 parser 解析。
8. 单路径失败不影响其它路径。
9. 超过 `max_records` 后停止采集并返回 `record_limit_reached`。

## 8. Parser 设计

### 8.1 shadow parser

输入：

- `/etc/passwd`
- `/etc/shadow`

输出：

- `account`
- `credential_type=salted_hash`
- `credential_value=完整 shadow 密码字段`
- `salt`
- `algorithm_hint`

算法识别：

| 前缀 | 算法 |
|:---|:---|
| `$1$` | md5-crypt |
| `$2a$/$2b$/$2y$` | bcrypt |
| `$5$` | sha256-crypt |
| `$6$` | sha512-crypt |
| `$y$` | yescrypt |

特殊值：

| 值 | 处理 |
|:---|:---|
| 空字段 | 输出 `empty_password` finding 所需材料 |
| `!` 或 `*` 前缀 | locked，不生成匹配材料 |
| 未识别 | `unsupported_hash_algorithm` |

### 8.2 ini parser

支持 MySQL、PostgreSQL、应用连接配置中的 section/key。

extractor：

```json
{
  "type": "ini",
  "section": "client",
  "account_selector": "user",
  "password_selector": "password",
  "format_hint": "plaintext"
}
```

### 8.3 yaml/json parser

使用结构化 parser，不使用字符串 grep。

支持 selector：

- `database.username`
- `database.password`
- `auth.token`
- `mcp.servers[0].env.API_KEY`

### 8.4 line_key_value parser

支持 Redis、properties 类文件：

```text
requirepass password
password = password
```

### 8.5 htpasswd parser

解析：

```text
user:hash
```

输出：

- `account=user`
- `credential_type=hash` 或 `salted_hash`
- `algorithm_hint=bcrypt/apr1/sha1/crypt`

## 9. 路径和安全策略

### 9.1 允许路径来源

Agent 只接受 api-server 下发的明确路径：

- 应用资产中的配置路径。
- Profile 默认路径。
- 受控辅助工具返回后由 api-server 二次下发的路径。

### 9.2 禁止行为

- 禁止 `find`。
- 禁止 `locate`。
- 禁止递归扫描。
- 禁止任意 shell。
- 禁止读取未授权路径。
- 禁止读取文件全文并回传给 LLM。
- 禁止读取 `/proc/*/environ` 全量内容并回传。

### 9.3 路径校验

拒绝：

- 包含 `..` 越权。
- 包含 shell metacharacters：`;`、`|`、`&`、`` ` ``、`$(`。
- 包含通配符：`*`、`?`、`[]`。
- 目录路径传给文件读取工具。

## 10. 辅助定位工具

### 10.1 `ProbePath`

参数：

```json
{
  "path": "/etc/redis/redis.conf"
}
```

返回：

```json
{
  "path": "/etc/redis/redis.conf",
  "exists": true,
  "type": "file",
  "size": 2048,
  "mode": "0640",
  "owner": "redis"
}
```

### 10.2 `ListConfigDir`

只允许非递归列目录。最多返回 200 项。

参数：

```json
{
  "dir": "/etc/redis",
  "suffix_allowlist": [
    ".conf",
    ".cnf",
    ".yml",
    ".yaml",
    ".json",
    ".properties"
  ],
  "max_entries": 200
}
```

### 10.3 `ReadConfigSlice`

最多读取 16KB 或指定行范围。敏感值可按策略脱敏。

### 10.4 `ServiceUnitInspect`

读取指定 service 的：

- `ExecStart`
- `EnvironmentFile`
- `WorkingDirectory`
- `User`

不执行 systemctl 命令，优先读取 systemd unit 文件。

### 10.5 `ProcessConfigHints`

只允许查询资产中已知 pid：

- `cmdline`
- `cwd`
- 有限 open config files

不返回环境变量全量内容。

## 11. 日志要求

允许记录：

- `task_id`
- `plan_id`
- `host_id`
- `application`
- `source_path`
- `parser`
- `record_count`
- `error_code`
- `tool_name`

禁止记录：

- `credential_value`
- `salt`
- 密码明文。
- hash 原文。
- token/API key。

## 12. 测试用例

- `CollectCredentials` 能解析 `/etc/shadow` 样例并输出账号、hash、salt、算法提示。
- `CollectCredentials` 能解析 Redis `requirepass`。
- `CollectCredentials` 能解析 MySQL ini 的账号和密码。
- `CollectCredentials` 对权限不足返回 `permission_denied`。
- `CollectCredentials` 对字段不存在返回 `field_not_found`。
- `ListConfigDir` 不允许递归。
- `ReadConfigSlice` 限制 16KB。
- 工具层拒绝包含 `find`、shell、通配符和递归路径的参数。
- Agent 日志不包含 `credential_value`、salt、hash 原文。
- 单路径失败不影响其它路径。

## 13. 验收标准

- Agent 支持 `WeakPassword.CollectCredentials`。
- Agent 支持 5 个受控辅助定位工具。
- Agent 能按指定文件和字段采集账号密码材料并标准化输出。
- Agent 禁止 `find`、全盘递归搜索和任意 shell。
- Agent 读文件失败时返回结构化错误。
- Agent 日志不泄露密码、hash、salt、token。
