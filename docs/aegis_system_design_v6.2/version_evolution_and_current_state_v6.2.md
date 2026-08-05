# Aegis V6.2 版本演进与当前实现基线

**版本**：6.2
**日期**：2026-08-06
**状态**：Agent Guard 工具事件/真实会话/运行时设置/只读规则目录实现基线；专用宿主机门禁和完整 P5 正文待验证

> 当前实现基线见 [current_implementation_baseline_2026-08-06.md](current_implementation_baseline_2026-08-06.md)。

## 1. 文档目的

本文明确 V6.2 不是基于单一旧版本重新设计。V6.2 以 V6.1 的文档和实际代码为直接基线，并向前追溯运行时检测、配置同步、资产采集、动态 eBPF、业务编排和阻断能力的来源。

设计判断优先级：

1. 当前仓库代码、迁移和协议。
2. V6.1 已实现或修复文档。
3. V6.0 总体架构及实施状态。
4. V5.8、V5.7、V5.6、V5.5 及更早版本中的仍然有效设计。

旧文档与当前代码冲突时，以当前代码为准，并在 V6.2 中显式记录差距。

## 2. 版本演进

| 版本 | 已形成能力 | V6.2 处理方式 |
| --- | --- | --- |
| V5.0/V5.2 | eBPF 运行时事件、Sigma 匹配、`RuntimeEvent`、告警与 `kill_process` 阻断 | 复用运行时事件和阻断主链路；不复用只按 MITRE 命中的策略模型 |
| V5.5 | frontend → api-server → server → Agent 与 Agent → server → Kafka → dc → PostgreSQL 主架构 | 完整复用 |
| V5.6 | Agent 只读工具、进程树、网络、打开文件、日志查询 | 复用进程取证能力；运行时归属改为持续标签传播，不依赖按需扫描 |
| V5.7 | `ConfigSync`、文件/网络 eBPF、内核能力降级、阻断原因透传 | 复用配置同步、文件/网络传感器和能力降级；采集由硬编码敏感路径升级为 Agent 全行为语义采集 |
| V5.8 | 智能资产采集、`host_application_assets`、AI Agent/MCP 分类、容器元数据、动态 eBPF 包与状态上报 | 复用 AI Agent 资产和 container ID；Agent Guard 核心 eBPF 不作为动态包加载 |
| V6.0 | 双模智能控制面、统一业务 Service/Repository、Agent 实时证据、审计和异步执行语义 | 复用控制面、LLM worker、认证、审计、WebSocket 和业务事实表；LLM 只做异步研判，不进入本地同步阻断 |
| V6.1 | 弱密码前后端/数据库/Agent 端到端模块、受控文件访问、容器资产路径、PID 去重、在线状态和真实证据约束 | 作为当前实现和工程风格基线；复用路径校验、任务状态、错误结构和前端状态表达 |

## 3. V6.1 当前实现约束

V6.1 没有单一“总架构文档”，当前架构由以下内容共同约束：

- 弱密码 Agent、api-server、数据库、前端端到端设计。
- Assistant 工具映射、工作流知识、确定性执行和真实完成证据设计。
- 软件资产采集、容器路径元数据、PID 去重、在线 Agent 状态等修复设计。
- V6.0 总体架构在 V6.1 中继续有效的部分。

V6.2 遵守以下 V6.1 规则：

1. Agent 负责主机侧采集和受控执行，api-server 负责业务策略与编排，server 只负责连接和命令转发。
2. 异步受理不等于完成；策略下发必须有 Agent 应用状态和版本证据。
3. 离线、超时、跳过、不可观测不能算成功。
4. 前端和后端使用相同状态语义，不以关闭连接或收到请求作为成功。
5. 日志和事件不得泄露密码、token、环境变量值、文件/网络内容或完整工具输出。
6. 所有最终结论可追溯到真实 Agent 行为、规则/模型版本、反证、不确定性和处置结果。

## 4. 当前代码事实

### 4.1 可直接复用

| 当前能力 | 实际位置 | V6.2 用途 |
| --- | --- | --- |
| AI Agent 配置资产识别 | `agent/internal/assets/ai_agent_collector.go` | 增加 Codex、OpenClaw、Hermes Profile 与运行特征 |
| 进程快照 | `agent/internal/assets/process_collector.go` | 启动时发现实例、周期校准、PID start time 和 container ID |
| fork/exec/exit eBPF | `agent/internal/ebpf/bpf/fork.bpf.c`、`execve.bpf.c`、`exit.bpf.c` | 命令/进程行为、进程成员标签传播和生命周期 |
| 文件/网络 eBPF | `agent/internal/ebpf/bpf/file.bpf.c` 及现有网络传感器 | 全行为兼容监控基础，需补读、网络关联和动态资源策略 |
| 配置同步 | `proto/agent_comm.proto`、`agent/internal/configmgr/configmgr.go` | 下发 `agent_guard_bundle` |
| 阻断器 | `agent/internal/blocker/` | 增加 freeze/resume/kill execution unit |
| 运行时事件 | `RuntimeEvent.event_data_json` | 承载 Agent Behavior/Guard 结构化事件 |
| Kafka/DC 链路 | server producer、`dc/internal/event_handler` | 入库、投影、告警和 WebSocket |
| LLM worker/真实证据原则 | `api-server/internal/llm`、`internal/assistant` 及 V6.0/V6.1 设计 | 异步 Security Analyst、结构化输出和证据引用校验 |
| AI Agent 资产表 | `host_application_assets` | 关联静态资产与运行实例；Native Hook 支持 Codex/Claude Code/OpenClaw/Hermes/Zcode |
| 前端 AI Agent 资产路由 | `/hosts/assets/ai-agents` | 从资产详情跳转到运行防护实例 |

### 4.2 必须补齐的差距

| 差距 | 当前表现 | V6.2 改造 |
| --- | --- | --- |
| Agent 类型不完整 | 资产 Profile 未覆盖 Codex、OpenClaw、Hermes | 版本化 Adapter Profile |
| 资产不等于运行实例 | 当前主要记录配置目录和周期快照 | 新增 runtime instance/execution unit |
| exec PPID 不完整 | `execve.bpf.c` 中 PPID 当前为 0 | 使用 fork 标签和 `/proc` 校准 |
| Docker 归属不可靠 | 当前只提取部分 `/docker/` cgroup 格式和短 ID | 兼容 cgroup v1/v2、systemd scope、containerd/Kubernetes |
| 行为采集过窄 | `file.bpf.c` 固定前缀且主要采集写意图，缺少统一 Agent 行为模型 | 命令/进程、文件、网络、身份、内核和隔离统一事件，路径/dirfd 解析与聚合 |
| 动态 loader 不支持 LSM attach | 当前支持 kprobe/kretprobe/tracepoint，uprobe 未支持 | Agent Guard 内置 BPF LSM loader；公共 loader 后续可补 LSM |
| 现有阻断无暂停 | 主要支持 kill/quarantine/network/user/permission | 增加 cgroup freeze、resume 和 pidfd/SIGSTOP fallback |
| 事件缺少 Agent 语义 | RuntimeEvent 顶层无 instance/unit/policy 字段 | 先使用 `event_data_json` 保持协议兼容 |
| 没有 session/操作链 | 当前事件主要按单点展示 | session、进程链、时间线和 PID 主干行为全景树 |
| 没有跨事件攻击性分析 | 当前 Sigma/告警偏单事件，Agent Guard 无 finding | DC 序列/聚合规则、Finding、Evidence Graph |
| 缺少开箱即用 Agent 行为规则 | 现有 Sigma/策略未按 Agent session 和执行单元定义五项基础行为 | 内置敏感目录、外链、文件生成、敏感命令、提权五个版本化规则 |
| 智能分析无专用安全边界 | 当前 Assistant/LLM 面向其他业务 | 有界脱敏 evidence、固定 JSON Schema、无工具、AI-only 不阻断 |
| 无隔离覆盖状态 | 页面无法区分无沙箱、监控、阻断和远程不可见 | 新增 capability/coverage 状态 |
| 服务端链路不适合同步阻断 | Kafka/DC 为异步处理 | 本地逃逸/通用规则仍负责同步决策；Agent Guard 工具命中由 api-server 异步匹配，服务端保存 Finding |

### 4.3 2026-08-06 实施后的当前代码事实

2026-08-06 已形成以下代码基线：

| 已实现能力 | 实际位置 | 验证事实 |
| --- | --- | --- |
| 11 张表、五规则、六 Profile | `migrations/029_v6.2_agent_guard.sql`、`api-server/internal/model/agent_guard_manifest.go` | SQL/Go/Agent stable key、version、canonical digest 回归；冲突不覆盖 |
| 运行身份、session、execution unit、PID/cgroup 归属 | `agent/internal/agentguard/` | 多 Agent/多实例、PID reuse、fork、cgroup v1/v2、container 与 ambiguous 回归 |
| monitor-only 行为与状态链 | `agent/internal/ebpf/`、`server/internal/grpc_server/`、`dc/internal/pipeline/` | bundle/config status 跨服务契约、投影/重放/脱敏测试 |
| 通用规则、Finding、逃逸 audit | `dc/internal/pipeline/agent_*`、`api-server/internal/service/agent_guard_tool_rule_service.go` | 通用 OS 事实投影；可信工具事件由 api-server 匹配 AGB-004，直接引用工具 event ID，DC 不重复命中 |
| 有界智能分析 | `api-server/internal/service/agent_guard_analysis_service.go` | 固定 Schema、event ID、提示注入边界、AI-only action ceiling |
| BPF LSM 与 execution-unit freeze/action | `agent/internal/ebpf/bpf/agent_guard_lsm.bpf.c`、`agent/internal/agentguard/actions.go`、各服务 action 文件 | BPF build/readelf、状态机、protected target、timeout/auto-resume 非破坏性测试 |
| 可信工具语义、真实会话和 Hook 设置 | `agent/internal/agentguard/tool_*`、`agent/internal/agentguard/runtime_settings.go`、`api-server/internal/service/agent_guard_runtime_settings_service.go` | 五类 Native Hook、session start/end、tool call 生命周期、运行时设置即时下发、tool→process PID 关联 |
| Agent Guard API 与前端 | `api-server/internal/api/handler/agent_guard_handler.go`、`frontend/src/views/detection/AgentGuard/` | 细粒度权限、会话/实例分页、会话范围安全分析、只读内置规则目录、运行时开关、production build |
| fresh/upgrade 发布结构 | `scripts/build_release_package.sh`、`docker-compose.yml` | release contract、Compose config、generate-only v6.2 通过，默认关闭 |

代码完成不等于发布资格通过。当前未在专用宿主机执行真实 LSM `EPERM` 和
freeze/resume，也未对运行中共享 Compose 栈写入端到端测试数据。完整证据、
基线问题和剩余门禁见 [implementation_status_v6.2.md](implementation_status_v6.2.md)。

## 5. V6.2 继承与替换矩阵

| 领域 | 继承 | 新增/替换 |
| --- | --- | --- |
| 静态资产 | `host_application_assets(category=ai_agent)` | 关联 `agent_runtime_instances.asset_id` |
| 主机进程 | V5.8 `/proc` 快照 | fork 标签传播 + cgroup 归属 + 周期校准 |
| 行为事件 | V5.7 file/network event 和 fork/exec/exit 字段 | Agent 进程过滤、统一行为 Schema、session、资源分类、聚合与完整性 |
| 配置同步 | V5.7 `ConfigSync` | `agent_guard_bundle`、版本/digest、应用状态事件 |
| eBPF 扩展 | V5.8 loader/ringbuf/perf/能力检测 | 安全关键内置 Agent Guard BPF、BPF LSM 能力等级 |
| 告警与事件 | `RuntimeEvent`、Kafka、DC、alerts | `agent_behavior_events`、`agent_security_findings`、analysis run 与 Agent Guard 告警类型 |
| 阻断 | `BlockCommand` 和 blocker 审计 | `freeze_execution_unit`、`resume_execution_unit`、`kill_agent_instance` |
| 智能分析 | V6.0/V6.1 LLM worker 和真实证据约束 | Security Analyst、有界证据、规则+AI 联合研判和动作上限 |
| API | Gin handler/service/repository 模式 | `/api/v1/agent-guard/*` behavior/finding/analysis API |
| 前端 | Vue 3、Element Plus、Pinia、Axios、现有导航和 i18n | 外层 Agent 基本信息列表；点击 Agent 后在详情抽屉展示实例、PID 行为全景、Finding、Analysis 和逃逸分析 |
| 审计 | command audit、audit log、Assistant 审批原则 | 策略发布和人工 freeze/resume 写入审计 |

## 6. 外部 Agent 隔离事实基线

产品 Profile 只描述可验证的运行事实，不能把应用层工具权限当成 OS 沙箱。

### 6.1 Codex

Codex Linux sandbox 使用 bubblewrap、Linux namespaces 和 seccomp。Aegis 需要区分宿主机控制进程和实际 sandbox worker，以 worker 的 namespace/cgroup/mount 基线判断隔离状态。

参考：[OpenAI Codex Linux sandbox](https://github.com/openai/codex/blob/main/codex-rs/linux-sandbox/README.md)

### 6.2 OpenClaw

OpenClaw 支持关闭沙箱以及 Docker、SSH、OpenShell 等执行后端。部分工具执行可以在沙箱中，但 Gateway、插件或其他宿主机组件不必处于同一个隔离边界。

参考：[OpenClaw Sandboxing](https://docs.openclaw.ai/gateway/sandboxing)

### 6.3 Hermes

Hermes 默认 local terminal backend 直接在宿主机执行，也支持 Docker、Singularity、SSH、Modal、Daytona 等后端；官方安全模型明确将 OS 级隔离视为对抗恶意 LLM 的安全边界。

参考：

- [Hermes Security Model](https://github.com/NousResearch/hermes-agent/blob/main/SECURITY.md)
- [Hermes Configuration](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/configuration.md)

## 7. V6.2 不继承的旧假设

以下旧设计不能继续沿用：

- “进程名匹配即可确定 Agent 身份”。
- “Agent 的所有子进程都由 PPID 直接连接”。
- “控制进程在宿主机意味着沙箱已经逃逸”。
- “tracepoint 上报后再 kill 等价于提前阻断”。
- “所有容器执行进程都是 Agent 直接子进程”。
- “沙箱配置存在就代表实际生效”。
- “远程执行可以由本地主机 Agent 完整观测”。
- “动态 DetectionPackage 适合作为不可被策略卸载的核心防护边界”。
- “只监控敏感文件就足以判断 Agent 是否具有攻击性”。
- “每次 syscall 都必须上传才叫全行为采集”。
- “LLM 判定 malicious 就可以直接自动暂停 Agent”。
- “没有工具 Hook 时可以根据进程名推测并展示工具调用”。

## 8. 文档与代码同步要求

实施 V6.2 时，每个阶段都必须回写本文中的“当前代码事实”：

- 已完成的差距改为实现路径和测试证据。
- Profile 行为发生变化时记录 Profile version 和来源。
- 协议采用 `event_data_json` 的字段必须形成固定 JSON Schema 和契约测试。
- 如果最终选择新增 proto 字段，必须保持字段号向后兼容并同步四个 Go 模块的生成代码。

## 9. 本次设计输入索引

### 9.1 V6.1 当前版本

| 文档 | 本次使用内容 |
| --- | --- |
| [weak_password_agent_program_design_v6.1.md](../aegis_system_design_v6.1/weak_password_agent_program_design_v6.1.md) | Agent 受控路径、结构化错误、日志脱敏和工具边界 |
| [weak_password_api_server_design_v6.1.md](../aegis_system_design_v6.1/weak_password_api_server_design_v6.1.md) | Handler/Service/Repository、Agent 工具转发、异步状态 |
| [weak_password_database_design_v6.1.md](../aegis_system_design_v6.1/weak_password_database_design_v6.1.md) | V6.1 数据建模、状态、索引和审计方式 |
| [weak_password_frontend_prd_v6.1.md](../aegis_system_design_v6.1/weak_password_frontend_prd_v6.1.md) | 前端页面、任务状态、失败和空状态 |
| [assistant_tool_mapping_and_intent_decomposition_design_v6.1.md](../aegis_system_design_v6.1/assistant_tool_mapping_and_intent_decomposition_design_v6.1.md) | 后端确定性校验、真实实体绑定、完成证据 |
| [assistant_workflow_knowledge_high_level_tools_and_deterministic_execution_design_v6.1.md](../aegis_system_design_v6.1/assistant_workflow_knowledge_high_level_tools_and_deterministic_execution_design_v6.1.md) | 受理/运行/终态、覆盖率和证据一致性 |
| [software_asset_collection_pipeline_fix.md](../aegis_system_design_v6.1/fix/software_asset_collection_pipeline_fix.md) | 当前资产采集真实链路 |
| [weak_password_container_asset_path_metadata_fix.md](../aegis_system_design_v6.1/fix/weak_password_container_asset_path_metadata_fix.md) | 容器资产和路径元数据 |
| [weak_password_pid_dedupe_and_process_config_discovery_fix.md](../aegis_system_design_v6.1/fix/weak_password_pid_dedupe_and_process_config_discovery_fix.md) | PID 去重和进程配置发现 |
| [assistant_runtime_evidence_and_async_execution_fix.md](../aegis_system_design_v6.1/fix/assistant_runtime_evidence_and_async_execution_fix.md) | 异步证据、终态和失败表达 |

### 9.2 V6.0 总体控制面

| 文档 | 本次使用内容 |
| --- | --- |
| [README.md](../aegis_system_design_v6.0/README.md) | V6.0 版本定位和现有模块边界 |
| [overall_architecture_design_v6.0.md](../aegis_system_design_v6.0/overall_architecture_design_v6.0.md) | 前端/api-server/server/Agent/DC 总体通信关系 |
| [agent_runtime_tool_orchestration_design_v6.0.md](../aegis_system_design_v6.0/agent_runtime_tool_orchestration_design_v6.0.md) | Agent 工具、风险、审计和后端编排边界 |
| [implementation_blueprint_v6.0.md](../aegis_system_design_v6.0/implementation_blueprint_v6.0.md) | 文件级落地和跨模块实施方式 |
| [ai_asset_collection_ui_design_v6.0.md](../aegis_system_design_v6.0/ai_asset_collection_ui_design_v6.0.md) | AI Agent 资产前端入口 |
| [host_attack_investigation_agent_design_v6.0.md](../aegis_system_design_v6.0/host_attack_investigation_agent_design_v6.0.md) | 运行时证据链与研判数据需求 |

### 9.3 V5.8 资产与动态 eBPF

| 文档 | 本次使用内容 |
| --- | --- |
| [README.md](../aegis_system_design_v5.8/README.md) | 智能资产和动态 eBPF 总体定位 |
| [overall_architecture_design_v5.8.md](../aegis_system_design_v5.8/overall_architecture_design_v5.8.md) | builder/server/Agent/DC/Kafka 链路 |
| [agent_intelligent_asset_collection_design_v5.8.md](../aegis_system_design_v5.8/agent_intelligent_asset_collection_design_v5.8.md) | `/proc`、PID/PPID/start time/container ID 采集 |
| [backend_intelligent_asset_collection_design_v5.8.md](../aegis_system_design_v5.8/backend_intelligent_asset_collection_design_v5.8.md) | 资产控制面服务与 Agent 工具 |
| [database_intelligent_asset_collection_design_v5.8.md](../aegis_system_design_v5.8/database_intelligent_asset_collection_design_v5.8.md) | `host_application_assets` 等资产表 |
| [frontend_intelligent_asset_collection_design_v5.8.md](../aegis_system_design_v5.8/frontend_intelligent_asset_collection_design_v5.8.md) | 主机资产页面和字段 |
| [agent_dynamic_ebpf_design_v5.8.md](../aegis_system_design_v5.8/agent_dynamic_ebpf_design_v5.8.md) | ringbuf/perf、hook allowlist、动态 loader 和状态上报 |
| [api_grpc_design_v5.8.md](../aegis_system_design_v5.8/api_grpc_design_v5.8.md) | API Server → Server → Agent 配置/命令和 RuntimeEvent |

### 9.4 V5.7 运行时与配置同步

| 文档 | 本次使用内容 |
| --- | --- |
| [agent_config_sync_design.md](../aegis_system_design_v5.7/agent_config_sync_design.md) | `ConfigSync`、重连全量同步和 Agent ConfigManager |
| [ebpf_file_network_event_design.md](../aegis_system_design_v5.7/ebpf_file_network_event_design.md) | 文件操作 Hook、字段、过滤和性能要求 |
| [ebpf_kernel_adaptation_design.md](../aegis_system_design_v5.7/ebpf_kernel_adaptation_design.md) | 内核/BTF/ringbuf/perf 能力与降级 |
| [ai_auto_block_design.md](../aegis_system_design_v5.7/ai_auto_block_design.md) | 告警、策略和阻断状态 |
| [auto_block_status_update_fix_design.md](../aegis_system_design_v5.7/auto_block_status_update_fix_design.md) | Agent 真实阻断结果回写 |

### 9.5 V5.6 及更早基础

| 文档 | 本次使用内容 |
| --- | --- |
| [architecture_design_v5.6.md](../aegis_system_design_v5.6/architecture_design_v5.6.md) | Agent 双向 gRPC、Kafka/DC 主链路 |
| [agent_detailed_design_v5.6.md](../aegis_system_design_v5.6/agent_detailed_design_v5.6.md) | Agent 进程树、文件、网络和日志工具 |
| [block_failure_reason_trace_design.md](../aegis_system_design_v5.6/block_failure_reason_trace_design.md) | 阻断失败原因端到端透传 |
| [architecture_design_v5.5.md](../aegis_system_design_v5.5/architecture_design_v5.5.md) | 微服务边界和运行时数据流 |
| [communication_protocol_design_v5.5.md](../aegis_system_design_v5.5/communication_protocol_design_v5.5.md) | Agent/Server/API 通信基础 |
| [README.md](../aegis_system_design_v5.2/README.md) | eBPF/Sigma/告警/阻断早期基础 |
