# Aegis V6.3 智能体会话感知前端设计

## 1. 页面目标

新增独立页面“智能体会话感知”，视觉和交互结构参考当前
`AgentGuard/AgentGuardLayout.vue`：Hero、KPI、状态告警、筛选卡、主列表和详情
抽屉。页面的第一层主对象是资产中心的 Agent，直接复用
`AgentGuardQueryRepository.ListAgents` 返回的 Agent 资产数据；会话只在选中
Agent 的详情抽屉中展示。

## 2. 路由与导航

```text
route: /detection/agent-guard/session-awareness
name: AgentSessionAwareness
titleKey: routes.agentSessionAwareness
permission: agent_session_awareness:read
```

`App.vue` 在“智能体防护”下新增第四项：

```text
智能体事件感知与防护
智能体逃逸防护
智能体配置检测
智能体会话感知
```

图标建议 `ChatLineRound` 或现有 Element Plus 会话类图标，不引入新的图标包。

## 3. 推荐目录

```text
frontend/src/views/detection/AgentGuard/
  SessionAwareness.vue
  components/
    SessionAwarenessMetrics.vue
    SessionAwarenessFilters.vue
    SessionAwarenessTable.vue
    SessionAwarenessDetailDrawer.vue
    SessionConversationTimeline.vue
    SessionRuleAnalysis.vue
    SessionAIAnalysis.vue
    SessionRelatedBehaviors.vue
    SessionCollectionStatus.vue
    SessionTokenUsage.vue

frontend/src/api/agentSessionAwareness.ts
frontend/src/store/agentSessionAwareness.ts
frontend/src/types/agentSessionAwareness.ts
frontend/src/i18n/locales/{zh-CN,en-US}/agentSessionAwareness.ts
```

## 4. 页面结构

### 4.1 Hero

标题：智能体会话感知

说明：集中查看 Claude Code 与 Codex CLI 的脱敏会话、提示词安全规则、AI 研判、
Token 用量和关联行为。

右侧按钮：

- `会话规则`：只读内置规则目录；
- `采集与分析设置`：有 settings write 权限时显示。

### 4.2 KPI

五张指标卡：

| 指标 | 说明 |
| --- | --- |
| 总会话 | 当前筛选/时间范围内会话数 |
| 活跃会话 | status active_inferred/idle_inferred，并显示“状态推断”提示 |
| 风险会话 | overall risk high/critical 或 AI malicious |
| 采集不完整 | partial/metadata_only/unsupported/source_not_found |
| 可见内容 Token | 当前时间范围估算总量，必须带 `~` |

Token 卡 tooltip：这是脱敏可见内容估算，不代表来源计费或 Aegis AI 调用量。

### 4.3 状态告警

会话感知页面的 Agent 状态只使用两个值：`运行中` 和 `已停止`。这里的 Agent
指资产中心中的 Claude Code 或 OpenAI Codex CLI，不是 Aegis Agent 的连接/心跳状态，
因此不显示 `陈旧`。静态采集也不依赖目标 Agent 是否正在运行；只要资产配置仍存在，
“采集会话”按钮即可下发扫描请求。Aegis Agent 暂时未连接时请求进入待重连状态，
不能把它改写成目标 Agent 的“陈旧”。

### 4.4 筛选卡

第一行：主机、Agent 类型、会话状态、coverage、时间范围。

第二行：规则状态、AI verdict、风险级别、Token 范围、查询/重置。

V6.3 不提供正文关键词输入框，避免前端误导为全文索引或把敏感关键字写入 URL。

筛选写入 route query 的只允许枚举、UUID、数值和时间；不放会话正文、excerpt
或用户 prompt。

## 5. Agent 列表

主列表沿用 Agent Guard 的 Agent 资产维度，数据源为
`host_application_assets.category = 'ai_agent'`（兼容已有 Agent Guard 的主机、运行
状态和覆盖信息）。每行提供“查看会话”和“采集会话”两个操作，不能把全局会话列表
作为页面首屏。

## 6. Agent 详情与会话列表

选中 Agent 后打开详情抽屉，头部展示 Agent 名称、主机、类型、运行状态和资产更新时间；
详情主体展示仅属于该 `host_id + agent_type` 的会话分页列表。会话列表列出会话 ID、
状态、消息数、估算 Token、规则风险和最后发现时间。点击会话再打开会话内容和分析
对话框/子抽屉。

采集操作只放在 Agent 主列表，详情头部仅提供“刷新会话”，避免把采集控制与会话查看
混在同一个详情操作区。只要该资产的配置文件仍存在，主列表的“采集会话”操作在目标
Agent 已停止时也保持可用。按钮请求
`POST /agent-guard/session-awareness/agents/:host_id/collect?agent_type=...`，服务端
通过现有 Server→Agent 配置同步通道下发 `agent_session_collect`，Agent 执行一次有界
静态 JSONL 扫描；这条路径不安装 Hook、不监听文件系统，也不把原始路径或未脱敏内容
放入控制消息。

### 6.1 会话列表

列：

| 列 | 内容 |
| --- | --- |
| 会话 | 脱敏标题/短 session audit ID |
| 主机 | hostname/IP（按现有权限） |
| Agent | Claude Code/Codex CLI badge + source version |
| 项目 | 脱敏 project name，不展示完整 home path |
| 消息/工具 | item/turn/tool count |
| Token | `~12.3k` 可见内容估算；tooltip 显示 method/version |
| 规则分析 | clean/matched/failed、hit 数和最高级别 |
| AI 分析 | not run/queued/running/verdict/confidence |
| 覆盖 | complete/partial/metadata/unsupported/source not found |
| 最后活动 | 相对时间 + tooltip 绝对时间 |
| 操作 | 查看详情；有权限时手工 AI 分析 |

coverage tooltip 必须说明：`complete` 是“截至最近扫描已读取当时所有完整记录”，
不代表静态来源会话已经结束。

### 6.2 内置规则目录

页面顶部提供“内置安全规则”入口，弹出只读规则目录。目录展示规则键/版本、名称、
说明、分类、执行引擎、默认严重级别、默认/推荐动作、默认状态和摘要校验值；规则
目录由 API 返回，页面不提供编辑、删除或运行时覆盖入口。配置检测页面使用同一目录
组件展示 `AGC-*` 配置规则，会话感知页面展示 `ASR-*` 会话规则，视觉和交互与事件
感知的内置规则卡片保持一致。

规则目录接口：

```text
GET /api/v1/agent-guard/configuration-rules
GET /api/v1/agent-guard/session-awareness/rules
```

### 5.1 风险表达

- 规则 matched 使用“规则命中”，不使用“攻击成功”；
- AI suspicious/malicious 带“AI 判断”前缀；
- behavior confirmed 才显示“执行已证实”；
- partial/inconclusive 不能用绿色“安全”；
- 颜色不是唯一信号，文本和 icon 同时表达。

### 5.2 分页与加载

- 服务端分页默认 20，可选 20/50/100；
- 排序由 API 执行；
- filter/page/sort 更新 route query；
- 并发请求使用 request sequence 或 AbortController，旧响应不能覆盖新筛选；
- refresh 失败保留已加载列表并显示 warning。

## 7. 会话详情抽屉

宽度建议 `min(1180px, 94vw)`，移动端全屏。头部显示：

- Agent、host、project、session status；
- coverage/content mode；
- rule/AI/overall risk；
- Token summary；
- first/last source time。

Tabs：

```text
会话内容 | 规则分析 | AI 分析 | 关联行为 | 采集信息
```

会话内容时间线的每条 item 右侧保留证据栏：规则命中显示规则键、名称、严重级别、
脱敏证据和消息序号；AI 分析显示分析分段、风险结论、摘要和覆盖的消息序号。AI
分析完成后不得只在抽屉底部显示一份 JSON，总结必须能够回指到具体会话位置。

深链 query 只保存 `session_id` 和 tab，不保存正文 item 或 excerpt。若需要 item
定位，使用短期 UI state 或 `item_id` UUID；API 仍重新鉴权。

### 7.1 会话内容 Tab

### 7.1 Timeline

item 视觉：

- user：左侧用户气泡；
- assistant：右侧/中性回复块；
- tool call/result：可折叠结构卡；
- permission：请求/允许/拒绝 badge；
- compaction：时间轴分隔线；
- subagent：父会话内嵌可展开分支；
- lifecycle/error：只有静态记录明确存在时才显示；推断状态使用独立标记。

每个 item 显示：时间、类型、source sequence、`~Token`、redaction/truncation/
suppressed badge。tool result 默认折叠，只展示安全摘要。

### 7.2 虚拟化和分页

- cursor 50 items；
- 向前/向后加载；
- 超过 300 个已加载 item 使用 virtual list；
- 风险定位时请求包含目标 item 的窗口，而不是加载整个会话；
- code block 使用 text rendering/highlight，不执行 HTML；
- 所有内容按纯文本或安全 markdown renderer，禁用 raw HTML、外链自动请求和
  图片加载。

### 7.3 操作

- 默认不提供整会话复制/导出；
- 分析员可复制单个脱敏 item，操作写审计；
- suppressed 内容不可展开；
- 无 `content:read` 权限时 Tab 显示权限说明且不发 items 请求。

## 8. 规则分析 Tab

顶部：最新 run 状态、rule catalog digest、输入 sequence 范围、耗时、hit 数。

列表字段：规则名/键/版本、严重级别、item/turn、matched signal、脱敏 excerpt、
置信度、状态。点击“定位证据”切到会话内容 Tab 并高亮 code point range。

状态：

- clean：本次范围无命中；
- matched：显示所有 hit；
- pending/running：进度 skeleton；
- failed：safe error 和重试提示，不显示 clean；
- superseded：历史 run 折叠展示。

规则目录抽屉展示 ASR-PROMPT-001..010 的说明、目标 item、matcher 类型、默认
级别和 digest，不展示可被绕过的完整内部 regex 细节给无管理权限角色。

## 9. AI 分析 Tab

### 9.1 结论卡

- verdict、severity、confidence；
- summary、risk categories/stage；
- evidence/counter-evidence；
- uncertainties、recommended disposition；
- prompt/model/schema version；
- final/incremental/manual 标识。

### 9.2 Chunk 进度

表格：chunk index、sequence range、estimated input、actual input/output、状态、
verdict、耗时、retry。默认只展示结构化摘要，不返回 provider 原始请求/响应。

### 9.3 Token 卡

```text
会话可见内容（估算）  ~12.3k
来源调用 input/output   80k / 5.2k（partial）
来源 cache read/create  42k / 8k
Aegis AI input/output   18.6k / 2.1k（actual）
```

缺失项显示 `-` 和原因。禁止把 null 渲染为 0。

### 9.4 操作

有 `ai:run` 权限时显示“重新分析”。确认框说明范围、预估 chunk 数、预估输入 Token
和可能成本，但不展示/提交正文。重复 input digest 默认返回现有 run，只有明确
`force_new_run` 且有权限才新建。

## 10. 关联行为 Tab

复用现有 Agent Guard evidence UI 语言：

- tool call -> PID/start ticks -> process/file/network/identity event -> finding；
- confirmed/probable/ambiguous/unattributed badge；
- intent requested/planned/attempted 与 executed 分开展示；
- 可跳转到“智能体事件感知与防护”的相同 session/finding 深链；
- 不复制完整进程树到会话表，只按需加载关联证据。

## 11. 采集信息 Tab

显示：

- content mode、coverage 和 coverage reasons；
- source mode（static scan/backfill）、attestation、version、schema fingerprint；
- first/last sequence、missing ranges；
- parser version、scan interval、last scan、cursor offset/lag、spool/retry 摘要；
- 来源目录/文件可用性、最近文件 mtime 和状态推断依据；
- Token estimation method/version/time。

不显示明文 transcript path、home、source session raw ID 或 username。

## 12. Store

Pinia state 至少分离：

```text
overview
sessions / total / query
selectedSession
items / cursors / targetWindow
ruleRuns / ruleHits
aiRuns / aiChunks
relatedBehaviors
collectionStatus
loading.*
errors.*
realtimeState
requestSequence.*
```

关闭 drawer 时清除正文、excerpt 和 chunk detail，metadata 列表可保留。登出、权限
变化、路由离开后立即清空敏感 state。

## 13. i18n

必须同步 `zh-CN` 和 `en-US`：

- 页面/路由/菜单；
- Agent、coverage、content mode、rule/AI status；
- Token 三类指标和 tooltip；
- item types、风险类别、stage；
- loading/empty/error/permission/stale/degraded；
- 操作确认和审计提示。

不得在模板硬编码只有中文的风险状态，也不得直接显示内部英文 enum。

## 14. 前端安全

- 正文不写 localStorage/sessionStorage/IndexedDB；
- 不写 console、analytics、Sentry breadcrumb 或错误上报；
- query/url/title 不含正文；
- markdown 禁止 raw HTML、iframe、image、自动链接预览；
- tool args/result 以安全 JSON tree/纯文本渲染；
- copy 使用显式按钮并审计；
- API 403 后清除对应正文 state；
- 页面不可将后端错误对象直接 stringify。

## 15. 前端测试

### 15.1 API/类型

- query arrays、page/sort/token range 序列化；
- cursor item API；
- 403/404/409/503 safe error；
- null usage 不变成 0；
- rule/AI/coverage enum fallback。

### 15.2 页面

- 导航/路由/权限；
- KPI loading/stale/degraded；
- 筛选更新 query、旧响应不覆盖；
- empty/no match/permission denied/load failed；
- drawer tab 深链和关闭清理；
- realtime reconnect 保留数据。

### 15.3 内容和安全

- user/assistant/tool/permission/compact/subagent 渲染；
- redacted/truncated/suppressed badge；
- XSS/HTML/image payload 不执行；
- 无 content 权限不请求正文；
- copy 审计调用；
- DOM/URL/storage/console 无未授权正文。

### 15.4 分析

- rule hit 定位和 Unicode offset；
- rule matched + AI benign/resisted 的分歧表达；
- AI chunk progress/retry/failed/inconclusive；
- Token 估算、来源 usage、AI actual usage 分栏和 tooltip；
- behavior probable 不显示 confirmed。

## 16. 前端验收

1. 页面结构与现有 EventProtection 保持一致的视觉层级。
2. 20/50/100 分页和筛选均由服务端执行。
3. 500+ item 会话可流畅定位风险，不一次加载全文。
4. 规则和 AI 结果独立、分歧可理解。
5. Token 指标含来源、方法、coverage，不误导为精确计费。
6. 未授权、partial、AI failed 均不会显示“安全”。
7. i18n、响应式、键盘焦点和 screen-reader label 通过。
