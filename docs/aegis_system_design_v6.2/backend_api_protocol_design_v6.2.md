# Aegis V6.2 后端、API 与通信协议设计

**版本**：6.2  
**日期**：2026-07-30  
**状态**：设计完成，待实施

## 1. 组件边界

V6.2 沿用当前 Aegis 服务职责：

| 组件 | V6.2 职责 |
| --- | --- |
| `api-server` | 策略/Profile/分析配置、查询、智能研判编排、人工处置、鉴权和审计 |
| `server` | Agent 在线连接、配置/命令转发、事件写 Kafka |
| `agent` | 本机识别、全行为采集、原子规则决策、拒绝、冻结和事件上报 |
| `dc` | Kafka 消费、规范化、资源分类、序列关联、Finding、告警和 WebSocket |
| `PostgreSQL` | 内置规则定义、策略、状态、实例、session、行为事件、Finding、分析和动作事实 |
| `Redis` | 可选的短期查询缓存/发布锁，不作为 Agent Guard 事实源 |
| `Kafka` | 复用 `aegis.security.events` 传递运行时事件 |

本地实时阻断链路不能调用 api-server/server/DC；这些服务不可用时，Agent 使用 last-known-good bundle。

## 2. api-server 模块

### 2.1 推荐目录

```text
api-server/internal/
├── api/handler/
│   └── agent_guard_handler.go
├── model/
│   └── agent_guard.go
├── repository/
│   ├── agent_guard_profile_repo.go
│   ├── agent_guard_policy_repo.go
│   ├── agent_behavior_rule_repo.go
│   ├── agent_guard_runtime_repo.go
│   ├── agent_behavior_event_repo.go
│   ├── agent_security_finding_repo.go
│   ├── agent_analysis_run_repo.go
│   └── agent_guard_action_repo.go
└── service/
    ├── agent_guard_policy_service.go
    ├── agent_guard_bundle_service.go
    ├── agent_guard_query_service.go
    ├── agent_panorama_query_service.go
    ├── agent_security_analysis_service.go
    └── agent_guard_action_service.go
```

### 2.2 服务职责

#### AgentGuardPolicyService

- 策略 CRUD。
- 采集分类、资源、原子/关联规则、例外、分析触发、动作和 freeze timeout 校验。
- host/host group/agent type 范围校验。
- draft/published/disabled 状态流转。
- 发布前生成规范化 compiled preview。
- 创建审计日志。

#### AgentGuardBundleService

- 查询所有 published policy 和 enabled profile。
- 按主机展开选择器。
- 根据 Agent capability 生成主机 bundle。
- 生成递增 `bundle_version` 和 SHA-256 digest。
- 通过现有 Server gRPC `SyncAgentConfig` 下发。
- 跟踪 pending/applied/failed/stale。
- Agent 重连时返回当前主机最新 bundle。

#### AgentGuardQueryService

- 概览统计。
- 实例、session、执行单元、行为事件、finding、analysis、动作和下发状态分页查询。
- 读取 `runtime_events` 原始证据与 `agent_behavior_events` 查询投影。
- 组装实例详情、行为时间线和 PID 主干行为全景树。
- 不根据缺失事件推断“安全”。

#### AgentSecurityAnalysisService

- 接收 DC 创建的 analysis request 或周期扫描待分析 finding。
- 按 finding、session、execution unit 和时间范围读取有界证据窗口。
- 对命令、路径、URL 和资源字段完成二次脱敏并标记为不可信 evidence。
- 调用现有 LLM client/worker queue，不向分析器暴露主机工具、策略写入或阻断能力。
- 校验固定 JSON Schema、枚举、数值范围和引用的 event ID。
- 保存 model/provider/prompt version、input/output digest、latency 和失败原因。
- 只更新 finding 的 analysis 投影，不改写原始行为事件和规则命中。
- AI-only 结论不能直接调用 `AgentGuardActionService`。

#### AgentGuardActionService

- freeze/resume/kill 请求鉴权。
- 校验实例/执行单元属于目标主机且状态适用。
- 创建 `agent_guard_actions(pending)`。
- 通过现有 BlockCommand 转发。
- 根据 Agent 返回和后续 action event 更新真实状态。
- 写 command audit/audit log。

## 3. HTTP API

路由组：

```text
/api/v1/agent-guard
```

### 3.1 概览与覆盖

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/overview` | 实例、覆盖、事件、阻断和降级统计 |
| GET | `/coverage` | 按主机/Agent/隔离类型查询覆盖能力 |
| GET | `/hosts/:host_id/status` | 主机 Agent Guard capability、bundle 和错误 |

`GET /overview` 响应：

```json
{
  "code": 0,
  "data": {
    "running_instances": 12,
    "execution_units": 28,
    "coverage": {
      "full_enforcement": 8,
      "monitor_only": 4,
      "no_isolation": 2,
      "remote_unobservable": 1,
      "degraded": 1
    },
    "behaviors_24h": {
      "process": 420,
      "file": 1200,
      "network": 98,
      "identity": 4,
      "isolation": 6,
      "denied": 4,
      "frozen": 1
    },
    "findings_24h": {
      "suspicious": 9,
      "malicious": 2,
      "analysis_pending": 1
    },
    "builtin_rule_hits_24h": {
      "AGB-BUILTIN-001": 8,
      "AGB-BUILTIN-002": 14,
      "AGB-BUILTIN-003": 32,
      "AGB-BUILTIN-004": 6,
      "AGB-BUILTIN-005": 1
    },
    "policy_hosts": {
      "applied": 20,
      "pending": 1,
      "failed": 2,
      "stale": 1
    }
  }
}
```

### 3.2 Profile

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/profiles` | 查询内置和自定义 Profile |
| GET | `/profiles/:id` | Profile 详情和支持隔离族 |
| POST | `/profiles/validate` | 校验 Profile JSON，不保存 |
| POST | `/profiles` | 新建自定义 Profile |
| PUT | `/profiles/:id` | 创建新 Profile version |
| POST | `/profiles/:id/enable` | 启用 |
| POST | `/profiles/:id/disable` | 禁用 |

内置 Profile 默认只读；升级随 Aegis 版本或受信 Profile bundle 进行。第一阶段可以只开放 GET，保留写 API 到后续阶段。

### 3.3 内置规则

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/rules` | 查询五个内置规则及当前策略覆盖、hit/finding 统计 |
| GET | `/rules/:rule_key` | 规则当前版本、参数 Schema、默认值和支持能力 |
| GET | `/rules/:rule_key/versions` | 历史版本 |
| POST | `/rules/:rule_key/preview` | 根据目标主机和覆盖参数预估命中、降级和影响 |

首批稳定 rule key：

```text
AGB-BUILTIN-001  操作敏感目录
AGB-BUILTIN-002  外部网络连接
AGB-BUILTIN-003  文件生成
AGB-BUILTIN-004  敏感命令执行
AGB-BUILTIN-005  提权行为
```

内置规则 API 不提供 DELETE，也不允许 PUT 修改定义。启停、参数、severity、action 和 exception 通过 policy draft 中的 `builtin_rule_overrides` 保存并发布。

规则完整契约见
[builtin_behavior_rules_and_panorama_tree_v6.2.md](builtin_behavior_rules_and_panorama_tree_v6.2.md)。

### 3.4 策略

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/policies` | 分页查询 |
| POST | `/policies` | 创建 draft |
| GET | `/policies/:id` | 详情、版本和下发统计 |
| PUT | `/policies/:id` | 更新 draft |
| POST | `/policies/:id/validate` | 校验并返回编译预览 |
| POST | `/policies/:id/publish` | 发布新版本并异步下发 |
| POST | `/policies/:id/disable` | 停用并生成新 bundle |
| GET | `/policies/:id/deliveries` | 主机应用状态 |

创建策略请求：

```json
{
  "policy_key": "prod-ai-agent-guard",
  "name": "生产智能体防护",
  "priority": 100,
  "targets": {
    "host_ids": [],
    "host_group_ids": ["uuid"],
    "agent_types": ["codex", "openclaw", "hermes"]
  },
  "collection": {
    "categories": [
      "process",
      "file",
      "network",
      "identity",
      "persistence",
      "isolation",
      "kernel",
      "ipc",
      "tool",
      "control"
    ],
    "command_argv": "redacted",
    "file_content": "disabled",
    "network_content": "disabled",
    "aggregation": {
      "file_read_write_seconds": 2
    }
  },
  "builtin_rule_overrides": [
    {
      "rule_key": "AGB-BUILTIN-001",
      "rule_version": 1,
      "enabled": true,
      "severity_override": "high",
      "action_override": "alert",
      "parameters": {
        "resource_groups": ["credential", "privilege_policy"]
      },
      "exceptions": []
    },
    {
      "rule_key": "AGB-BUILTIN-002",
      "rule_version": 1,
      "enabled": true,
      "parameters": {
        "trusted_cidrs": ["10.0.0.0/8"],
        "trusted_domains": ["models.example.internal"]
      },
      "exceptions": []
    }
  ],
  "atomic_rules": [
    {
      "rule_id": "uuid",
      "rule": "protected_resource_access",
      "resource": {
        "type": "file",
        "path": "/etc/shadow",
        "match": "exact"
      },
      "operations": ["read", "write", "delete", "rename"],
      "action": "deny",
      "severity": "critical"
    }
  ],
  "correlation_rules": [
    {
      "rule_id": "AGB-DOWNLOAD-EXEC-001",
      "window_seconds": 120,
      "action": "alert",
      "severity": "high"
    }
  ],
  "analysis": {
    "enabled": true,
    "trigger_severities": ["medium", "high", "critical"],
    "ai_only_action_ceiling": "alert",
    "evidence_window_seconds": 300
  },
  "escape_rules": [
    {
      "rule_id": "uuid",
      "rule": "join_external_namespace",
      "action": "deny_and_freeze",
      "severity": "critical"
    }
  ],
  "freeze_timeout_seconds": 300
}
```

校验至少覆盖：

- collection category、aggregation window 和数据采集上限。
- 文件资源路径必须是绝对路径。
- 禁止 `..` 和 shell metacharacter。
- glob 只能使用允许语法。
- atomic/correlation/operation/action/escape rule 必须在枚举中。
- correlation window、group key 和 required evidence 合法。
- 五个 builtin rule key/version 存在，parameters 符合对应 JSON Schema。
- trusted domain 使用 DNS label 边界，CIDR 合法且地址分类预览可解释。
- sensitive command 条件按 executable/argv token 边界校验。
- `file_content`、`network_content`、完整 stdin/stdout/stderr 不能被策略启用。
- `ai_only_action_ceiling` 第一版只能为 `audit` 或 `alert`。
- `deny_and_freeze` 必须指定 high/critical。
- freeze timeout 范围 30～900。
- selector 至少指定一个 Agent 类型或显式 `*`。
- 同一策略规则 ID 唯一。
- 同优先级冲突规则给出确定性预览。

### 3.5 运行实例、Session 和执行单元

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/agents` | 按 host + Agent asset 聚合的外层基本信息列表 |
| GET | `/instances` | Agent 实例列表 |
| GET | `/instances/:id` | 实例、控制进程、session、执行单元、覆盖和近期 finding |
| GET | `/instances/:id/process-tree` | 当前进程树快照 |
| GET | `/instances/:id/sessions` | 官方或推导的行为 session |
| GET | `/sessions/:id` | session 摘要、来源和证据完整性 |
| GET | `/sessions/:id/timeline` | 跨行为域操作时间线 |
| GET | `/panorama` | 按 Agent asset/实例/session 查询详情抽屉的行为全景树根 |
| GET | `/instances/:id/panorama` | 指定实例全景树 |
| GET | `/sessions/:id/panorama` | 指定 session 全景树 |
| GET | `/panorama/nodes/:node_id/children` | 懒加载树节点子项 |
| GET | `/execution-units/:id` | 隔离基线、实际状态和差异 |
| GET | `/execution-units/:id/timeline` | 行为/finding/分析/动作时间线 |

`GET /agents` 专门服务两个前端子页的外层 Agent 列表，一行表示
`host_id + asset_id`。未关联静态资产但已确认运行实例时，返回服务端生成的
稳定 `agent_scope_key`，`asset_id` 可空。响应只包含基本信息：

```json
{
  "agent_scope_key": "signed-scope",
  "asset_id": "uuid",
  "host": {
    "id": "uuid",
    "hostname": "prod-ai-01",
    "ip": "10.0.1.21"
  },
  "agent_type": "codex",
  "display_name": "Codex",
  "profile_key": "codex-linux",
  "running_instance_count": 2,
  "controller_pids": [4100, 4400],
  "runtime_status": "running",
  "isolation_types": ["linux_namespace"],
  "coverage_level": "full_enforcement",
  "coverage_reasons": [],
  "high_risk_finding_count": 2,
  "escape_finding_count": 0,
  "action_status": "none",
  "last_seen_at": "2026-07-30T10:00:00Z"
}
```

该 API 不返回 cmdline、文件路径、连接地址、隔离基线和分析正文。详细数据只在
用户打开 Agent 抽屉后，通过 `asset_id/agent_scope_key` 查询 `/instances`、
`/panorama`、`/findings` 和 `/execution-units/:id`。

外层列表查询条件：

```text
host_ids
agent_types
runtime_status
coverage
isolation_type
has_high_risk
has_escape_finding
keyword（Agent 名称、controller PID、主机名或 IP）
page/page_size
```

实例查询条件：

```text
host_id
asset_ids
agent_types
instance_ids
profile_key
status
coverage
isolation_type
container_id
start_time/end_time
page/page_size
```

`GET /instances?host_id=<id>` 必须返回该主机全部 Agent 类型和同类型的全部
runtime instance，不能按 `agent_type` 去重。每行稳定身份使用 instance ID，
控制进程身份使用 `controller_pid + controller_start_ticks`。

`GET /panorama` 专门服务已选 Agent 的详情抽屉，支持：

```text
agent_scope_key=<与 asset_id 二选一>
asset_id=<与 agent_scope_key 二选一>
instance_ids=<可多选>
session_id=<可选>
```

层级固定为 agent_asset/type → instance → session → execution unit →
process。主机信息作为 Agent 根节点数据返回，不单独渲染 Host 树节点。
command/file/network/privilege/isolation/rule/finding 节点挂在真实发起进程下。

Process 节点必须返回 PID、PPID、cmdline；File 节点必须返回 operation、file name、resolved path；Network 节点必须返回 destination IP/domain/port/protocol。命令和路径仍受 evidence 权限控制。

树 API 的 node type 增加 `agent_asset`。使用签名/编码 node ID、
cursor、每节点分页和 `has_children/child_count`，不能一次返回主机全部历史。
排序使用 `occurred_at + agent_sequence`。同类型多个运行实例不得合并。

### 3.6 行为事件与安全结论

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/behaviors` | 分页筛选规范化行为事件 |
| GET | `/behaviors/:event_id` | 行为、actor、resource、outcome 和采集完整性 |
| GET | `/behaviors/:event_id/raw` | 有权限时查询对应 runtime event 原始 JSON |
| GET | `/findings` | 分页筛选规则/智能分析安全结论 |
| GET | `/findings/:finding_id` | 结论、规则、证据图、反证、分析和动作 |
| POST | `/findings/:finding_id/handle` | 标记处置状态和备注 |
| POST | `/findings/:finding_id/analyze` | 人工触发/重试异步智能研判 |
| GET | `/findings/:finding_id/analyses` | 历史 analysis run |
| GET | `/analyses/:analysis_id` | 模型、提示词版本、摘要、输出和失败信息 |

筛选：

```text
host_id
agent_type
instance_id
session_id
execution_unit_id
category
operation
outcome
resource_type
resource_classification
decision
severity
verdict
confidence_min
rule_id
policy_id
resource_keyword
analysis_status
start_time/end_time
handled
page/page_size
```

行为 API 永不提供文件内容、网络内容、stdin/stdout/stderr 或原始环境变量。命令和路径详情需要 `agent_guard:evidence:read` 权限。

### 3.7 人工动作

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/execution-units/:id/freeze` | 暂停执行单元 |
| POST | `/execution-units/:id/resume` | 恢复执行单元 |
| POST | `/execution-units/:id/kill` | 终止执行单元 |
| POST | `/instances/:id/kill` | 终止整个 Agent 实例 |
| GET | `/actions` | 动作记录 |
| GET | `/actions/:id` | 请求、Agent 返回和最终状态 |

动作请求：

```json
{
  "reason": "确认存在未授权 namespace 访问",
  "hold": false
}
```

服务端返回 `accepted` 只表示命令已受理：

```json
{
  "action_id": "uuid",
  "command_id": "AG-GUARD-uuid",
  "status": "pending"
}
```

前端必须继续查询或接收 WebSocket，直到 `success/failed/expired`。

所有动作 API 只接受一个明确 `execution_unit_id` 或一个明确 `instance_id`，
不提供 host 级 freeze/resume/kill。服务端必须校验目标 unit 确实属于请求中
展示的 host/instance，并在审计中保存解析后的目标；对一个 unit 的动作不得
扩展到同机其他 Agent 或同类型其他实例。

## 4. 配置 bundle

### 4.1 ConfigSync 复用

复用当前协议：

```proto
message ConfigSync {
  string config_type = 1;
  string action = 2;
  string payload = 3;
}
```

V6.2 新增语义：

```text
config_type = agent_guard_bundle
action = full_sync
```

不使用 incremental 作为第一版，以避免 Agent 重连后部分规则状态不一致。bundle 全量但经过 JSON 压缩和 digest 去重。

### 4.2 Bundle Schema

```json
{
  "schema": "aegis.agent_guard.bundle.v1",
  "bundle_version": 1007,
  "generated_at": "2026-07-30T10:00:00Z",
  "host_id": "uuid",
  "profiles": [],
  "builtin_rules": [
    {
      "rule_key": "AGB-BUILTIN-001",
      "rule_version": 1,
      "compiled_parameters": {},
      "digest": "sha256:..."
    }
  ],
  "policies": [],
  "defaults": {
    "mode": "monitor_only",
    "freeze_timeout_seconds": 300,
    "reconcile_interval_seconds": 30
  },
  "digest": "sha256:..."
}
```

digest 计算时不包含 `digest` 字段自身，并对 JSON 进行 canonical encoding。

### 4.3 应用响应

现有 `ConfigSyncResponse.applied` 可以提供同步 RPC 的即时结果：

```json
{
  "agent_guard_bundle": true
}
```

为满足 V6.1 “受理不等于完成”的要求，Agent 还必须上报：

```text
event_type = agent_guard_config_status
```

```json
{
  "bundle_version": 1007,
  "digest": "sha256:...",
  "status": "applied",
  "profile_count": 3,
  "builtin_rule_versions": {
    "AGB-BUILTIN-001": 1,
    "AGB-BUILTIN-002": 1,
    "AGB-BUILTIN-003": 1,
    "AGB-BUILTIN-004": 1,
    "AGB-BUILTIN-005": 1
  },
  "policy_count": 2,
  "coverage": {},
  "error_code": "",
  "error_message": "",
  "applied_at": "..."
}
```

状态：

- `received`
- `validating`
- `applied`
- `rejected`
- `degraded`

## 5. 人工动作通信

### 5.1 BlockCommand 复用

复用：

```proto
message BlockCommand {
  string command_id = 1;
  string host_id = 2;
  string action = 3;
  string target = 4;
  string reason = 5;
}
```

新增 action 字符串：

```text
freeze_execution_unit
resume_execution_unit
kill_execution_unit
kill_agent_instance
hold_execution_unit
```

`target`：

- execution unit action：`execution_unit_id`
- instance action：`instance_id`

Agent 必须按本地 registry 解析 UUID，禁止 API 直接传 PID 作为上述 action 的 target，避免 PID 重用。

### 5.2 结果

同步 `BlockResponse` 返回真实失败原因。Agent 还上报 `agent_guard_action_status`，用于：

- 本地自动 freeze。
- freeze timeout 自动恢复。
- 命令响应后实际 cgroup 状态变化。

## 6. RuntimeEvent 契约

### 6.1 顶层字段

为保持当前各组件 protobuf 兼容，V6.2 第一版不要求新增 proto 字段。使用：

| RuntimeEvent 字段 | V6.2 值 |
| --- | --- |
| `event_id` | 全链路唯一 UUID |
| `host_id` | 真实 host UUID |
| `event_type` | Agent Behavior/Guard 事件类型 |
| `process_name/pid/ppid/uid/command_line/file_path` | 常用查询摘要 |
| `matched_rule_id` | 本地原子规则 ID，普通行为未匹配为空 |
| `matched_rule_title` | 规则名称 |
| `severity` | info/low/medium/high/critical |
| `process_tree` | 命中时进程链 JSON 摘要 |
| `event_data_json` | 完整 `aegis.agent_behavior.v1` 或状态事件 JSON |

### 6.2 事件类型

```text
agent_guard_config_status
agent_instance_started
agent_instance_updated
agent_instance_stopped
agent_execution_unit_started
agent_execution_unit_updated
agent_execution_unit_stopped
agent_behavior
agent_sandbox_violation
agent_isolation_drift
agent_guard_action_status
agent_guard_health
```

### 6.3 Schema 版本

行为事件 `event_data_json.schema` 固定：

```text
aegis.agent_behavior.v1
```

配置、实例、执行单元和动作状态事件继续使用 `aegis.agent_guard.v1`。消费者按 `event_type + schema` 路由，不能仅凭 schema 猜测业务类型。

消费者规则：

- 未知字段忽略。
- 必填字段缺失则事件仍写 `runtime_events`，Agent Guard 投影标记 parse_failed。
- schema 大版本未知时不创建 finding 或自动阻断告警，但保留原始事件。
- JSON 无效时沿用 DC 当前 normalize 行为，并增加解析错误指标。

### 6.4 顺序、吞吐和背压

行为 JSON 必须携带：

```text
host_boot_id
agent_sequence
occurred_at
occurred_monotonic_ns
collection.lost_events_since_last
```

- Agent 用户态 normalizer 为每个 host boot 分配单调 `agent_sequence`。
- Server 写 Kafka 时使用 `host_id + instance_id` 作为 message key，尽量保持单实例分区顺序。
- DC 以 `host_boot_id + agent_sequence` 检测缺口；乱序事件按允许迟到窗口关联，不能仅按 Kafka 到达顺序构建攻击链。
- 第一阶段复用现有 `ReportEvent(RuntimeEvent)`，由 Agent 本地聚合和有界发送队列控制速率。
- 当压测证明单事件 RPC 无法满足行为吞吐时，再以新增 RPC/字段号方式增加 `ReportEventBatch`；不得未经压测先破坏现有协议。
- 发送队列满时优先保留 deny、状态变化、逃逸和 finding 必需证据，并上报 drop counter；Server/DC 不把序列缺口解释为“期间无行为”。

## 7. Server 设计

### 7.1 修改点

```text
server/internal/grpc_server/
├── server.go
└── api_server_impl.go
```

职责：

1. `SyncAgentConfig` 接收 api-server 主机级 bundle。
2. 在线 Agent 优先使用现有 callback/stream 下发。
3. 记录当前主机最新 bundle version/digest，以便重连补发。
4. Agent Guard 事件不做业务解析，保持原始结构进入 Kafka。
5. BlockCommand 原样透传 action/target/reason，并保留 Agent 原始失败原因。

不在 Server 中复制 policy 表或 Adapter Profile 业务模型。持久化事实仍由 api-server/PostgreSQL 管理；Server 只缓存在线下发所需摘要。

### 7.2 重连

```text
Agent Register/stream ready
  -> Server 获取/持有最新主机配置摘要
  -> 下发 agent_guard_bundle
  -> 等待 Agent config status event
```

如果 Server 没有 bundle 缓存，可通过现有控制面同步机制重新请求或等待 api-server 定期 reconciliation，不允许发送空 bundle 覆盖 Agent last-known-good。

## 8. DC 设计

### 8.1 推荐目录

```text
dc/internal/
├── event_handler/
│   └── agent_guard_handler.go
├── model/
│   └── agent_guard.go
├── repository/
│   └── agent_guard_repository.go
└── pipeline/
    ├── agent_guard_projector.go
    ├── agent_behavior_normalizer.go
    ├── agent_resource_classifier.go
    ├── agent_behavior_correlator.go
    ├── agent_rule_engine.go
    ├── agent_finding_manager.go
    └── agent_evidence_window.go
```

### 8.2 路由

```go
if strings.HasPrefix(eventType, "agent_") &&
   isAgentGuardEventType(eventType) {
    persistRawRuntimeEvent()
    projectAgentBehaviorOrGuardEvent()
    updateRuntimeState()
    evaluateAtomicAndSequenceRules()
    updateSecurityFinding()
    maybeRequestAnalysis()
    maybeGenerateAlert()
    broadcastTypedUpdate()
}
```

### 8.3 原始与投影

- `runtime_events` 保存 Agent 上报的原始事件。
- `agent_behavior_events` 保存便于筛选的统一行为投影。
- `agent_security_findings` 保存规则和智能分析形成的安全结论。
- `agent_security_analysis_runs` 保存每次异步分析的版本、输入摘要、输出和失败事实。
- 两者使用同一个 `event_id` 幂等关联。
- 行为事件和 finding 通过 evidence relation 关联，finding 不覆盖原始事件。
- DC Kafka 重放时使用 upsert/do nothing 和规则幂等键防重复。

### 8.4 告警生成

不为每个普通行为事件创建 alert。

| 条件 | 行为 |
| --- | --- |
| 普通行为、无规则命中 | 只写行为投影 |
| decision=`audit` 且 severity info/low | 只写事件或低等级 finding |
| decision=`alert` | 根据去重键生成/更新告警 |
| 高置信关联规则 | 创建/更新 finding；按策略生成告警 |
| AI-only verdict=`malicious` | finding 标记需复核并告警，不自动 freeze |
| 规则 + AI 且证据完整 | 提升 finding 置信度；动作仍受发布策略上限约束 |
| `would_deny` | 生成 capability gap 告警或事件，按策略决定 |
| `deny` | high/critical 告警 |
| `deny_and_freeze` | critical 告警并关联 action |
| action failed | 提升告警，保留失败原因 |

去重键：

```text
host_id + instance_id + session_id + execution_unit_id + rule_id + correlation_bucket
```

时间窗口由 Agent Guard 配置控制，默认建议 5 分钟。

DC 首批必须注册五个内置 evaluator：

| Rule ID | DC 职责 |
| --- | --- |
| `AGB-BUILTIN-001` | 根据 resolved path/resource classification 和 operation/outcome 分层 |
| `AGB-BUILTIN-002` | 根据 IP/CIDR、可信 DNS 证据、trusted list 和代理限制判定 externality |
| `AGB-BUILTIN-003` | 区分 create attempt/success，并按路径、mode、executable/hidden 属性分层 |
| `AGB-BUILTIN-004` | 按 resolved executable 和 argv token 边界分类敏感命令 |
| `AGB-BUILTIN-005` | 根据 credential/capability/namespace before/after 区分 attempted/succeeded/inconclusive |

DC 还要在同一 instance/session/process chain 的 5 分钟默认窗口中关联五项 rule hit。任何单点 hit 和联合 finding 都保留各自 rule version 与 evidence event IDs。

### 8.5 智能分析

实时阻断不调用 LLM。DC 只对满足分析触发条件的 finding 构建 evidence window，并通过已有 worker-queue 模式请求 `AgentSecurityAnalysisService`。

分析输入必须包含：

- 有界、脱敏、按时间排序的命令/文件/网络/身份/隔离证据。
- rule hit、allowlist、业务标签、进程链和资源关系。
- outcome、反证、截断字段、drop counter 和远程不可观测范围。

分析输出必须符合 `verdict/attack_probability/confidence/evidence_event_ids/counter_evidence/uncertainties/recommended_action` JSON Schema。引用不存在 event ID、包含未知枚举或越权动作的输出标记为 `invalid_output`。

LLM 结论不能：

- 修改原始行为或规则命中。
- 将不确定推断写成已证实操作。
- 直接回写“阻断已成功”。
- 在没有规则证据和策略授权时触发自动 freeze。

模型超时或不可用时保留规则 finding，并将 analysis run 标记失败；不得把失败解释为 benign。

## 9. WebSocket/通知

复用现有 WebSocket 服务，新增 typed payload：

```text
agent_guard.instance_updated
agent_guard.agent_summary_updated
agent_guard.behavior_created
agent_guard.finding_updated
agent_guard.analysis_updated
agent_guard.action_updated
agent_guard.delivery_updated
```

消息示例：

```json
{
  "type": "agent_guard.action_updated",
  "data": {
    "action_id": "uuid",
    "execution_unit_id": "uuid",
    "status": "success",
    "action": "freeze_execution_unit"
  }
}
```

通知抽屉只推 high/critical finding、deny/freeze/action failed，不推普通行为事件或所有 audit。

## 10. 权限与审计

建议权限：

```text
agent_guard:read
agent_guard:evidence:read
agent_guard:analysis:read
agent_guard:analysis:run
agent_guard:policy:write
agent_guard:policy:publish
agent_guard:action:freeze
agent_guard:action:resume
agent_guard:action:kill
agent_guard:profile:write
```

如果当前部署仍采用简化角色模型，至少保证：

- 所有 GET 走现有认证中间件。
- evidence 命令/路径详情、analysis 输入摘要按独立权限控制。
- policy publish、profile 修改、analysis retry、freeze/resume/kill 仅授权角色。
- 写操作进入 audit log/command audit。
- 不能只依赖前端隐藏按钮。

审计字段：

```text
username
action
object_type/object_id
host_id
instance_id/execution_unit_id
before/after 或 policy version
reason
request_id
result/error
created_at
```

## 11. 错误码

| 错误码 | 含义 | HTTP |
| --- | --- | --- |
| `agent_guard_policy_invalid` | 策略校验失败 | 400 |
| `agent_guard_rule_not_found` | 内置规则或版本不存在 | 404 |
| `agent_guard_rule_version_unsupported` | 目标组件不支持该规则版本 | 409 |
| `agent_guard_rule_digest_mismatch` | 内置定义与数据库/组件 digest 不一致 | 409 |
| `agent_guard_rule_override_invalid` | 参数、例外或动作覆盖不合法 | 400 |
| `agent_guard_panorama_node_invalid` | 全景树 node ID 无效、过期或不属于查询范围 | 400/403 |
| `agent_guard_profile_invalid` | Profile 校验失败 | 400 |
| `agent_guard_policy_not_draft` | 非 draft 不可直接修改 | 409 |
| `agent_guard_instance_not_found` | 实例不存在 | 404 |
| `agent_guard_execution_unit_not_found` | 执行单元不存在 | 404 |
| `agent_guard_agent_offline` | 主机 Agent 离线 | 409 |
| `agent_guard_action_not_supported` | 主机不支持该动作 | 409 |
| `agent_guard_unit_state_conflict` | frozen/stopped 等状态冲突 | 409 |
| `agent_guard_remote_unobservable` | 远程执行不可观测/处置 | 409 |
| `agent_guard_behavior_not_found` | 行为事件不存在 | 404 |
| `agent_guard_finding_not_found` | 安全结论不存在 | 404 |
| `agent_guard_analysis_unavailable` | 智能分析服务不可用 | 503 |
| `agent_guard_analysis_invalid_output` | 智能分析输出未通过 Schema/证据校验 | 502 |
| `agent_guard_delivery_failed` | 配置下发失败 | 502 |
| `agent_guard_enforcement_unavailable` | 无内核阻断能力 | 409 |

错误响应必须带稳定 code 和可读 message，不能只返回“执行失败”。

## 12. 幂等、并发和一致性

### 12.1 策略发布

- `policy_id + version` 不可变。
- publish 使用事务：冻结版本 → 生成 bundle → 创建 delivery。
- 同一 policy draft 的重复 publish 使用 idempotency key。
- 同主机 bundle version 单调递增。

### 12.2 动作

- `command_id` 唯一。
- 同一 unit 的并发 freeze 合并或返回当前 pending action。
- resume 只能作用于 frozen/freezing。
- stopped unit 不能 resume。
- Agent action status 可以乱序到达，Repository 根据状态机拒绝从终态倒退。

### 12.3 行为、Finding 和分析

- `event_id` 唯一。
- DC 原始事件和投影独立幂等。
- finding 幂等键包含 rule version、session/unit、correlation bucket。
- analysis run 使用 finding + evidence digest + model/prompt version 幂等，重试创建新 attempt 但不重复创建 finding。
- 迟到事件可以更新 open finding 的 evidence；已关闭 finding 通过新 revision 记录，不覆盖审计。
- 数据库暂时不可用时 Kafka 允许重放。
- WebSocket 推送不是事实源，页面刷新必须通过 API 恢复真实状态。

## 13. 日志与指标

### 13.1 api-server

记录：

- policy/profile validate/create/publish/disable。
- bundle 生成、主机数量、version/digest。
- delivery 和 reconciliation。
- analysis request/run 的 ID、model/prompt version、input digest、状态、耗时和错误码。
- 人工 action 请求和结果。

不记录完整策略 payload、模型 evidence 正文或模型原始输出；路径可记录 rule count 或经权限控制的 hash。

### 13.2 server

记录 host ID、bundle version/digest、command ID、Agent 返回错误。不得记录 bundle 全文。

### 13.3 DC

记录 event ID/category/operation、host、instance/session/unit、投影/规则/finding/analysis 结果和 alert/action 关联。command line、file path、URL 和 evidence 正文默认不打 info 日志。

### 13.4 指标

```text
aegis_agent_guard_bundle_deliveries_total{status}
aegis_agent_guard_bundle_apply_latency_seconds
aegis_agent_behavior_events_consumed_total{category,operation,outcome}
aegis_agent_behavior_projection_errors_total{schema}
aegis_agent_behavior_rule_evaluations_total{rule,result}
aegis_agent_builtin_rule_hits_total{rule_key,rule_version,severity}
aegis_agent_security_findings_total{severity,verdict,source}
aegis_agent_security_analysis_runs_total{status,model}
aegis_agent_security_analysis_latency_seconds{model}
aegis_agent_guard_alerts_total{severity,source}
aegis_agent_guard_actions_total{action,status,source}
aegis_agent_guard_instances_current{agent_type,status}
aegis_agent_guard_coverage_current{level}
aegis_agent_panorama_query_latency_seconds{node_type}
aegis_agent_panorama_nodes_returned_total{node_type}
```

## 14. 后端测试设计

### 14.1 api-server

- 策略采集分类、资源、原子/序列规则、analysis/action/timeout 校验。
- 五个内置规则列表、版本、parameters Schema、不可删除和 policy override 校验。
- 全景树根/子节点 cursor、evidence 权限、PID reuse 和签名 node ID。
- policy version/digest 稳定。
- host selector 展开和空范围。
- publish 事务与重复请求幂等。
- delivery 状态不能把 failed 误报 applied。
- freeze/resume/kill 权限和状态冲突。
- behavior/finding/analysis API 分页、证据权限、筛选和错误码。
- evidence window 限界、二次脱敏和 event ID 存在性校验。
- AI-only malicious 无法调用 action service。
- 模型超时、非法 JSON、未知枚举和越权 recommended action 正确降级。

### 14.2 server

- online callback 和 stream 两种配置下发。
- offline 主机返回真实错误或进入待补发状态。
- Agent 原始 action error 透传。
- Agent 重连不发送空 bundle。
- Agent Guard RuntimeEvent 不丢失 `event_data_json`。

### 14.3 dc

- 每个行为 category/operation 和状态 event type 解析、规范化和投影。
- 无效 JSON 保留 raw event 并产生 parse metric。
- Kafka 重放不重复 behavior/finding/analysis/action。
- 普通行为事件不生成 alert 风暴。
- 下载→写入→chmod→execute 的乱序事件产生一个 finding。
- 不同 instance/session 的事件不能错误关联。
- 五个内置 evaluator 的正常、例外、不充分证据和 rule version 测试。
- 外链 IP/domain/trusted CIDR、文件 create attempt/success、敏感命令 token 边界和提权 before/after 测试。
- 五规则联合链形成一个 finding，引用全部 behavior event ID。
- allowlist 只抑制对应 finding，不删除行为事实。
- deny/freeze/action failed 生成正确告警。
- 实例和执行单元状态不倒退。
- WebSocket payload 与 API 查询结果一致。

## 15. 协议兼容策略

第一版优先复用字符串扩展点，减少跨组件 protobuf 同步风险：

- `ConfigSync.config_type`
- `BlockCommand.action`
- `RuntimeEvent.event_type`
- `RuntimeEvent.event_data_json`

旧 Agent：

- 不认识 `agent_guard_bundle` 时返回 applied=false 或 unknown config。
- api-server 将主机标记 `unsupported_agent_version`。
- 不得向旧 Agent 下发 freeze action。

如果后续事件查询量和 JSON 解析成本证明需要顶层字段，再以新增字段号方式扩展 `RuntimeEvent`，不得修改或复用已有字段号。
