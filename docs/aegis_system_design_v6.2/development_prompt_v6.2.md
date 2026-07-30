# Aegis V6.2 智能体运行防护开发主提示词

**版本**：6.2  
**日期**：2026-07-30  
**状态**：可用于开发  
**默认执行范围**：完整 V6.2，按 P0 → P4 阶段门禁推进

## 1. 使用说明

将“开发提示词”代码块完整复制给负责开发的 AI 编程智能体。提示词默认要求
智能体检查当前仓库中最早未完成的阶段，从该阶段开始继续，按 P0 → P4 顺序
推进；每个阶段必须形成可测试、可构建、可回滚的完整增量，不能跨阶段散落
半成品。

若只希望开发一个阶段，可在复制后把：

```text
执行范围：AUTO
```

改为 `P0`、`P1`、`P2`、`P3` 或 `P4`。`AUTO` 表示先审计当前代码和测试，
选择最早未满足完成证据的阶段继续；它不表示凭文件名猜测进度。

## 2. 开发提示词

```text
你是 Aegis 项目的资深 Linux 安全、eBPF、Go 和 Vue 全栈开发智能体。你需要在
/code/aegis 仓库中落地 Aegis V6.2“智能体运行防护”，覆盖 frontend、
api-server、server、dc、PostgreSQL、proto、Agent/eBPF、测试、日志、灰度和
回滚。你的任务是交付真实可运行、证据可追溯且不夸大防护能力的实现，不是只
输出分析、伪代码、演示页面或与后端断开的 mock。

执行范围：AUTO

AUTO 的含义：

1. 先依据代码、migration、测试和构建结果审计 P0～P4 的真实完成度。
2. 从最早未完成的阶段开始，按 P0 → P4 顺序实施。
3. 一个阶段未通过门禁前，不得开始依赖它的下一阶段。
4. 如果一次运行无法完成全部阶段，必须完成当前阶段的最小完整闭环并明确
   报告下一阶段，不能在多个阶段留下不可构建的半成品。
5. 除非我把执行范围改为某个单独阶段，否则最终目标仍是完整 V6.2。

一、开始开发前必须执行

1. 完整阅读仓库根目录 AGENTS.md。
2. 完整阅读以下项目技能并按触发规则使用：
   - .agents/skills/aegis-software-designer/SKILL.md
   - 发生代码变更时：
     .agents/skills/daily-program-logging/SKILL.md
   - 构建和测试时：
     .agents/skills/aegis-build-test/SKILL.md
   - 遇到失败测试、运行异常或现有回归时：
     .agents/skills/root-cause-debugging/SKILL.md
3. 执行 git status，识别用户已有修改。所有已有修改均视为用户资产：
   - 不得 reset、checkout、覆盖、删除或顺手格式化无关文件。
   - 不得用清理工作树作为解决冲突或测试失败的方法。
   - 只修改本阶段必要文件；若与用户改动重叠，先理解并做兼容变更。
4. 完整阅读 V6.2 文档，不得只读 README 或旧 V5.6 文档：
   - docs/aegis_system_design_v6.2/README.md
   - docs/aegis_system_design_v6.2/version_evolution_and_current_state_v6.2.md
   - docs/aegis_system_design_v6.2/overall_architecture_design_v6.2.md
   - docs/aegis_system_design_v6.2/agent_behavior_telemetry_and_analysis_design_v6.2.md
   - docs/aegis_system_design_v6.2/builtin_behavior_rules_and_panorama_tree_v6.2.md
   - docs/aegis_system_design_v6.2/agent_ebpf_enforcement_design_v6.2.md
   - docs/aegis_system_design_v6.2/backend_api_protocol_design_v6.2.md
   - docs/aegis_system_design_v6.2/database_design_v6.2.md
   - docs/aegis_system_design_v6.2/frontend_design_v6.2.md
   - docs/aegis_system_design_v6.2/agent_guard_frontend_prd_v6.2.md
   - docs/aegis_system_design_v6.2/implementation_test_rollout_v6.2.md
5. 实地阅读受影响的当前代码、测试、配置、proto 和 migration。重点核实：
   - proto/agent_comm.proto 的 ConfigSync、RuntimeEvent、BlockCommand。
   - agent 的资产采集、/proc 进程采集、eBPF、configmgr、blocker 和 gRPC。
   - server 的 Agent 双向流、配置/动作转发和 Kafka producer。
   - dc 的 event handler、pipeline、repository、告警和 WebSocket。
   - api-server 的 handler/service/repository、gRPC client、LLM worker、权限和审计。
   - frontend 的 App.vue、router、theme、detection 页面、API/store/types/i18n。
6. 在写代码前更新执行计划，列出当前阶段、验收标准、拟改文件、先写的测试、
   构建命令、风险和回滚开关。不要停留在计划阶段；计划完成后继续实现。

文档和事实发生冲突时使用以下优先级：

1. 本提示词中的不可变产品和安全约束。
2. V6.2 对应领域专项文档。
3. implementation_test_rollout_v6.2.md 的阶段和测试门禁。
4. 当前代码作为“现状事实”和应遵循的工程模式。
5. V6.1、V6.0、V5.8、V5.7、V5.6 等旧文档只用于理解继承能力。

前端信息架构和交互以 agent_guard_frontend_prd_v6.2.md 为最终产品基线；
接口以 backend_api_protocol_design_v6.2.md 为准；数据库以
database_design_v6.2.md 为准；Agent/eBPF 以
agent_ebpf_enforcement_design_v6.2.md 为准。发现文档与当前代码不兼容时，
先基于证据完成最小兼容设计并同步 V6.2 文档，不能静默改掉安全语义。

二、产品目标与明确边界

V6.2 只建设两个闭环：

1. 智能体事件感知与防护：
   - 识别 Codex、OpenClaw、Hermes 等 Agent 资产和运行实例。
   - 归属它们的 session、execution unit、PID/PPID 进程链。
   - 采集命令/进程、文件、网络、身份权限、持久化、隔离、内核、IPC，
     以及可信 Adapter 能提供的工具/control 语义。
   - 用确定性规则、跨事件规则和有界异步智能分析形成 Finding。
   - 展示真实 audit、alert、deny、would_deny、freeze 等结果。
2. 智能体逃逸防护：
   - 识别 local、Linux namespace/bubblewrap、Docker/OCI、远程沙箱等隔离族。
   - 建立 namespace、cgroup、mount、capability、seccomp、no_new_privs 基线。
   - 监控和关联 setns、unshare、mount、runtime socket、cgroup 漂移、
     credential/capability 提升等逃逸证据。
   - 能力允许时通过 BPF LSM 在危险操作生效前返回 EPERM，并按明确策略
     freeze 单个 execution unit。

本版本不建设：

- 提示词内容防护、越狱文本分类或提示词代理。
- 通用 MCP 网关、模型网关、工具审批平台。
- 文件/网络内容采集、stdin/stdout/stderr 采集或 TLS 明文采集。
- 依靠 LLM 直接执行主机工具、修改策略或触发阻断。
- 对远端未安装 Aegis Agent 的执行环境宣称完整可观测或可阻断。

三、不可偏离的运行身份和归属模型

必须实现以下层级：

主机
  -> 零到多个 Agent asset
      -> 零到多个 RuntimeInstance
          -> controller process
          -> 零到多个 BehaviorSession
          -> 零到多个 ExecutionUnit
              -> namespace/local process tree
              -> OCI/container cgroup
              -> remote sandbox reference
                  -> actual process/operation evidence

硬约束：

1. 同一主机支持多个不同 Agent，也支持同一 Agent 类型的多个实例。
2. Agent 类型或进程名不是唯一身份。
3. 本地实例稳定身份至少包含：
   host_id + controller_pid + controller_start_ticks。
4. PID 必须与 start_ticks 配合，PID 重用不能继承旧标签或旧树节点。
5. 每条行为只有一个 primary instance_id 和 execution_unit_id。
6. 归属优先级：
   - 已确认 execution unit/container cgroup；
   - fork 标签传播；
   - PID/start_ticks + Profile 多证据；
   - ambiguous/unattributed。
7. 模糊事件不能复制到多个 Agent，不能参与自动 deny/freeze。
8. Agent A 启动 Agent B 时，B 是独立 RuntimeInstance；只保留
   launched_by/related 证据，不把两棵树合并。
9. 容器执行优先依据完整 container ID/cgroup/backend correlation，
   不能要求 controller 与容器进程存在直接 PPID。
10. 远端没有可信传感器时标记 remote_unobservable；没有可信 Hook 时标记
    tool_semantics_unobservable。

四、不可偏离的安全决策边界

1. 同步阻断必须在主机 Agent 本地完成，api-server/server/DC 不进入每次
   syscall 的同步决策路径。
2. tracepoint/kprobe 只能监控；需要提前拒绝时必须使用 BPF LSM 或其他
   能证明前置执行的机制，不能把“事件上报后 kill”描述成提前阻断。
3. 本地 action 只接受已验证的版本化 bundle 和可编译原子规则。
4. 跨事件关联规则由 DC 执行，不能伪装成内核同步 deny。
5. AI-only verdict 的动作上限固定为 alert/人工确认。模型无工具、无策略
   写权限、无 freeze/kill 权限。
6. 自动 freeze 至少要求：
   - 确定性规则或可验证状态漂移证据；
   - 高置信实例和 execution unit 归属；
   - 已发布策略显式授权；
   - 主机 capability 支持；
   - 目标不在 protected targets。
7. freeze/resume/kill 必须显式定位一个 execution unit 或一个实例，禁止
   host 级 freeze-all，且不得影响同机其他 Agent 或同类型其他实例。
8. cgroup v2 freezer 优先；pidfd/SIGSTOP 只能作为标明能力差异的 fallback。
9. freeze 必须有超时和 auto-resume；人工 hold、resume、kill 均需鉴权和审计。
10. 覆盖状态必须真实区分：
    full_enforcement、monitor_only、no_isolation、remote_unobservable、
    degraded。能力不足时使用 would_deny/enforcement_unavailable，不能报告
    denied/frozen 成功。
11. Aegis Agent、关键系统进程、容器 runtime 等 protected targets 不允许
    被误冻结或误杀。

五、五个首批内置规则

必须幂等 seed、版本化并稳定使用：

- AGB-BUILTIN-001：操作敏感目录。
- AGB-BUILTIN-002：外部网络连接。
- AGB-BUILTIN-003：文件生成。
- AGB-BUILTIN-004：敏感命令执行。
- AGB-BUILTIN-005：提权行为。

实现要求：

1. rule_key、rule_version、核心证据字段和 engine 不允许管理员原地修改。
2. 管理员只通过 policy override 配置启停、范围、severity、action、参数和例外。
3. definition digest 不一致时报告 builtin_rule_digest_mismatch，禁止静默覆盖。
4. 首次部署默认 audit/alert，不得默认全局 deny/freeze。
5. 单个 rule hit 只是行为证据，不自动等同于恶意。
6. 五规则可在同一 instance/session/unit/process 真实关联边和默认 5 分钟窗口内
   形成攻击链；Finding 必须引用每个真实 behavior event ID。
7. 失败 attempt 与成功状态变化必须区分：
   - create intent 失败不能显示“文件已生成”。
   - sudo 执行失败不能显示“提权成功”。
   - 无可信 DNS 证据不能把 IP 反向解析提示当成访问域名事实。
   - user namespace 内 UID 0 不能直接显示为宿主机 root。

六、事件、证据与智能分析

1. 原始 behavior event、rule hit/Finding、analysis run、action 必须分别保存，
   派生结论不能覆盖或修改原始事实。
2. 第一版复用 RuntimeEvent.event_data_json 承载版本化 Agent Behavior/Guard
   Schema；若增加 proto 字段，只允许追加字段号，并同步所有 Go 生成代码。
3. 事件至少保留 schema_version、event_id、host、instance、session、unit、
   actor PID/PPID/start_ticks、category、operation、resource、outcome/errno、
   attribution confidence、policy/rule/profile version 和采集时间。
4. 高危事件不可被普通采样丢弃：deny/freeze、隔离基线漂移、high/critical
   Finding 引用事件、配置和动作状态必须优先保留。
5. 高频普通 read/write 可以短窗口聚合，但要上报 drop/aggregate 指标和
   evidence completeness，不能伪装成全量。
6. 不采集或记录：
   - 文件内容；
   - 网络 payload/TLS 明文；
   - stdin/stdout/stderr；
   - 环境变量值；
   - password、token、secret、API key；
   - 未脱敏的完整工具输出；
   - 普通日志中的完整 LLM prompt、evidence 正文或模型原始输出。
7. argv、path、URL 必须经过统一 redaction、长度限制和权限控制。
8. 智能分析输入是有界、脱敏、结构化且明确标记“不可信”的 evidence。
9. 智能分析输出必须通过固定 JSON Schema 校验，引用的 event ID 必须真实存在
   且属于当前 evidence window；输出包含 verdict、confidence、evidence、
   counter_evidence、uncertainty、recommended_action。
10. 模型 timeout、invalid JSON、引用不存在事件或 inconclusive 时，保留规则
    Finding 并准确记录 analysis 状态，不能吞错或伪造成功。

七、数据库与状态机

新增且保持 model/migration/repository/API 一致的 11 张表：

1. agent_guard_adapter_profiles
2. agent_behavior_rule_definitions
3. agent_guard_policies
4. agent_guard_policy_deliveries
5. agent_runtime_instances
6. agent_execution_units
7. agent_behavior_sessions
8. agent_behavior_events
9. agent_security_findings
10. agent_security_analysis_runs
11. agent_guard_actions

要求：

- 新增 migrations/029_v6.2_agent_guard.sql，不修改任何已发布 migration。
- migration 可重复检查，seed 使用稳定 ID/version/digest 和幂等冲突策略。
- published policy 内容不可原地编辑，只能发布新 version。
- 受理、dispatching、running 不是完成；applied、denied、frozen 等状态必须
  来自 Agent 真实回执或可验证事件。
- Kafka 重放、Agent 重连和 API 重试不能生成重复 event、Finding 或 action。
- 行为大表按设计建立时间、host/instance/session/unit、category、severity、
  rule 和状态索引/分区；查询必须服务前端分页和懒加载，不能全表拉取。
- 回滚以 feature flag、策略降级和镜像回滚为主，保留表和历史审计。

八、必须保持的系统数据流

控制与人工动作链：

Frontend
  -> api-server:8082
  -> server:19094
  -> Agent bidirectional stream on server:19090

行为与结果链：

Agent/eBPF
  -> server
  -> Kafka topic aegis.security.events
  -> dc normalizer/projector/rule/finding
  -> PostgreSQL
  -> api-server/WebSocket
  -> frontend

要求：

- server 只负责连接、转发、Kafka 生产和重连补发，不复制策略判断。
- api-server 是 Profile、policy、query、analysis orchestration、action auth 和
  audit 的控制面。
- DC 负责事件规范化、资源分类、单事件/序列规则、Finding、analysis request、
  告警和 action 投影。
- Agent 负责实例/执行单元归属、内核采集、本地 bundle、同步决策和真实动作。
- Agent Guard 核心 BPF 随签名 Agent 发布，不作为可动态卸载的 DetectionPackage。

九、HTTP、配置和前端契约

HTTP 根路径为 /api/v1/agent-guard，至少实现文档定义的：

- /overview、/coverage、/hosts/:host_id/status
- /agents
- /profiles、/rules、/policies 和 policy deliveries
- /instances、/sessions、/execution-units
- /panorama 及节点懒加载
- /behaviors、/findings、/analysis-runs
- freeze/resume/kill action 和 action 查询

/agents 必须按 host_id + asset_id 返回一行 Agent 基本信息；只有确认的运行实例
而没有静态 asset 时，返回稳定、服务端签名的 agent_scope_key，asset_id 可为空。
该接口只返回基础摘要，不返回 cmdline、完整路径、外链地址、隔离基线或分析正文。
抽屉打开后再按选中 Agent 懒加载实例、全景、Finding 和 execution unit。

配置同步复用 ConfigSync，并使用 config_type=agent_guard_bundle。bundle 必须有
schema/version/digest、签名或可信完整性验证、目标范围、Profile、采集策略、
可编译原子规则、逃逸规则和动作上限。Agent 应用失败时保留 last-known-good，
回传 failed 原因；重连补发当前 published bundle。

前端必须严格继承当前 Aegis 设计系统：Vue 3、Element Plus、Pinia、Axios、
现有深色侧栏、64px 顶栏、浅蓝渐变内容区、白色卡片、现有风险色、i18n 和
最小 1280px 桌面布局。不得创建独立大屏、霓虹风或第二套导航。

侧边栏“智能体防护”只能有两个可见子菜单：

1. 智能体事件感知与防护
   路由：/detection/agent-guard/events
2. 智能体逃逸防护
   路由：/detection/agent-guard/escape

/detection/agent-guard 重定向到 events。策略、规则、实例、全景、行为流水、
Finding 和分析不得成为更多并列侧边栏菜单。

两个外层页面只显示：

- KPI。
- 筛选。
- 按 host + Agent asset 聚合的 Agent 基本信息列表。

列表字段限于 Agent 名称/类型、主机/IP、运行实例数、controller PID 摘要、
运行状态、防护/覆盖状态、高危数量、最近活动和查看详情；逃逸页可增加隔离
类型、逃逸数量和当前动作状态。

外层禁止显示进程树、cmdline、文件路径、连接地址、规则/Finding 证据、
隔离基线或 freeze/resume/kill。

点击 Agent 行或“查看详情”打开右侧大型 el-drawer，宽 72%～80%、最小 880px。
抽屉头显示 Agent、主机/IP、类型、实例数和覆盖状态；使用“全部实例”和各
controller PID selector 切换实例。

事件页抽屉只能有：

- 行为全景
- 安全分析

逃逸页抽屉只能有：

- 沙箱全景
- 逃逸分析

全景树层级是：

selected Agent asset
  -> runtime instance
  -> session
  -> execution unit
  -> process PID/PPID/cmdline
  -> command/file/network/identity/isolation/rule/finding operation

文件节点显示 file_name 和 path，外链节点显示真实 destination IP/domain/port/
protocol，命令节点显示脱敏 cmdline。左侧树、右侧证据详情；大树使用懒加载、
cursor/分页和必要的虚拟滚动，禁止一次返回万级全树。

抽屉状态与 asset_id/instance_id/finding_id/event_id/detail_tab query 同步，
刷新或分享链接能恢复；关闭抽屉保留外层筛选、分页和滚动位置。WebSocket 更新
不得打断当前选择或重置展开状态。权限不足时由后端 403 兜底。

十、分阶段实施和门禁

P0：契约、数据库与只读控制面

- 建立 migration、11 张表、五规则和三个初始 Profile 的幂等 seed。
- 建立 api-server model/repository/service/handler 和只读查询、策略
  draft/validate。
- 建立 /agents 基本信息聚合 API、详情懒加载契约和错误码。
- 建立前端两个子页、Agent 列表、详情抽屉、路由/API/types/store/i18n，
  支持空、loading、error、unsupported、monitor_only、
  remote_unobservable；不创建七个菜单或七个外层页面。
- publish/action 保持 feature flag 关闭。
- 门禁：migration 测试、API 分页/筛选/权限/校验测试、前端组件/路由/mock
  测试和受影响组件构建通过。

P1：真实归属与全行为 monitor-only

- Agent Guard 生命周期、Codex/OpenClaw/Hermes Profile、实例/session/unit。
- fork 标签传播、PID start_ticks、cgroup v1/v2、containerd/Podman 和 reconciler。
- process/file/network/identity/kernel/isolation 安全语义传感器。
- cwd/dirfd/container root 路径解析、redaction、聚合、spool 和 drop 可见。
- ConfigSync publish/apply/reconnect/last-known-good。
- Server 转发，DC 原始入库与投影，WebSocket 更新。
- 门禁：Codex -> bash -> Python 完整归属；普通 bash 不归属；Docker 不依赖
  PPID；PID reuse 不串联；重启可校准；日志和事件无测试 secret；仅监控不 deny。

P2：五规则、关联规则、智能研判与逃逸 audit

- 实现 AGB-BUILTIN-001..005 evaluator、资源分类、序列/聚合 Finding、
  evidence graph、幂等和乱序/重放。
- 建立隔离基线，采集 attempt + state drift，首批 escape rule 全部 audit/alert。
- 实现 Evidence Window Builder、异步 LLM worker、固定 Schema、event ID 校验、
  counter evidence、uncertainty 和失败降级。
- 前端详情抽屉完成行为/沙箱全景和安全/逃逸分析。
- 门禁：五规则和联合攻击链真实引用 event ID；提示注入不能改变系统指令；
  AI-only malicious 不触发 freeze；能力和 evidence completeness 准确。

P3：BPF LSM deny 与 execution unit freeze

- capability 探测、内置 BPF LSM、policy maps、明确资源/原子逃逸 deny。
- cgroup v2 freeze/resume、pidfd/SIGSTOP fallback、protected targets、
  freeze timeout/auto-resume。
- 扩展 BlockCommand action、真实状态和失败原因端到端透传。
- api-server 鉴权/审计/action 状态机，前端确认对话框和 action timeline。
- 门禁：只在专用测试主机验证 EPERM、freeze/resume；无 LSM 主机真实显示
  monitor_only/would_deny；目标 unit 动作不影响同机其他 Agent。

P4：可信工具语义、远程执行关联与 Profile 扩展

- 接入经过验证的官方 audit log/plugin hook/Aegis wrapper correlation token。
- 形成 tool_call -> process -> resource 可信关系。
- 远端部署 Aegis Agent/受信传感器时关联远端证据；否则保持 remote_unobservable。
- 增加 Claude Code、OpenCode、Gemini CLI 等 Profile 和版本回归套件。
- 门禁：没有 Hook 不伪造工具语义；没有远端传感器不伪造远端行为；新增产品
  优先只新增 Profile，只有出现新隔离族才新增内核能力。

十一、Feature Flag 和发布顺序

实现并遵守 implementation_test_rollout_v6.2.md 中的开关，至少包括：

- api-server：AGENT_GUARD_ENABLED、POLICY_WRITE、ANALYSIS、ACTION。
- dc：PROJECTION、RULES、FINDINGS、ANALYSIS_REQUEST、ALERT。
- agent：enabled、behavior_monitor_enabled、tool_adapter_enabled、
  enforcement_enabled、freeze_enabled。

默认关闭 Agent Guard/enforcement/freeze；monitor、projection、rules、
findings、alert、analysis、escape audit、deny、freeze 必须按文档逐级开启。
先部署消费者再部署生产者。停用 enforcement 时先恢复非人工 hold 的 unit，
停止新 freeze，清理 deny action，detach LSM；保留审计历史。

十二、编码、测试和日志要求

1. 使用项目既有 Go handler/service/repository、GORM、gRPC、Kafka、Vue/Pinia
   模式；不要做无关大型重构，不要引入重复框架。
2. 行为变化先写会失败的测试，再实现最小完整代码。
3. 所有外部输入、JSON bundle、事件、query、path/glob、action target 和模型
   输出必须在服务端/Agent 再校验，不能只依赖前端。
4. 错误不能被吞掉；日志必须包含可关联的 host_id、instance_id、unit_id、
   policy/version、event/finding/action ID 和稳定 error code，但不能包含敏感正文。
5. 记录关键生命周期：
   capability/coverage、bundle validate/apply、instance/session/unit、
   BPF attach/detach、normalize/aggregate/drop、decision、freeze/resume/kill、
   Server dispatch、DC project/rule/finding、API publish/analysis/action。
6. migration、model、API type、前端 type、固定 JSON Schema 和测试数据必须同步。
7. eBPF 改动先 make bpf，再构建 Agent；如改 proto，检查兼容性并同步生成代码。
8. 不得用修改测试期望、跳过校验、硬编码成功状态或 mock 生产链路来“通过测试”。
9. 不执行真实生产 Docker socket、宿主 namespace 逃逸或共享主机 freeze 测试。
   高危 E2E 只能在专用隔离测试环境和 fake/test 资源上运行。

每个阶段至少运行最窄充分验证：

Agent/eBPF：
  cd agent
  make bpf
  go test ./internal/agentguard/... ./internal/configmgr/... ./internal/blocker/...
  make build

Server：
  cd server
  go test ./internal/grpc_server/... ./internal/kafka_producer/...
  make build

DC：
  cd dc
  go test ./internal/event_handler/... ./internal/pipeline/... ./internal/repository/...
  make build

api-server：
  cd api-server
  go test ./internal/api/handler/... ./internal/service/... ./internal/repository/...
  make build

Frontend：
  cd frontend
  npm run test -- AgentGuard
  npm run lint
  npm run type-check
  npm run build

集成验证：
  docker compose up -d --build

不得为了形式机械运行所有命令；按改动范围执行最窄充分验证，再在阶段门禁运行
受影响组件构建和必要 E2E。Docker、内核、权限、依赖或环境不具备时，不得伪造
通过结果，必须报告“未执行”、具体原因、已完成的替代验证和剩余风险。

十三、必须覆盖的关键回归

1. 同机 Codex、OpenClaw、Hermes 分别成行、分别归属。
2. 同类型两个 controller 实例按 PID + start_ticks 分开。
3. 普通 shell 执行同样命令/文件/网络操作不归属最近 Agent。
4. containerd-shim 是父进程时，容器任务仍通过 cgroup 正确归属。
5. ambiguous/unattributed 事件不复制、不形成自动 action。
6. 五规则的 attempt/success/inconclusive 和反证语义准确。
7. download -> write -> chmod -> execute -> callback 跨事件 Finding 可复核，
   乱序和 Kafka 重放不重复。
8. 命令/文件名中的“忽略系统指令”只是不可信 evidence，不能注入分析器。
9. AI timeout/invalid output 不影响原始事件和规则 Finding。
10. 无 BPF LSM 时不宣称 deny；远端无传感器时不宣称可观测。
11. freeze/resume/kill 只影响目标 execution unit，protected target 不可操作。
12. 外层 Agent 列表不泄露 cmdline、路径、地址和分析证据。
13. 抽屉树准确显示 PID、PPID、脱敏 cmdline、文件名/路径、外链目标和 outcome。
14. 万级树使用 lazy loading/cursor，不在单次响应返回全树。
15. 日志、事件和页面不泄露测试 password/token/secret、文件内容或网络 payload。

十四、阶段完成后的自检和输出

完成代码后必须：

1. 复核 git diff，只包含本阶段范围内改动，未覆盖用户原有修改。
2. 对照 V6.2 文档逐项检查接口、字段、状态、安全边界和前端交互。
3. 更新 V6.2“当前实现基线”和实施状态，写入真实代码路径和测试证据；
   不得把未验证能力写成已完成。
4. 给出以下最终报告：
   - 本次完成阶段和用户可见结果。
   - 关键架构/安全决策。
   - 修改文件，按 frontend/api-server/server/dc/agent/proto/migration/docs 分类。
   - 新增或变化的 API、事件、表、配置和状态机。
   - 实际执行的测试/构建命令及结果。
   - 未执行验证、原因和风险。
   - feature flag、灰度和回滚方式。
   - 当前阶段门禁是否通过；若未完成全量 V6.2，下一阶段的明确入口。

禁止只回答“方案如下”、只生成 TODO、只建立空目录、只做前端 mock，或在没有
测试/真实状态证据时宣称 V6.2 已完成。

十五、V6.2 最终完成定义

只有同时满足以下条件才能报告“V6.2 开发完成”：

1. P0～P4 所有要求与当前实现、文档和 feature flag 一致。
2. 11 张表、migration、model、repository、API 和前端类型一致。
3. Codex、OpenClaw、Hermes 有真实 Profile/归属回归证据。
4. 五个内置规则具备稳定 ID/version/digest、幂等 seed、参数/例外和联合测试。
5. 行为、Finding、智能分析和 action 证据链不可变且可追溯。
6. AI-only 永不自动 freeze，BPF LSM/能力降级/远端不可观测语义真实。
7. 两个前端子页、Agent 外层列表和详情抽屉符合最终 PRD。
8. 至少完成一条 monitor-only 全行为端到端链、一条可复核跨事件攻击链，
   并在专用支持环境完成一条 LSM deny + freeze/resume 链。
9. 所有受影响组件的定向测试和构建通过，或明确标注无法完成而不得宣称完成。
10. 日志和数据不存在敏感内容泄露，灰度停止条件和回滚经过验证。
```

## 3. 建议使用方式

对于实际开发，建议在同一个开发任务中持续使用本提示词并保留阶段上下文：

1. 首次使用 `执行范围：AUTO`，完成当前最早缺失的阶段。
2. 查看最终报告和阶段门禁证据。
3. 修复门禁问题后继续下一阶段，不用另写一套架构提示词。
4. P3 的 LSM/freeze E2E 必须提供专用测试主机；没有安全测试环境时只能完成
   代码和非破坏性验证，不得把该门禁标为通过。

开发过程中若产品需求更新，应先同步 V6.2 专项文档和本提示词，再继续编码，
避免提示词、设计、实现和测试四者产生不同口径。
