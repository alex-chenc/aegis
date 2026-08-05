# V6.2 当前实现基线（2026-08-06）

本文是 V6.2 设计文档与当前代码发生差异时的实现事实基线。它不替代各专项设计中的目标、安全约束和协议定义；专项文档中与本文冲突的旧流程，以本文为准，并应在下一次专项修订时删除旧描述。

## 1. 当前交付范围

当前已落地的是 Agent Guard 的运行行为监控、可信工具事件、真实会话边界、规则命中和前端展示闭环。完整的“会话正文采集、长会话分段、AI 会话语义分析和授权原文导出”仍属于 P5 设计范围，不能在当前实现状态中宣称已完成。

已验证的主要能力：

- Agent Guard 运行实例按 `host_id + controller_pid + controller_start_ticks` 识别，行为事件使用真实 session ID 进行范围关联。
- Codex、Claude Code、OpenClaw、Hermes、Zcode 支持原生 Hook 注入开关；开关由控制面立即下发，Agent 在内存应用并执行 Hook provision/remove，不要求用户手工编辑 Agent 本地配置文件。
- 工具事件包含 `tool_call_started`、`tool_call_completed`、`tool_call_failed`，使用签名 Hook/可信 Adapter 进入 Agent。
- Agent eBPF 不负责 Agent Guard 工具命令规则命中，只负责进程事实、PID/PPID/start_ticks、资源事件和 tool→process 关联所需的证据。
- API-server 消费 `aegis.security.events` 中的 Agent 工具行为事件，基于工具输入中的命令行匹配规则，并将 finding 直接写入安全发现表。
- DC 对 Agent Guard 行为只做规范化、投影、幂等落库和通知；不再调用 Agent Guard 工具命令规则评估器，也不生成这类 finding。
- 规则命中的直接证据是工具行为事件的 `raw_event_id`；eBPF/PID 关联缺失时，工具命令规则仍可命中，但关联状态必须显示为未匹配或不可验证。
- 前端策略入口改为只读的“内置防护策略”目录，展示两类内置策略视图和五条内置规则的完整定义；不再从页面创建、发布或查看历史策略版本。

## 2. 权威事件链路

### 2.1 工具命令规则链路

```text
Codex/Claude Code/OpenClaw/Hermes/Zcode 原生 Hook
  -> Agent signed Hook ingress
  -> Agent session/tool normalizer
  -> Server Agent stream
  -> Kafka: aegis.security.events
  -> DC: Agent Guard 行为规范化/投影
  -> API-server: agent_guard_tool_rule_consumer
  -> API-server 命令行规则匹配
  -> agent_security_findings
  -> Frontend 安全分析
```

API-server 当前消费组为：

```text
aegis-api-server-consumer-agent-guard-tool-rules
```

API-server 对 `tool_call_started`、`tool_call_completed` 和 `tool_call_failed` 都可以进行匹配。存在 `tool_call_id` 时，开始与结束事件使用相同 finding 幂等键合并；没有 `tool_call_id` 时才退化使用事件 ID。

### 2.2 eBPF 的边界

eBPF/`/proc` 只提供以下事实或关联：

- PID、PPID、start_ticks、可见命令行和 executable。
- fork/exec/exit、文件、网络、身份、namespace/cgroup 等操作系统事实。
- 工具事件与真实进程/资源之间的绑定证据。
- 关联失败、PID 重用、事件丢失和命令行不可见等完整性状态。

它不根据命令行或 eBPF 的 `MatchedRuleId` 为 Agent Guard 工具事件创建规则命中。旧的通用运行时 Sigma/eBPF 链路仍是独立数据面，不得把其结果伪装成可信工具调用规则命中。

### 2.3 DC 的边界

DC 仍然负责 Kafka 消费、事件规范化、资源分类、幂等投影、会话/执行单元索引和 WebSocket 通知；但 Agent Guard 工具事件不再经过 `ProcessBehavior` 或 DC 内置规则评估器生成 finding。API-server 是工具命令规则的唯一规则归属方。

## 3. 规则与内置策略目录

### 3.1 五条内置规则

规则定义事实源仍为 `agent_behavior_rule_definitions` 和现有 `/api/v1/agent-guard/rules` 接口：

| 规则键 | 中文名称 | 行为域 | 当前工具命中归属 |
| --- | --- | --- | --- |
| `AGB-BUILTIN-001` | 操作敏感目录 | file | OS 事实/规则定义；工具命令不直接命中 |
| `AGB-BUILTIN-002` | 外部网络连接 | network | OS 事实/规则定义；工具命令不直接命中 |
| `AGB-BUILTIN-003` | 文件生成 | file | OS 事实/规则定义；工具命令不直接命中 |
| `AGB-BUILTIN-004` | 敏感命令执行 | process/tool | API-server 工具命令行匹配 |
| `AGB-BUILTIN-005` | 提权行为 | identity/process | OS 事实/规则定义；工具命令不直接命中 |

每条规则展示规则键、版本、中文名称、说明、行为域、默认严重级别、默认动作、推荐动作、执行位置、必需证据、允许条件、MITRE 映射、默认参数、参数约束和 digest。内置定义不可从前端删除或原地修改。

### 3.2 内置策略视图

当前“策略”弹窗是只读目录，不是 `agent_guard_policies` 的 draft/publish 管理器。前端将内置规则按真实职责分为两个内置策略视图：

1. **智能体行为监控策略**：文件、网络、进程和身份行为规则。
2. **智能体工具命令审计策略**：工具调用命令规则 `AGB-BUILTIN-004`，明确显示 API-server 匹配和 Agent eBPF 关联边界。

这两个视图是内置产品能力的展示分组，不新增数据库策略表，也不生成新的下发版本。历史 `agent_guard_policies` 数据和策略 API 保留用于兼容、Bundle 生成及历史审计；当前页面不再允许用户从这里新建、发布、停用或查看“待下发”状态。

## 4. 运行时设置与 Hook 注入

控制面设置使用 `agent_guard_runtime_settings.v1`，通过现有 `system_configs` 按主机保存，配置类型为 `agent_guard_runtime_settings`。主要字段：

- `tool_adapter_enabled`：工具调用适配器开关。
- `session_hook_enabled`：智能体会话 Hook 开关。
- `injections[]`：Codex、Claude Code、OpenClaw、Hermes、Zcode 各自的 Hook 开关。
- `version`、`dispatch_status`、`dispatch_error_code`：控制面下发事实和失败原因。

点击开关后的状态语义：

```text
Frontend -> api-server 保存 settings -> Server/Agent ConfigSync
  -> Agent 校验并在内存应用
  -> 开启：provision Hook、开始上报
  -> 关闭：remove Hook、停止对应事件上报
```

`pending_reconnect` 只表示 Agent 当前离线，不能被翻译成“已应用”；在线 Agent 成功响应后才记录 applied。设置页面不要求用户执行 shell 命令，也不把设置写入 Agent TOML；Agent 只持久化自身 last-known-good runtime settings 以便重启恢复。

## 5. 会话和前端行为

- 行为全景按实例和真实 session ID 查询，session 列表和行为列表均支持分页。
- 真实 session ID 只能来自 Agent Hook 的 `session_started/session_activated/session_ended` 生命周期事件；没有真实 ID 时不能把进程活动伪造成某个 Codex 会话。
- 安全分析按当前选中的 session 过滤，不展示全主机或全 Agent 的混合 finding。
- 行为模式请求同时携带 `session_id` 和 `finding_domain=tool`；逃逸模式使用
  `finding_domain=escape`，服务端按域过滤历史/其他来源 Finding。
- 安全分析右侧只展示命中规则名称、命中工具、工具输入/输出摘要、命令行和能够验证的 PID/PPID 关联，不展示全量进程树。
- 命令行以工具事件的结构化 command/cmdline 为主；eBPF `/proc/<pid>/cmdline` 只作为进程关联证据，不替换工具命令事件的直接证据。
- 安全分析标签不再在标题旁显示 finding 数字；数量在内容区按当前会话分页数据展示。
- 策略入口显示“内置防护策略”，规则名称使用中英文 i18n，不把内部英文 key 当作唯一展示名称。

## 6. 兼容、数据和安全边界

- 不删除历史行为事件和历史 finding；新工具命中使用 API-server 产生的 finding，并在 `evidence_graph` 中记录 `rule_owner=api-server`、source event ID 和 correlation 状态。
- 工具命中不要求必须存在 eBPF 进程树；缺少关联时仍可展示工具规则命中，但不得声称已完成 PID 绑定。
- 工具命中不会直接触发 Agent 本地 deny/freeze；当前链路是审计/告警闭环，动作能力仍由独立的本地确定性防护链路控制。
- 工具 Hook 不采集 prompt、完整工具输出、stdin/stdout/stderr、环境变量、文件正文、网络 payload、密码和 token。
- API-server、DC、Agent 日志记录稳定事件名和摘要字段，不记录完整命令行或大段原始载荷。

## 7. 当前验证状态

已验证：

- API-server 工具规则服务/Repository 定向测试通过。
- DC Agent Guard 事件处理定向测试和 DC 全量 Go 测试通过。
- Agent 全量 Go 测试通过。
- 前端 Agent Guard 定向测试、内置策略目录测试和生产构建通过。
- API-server、DC、前端容器重建并健康；Agent 二进制已重建并重启。
- 真实日志已出现 `agent_guard_tool_rule_matched`，规则名为“敏感命令执行”，规则归属为 API-server。

未完成或需专用宿主机验证：

- BPF LSM 真实 `EPERM`、freeze/resume/kill 的生产能力门禁。
- 完整 P5 会话正文采集、AI 会话语义分析和授权 reveal/export。
- 旧通用运行时 Sigma/eBPF 数据面的整体规则归属重构；它不属于本次 Agent Guard 工具命令规则链路。
