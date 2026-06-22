# Aegis V6.1 弱密码检测 api-server 设计

## 1. 文档目标

本文定义 V6.1 弱密码检测在 `api-server` 中的服务、接口、任务编排、LLM 调用、Agent 工具下发、字典匹配、错误处理和测试方案。server 仍只负责工具转发，Agent 负责采集，api-server 是业务编排中心。

## 2. 推荐目录

```text
api-server/internal/api/handler/weak_password_handler.go
api-server/internal/model/weak_password.go
api-server/internal/repository/weak_password_repository.go
api-server/internal/service/weak_password_service.go
api-server/internal/service/weak_password_asset_planner.go
api-server/internal/service/weak_password_collection_repair.go
api-server/internal/service/weak_password_matcher.go
api-server/internal/service/weak_password_dictionary_service.go
api-server/internal/assistant/tools/weak_password_tools.go
api-server/internal/llm/weak_password_prompts.go
```

## 3. 组件职责

| 组件 | 职责 |
|:---|:---|
| `WeakPasswordHandler` | HTTP API 参数解析、权限校验、响应封装 |
| `WeakPasswordService` | 任务创建、状态流转、异步执行 |
| `WeakPasswordAssetPlanner` | 查询应用资产，调用 LLM 分析可采集应用 |
| `WeakPasswordCollectionRepairService` | 采集失败后调用 LLM 决策受控 Agent 工具 |
| `WeakPasswordMatcher` | 明文匹配、LLM hash 匹配、服务端二次校验 |
| `WeakPasswordDictionaryService` | 默认 1000 字典、上传字典、AI 生成字典 |
| `WeakPasswordRepository` | 读写弱密码相关表 |
| `Assistant WeakPassword Tools` | 智能体模式下发起扫描、查询、解释 |

## 4. HTTP API

### 4.1 一键分析应用资产

`POST /api/v1/weak-password/asset-applications/analyze`

请求：

```json
{
  "scope": {
    "host_ids": [],
    "host_group_ids": [],
    "application_types": [
      "redis",
      "mysql",
      "ai_agent"
    ],
    "online_agents_only": true
  }
}
```

响应：

```json
{
  "analysis_id": "analysis-uuid",
  "status": "completed",
  "application_asset_count": 12,
  "candidate_count": 5,
  "candidates": [
    {
      "candidate_application_id": "candidate-uuid",
      "host_id": "host-uuid",
      "asset_id": "asset-uuid",
      "application_name": "redis",
      "application_type": "redis",
      "confidence": 0.96,
      "candidate_paths": [
        "/etc/redis/redis.conf"
      ],
      "credential_types": [
        "plaintext"
      ],
      "ai_reason": "redis application asset contains config path and auth-capable service"
    }
  ]
}
```

如果没有应用资产：

```json
{
  "analysis_id": "",
  "status": "failed",
  "error_code": "no_application_assets",
  "message": "当前范围没有应用资产，请先采集资产"
}
```

### 4.2 查询应用资产分析结果

`GET /api/v1/weak-password/asset-applications`

查询参数：

| 参数 | 说明 |
|:---|:---|
| `analysis_id` | 指定分析批次 |
| `host_id` | 主机过滤 |
| `application_type` | 应用类型 |
| `confidence` | 置信度过滤 |
| `page/page_size` | 分页 |

### 4.3 针对单个应用创建弱密码检查任务

`POST /api/v1/weak-password/tasks/by-application`

请求：

```json
{
  "candidate_application_id": "candidate-uuid",
  "dictionary_policy": {
    "use_default_1000": true,
    "dictionary_ids": [],
    "use_ai_generated": false,
    "hybrid": true,
    "fuzzy": true
  },
  "ai_policy": {
    "repair_collection_errors": true,
    "encrypted_password_llm_match": true,
    "max_agent_tool_calls_per_app": 10
  }
}
```

响应：

```json
{
  "task_id": "task-uuid",
  "scan_application_id": "scan-app-uuid",
  "status": "pending"
}
```

### 4.4 查询任务进度

`GET /api/v1/weak-password/tasks/:id/progress`

响应：

```json
{
  "task_id": "task-uuid",
  "status": "collecting_credentials",
  "progress": 42,
  "current_stage": "ai_repair_locator",
  "current_host_id": "host-uuid",
  "current_application": "redis",
  "agent_tool_call_count": 4,
  "max_agent_tool_calls": 10,
  "last_agent_tool": "WeakPassword.ServiceUnitInspect",
  "last_error_code": "file_not_found",
  "message": "正在根据 systemd unit 重新定位配置文件"
}
```

### 4.5 字典接口

| 方法 | 路径 | 说明 |
|:---|:---|:---|
| `GET` | `/api/v1/weak-password/dictionaries/default` | 查询默认 1000 字典摘要 |
| `GET` | `/api/v1/weak-password/dictionaries` | 查询字典列表 |
| `POST` | `/api/v1/weak-password/dictionaries` | 上传或保存字典 |
| `POST` | `/api/v1/weak-password/dictionaries/ai-generate` | AI 一键生成字典 |

AI 生成字典请求：

```json
{
  "target": "application",
  "application_type": "redis",
  "organization_keywords": [
    "aegis",
    "prod"
  ],
  "account_keywords": [
    "admin",
    "redis"
  ],
  "count": 200,
  "rules": [
    "append_year",
    "append_special_char",
    "capitalize",
    "leet_replace"
  ],
  "deduplicate_with_default": true
}
```

响应：

```json
{
  "dictionary_id": "dictionary-uuid",
  "entry_count": 200,
  "deduplicated_count": 38,
  "status": "generated"
}
```

## 5. 任务执行流程

### 5.1 一键分析应用资产

1. Handler 校验权限和 scope。
2. Service 查询 `host_application_assets` 等应用资产结果。
3. 如果没有应用资产，返回 `no_application_assets`，不创建弱密码检查任务。
4. Planner 构造脱敏资产上下文。
5. 调用 LLM 输出候选应用、路径、Profile、extractor。
6. 服务端校验 LLM 输出：
   - 只允许应用资产中的应用。
   - path 必须来自应用资产、Profile 默认路径或后续受控辅助工具。
   - extractor 必须在 allowlist。
7. 写入 `weak_password_asset_app_analyses` 和 `weak_password_candidate_applications`。
8. 返回候选应用列表。

### 5.2 单应用检查任务

1. 查询 `candidate_application_id`。
2. 创建 `weak_password_scan_tasks`、`weak_password_scan_hosts`、`weak_password_scan_applications`。
3. 生成 `CredentialCollectionPlan`。
4. 调用 serverClient.ExecuteTool：

```go
resp, err := serverClient.ExecuteTool(
    ctx,
    callID,
    hostID,
    "WeakPassword.CollectCredentials",
    string(argumentsJSON),
    180,
)
```

5. 记录 `weak_password_agent_tool_calls`。
6. 解析 Agent 返回的 `CredentialRecord[]`。
7. 如果读取失败，进入 AI 修复定位。
8. 匹配密码并写入 finding。

### 5.3 AI 修复定位

当 Agent 返回 `file_not_found`、`field_not_found`、`unsupported_format` 等可重试错误时：

1. 构造修复上下文。
2. LLM 只能从 allowlist 中选择一个工具：
   - `WeakPassword.ProbePath`
   - `WeakPassword.ListConfigDir`
   - `WeakPassword.ReadConfigSlice`
   - `WeakPassword.ServiceUnitInspect`
   - `WeakPassword.ProcessConfigHints`
3. api-server 校验工具和参数，拒绝 `find`、shell、递归扫描。
4. 调用 Agent 工具并记录次数。
5. 每个应用累计最多 10 次 Agent 辅助工具调用。
6. 如果 10 次仍未得到有效配置，写入 `config_discovery_failed`。

### 5.4 密码匹配

明文：

1. 加载默认 1000 字典、任务字典、上传字典和 AI 生成字典。
2. 生成混合候选和模糊候选。
3. 与 `credential_value` 直接比较。
4. 命中后写入 `weak_password_findings`。

hash 或加密材料：

1. 构造 `LLMPasswordMatchJob`。
2. 明确标注 `credential_block` 和 `dictionary_block`。
3. 调用 LLM 返回候选。
4. 对可支持算法执行服务端 verifier 校验。
5. 校验通过写入 `confirmed`；校验失败写入 `llm_match_verify_failed` 或丢弃候选。
6. 不可校验专有格式写入 `ai_inferred_needs_confirm`。

## 6. LLM Prompt 设计

### 6.1 应用资产分析 Prompt

输入只包含应用资产及其关联摘要：

```json
{
  "task": "analyze_collectable_credential_sources",
  "application_assets": [],
  "constraints": {
    "analyze_only_application_assets": true,
    "do_not_scan_filesystem": true,
    "output_json_only": true
  }
}
```

输出必须是 JSON：

```json
{
  "collectable_applications": [
    {
      "asset_id": "asset-uuid",
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
      "reason": "application asset contains redis config path"
    }
  ]
}
```

### 6.2 AI 字典生成 Prompt

输入：

```json
{
  "task": "generate_weak_password_dictionary",
  "application_type": "redis",
  "organization_keywords": [],
  "account_keywords": [],
  "count": 200,
  "rules": [],
  "output_json_only": true
}
```

输出：

```json
{
  "candidates": [
    {
      "candidate": "Redis@2026",
      "rule": "capitalize+append_special_char+append_year",
      "risk_level": "high"
    }
  ]
}
```

## 7. 权限和审计

| 操作 | 权限 | 审计内容 |
|:---|:---|:---|
| 一键分析应用资产 | weak_password:read | scope、候选数量、模型 |
| 创建检查任务 | weak_password:scan | 目标应用、字典策略、AI 策略 |
| AI 生成字典 | weak_password:dictionary:write | 提示词摘要、数量、模型 |
| reveal 完整密码 | weak_password:reveal + 审批 | finding、查看人、审批人、水印 |

## 8. 日志要求

允许记录：

- `task_id`
- `host_id`
- `asset_id`
- `scan_application_id`
- `call_id`
- `tool_name`
- `agent_tool_call_count`
- `error_code`
- `dictionary_id`
- `entry_count`

禁止记录：

- 密码明文。
- hash 原文。
- salt 原文。
- token/API key。
- 完整 LLM prompt。
- 完整字典明文列表。

## 9. 错误处理

| 场景 | 行为 |
|:---|:---|
| 无应用资产 | 返回 `no_application_assets` |
| Agent 离线 | 应用检查失败，错误码 `agent_not_connected` |
| LLM 输出非法路径 | 拒绝计划并记录 `invalid_llm_plan` |
| Agent 权限不足 | 记录 `permission_denied`，不尝试绕过 |
| 10 次工具调用仍失败 | 记录 `config_discovery_failed` |
| LLM 命中校验失败 | 记录 `llm_match_verify_failed` |

## 10. 测试用例

- 无应用资产时不创建检查任务。
- 一键分析只使用应用资产，不使用全盘扫描结果。
- LLM 输出非法路径被拒绝。
- 单应用检查能生成任务、主机明细和应用明细。
- Agent 工具调用次数累计到 10 后停止。
- `config_discovery_failed` 能写入错误表并返回前端。
- 默认 1000 字典能被任务引用。
- AI 生成字典执行去重和数量限制。
- 明文密码直接匹配成功。
- hash 匹配必须走 LLM 候选和服务端 verifier。
- reveal 完整密码必须走审批。

## 11. 验收标准

- api-server 提供应用资产一键分析接口。
- api-server 能基于候选应用创建单应用弱密码检查任务。
- api-server 能通过 serverClient 下发 `WeakPassword.CollectCredentials`。
- api-server 能控制每个应用最多 10 次 Agent 辅助工具调用。
- api-server 能管理默认 1000 字典和 AI 生成字典。
- api-server 能将进度、错误和结果提供给前端。

