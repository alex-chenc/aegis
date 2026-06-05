# Aegis V6.0 开发测试提示词: 双模智能安全指挥台

**版本**: 6.0  
**日期**: 2026-06-05  
**状态**: 开发测试提示词  
**目标读者**: Codex/AI 开发代理、后端开发、前端开发、测试开发

---

## 1. 使用方式

把本文作为 V6.0 功能开发的总提示词。开发代理必须先阅读并遵守以下设计文档：

- `docs/aegis_system_design_v6.0/README.md`
- `docs/aegis_system_design_v6.0/prd_design_v6.0.md`
- `docs/aegis_system_design_v6.0/overall_architecture_design_v6.0.md`
- `docs/aegis_system_design_v6.0/backend_development_design_v6.0.md`
- `docs/aegis_system_design_v6.0/frontend_development_design_v6.0.md`
- `docs/aegis_system_design_v6.0/api_database_design_v6.0.md`
- `docs/aegis_system_design_v6.0/implementation_blueprint_v6.0.md`
- `docs/aegis_system_design_v6.0/agent_runtime_tool_orchestration_design_v6.0.md`
- `docs/aegis_system_design_v6.0/external_mcp_datasource_design_v6.0.md`
- `docs/aegis_system_design_v6.0/host_attack_investigation_agent_design_v6.0.md`
- `docs/aegis_system_design_v6.0/assistant_api_curl_test_cases_v6.0.md`

---

## 2. 总任务提示词

你是 Aegis V6.0 的资深全栈开发与测试工程师。请在现有 Aegis 仓库中开发 **V6.0 双模智能安全指挥台** 功能。

V6.0 的目标是把 Aegis 从“页面操作驱动的主机安全平台”升级为“普通模式 + 智能模式共用同一控制面”的双模安全系统。普通模式必须完整保留 V5.8 的页面和能力；智能模式新增 `/assistant` 全局智能体工作台，用户可以通过自然语言完成主机资产查询、基线检查、漏洞治理、告警溯源、动态检测包管理、阻断策略分析、系统配置排障、外接 MCP 数据源查询和主机攻击研判。

实现时必须遵守：

1. 第一版不新增独立微服务，Assistant 编排层放在 `api-server/internal/assistant`。
2. 必须复用现有 `github.com/alex-chenc/agent-runtime`，参考现有 AI 分析页的 runtime 接入方式。
3. 智能模式不建立独立业务数据，必须调用现有 service/repository/gRPC 能力并写入现有业务表。
4. AI 不允许直接写数据库，不允许绕过 service，不允许直接调用 HTTP handler。
5. Tool 层只做参数校验、风险声明、调用现有业务函数、格式化结果、生成审批摘要。
6. 不允许把所有工具一次性暴露给大模型；必须通过 IntentRouter + ToolSelector 按需注入工具。
7. 高风险动作必须经过审批网关，支持 `request_approval`、`whitelist`、`full_access` 三种模式。
8. 所有工具调用、审批、外部 MCP 查询、主机攻击研判证据必须可审计、可追溯。
9. 外接 MCP 只作为受控外部数据源，由 api-server 查询并脱敏后注入上下文；Aegis 不对外提供 MCP Server。
10. 主机攻击研判使用 `host_attack_investigation` Profile 和高层 Investigation 工具，不把底层工具全集直接交给模型。

---

## 3. 开发阶段

### Phase 0: 数据库、模型、Repository

目标：

- 新增 Assistant 会话、消息、上下文引用、工具调用、审批、工具策略、记忆、外接 MCP 数据源、外部查询日志、攻击研判报告和证据表。
- 完成 GORM model、repository、迁移文件和基础单测。

必须实现：

- `migrations/015_v6.0_assistant_tables.sql`
- `api-server/internal/model/assistant.go`
- `api-server/internal/model/assistant_investigation.go`
- `api-server/internal/model/external_mcp.go`
- `api-server/internal/repository/assistant_*_repo.go`
- `api-server/internal/repository/external_mcp_*_repo.go`

测试要求：

- repository create/find/list/update 单测。
- 枚举状态校验。
- 唯一索引和分页查询测试。
- 迁移 SQL 可重复执行。

### Phase 1: Assistant 会话、消息、RunManager 和 SSE

目标：

- `/assistant` 可创建会话、发送消息、流式返回 token/tool/status/done 事件。
- 支持取消运行和查询消息历史。

必须实现：

- `api-server/internal/assistant/service.go`
- `api-server/internal/assistant/run_manager.go`
- `api-server/internal/assistant/runtime_factory.go`
- `api-server/internal/assistant/event.go`
- `api-server/internal/api/handler/assistant_handler.go`
- `GET/POST /api/v1/assistant/sessions`
- `POST /api/v1/assistant/sessions/:session_id/message`
- `GET /api/v1/assistant/sessions/:session_id/stream`
- `POST /api/v1/assistant/sessions/:session_id/cancel`

测试要求：

- Handler 单测覆盖创建会话、发送消息、取消运行。
- SSE 事件至少覆盖 `run_started`、`assistant_delta`、`tool_call`、`approval_required`、`done`、`error`。
- RunManager 并发安全测试：同一 session 不能重复启动冲突 run。

### Phase 2: ToolRegistry、IntentRouter 和只读工具

目标：

- 完成工具注册中心、工具目录、意图路由、工具选择和工具执行网关。
- 先实现只读工具，让智能体可以查询和解释主机、告警、任务、漏洞、DetectionPackage、Sigma 规则、阻断记录、系统配置。

必须实现：

- `tool_registry.go`
- `tool_catalog.go`
- `tool_selector.go`
- `intent_router.go`
- `tool_dispatcher.go`
- `tool_gateway.go`
- `tools/host_tools.go`
- `tools/task_tools.go`
- `tools/vulnerability_tools.go`
- `tools/detection_query_tools.go`
- `tools/package_tools.go`
- `tools/config_tools.go`
- `tools/audit_tools.go`

工具约束：

- Tool 不直接拼 SQL。
- Tool 不调用 handler。
- Tool 必须调用 service/repository/gRPC。
- 每个工具必须声明 domain、risk_level、required_permission、input_schema、output_schema。
- 每次运行只注入与用户意图相关的小工具集合。

测试要求：

- ToolCatalog 包含所有设计文档要求的工具。
- ToolSelector 在不同意图下返回不同工具集合。
- 只读工具不会创建、修改、删除业务数据。
- 只读工具返回结果可以生成 result card。

### Phase 3: 审批网关、工具策略和白名单

目标：

- 实现工具风险策略和审批流。
- 支持 `request_approval`、`whitelist`、`full_access` 三种审批模式。
- 高风险动作在执行前暂停 run，前端展示审批卡片，批准后继续执行，拒绝后写入拒绝结果。

必须实现：

- `risk_policy.go`
- `approval_gate.go`
- `tool_policy_service.go`
- `GET /api/v1/assistant/tools`
- `GET/PUT /api/v1/assistant/tool-approval-policy`
- `PUT /api/v1/assistant/tools/:tool_name/whitelist`
- `POST /api/v1/assistant/tools/whitelist/batch`
- `POST /api/v1/assistant/tools/whitelist/reset-defaults`
- `GET /api/v1/assistant/approvals/:approval_id`
- `POST /api/v1/assistant/approvals/:approval_id/approve`
- `POST /api/v1/assistant/approvals/:approval_id/reject`

测试要求：

- `request_approval` 模式下所有写操作都产生 pending approval。
- `whitelist` 模式下白名单工具直接执行，非白名单工具需要审批。
- `full_access` 模式下工具直接执行，但仍写审计。
- 审批批准后工具只执行一次，拒绝后不得执行。
- 重复 approve/reject 必须幂等。

### Phase 4: 写操作工具和 V5.8 能力映射

目标：

- 智能体可安全触发现有核心业务动作：基线检查、漏洞扫描、脚本生成、阻断、动态检测包草稿生成、构建、审核、签名、启用、禁用、回滚、卸载、hook allowlist 修改。

必须实现：

- `tools/baseline_tools.go`
- `tools/detection_tools.go`
- `tools/sigma_rule_tools.go`
- `tools/block_tools.go`
- `tools/agent_tool_proxy.go`
- `tools/package_tools.go` 中写操作能力

安全要求：

- DetectionPackage 签名、启用、hook allowlist 修改必须是高风险工具。
- 阻断命令必须是高风险工具。
- 修改系统配置必须是高风险工具。
- 工具执行结果必须落库到 `assistant_tool_calls` 和相关业务表。

测试要求：

- 高风险工具在默认审批模式下必须暂停等待审批。
- 工具失败必须返回明确错误，不得吞掉异常。
- 写操作执行后通过普通页面 API 查询到一致结果。

### Phase 5: 前端 Assistant 工作台

目标：

- 新增 `/assistant` 智能模式工作台。
- 支持会话列表、对话流、工具调用卡片、审批卡片、计划面板、上下文引用、结果卡片和普通页面进入智能模式。

必须实现：

- `frontend/src/api/assistant.ts`
- `frontend/src/store/assistant.ts`
- `frontend/src/views/assistant/AssistantWorkspace.vue`
- `AssistantSessionSidebar.vue`
- `AssistantConversation.vue`
- `AssistantComposer.vue`
- `AssistantPlanPanel.vue`
- `AssistantToolCallCard.vue`
- `AssistantApprovalCard.vue`
- `AssistantResultCard.vue`
- `AskAssistantButton.vue`

前端要求：

- 使用 Vue 3、Pinia、Element Plus 和现有主题。
- 运维工具界面要克制、紧凑、可扫描，不做营销式布局。
- SSE 断线要能提示并允许重连。
- 审批卡片必须展示工具名、风险等级、影响摘要、参数预览、回滚提示。
- 普通模式页面通过 `AskAssistantButton` 携带上下文进入智能模式。

测试要求：

- Store 单测覆盖会话、消息、SSE 事件、审批状态。
- 组件测试覆盖工具卡片、审批卡片、空状态、错误状态。
- 路由守卫和登录态保持与现有系统一致。

### Phase 6: 外接 MCP 数据源

目标：

- 支持配置外接 MCP 数据源，供智能体查询外部 SIEM、CMDB、EDR、工单、威胁情报等证据。
- 大模型不直接连接 MCP endpoint，由 api-server 受控查询、脱敏、标准化后注入上下文。

必须实现：

- `external_mcp_source_service.go`
- `external_mcp_client_factory.go`
- `external_mcp_query_planner.go`
- `external_mcp_context_builder.go`
- `external_mcp_redactor.go`
- `external_mcp_normalizer.go`
- `tools/external_mcp_tools.go`
- `GET/POST/PUT/DELETE /api/v1/assistant/mcp-sources`
- `POST /api/v1/assistant/mcp-sources/:id/test`
- `POST /api/v1/assistant/mcp-sources/:id/sync-schema`

安全要求：

- MCP 凭据必须加密存储。
- 查询结果注入 prompt 前必须脱敏。
- 外部查询日志必须保留 source、query、duration、status、redaction_summary。
- 不允许把外部 token、password、secret 原文写入消息或 prompt。

测试要求：

- MCP source CRUD。
- 连接测试成功/失败。
- schema 同步。
- 脱敏规则覆盖 token/password/secret/key。
- 外部查询失败不影响主会话继续返回可解释错误。

### Phase 7: 主机攻击研判智能体

目标：

- 新增 `host_attack_investigation` Profile。
- 用户可针对主机、告警或时间窗口发起攻击研判，系统输出证据矩阵、入口推断、攻击时间线、攻击路径图、置信度和处置建议。

必须实现：

- `host_attack_investigation_service.go`
- `investigation_plan_builder.go`
- `evidence_collector.go`
- `evidence_correlator.go`
- `attack_timeline_builder.go`
- `entry_point_inferer.go`
- `attack_path_builder.go`
- `compromise_scorer.go`
- `investigation_report_builder.go`
- `tools/investigation_tools.go`
- `POST /api/v1/assistant/investigations/host-attack`
- `GET /api/v1/assistant/investigations/:id`
- `GET /api/v1/assistant/investigations/:id/evidence`

研判要求：

- 不把底层工具全集交给模型。
- 使用高层 `Investigation.*` 工具编排资产、漏洞、基线、告警、Agent 和外部证据。
- 所有结论必须绑定 evidence_id。
- 无证据时必须输出“不确定”，不能编造攻击入口。
- 攻击路径图必须可追溯到时间线和证据矩阵。

测试要求：

- 无证据主机输出低置信度/不确定。
- 有告警 + 弱口令 + 外部登录证据时可推断入口。
- 证据缺失时不生成确定性结论。
- 结果卡片包含 evidence、timeline、entry_point、attack_path、score。

---

## 4. 全局测试矩阵

| 层级 | 测试内容 | 命令或方式 |
|:---|:---|:---|
| Go 单测 | assistant service/repo/tool/risk/approval/investigation | `cd api-server && go test ./internal/assistant/... ./internal/repository/... ./internal/service/...` |
| Handler 单测 | Assistant HTTP API、审批、MCP source | `cd api-server && go test ./internal/api/handler/...` |
| Agent-runtime 接入 | RuntimeFactory、ToolGateway、SSE 事件 | Go 单测 + deterministic test mode |
| 前端单测 | store、组件、路由、SSE 事件处理 | `cd frontend && npm run test -- assistant` |
| 类型检查 | Vue/TS 类型 | `cd frontend && npm run type-check` |
| 构建 | api-server/server/frontend | `make build` / `npm run build` |
| API 验收 | curl + jq | 参考 `assistant_api_curl_test_cases_v6.0.md` |
| 端到端 | 创建会话 -> 工具调用 -> 审批 -> 结果卡片 | Docker Compose + curl |

如果当前环境缺少 Docker、依赖、真实 LLM 或外部 MCP 服务，必须报告阻塞原因，并至少完成可运行的单元测试和确定性 mock 测试。

---

## 5. 最小验收清单

开发完成后，至少满足：

- `/assistant` 页面可访问。
- 能创建会话、发送消息、接收 SSE。
- `assistant_sessions`、`assistant_messages`、`assistant_tool_calls`、`assistant_approvals` 正常落库。
- 只读工具能查询主机、告警、漏洞、任务、DetectionPackage。
- 写操作工具在默认模式下需要审批。
- 批准审批后工具执行，拒绝审批后工具不执行。
- 白名单模式和 full_access 模式生效。
- 普通页面可携带上下文进入 Assistant。
- 外接 MCP 数据源可配置、测试、同步 schema，查询结果会脱敏。
- 主机攻击研判能生成证据矩阵、时间线、入口推断、攻击路径和置信度。
- 所有工具调用、审批和外部查询都有审计轨迹。
- curl 测试用例中的核心用例可通过。

---

## 6. 禁止事项

- 禁止删除或削弱 V5.8 普通模式页面。
- 禁止让 AI 直接执行 SQL。
- 禁止让 AI 直接调用 HTTP handler。
- 禁止让 AI 直接连接外部 MCP endpoint。
- 禁止把所有工具一次性暴露给模型。
- 禁止把密钥、token、password、secret 原文写入 prompt、消息、日志或审计。
- 禁止高风险动作绕过审批策略。
- 禁止主机攻击研判在没有证据时编造攻击入口或攻击路径。

---

## 7. 提交前自检提示词

在提交代码前，请逐项自检：

1. 我是否复用了现有 service/repository/gRPC，而不是复制业务逻辑？
2. 我是否把 handler 内重业务逻辑下沉到了 service？
3. 我是否为每个工具声明了风险等级和权限？
4. 我是否验证了审批模式和白名单策略？
5. 我是否保证外部 MCP 数据和命令行参数脱敏？
6. 我是否为主机攻击研判结论绑定了证据？
7. 我是否运行了最窄有用测试，并记录无法运行的原因？

---

## 8. 交付说明模板

完成开发后，向用户汇报时使用：

```text
已完成 V6.0 双模智能安全指挥台开发。

主要改动：
- 后端：Assistant 编排层、ToolRegistry、审批网关、外接 MCP、主机攻击研判。
- 前端：/assistant 工作台、SSE 对话、工具卡片、审批卡片、普通页面智能入口。
- 数据库：assistant_*、external_mcp_*、investigation_* 表。
- 测试：单测、接口测试、curl 验收。

验证：
- 已运行 ...
- 未运行 ...，原因是 ...

风险：
- ...
```
