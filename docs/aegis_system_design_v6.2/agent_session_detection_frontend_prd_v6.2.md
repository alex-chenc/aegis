# Aegis V6.2 智能体会话检测前端 PRD

**版本**：6.2-P5
**日期**：2026-08-06
**状态**：完整会话正文/AI 语义页面待实施；当前事件页已实现 Native Hook 真实 session 与工具事件安全分析
**父导航**：智能体防护
**子标签**：智能体会话检测
**完整 P5 首批 Agent**：Codex、Claude Code、OpenCode

> 当前事件页的 Native Hook 运行时支持 Codex、Claude Code、OpenClaw、Hermes、Zcode，
> 可展示真实 session ID 和工具命中；本 PRD 的正文时序、AI 语义和风险标记属于完整 P5，
> 不应覆盖已有的会话范围工具安全分析。

## 1. 页面目标

安全运营人员在该页面完成四件事：

1. 盘点哪些主机、用户和项目正在使用 Codex、Claude Code、OpenCode，会话是否
   完整采集、是否仍在运行。
2. 按真实时间顺序查看用户消息、助手可见回复、工具调用、权限决策、compact
   和子智能体分支。
3. 查看 AI 对会话恶意意图的判断、风险类别、证据消息、反证和不确定性。
4. 把语义风险与 Aegis 已采集的 PID、命令、文件、外链、提权和逃逸行为关联，
   区分“说了什么、计划做什么、尝试了什么、实际做成了什么”。

页面不提供 prompt 实时阻断、会话修改、工具审批或 freeze/kill。会话 AI 的
结论只用于标记、告警和人工复核；主机动作仍由原“事件感知与防护/逃逸防护”
页面基于确定性 OS 证据处理。

## 2. 继承当前 Aegis 前端

必须复用当前项目：

- `frontend/src/App.vue` 的深色左侧导航和 64px 顶栏。
- `frontend/src/styles/aegis-theme.css` 的浅蓝渐变背景、白卡片、圆角和风险色。
- `AgentGuardLayout.vue` 的 KPI、筛选卡、服务端分页、错误保留和 WebSocket
  更新方式。
- `AgentSummaryTable.vue`、`AgentDetailDrawer.vue` 的表格密度、抽屉标题、
  loading/empty/error 和权限表达。
- Element Plus、Pinia、Axios、Vue Router、现有 i18n 和 1280px 最低桌面宽度。

不得引入第二套顶部导航、深色大屏、聊天产品式全屏首页或与 Aegis 不一致的
霓虹风。会话正文可以采用聊天气泡/时序卡片，但必须放在现有白色证据卡片中，
不伪装成可继续对话的输入界面。

## 3. 信息架构

### 3.1 侧边栏

```text
智能体防护
├── 智能体事件感知与防护
├── 智能体逃逸防护
└── 智能体会话检测
```

“智能体会话检测”是第三个可见子菜单，不把“会话列表、AI 分析、风险标记、
采集策略”继续展开成更多侧边栏菜单。

### 3.2 路由

```text
/detection/agent-guard/sessions
```

详情仍使用当前页面右侧大抽屉，query 可恢复：

```text
/detection/agent-guard/sessions
  ?session_audit_id=<uuid>
  &detail_tab=conversation|analysis|behavior
  &item_id=<uuid>
  &marking_id=<uuid>
```

来自告警中心：

```text
/detection/agent-guard/sessions
  ?session_audit_id=<uuid>
  &detail_tab=analysis
  &marking_id=<uuid>
```

来自事件全景：

```text
/detection/agent-guard/sessions
  ?session_audit_id=<uuid>
  &detail_tab=behavior
  &event_id=<uuid>
```

关闭抽屉后保留筛选、分页、排序和列表滚动位置。

## 4. 页面结构

```text
SECURITY OPERATIONS / 智能体防护 / 智能体会话检测

[采集会话] [活动会话] [高危/恶意] [待分析/不完整]

[Agent] [主机] [用户] [项目] [风险] [采集完整性] [时间] [查询][重置]

┌──────────────────────────────────────────────────────────────┐
│ 会话/风险 │ Agent │ 主机/用户 │ 项目 │ 消息/工具 │ 完整性 │ 时间 │
├──────────────────────────────────────────────────────────────┤
│ ● Critical  opencode ...   恶意载荷/C2、数据外传  [查看会话] │
│ ● High      claude-code... 凭据访问（仅尝试）      [查看会话] │
│ ● Normal    codex ...      未发现明显风险          [查看会话] │
└──────────────────────────────────────────────────────────────┘

点击行 -> 右侧 80% 抽屉
┌──────────────────────────────────────────────────────────────┐
│ 会话审计 · OpenCode | host/user/project | complete | malicious │
│ [完整会话] [AI 语义分析] [关联行为]                            │
│                                                              │
│ 左：消息/工具时序（虚拟列表）   右：当前 item 证据和风险标记   │
└──────────────────────────────────────────────────────────────┘
```

## 5. 顶部 KPI

| KPI | 定义 | 点击行为 |
| --- | --- | --- |
| 采集会话 | 当前筛选时间内发现的 distinct audit session | 清空 session status |
| 活动会话 | `status=active|idle` 且采集心跳未 stale | 筛选活动 |
| 高危/恶意 | open marking severity=high/critical 或 verdict=malicious | 风险优先排序 |
| 待分析/不完整 | analysis pending/failed 或 coverage partial/metadata_only | 筛选需处理 |

KPI 必须区分：

- 语义 `malicious` 不等于已执行恶意行为。
- `partial` 不等于无风险。
- analysis `pending/running` 不等于已完成。
- dismissed/false-positive 默认不计入 open 高危数。

## 6. 筛选卡

第一行：

- Agent：Codex、Claude Code、OpenCode，多选。
- 主机/主机组，多选。
- OS 用户：展示 username + UID；只有授权范围可见。
- 项目：按脱敏项目名或 project hash 搜索。
- 会话状态：active、idle、ended、deleted_at_source、unknown。
- 时间范围，默认最近 24 小时。

第二行：

- 风险结论：benign、suspicious、malicious、inconclusive、未分析。
- Severity：critical/high/medium/low/info。
- 风险类别：凭据、外传、持久化、提权、逃逸、破坏、防御规避、C2、横向、
  策略绕过。
- 采集完整性：complete、partial、metadata_only、disabled、unsupported。
- 标记状态：open、confirmed、dismissed、false_positive。
- 关键字：只检索授权的脱敏正文索引、会话 ID、项目或工具名。

交互：

- 筛选改变后不立即请求，点击“查询”或 Enter 执行。
- 已应用筛选同步 URL query；正文关键字不保存在浏览器 storage。
- 无 `agent_session:content:read` 时隐藏正文关键字搜索，并显示权限说明。
- Query 不允许搜索 token/private key 等被 redaction 的原始值。

## 7. 会话列表

一行表示一个产品 `source_session_id` 在一台主机、一个 UID 下的审计会话。

### 7.1 字段

| 列 | 内容 |
| --- | --- |
| 风险 | 最高 open marking severity、verdict、是否 behavior-confirmed |
| 会话 | 脱敏 title、短 session ID、active/ended、父/子会话提示 |
| Agent | Codex/Claude Code/OpenCode、版本、采集 source mode |
| 主机/用户 | hostname/IP、username/UID；按权限脱敏 |
| 项目 | 脱敏 project、cwd 摘要，不展示完整 home path |
| 活动 | turn/message/tool/subagent 数量 |
| 采集 | complete/partial/metadata_only/unsupported + 原因 tooltip |
| 分析 | 最新 analysis 状态、模型/Prompt version 摘要、完成时间 |
| 时间 | start、last activity、duration |
| 操作 | 查看会话 |

### 7.2 风险表达

每行同时显示两个维度，禁止合并成模糊“危险”标签：

```text
[恶意语义 · High] [行为已证实]
[可疑语义 · Medium] [暂无行为证据]
[分析不确定] [采集不完整]
[行为 Critical] [语义存在反证]
```

- critical 红、high 橙、medium 黄、normal 绿，且带文字/图标。
- `behavior-confirmed` 使用实心盾牌；`semantic-only` 使用对话气泡图标。
- `partial/metadata_only` 使用灰/蓝信息标签，不得使用绿色安全标签。

### 7.3 外层禁止内容

列表不展示：

- 用户 prompt 或助手回复正文。
- tool args/result、文件路径、外链地址、secret redaction 片段。
- AI evidence excerpt、完整分析 summary、OS 行为正文。
- reveal/export 按钮。

这些信息只在详情抽屉且通过后端权限校验后加载。

## 8. 会话详情抽屉

- 使用 `el-drawer`，桌面宽度 80%，最小 960px；1280px 下仍保留可读内容区。
- 标题：`会话审计 · <Agent> · <短 Session ID>`。
- 标题下：Agent/version、主机、用户、项目、开始/结束、source mode、coverage、
  content mode、最新 verdict。
- active 会话展示“实时采集中”；WebSocket 只触发增量拉取，不携带正文。
- 右上角操作：重新分析、处理标记、导出；按权限显示。
- reveal 原文不作为常驻开关，必须在具体 item 上发起审批并有超时。

抽屉只有三个 Tab：

1. 完整会话。
2. AI 语义分析。
3. 关联行为。

## 9. Tab 一：完整会话

### 9.1 布局

```text
┌──────────────────────────────┬──────────────────────┐
│ 会话时序                     │ 当前 Item 详情       │
│                              │                      │
│ 10:01 用户                   │ source/message ID    │
│   帮我检查服务器配置...      │ role/type/time       │
│                              │ redaction/completeness│
│ 10:02 助手                   │ risk markings        │
│   我会先读取...              │ related tool/behavior│
│   [风险: credential_access]  │ copy/reveal audit    │
│                              │                      │
│ 10:02 工具 Bash [denied]     │                      │
│   command: ******            │                      │
│  └─ permission denied        │                      │
└──────────────────────────────┴──────────────────────┘
```

- 左侧 65%，按 item sequence 虚拟滚动。
- 右侧 35%，选中 item 的结构化详情；sticky，不跟随长内容滚走。
- item cursor 50/页，上下滚动接近边界时预取；不能一次加载整段长会话。
- 顶部提供 item 类型、risk category、tool name 搜索和“仅看风险”。
- 点击 AI evidence ID 自动切换本 Tab 并定位/高亮对应 item。

### 9.2 Item 显示

#### 用户消息

- 左边蓝灰色头像/标签“用户”，显示脱敏文本。
- 标记 `redacted N`、`truncated`、`content_disabled`。
- 不显示 source home 绝对路径，项目路径使用脱敏值。

#### 助手消息

- 白色/浅蓝卡片，“助手”标签和 model 摘要。
- 只显示用户可见回答，不显示隐藏 reasoning。
- 风险 span 不使用前端自行 NLP，全部来自后端 marking/item refs。

#### 工具调用

- 可折叠证据卡，标题显示 tool、status、duration、permission。
- Bash/shell：脱敏 command preview、exit code；默认不显示 stdout/stderr。
- File：operation、脱敏 path、成功/失败；正文不展示。
- Network/MCP：server/tool、目标分类和状态；secret args 被替换。
- 关联到 OS event 时显示“行为已证实”，点击跳转关联行为 Tab。

#### Permission

必须把 request 与 decision 分开：

```text
权限请求：Bash 跨越 workspace 边界
结果：denied by user
```

permission requested/allowed 不代表工具成功；只有 PostToolUse/OS outcome 才能
显示 executed/succeeded。

#### Compact

以横向分隔线显示：

```text
──── 10:30 自动压缩上下文 · 原始 item 仍保留 · summary 已脱敏 ────
```

compact 后不丢弃 UI 历史；缺失历史时明确显示 missing range。

#### 子智能体

- 父时序中显示可展开“子智能体 Explore / security-reviewer”。
- 展开后加载独立分支 item，显示 agent ID/type/status。
- 子智能体风险归属当前分支，同时汇总到父 session；不能重复计数为两个独立
  high marking，除非产品本身有独立 source session。

#### Lifecycle/Error

startup/resume/idle/end/delete/collector gap/unsupported parser 使用信息卡，不与
用户/助手气泡混淆。

### 9.3 内容操作

| 操作 | 约束 |
| --- | --- |
| 复制脱敏内容 | `content:read`，记录 item ID 和操作人，不记录正文 |
| 查看授权原文 | `content:reveal`，填写 purpose/ticket，审批后短时显示 |
| 导出 | `export`，选择 metadata/redacted；raw 需更高审批 |
| 定位风险 | 点击 marking，滚动到 evidence item |

原文 reveal：

- 明显红色边界和剩余有效时间。
- 默认 60 秒后自动遮盖；切换 session/关闭抽屉立即清除内存。
- 禁止 browser cache、localStorage、打印到 console。
- 前端不能批量 reveal 整个会话，除非单独合规导出审批。

## 10. Tab 二：AI 语义分析

### 10.1 顶部结论卡

展示：

- verdict：benign/suspicious/malicious/inconclusive。
- severity、confidence。
- judgment source：session_ai/session_rule/session_behavior_combined/human。
- analysis status、model/provider、Prompt/Schema version、input sequence range。
- coverage/evidence completeness。
- 明确警示：`AI 语义结论不会直接冻结或终止 Agent`。

### 10.2 风险类别卡

每个 category 卡包含：

```text
凭据/密钥获取 · High
阶段：planned / attempted / executed / unknown
语义证据：消息 #12、工具 #14
行为证据：AGB-BUILTIN-001 event #...
结论：尝试读取测试凭据文件，权限被拒绝，未观察到成功读取
[查看消息] [查看关联行为]
```

category：凭据、外传、持久化、提权、逃逸、破坏、防御规避、恶意载荷/C2、
横向移动、策略绕过。

### 10.3 证据和反证

左右双列：

- 支持证据：item/tool/event/finding 引用，按时间。
- 反证/不确定性：合法运维上下文、permission denied、命令失败、未观察到
  resource change、partial coverage、remote_unobservable。

前端不允许只显示模型 summary 而隐藏反证。每个引用都能定位到完整会话或
关联行为；不存在/无权限的证据显示稳定状态，不删除引用。

### 10.4 攻击链

当有联合证据时显示小型有向时间链，不使用自由拖拽图：

```text
[用户请求获取密钥]
  -> [助手计划读取 ~/.ssh]
  -> [Bash tool requested]
  -> [file open /home/**/.ssh/id_* success]
  -> [external connect success]
```

每节点标注来源：Conversation / Tool / OS Behavior / Finding；语义节点使用虚线，
已证实 OS 节点使用实线。

### 10.5 分析历史和人工处置

- 分析历史按 attempt 显示，不覆盖旧结论。
- “重新分析”要求原因，显示将使用当前策略和最新已采集 sequence。
- marking 操作：确认风险、驳回、标记误报；必须填写处置理由。
- 人工结论不修改 AI run，只新建 `judgment_source=human` disposition。
- 无 freeze/kill 按钮；需要主机处置时跳转原 Agent Guard Finding/执行单元。

## 11. Tab 三：关联行为

### 11.1 汇总

- 关联 RuntimeInstance/BehaviorSession/ExecutionUnit。
- confirmed/probable/ambiguous/unattributed 数量。
- 五个内置规则命中和逃逸 Finding。
- 对应动作状态只读展示。

### 11.2 时间轴

```text
10:02:01 Conversation 用户请求
10:02:04 Conversation 助手计划
10:02:05 Tool Bash requested
10:02:05 Process PID 4210 exec /usr/bin/bash
10:02:06 File PID 4210 read /test/credential [denied]
10:02:07 Tool Bash failed
```

每条 OS 行为显示 PID/PPID、脱敏 cmdline、文件/网络/身份/隔离资源和 outcome；
点击可跳到原“智能体事件感知与防护”详情抽屉。

### 11.3 关联可信度

| 状态 | UI |
| --- | --- |
| confirmed | 绿色/蓝色“确定关联”，显示 correlation/tool/process evidence |
| probable | 黄色“推定关联”，显示依据和时间窗口 |
| ambiguous | 灰色“可能关联”，不参与 combined 自动结论 |
| unattributed | “未归属”，保留在会话但不伪造 PID |

一个 OS event 只有一个 primary conversation session。跨会话相似命令不能仅按
时间接近重复挂载。

## 12. 采集覆盖抽屉

页面右上角提供“采集配置与覆盖”，不是侧边栏菜单。

### 12.1 覆盖矩阵

| 主机/用户 | Agent/version | Hook/Plugin | Transcript/API | 内容模式 | 完整性 | 原因 |
| --- | --- | --- | --- | --- | --- | --- |
| host-a/1000 | Codex 0.x | managed | parser supported | redacted | complete | - |
| host-a/1000 | Claude Code 2.x | user hook | missing range | redacted | partial | compact gap |
| host-b/1001 | OpenCode 1.x | plugin | API disabled | metadata | metadata_only | policy |

### 12.2 策略编辑

管理员可设置：

- 主机/主机组、Agent 类型、UID/用户、project hash 范围。
- metadata_only/redacted_text/authorized_full_text。
- 历史回补天数、子智能体、tool result preview。
- AI 启用、idle delay、chunk turns、成本预算。
- 脱敏/原文/metadata 保留期。

警示：

- full text 需要合规确认和明确 retention。
- 启用 Hook/Plugin 不得覆盖用户已有配置；发布预览显示合并 diff。
- unsupported version 不能强制标 complete。
- 禁止选择“所有主机所有用户所有历史”作为默认范围。

## 13. 页面状态

### 13.1 未启用

显示模块关闭、所需权限和“配置采集”入口；不显示假数据。

### 13.2 无会话

区分：

- 筛选无结果。
- 尚未发现受支持 Agent。
- Agent 存在但 session collection 未下发。
- 只有 metadata，无正文权限。

### 13.3 Partial/Unsupported

显示原因：

```text
source_version_unsupported
transcript_unavailable
hook_not_installed
plugin_not_loaded
history_persistence_disabled
sequence_gap
redaction_failed_content_suppressed
source_deleted
```

提供“查看覆盖详情”，不能用 `-` 隐藏原因。

### 13.4 实时断开

保留当前列表/抽屉数据，顶部显示“实时更新已断开，正在重连”；重连后按
`updated_at + session_id` 增量刷新，不清空用户选择。

### 13.5 分析失败

会话仍可查看；AI 卡显示 timeout/invalid_output/evidence_mismatch 等错误，
规则/OS 风险不被降级或删除。

## 14. 权限

| 权限 | 页面能力 |
| --- | --- |
| `agent_session:read` | 列表、metadata、风险统计 |
| `agent_session:content:read` | 脱敏会话正文 |
| `agent_session:content:reveal` | 审批后单 item 原文 |
| `agent_session:analyze` | 重新分析 |
| `agent_session:marking:handle` | 确认/驳回/误报 |
| `agent_session:export` | 导出 |
| `agent_session:policy:write` | 采集策略和覆盖配置 |

后端 403 是最终边界。前端不得先请求无权限正文再隐藏；应按 capability response
决定是否请求 content endpoint。

## 15. API 依赖

```text
GET  /api/v1/agent-guard/session-detection/overview
GET  /api/v1/agent-guard/session-detection/coverage
GET  /api/v1/agent-guard/session-detection/sessions
GET  /api/v1/agent-guard/session-detection/sessions/:id
GET  /api/v1/agent-guard/session-detection/sessions/:id/items
GET  /api/v1/agent-guard/session-detection/sessions/:id/tool-calls
GET  /api/v1/agent-guard/session-detection/sessions/:id/analysis-runs
GET  /api/v1/agent-guard/session-detection/sessions/:id/markings
GET  /api/v1/agent-guard/session-detection/sessions/:id/related-behaviors
GET  /api/v1/agent-guard/session-detection/sessions/:id/collection-status
POST /api/v1/agent-guard/session-detection/sessions/:id/analyze
POST /api/v1/agent-guard/session-detection/markings/:id/confirm
POST /api/v1/agent-guard/session-detection/markings/:id/dismiss
POST /api/v1/agent-guard/session-detection/markings/:id/false-positive
POST /api/v1/agent-guard/session-detection/sessions/:id/content/reveal
POST /api/v1/agent-guard/session-detection/sessions/:id/export
```

列表响应不返回正文；抽屉按 Tab 懒加载。items 使用 cursor：

```json
{
  "items": [],
  "next_cursor": "opaque",
  "previous_cursor": "opaque",
  "has_more": true,
  "missing_ranges": [],
  "content_authorized": true
}
```

## 16. 前端目录

```text
frontend/src/
├── api/agentSessionDetection.ts
├── types/agentSessionDetection.ts
├── store/agentSessionDetection.ts
└── views/detection/AgentSessionDetection/
    ├── SessionDetection.vue
    ├── components/
    │   ├── SessionMetricCards.vue
    │   ├── SessionFilters.vue
    │   ├── SessionSummaryTable.vue
    │   ├── SessionDetailDrawer.vue
    │   ├── ConversationTimeline.vue
    │   ├── ConversationItemCard.vue
    │   ├── ConversationItemDetail.vue
    │   ├── ToolCallCard.vue
    │   ├── SubagentBranch.vue
    │   ├── SessionVerdictPanel.vue
    │   ├── SessionRiskCategoryCard.vue
    │   ├── SessionEvidenceColumns.vue
    │   ├── SessionAttackChain.vue
    │   ├── SessionBehaviorTimeline.vue
    │   ├── CollectionCoverageDrawer.vue
    │   ├── SessionPolicyEditor.vue
    │   ├── RiskDispositionDialog.vue
    │   └── ContentRevealDialog.vue
    └── sessionDetectionQuery.ts
```

稳定文案新增 `zh-CN/agentSessionDetection.ts` 和
`en-US/agentSessionDetection.ts`，不在组件中写裸中文/英文。

## 17. Store 和实时状态

Store 分开保存：

```text
overview
sessions / total / filters / sort
selectedSession
itemsBySession / cursors / itemIndex
toolCallsBySession
analysisRunsBySession
markingsBySession
relatedBehaviorsBySession
collectionStatusBySession
revealState（只在内存，超时清除）
loading/errors 按资源分离
```

要求：

- 列表失败但已有数据时保留旧数据并显示 warning。
- 一个 Tab 请求失败不清空其他 Tab。
- WebSocket 只更新 metadata；正文通过带权限的 HTTP 增量请求。
- item 使用 ID 幂等 upsert，revision 更新不重复插入。
- 切换 session 取消旧请求，防止响应串到新抽屉。
- reveal 内容不进入 Pinia 持久化、Vue devtools 可序列化状态或错误对象。

## 18. 性能、响应式和可访问性

- 会话列表服务端分页，默认 20，最大 100。
- items 默认 50/cursor，工具详情和子智能体懒加载。
- 会话时序使用虚拟列表；动态卡片高度需要缓存并在展开后重新测量。
- 搜索定位缺页 item 时，先请求包含 item 的 anchor page，再滚动。
- 1280px 抽屉左右列最小 600/300px；更窄时右详情改为抽屉内下层面板。
- 键盘可聚焦会话行、Tab、item、风险证据和处置按钮。
- 风险、角色、状态不能只靠颜色；使用文本、icon 和 `aria-label`。
- 长 prompt、command、path 使用换行/折叠，不造成水平页面溢出。

## 19. 测试用例

### 19.1 导航

1. “智能体防护”下准确显示三个子菜单。
2. sessions 路由、面包屑、标题和权限正确。
3. query 刷新恢复 session、Tab、item/marking 定位。

### 19.2 列表

1. Codex、Claude Code、OpenCode 可以分别和组合筛选。
2. 同机同类型多个 session 分行，不按 Agent asset 聚合。
3. 列表 DOM 不包含 prompt、assistant、tool args/result。
4. semantic-only 和 behavior-confirmed 标签不同。
5. partial/metadata_only/unsupported 不显示成绿色已完整。
6. 权限不足不请求正文 API。

### 19.3 完整会话

1. user/assistant/tool/permission/compact/subagent/lifecycle 顺序准确。
2. redacted/truncated/missing/unobservable 清楚可见。
3. tool requested、denied、failed、succeeded 不混淆。
4. AI evidence 点击定位到真实 item。
5. cursor、虚拟列表、历史预取和 active 增量不重复。
6. reveal 超时、切换 session 和关闭抽屉后清除原文。
7. DOM/console/storage/URL 中无未授权 secret fixture。

### 19.4 AI 分析

1. verdict/category/severity/confidence 和来源显示准确。
2. supporting evidence 与 counter evidence 同时展示。
3. intent/planned/attempted/executed 不混淆。
4. AI-only malicious 明确提示不会自动 freeze。
5. invalid output/partial coverage 仍保留会话和 OS Finding。
6. 人工 disposition 不覆盖历史 analysis run。

### 19.5 关联行为

1. 会话 tool call 可定位 PID/文件/网络/提权/逃逸事件。
2. confirmed/probable/ambiguous/unattributed 标签和证据准确。
3. 同一 OS event 不被重复关联到多个 primary session。
4. 跳转原 Agent Guard 页面时保留 event/finding/execution unit。

### 19.6 页面状态

1. feature off、无 Agent、未纳管 UID、无 session、无筛选结果分别表达。
2. WebSocket 断线保留页面，重连不重置用户状态。
3. API 403/404/409/429/500 使用稳定错误文案。
4. analysis/reveal/export/marking 各自 loading，防止重复提交。

## 20. 前端验收标准

1. 新增“智能体会话检测”第三个子标签，样式与现有 Aegis 完全一致。
2. 外层只展示会话元数据、完整性和风险摘要，不泄露会话正文。
3. 点击会话打开 80% 详情抽屉，只有完整会话、AI 语义分析、关联行为三个 Tab。
4. Codex、Claude Code、OpenCode 的不同 source item 被归一为一致 UI，又能看到
   产品来源和 coverage 差异。
5. 用户能定位每一个风险标记对应的真实消息/tool/OS event，并看到反证。
6. 页面明确区分 requested/planned/attempted/executed 和 semantic-only/
   behavior-confirmed。
7. partial、metadata_only、unsupported、redacted、truncated 都不能被包装成完整。
8. AI-only 结论不提供 freeze/kill；处置动作只处理 marking。
9. reveal/copy/export 权限、审批、超时和审计完整，浏览器不持久化原文。
10. 万级 item 长会话通过 cursor + 虚拟列表保持可用，实时增量不打断当前阅读。
