# Aegis V6.2 智能体运行防护设计文档

**版本**：6.2  
**日期**：2026-07-30  
**状态**：设计完成，待实施  
**主题**：AI Agent 全行为采集、操作链可视化、安全分析与隔离逃逸监控/阻断

## 1. 版本定位

V6.2 在 V6.1 当前实现基础上新增“智能体运行防护”能力，形成三个相互关联的闭环：

1. 识别 Codex、OpenClaw、Hermes 等 AI Agent 的运行实例、session、执行单元和实际执行进程，采集命令/进程、文件、网络、身份权限、持久化、内核与隔离控制等操作。
2. 将离散操作关联为进程链、时间线和 PID 主干行为全景树，通过确定性规则、跨事件行为规则与 Aegis 智能分析器联合判断行为是否具有攻击性。
3. 识别 AI Agent 实际采用的本地进程、Linux namespace、OCI 容器或远程沙箱隔离方式，监控隔离边界变化和逃逸行为；在本机能力允许时通过 BPF LSM 提前拒绝，并按策略暂停对应执行单元。

敏感文件访问只是文件行为域的一类高风险证据，不再是 V6.2 的能力边界。V6.2 不建设提示词防护、越狱文本识别、MCP 网关、模型代理或通用工具审批模块；可选产品 Adapter 只能补充可信工具调用语义，底层事实仍以操作系统行为为准。

## 2. 核心对象

V6.2 使用以下分层运行模型，避免把宿主机控制进程错误地当成沙箱进程：

```text
主机
  -> 零到多个 Agent 资产（Codex/OpenClaw/Hermes）
      -> 零到多个 Agent 运行实例（controller PID + start_ticks）
          -> 控制进程（通常在宿主机）
          -> 一个或多个执行单元
              -> 本地进程树
              -> bubblewrap namespace
              -> Docker/OCI container cgroup
              -> 远程 SSH/Modal/Daytona/OpenShell 沙箱
                  -> 实际执行进程
```

- 同一主机支持多种 Agent 并存，也支持同一种 Agent 同时存在多个运行实例。
- 服务端以主机作为归属和筛选范围，同机各 Agent/实例保持独立；前端点击
  Agent 后，以选中的 Agent asset 为抽屉根节点展示其实例和进程树。
- 行为采集覆盖控制进程和所有本机可观测执行单元。
- 隔离逃逸判断只针对声明了隔离边界的执行单元。
- 远程执行节点未部署 Aegis Agent 时必须显示 `remote_unobservable`，不能宣称已防护。
- 工具调用只有在 Agent 提供可信日志、Hook 或 correlation token 时才能关联；没有 Hook 不影响 OS 行为采集，但必须显示 `tool_semantics_unobservable`。

## 3. 文档索引

| 文档 | 内容 |
| --- | --- |
| [version_evolution_and_current_state_v6.2.md](version_evolution_and_current_state_v6.2.md) | V5.0 至 V6.1 演进、当前代码事实、V6.2 继承/替换关系 |
| [overall_architecture_design_v6.2.md](overall_architecture_design_v6.2.md) | 总方案、威胁模型、总体架构、三条业务链路、组件职责和安全边界 |
| [agent_behavior_telemetry_and_analysis_design_v6.2.md](agent_behavior_telemetry_and_analysis_design_v6.2.md) | 全行为域、统一事件模型、操作链、规则关联、智能研判和动作边界 |
| [builtin_behavior_rules_and_panorama_tree_v6.2.md](builtin_behavior_rules_and_panorama_tree_v6.2.md) | 五个首批内置规则、联合攻击链、规则页面和 PID 主干行为全景树 |
| [agent_ebpf_enforcement_design_v6.2.md](agent_ebpf_enforcement_design_v6.2.md) | Agent 运行实例识别、进程/cgroup 归属、行为传感器、逃逸检测、BPF LSM 和暂停机制 |
| [backend_api_protocol_design_v6.2.md](backend_api_protocol_design_v6.2.md) | api-server、server、dc、规则/智能分析、Kafka、gRPC、HTTP API、配置和事件契约 |
| [database_design_v6.2.md](database_design_v6.2.md) | 数据表、字段、索引、状态机、数据保留和迁移策略 |
| [frontend_design_v6.2.md](frontend_design_v6.2.md) | 前端路由、页面、交互、类型、实时状态和测试 |
| [agent_guard_frontend_prd_v6.2.md](agent_guard_frontend_prd_v6.2.md) | 基于当前 Aegis 前端实地盘点的“双子页 Agent 列表 + 详情抽屉”PRD、V4 原型、字段、交互和验收 |
| [implementation_test_rollout_v6.2.md](implementation_test_rollout_v6.2.md) | 文件级实施清单、分阶段开发、测试矩阵、灰度、回滚和完成定义 |
| [development_prompt_v6.2.md](development_prompt_v6.2.md) | 可直接交给编码智能体使用的 V6.2 全栈开发主提示词、阶段门禁和交付格式 |

## 4. 核心设计决策

| 决策 | 结论 |
| --- | --- |
| 是否按每个产品开发一套 eBPF | 否。内核能力按四类隔离族实现，产品差异通过版本化 Adapter Profile 描述 |
| 是否仅依赖 PPID 识别子进程 | 否。本地进程使用 fork 标签传播；容器使用 container/cgroup；远程执行使用远端传感器 |
| 服务端是否参与实时阻断 | 不参与同步决策。服务端负责策略、审计和人工处置；阻断必须在主机 Agent 本地完成 |
| 是否采集全量 syscall | 否。采集具有安全语义的行为事件；高频 read/write 做短窗口聚合，不能以不可控数据量换取“全量”名义 |
| 攻击性如何判断 | 本地确定性规则 + DC 单事件/序列规则 + 异步智能研判；原始事实和安全结论分开保存 |
| 智能分析能否单独自动阻断 | 默认不能。AI-only 结论只告警/待确认；自动阻断需要确定性规则证据和显式策略授权 |
| 是否能看到 Agent 工具调用 | 有可信 Adapter Hook 时关联 tool call；否则只展示可证明的 OS 行为并标记工具语义不可观测 |
| 首批是否提供开箱即用规则 | 是。内置敏感目录、外链、文件生成、敏感命令和提权五个版本化规则族 |
| 全景如何展示 | 外层只展示按 host + Agent asset 聚合的基本信息列表；点击 Agent 后在详情抽屉中按 instance → session → execution unit → PID 展示全景和分析 |
| tracepoint 是否能作为强阻断 | 不能。tracepoint 用于兼容监控；真正的提前拒绝使用 BPF LSM |
| 所有内核是否宣称完整阻断 | 否。按 `full_enforcement`、`monitor_only`、`no_isolation`、`remote_unobservable` 展示真实覆盖能力 |
| 核心防护是否使用动态 DetectionPackage | 否。Agent Guard 属于安全关键内置模块，随 Agent 签名发布；动态配置只描述产品特征和策略 |
| 是否复用现有事件流 | 是。复用 `RuntimeEvent.event_data_json`、Server Kafka 转发、DC 入库与告警链路 |
| 是否复用现有配置同步 | 是。新增 `ConfigSync.config_type=agent_guard_bundle`，保留 V5.7/V5.8 兼容语义 |
| 是否复用现有阻断命令 | 是。扩展 `BlockCommand.action`，支持执行单元冻结、恢复和终止 |

## 5. 首批支持范围

第一批内置 Profile：

- Codex Linux：控制进程 + bubblewrap/Linux namespace 执行单元。
- OpenClaw：sandbox off、本地执行、Docker 执行、SSH/OpenShell 远程执行。
- Hermes：local、Docker、Singularity、SSH、Modal、Daytona，以及 whole-process Docker/OpenShell。

后续新增 Claude Code、OpenCode、Gemini CLI 等 Agent 时，优先新增 Profile；只有出现新的操作系统隔离族时才新增 Agent 代码。

首期平台限定：

- Linux 主机。
- Aegis Agent 具备读取 `/proc`、加载 eBPF 和执行受控阻断所需权限。
- BPF LSM 不可用时自动降级为监控模式，不允许静默宣称阻断已启用。

## 6. 总体完成标准

V6.2 只有同时满足以下条件才算完成：

1. `codex -> shell -> python/node` 等多层子进程的命令、文件、网络、权限和隔离行为能关联到正确实例、session、执行单元和进程链。
2. 与 Agent 无关的普通进程不被归属到 Agent 行为流。
3. OpenClaw/Hermes Docker 进程通过 container/cgroup 归属，不依赖其与控制进程的 PPID 关系。
4. “下载 → 写入 → chmod → 执行 → 外连”等跨事件行为能形成单一 finding，并逐项引用原始事件。
5. Aegis 智能分析器输出攻击性、证据、反证和不确定性；AI-only 结论不能直接自动 freeze。
6. Codex namespace 执行单元发生未授权 `setns`、mount、namespace/cgroup 漂移时产生可复核证据。
7. 支持 BPF LSM 的主机可在明确危险操作生效前返回 `EPERM`；高危策略可冻结对应执行单元。
8. 不支持 BPF LSM、未启用沙箱、工具语义缺失或远程不可观测时，前端显示真实覆盖状态和原因。
9. 采集、规则、分析、finding、自动动作和人工恢复全部具备版本、来源、结果和时间链路。
10. 五个内置规则具备稳定 ID/version、默认参数、例外、灰度和联合关联测试。
11. 全景树直接展示 PID、PPID、cmdline、文件名/完整路径及外链 IP/domain/port。
12. 前端、api-server、server、dc、数据库、Agent/eBPF 均有定向测试并通过受影响组件构建。
13. 同一主机上的多种 Agent 和同类型多个运行实例能分别归属、展示和筛选；
    对一个 execution unit 的动作不会影响同机其他 Agent。
