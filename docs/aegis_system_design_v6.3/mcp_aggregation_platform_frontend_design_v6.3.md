# Aegis V6.3 MCP 聚合管控前端设计

- **版本**：V6.3 方案版
- **日期**：2026-08-11
- **状态**：设计完成，尚未实现
- **依赖文档**：
  - [MCP 聚合治理平台总体设计](mcp_aggregation_governance_platform_design_v6.3.md)
  - [MCP 聚合平台 API、协议与数据库设计](mcp_aggregation_platform_api_database_design_v6.3.md)
  - [MCP 聚合平台实施、测试与发布设计](mcp_aggregation_platform_implementation_test_rollout_v6.3.md)

## 1. 设计结论

V6.3 在现有“系统配置”侧边栏下只新增一个入口，固定命名为 **MCP 聚合管控**。
该入口对应一个路由和一个聚合页面，不把 Server、Tool、Client、审批、审计和安全分析
拆成更多侧边栏菜单。

```text
系统配置
├── 模型配置
├── Agent 安装
├── 命令审计配置
├── 审计日志
├── eBPF Hook 白名单
└── MCP 聚合管控              <- V6.3 新增且唯一的新入口
```

页面默认展示远程 MCP Server 列表，主操作为“接入远程 MCP”。V6.3 只支持远程
MCP Server，不出现 stdio、command、本地进程或 Runner 的接入选项。

“智能资产采集 / MCP 资产”继续表示 Agent 发现的资产线索；“MCP 聚合管控”表示经过
平台登记、准入、审批、发布并可供 Client 调用的服务。二者不共用页面、路由或状态文案。

## 2. 与现有前端的集成点

### 2.1 菜单与路由

| 项目 | 设计值 |
| --- | --- |
| 侧边栏父级 | `系统配置` |
| 中文菜单 | `MCP 聚合管控` |
| 英文菜单 | `MCP Aggregation Control` |
| 图标 | 复用 Element Plus `Connection` |
| 路由 | `/settings/mcp-aggregation` |
| 路由名 | `MCPAggregationControl` |
| 标题 Key | `routes.mcpAggregationControl` |
| 页面组件 | `views/settings/MCPAggregationControl.vue` |
| 入口权限 | `mcp:server:read` |
| Feature Flag | 不设置；MCP 聚合管控默认可用 |

路由使用懒加载，避免平台页面及 JSON/Schema 查看组件进入所有页面的首屏包：

```ts
{
  path: '/settings/mcp-aggregation',
  name: 'MCPAggregationControl',
  component: () => import('../views/settings/MCPAggregationControl.vue'),
  meta: {
    titleKey: 'routes.mcpAggregationControl',
    permission: 'mcp:server:read'
  }
}
```

当前全局路由守卫只校验登录和强制修改密码，`meta.permission` 尚未形成统一拦截；因此实现
本页面前，必须补齐统一的 capability 获取和路由校验。隐藏菜单只是体验优化，后端 API
仍是最终授权边界。

### 2.2 国际化

至少新增以下文案 Key，页面正文也必须进入 `zh-CN`、`en-US` 资源，禁止在组件散落硬编码：

```text
app.menu.mcpAggregationControl
routes.mcpAggregationControl
mcpAggregation.*
```

## 3. 页面信息架构

页面内部使用 `el-tabs` 组织六个工作区；这些是同一路由内的内容标签，不增加侧边栏入口。
当前标签通过安全的 `?tab=` 查询参数保存，刷新后可恢复。

| 内部标签 | Key | 主要对象 | 默认权限 |
| --- | --- | --- | --- |
| 远程服务 | `servers` | 接入任务、Server、Revision、健康与漂移 | `mcp:server:read` |
| 工具与发布 | `catalogs` | Tool Revision、Catalog、Release、Diff | `mcp:catalog:read` |
| Client 授权 | `clients` | Client、Grant、Scope、配额和有效期 | `mcp:client:read` |
| 审批中心 | `approvals` | 准入审批、发布审批、调用审批 | `mcp:approval:read` |
| 调用审计 | `invocations` | 调用列表、四阶段证据和 Trace | `mcp:invocation:read` |
| 安全分析 | `security` | 规则、AI Verdict、Activity 和告警 | `mcp:security:read` |

无某个内部标签权限时不渲染该标签，也不发起其 API 请求。用户拥有入口权限但没有任何
附加域权限时，只能查看“远程服务”。

### 3.1 桌面端线框

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│ MCP 聚合管控                                      [ 接入远程 MCP ]          │
│ 统一完成远程 Server 准入、工具发布、Client 授权、审计和安全分析            │
├───────────┬───────────┬───────────┬───────────┬─────────────────────────────┤
│远程服务 12│已发布工具48│有效Client 7│待审批  3 │24h 高危调用 2              │
├─────────────────────────────────────────────────────────────────────────────┤
│ [远程服务] [工具与发布] [Client 授权] [审批中心 3] [调用审计] [安全分析]   │
├─────────────────────────────────────────────────────────────────────────────┤
│ 关键字 [     ] 环境 [全部] 状态 [全部] 风险 [全部] 健康 [全部] [查询][重置]│
├─────────────────────────────────────────────────────────────────────────────┤
│ 服务名称 │ 环境 │协议/认证│工具│风险│准入/发布│健康/漂移│最近同步│ 操作 │
│ Finance   │生产 │HTTP/OAuth│  8 │ L2 │ 已发布  │  正常   │ 2分钟前 │详情… │
│ Ops       │测试 │HTTP/API  │ 12 │ L3 │ 待审批  │  漂移   │ 1小时前 │详情… │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                        共 12 条  < 1 2 >   │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 4. 页面公共区域

### 4.1 Page Hero

- 标题：`MCP 聚合管控`；
- 描述：`统一接入远程 MCP Server，控制工具发布和 Client 授权，并审计每次调用。`；
- 主按钮：`接入远程 MCP`，只有 `mcp:onboarding:create` 时显示；
- 次级状态：控制面或 Gateway 降级时显示全页 `el-alert`，不能只用 toast；
- 不显示上游凭据、完整 endpoint 或 payload。

### 4.2 顶部指标

首屏显示五个可点击指标卡，点击后切换到相应标签并应用筛选：

1. 远程服务数；
2. 已发布工具数；
3. 有效 Client 数；
4. 待我审批数；
5. 24 小时高危调用数。

指标带更新时间。部分数据源不可用时显示 `--` 和“数据暂不可用”，不能把未知值显示为 0。

### 4.3 页面状态

每个标签独立处理以下状态，切换标签不能被另一个标签的错误覆盖：

- `loading`：表格骨架屏，首次加载不展示伪数据；
- `empty`：说明原因并给出当前用户可执行的下一步；
- `error`：显示稳定错误码、请求 ID 和重试按钮；
- `permission_denied`：说明缺少的业务权限，不自动重复请求；
- `degraded`：保留已缓存数据并显示数据时间和不可用能力；
- `stale`：数据刷新失败时显式标记“可能已过期”；
- `feature_disabled`：直接访问路由时展示未启用页，不渲染写按钮。

## 5. 远程服务标签

这是页面默认标签，承载“一键接入”主流程。

### 5.1 筛选和列表

筛选项：关键字、环境、协议版本、认证方式、风险等级、准入/发布状态、健康/漂移状态、
Owner。筛选、分页和排序同步到 URL，但 endpoint、认证信息和错误详情不得进入 URL。

| 列 | 展示规则 |
| --- | --- |
| 服务名称 | 名称、短 ID；点击打开详情抽屉 |
| Endpoint | 只显示 `scheme + host + 受控路径摘要`，查询串和 userinfo 永不展示 |
| 环境 | 开发、测试、生产 |
| 协议/认证 | Streamable HTTP 或迁移期 SSE；OAuth/API Key/Bearer/None |
| 工具 | 已发现总数 / 已发布数 |
| 风险 | L1-L4；Unknown 不能显示为低风险 |
| 状态 | 区分准入、审批和发布三个状态，不合并成一个“正常” |
| 健康/漂移 | 正常、降级、不可用、已漂移、隔离 |
| 最近同步 | 绝对时间 + 相对时间；标出是否过期 |
| 操作 | 详情、重试发现、连通性测试、提交审批、暂停等按状态显示 |

列表状态文案必须保持领域语义：`approved` 不能显示为“已发布”，`publishing` 不能显示为
“已生效”，`quarantined` 不能显示为普通离线。

### 5.2 一键接入远程 MCP

点击主按钮打开右侧 `el-drawer`。接入表单保持单屏完成，只包含远程接入所需信息：

| 字段 | 必填 | 规则 |
| --- | --- | --- |
| 服务名称 | 是 | 租户内唯一，2-64 字符 |
| Remote MCP Endpoint | 是 | 生产仅允许 HTTPS；拒绝 userinfo、fragment 和非法 URL |
| 认证方式 | 是 | OAuth 2.1、API Key、Bearer；`None` 仅开发环境可选 |
| 凭据 | 视认证方式 | 优先选择 Credential Ref；新 secret 只能一次性提交，不回显 |
| Owner 团队 | 是 | 从当前租户可选团队中选择 |
| 环境 | 是 | 开发、测试、生产 |
| 目标 Catalog | 是 | 选择已有 Catalog 或仅创建待发布草稿 |
| 发布策略 | 是 | `完成审批后发布`；仅合规 L1 内部只读可选自动发布 |

表单明确不包含 `command`、`args`、`cwd`、`env`、`stdio`、本地文件选择和 Runner。

提交使用 `POST /api/v1/mcp-platform/onboarding-jobs`，请求头携带前端生成的
`Idempotency-Key`。按钮在请求期间禁用；网络重试复用原 Key，防止双击或超时产生两个
Server。成功标准是“接入任务已创建”，不是“服务已发布”。

OAuth 类型提交后进入 `等待授权`，提供“去授权”按钮。授权窗口返回只携带短期 state，
不得把 access token 写入 URL、`localStorage` 或页面日志。

### 5.3 接入进度抽屉

任务创建后，原抽屉切换为步骤进度，不要求用户停留等待：

```text
校验 Endpoint -> 认证/授权 -> 发现能力与工具 -> Schema 校验
 -> 安全扫描与分级 -> 生成 Release -> 等待审批 -> 发布 -> 生效
```

状态映射：

| 后端状态 | UI 表现 | 可用操作 |
| --- | --- | --- |
| `validating_endpoint` | 校验远程地址 | 取消 |
| `awaiting_auth` | 等待 OAuth 授权 | 去授权、取消 |
| `authenticating` | 验证上游凭据 | 取消 |
| `discovering` | 获取 capability 和工具 | 取消 |
| `validating_tools` | 校验名称和 Schema | 取消 |
| `security_scanning` / `classifying` | 安全扫描和风险分级 | 取消 |
| `building_release` | 生成不可变候选 Release | 查看已发现工具 |
| `awaiting_approval` | 等待准入/发布审批 | 查看审批、撤回 |
| `publishing` | 正在发布 | 只读，不允许重复提交 |
| `active` | 已发布且可被授权 | 查看服务、进入工具与发布 |
| `failed` | 显示稳定失败阶段和原因 | 修正配置、重试、取消 |
| `cancelled` | 已取消，保留审计记录 | 返回列表 |

进度通过任务查询接口轮询；前台 2 秒起步并指数退避，页面不可见时降频。WebSocket 只作为
刷新提示，不承载 secret 或完整任务对象。收到完成事件后必须重新读取任务和 Server，不能
依赖事件内容直接拼出“已发布”。

### 5.4 Server 详情抽屉

详情使用 72-80% 宽度抽屉，内部标签如下：

- **概览**：Owner、环境、脱敏 endpoint、transport、auth、protocol、风险、状态、健康、SLA；
- **工具**：发现工具、平台 alias、Schema 状态、风险、是否发布；
- **版本与 Diff**：不可变 Revision、Schema/annotations/capability diff；
- **健康与漂移**：最近探测、错误率、p95、凭据过期、漂移和隔离原因；
- **调用与安全**：调用量、错误率、规则/AI 风险趋势及最近高危调用。

高风险动作使用确认对话框并显示影响对象；暂停、隔离、回滚、重新准入不能共用模糊的
“确定操作”文案。

## 6. 工具与发布标签

上半区是 Catalog 选择和当前 Release 摘要，下半区为 Tool Revision 列表。

工具列表至少展示：平台 alias、上游 Server、风险等级、副作用、输入/输出 Schema 状态、
审批模式、发布状态、获授权 Client 数、最近漂移和最近调用时间。

发布工作流：

1. 从已准入 Tool Revision 选择工具；
2. 配置平台 alias、verified metadata、资源范围、限速、审批模式和输出规则；
3. 展示相对上一 Release 的增加、删除、Schema、风险和权限 Diff；
4. 提交发布审批，页面记录审批 digest；
5. 审批通过后由有发布权限的用户发布；
6. 发布成功后显示新的不可变 Release ID 和 Catalog endpoint。

回滚是把 Catalog 指针切回已有不可变 Release，不能在前端编辑历史 Release。Catalog
endpoint 只可复制，Client 永远看不到上游 endpoint 或 credential。

## 7. Client 授权标签

Client 列表展示：名称、Client 类型、目标 Catalog、授权工具数、Scope/资源约束、配额、
有效期、凭据状态、最近调用和状态。

“新增/编辑授权”抽屉必须让用户明确选择：

- Catalog 和固定 Release 或允许跟随最新已批准 Release；
- Tool allowlist；
- 用户/角色/资源边界；
- QPS、并发、日调用量和最大响应大小；
- 生效和过期时间；
- 是否允许触发运行时审批。

保存前显示授权摘要和影响范围。撤销 Grant 后 UI 立即标记 `revoked`，但只有网关确认快照
刷新后才显示“已生效”；不得仅因前端保存成功就宣称调用已经被阻断。

## 8. 审批中心标签

审批中心用二级分段筛选展示“准入审批 / 发布审批 / 调用审批”，并支持“待我审批、我已处理、
全部”视图。待审批数量同步到内部标签 badge 和顶部指标。

调用审批卡至少展示：调用用户、Client、Catalog Release、Tool Revision、脱敏参数摘要、
资源目标、风险、规则命中、调用原因、有效期和审批 digest。审批按钮只有
`mcp:approval:decide` 时可见。

- 批准/拒绝必须填写原因；
- L4 或策略要求双人审批时展示当前票数和职责分离约束；
- 参数、目标、Revision 或 Policy digest 变化后显示“审批已失效”；
- 过期、撤销、已执行的审批不能再次操作；
- UI 不允许编辑审批绑定内容后沿用原审批。

## 9. 调用审计标签

### 9.1 列表

筛选项：时间、Client、用户、Server、Tool、Catalog/Release、执行状态、规则风险、AI 状态、
Trace ID。列表显示 invocation ID、调用方、工具、风险、审批、上游结果、交付结果、规则、
AI、耗时和时间。

规则风险和 AI Verdict 使用两个独立列。`analysis_pending`、`degraded`、`unknown` 使用灰/橙色，
绝不能使用绿色安全标签。

### 9.2 四阶段证据抽屉

调用详情固定按时间线展示：

1. Client 原始请求；
2. 平台策略处理后的实际上游请求；
3. MCP Server 原始返回；
4. 平台脱敏/裁剪后实际交付给 Client 的结果。

默认只展示脱敏摘要、digest、对象大小、保存状态和 object ref。查看完整 payload 需要
`mcp:audit:payload:read`、填写 purpose、二次认证并获取短期 reveal session；查看器超时自动
清空，不缓存到 Pinia 持久化、URL、console、埋点或 WebSocket。

JSON/Schema 查看器只渲染纯文本和结构树，不执行 HTML、Markdown、链接预览或外部资源加载。
`ResourceLink` 默认不可直接访问；若策略允许，先显示目标域名和安全确认。

## 10. 安全分析标签

页面把确定性规则和 AI 分析明确拆开：

- 顶部：综合风险分布、规则高危数、AI 高危数、分析待处理/降级数；
- 左侧：规则命中趋势、规则类型、phase、严重度；
- 右侧：AI Verdict 趋势、模型/版本、置信度、待处理时长；
- 下方：高危 Invocation/Activity 列表，可跳转到调用证据；
- 详情：综合结论如何由规则和 AI 证据得出，并标明谁不能覆盖谁。

AI 只提供安全分析，不展示“AI 批准调用”按钮。AI 不可用时规则结果仍正常显示，综合状态为
“AI 分析降级/待处理”，不能将其解释为安全。

## 11. 前端数据与组件设计

### 11.1 建议文件边界

```text
frontend/src/
├── api/mcpAggregation.ts
├── types/mcpAggregation.ts
├── store/mcpAggregation.ts
├── views/settings/MCPAggregationControl.vue
└── views/settings/mcp-aggregation/
    ├── RemoteServerTab.vue
    ├── MCPOnboardingDrawer.vue
    ├── MCPOnboardingProgressDrawer.vue
    ├── MCPServerDetailDrawer.vue
    ├── ToolCatalogTab.vue
    ├── ClientGrantTab.vue
    ├── ApprovalCenterTab.vue
    ├── InvocationAuditTab.vue
    ├── MCPSecurityTab.vue
    ├── StatusTag.vue
    └── SafePayloadViewer.vue
```

新页面使用独立 `mcpAggregation` API 和类型，不扩展现有 `api/assistant.ts` 的 legacy
`/assistant/mcp-sources`。后者是 V6.0 Assistant 外部数据源兼容路径，不是 V6.3 平台事实源。

### 11.2 Store 边界

Store 只保存：

- 当前标签、非敏感筛选、分页和排序；
- 列表摘要、指标和更新时间；
- 当前选中的 UUID；
- 各标签独立的 loading/error/stale 状态。

Store 不保存 secret、OAuth token、完整 endpoint、原始请求/响应或 reveal payload。抽屉关闭、
退出登录、reveal session 过期时清理内存中的敏感数据。

同一筛选连续请求使用 `AbortController` 或递增 request sequence，丢弃旧响应，避免用户快速
切换 Server/Client 后把旧详情渲染到新对象。写操作成功后按服务端返回版本重新读取，不在
本地乐观伪造审批或发布状态。

### 11.3 URL 状态

允许进入 URL 的字段：

```text
tab, page, page_size, sort, environment, status, risk, owner_id,
server_id, client_id, invocation_id, time_range
```

禁止进入 URL 的字段：endpoint、secret、credential ref 明文、arguments、result、payload、
审批理由、用户输入正文和错误堆栈。

## 12. API 映射

前端以 API/数据库设计文档为最终契约，主要映射如下：

| 前端能力 | API 组 |
| --- | --- |
| 指标 | `GET /api/v1/mcp-platform/overview` |
| 一键接入 | `POST /api/v1/mcp-platform/onboarding-jobs` |
| 接入任务 | `GET /onboarding-jobs/{id}`、`POST /retry`、`POST /cancel` |
| Server | `GET /servers`、`GET /servers/{id}`、Revision、Tool、Diff、Test、Suspend |
| Tool/Catalog | `GET /tools`、Catalog、Release、Diff、Publish、Rollback |
| Client/Grant | Client Registry、Grant create/update/revoke |
| 审批 | Approval list/detail/approve/reject/withdraw |
| 调用审计 | Invocation list/detail/events/payload reveal |
| 安全分析 | rule verdict、AI run/verdict、activity、alert |

所有列表 API 返回服务端分页和稳定排序；错误结构至少包含 `code`、安全的 `message`、
`request_id` 和可选 `retry_after`。前端不能向用户直接展示上游错误正文。

## 13. 权限设计

### 13.1 页面权限矩阵

| 权限 | 前端能力 |
| --- | --- |
| `mcp:server:read` | 看见菜单、进入页面、查看 Server |
| `mcp:onboarding:read` | 查看有权访问的接入任务和进度 |
| `mcp:onboarding:create` | 创建远程接入任务 |
| `mcp:onboarding:operate` | 重试、取消、继续 OAuth |
| `mcp:server:review` | 提交/执行准入相关操作 |
| `mcp:catalog:read` | 查看 Tool、Catalog、Release |
| `mcp:catalog:publish` | 提交发布、发布、回滚 |
| `mcp:client:read` / `mcp:client:write` | 查看/管理 Client |
| `mcp:grant:write` | 创建、修改、撤销 Grant |
| `mcp:approval:read` / `mcp:approval:decide` | 查看/处理审批 |
| `mcp:invocation:read` | 查看调用摘要和四阶段元数据 |
| `mcp:audit:payload:read` | 发起受审计的 payload reveal |
| `mcp:security:read` | 查看规则、AI、Activity 和告警 |

登录会话需要由后端返回 `capabilities` 或通过 `/me/capabilities` 获取；前端提供统一 `can()`，
路由、菜单、标签和按钮使用同一事实源。Capability 过期或后端返回 403 时立即撤销本地可见写
能力。任何按钮隐藏都不能替代服务端鉴权、tenant scope 和资源级授权。

### 13.2 高风险操作

发布、回滚、暂停、隔离、撤销 Grant、批准和拒绝必须：

- 显示准确对象、当前版本和影响；
- 使用后端版本/digest 做并发控制；
- 防止重复提交；
- 对 409 显示“对象已变化，请刷新后重试”；
- 对 403 清理旧 capability 并展示权限变化；
- 不在前端绕过双人审批或职责分离。

## 14. 响应式与可访问性

- 大于等于 1280 px：指标五列、完整表格、详情抽屉 72-80%；
- 768-1279 px：指标两至三列、内部标签横向滚动、次要表格列可折叠；
- 小于 768 px：筛选进入抽屉、指标单/双列、表格转关键字段卡片、详情抽屉宽 94vw；
- 所有状态不只依赖颜色，配套文字和图标；
- 键盘可完成标签切换、筛选、打开详情和确认操作；
- 动态进度使用 `aria-live="polite"`，高风险错误使用 `assertive`；
- 大型 JSON 虚拟滚动并设置显示上限，避免浏览器因上游超大结果失去响应。

## 15. 测试设计

### 15.1 单元和组件测试

1. 侧边栏只新增一个“`MCP 聚合管控`”入口，路由和中英文标题正确；
2. MCP 功能默认可用；无 `mcp:server:read` 时仅由路由守卫阻止进入，后端继续执行最终授权；
3. 接入表单只包含远程 MCP 字段，不出现 stdio/command/Runner；
4. 生产环境拒绝 HTTP、URL userinfo、fragment 和非法 endpoint；
5. 新 secret 提交后立即从组件状态清除，编辑时不回显；
6. 双击、超时重试和页面恢复复用 Idempotency-Key，不创建重复任务；
7. 每个 onboarding 状态映射到正确步骤、标签和按钮；
8. `approved`、`published`、`dispatching`、`succeeded` 不发生错误映射；
9. 无内部标签权限时不渲染标签且不发送对应请求；
10. 规则和 AI 独立展示，pending/degraded/unknown 不使用绿色；
11. 旧请求晚返回时不会覆盖新选择对象；
12. 安全 JSON 查看器不执行 HTML、Markdown、链接或外部资源；
13. reveal 关闭/过期/退出登录后 payload 从内存清理；
14. endpoint、secret、payload、审批理由不会进入 URL、console 或埋点；
15. 403、409、429、5xx、断网和 stale cache 都有稳定页面状态。

### 15.2 E2E 场景

1. 管理员输入远程 HTTPS endpoint 和 Credential Ref，一次点击创建任务；
2. OAuth Server 在 `awaiting_auth` 完成授权后继续原任务；
3. L1 内部只读 Server 自动完成接入并进入 active；
4. L2-L4 Server 停在审批节点，通过审批后才发布；
5. Schema 漂移触发隔离，列表、详情和发布操作同时反映状态；
6. 创建 Catalog Release、查看 Diff、审批、发布、回滚均显示正确不可变版本；
7. Client Grant 撤销后网关拒绝调用，前端展示生效证据；
8. L3/L4 运行时审批绑定参数和 digest，修改参数后原审批失效；
9. 一次调用能从列表进入四阶段证据，并分别查看规则和 AI 结果；
10. MinIO、AI、Gateway、控制面分别降级时，UI 符合失败矩阵且不伪报安全/成功；
11. 普通只读用户无法看到写按钮、payload 或其他 tenant 数据；
12. 中英文、窄屏、键盘导航、超大 JSON 响应通过可用性测试。

## 16. 验收标准

1. “系统配置”下恰好新增一个名为“`MCP 聚合管控`”的菜单，其他 MCP 工作区不扩散到侧边栏；
2. 页面默认进入“远程服务”，主操作可一键创建远程 MCP 接入任务；
3. 接入 UI 和接口不接受 stdio、本地 command 或 Runner；
4. 用户能在同一页面完成 Server 接入、工具发布、Client 授权、审批、审计和安全分析；
5. 所有重要状态区分准入、审批、发布、调用、规则和 AI，不把中间态显示为成功；
6. 每次调用可以在前端追溯四阶段证据，默认只展示脱敏摘要；
7. secret、上游 credential、原始 payload 不进入 URL、浏览器持久化、console、埋点和通知；
8. 页面、标签、按钮和 API 均受 capability 控制，后端继续执行最终权限判定；
9. loading、empty、error、403、degraded 和 stale 状态均可验证；
10. MCP 不提供全局关闭开关；停用通过 Server、Tool、Catalog、Grant、Policy 和审批状态完成，现有 MCP 资产页、Assistant 和其他设置页无回归。

## 17. 实施顺序与回滚

建议按以下顺序实现：

1. capability 会话、统一 `can()` 和路由守卫；
2. 菜单、路由、页面骨架、指标和远程服务列表；
3. 一键接入及任务进度；
4. 工具/Catalog Release 和审批中心；
5. Client/Grant；
6. 调用审计、四阶段查看器和安全分析；
7. 响应式、可访问性、E2E 和按 Server/Tool/Grant 的安全策略验证。

回滚时暂停受影响 Server/Tool/Catalog/Grant 并切回上一签名 Release，历史审计应保留只读查询能力，
不得删除接入任务、Release、审批或调用证据。旧 `/assistant/mcp-sources` 只作为迁移期兼容路径，
不能被前端静默重新当成 V6.3 平台后端。
