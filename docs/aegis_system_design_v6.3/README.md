# Aegis V6.3 设计文档

- **版本**：V6.3 方案版
- **日期**：2026-08-11
- **状态**：会话感知已有代码基线；Agent Guard 智能体高层工具第一阶段已实现；MCP 聚合治理平台已落地 P0/P1 及 P2 运行时确定性规则闭环，完整四阶段上下文持久化、跨调用 Activity、durable AI 和规模化发布仍在开发
- **首批会话感知产品**：Claude Code、OpenAI Codex CLI
- **主题**：智能体会话感知；智能体模式集成 V6.2 Agent Guard 全能力；MCP 聚合治理平台

## 1. 版本定位

V6.3 在 V6.2 Agent Guard 已实现的智能体运行实例、真实会话边界、可信工具
事件、eBPF 行为证据和安全发现之上，新增“会话正文感知与安全分析”能力。

V6.3 同时新增“智能体模式集成 V6.2 Agent Guard 全能力”专项：在 `/assistant`
通过现有 Tool Registry、exact capability、RBAC、审批和 agent-runtime 动态规划，
复用 V6.2 的查询、研判、运行时设置和受控动作能力。该专项不复制业务数据，不改变
Agent/eBPF、Kafka/DC、规则归属或动作状态机。详细设计见
[智能体模式 Agent Guard 集成设计](assistant_agent_guard_integration_design_v6.3.md)和
[智能体模式 Agent Guard 开发文档](assistant_agent_guard_development_design_v6.3.md)。

V6.3 还新增 MCP 聚合治理平台专项：把 v6.0 的“Assistant 受控访问外部 MCP 数据源”
升级为组织级双向平台。所有受管 Client 只连接 Aegis Catalog endpoint；上游 MCP Server
必须完成登记、发现、安全评估、审批和发布，平台统一执行 Client/工具授权、调用审批、
输入输出控制、完整审计、规则分析和逐调用 AI 安全分析。详细设计见
[MCP 聚合治理平台总体设计](mcp_aggregation_governance_platform_design_v6.3.md)。
本期只支持可通过网络访问的远程 MCP Server，并提供一键接入编排；不实现 stdio、
本地 command 或 Runner。

V6.2 已经存在完整会话正文采集的 P5 草案，但 V6.2 基线当时只实现了 session
start/end 和工具生命周期，不采集 prompt、助手可见回复或完整工具结果。当前 V6.3
代码已将该草案收敛为会话感知实现，仍需按以下产品和验收约束完成验证：

1. 首批只支持 Claude Code 和 Codex CLI，不包含 OpenCode。
2. 页面固定命名为“智能体会话感知”，不再使用“智能体会话检测”。
3. 安全结论明确拆分为“规则分析”和“AI 分析”，两者结果独立可见。
4. AI 不接收无界完整会话，而按 Token 预算和 turn 边界分段分析。
5. 每个会话显示“可见内容 Token 估算”；存在来源 usage 时，另行显示来源
   上报的模型调用用量，禁止混为一个指标。
6. 默认只上传和保存脱敏文本，不在 V6.3 实现原文 reveal/export。

## 2. 核心数据流

### 2.1 智能体会话感知

```text
Claude Code / Codex CLI
  -> 本地会话 JSONL 落盘
  -> Aegis Agent 定时静态扫描（有界发现、版本化解析、文件游标）
  -> agentsession（校验、脱敏、排序、加密 spool）
  -> gRPC ReportAgentSessionBatch
  -> Server
  -> Kafka: aegis.agent.sessions.v1
  -> DC（幂等投影、完整性检查、行为关联）
  -> PostgreSQL
  -> api-server（Token 汇总、规则分析、AI 分段分析、查询）
  -> WebSocket 元数据通知
  -> Frontend 智能体会话感知
```

### 2.2 MCP 聚合治理

```text
受管 MCP Client
  -> Aegis MCP Gateway（OAuth / Catalog / Grant / Policy / Approval）
  -> 已审批的 Tool Revision
  -> 已准入并发布的 Remote MCP Server
  -> 四阶段加密审计（Client 请求 / 上游请求 / 上游结果 / Client 结果）
  -> Kafka: aegis.mcp.invocations.v1
  -> DC 确定性序列规则 + api-server AI Worker
  -> PostgreSQL / MinIO / 告警 / Frontend
```

## 3. 文档索引

| 文档 | 内容 |
| --- | --- |
| [prd_design_v6.3.md](prd_design_v6.3.md) | PRD、用户故事、页面范围、验收指标 |
| [adr_reference_and_decisions_v6.3.md](adr_reference_and_decisions_v6.3.md) | Uber ADR 调研、复用点、差异和设计决策 |
| [overall_architecture_design_v6.3.md](overall_architecture_design_v6.3.md) | 总体架构、组件职责、数据流、信任边界 |
| [agent_collection_design_v6.3.md](agent_collection_design_v6.3.md) | Agent 静态扫描、Claude/Codex parser、脱敏、游标、spool |
| [security_analysis_design_v6.3.md](security_analysis_design_v6.3.md) | 提示词规则、AI 分段、Token 估算、综合风险 |
| [backend_api_protocol_design_v6.3.md](backend_api_protocol_design_v6.3.md) | Proto、Server、Kafka、DC、api-server、HTTP API |
| [database_design_v6.3.md](database_design_v6.3.md) | 数据模型、表、索引、迁移、保留与回滚 |
| [frontend_design_v6.3.md](frontend_design_v6.3.md) | 页面结构、列表、详情、状态、权限和测试 |
| [implementation_test_rollout_v6.3.md](implementation_test_rollout_v6.3.md) | 实施阶段、测试、日志、指标、灰度与回滚 |
| [development_prompt_v6.3.md](development_prompt_v6.3.md) | 可直接交给开发智能体的主提示词 |
| [assistant_agent_guard_integration_design_v6.3.md](assistant_agent_guard_integration_design_v6.3.md) | `/assistant` 集成 V6.2 Agent Guard 全能力的范围、架构、工具目录、RBAC、审批和验收 |
| [assistant_agent_guard_development_design_v6.3.md](assistant_agent_guard_development_design_v6.3.md) | 文件级实施、共享应用服务、工具契约、测试、日志、灰度和回滚 |
| [assistant_mcp_aggregation_integration_design_v6.3.md](assistant_mcp_aggregation_integration_design_v6.3.md) | V6.1 智能体模式接入 V6.3 MCP 聚合管控的 Client、Gateway、工具契约、权限、迁移和灰度设计 |
| [mcp_aggregation_governance_platform_design_v6.3.md](mcp_aggregation_governance_platform_design_v6.3.md) | MCP 聚合平台目标、准入准则、Gateway、Catalog、工具控制、完整审计、规则和 AI 安全分析 |
| [mcp_aggregation_platform_api_database_design_v6.3.md](mcp_aggregation_platform_api_database_design_v6.3.md) | MCP 控制面/协议/API、数据库、Kafka、RBAC 和 v6.0 迁移 |
| [mcp_aggregation_platform_frontend_design_v6.3.md](mcp_aggregation_platform_frontend_design_v6.3.md) | “MCP 聚合管控”单一菜单入口、远程 Server 一键接入、页面标签、权限、状态、安全交互和前端验收 |
| [mcp_aggregation_platform_implementation_test_rollout_v6.3.md](mcp_aggregation_platform_implementation_test_rollout_v6.3.md) | MCP 平台分阶段实施、测试、日志、指标、灰度、停止条件和回滚 |
| [mcp_aggregation_platform_development_prompt_v6.3.md](mcp_aggregation_platform_development_prompt_v6.3.md) | 可直接交给开发智能体的 MCP 聚合管控全模块主提示词和 P0-P5 分阶段任务入口 |
| [fix/mcp_context_and_rule_matching_alignment_v6.3.md](fix/mcp_context_and_rule_matching_alignment_v6.3.md) | MCP 上下文采集边界、实时规则匹配、历史记录限制和安全分析最终交互校准 |

## 4. V6.3 核心决策

| 编号 | 决策 |
| --- | --- |
| V63-D01 | 借鉴 ADR 的“来源 parser -> 统一模型”，但使用 Aegis Go Agent 和现有传输链路，不嵌入 ADR Python Sensor |
| V63-D02 | 正文只通过 ADR 风格的本地 JSONL 静态扫描获取；不安装或依赖 Claude/Codex Hook |
| V63-D03 | 会话正文使用专用 gRPC/Kafka/数据库链路，不进入 `RuntimeEvent.event_data_json` |
| V63-D04 | Agent 启动/周期/手工触发有界扫描，以 dev/inode/offset cursor 增量读取；不使用文件监听器 |
| V63-D05 | 规则分析是确定性检测，AI 分析是无工具、只读、结构化输出的异步语义检测 |
| V63-D06 | AI chunk 默认目标 6,000 tokens、硬上限 8,000 tokens，并根据模型上下文动态下调 |
| V63-D07 | 会话 Token 指标至少分成可见内容估算、来源上报用量、Aegis AI 分析用量三组 |
| V63-D08 | 会话文本和工具输出均是不可信数据，不能成为分析器指令，也不能触发分析器工具调用 |
| V63-D09 | V6.3 只做检测、展示、告警和人工研判，不由会话规则或会话 AI 自动 deny/freeze/kill |
| V63-D10 | 新增 migration 使用 `032_v6.3_agent_session_awareness.sql`；现有 030、031 已被 V6.2 使用 |
| V63-D11 | V6.2 Agent Guard 通过 Assistant Tool Registry 接入智能体模式，不建立第二套业务表或专用规划器 |
| V63-D12 | Agent Guard HTTP 与 Assistant 工具必须共享 scope、脱敏、错误和状态语义；工具不得直接绕过应用服务访问原始 model |
| V63-D13 | V6.2 当前基线已降级为历史兼容的 policy draft/publish API 不向模型开放；内置策略/规则保持只读 |
| V63-D14 | 参照 V6.1，高层业务 capability 面向模型；Agent Guard 原子查询、策略解析、raw evidence 和 dispatch 按 contextual/companion/internal 分层 |
| V63-D15 | 隐藏策略实现细节但保留受权限控制的规则摘要、coverage、风险和证据缺口；隐藏不替代后端授权 |
| V63-D16 | 新增独立 `mcp-gateway`，控制面与高并发数据面隔离 |
| V63-D17 | Client 只接入按用途发布的 Catalog endpoint，不提供默认全工具 endpoint |
| V63-D18 | Catalog Release 和 Tool Revision 不可变；上游漂移必须重新准入 |
| V63-D19 | MCP `2026-07-28` 为主协议，旧版本通过兼容适配器和迁移期限支持 |
| V63-D20 | 发布审批与调用审批分离；调用审批绑定参数、目标、Revision 和 Policy digest |
| V63-D21 | 四阶段完整 payload 加密存 MinIO，PostgreSQL 保存索引、摘要、digest 和 object ref |
| V63-D22 | 每个受理调用执行确定性规则与 AI 分析；AI 不能降低确定性风险或改变权限 |
| V63-D23 | V6.3 只纳管远程 MCP Server，不支持 stdio、本地 command 或 Runner |
| V63-D24 | 上游 annotations 是不可信提示，工具控制使用平台 verified metadata 和 Policy |
| V63-D25 | v6.0 外部 MCP 与 Assistant 迁入新平台；`tools/aegis-mcp` 以远程 HTTP 形态按普通 Server 接入，stdio 仅作开发兼容 |
| V63-D26 | 前端仅在“系统配置”新增一个“MCP 聚合管控”入口，所有治理工作区使用同页内部标签 |
| V63-D27 | 安全规则主页面只保留“查看安全规则”按钮；完整规则表在抽屉中展示，调用安全判定位于其下方且不展示 AI 状态 |
| V63-D28 | 新 MCP 调用使用运行时上下文执行确定性 pre/post 规则并写入 rule hit；历史调用缺少上下文时只能标记历史投影，不回溯伪造命中 |
| V63-D29 | MCP 上下文采用内存实时评估、PostgreSQL 脱敏摘要、MinIO 加密受限正文的分层策略；敏感值不得进入日志、Kafka、前端或 AI 输入 |

## 5. 完成标准

只有以下条件同时满足，才可宣称 V6.3 已完成：

1. Claude Code、Codex CLI 各至少两个受支持版本 fixture 和一个未知版本降级
   fixture 通过。
2. 新建、继续、compact、结束和子智能体会话不会串联或重复。
3. 用户消息、助手可见回复、工具调用/结果和权限事件按真实顺序展示；隐藏推理
   不采集。
4. 所有正文先脱敏再离开主机；secret fixture 不出现在 gRPC、Kafka、数据库、
   日志、WebSocket 和浏览器存储。
5. 规则分析可独立运行，首批提示注入/越狱规则能定位到 item 和字符区间。
6. AI 分析按 Token 预算分段，超长单 turn、compact、失败重试和最终聚合均有
   明确状态。
7. 每个会话显示 Token 估算方法与更新时间；来源 usage 和 Aegis 分析用量不被
   标成“会话内容 Token”。
8. 页面结构与“智能体事件感知与防护”一致，具备顶部指标、筛选、列表、详情
   抽屉、加载/空/错误/降级状态。
9. 未授权角色看不到正文，正文不进入 URL、console、埋点、普通错误信息或通知。
10. 断网、Kafka 重放、Agent/API 重启、源文件 rotate/truncate 后数据无重复；
    无法补全时显示 missing range，不静默宣称 complete。
11. 智能体模式可查询 V6.2 Agent Guard 概览、Agent、实例、行为会话、执行单元、
    行为、全景、规则、Finding、分析、配置检测和动作状态。
12. Agent Guard 写操作保持现有细分 RBAC；等待审批后角色或目标状态变化时必须拒绝
    执行，`full_access` 不赋予缺失的 `agent_guard:*` 权限。
13. runtime settings、freeze/resume/kill 和 session delete 使用真实领域服务和状态机；
    pending/dispatching 不得在智能体回答中称为完成。
14. kill/delete 的后端硬确认不能由模型生成或被 Assistant `full_access` 绕过。
15. 关闭 Agent Guard Assistant feature flag 后，V6.2/V6.3 普通页面和数据面无回归。
16. 普通助手目录优先展示高层 Agent Guard 业务能力，不能从全量领域 capability 或内部策略工具中自由选举。
17. 受管 Client 只通过 Aegis Catalog endpoint 获取有效工具；已撤销 Grant 或已暂停工具
    无法通过缓存名称直接调用。
18. MCP Server/Tool 完成准入、独立审批、不可变发布、漂移隔离、暂停和回滚闭环。
19. 下游 Token 与上游凭据严格分离，issuer/audience/resource、逐 Client consent 和
    credential 轮换通过安全测试。
20. 每次 `tools/call` 可关联 Client 请求、实际上游请求、上游原始结果和实际交付结果，
    四阶段 payload/digest 对账率为 100%。
21. bearer token、密码、私钥和 canary secret 在日志、数据库明文字段、Kafka、UI、Trace
    和 AI 输入中的泄漏数为 0。
22. 所有受理调用都有规则结果和 AI run 或明确失败/降级状态；AI 不能降低规则风险、
    越权批准或直接执行动作。
23. L3/L4 调用审批绑定用户、Client、Release、Tool Revision、参数、目标和 Policy digest；
    非幂等调用不会因重试重复执行。
24. MinIO、Kafka、控制面、上游和 AI 故障符合 fail-closed/degraded 矩阵，不出现成功但
    无审计、待分析却显示安全或结果未知却自动重试。
25. v6.0 External MCP 与 Assistant 完成兼容迁移；Gateway、DC、api-server 和 frontend
    通过定向构建与跨服务 E2E 后方可宣称专项完成。
26. “系统配置”下只新增一个“MCP 聚合管控”入口；远程接入、工具发布、Client 授权、审批、
    调用审计和安全分析在同一页面内完成，且不与“MCP 资产”混淆。

## 6. 参考资料

- [Uber ADR](https://github.com/uber/ADR)
- [ADR Sensor](https://github.com/uber/ADR/tree/main/Sensor)
- [ADR Detection](https://github.com/uber/ADR/tree/main/Detection)
- [ADR Claude Parser](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/parsers/claude_parser.py)
- [ADR Codex Parser](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/parsers/codex_parser.py)
- [Claude Code Sessions](https://code.claude.com/docs/en/sessions)
- [Claude Code application data](https://code.claude.com/docs/en/claude-directory)
