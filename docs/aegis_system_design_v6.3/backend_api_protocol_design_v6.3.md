# Aegis V6.3 后端、API 与通信协议设计

## 1. 组件变更总览

| 组件 | 新增职责 |
| --- | --- |
| proto | 会话 item/batch/source usage 和 ACK 契约 |
| Agent | `agentsession` 采集、脱敏、cursor、spool、batch |
| Server | 专用 gRPC ingest、校验、Kafka producer |
| DC | 专用 consumer、投影、Token 估算、完整性和行为关联 |
| api-server | 会话查询、规则引擎、durable AI workers、marking、设置 |
| Frontend | 智能体会话感知页面 |

## 2. Proto

新增独立的 `proto/agent_session_comm.proto`（不改动现有 `agent_comm.proto` 的消息和接口）
并在 Server/Agent/api-server 生成对应代码：

```protobuf
message AgentSessionSourceUsage {
  int64 input_tokens = 1;
  int64 output_tokens = 2;
  int64 cache_creation_input_tokens = 3;
  int64 cache_read_input_tokens = 4;
  string coverage = 5; // none|partial|complete
}

message AgentSessionItem {
  string item_id = 1;
  string source_message_id = 2;
  string source_part_id = 3;
  string source_revision = 4;
  uint64 source_sequence = 5;
  string turn_id = 6;
  string parent_item_id = 7;
  string item_type = 8;
  string role = 9;
  int64 occurred_at_unix_nano = 10;
  bytes normalized_json = 11; // only redacted/metadata content
  string content_digest = 12;
  string source_digest = 13;
  string previous_item_digest = 14;
  AgentSessionSourceUsage source_usage = 15;
}

message AgentSessionBatchRequest {
  string schema = 1; // aegis.agent_session_batch.v1
  string batch_id = 2;
  string host_id = 3;
  string agent_type = 4; // claude-code|codex
  int64 source_subject_uid = 5;
  string source_storage_namespace_hash = 6;
  string source_session_id = 7;
  string source_version = 8;
  string source_mode = 9; // static_scan|static_backfill
  string source_attestation = 10; // versioned_static_parser
  uint64 first_sequence = 11;
  uint64 last_sequence = 12;
  repeated AgentSessionItem items = 13;
  string compression = 14;
  string batch_digest = 15;
  string collection_coverage = 16;
  bytes session_metadata_json = 17;
}

message AgentSessionBatchResponse {
  bool success = 1;
  uint64 accepted_through_sequence = 2;
  string error_code = 3;
  string message = 4;
  bool retryable = 5;
}
```

`AgentService` 新增：

```protobuf
rpc ReportAgentSessionBatch(AgentSessionBatchRequest)
    returns (AgentSessionBatchResponse);
```

### 2.1 大小与校验

- 单 item `normalized_json` 最大 256 KiB；
- 单 batch 最大 1 MiB 或 100 items；
- batch 只能包含一个 source session；
- `first_sequence <= item.sequence <= last_sequence` 且单调不降；
- digest 使用 canonical JSON/明确 protobuf 字段顺序定义，不能 hash 非确定性 map；
- source usage 必须非负且每项不超过配置上限；异常值拒绝或置为 unavailable；
- `normalized_json` 只允许 schema 字段，不允许 raw transcript envelope。

### 2.2 ACK

Server 只在 Kafka 写入成功后返回 success。Agent 只删除
`accepted_through_sequence` 之前且 digest 一致的 spool record。

错误：

```text
agent_session_ingest_disabled
agent_session_schema_unsupported
agent_session_source_unsupported
agent_session_batch_too_large
agent_session_sequence_invalid
agent_session_digest_invalid
agent_session_host_mismatch
agent_session_kafka_unavailable
```

## 3. Server

新增建议目录：

```text
server/internal/grpc_server/agent_session_ingest.go
server/internal/queue/agent_session_producer.go
```

处理顺序：

1. 检查 `AGENT_SESSION_INGEST_ENABLED`；
2. 从已认证 Agent 连接上下文获得 host ID，不信任 request 自报 host；
3. 校验 schema、source、UID、batch limits、sequence、digest；
4. 清理 transport metadata 中可能包含正文的错误；
5. Kafka key：
   `host_id:source_uid:agent_type:storage_namespace_hash:source_session_id`；
6. 写 `aegis.agent.sessions.v1`；
7. 成功后 ACK。

Server 不做：正文解析、Token 估算、规则匹配、AI 调用或数据库正文写入。

Kafka 建议：

```text
topic = aegis.agent.sessions.v1
compression = zstd
retention = 24h..72h
producer = server only
consumer = dc session consumer（api-server 可在开发/单体部署中以独立 group 启用兼容投影）
```

专用 topic 不接通用日志 sink、告警 connector 或现有 runtime event consumer。

## 4. DC

新增建议目录：

```text
dc/internal/sessionaudit/
  consumer.go
  validator.go
  projector.go
  token_estimator.go
  completeness.go
  behavior_linker.go
  processing_watermark.go
```

### 4.1 消费事务

单 batch 事务内：

1. upsert session；
2. 按 source identity/revision/digest upsert items；
3. upsert tool call lifecycle；
4. 使用 `aegis_visible_v1` 计算新/变更 item 的可见 Token 估算；
5. 重算 session 增量计数和 source usage；
6. 检测 gap、digest conflict、coverage；
7. 建立/更新 conversation -> behavior 的边；
8. 对新 sequence range 创建或合并 pending rule run；
9. commit 后发布 metadata notification。

Kafka offset 只在数据库事务成功后提交。重放依赖唯一键和 digest 幂等。

### 4.2 Token 估算归属

DC 是页面“可见内容 Token（估算）”的权威计算方。Agent 不上报该值，避免不同
Agent 版本产生不一致算法。api-server chunker读取持久化估算，并在序列化后用
相同 golden fixture 算法做 preflight；模型实际 usage 另存。

## 5. 内置规则目录 API

配置检测和会话感知沿用事件感知的规则目录约定：规则由服务端维护为内置、版本化、
只读清单，前端只负责查询和展示，不允许通过页面修改规则正文或默认动作。每条规则
包含 `rule_key`、`rule_version`、名称/说明、分类、执行引擎、默认严重级别、默认/推荐
动作、启用状态和 `sha256` 摘要，便于审计和前后端版本校验。

```text
GET /api/v1/agent-guard/configuration-rules
GET /api/v1/agent-guard/session-awareness/rules
GET /api/v1/agent-guard/session-awareness/sessions/:id/rule-hits
GET /api/v1/agent-guard/session-awareness/sessions/:id/ai-analysis
```

两个接口都返回 `{ items, total }`，支持 `keyword` 过滤及 `page/page_size` 分页。配置
规则使用 `AGC-*` 键并由 `api_config_static` 规则引擎执行；会话规则使用 `ASR-*` 键，
由 `api_session_static` 对脱敏会话 item 做确定性匹配。规则命中只代表模式证据，不代表
攻击成功；规则分析也不调用 LLM。

`rule-hits` 返回 `item_id/item_sequence`，`ai-analysis` 返回每个分析分段覆盖的
`item_sequences`。前端据此把策略命中和 AI 结论渲染到会话时间线的具体消息旁边，不能
只展示一个没有位置关系的会话级摘要。

### 4.3 行为关联

关联强度：

```text
confirmed:
  exact behavior_session/external session/correlation token/tool call ID/PID start_ticks

probable:
  same host + instance + source UID + cwd/project hash + bounded time window

ambiguous/unattributed:
  multiple candidates or insufficient identity
```

只有 confirmed 行为可显示“执行已证实”；probable 明确显示“可能关联”。

## 5. api-server 模块

新增建议目录：

```text
api-server/internal/model/agent_session_awareness.go
api-server/internal/repository/agent_session_awareness_repo.go
api-server/internal/service/agent_session_query_service.go
api-server/internal/service/agent_session_rule_service.go
api-server/internal/service/agent_session_ai_service.go
api-server/internal/service/agent_session_chunker.go
api-server/internal/service/agent_session_risk_service.go
api-server/internal/service/agent_session_settings_service.go
api-server/internal/api/handler/agent_session_awareness_handler.go
```

### 5.1 Durable workers

现有 finding AI service 使用进程内 channel。V6.3 会话 AI 可能跨多个 chunk 且耗时
更长，run/chunk 数据库行必须是 queue source of truth：

- worker 使用 `FOR UPDATE SKIP LOCKED` 领取 pending row；
- `lease_owner/lease_expires_at/attempt` 防并发和崩溃遗留；
- 进程内 channel 只能用于 wake-up，不能代表任务已持久化；
- 启动时恢复 pending 和过期 running；
- 幂等键阻止同一 input/prompt/model 重复 run；
- 停机停止领取新 chunk，让正在执行的请求在 grace period 内完成或租约过期恢复。

规则 run 同样持久化，但不需要外部模型；按 session 保序、跨 session 并发。

### 5.2 模型客户端

复用当前 active LLM 配置和 `llm.Client`，但新建 no-tool client facade：

- 只暴露 messages + response format；
- 不接受 tool definitions；
- timeout/retry/usage 通过结构化 result 返回；
- API key 解密错误只记安全 error code；
- provider 原始响应不进入日志。

## 6. HTTP API

根路径：

```text
/api/v1/agent-guard/session-awareness
```

### 6.1 总览与查询

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/overview` | read | KPI 和更新时间 |
| GET | `/coverage` | read | host/source/version 覆盖矩阵 |
| GET | `/sessions` | read | 服务端分页列表 |
| POST | `/agents/:host_id/collect?agent_type=claude-code\|codex` | read | 向指定 Agent 下发一次静态会话采集 |
| GET | `/sessions/:id` | read | metadata、最新规则/AI/综合风险 |
| GET | `/sessions/:id/items` | content:read | cursor 分页 item |
| GET | `/sessions/:id/tool-calls` | content:read | tool lifecycle 和行为关联 |
| GET | `/sessions/:id/rule-analyses` | rule:read | rule run 历史 |
| GET | `/sessions/:id/rule-hits` | rule:read | hit 分页和定位 |
| GET | `/sessions/:id/ai-analyses` | ai:read | AI run 历史 |
| GET | `/ai-analyses/:run_id/chunks` | ai:read | chunk 状态和 usage，不返回模型原始响应 |
| GET | `/sessions/:id/related-behaviors` | read | 关联行为/finding |
| GET | `/sessions/:id/collection-status` | read | cursor/gap/parser/coverage |
| GET | `/rules` | rule:read | 内置会话规则目录 |

`/sessions` 支持：

```text
host_ids[]
agent_types[]
source_uids[]
status
coverage
content_mode
rule_status
rule_keys[]
ai_status
ai_verdict
severity
token_min/token_max
time_from/time_to
sort=last_source_at|visible_token_estimate|severity
order=asc|desc
page/page_size
```

V6.3 不提供会话正文全文 keyword 搜索。

### 6.2 操作

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/sessions/:id/ai-analyses` | ai:run | 手工创建 final AI run |
| POST | `/markings/:id/confirm` | marking:handle | 确认风险 |
| POST | `/markings/:id/dismiss` | marking:handle | 驳回 |
| POST | `/markings/:id/false-positive` | marking:handle | 标记误报 |
| GET | `/settings?host_id=` | read | 读取采集/分析设置 |
| PUT | `/settings` | settings:write | 保存并下发设置 |

手工 AI 请求体：

```json
{
  "reason": "ticket SEC-1234",
  "scope": "final",
  "force_new_run": false
}
```

`reason` 只用于审计，限制长度并脱敏；不能作为模型分析指令。

### 6.3 响应保护

- 正文 API `Cache-Control: no-store, private`；
- 所有 content endpoint 做服务端 permission check，不能只靠前端隐藏；
- excerpt 由服务端产生；
- error response 只含 safe code/message；
- 列表不返回 content、tool args/result 或 AI raw response；
- page size 默认 20/50，最大 100；item cursor 默认 50，最大 200。

## 7. WebSocket

事件：

```text
agent_session_awareness.created
agent_session_awareness.updated
agent_session_awareness.coverage_updated
agent_session_awareness.rule_updated
agent_session_awareness.ai_updated
agent_session_awareness.marking_updated
```

payload 只含：session/run/marking ID、status、severity、verdict、计数、sequence、
updated_at。前端收到后按需 REST refresh。

## 8. 配置

### 8.1 Feature flags

```text
agent:
  agent_session.enabled=false
  agent_session.static_scan_enabled=false
  agent_session.static_backfill_enabled=false

server:
  AGENT_SESSION_INGEST_ENABLED=false
  AGENT_SESSION_KAFKA_TOPIC=aegis.agent.sessions.v1

dc:
  AGENT_SESSION_PROJECTION_ENABLED=false
  AGENT_SESSION_BEHAVIOR_LINK_ENABLED=false
  AGENT_SESSION_RULE_REQUEST_ENABLED=false

api-server:
  AGENT_SESSION_AWARENESS_ENABLED=false
  AGENT_SESSION_RULE_ANALYSIS_ENABLED=false
  AGENT_SESSION_AI_ANALYSIS_ENABLED=false
  AGENT_SESSION_AI_AUTO_TRIGGER_ENABLED=false
```

子开关必须受父开关约束；例如 awareness off 时 AI auto trigger 不能单独生效。

### 8.2 Settings

使用独立 config type：

```text
agent_session_awareness_settings.v1
agent_session_collection_bundle.v1
```

正文静态扫描不复用现有 `agent_guard_runtime_settings.v1.session_hook_enabled`；后者
仍只表示 V6.2 运行时 session/tool Hook 能力，开启或失败都不影响 V6.3 文件扫描。

## 9. 错误码

```text
agent_session_awareness_disabled
agent_session_not_found
agent_session_content_forbidden
agent_session_content_unavailable
agent_session_source_unsupported
agent_session_collection_partial
agent_session_rule_disabled
agent_session_rule_run_failed
agent_session_ai_disabled
agent_session_ai_queue_full
agent_session_ai_context_budget_invalid
agent_session_ai_invalid_output
agent_session_ai_evidence_mismatch
agent_session_ai_provider_unavailable
agent_session_marking_conflict
agent_session_settings_invalid
agent_session_settings_dispatch_failed
```

## 10. 后端日志

沿用 Zap 的稳定 snake_case event name。在最能说明结果的一层记录一次：

```text
server: agent_session_batch_rejected / agent_session_batch_published
dc:     agent_session_batch_projected / agent_session_projection_failed
api:    agent_session_rule_analysis_completed
api:    agent_session_ai_run_queued / completed / failed
api:    agent_session_settings_dispatched
```

公共字段：`request_id`、`host_id`、`session_id`、`run_id`、`chunk_id`、sequence
range、count、bytes、digest、status、error_code、retry、latency。正文、source path、
source session raw ID、tool payload、AI response 和 credential 一律禁止。
