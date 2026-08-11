# Aegis V6.3 MCP 聚合管控模块开发主提示词

## 1. 使用说明

将第 2 节完整复制给负责实现的 Codex、Claude Code 或其他开发智能体。

该模块跨越 `api-server`、新增 `mcp-gateway`、`dc`、PostgreSQL、Redis、Kafka、MinIO、
`frontend`、Docker Compose 和离线发布流程，不建议一次提交未经验证的全量改动。按提示词中的
P0-P5 分阶段开发，每阶段单独评审和验证；只有全部阶段满足完成门槛后，才能宣称 V6.3
“MCP 聚合管控”完成。

## 2. 可直接复制的开发提示词

````text
你正在 /code/aegis 仓库实现 Aegis V6.3“MCP 聚合管控”完整模块。

一、最终目标

把 Aegis 建设为组织级远程 MCP 聚合和安全治理平台：

1. Remote MCP Server 通过“一键接入”进入平台；
2. 平台自动完成 endpoint、认证、协议、工具、Schema、安全和风险检查；
3. Server/Tool 经准入审批后，以不可变 Catalog Release 发布；
4. MCP Client 只连接 Aegis Catalog endpoint，不直接获得上游 endpoint 或凭据；
5. Client 只能看到和调用 Grant 允许的工具；
6. 平台执行输入校验、工具控制、限速、审批、输出校验、脱敏和隔离；
7. 每次调用保存 Client 请求、实际上游请求、上游原始结果、实际交付结果四阶段证据；
8. 确定性规则和 AI 分析分别评估每次调用及跨调用 Activity 的安全风险；
9. 前端只新增一个“系统配置 / MCP 聚合管控”入口，在同一页面完成接入、发布、授权、
   审批、审计和安全分析；
10. v6.0 External MCP 迁移到新平台，Assistant 成为首个内置 Client，但迁移期间禁止同一次
    上游写操作在新旧路径双执行。

二、开始前必须读取并遵循

1. /code/aegis/AGENTS.md
2. /code/aegis/.agents/skills/aegis-software-designer/SKILL.md
3. /code/aegis/.agents/skills/daily-program-logging/SKILL.md
4. /code/aegis/.agents/skills/aegis-build-test/SKILL.md
5. 涉及离线发布时读取：
   /code/aegis/.agents/skills/aegis-release-packaging/SKILL.md
6. 出现失败测试、运行错误或行为回归时读取：
   /code/aegis/.agents/skills/root-cause-debugging/SKILL.md
7. 完整读取以下目标设计：
   - docs/aegis_system_design_v6.3/README.md
   - docs/aegis_system_design_v6.3/mcp_aggregation_governance_platform_design_v6.3.md
   - docs/aegis_system_design_v6.3/mcp_aggregation_platform_api_database_design_v6.3.md
   - docs/aegis_system_design_v6.3/mcp_aggregation_platform_frontend_design_v6.3.md
   - docs/aegis_system_design_v6.3/mcp_aggregation_platform_implementation_test_rollout_v6.3.md
8. 读取迁移来源：
   - docs/aegis_system_design_v6.0/external_mcp_datasource_design_v6.0.md
   - api-server/internal/model/external_mcp.go
   - api-server/internal/repository/external_mcp_*_repo.go
   - api-server/internal/assistant/external_mcp_*.go
   - api-server/internal/assistant/tools/external_mcp_tools.go
   - api-server/internal/api/handler/assistant_handler.go
   - api-server/cmd/main.go
9. 读取当前前端的路由、鉴权、设计系统和测试方式：
   - frontend/src/App.vue
   - frontend/src/router/index.ts
   - frontend/src/api/auth.ts
   - frontend/src/utils/auth.ts
   - frontend/src/composables/useRole.ts
   - frontend/src/views/settings/AssistantToolPolicySettings.vue
   - frontend/src/api/index.ts
   - frontend/package.json
10. 读取 docker-compose.yml、.env.example、api-server/dc 的入口、配置、Kafka、MinIO、Redis、
    LLM worker、Repository、HTTP response 和日志实现，复用当前项目模式。

不要只按设计文档猜测代码。开始时先执行 git status，确认用户和其他任务已有的未提交改动，
不得覆盖、回退或格式化无关文件。检查 migrations/ 的实际最大编号；设计稿中的 033/034 只是
当时建议，若编号已被其他 V6.3 工作占用，使用接下来两个未占用的连续编号，并同步所有文档、
测试、Docker 初始化和发布脚本。不得复用同一个 migration 编号。

三、当前代码基线

1. 当前仓库存在 v6.0 `external_mcp_sources`、`external_mcp_query_logs`、
   `ExternalMCP.*` Assistant 工具和旧查询链路；它们是迁移来源，不是新平台事实源。
2. 当前没有可直接复用的独立 `mcp-gateway` 进程；必须新增独立 Go 服务和镜像。
3. `tools/aegis-mcp` 已提供远程 Streamable HTTP 形态，可作为 dev 环境的普通远程 Server 通过 `/onboarding-jobs` 接入；stdio 仅保留开发兼容模式。
4. 当前“智能资产采集 / MCP 资产”只表示发现到的资产线索，不等于已准入或已发布。
5. 当前前端登录会话主要保存 token、username、role，全局路由守卫未完整执行细粒度
   `meta.permission`；实现新页面前必须建立由后端提供的 capability 事实源和统一 `can()`。
6. 已有 PostgreSQL、Redis、Kafka、MinIO、WebSocket、LLM client/worker、Element Plus、Pinia、
   Axios 和 vue-i18n 应优先复用，但不能因复用而绕过新领域的数据和安全语义。

四、不可改变的产品和架构决策

1. V6.3 只纳管可通过网络访问的 Remote MCP Server。
2. 生产主传输为 HTTPS Streamable HTTP；legacy HTTP+SSE 只做有期限的远程迁移兼容。
3. 禁止支持 stdio、本地 command、args、cwd、env、任意进程启动或 MCP Runner。
4. 新增独立 Go 进程 `mcp-gateway`：Aegis 对下游是 MCP Server，对上游是 MCP Client。
5. `api-server` 是控制面事实源；Gateway 不能管理草稿或临时解释管理员配置。
6. Gateway 只加载已签名、版本化、可验证的 Catalog/Policy 快照；签名失败、过期或缺失时
   按失败矩阵 fail closed。
7. `dc` 消费 `aegis.mcp.invocations.v1`，负责投影、跨调用序列规则、风险聚合和告警。
8. `api-server` durable AI worker 负责逐调用和 Activity 的异步语义分析；除调用已配置的 LLM
   provider API 外，AI 无工具、无 shell/file、无任意网络访问/搜索、无 MCP，不能改权限、审批、
   Policy、调用结果或执行动作。
9. Catalog Release、Server Revision、Tool Revision 不可变。上游漂移只能生成新 Revision，
   重新准入后再发布，不能原地覆盖已发布工具。
10. `approved` 不等于 `published`；只有 Catalog Release 发布后工具才对 Client 可见。
11. Client 只连接 `/mcp/v1/catalogs/{catalog_key}`，禁止提供默认“全部工具”endpoint。
12. 下游用户/Client Token 绝不传给上游；Gateway 使用独立上游 service credential 或经逐
    Client consent 的用户委托凭据。
13. 所有身份链必须区分 user、downstream client、catalog/release、tool revision、server
    revision 和 upstream service identity。
14. 上游 description、annotations、icons、resource link、Tool result 和 `_meta` 都是不可信数据。
15. 平台授权和工具控制只使用评审后的 verified metadata 与 Policy，不能信任上游自报
    `readOnlyHint` 或其他 annotation。
16. Server/Tool 准入审批、Catalog 发布审批和运行时调用审批是不同领域状态，禁止混用。
17. L3/L4 审批必须绑定 user、Client、Grant、Catalog Release、Tool Revision、Server Revision、
    effective arguments、目标资源和 Policy digest；任一变化使旧审批失效。
18. 未确认幂等性的写工具不得因网络超时自动重试。结果未知时记录 `outcome_unknown` 并进入调查。
19. 每个受理 `tools/call` 都必须产生规则结果以及 AI run 或明确的 disabled/degraded/failed 状态；
    AI pending/unknown 不能显示为安全。
20. AI 结论不能降低确定性规则风险；综合风险至少等于 deterministic floor。
21. 完整敏感 payload 使用信封加密存 MinIO；PostgreSQL 只保存索引、脱敏摘要、digest、对象
    reference 和状态；Kafka 不承载无限正文。
22. 无法建立 durable 审计恢复路径时，Gateway 不得返回虚假成功。
23. MCP 聚合管控不设置全局 feature flag，控制面、Gateway、DC 投影和前端入口默认可用；按 Server/Catalog/Client/Tool 的审批、Grant、Policy、发布和隔离状态完成安全停用。
24. 不删除历史 Revision、Release、Invocation、Approval、Rule、AI 或审计证据完成回滚。
25. 资产发现只能生成 candidate/finding，不能自动连接、读取 credential 或发布工具。

五、目标组件边界

新增 `mcp-gateway/`，建议保持以下职责分层，名称可按现有 Go 包风格微调：

  cmd/main.go
  internal/protocol/      MCP 版本、JSON-RPC、能力协商和兼容适配
  internal/downstream/    Aegis Catalog MCP endpoint
  internal/upstream/      Remote MCP Client、发现、调用、取消和超时
  internal/catalog/       已签名 Release/Tool 映射和原子快照
  internal/authn/         OAuth resource、Client、user、audience 和 consent
  internal/policy/        Grant、资源范围、速率、pre/post 决策
  internal/approval/      运行时审批门禁和 resume revalidation
  internal/audit/         四阶段 payload、digest、outbox 和 Kafka
  internal/transform/     input/output Schema、参数变换、脱敏和 quarantine
  internal/transport/     Streamable HTTP 与 legacy SSE adapter

`api-server/internal/mcpplatform/` 负责：overview、onboarding、Server/Revision/Tool、评审、
Catalog/Release、Client/Grant、Policy、Approval、调用查询、安全查询、快照签名和 AI worker。
Gin Handler 只做认证、参数解析、service 调用和安全 response，不把业务状态机堆在 Handler。

`dc/internal/pipeline/mcp/` 负责 Kafka 幂等消费、Invocation/Activity 投影、序列规则、综合风险、
告警和 WebSocket 元数据通知。不得把 raw payload 放入 WebSocket。

前端使用独立 API、类型、store 和组件，不扩展 legacy `frontend/src/api/assistant.ts` 的
`/assistant/mcp-sources`：

  frontend/src/api/mcpAggregation.ts
  frontend/src/types/mcpAggregation.ts
  frontend/src/store/mcpAggregation.ts
  frontend/src/views/settings/MCPAggregationControl.vue
  frontend/src/views/settings/mcp-aggregation/*

以仓库现有 `frontend/src/store/` 单数目录为准，不新建风格不一致的 `stores/`。

六、远程 Server 一键接入

控制面入口固定为：

  POST /api/v1/mcp-platform/onboarding-jobs

请求只接受：display name、remote HTTPS endpoint、auth type/credential ref、Owner team、environment、
target Catalog 和 publish policy，并要求 `Idempotency-Key`。一次性 secret 通过独立 Secret 写入
流程进入 Credential Store；数据库和普通 API 只保存 credential ref。

接入任务必须 durable、可恢复、可查询、可取消、可使用同一幂等键重试，状态至少覆盖：

  created
  validating_endpoint
  awaiting_auth
  authenticating
  discovering
  validating_tools
  security_scanning
  classifying
  building_release
  awaiting_approval
  publishing
  active
  failed
  cancelled

自动执行：endpoint/SSRF 检查、OAuth/credential 验证、版本协商、server/discover 或兼容 initialize、
tools/list 全量分页、Schema 和内容校验、工具 alias 消歧、协议/响应限制测试、安全扫描、风险分级、
创建不可变 Revision/Tool Revision、生成 Catalog Release 草稿、按风险等待审批或原子发布、启用
健康和漂移检测。

L1 可信内部只读模板可以自动审批和发布；L2-L4 必须停在 `awaiting_approval`。任一步失败只留下
Draft 和稳定错误码，不向 Client 暴露部分工具，不生成重复 Server/Revision/Release。

SSRF 控制必须覆盖：初始 URL、DNS A/AAAA、连接目标、每次 redirect、OAuth metadata/JWKS、
重解析和 DNS rebinding。生产拒绝 HTTP、userinfo、fragment、metadata、link-local、loopback、
未批准私网、重定向到禁区、无限响应、过深 JSON 和非法 content type。开发 loopback 例外必须
显式配置，默认关闭且不可带入生产。

七、控制面 API 和数据模型

API 根路径固定为 `/api/v1/mcp-platform`，至少实现设计文档定义的：

- `GET /overview`；
- onboarding job list/detail/retry/cancel；
- Server、Revision、Tool、discover、test、diff、submit-review、suspend、retire；
- admission review；
- Catalog、Release validate/create/diff/submit-review/publish/rollback；
- Client activate/suspend/revoke/credential rotate；
- Grant create/update/simulate/approve/revoke；
- runtime Approval list/detail/approve/reject/cancel；
- Invocation list/detail/events/redacted payload/payload reveal request；
- Activity graph、rule hits、AI analysis run/retry、security verdict。

所有列表使用服务端分页、稳定排序、tenant/resource scope。所有写 API 使用幂等键、expected version、
manifest/request digest 或等价的乐观并发控制。错误 envelope 至少包含稳定 `code`、安全 `message`、
`request_id` 和可选 `retry_after`，禁止返回上游堆栈、endpoint、credential 或原始结果。

数据库至少覆盖以下领域对象，字段、索引、约束和状态以 API/数据库设计文档为准：

- mcp_onboarding_jobs；
- mcp_servers、mcp_server_revisions、mcp_tool_revisions、admission reviews；
- mcp_catalogs、mcp_catalog_releases、mcp_catalog_release_tools；
- mcp_clients、mcp_client_grants；
- mcp_policy_sets、mcp_policy_versions；
- mcp_invocations、mcp_invocation_events、mcp_invocation_payload_refs；
- mcp_approval_requests 和 approver decisions；
- Kafka outbox/consumer idempotency；
- mcp_rule_definitions、mcp_rule_hits；
- mcp_ai_analysis_runs、mcp_ai_analysis_chunks；
- mcp_security_verdicts 和 Activity 投影。

Revision/Release/Review/Invocation/Approval/Rule/AI 数据不能通过普通级联删除。Server/Client 使用
retire/revoke；payload retention 删除对象时写 tombstone，legal hold 优先。不要使用 GORM
AutoMigrate 替代正式 SQL migration；如项目需要注册 model，只把它作为运行时映射。

八、下游 Gateway 调用顺序

`tools/call` 必须按以下顺序执行，不能为降低延迟重排安全边界：

1. HTTP/TLS/Origin/大小/协议版本和 JSON-RPC 验证；
2. 下游 OAuth issuer/audience/resource/expiry 与 Client 身份；
3. Catalog、已签名 Release、Tool alias 到固定 Tool Revision 的解析；
4. 当前 user/Client/Grant/Server/Tool/suspend/revoke 状态重校验；
5. input Schema、header/body 一致性、参数和资源范围约束；
6. pre-call deterministic rule 与 Policy；
7. rate/concurrency/cost budget；
8. 创建 Invocation 和 Client 请求 payload/digest；
9. 需要时创建审批并停止，批准恢复时从身份和版本开始重新验证；
10. 生成 effective upstream request，保存第二阶段 payload/digest；
11. Credential Broker 获取短期上游身份，调用固定 Server Revision；
12. 流式保存上游原始结果，形成第三阶段 payload/digest；
13. output Schema、大小、内容、secret、prompt injection 和 post policy；
14. allow/redact/truncate/quarantine；
15. 保存实际 delivered result，形成第四阶段 payload/digest；
16. 事务更新 Invocation/Events/Outbox 后向 Client 返回；
17. Kafka/DC/AI 异步分析、Activity 聚合和告警。

Client 已记住一个工具名也不能绕过当前有效集合直接 call。工具映射只能来自签名 Release 中的
`catalog_release_tool_id -> tool_revision_id -> server_revision_id -> upstream_name`，禁止解析用户
提交的字符串来临时选择 endpoint。

九、四阶段审计、安全规则和 AI

四阶段固定为：

1. `client_request`：Client 原始请求；
2. `effective_upstream_request`：策略变换后实际发往上游的请求；
3. `upstream_response`：上游原始返回；
4. `delivered_response`：平台实际交付给 Client 的结果。

每阶段保存 status、size、classification、digest、encrypted object ref、redacted summary 和完整性
状态。大 payload 必须流式限界和加密，禁止整体载入内存或写明文临时文件。Kafka 事件只包含 ID、
digest、状态、classification 和 opaque ref。

首批确定性规则至少覆盖：secret/credential、prompt injection、SQL/shell/path/header 注入、
危险资源范围、私密读取后外发、审批绕过、Schema/Revision drift、异常结果类型和高危工具序列。
pre-call deny 也必须形成 Invocation、审计和 AI 状态，不能无记录丢弃。

AI 输入只包含经授权、脱敏、限界、带 evidence ID 的结构化数据。工具结果始终是 untrusted data，
明确与 system instruction 隔离。AI worker 的网络权限只允许已配置的 LLM provider endpoint，禁止
任意网络请求、搜索和 MCP。AI 输出使用严格 schema，验证 verdict、severity、confidence、
evidence IDs、reason codes 和建议；引用不存在或越权 evidence 时结果为 inconclusive。数据库 run/
chunk/lease 是 durable queue 事实源，进程内 channel 只能 wake-up。provider timeout、rate limit、
invalid JSON、context overflow、worker restart 都必须产生可恢复或明确失败状态。

十、权限和凭据

实现设计文档中的细粒度权限，至少包括：

  mcp:onboarding:read/create/operate
  mcp:server:read/write/discover/review
  mcp:catalog:read/write/publish
  mcp:client:read/write
  mcp:grant:write
  mcp:approval:read/decide
  mcp:invocation:read
  mcp:audit:payload:read
  mcp:security:read
  mcp:security:ai:retry
  mcp:policy:read/write/publish
  mcp:break_glass

Owner 不能审批自己的 Revision；writer 不自动包含 reviewer/publisher；`full_access`、系统 admin、
Assistant 模型和前端隐藏按钮都不能隐式获得 MCP 细粒度权限。所有 Repository 查询必须落实 tenant、
team、user、Client 和 object scope，不允许只在 Handler 或 UI 过滤。

后端 `/auth/me` 或专用 capability endpoint 返回当前有效 capability 和版本/过期信息。前端建立统一
`can()` 和路由守卫，capability snapshot 只保存在内存并按版本/过期时间刷新，不把它作为长期
`localStorage` 授权依据；能力过期或 403 时清理旧权限，不自动重复写请求。后端仍是最终授权边界。

Credential Store/Broker 只接受 task-relevant credential ref；secret 创建和 rotate 只返回一次性
delivery handle 或状态，普通 JSON 不回显 secret。payload reveal、credential rotate、Break-glass
和高风险审批要求二次认证、purpose、短 TTL 和完整审计。

十一、前端要求

“系统配置”下恰好新增一个菜单：`MCP 聚合管控`。

- 路由：`/settings/mcp-aggregation`；
- route name：`MCPAggregationControl`；
- icon：复用 Element Plus `Connection`；
- MCP 聚合管控默认可用，不设置全局 feature flag；
- 入口 permission：`mcp:server:read`；
- 中英文 i18n 完整，禁止散落硬编码。

同一页面内部使用六个标签：远程服务、工具与发布、Client 授权、审批中心、调用审计、安全分析。
默认标签是远程服务，主按钮是“接入远程 MCP”。不要新增六个侧边栏菜单，也不要把页面放入
“智能资产采集 / MCP 资产”。

一键接入表单只显示远程字段；提交后显示 durable job 步骤。前端成功提示必须区分“任务已创建”、
“等待审批”、“已审批”和“已发布”，不得乐观伪造 active。

所有标签具备 loading、empty、error、permission denied、degraded 和 stale 状态；Server/Tool/Catalog/Grant 的停用状态必须明确展示。
规则和 AI 分列展示；analysis pending/degraded/unknown 不使用绿色。Server、Tool、Release、Grant、
Approval 和 Invocation 详情均显示稳定 ID、版本/digest 和准确状态。

调用审计详情按四阶段展示，默认只有脱敏摘要。raw payload reveal 要求专门权限、purpose、二次认证
和短期内存查看器；secret、endpoint、arguments、result、payload、审批理由不得进入 URL、Pinia
持久化、localStorage/sessionStorage/IndexedDB、console、埋点、通知或 WebSocket。

SafePayloadViewer 只能渲染纯文本和结构树，不执行 HTML/Markdown/脚本/链接预览，不自动加载
ResourceLink、image、icon 或任意外部 URL。超大 JSON 使用大小上限和虚拟滚动。

十二、可观测性要求

使用项目现有 Zap 和稳定 snake_case 事件。按 daily-program-logging 设计以下领域事件：Gateway
启动/停止、快照加载/拒绝、onboarding 各阶段、发现、漂移、评审、Release 发布/回滚、Client/Grant、
审批创建/决定/失效、调用拒绝、上游失败/结果未知、结果隔离、outbox 延迟、审计完整性失败、AI
完成/失败。

日志只允许 ID、digest、count、status、risk tier、error code、attempt、latency 和安全 hash。禁止
Authorization、Cookie、token、API key、password、private key、client secret、完整 endpoint、
credential ref 敏感路径、arguments、result、Schema 大对象、用户正文、MinIO URL、DEK、OAuth
code/state/PKCE verifier。使用 canary fixture 捕获 success/deny/error/retry/approval/AI 日志并断言
0 泄漏。同一错误在最能说明结果的一层记录一次，高频成功调用使用审计事件和采样日志。

指标 label 禁止 user ID、invocation ID、endpoint、arguments 等高基数字段。Trace 使用 W3C
traceparent/tracestate，baggage allowlist，不把下游不可信 baggage 原样传给上游。

十三、开发阶段和每阶段门槛

严格按 P0-P5 实施。每阶段开始前先列出影响文件、API、数据、测试和资源级回滚动作；优先先写能失败的
行为测试；阶段完成后运行最窄有效测试和受影响组件构建，复核 diff，再进入下一阶段。

P0：契约、migration、RBAC、控制面骨架和前端真实空状态

- 固化 canonical JSON/digest、协议内部接口、错误码、状态枚举和配置；
- 选择并固定官方 Tier 1 Go SDK 兼容版本；验证许可证、SBOM、CVE、协议 fixture，不盲目复制私有
  MCP 类型；若当前官方版本与设计稿不同，先记录兼容决策，不静默改协议；
- 新增不冲突的正式 migrations、models、repositories、tenant scope 和 capability；
- 实现 overview、onboarding/Server/Revision/Tool、Review、Catalog/Release、Client/Grant、Policy、
  Approval 的安全 CRUD 和状态机骨架；
- 实现签名 Catalog/Policy bundle 的生成、验证、版本和 diff；
- 新增 `mcp-gateway` 可构建的服务骨架、health/readiness 和默认关闭配置；
- 前端增加唯一菜单、路由、capability 守卫、六个标签和 mock-free 空/错误状态；
- 验证不可变对象不能原地 Update，Owner/Reviewer/Publisher 分离，资源暂停/撤销后无越权调用。

P1：真实远程发现、一键接入和只读准入

- 实现 Remote Streamable HTTP Client、迁移期 legacy SSE、版本协商和 tools/list 分页；
- 必须执行真实 MCP discover/initialize 和 tools/list，不得用 `/ping`、固定 schema 或 mock 成功
  代替协议发现；
- 实现 durable onboarding worker、OAuth waiting/resume、幂等重试和稳定失败恢复；
- 实现 SSRF/DNS/redirect/Origin/response limit、Schema/annotation/content 检查；
- 实现风险分级、健康、Revision diff、drift/quarantine；
- 前端完成远程服务列表、接入表单、进度和 Server 详情；
- 验证一次提交完成全流程，失败无部分发布，L1 可自动发布，L2-L4 等待审批。

P2：下游 Gateway、Client/Grant、只读调用和四阶段审计闭环

- 实现 OAuth Resource Server、Catalog endpoint、server/discover、tools/list、tools/call；
- 实现有效工具集合、稳定 alias、input/output Schema、timeout/cancel；
- 实现签名快照原子加载、Credential Broker、四阶段 MinIO payload、PostgreSQL metadata、Kafka
  outbox 和恢复；
- Assistant 作为首个内置只读 Client 灰度接入；
- 前端完成工具/发布、Client/Grant、调用审计摘要和四阶段查看器；
- 验证撤销 Grant 后记住旧工具名仍无法调用，四阶段对账 100%，审计依赖故障不虚假成功。

P3：写工具、Policy、运行时审批和 payload reveal

- 实现 L3/L4 verified side effects、资源范围、pre/post transform、限速/并发/成本；
- 实现 MCP 能力协商下的 InputRequired/Task/兼容 approval-required 错误；
- 实现组织/双人/Break-glass 审批、resume 全量重校验、审批过期/失效和非幂等防重；
- 实现 post-call redact/truncate/quarantine、受审计 payload reveal；
- 前端完成审批中心、Release diff/publish/rollback、高风险确认和短期 reveal；
- 验证改参数/目标/身份/版本/digest 使审批失效，timeout unknown 不重试，原始隔离结果不返回 Client。

P4：规则、AI、Activity、告警和安全分析前端

- 实现 pre/post/sequence 规则、DC 幂等投影和 Activity 调用链；
- 实现 durable AI runs/chunks/lease、结构化 verdict、evidence ownership、risk reducer 和成本状态；
- 实现规则/AI/综合风险查询、告警和只含 metadata 的 WebSocket；
- 前端完成安全分析、规则/AI 分离、Activity 和证据跳转；
- 验证 prompt injection 不改变 worker 行为，伪造 evidence 为 inconclusive，AI safe 不降低规则风险，
  每个调用最终有完整或明确降级状态。

P5：v6.0 迁移、旁路治理、性能、灰度、回滚和离线发布

- 导入旧 remote source 为 draft，不自动信任 schema cache，不伪造缺失 raw payload；
- Assistant 切换到 Gateway，先做审计对账，确保新旧不双执行；
- command/stdio 资产只显示本期不支持；旁路发现只形成 finding，不改用户配置；
- 完成容量、故障注入、OAuth、SSRF、secret leak、兼容、安全、灾备和 E2E；
- 更新 docker-compose、Nginx、.env.example、health、metrics、migration 初始化和离线发布包；
- 按 shadow -> 内部只读 -> 扩大只读 -> 单个 L3 写工具灰度；
- 验证 Catalog 回滚、Gateway/Policy bundle 回滚、资源暂停和旧路径临时回退不丢证据、不双执行。

十四、最低测试矩阵

1. Protocol：主版本与兼容版本、initialize/discover、tools/list cursor、tools/call、cancel、SSE、
   Header/Body 冲突、JSON-RPC error 与 tool error。
2. Canonicalization：JSON 字段顺序、Unicode、number/null、Schema、Release/Policy/arguments digest。
3. Onboarding：OAuth waiting、API key ref、网络失败、worker restart、幂等并发、取消、失败重试、
   L1 自动发布、L2-L4 审批、无部分发布。
4. Network：metadata/link-local/loopback/private IP、redirect、DNS rebinding、OAuth metadata/JWKS SSRF、
   Origin、超大/深层/慢响应。
5. Catalog/Grant：重名 alias、固定/滚动 Release、过期/撤销/suspend、旧缓存直调、tenant 隔离、
   verified metadata 不被漂移覆盖。
6. Approval/Policy：allow/transform/confirm/approval/deny/quarantine、参数绑定、quorum、职责分离、
   expiry、Break-glass、resume reauthorization、非幂等 exactly-once attempt、outcome unknown。
7. Audit：四阶段 object/digest/ref/event/outbox 一一对应、流式加密、secret 抑制、依赖故障恢复、
   重放/乱序/缺项/hash mismatch、retention/legal hold/tombstone/reveal audit。
8. Rule/AI：pre/post/sequence、prompt injection、secret、private-read-then-exfiltrate、invalid JSON、
   timeout/rate limit/context/worker restart、fake evidence、AI downgrade attempt、漏 run 对账。
9. API/RBAC：所有 capability、tenant/resource scope、分页排序、idempotency、expected digest、403/409/
   429/5xx、安全错误、二次认证和 secret 0 回显。
10. UI：唯一菜单、路由守卫、一键接入无 stdio 字段、所有状态、旧响应竞态、状态文案、XSS、
    SafePayloadViewer、敏感状态清理、规则/AI 分离、中英文、窄屏和键盘导航。
11. E2E：只读正常链路、撤销后直调、Schema 漂移、高风险写审批、上游提示注入、Token 隔离、
    MinIO/Kafka/PostgreSQL 故障、一键接入原子发布、v6.0 迁移、Gateway 旁路检测。
12. 性能：1,000 Tools/Catalog 的 list、500 RPS 治理开销、10 MiB 流式结果、Kafka 中断 outbox、
    100 Gateway 快照原子切换、10 万 AI backlog、单 Activity 1,000 调用不出现 O(n²)。

fixture 全部使用合成数据和测试 canary，不使用真实 endpoint、凭据、用户 payload 或生产数据。
测试失败输出也必须脱敏。

十五、验证要求

按 `/code/aegis/.agents/skills/aegis-build-test/SKILL.md` 选择最窄且充分的验证组合：

- api-server：相关 Go 包测试，扩大时 `go test ./...`，并执行 `make build`；
- mcp-gateway：相关 Go 包、协议 fixture、race/并发测试和独立构建；
- dc：MCP pipeline 定向测试、必要时全模块测试和 `make build`；
- frontend：定向 Vitest、`npm run type-check`、`npm run lint`、`npm run build`；
- migration：空库升级、现有数据升级、重复执行、约束/索引、回滚演练；
- Compose：只重建受影响服务，验证 health/readiness、PostgreSQL、Redis、Kafka、MinIO 和 Gateway；
- 协议/跨服务变化：执行真实 mock Remote MCP Server -> Gateway -> Kafka/DC -> PostgreSQL/MinIO ->
  API/Frontend 的 E2E；
- 发布阶段：按 `aegis-release-packaging` 验证 Linux AMD64 离线镜像、migration、配置和启动脚本。

不得用一个全栈 build 代替定向测试，也不得因环境缺失宣称验证通过。记录每条实际命令、结果、
耗时和未执行原因。禁止为通过测试吞错、放宽安全断言、改成假成功、把 unknown/null 当安全/0、
跳过审计或降低既定测试期望。

十六、每阶段复核

1. git diff 只有任务相关改动，不覆盖并行工作；
2. migration/model/repository/API/types/status/error code 完全一致；
3. Catalog/Revision/Approval digest 与前后端展示一致；
4. MCP 聚合管控默认可用，按资源审批/策略停用后现有 Aegis 功能无回归；
5. Client Token 未传上游，credential/payload canary 全局泄漏搜索为 0；
6. 四阶段 payload/digest/outbox/event 可对账；
7. approved/published、pending/succeeded、unknown/safe 没有混淆；
8. AI 无工具、不能降权、不能授权或执行；
9. 非幂等写无重复上游调用；
10. 文档、配置、健康检查、指标、日志和回滚说明与代码同步。

十七、停止条件和回滚原则

发现以下任一情况立即停止扩量：未审计成功调用；四阶段证据无法对账；Client Token 传上游；
credential/payload 泄漏；Grant 撤销后仍可调用；审批参数替换或重复执行；漂移未隔离；已发布
Release 被原地修改；审计依赖失败仍返回成功；AI/规则被用于降低确定性风险；onboarding 失败后
暴露部分工具或产生重复对象。

逻辑回滚顺序：暂停受影响 Server/Tool/Catalog -> 对账 outcome unknown 写调用 -> Catalog 指针切回
上一签名 Release -> Gateway/Policy bundle 切回上一验证版本 -> 暂停受影响资源 -> 必要时让
Assistant 临时回到旧只读路径但保持调用 ID 对账。保留所有审计和分析证据。禁止 DROP 审计表、
删除 PostgreSQL volume/MinIO bucket、恢复泄漏凭据、手改失败调用为成功或改成无审计 fail-open 代理。

十八、最终交付报告

每阶段报告：用户可见结果、实现范围、关键设计/安全边界、修改文件、API/migration/config 变化、
实际测试与构建命令及结果、未运行验证、已知风险、下一阶段和资源级回滚动作。

最终只有同时满足以下条件，才能说“MCP 聚合管控 V6.3 已完成”：

1. Remote Server 一键接入、真实协议发现、风险分级、审批、原子发布、漂移隔离和回滚闭环；
2. Client 只连接 Aegis Catalog endpoint，Grant/suspend/revoke 不可绕过；
3. OAuth identity chain、上下游 Token 隔离、credential rotation 和 consent 通过安全测试；
4. L1-L4 工具控制、运行时审批绑定和非幂等防重通过；
5. 四阶段证据对账率 100%，canary 明文泄漏数为 0；
6. 每次受理调用有规则与 AI run/明确失败状态，AI 不能降低规则风险；
7. MinIO/Kafka/Redis/PostgreSQL/控制面/上游/AI 故障符合失败矩阵；
8. “系统配置”下只有一个“MCP 聚合管控”入口，完整前端状态和安全查看器通过测试；
9. v6.0 Remote MCP 与 Assistant 完成兼容迁移且无双执行；
10. api-server、mcp-gateway、dc、frontend 的定向测试、构建、跨服务 E2E、容量、安全和离线发布
    验证通过；
11. 灰度稳定窗口内 SLO、审计完整率、规则/AI backlog 和成本符合门槛；
12. 文档和实际实现状态一致。

如果只完成设计、骨架、前端页面或某一个阶段，必须准确报告“完成 Pn，后续阶段未实现”，不得
把 mock、占位 `/ping`、旧 ExternalMCP 路径或仅 UI 展示描述为聚合平台已完成。
````

## 3. 建议分阶段使用方式

如果希望将主提示词拆成多个开发任务，可在每个任务中附上第 2 节，并分别追加以下一句：

1. `只实现 P0：契约、迁移、RBAC、控制面/Gateway 骨架和前端真实空状态；通过门槛后停止。`
2. `在已验收 P0 上只实现 P1：远程发现、一键接入、准入、健康和漂移；通过门槛后停止。`
3. `在已验收 P0-P1 上只实现 P2：Gateway 只读调用、Client/Grant 和四阶段审计；通过门槛后停止。`
4. `在已验收 P0-P2 上只实现 P3：写工具、Policy、运行时审批和 payload reveal；通过门槛后停止。`
5. `在已验收 P0-P3 上只实现 P4：确定性规则、AI、Activity、告警和安全分析前端；通过门槛后停止。`
6. `在已验收 P0-P4 上只执行 P5：v6.0 迁移、性能、安全、灰度、回滚和离线发布验证。`

每个后续阶段开始前必须读取上一阶段真实交付报告、当前 git diff、migration 和 API，不得只依赖
最初提示词假设代码状态。
