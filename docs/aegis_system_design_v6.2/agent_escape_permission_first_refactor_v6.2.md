# Agent 逃逸检测权限优先重构（V6.2）

## 目标

逃逸检测不再把所有隔离变化或高风险系统调用直接视为逃逸。判定顺序固定为：

1. 读取并校验当前智能体的有效权限/沙箱快照；
2. 将 Hook 的工具调用、命令和 session 与 eBPF 事件的 PID + `start_ticks` 关联；
3. 判断目标是否超出该权限允许的 workspace、临时目录和网络边界；
4. 使用 eBPF 的命令执行结果及 PID + `start_ticks` 关联确认是否真的执行；
5. 只有 Hook、命令和执行结果链完整时才生成逃逸告警。

## 产品边界适配

权限快照由原生 Hook 签名传递，解析器按 `agent_type + backend` 选择语义，不再把所有产品套用 Codex 规则：

| 智能体 | 有效边界 | 允许的逃逸语义 |
| --- | --- | --- |
| Codex | workspace/temp roots、网络开关、进程边界 | 工作区外访问、网络越界、运行时 socket、namespace/mount/ptrace/capability |
| Claude Code | Seatbelt/bubblewrap 沙箱、cwd/会话临时目录、网络 proxy/allowlist；`bypassPermissions` 不等于关闭沙箱 | 沙箱外文件/网络访问、运行时 socket、进程边界；沙箱未启用时只按实际确认/边界元数据判断 |
| OpenClaw | `mode/scope`、Docker/Podman/OpenShell 后端、`workspaceAccess`、网络 none、`tools.elevated` | workspace/网络越界、运行时 socket、进程边界、`elevated` 绕过沙箱 |
| Hermes | local 默认无隔离；Docker/Singularity/Modal/Daytona/Vercel/OpenShell 后端；`HERMES_WRITE_SAFE_ROOT` | 容器/远端边界越界、安全写入根外写入、网络及进程边界 |
| Zcode | Default/Confirm Before Changes/Auto Edit/Plan/Full Access 的确认语义；不臆造未公开的 OS 沙箱 | 未确认的命令/文件/网络实际执行；Full Access 不检测 |

远端后端缺少 remote execution sensor 时，权限为 `remote_unobservable`，不生成 finding；明确 `no_isolation` 的本地运行也不生成逃逸 finding，避免把产品本来允许的行为误报为逃逸。

## 权限分类

| 分类 | 识别 | 逃逸检测 |
| --- | --- | --- |
| `full_access` | 产品明确报告 Full Access，或 Codex `danger-full-access`/`bypassPermissions`；Claude 只有在沙箱同时关闭时才把 `bypassPermissions` 视为 Full Access | 不适用；不生成逃逸告警，仍可保留独立的行为审计 |
| `restricted` | 受限沙箱、容器后端、工作区访问级别或确认模式已完整上报 | 按产品适配的 workspace、临时目录、域名 allow/deny、确认和进程边界检测 |
| `unknown` | 缺少或校验不完整的权限快照 | 不生成逃逸告警，只保留降级/可观测性状态 |

`approvals_reviewer=auto_review` 只改变审核方式，不扩大沙箱边界；网络开关是独立维度。授权的越界动作归类为 `authorized_boundary_expansion`，不归类为逃逸。

## 判定矩阵

| Hook/命令 | eBPF/PID 执行结果 | 结论 |
| --- | --- | --- |
| 未越界 | 任意 | 不报 |
| 越界请求 | 已关联且返回失败/被拒绝 | `policy_violation_attempt`，可疑 |
| 越界执行成功 | 已关联，PID 与 `start_ticks` 一致且 eBPF 返回成功 | `confirmed_escape`，逃逸 |
| 越界执行成功 | 已授权 | `authorized_boundary_expansion`，不报 |
| 越界执行成功 | 未关联 | 不报，记录 `evidence_insufficient` |
| Full Access | 任意 | `not_applicable`，不报 |

## 数据链路与会话范围

Native Hook 传递 `permission_mode`、`sandbox_mode`、`cwd`、workspace/temp roots、审批和网络元数据；Agent 对其做签名校验并保存到真实 Hook session 的短期权限快照。session 状态事件将快照写入 `agent_behavior_sessions.permission`，因此详情页可以按 session 隔离展示权限，不能用主机级或实例级默认值代替。

工具开始事件绑定 `tool_call_id`、工具和命令，eBPF 事件必须用 PID + `start_ticks` 关联到该调用。逃逸事件只携带脱敏后的权限摘要、Hook 证据、进程证据和 eBPF 执行结果。finding 使用 `escape:v2:<session>:<rule>:<event>` 幂等键，并且 API 查询必须带 `session_id`。

逃逸检测复用行为检测的可信 Hook/eBPF 数据流，但不复用行为规则或旧的隔离漂移规则；逃逸有独立的权限边界规则目录、Hook 点和内置防护策略。

## 内置逃逸规则

| 规则 | 触发边界 | 关键证据 |
| --- | --- | --- |
| `AGE-BUILTIN-101` 访问工作区外路径 | 访问 workspace/temp roots 之外的文件 | 权限 roots、Hook 工具/命令、PID/start_ticks、eBPF 文件结果 |
| `AGE-BUILTIN-102` 受限网络访问 | 网络权限关闭时 connect/curl/socket | 网络权限、Hook 工具/命令、PID/start_ticks、目标和 eBPF 结果 |
| `AGE-BUILTIN-103` 访问容器运行时控制接口 | Docker/containerd/CRI-O/Podman socket | 权限、Hook、PID/start_ticks、socket 和 eBPF 结果 |
| `AGE-BUILTIN-104` 执行进程边界操作 | setns、mount、ptrace、内核加载、身份/capability 变更 | 权限、Hook、PID/start_ticks、syscall 和 eBPF 结果 |
| `AGE-BUILTIN-105` 绕过操作确认边界 | Zcode/Claude 的 `approval_required` 未获批准即实际执行 | Hook、确认状态、PID/start_ticks 和 eBPF 结果 |
| `AGE-BUILTIN-106` 写入 Hermes 受保护路径 | `HERMES_WRITE_SAFE_ROOT` 外的实际写入 | Hook、safe root、PID/start_ticks 和 eBPF 文件结果 |
| `AGE-BUILTIN-107` OpenClaw 提权绕过沙箱 | Docker/Podman/OpenShell 中 `tools.elevated` 工具实际执行 | Hook、elevated、PID/start_ticks 和 eBPF 结果 |

这些规则由 `/api/v1/agent-guard/escape-rules` 独立提供给“内置防护策略”弹窗；行为策略的开关或目录变更不会隐式创建、删除或修改逃逸 Hook。

## 安全与降级

- 权限快照缺失、签名无效、PID 复用或 Hook/PID/执行结果链不完整时，fail-closed 到“无逃逸告警”，避免误报；原始事件可保留为不可用证据，但不能写入 escape finding。
- Full Access 仅跳过逃逸策略，不关闭独立的行为监控、工具审计或主机安全规则。
- 不记录原始命令输出、token、环境变量；日志只记录事件 ID 哈希、权限分类、关联状态、证据完整性和结论。

## 验收标准

- Full Access 下运行 runtime socket、namespace、cgroup 或网络命令不会生成逃逸 finding。
- 受限模式下命令被拒绝时只生成可疑的 `policy_violation_attempt`；成功越界且链路完整时生成 `confirmed_escape`。
- 权限未知、Hook 缺失、PID/start_ticks 不匹配或执行结果不完整时不生成 finding。
- 前端先展示权限态，再按“权限规则 → Hook/工具命令 → eBPF/PID 执行结果 → 判定动作”竖向展示证据。
- 详情必须先选择一个 session；Full Access session 显示“无需逃逸检测”且 finding 数为 0，受限 session 才请求 `finding_domain=escape&session_id=...`。
- V6.2 迁移删除 `escape:v1:*` 历史发现，旧的 `/proc`、cgroup 隔离复核字段不再出现在逃逸详情或内置规则目录。
