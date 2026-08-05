# Aegis V6.2 Agent Guard 前端设计

**版本**：6.2  
**日期**：2026-08-06
**状态**：Agent Guard 运行时设置、真实会话分页、工具命中安全分析和只读内置规则目录已按当前实现更新；完整 P5 会话正文 UI 仍待实施

> 当前实现基线见 [current_implementation_baseline_2026-08-06.md](current_implementation_baseline_2026-08-06.md)。本文后文的旧策略编辑/发布流程仅作为历史 Bundle 兼容设计，不是当前页面入口。

## 1. 设计目标

前端需要让用户清楚回答六个问题：

1. 每台主机有哪些 AI Agent、每种 Agent 有几个运行实例，实际执行进程在哪里。
2. 一个 Agent session 依次执行了哪些命令、操作了哪些文件、连接了哪些网络目标、改变了哪些权限或隔离资源。
3. 哪些可信工具调用命中了规则，规则引用了哪个工具事件及其可关联 PID/PPID。
4. Aegis 智能分析器给出了什么攻击性结论、反证和不确定性，该结论是否经过规则佐证。
5. Agent 是否真的处于 OS 隔离中，Aegis 能监控还是能阻断。
6. 出现高危行为后，哪个执行单元被暂停、是否成功、由谁恢复。

UI 不得把以下状态混为一谈：

- “Agent 已识别”与“Agent 正在运行”。
- “配置了沙箱”与“已验证隔离生效”。
- “监控到行为”与“已阻断”。
- “命令已下发”与“执行单元已冻结”。
- “远程执行”与“远程已受保护”。
- “行为事实”与“规则/智能分析结论”。
- “没有观察到行为”与“行为被证明不存在”。
- “推导 session”与“Agent 官方 session”。

## 2. 与当前前端集成

当前前端已经存在：

- `/hosts/assets/ai-agents` AI Agent 资产列表。
- `/detection/alerts` 告警中心。
- `/detection/policies` 阻断策略。
- `/settings/ebpf-hooks` 动态 eBPF Hook 设置。
- Vue 3、Element Plus、Pinia、Axios、ECharts、WebSocket 和中英文 i18n。

V6.2 必须继承当前前端的真实设计系统，而不是建设一套独立大屏：

- `frontend/src/App.vue`：深色左侧导航、64px 顶栏、`SECURITY OPERATIONS` kicker、面包屑和 24px 主内容间距。
- `frontend/src/styles/aegis-theme.css`：浅蓝渐变工作区、白色圆角卡片、蓝色主操作以及 critical/high/medium/low 风险色。
- `frontend/src/views/detection/Overview.vue`：统计卡片与内容卡布局。
- `frontend/src/views/detection/Alerts.vue`、`Rules.vue`、`Policies.vue`：筛选卡、表格、分页、开关、危险操作样式。
- `frontend/src/components/ProcessTree.vue`：进程关系、PID/PPID、cmdline 和风险节点的现有表达方式。

V6.2 在侧边栏新增独立“智能体防护”父菜单，P5 后父菜单下有三个可见子菜单：

1. 智能体事件感知与防护。
2. 智能体逃逸防护。
3. 智能体会话检测。

策略、规则、运行实例、行为流水和安全结论作为事件/逃逸子页内部区域、抽屉或
详情状态；会话列表、会话分析、风险标记和采集覆盖作为会话检测子页内部区域，
不再占用一级或二级侧边栏菜单。不把 Agent Guard 策略混入通用 MITRE 阻断
策略，因为 Agent Guard 的目标是 Agent 类型、全行为域、关联规则和隔离规则，
不是单一 MITRE ID。

## 3. 路由

```text
/detection/agent-guard
  redirect -> /detection/agent-guard/events

/detection/agent-guard/events
  智能体事件感知与防护

/detection/agent-guard/escape
  智能体逃逸防护

/detection/agent-guard/sessions
  智能体会话检测
```

Agent 详情不导航到独立页面，通过 `asset_id/instance_id/finding_id/event_id`
和 `detail_tab=panorama|analysis` 打开当前子页的 `AgentDetailDrawer`。刷新和
分享 URL 后必须恢复抽屉。

在现有 `frontend/src/router/index.ts` 中使用 lazy import，路由 meta 增加：

```ts
meta: {
  titleKey: 'routes.agentGuardEvents',
  permission: 'agent_guard:read'
}
```

AI Agent 资产列表中的“运行防护”入口跳转：

```text
/detection/agent-guard/events?asset_id=<asset-id>&detail_tab=panorama
```

告警中心中 `judgment_source=agent_guard_rule|agent_guard_ai|agent_guard_combined`
的告警跳转到 `/detection/agent-guard/events?finding_id=<id>&detail_tab=analysis`。
隔离逃逸告警跳转到
`/detection/agent-guard/escape?event_id=<event-id>&detail_tab=analysis`。

## 4. 推荐目录

```text
frontend/src/
├── api/
│   └── agentGuard.ts
├── types/
│   └── agentGuard.ts
├── store/
│   └── agentGuard.ts
├── views/detection/AgentGuard/
│   ├── AgentGuardLayout.vue
│   ├── EventProtection.vue
│   ├── EscapeProtection.vue
│   ├── InstanceDetail.vue
│   ├── BehaviorDetail.vue
│   ├── FindingDetail.vue
│   └── components/
│       ├── AgentGuardPageHeader.vue
│       ├── AgentGuardFilters.vue
│       ├── GuardMetricCards.vue
│       ├── AgentSummaryTable.vue
│       ├── AgentDetailDrawer.vue
│       ├── AgentRuntimeSelector.vue
│       ├── AgentBehaviorDetailTabs.vue
│       ├── AgentEscapeDetailTabs.vue
│       ├── CoverageBadge.vue
│       ├── CoverageReasonList.vue
│       ├── AgentIdentityCard.vue
│       ├── ExecutionUnitTable.vue
│       ├── IsolationBaselinePanel.vue
│       ├── IsolationDiffViewer.vue
│       ├── GuardProcessTree.vue
│       ├── BehaviorTimeline.vue
│       ├── AgentPanoramaExplorer.vue
│       ├── AgentSecurityProcessTree.vue
│       ├── AgentSecurityAnalysis.vue
│       ├── BuiltinRuleCard.vue
│       ├── AgentGuardRuntimeSettingsDialog.vue
│       ├── FindingEvidenceGraph.vue
│       ├── AnalysisVerdictPanel.vue
│       ├── EvidenceCompletenessBadge.vue
│       ├── BuiltinPolicyCatalog.vue
│       ├── GuardActionDialog.vue
│       └── GuardActionStatus.vue
└── i18n/locales/
    ├── zh-CN/
    └── en-US/
```

遵守 V6.1 i18n 设计：路由和稳定模块文案使用结构化 key；自动抽取文案继续走现有 generated 机制，不在组件中新增裸字符串。

## 5. 导航设计

侧边栏新增与“智能异常检测”同级的 `el-sub-menu`：

```text
智能体防护
  智能体事件感知与防护
  智能体逃逸防护
  智能体会话检测
```

禁止把“概览、策略、内置规则、运行实例、行为全景、行为流水、安全结论”继续展开为七个并列子菜单。它们分别收纳为：

- “智能体事件感知与防护”：统计、五个内置规则、运行实例、行为全景树、行为事件、Finding、智能分析和策略配置入口。
- “智能体逃逸防护”：沙箱覆盖、沙箱执行树、隔离基线/diff、逃逸事件和 freeze/resume/kill。

三个子菜单使用当前 `App.vue` 的蓝青渐变激活态；页面不再增加一套横向产品导航。页面内部可以使用筛选器、分段控件、抽屉和详情路由，但不能形成第三层全局菜单。

视觉与产品交互基线以
[agent_guard_frontend_prd_v6.2.md](agent_guard_frontend_prd_v6.2.md)
及 [agent_session_detection_frontend_prd_v6.2.md](agent_session_detection_frontend_prd_v6.2.md)
为准。

## 6. 子页一：智能体事件感知与防护

外层页面只展示 KPI、筛选和 Agent 基本信息列表。策略、规则、运行实例、
行为全景、行为流水、Finding 和智能分析都在全局配置入口或选中 Agent 的
详情抽屉中呈现，不能在外层列表下方直接展开。

### 6.1 顶部统计

- Agent 资产数。
- 正在运行实例数。
- high/critical Finding 数。
- 已成功 deny/freeze 数。

### 6.2 外层 Agent 基本信息列表

一行表示 `host + Agent asset`，字段：

- Agent 显示名称和类型。
- 主机。
- 运行实例数。
- controller PID 摘要。
- 运行状态。
- coverage 状态。
- high/critical Finding 数。
- 最近活动。
- “查看详情”。

外层禁止展示进程树、cmdline、文件路径、外链、隔离基线、规则命中正文、
智能分析正文和 freeze/resume/kill。

### 6.3 Agent 详情抽屉

点击行或“查看详情”打开 `AgentDetailDrawer`：

- 标题：`智能体详情 · <Agent>`。
- 头部：主机/IP、类型、实例数、防护状态。
- 会话 ID selector：服务端分页展示 Native Hook 真实会话；运行实例作为辅助筛选，
  不用 controller PID 伪造会话。
- 仅两个 tabs：`行为全景`、`安全分析`。

`行为全景` 展示当前会话的行为事实和实际 PID/PPID/cmdline；`安全分析` 只展示当前
会话命中的规则名称、工具名称、工具输入/结果、匹配命令行和可关联 PID/PPID，
不展示全量进程树，也不把全量 Finding 混入当前会话。安全分析页签标题不显示命中数量。
抽屉关闭后保留外层分页、筛选和滚动位置。

本节之后的“策略管理、运行实例、实例详情、行为流水与安全结论”均为全局
配置入口或详情抽屉内部模块，不是外层页面区块和侧边栏菜单。

## 7. 子页一内部视图：策略兼容数据与内置规则目录

当前页面不提供策略新建、编辑、发布、停用或下发状态操作。`agent_guard_policies`
和 Bundle 仍由后端保留，用于通用 OS 原子规则、隔离逃逸、旧版本兼容和审计；当前
Native Hook/工具适配器通过页面设置按钮使用 `agent_guard_runtime_settings.v1` 即时
下发。下面的策略管理小节仅描述历史兼容接口，不应实现为当前 UI。

### 7.1 历史策略 API 兼容说明（当前 UI 不实现）

字段：

- 策略名称/key。
- 当前版本。
- 状态。
- 优先级。
- Agent 范围。
- 主机范围。
- 采集分类数。
- 原子规则数。
- 关联规则数。
- 智能分析策略状态。
- 逃逸规则数。
- applied/failed/stale 主机数。
- 更新时间/发布人。

操作：

- 查看。
- 编辑 draft。
- 复制为新版本。
- 校验。
- 发布。
- 停用。

历史 published 策略不能原地编辑，接口层点击编辑应创建新 draft version；当前页面
不提供该操作，避免把内置规则目录误解为用户策略发布器。

### 7.2 策略编辑

五步表单：

#### 第一步：范围

- 策略名称和说明。
- Agent 类型：Codex/OpenClaw/Hermes/全部。
- 可选 Profile。
- 主机/主机组。
- 优先级。

#### 第二步：行为采集

配置项：

- 行为域：process/file/network/identity/persistence/isolation/kernel/ipc/tool/control。
- 命令 argv：仅允许 redacted。
- file/network content：固定 disabled。
- 普通 read/write 聚合窗口。
- 工具语义来源与不可观测提示。
- 数据保留级别和高危证据不可采样说明。

#### 第三步：规则与资源

两个编辑区：

1. 原子规则：通用单次行为或资源规则，可以编译到 Agent 本地。
2. 关联规则：通用 OS 事件的序列/聚合规则；Agent Guard 工具命令规则不在此处
   编辑，由 api-server 对可信工具事件匹配。

规则字段：

- 行为 category/operation/outcome。
- 资源 type/classification/path/network/identity。
- 时间窗口和 group key。
- allow/negative conditions。
- 动作：audit/alert/deny/deny_and_freeze。
- 严重级别和置信度。

交互要求：

- 文件资源路径必须绝对路径。
- 输入 `..`、shell 字符或不支持 glob 时立即提示。
- `deny` 只能用于后端确认可编译的原子规则；关联规则不能伪装成内核同步 deny。
- `deny_and_freeze` 展示影响警告。
- 主机不支持 enforcement 时，预览明确显示将降级为 `would_deny`。

#### 第四步：智能分析

- 启用条件：finding severity、规则族、Agent/主机范围。
- evidence window。
- AI-only action ceiling，第一版最高为 alert。
- 分析超时、失败和 inconclusive 展示说明。
- 明确提示模型不读取文件内容、stdin/stdout/stderr，也没有主机工具或阻断权限。

#### 第五步：隔离逃逸

规则按分类选择：

- namespace。
- mount/root。
- container runtime。
- cgroup。
- ptrace。
- BPF/module。
- capability。

每条规则显示：

- 监控对象。
- 可能的正常场景。
- 支持的隔离族。
- 推荐动作。
- 当前选择动作。

### 7.3 发布确认

发布前调用 validate API，展示：

- 规范化规则。
- 本地原子规则与服务端关联规则的执行位置。
- 采集数据量估算和聚合策略。
- 智能分析触发量估算、AI-only 动作上限。
- 冲突/覆盖关系。
- 目标主机数。
- full enforcement 数。
- monitor-only 数。
- unsupported/remote-unobservable 数。
- 将被 freeze 规则数量。

确认弹窗必须要求输入发布理由；不能只显示“确定发布吗”。

### 7.4 下发状态

PolicyDeliveryTable：

- 主机。
- Agent 版本。
- bundle version/digest 短值。
- capability。
- coverage。
- status。
- error code/message。
- dispatched/received/applied time。

“发送成功”不能映射成 applied；received 和 applied 分开显示。

### 7.5 内置规则页

在 `/detection/agent-guard/events?view=rules` 或规则目录抽屉中固定展示五个不可删除、
不可编辑的规则，按“行为监控”和“工具命令”两个内置策略视图分组：

| Rule ID | 页面名称 | 关键配置 |
| --- | --- | --- |
| `AGB-BUILTIN-001` | 操作敏感目录 | 资源分组、路径/操作、例外、action |
| `AGB-BUILTIN-002` | 外部网络连接 | trusted CIDR/domain/port、例外、action |
| `AGB-BUILTIN-003` | 文件生成 | 路径/属性风险分层、例外、action |
| `AGB-BUILTIN-004` | 敏感命令执行 | 命令分类、executable/argv 条件、例外、action |
| `AGB-BUILTIN-005` | 提权行为 | attempt/succeeded、capability/namespace、例外、action |

每个 BuiltinRuleCard 显示 rule ID/version、enabled、执行位置、当前 severity/action、今日 hit/finding、例外数和 unsupported 主机数。

每个规则详情展示中文名称、英文名称、描述、类别、版本、严重级别、默认/推荐动作、
执行位置、规则归属、所需 evidence、allow conditions、MITRE、默认参数、参数 Schema
和 digest。页面不提供“配置”、policy override、例外、灰度、发布或删除按钮。
`AGB-BUILTIN-004` 明确标记为“api-server 工具事件命令匹配”；eBPF 关联字段仅是补充证据。

规则页面底部提供“在行为全景中查看”，跳转时携带 `rule_key`、时间和当前 Agent/主机范围。

### 7.6 Agent Guard 设置按钮

“智能体事件感知与防护”页提供“设置”按钮，打开运行时设置对话框：

- 工具调用适配器：`AgentGuardToolAdapterEnabled`，中文名“智能体工具调用采集”。
- 智能体会话 Hook：`AgentGuardSessionHookEnabled`，中文名“智能体会话生命周期采集”。
- Native Hook 注入开关：Codex、Claude Code、OpenClaw、Hermes、Zcode。

开关为真正的启用/关闭样式。打开后 api-server 立即保存并下发，在线 Agent 应用后
开始上报工具和会话事件；关闭后 Agent 清理对应 Hook 配置并停止上报。页面不展示
“待下发、等待 Agent 重连、失败、未启用”等旧状态作为功能状态；若主机离线或应用失败，
只在保存结果/错误提示中说明原因，开关本身表达期望状态。

## 8. 子页一内部视图：运行实例

### 8.1 筛选

- 主机。
- Agent 类型。
- Profile。
- 运行状态。
- 隔离类型。
- 覆盖等级。
- container ID。
- 时间范围。

### 8.2 表格

字段：

- Agent。
- 主机/IP。
- 控制进程 PID。
- 运行用户。
- Profile/version。
- 执行单元数。
- 实际隔离方式。
- 覆盖状态。
- 最近活动。
- 高危 finding 数。
- 当前动作状态。

状态标签必须有 tooltip 说明：

```text
no_isolation:
该 Agent 当前使用本地执行后端，没有可验证的 OS 沙箱边界。
操作系统行为监控仍然有效，但不能称为沙箱逃逸防护。
```

### 8.3 操作

- 查看详情。
- 跳转 AI Agent 资产。
- 对整个实例执行 kill（高风险、二次确认）。

实例列表不直接提供“一键恢复全部”，避免误恢复多个执行单元。

## 9. 子页内部深链：实例详情

### 9.1 Agent 身份卡

- Agent type/display name。
- asset ID 链接。
- Profile key/version。
- 识别证据和 confidence。
- controller PID/start/exe/cmdline（默认脱敏折叠）。
- run user。
- started/last seen。

### 9.2 执行单元表

- type。
- root PID。
- container/cgroup。
- namespace 摘要。
- seccomp/no_new_privs/capability。
- coverage。
- status。
- first/last seen。
- freeze/resume/kill。

remote unit：

- backend。
- remote execution/session ID。
- remote host reference。
- 远端 Aegis Agent 关联状态。

### 9.3 Behavior Session

session 列表显示：

- session ID 摘要。
- source：Native Hook/verified adapter；旧的 execution unit/activity window 仅作为历史兼容来源。
- confidence：真实 Hook 会话使用 confirmed；无可信 ID 的行为进入未归属索引，不生成可选择的官方 session。
- 开始/结束时间。
- 命令、文件、网络、权限、隔离行为计数。
- finding 数和最高风险。
- 证据完整性：drop、truncated、tool semantics、remote visibility。

会话 ID 服务端分页，当前安全分析只能绑定一个选中的真实会话；不能把全量 Finding
或其他会话的行为填入当前会话。

### 9.4 隔离基线

IsolationBaselinePanel 分成：

- Namespace。
- Cgroup。
- Mount。
- Capability。
- Seccomp/no_new_privs。
- 风险入口。

IsolationDiffViewer 只在发生漂移时展示 before/after，差异字段高亮。

### 9.5 进程树

GuardProcessTree 节点：

- PID/name。
- exe/cmdline 摘要。
- 控制进程/worker/container process 角色。
- namespace/cgroup。
- 最近 process/file/network/identity/isolation 行为计数。
- 关联 finding 数。

节点颜色表达角色，不表达安全结论。

### 9.6 时间线

统一时间线事件：

```text
instance started
session started/inferred
execution unit started
process exec/exit
file open/read/write/rename/chmod
network connect/listen
identity/capability change
sandbox violation
rule finding created/updated
AI analysis pending/succeeded/failed
freeze requested/succeeded/failed
manual resume
unit stopped
```

每项带 behavior/finding/analysis/action ID，可打开详情。

### 9.7 Agent 行为全景树

全景树只在选中 Agent 的 `AgentDetailDrawer > 行为全景` tab 中加载。
抽屉顶部先分页选择真实 Session ID，再按 runtime instance 展示，树固定主干：

```text
Selected Agent asset/type
└── Agent runtime instance：controller PID / start_ticks
    └── Session
        └── Execution unit
            └── Process：PID / PPID / cmdline
                ├── Child process：PID / PPID / cmdline
                ├── Command：PID / executable / cmdline / exit
                ├── File：operation / file name / full path / outcome
                ├── Network：destination IP/domain:port / protocol / outcome
                ├── Privilege：before -> after / outcome
                └── Rule/Finding：rule ID / severity / action
```

操作必须挂在真实发起 PID 下，不能单独按事件类型组成与进程无关的树。具体节点：

- Agent asset/type：Agent 产品、资产 ID、Profile、运行实例数和最高风险。
- Runtime instance：controller PID/start ticks、状态、覆盖、session/unit 数。
- Process：首行 `name · PID · PPID`，第二行展示脱敏 cmdline。
- File：首行 `operation · file_name`，第二行展示完整 resolved path。
- Network：首行展示 `destination_ip:port / protocol`，有可信 DNS 关联时同时展示 observed domain。
- Command：展示 PID、executable、cmdline、cwd、exit code/signal。
- Privilege：展示 UID/GID/capability before/after 以及 attempted/succeeded/inconclusive。
- Rule/Finding：展示五个内置规则的 ID/version、severity、decision source 和 action。

页面布局：

```text
抽屉顶部：Agent 基本信息和 Session ID 分页 selector
Tab 顶部：运行实例/时间/五规则/风险/行为域筛选
左侧：可虚拟滚动、懒加载的行为全景树
右侧：选中节点完整证据

安全分析 tab 使用命中规则列表和工具调用列表，不使用全量行为全景树：

```text
命中规则名称
  -> 工具名称 + tool_call_id
  -> 工具输入/结果
  -> 匹配命令行
  -> 关联 PID/PPID/cmdline 或 unattributed
```
```

交互：

- 选择具体 Session 时默认展开至首层 process；选择“全部实例”时各 instance
  为并列分支，不得合并 session/unit/process。
- 点击节点按 cursor 加载子进程和操作。
- 同一进程子项按 `occurred_at + agent_sequence` 排序。
- 搜索支持 PID、cmdline、文件名、完整路径、IP、domain。
- “仅风险”只隐藏无规则命中的操作，不破坏 Agent/instance/进程祖先路径。
- 点击 rule hit 高亮对应行为节点；点击 finding 展示引用节点集合。
- PID reuse 以 `PID + start_ticks` 形成不同节点。
- 树根和节点展示 drop、truncated、remote/tool unobservable。
- Session selector、运行实例和行为事件都使用服务端分页；安全分析不显示命中数量后缀。

详细节点/API 契约见
[builtin_behavior_rules_and_panorama_tree_v6.2.md](builtin_behavior_rules_and_panorama_tree_v6.2.md)。

## 10. 子页一内部视图：行为流水与安全结论

### 10.1 行为流水

行为页按 category tabs：

```text
全部 | 命令/进程 | 文件 | 网络 | 身份权限 | 持久化 | 隔离/内核 | 工具/控制
```

筛选：

- 主机/Agent/实例/session/执行单元。
- category/operation/outcome。
- resource type/classification/keyword。
- decision。
- severity。
- policy。
- 时间。

- 时间。
- Agent/主机。
- 进程。
- category/operation/outcome。
- resource 摘要。
- policy/rule。
- decision。
- visibility/completeness。

命令 argv、路径和网络目标默认单行省略并 tooltip，按 `agent_guard:evidence:read` 权限返回详情。任何页面都不展示文件内容、网络内容、stdin/stdout/stderr 或环境变量值。

行为详情分区：

1. 行为事实：category、operation、outcome、errno。
2. Actor：PID/start time、exe、脱敏 argv、cwd、进程链。
3. Resource：类型、分类、身份摘要和元数据。
4. Session/执行单元/隔离上下文。
5. Collection：sensor、visibility、truncated fields、drop counter。
6. 本地策略 decision。
7. 引用该行为的 finding/action。
8. 原始 JSON（管理员权限、折叠展示）。

### 10.2 安全结论

Finding 列表字段：

- 时间、主机、Agent、session。
- 标题、规则族和攻击阶段。
- severity/verdict/confidence。
- source：rule/AI/combined。
- 证据事件数和 evidence completeness。
- analysis status。
- action 状态和处置状态。

Finding 详情：

1. **安全结论**：verdict、confidence、severity，清楚标识规则事实和 AI 研判。
2. **攻击链**：阶段、时间、进程、资源和引用 event ID。
3. **规则命中**：rule ID/version、条件、窗口、allow/negative condition。
4. **智能分析**：summary、intent hypotheses、counter evidence、uncertainties、recommended action。
5. **证据图**：每个结论可以点击到原始 behavior。
6. **证据完整性**：丢失、截断、工具语义不可观测和远程盲区。
7. **策略和动作**：policy version、自动/人工动作及真实状态。
8. **分析历史**：model/provider/prompt version、input digest、状态和耗时。
9. **处置记录**：调查中、已控制、已解决或误报，以及操作者。

Agent Guard 工具命中详情必须额外展示：

```text
命中的规则名称（中文，支持英文名称/规则 ID 辅助查看）
工具名称
tool_call_id
工具输入和结果摘要
匹配出的命令行
PID / PPID / cmdline（eBPF 关联成功时）
session ID
规则归属：api-server
直接证据：Hook 工具 raw event ID
```

安全分析查询必须携带选中 `session_id`；没有命中工具事件时只显示该会话为空，
不能回退到全量主机或全量 Agent findings。命令行按 Hook 工具输入的规范化/脱敏
结果显示，使用参数边界和空格分隔，不把 JSON 原始转义串直接当作用户可读命令行。

AI-only finding 显示醒目标识：

```text
该结论来自智能分析，尚无满足自动阻断条件的确定性规则证据。
```

对于 `would_deny`：

```text
策略期望拒绝，但该主机不具备所需内核能力。本次操作未被证明已阻断。
```

## 11. 子页二：智能体逃逸防护

### 11.1 顶部统计与筛选

顶部固定展示：

- Agent 资产。
- 受监控实例。
- 逃逸尝试。
- 已冻结。

外层筛选项为主机、Agent 类型、运行状态、隔离方式、coverage，以及
`Agent 名称 / controller PID / 主机` 关键字。筛选、查询和重置样式直接复用
`Alerts.vue` 与 `Policies.vue`。

### 11.2 外层 Agent 基本信息列表

一行表示 `host + Agent asset`，复用事件子页基础字段，并增加隔离方式、
逃逸 Finding 数和当前处置状态。外层不得展示 execution unit、container ID、
基线差异、沙箱执行树或动作按钮。

### 11.3 逃逸防护详情抽屉

点击 Agent 打开 `AgentDetailDrawer` 的 escape 模式：

- 标题：`逃逸防护详情 · <Agent>`。
- 头部：主机/IP、类型、实例数、隔离方式和最高风险。
- runtime instance selector。
- 仅两个 tabs：`沙箱全景`、`逃逸分析`。

`沙箱全景` 左侧以选中 Agent 的真实执行边界为树根：

```text
Selected Agent asset/type
└── Agent runtime instance
    └── execution unit / container / namespace
        └── process：PID / PPID / cmdline
            ├── runtime socket access
            ├── setns / unshare attempt
            ├── mount/root drift
            ├── cgroup drift
            ├── capability change
            └── escape rule / action
```

只有已声明隔离边界的 execution unit 才显示“逃逸”结论。
`no_isolation`、`monitor_only` 和 `remote_unobservable` 必须显示覆盖原因，
不能用绿色“安全”状态替代不可观测事实。

沙箱全景右侧上方 `IsolationBaselinePanel` 固定展示 Namespace、Cgroup、Mount、
Capability 和 Seccomp/no_new_privs。发生漂移时显示 expected/observed，
正常项保持紧凑状态行。

右侧下方展示当前逃逸事件：

- PID、PPID 和脱敏 cmdline。
- execution unit/container/cgroup。
- 目标 namespace、runtime socket 或隔离资源。
- 命中规则、规则版本和证据。
- decision 与 action 状态。
- 请求、下发、Agent 接收和终态时间。

`逃逸分析` tab 展示 Finding、规则/策略、attempt 与 drift 关联、智能研判、
证据完整性和处置历史。

抽屉底部只保留“查看证据、恢复执行、终止执行单元”三个上下文动作；
freeze 由策略自动触发或事件详情中的受控操作触发。
动作详情固定展示目标 execution unit，并提示“仅影响当前执行单元，不影响
本机其他 Agent”。禁止主机级批量 freeze。

## 12. 人工动作交互

### 12.1 Freeze

确认内容：

- 主机。
- Agent。
- 执行单元。
- 影响的 PID 数。
- 容器/cgroup。
- freeze timeout。
- 是否已存在高危事件。

提交后状态：

```text
pending -> dispatching -> running -> success/failed
```

按钮不能在 API 返回 accepted 后立即变成“已冻结”。

### 12.2 Resume

只在 unit status frozen 时启用。要求填写理由。

如果 deny 规则仍然生效，提示：

```text
恢复只解除暂停，不会关闭内核拒绝策略。
```

### 12.3 Kill

- `kill_execution_unit` 与 `kill_agent_instance` 分开。
- 必须二次确认。
- 展示 protected target 校验结果。
- 整个实例 kill 要求输入 Agent 名称或确认短语。

### 12.4 失败

显示 Agent 原始结构化错误：

- offline。
- unit not found/stopped。
- cgroup freezer unavailable。
- protected target。
- PID identity changed。
- permission denied。
- timeout。

不统一改成“操作失败”。

## 13. TypeScript 类型

```ts
export type AgentGuardCoverage =
  | 'full_enforcement'
  | 'behavior_monitor_escape_enforce'
  | 'monitor_only'
  | 'no_isolation'
  | 'remote_unobservable'
  | 'unsupported_profile'
  | 'degraded'

export type AgentGuardDecision =
  | 'allow'
  | 'audit'
  | 'alert'
  | 'deny'
  | 'deny_and_freeze'
  | 'would_deny'
  | 'enforcement_unavailable'

export type ExecutionUnitType =
  | 'local_process_tree'
  | 'linux_namespace'
  | 'oci_container'
  | 'remote_sandbox'
  | 'whole_process_container'

export interface AgentGuardAgentSummary {
  agent_scope_key: string
  asset_id?: string
  host: {
    id: string
    hostname: string
    ip: string
  }
  agent_type: string
  display_name: string
  profile_key?: string
  running_instance_count: number
  controller_pids: number[]
  runtime_status: 'running' | 'stale' | 'stopped' | 'unknown'
  isolation_types: ExecutionUnitType[]
  coverage_level: AgentGuardCoverage
  coverage_reasons: string[]
  high_risk_finding_count: number
  escape_finding_count: number
  action_status?: string
  last_seen_at?: string
}

export interface AgentRuntimeInstance {
  id: string
  host_id: string
  asset_id?: string
  agent_type: string
  display_name?: string
  profile_key: string
  profile_version: number
  controller_pid: number
  controller_start_ticks: string
  run_user?: string
  detection_confidence: 'candidate' | 'probable' | 'confirmed'
  status: 'running' | 'stale' | 'stopped' | 'unknown'
  coverage_level: AgentGuardCoverage
  coverage_reasons: string[]
  execution_unit_count: number
  behavior_session_count: number
  high_risk_finding_count: number
  first_seen_at: string
  last_seen_at: string
}

export interface AgentExecutionUnit {
  id: string
  instance_id: string
  unit_type: ExecutionUnitType
  root_pid?: number
  cgroup_id?: string
  cgroup_path?: string
  container_id?: string
  remote_backend?: string
  coverage_level: AgentGuardCoverage
  coverage_reasons: string[]
  status: string
  isolation_baseline: Record<string, unknown>
  isolation_actual: Record<string, unknown>
  isolation_diff: Record<string, unknown>
  first_seen_at: string
  last_seen_at: string
}

export type AgentBehaviorCategory =
  | 'process'
  | 'file'
  | 'network'
  | 'identity'
  | 'persistence'
  | 'isolation'
  | 'kernel'
  | 'ipc'
  | 'tool'
  | 'control'

export interface AgentBehaviorSummary {
  id: string
  instance_id: string
  session_id?: string
  execution_unit_id?: string
  category: AgentBehaviorCategory
  operation: string
  outcome: 'success' | 'failure' | 'denied' | 'unknown'
  actor: Record<string, unknown>
  resource: Record<string, unknown>
  decision: AgentGuardDecision
  collection: {
    visibility: 'complete' | 'partial' | 'unobservable'
    truncated_fields: string[]
    lost_events_since_last: number
  }
  occurred_at: string
}

export interface AgentSecurityFindingSummary {
  id: string
  title: string
  severity: 'info' | 'low' | 'medium' | 'high' | 'critical'
  verdict: 'benign' | 'suspicious' | 'malicious' | 'inconclusive'
  confidence: number
  decision_sources: Array<'rule' | 'ai' | 'combined'>
  evidence_event_count: number
  analysis_status?: string
  status: 'open' | 'investigating' | 'contained' | 'resolved' | 'dismissed'
  last_observed_at: string
}

export interface BuiltinAgentBehaviorRuleSummary {
  rule_key:
    | 'AGB-BUILTIN-001'
    | 'AGB-BUILTIN-002'
    | 'AGB-BUILTIN-003'
    | 'AGB-BUILTIN-004'
    | 'AGB-BUILTIN-005'
  rule_version: number
  name: string
  name_en?: string
  description?: string
  categories: string[]
  default_enabled: boolean
  engine: 'agent_atomic' | 'dc_single_event' | 'dc_correlation' | 'agent_and_dc'
  execution_location?: 'agent_local' | 'dc_runtime' | 'api_server_tool_event' | string
  rule_owner?: 'agent' | 'dc' | 'api-server' | string
  severity: 'info' | 'low' | 'medium' | 'high' | 'critical'
  action: AgentGuardDecision
  recommended_action?: AgentGuardDecision
  required_evidence?: unknown[]
  allow_conditions?: unknown[]
  mitre?: unknown[]
  default_parameters?: Record<string, unknown>
  parameters_schema?: Record<string, unknown>
  digest?: string
}

export interface AgentGuardRuntimeSettings {
  schema: 'aegis.agent_guard.runtime_settings.v1'
  version: number
  host_id: string
  tool_adapter_enabled: boolean
  session_hook_enabled: boolean
  injections: Array<{
    agent_type: 'codex' | 'claude-code' | 'openclaw' | 'hermes' | 'zcode' | string
    enabled: boolean
    status?: string
    error_code?: string
  }>
  dispatch_status?: string
  dispatch_error_code?: string
}

export type PanoramaNodeType =
  | 'agent_asset'
  | 'instance'
  | 'session'
  | 'execution_unit'
  | 'process'
  | 'command'
  | 'file'
  | 'network'
  | 'privilege'
  | 'isolation'
  | 'rule_hit'
  | 'finding'

export interface PanoramaTreeNode {
  id: string
  parent_id?: string
  node_type: PanoramaNodeType
  occurred_at?: string
  label: string
  severity?: 'info' | 'low' | 'medium' | 'high' | 'critical'
  outcome?: 'success' | 'failure' | 'denied' | 'unknown'
  has_children: boolean
  child_count: number
  data: {
    pid?: number
    ppid?: number
    start_ticks?: string
    cmdline?: string
    operation?: string
    file_name?: string
    resolved_path?: string
    destination_ip?: string
    destination_port?: number
    protocol?: string
    observed_domain?: string
    [key: string]: unknown
  }
}
```

API 类型继续遵守当前：

```ts
interface ApiResponse<T> {
  code: number
  message: string
  data: T
}
```

分页统一 `{ items, total }`，不要重现 `data/items/logs` 字段不一致。

## 14. API 客户端

`frontend/src/api/agentGuard.ts`：

```ts
getAgentGuardOverview()
getAgentGuardCoverage(params)
listAgentGuardAgents(params)
listBuiltinAgentBehaviorRules(params)
getBuiltinAgentBehaviorRule(ruleKey)
previewBuiltinAgentBehaviorRule(ruleKey, payload)
listAgentGuardPolicies(params)
getAgentGuardPolicy(id)
createAgentGuardPolicy(payload)
updateAgentGuardPolicy(id, payload)
validateAgentGuardPolicy(id)
publishAgentGuardPolicy(id, reason)
disableAgentGuardPolicy(id, reason)
listPolicyDeliveries(id, params)
listAgentGuardInstances(params)
getAgentGuardInstance(id)
getAgentGuardProcessTree(id)
listAgentBehaviorSessions(instanceId, params)
getAgentBehaviorSession(id)
getAgentBehaviorTimeline(id, params)
getAgentPanorama(params)
getAgentInstancePanorama(id, params)
getAgentSessionPanorama(id, params)
getAgentPanoramaNodeChildren(nodeId, params)
getExecutionUnit(id)
listAgentBehaviors(params)
getAgentBehavior(id)
listAgentSecurityFindings(params)
getAgentSecurityFinding(id)
handleAgentSecurityFinding(id, payload)
runAgentSecurityAnalysis(id)
listAgentSecurityAnalyses(id)
getAgentSecurityAnalysis(id)
freezeExecutionUnit(id, payload)
resumeExecutionUnit(id, payload)
killExecutionUnit(id, payload)
killAgentInstance(id, payload)
getAgentGuardAction(id)
```

## 15. Pinia Store

Store 只保存页面状态和短期缓存，不作为动作事实源：

```ts
interface AgentGuardState {
  overview: AgentGuardOverview | null
  agents: AgentGuardAgentSummary[]
  selectedAgentScopeKey: string | null
  selectedInstanceIds: string[]
  detailDrawerOpen: boolean
  detailMode: 'behavior' | 'escape'
  detailTab: 'panorama' | 'analysis'
  builtinRules: BuiltinAgentBehaviorRuleSummary[]
  policies: AgentGuardPolicySummary[]
  instances: AgentRuntimeInstance[]
  behaviors: AgentBehaviorSummary[]
  findings: AgentSecurityFindingSummary[]
  selectedInstance: AgentRuntimeInstanceDetail | null
  selectedBehavior: AgentBehaviorDetail | null
  selectedFinding: AgentSecurityFindingDetail | null
  panoramaRoot: PanoramaTreeNode | null
  panoramaChildren: Record<string, PanoramaTreeNode[]>
  panoramaCursors: Record<string, string>
  selectedPanoramaNode: PanoramaTreeNode | null
  pendingActions: Record<string, AgentGuardAction>
  loading: Record<string, boolean>
  errors: Record<string, string>
}
```

WebSocket 更新策略：

- action_updated：按 action ID 更新。
- behavior_created：行为列表按筛选插入/提示；全景树只标记对应 process `hasNewChildren`，不实时重建整棵树。
- finding_updated：按 finding ID 更新风险、证据数和分析状态。
- analysis_updated：更新当前 finding 的 analysis run，不覆盖规则结论。
- instance_updated：更新行，不重置分页和筛选。
- agent_summary_updated：更新外层 Agent 基本信息行；找不到当前页记录时只更新 KPI。
- delivery_updated：更新当前策略下发统计。
- rule_stats_updated：按 rule key 更新五个内置规则的 hit/finding 统计。

断线重连后重新请求 API，不依赖漏掉的 WebSocket 消息补全状态。

## 16. 页面状态

每个页面必须实现：

- loading。
- empty。
- error + retry。
- partial/degraded。
- stale。
- permission denied。

特殊空状态：

| 场景 | 文案/动作 |
| --- | --- |
| 无 AI Agent 资产 | 引导前往资产采集 |
| 有资产无运行实例 | “已识别安装资产，当前未观察到运行实例” |
| Agent Guard 运行时设置未应用 | 保留开关期望值，在保存结果中显示主机离线/应用错误；不把它展示成策略“待下发” |
| 主机不支持 BPF LSM | 展示 monitor-only 原因，不建议误开 deny |
| remote unobservable | 引导远端部署/关联 Aegis Agent |
| unsupported profile | 展示识别证据，并引导新增/升级 Profile |
| tool semantics unobservable | “未获得 Agent 工具 Hook；OS 行为仍在采集” |
| analysis unavailable | 保留规则 finding，提示智能分析暂不可用 |
| no findings | “当前筛选范围未形成安全结论”，不能显示“Agent 安全” |

## 17. 可访问性和安全

- coverage 不能只靠颜色，必须有文字和图标。
- freeze/resume/kill 按钮具备明确 aria label。
- 大表格支持键盘访问和可见焦点。
- critical 弹窗默认焦点放取消按钮。
- 不把完整命令、路径、URL、analysis evidence 或模型输出写入浏览器 console。
- 命令/路径详情和 analysis 输入摘要使用 `agent_guard:evidence:read` 权限。
- AI verdict 必须使用“研判/置信度”语义，不能显示为已证实事实。
- API 错误不展示后端 stack trace。
- 权限不足时隐藏动作并以后端 403 为最终边界。

## 18. 前端测试

### 18.1 API

- 路径、query、payload 和响应分页。
- stable error code 映射。
- freeze accepted 不映射 success。

### 18.2 组件

- CoverageBadge 所有状态及 reason tooltip。
- Policy editor 采集分类、原子/关联规则、路径/glob、AI-only ceiling 和 deny_and_freeze 校验。
- 五个 BuiltinRuleCard 的稳定 ID/version、override diff、不可删除和 unsupported 状态。
- 只读内置规则目录的中英文名称、详情字段、两个内置策略视图和规则归属。
- Runtime settings 设置对话框的两个开关、五类 Native Hook 注入、开启/关闭即时下发语义。
- Delivery received/applied 分离。
- Instance detail controller 与 execution unit 分离。
- Native Hook 真实 session ID 分页、无可信 ID 的未归属状态。
- BehaviorTimeline 多行为域排序、聚合和 completeness。
- AgentPanoramaTree instance/session/unit/process 层级、懒加载、虚拟滚动和每节点分页。
- 安全分析只展示当前 session 命中的规则、工具、命令行和关联 PID/PPID，不展示全量进程树。
- AgentSummaryTable 一行聚合 host + Agent asset，外层不渲染证据字段。
- AgentDetailDrawer 打开/关闭、query 恢复、实例 selector 和两个固定 tabs。
- 同类型多个 runtime instance 在抽屉内分离，不合并 session/unit/process。
- Process 节点 PID/PPID/cmdline，File 节点 filename/path，Network 节点 destination address/domain/port。
- PID reuse、仅风险保留祖先、rule hit 高亮和搜索定位。
- FindingEvidenceGraph 引用行为可跳转。
- AnalysisVerdictPanel 规则/AI/combined、反证和不确定性。
- Isolation diff before/after。
- Behavior detail `would_deny` 警示。
- Freeze/resume/kill 二次确认和失败原因。
- Freeze/resume/kill 目标只包含一个 execution unit，不改变同机其他 Agent。

### 18.3 Store/WebSocket

- behavior/finding/analysis/instance/action/delivery 增量更新。
- 重连后刷新。
- 分页筛选不被实时消息重置。
- 重复 behavior/finding/analysis/action 消息幂等。

### 18.4 页面

- loading/empty/error/degraded。
- 无权限路由和按钮。
- 外层 Agent 列表不包含进程树、路径、连接地址、基线或分析正文。
- 点击 Agent 打开抽屉，关闭后保留筛选、分页和滚动位置。
- AI Agent 资产到 instance 的跳转。
- Agent Guard alert 到 finding，再到 evidence behavior 的跳转。
- 中英文 key 完整。
- 响应式布局，1280px 和移动窄屏不溢出。

## 19. 前端验收

1. 两个子页外层只展示 Agent 及其主机、实例数、控制 PID、运行/防护等基本信息。
2. 点击 Agent 后才打开详情抽屉；事件抽屉只有行为全景/安全分析，逃逸抽屉
   只有沙箱全景/逃逸分析。
3. 用户能在抽屉中区分控制进程和实际 sandbox/container worker。
4. 用户能在抽屉中分页选择真实 session ID，再按运行实例通过树状全景查看
   命令、文件、网络、权限和隔离行为。
5. 全景树 process 节点展示 PID/PPID/cmdline，文件节点展示文件名称与路径，外链节点展示连接 IP/domain/port。
6. 五个内置规则可以查看完整中英文详情，但不能删除、编辑、启停或通过当前页面发布 policy override。
7. 每个 finding 能展示规则、智能研判、反证、不确定性和引用的原始行为；工具命中额外展示工具、命令行和关联 PID/PPID。
8. AI-only malicious 明确显示未满足自动阻断条件。
9. 证据丢失、截断、工具语义缺失和远程不可观测不会被展示成“无风险”。
10. freeze API accepted 后 UI 保持 pending，直到 Agent 终态。
11. 不支持阻断时 UI 不显示“已阻断”。
12. 所有高风险动作具备原因、二次确认、结果和审计入口。
13. 对一个 execution unit 的 freeze/resume/kill 不会影响同机其他 Agent，
    页面不提供主机级“全部冻结”入口。

## 20. P5 智能体会话检测

当前已实现的是 Agent Native Hook 的真实会话开始/结束边界和工具调用事件展示，
支持 Codex、Claude Code、OpenClaw、Hermes、Zcode；这不等于已经采集完整 user/assistant
会话正文。以下完整会话、正文 reveal/export、AI 语义标记仍属于 P5 后续实现，不得
覆盖当前“工具命中按真实 session 过滤”的前端契约。

P5 新增：

```text
/detection/agent-guard/sessions
```

外层是服务端分页会话列表，一行一个 Codex、Claude Code 或 OpenCode source
session，展示 Agent、主机/UID、项目摘要、消息/工具计数、采集完整性、AI verdict
和风险标记；外层禁止出现 prompt、assistant、tool args/result 正文。

点击会话打开 80% 详情抽屉，固定三个 Tab：

1. 完整会话：user/assistant/tool/permission/compact/subagent/lifecycle 时序。
2. AI 语义分析：verdict、category、证据、反证、不确定性和人工 marking 处置。
3. 关联行为：tool call 与 PID、文件、网络、提权、逃逸、Finding 的证据链。

新增目录建议：

```text
frontend/src/api/agentSessionDetection.ts
frontend/src/types/agentSessionDetection.ts
frontend/src/store/agentSessionDetection.ts
frontend/src/views/detection/AgentSessionDetection/**
frontend/src/i18n/locales/{zh-CN,en-US}/agentSessionDetection.ts
```

完整字段、交互、权限、虚拟列表、reveal/export 和测试以
[agent_session_detection_frontend_prd_v6.2.md](agent_session_detection_frontend_prd_v6.2.md)
为准。
