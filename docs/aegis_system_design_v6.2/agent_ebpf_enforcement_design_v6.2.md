# Aegis V6.2 Agent 与 eBPF 防护详细设计

**版本**：6.2  
**日期**：2026-08-06
**状态**：Agent OS 事实、PID/PPID 关联和本地逃逸决策设计；Agent Guard 工具命令规则由 api-server 匹配

> 当前实现基线见 [current_implementation_baseline_2026-08-06.md](current_implementation_baseline_2026-08-06.md)。
> eBPF/`/proc` 不再负责创建 Agent Guard 工具命中；它只提供 OS 事实以及工具事件到
> 实际 PID/PPID/start_ticks/cmdline 的关联。

## 1. 设计目标

Agent 侧是 V6.2 的实时安全边界，负责：

1. 识别 AI Agent 控制进程及其实际执行后端。
2. 持续维护 session、进程、namespace、container/cgroup 与 Agent 实例的归属。
3. 采集归属进程的命令/进程、文件、网络、身份权限、内核、IPC 和隔离控制行为。
4. 将内核事件规范化、脱敏、关联和聚合为统一 `AgentBehaviorEvent`；可信 Native Hook
   工具事件作为独立输入转发，不在 Agent 内进行工具命令规则匹配。
5. 执行服务端下发的原子行为策略和资源策略。
6. 建立执行单元隔离基线，检测逃逸行为和状态漂移。
7. 在本机内核能力允许时提前返回 `EPERM`。
8. 对有明确规则证据的高危执行单元执行 freeze/resume/kill。
9. 上报结构化行为，但不上传文件内容、环境变量值或网络内容。
10. 在同一主机同时运行多个 Agent、同类型多个实例时保持唯一归属和动作隔离。

## 2. 与当前 Agent 的集成

### 2.1 复用模块

| 当前模块 | 复用点 |
| --- | --- |
| `internal/assets` | AI Agent 配置资产、进程快照、PID/PPID/exe/cmdline/cwd/UID/start time/container ID |
| `internal/ebpf/bpf/fork.bpf.c` | fork 生命周期和父子标签传播 |
| `internal/ebpf/bpf/execve.bpf.c` | exec 元数据和新 worker 发现 |
| `internal/ebpf/bpf/exit.bpf.c` | PID 标签清理 |
| `internal/ebpf/bpf/file.bpf.c` | 文件监控 hook 和事件字段基础 |
| 现有网络 eBPF | connect/listen/DNS 等网络行为字段基础 |
| `internal/configmgr` | `ConfigSync` 接入 |
| `internal/blocker` | 人工阻断入口和审计 |
| `internal/client` | 配置接收、事件上报和阻断命令处理 |
| `internal/ebpf/kernel` | 内核/BTF/ringbuf/perf 能力探测 |

### 2.2 新增目录

```text
agent/internal/agentguard/
├── manager.go
├── types.go
├── capability.go
├── policy/
│   ├── bundle.go
│   ├── compiler.go
│   ├── store.go
│   └── matcher.go
├── profile/
│   ├── registry.go
│   ├── matcher.go
│   └── builtin_profiles.go
├── identity/
│   ├── instance_manager.go
│   ├── process_tracker.go
│   ├── cgroup_tracker.go
│   ├── namespace.go
│   └── reconciler.go
├── adapter/
│   ├── adapter.go
│   ├── local_process.go
│   ├── linux_namespace.go
│   ├── oci_container.go
│   └── remote_sandbox.go
├── detector/
│   ├── process_detector.go
│   ├── file_detector.go
│   ├── network_detector.go
│   ├── identity_detector.go
│   ├── kernel_detector.go
│   ├── escape_detector.go
│   └── drift_detector.go
├── behavior/
│   ├── event.go
│   ├── normalizer.go
│   ├── session_correlator.go
│   ├── resource_classifier.go
│   ├── redactor.go
│   └── aggregator.go
├── enforcement/
│   ├── decision_engine.go
│   ├── lsm.go
│   ├── freezer.go
│   ├── pidfd.go
│   └── protected_targets.go
└── reporter/
    ├── event_builder.go
    ├── status_reporter.go
    └── deduplicator.go

agent/internal/ebpf/bpf/
├── agent_guard_process.bpf.c
├── agent_guard_file.bpf.c
├── agent_guard_network.bpf.c
├── agent_guard_identity.bpf.c
├── agent_guard_escape.bpf.c
└── agent_guard_lsm.bpf.c
```

Agent Guard 是内置安全模块，不放入可按业务策略卸载的 `internal/dynpkg`。产品 Profile 和策略可以动态下发，但 BPF 程序必须随受信 Agent 二进制构建和发布。

## 3. 启动顺序

```text
Agent 启动
  -> 探测 kernel/BTF/ringbuf/perf/BPF LSM/cgroup v2/pidfd 能力
  -> 加载 last-known-good Agent Guard bundle
  -> 加载进程生命周期 BPF
  -> 加载进程、文件、网络、身份权限和逃逸 monitor BPF
  -> 能力允许时加载 BPF LSM
  -> 扫描 /proc，识别现有 Agent 实例和执行单元
  -> 填充 guarded_pids / guarded_cgroups maps
  -> 启动事件 reader、周期 reconciler 和状态 reporter
  -> 建立 gRPC 连接
  -> 接收服务端最新 bundle 并原子更新
```

不得等待服务端配置后才开始进程归属。服务端不可用时先使用本地 last-known-good；本地没有历史策略时仍识别实例并以 monitor-only 记录内置高危逃逸信号。

## 4. 本机能力探测

```go
type GuardCapabilities struct {
    KernelRelease      string   `json:"kernel_release"`
    BTF                bool     `json:"btf"`
    RingBuffer         bool     `json:"ring_buffer"`
    PerfBuffer         bool     `json:"perf_buffer"`
    BPFLSM             bool     `json:"bpf_lsm"`
    BPFPathResolution  bool     `json:"bpf_path_resolution"`
    CgroupVersion      int      `json:"cgroup_version"`
    CgroupFreeze       bool     `json:"cgroup_freeze"`
    Pidfd              bool     `json:"pidfd"`
    NamespaceRead      bool     `json:"namespace_read"`
    MountInfoRead      bool     `json:"mountinfo_read"`
    SupportedHooks     []string `json:"supported_hooks"`
    DegradedReasons    []string `json:"degraded_reasons"`
}
```

能力等级由事实计算，不由配置强制覆盖：

```go
func DeriveCoverage(c GuardCapabilities, unit IsolationType) CoverageLevel
```

如果配置要求 `deny` 但 `BPFLSM=false`，Agent 必须：

- 不把策略悄悄改成成功阻断。
- 生成 `would_deny` 或 `enforcement_unavailable`。
- 上报 capability reason。
- 前端显示 monitor-only。

## 5. Adapter Profile

### 5.1 数据结构

```go
type AdapterProfile struct {
    ProfileKey         string                 `json:"profile_key"`
    ProfileVersion     int64                  `json:"profile_version"`
    AgentType          string                 `json:"agent_type"`
    DisplayName        string                 `json:"display_name"`
    ControllerMatch    []ProcessMatchRule     `json:"controller_match"`
    WorkerMatch        []ProcessMatchRule     `json:"worker_match"`
    BackendDetectors   []BackendDetector      `json:"backend_detectors"`
    DefaultFamily      IsolationType          `json:"default_family"`
    ExpectedIsolation  IsolationExpectation   `json:"expected_isolation"`
    EscapeRules        []EscapeRule           `json:"escape_rules"`
    Digest             string                 `json:"digest"`
}
```

匹配字段只允许：

- exe basename/exact path。
- cmdline 固定参数或受限正则。
- 配置路径存在性。
- 父进程/祖先进程特征。
- cgroup/container label。
- namespace helper 进程。

Profile 不允许包含：

- shell。
- 可执行代码。
- eBPF 源码或对象。
- 任意文件读取表达式。
- 不受限正则。

### 5.2 匹配可信度

进程名可以被伪造，至少满足两个独立证据才标记为 `confirmed`：

```text
exe/cmdline + config asset
exe/cmdline + known parent
controller + known sandbox helper
controller + known container label
```

只有单一进程名时为 `candidate`，可以监控但默认不执行高危自动 freeze。

### 5.3 首批 Profile 行为

#### Codex

- 控制进程匹配 Codex 可执行路径、命令行和配置资产。
- 识别 `codex-linux-sandbox`、bubblewrap 或 namespace worker。
- 每个 worker 建立独立 `linux_namespace` execution unit。
- 控制进程保留在实例根，不参与 worker namespace 逃逸基线判断。

#### OpenClaw

- 识别 Gateway/Agent 控制进程和配置中的 sandbox backend。
- backend off/local：`local_process_tree`，coverage=`no_isolation`。
- Docker：关联 Docker 请求、container label/ID 和 cgroup。
- SSH/OpenShell：建立 remote unit；只有远端 Aegis Agent 回传关联状态才提升覆盖等级。

#### Hermes

- 识别 Python/Hermes 进程和 active profile。
- terminal local：`local_process_tree`。
- Docker/Singularity：容器执行单元。
- SSH/Modal/Daytona：远程执行单元。
- 整个 Hermes 运行于 Docker/OpenShell 时使用 `whole_process_container`。

## 6. 实例与执行单元管理

### 6.1 本地身份

```go
type ProcessIdentity struct {
    PID            uint32
    StartTimeTicks uint64
    PIDNamespace   uint64
}
```

用户态主键：

```text
host_id + pid + start_time_ticks
```

BPF 快速查找键使用 TGID；value 中保存 Agent 分配的短整数 slot。exit 时删除，reconciler 使用 start time 防止 PID 重用导致错误标签。

### 6.2 RuntimeInstance

```go
type RuntimeInstance struct {
    InstanceID          string
    AssetID             string
    ProfileKey          string
    ProfileVersion      int64
    AgentType           string
    Controller          ProcessIdentity
    ControllerExe       string
    ControllerCmdline   string
    RunUID               uint32
    Confidence           string
    Status               string
    FirstSeenAt          time.Time
    LastSeenAt           time.Time
    ExecutionUnitIDs     []string
}
```

Instance Manager 使用 `InstanceID -> RuntimeInstance` 和
`ProcessIdentity -> InstanceID` 两类索引，不能使用 `AgentType` 作为唯一键。
同一主机可以同时存在：

```text
Codex controller PID 4100
Codex controller PID 4400
OpenClaw controller PID 5200
Hermes controller PID 6100
```

四个 controller 分配四个独立 instance slot；相同 AgentType 不能覆盖旧 slot。

### 6.3 ExecutionUnit

```go
type ExecutionUnit struct {
    UnitID              string
    InstanceID          string
    Type                IsolationType
    RootProcess         ProcessIdentity
    CgroupID            uint64
    CgroupPath          string
    ContainerID         string
    RemoteExecutionID   string
    Baseline            IsolationBaseline
    Coverage            CoverageLevel
    Status              string
    FirstSeenAt         time.Time
    LastSeenAt          time.Time
}
```

### 6.4 隔离基线

```go
type IsolationBaseline struct {
    NamespaceInodes map[string]uint64 `json:"namespace_inodes"`
    CgroupID        uint64            `json:"cgroup_id"`
    CgroupPath      string            `json:"cgroup_path"`
    MountDigest     string            `json:"mount_digest"`
    SensitiveMounts []MountSnapshot   `json:"sensitive_mounts"`
    CapEffective    uint64            `json:"cap_effective"`
    CapPermitted    uint64            `json:"cap_permitted"`
    NoNewPrivileges bool              `json:"no_new_privileges"`
    SeccompMode     int               `json:"seccomp_mode"`
    RootLink        string            `json:"root_link"`
}
```

完整 mountinfo 不每次上传；Agent 保存基线摘要，事件发生时上传差异和必要 evidence。

## 7. 进程归属算法

### 7.1 本地 fork 标签传播

1. Instance Manager 识别控制进程，将 PID 写入 `guarded_pids`。
2. `sched_process_fork` 读取父 PID 标签并复制给 child PID。
3. `execve` 更新进程元数据，但不清除归属。
4. `sched_process_exit` 删除 PID 标签。
5. 用户态接收 fork/exec/exit 事件，维护完整进程树和数据库上报所需元数据。

这样可以覆盖 shell、double-fork 前的标签传播和进程改名。对于事件丢失或 Agent 启动前已有进程，由 reconciler 修复。

### 7.2 容器/cgroup 归属

Docker/containerd 创建的容器进程通常不是 AI Agent 的直接子进程。适配流程：

```text
识别 Agent 发起的 backend 请求
  -> 提取/观察 container ID、label 或 execution token
  -> 解析 container cgroup
  -> guarded_cgroups[cgroup_id] = execution unit slot
  -> BPF hook 通过 bpf_get_current_cgroup_id() 直接归属
```

支持识别：

- cgroup v1 `/docker/<id>`。
- cgroup v2 `docker-<id>.scope`。
- containerd/Kubernetes scope。
- Podman/libpod scope。

数据库保存完整 container ID；UI 可以展示短 ID。

如果无法证明某容器属于哪个 Agent 实例，标记 orphan candidate，不自动应用 freeze。

### 7.3 远程归属

本地主机只能记录：

- 控制进程。
- SSH/远程 backend 目标摘要。
- 本地连接时间和 execution/session ID。

远程 Aegis Agent 上报相同 correlation token 后，api-server/DC 将本地控制实例和远程执行单元关联。token 只用于关联，不作为认证凭据。

未关联时 coverage=`remote_unobservable`。

### 7.4 Reconciler

默认低频执行：

```text
扫描 /proc
  -> 校验 controller 是否仍存在且 start time 未变
  -> 补全漏失 fork
  -> 删除过期 PID 标签
  -> 校验 cgroup 成员
  -> 重新计算 namespace/capability 漂移
```

避免固定全主机高频扫描。间隔应可配置，默认建议 30 秒；事件活跃实例可缩短，空闲实例可延长。

### 7.5 多 Agent 冲突与跨 Agent 启动

`guarded_pids` 的一个 TGID 只能指向一个主 `instance_slot/unit_slot`，同一事件
不能复制给多个 Agent。归属优先级：

1. 已确认的 execution unit/container cgroup。
2. 已有 controller 的 fork 标签传播。
3. PID/start_ticks、exe、cmdline、资产和 Profile 多证据识别。
4. 证据冲突或只有进程名时标记 ambiguous candidate。

如果已归属 Agent A 的后代进程 exec 后满足 Agent B controller 的确认条件：

- 为 B 创建独立 RuntimeInstance 和 instance slot。
- controller 切换为 B 的主归属，B 的后代继承 B slot。
- 记录 `launched_by_instance=A`、controller process identity 和 correlation evidence。
- A 的历史 process/exec 事实保持不变，但 B 的后续行为不再计入 A。

ambiguous candidate 可以上报审计证据，但不得应用自动 freeze。只有归属到
明确 execution unit 的事件才能触发 `deny_and_freeze`。

## 8. BPF Map 设计

### 8.1 进程与 cgroup

```c
struct guard_subject {
    __u64 instance_slot;
    __u64 unit_slot;
    __u64 policy_slot;
    __u64 process_epoch;
    __u32 flags;
};

BPF_HASH(guarded_pids, __u32, struct guard_subject);
BPF_HASH(guarded_cgroups, __u64, struct guard_subject);
```

查找顺序：

1. `guarded_pids[tgid]`
2. 未命中时查 `guarded_cgroups[bpf_get_current_cgroup_id()]`
3. 都未命中直接返回 allow，不产生 Agent Behavior 事件

### 8.2 路径规则

```c
struct path_lpm_key {
    __u32 prefixlen;
    __u8 path[256];
};

struct path_rule_value {
    __u64 rule_slot;
    __u64 policy_slot;
    __u32 operation_mask;
    __u32 action;
};

BPF_LPM_TRIE(path_rules, struct path_lpm_key, struct path_rule_value);
```

exact 规则由用户态额外校验长度；prefix 直接使用 LPM。复杂 glob 在用户态监控路径处理，不宣称内核可完全阻断。`AGB-BUILTIN-001` 的可编译敏感路径进入该 map。

### 8.3 首批内置行为规则 Map

```c
BPF_LPM_TRIE(sensitive_path_rules, ...);       // AGB-BUILTIN-001
BPF_HASH(external_network_policy, ...);        // AGB-BUILTIN-002 的 CIDR/port 原子部分
BPF_HASH(file_create_policy_by_unit, ...);     // AGB-BUILTIN-003
BPF_HASH(legacy_executable_policy, ...);      // 仅兼容通用本地 executable/自保护规则，不产生 AGB-004 工具命中
BPF_HASH(privilege_transition_policy, ...);    // AGB-BUILTIN-005
```

边界：

- 域名 externality、复杂 argv、跨事件关系和 Agent Guard 工具命令匹配由服务端处理；
  其中 AGB-BUILTIN-004 由 api-server 对可信 Hook 工具输入执行。
- BPF LSM 只对 exact/prefix path、CIDR/port、resolved executable 和可验证 credential/capability transition 做同步决策。
- 文件创建、敏感命令和公网连接首期默认只采集/告警，不默认 deny。
- 提权规则必须携带 before/after 和 outcome；只观察到 `sudo` exec 不能在内核侧标记提权成功。

内置规则详细定义见
[builtin_behavior_rules_and_panorama_tree_v6.2.md](builtin_behavior_rules_and_panorama_tree_v6.2.md)。

### 8.4 逃逸规则与动作

```c
struct escape_policy_value {
    __u64 policy_slot;
    __u64 allowed_namespace_mask;
    __u64 allowed_capability_mask;
    __u32 action_by_rule[MAX_ESCAPE_RULES];
};

BPF_HASH(unit_escape_policies, __u64, struct escape_policy_value);
```

UUID 不直接存入高频 BPF map。Agent 分配稳定 slot，并在用户态映射 slot ↔ UUID。

## 9. 智能体行为采集

Agent 端只采集“已归属 Agent 的安全语义事件”，不是把每次 syscall 原样上传。统一事件契约和服务端分析见
[agent_behavior_telemetry_and_analysis_design_v6.2.md](agent_behavior_telemetry_and_analysis_design_v6.2.md)。

### 9.1 传感器与操作覆盖

| 行为域 | 兼容监控 Hook | 完整阻断/高可信 Hook |
| --- | --- | --- |
| process/command | fork/exec/exit tracepoint | `sched_process_*`、`lsm/bprm_check_security` |
| file open/read/write | open/read/write tracepoint + fd tracker | `lsm/file_open`、`inode_permission` |
| file create/delete/rename | 对应 syscall tracepoint | `lsm/path_*` |
| chmod/chown | syscall tracepoint | 对应 LSM/path/inode hook |
| network connect/listen | socket syscall/现有网络传感器 | `lsm/socket_connect`、`socket_bind` |
| identity/capability | setuid/setgid/capset tracepoint | `lsm/task_fix_setuid`、capability hook |
| namespace/mount/root | setns/unshare/mount/chroot tracepoint | 对应 BPF LSM hook |
| kernel/ptrace | bpf/perf/module/ptrace tracepoint/kprobe | 可用的 BPF LSM/security hook |
| tool/control | 产品 Adapter 官方日志/Hook | 不作为内核阻断依据 |

命令事件记录 exe、脱敏 argv、cwd、UID/GID、父进程和 exit 结果。不得采集完整 stdin/stdout/stderr。shell stdin 或解释器动态代码不可见时标记 `command_visibility=partial`。

### 9.2 五个内置规则的 Agent 证据

| Rule ID | Agent 必须提供的字段 |
| --- | --- |
| `AGB-BUILTIN-001` | PID/PPID/cmdline、operation、file name、raw/resolved/host path、outcome/errno |
| `AGB-BUILTIN-002` | PID/PPID/cmdline、destination IP/port、protocol、direction、DNS 关联来源、outcome/errno |
| `AGB-BUILTIN-003` | PID/PPID/cmdline、create attempt/result、file name/path、inode/dev、owner/mode、hash status |
| `AGB-BUILTIN-004` | Native Hook tool name/tool_call_id/tool input/result/command；PID/PPID、resolved executable、脱敏 cmdline、cwd、exit code/signal 仅由 eBPF/procfs 作为关联补充 |
| `AGB-BUILTIN-005` | PID/PPID/cmdline、UID/GID/capability/namespace before/after、outcome/errno |

事件缺少必需字段时仍上报行为事实，但 rule evaluation 为 `insufficient_evidence`，不得补默认值伪造完整 hit。

### 9.3 文件行为与路径解析

syscall 参数可能是：

- 绝对路径。
- 相对当前 cwd 的路径。
- 相对 `dirfd` 的路径。
- namespace/container 内路径。
- 符号链接。

用户态解析输出：

```go
type ResolvedPath struct {
    RawPath        string
    ResolvedPath   string
    HostPath       string
    MountNS        uint64
    Device         uint64
    Inode          uint64
    Resolution     string // exact, proc_root, cwd, dirfd, inode, unresolved
    Confidence     string
}
```

解析顺序：

1. 使用事件携带的 path 和 dirfd。
2. 通过 `/proc/<pid>/cwd`、`/proc/<pid>/fd/<dirfd>` 和 `/proc/<pid>/root` 解析。
3. 记录 namespace 内路径和可解析的宿主机路径。
4. 使用 `lstat/stat` 获取 dev+inode，用于去重和高可信证据。

不得因为解析失败丢弃事件；保存 raw path 并标记 `unresolved`。解析失败时默认不执行高风险自动 freeze，除非 LSM 已在内核侧明确命中规则。

当前 `file.bpf.c` 主要过滤写意图。V6.2 必须识别：

- `O_RDONLY`
- `O_RDWR`
- execute/read mapping 等可见操作

所有已归属 Agent 的文件行为都可以进入本地聚合，但只有以下事件必须逐条上报：

- 策略/资源分类命中的凭据、持久化、安全控制或容器控制资源。
- create/delete/rename/chmod/chown/execute 等状态变化。
- 关联规则需要的下载落地、脚本执行和清痕证据。
- deny/would_deny/enforcement_unavailable。

普通 read/write 使用 fd + inode/dev + process epoch 短窗口聚合。事件区分 `open_intent` 和 `read_observed`，且始终声明 `content_observed=false`。

### 9.4 网络、身份和内核行为

网络事件：

- TCP/UDP/Unix socket connect、bind、listen、accept。
- 源/目的地址端口、Unix socket 规范化路径、结果/errno。
- DNS query/answer 只在能力允许时采集并限流；反向解析结果不能冒充真实访问域名。
- 不采集 payload，不做 TLS 解密。

身份与内核事件：

- setuid/setgid/capset 前后身份。
- ptrace 目标进程身份。
- bpf、perf_event_open、module load 请求摘要。
- /dev、/proc、/sys 关键资源访问由文件资源分类补充。

### 9.5 规范化、脱敏和可观测性

每个事件在离开主机前必须完成：

1. instance/session/execution unit/process 归属。
2. category、operation、resource、outcome 规范化。
3. argv、URL、路径参数中的 token/password/credential 脱敏。
4. 字段限长，并记录 `truncated_fields`。
5. source/sensor/visibility/drop counter 写入 `collection`。

环境变量值、文件内容、网络内容和完整工具输出不得进入 ringbuf 上报结构或用户态事件。

### 9.6 聚合与优先级

```text
key = instance_slot + session + unit_slot + pid_epoch + category + operation + normalized resource
```

- 普通 file read/write、重复失败 open 和重复网络事件默认 1～3 秒短窗口聚合。
- fork/exec/exit、状态变化、deny、escape violation 和 finding 证据不采样丢弃，只允许合并 hit count。
- 首次、最近时间和命中次数写入 event evidence。
- 本地 spool 紧张时按 `critical_evidence > state_change > process/network > repetitive_io` 保留。
- 发生丢弃必须上报 category/sensor/reason 计数，覆盖状态转为 `degraded`。

### 9.7 工具调用关联

Adapter 可以接收产品 Native Hook 的可信工具事件，将：

```text
tool_call -> process exec -> file/network operations
```

关联为同一真实 session。`tool_call_started/completed/failed` 由 api-server 消费并匹配
AGB-BUILTIN-004；Agent 只通过 PID/start_ticks/时间/correlation 解析实际进程。若没有
可信 Hook，只上报 OS 行为并标记 `tool_semantics_unobservable`；不得根据进程名伪造 tool name。

## 10. 隔离逃逸监控

### 10.1 行为 Hook

基础监控：

- `sys_enter_setns`
- `sys_enter_unshare`
- `sys_enter_clone/clone3`
- `sys_enter_mount/umount2`
- `sys_enter_pivot_root/chroot`
- `sys_enter_open_tree/move_mount`
- `sys_enter_ptrace`
- `sys_enter_bpf`
- `sys_enter_perf_event_open`
- `sys_enter_init_module/finit_module`

完整阻断优先采用：

- `lsm/task_setns` 或内核可用的等价安全 Hook。
- `lsm/sb_mount`
- `lsm/ptrace_access_check`
- `lsm/bpf`
- `lsm/kernel_module_request`
- `lsm/task_fix_setuid`
- 文件/路径 LSM Hook 保护 `/proc/*/ns`、cgroupfs 和 runtime socket。

具体 Hook 名称以目标内核 BTF 和 libbpf 支持为准；构建时不能假设所有内核都有相同 LSM Hook。

### 10.2 行为与状态双重验证

syscall attempt 只说明尝试，不一定成功。Agent 同时进行：

- syscall/LSM 行为记录。
- syscall 返回值（可用时）。
- 后续 `/proc` namespace/cgroup/capability 状态校验。

事件证据：

```json
{
  "rule": "join_external_namespace",
  "syscall": "setns",
  "target_namespace_inode": 4026531840,
  "baseline_namespace_inode": 4026532901,
  "return_code": -1,
  "state_changed": false
}
```

### 10.3 Profile 允许范围

不能全局禁止 `unshare/clone3`。构建工具可能在沙箱内部创建更深层 namespace。

每个 Profile 声明：

- 允许创建的 namespace 类型。
- 是否允许内部 mount。
- mount 目标范围。
- 允许 capability。
- 允许访问的 runtime socket。
- 是否允许创建嵌套容器。

未声明行为按策略处理，默认 audit/alert，不默认 freeze。

### 10.4 关键高危规则

第一批可配置规则：

| 规则 | 典型证据 | 推荐默认动作 |
| --- | --- | --- |
| `join_external_namespace` | setns 目标不属于执行单元 | deny_and_freeze |
| `leave_expected_cgroup` | 成员 cgroup 与 baseline 不一致 | alert/deny_and_freeze |
| `access_container_runtime_socket` | docker/containerd/CRI socket | deny_and_freeze |
| `access_host_proc_root` | `/proc/1/root` 或外部 pid root | deny |
| `write_cgroupfs` | 修改 cgroup 配置/迁移进程 | deny |
| `mount_host_sensitive_path` | 新增宿主机敏感 bind mount | deny_and_freeze |
| `ptrace_external_process` | ptrace 非本 unit 进程 | deny |
| `ptrace_aegis_agent` | ptrace Aegis Agent | deny_and_freeze |
| `load_bpf_or_module` | bpf/module load | deny |
| `capability_escalation` | capability 超出 baseline/profile | deny_and_freeze |

## 11. 本地决策引擎

```go
type Decision string

const (
    DecisionAllow                  Decision = "allow"
    DecisionAudit                  Decision = "audit"
    DecisionAlert                  Decision = "alert"
    DecisionDeny                   Decision = "deny"
    DecisionDenyAndFreeze          Decision = "deny_and_freeze"
    DecisionWouldDeny              Decision = "would_deny"
    DecisionEnforcementUnavailable Decision = "enforcement_unavailable"
)
```

决策输入：

```go
type DecisionContext struct {
    Subject       GuardSubject
    Instance      RuntimeInstance
    Unit          ExecutionUnit
    Event         GuardEvent
    Policy        CompiledPolicy
    Profile       AdapterProfile
    Capabilities  GuardCapabilities
}
```

决策顺序：

1. 排除未归属进程和 protected target。
2. 找到实例和执行单元。
3. 按 policy priority 匹配 host/agent/path/escape rule。
4. 应用 Profile allow 约束。
5. 根据 capability 将无法执行的 deny 转为显式 degraded decision。
6. 生成不可变 decision evidence。
7. BPF LSM 返回 allow/EPERM。
8. `deny_and_freeze` 异步触发本地 freezer，但 deny 本身不能等待 freezer。

## 12. 暂停、恢复和终止

动作解析必须从单一 `execution_unit_id` 或 `instance_id` 得到目标 slot，
不得接受 host 级“全部 Agent”目标。freezer 在执行前再次校验 unit/instance、
PID/start_ticks 和 cgroup 身份；同机其他 instance slot 不进入目标 PID/cgroup
集合。

### 12.1 OCI/cgroup

优先写执行单元 cgroup v2：

```text
<cgroup>/cgroup.freeze = 1
```

确认：

```text
<cgroup>/cgroup.events contains frozen 1
```

恢复写 `0` 并确认 `frozen 0`。

### 12.2 托管本地实例

如果未来由 Aegis wrapper 启动 Agent，可创建专属 cgroup 并将控制进程/worker 纳入。V6.2 不强制第三方 Agent 必须由 Aegis 启动。

### 12.3 非托管本地实例

fallback：

1. 枚举当前 unit 全部 PID。
2. 使用 pidfd 确认进程身份。
3. 对进程树发送 `SIGSTOP`。
4. 再次扫描补停在操作期间新增的子进程。
5. 标记 `freeze_fallback`，不能声称与 cgroup freeze 同等可靠。

恢复使用 `SIGCONT`，只作用于当次保存的 pidfd/identity，防止 PID 重用。

### 12.4 Freeze 安全超时

策略包含：

```text
freeze_timeout_seconds
```

默认建议 300 秒，范围 30～900 秒。超时后：

- deny 内核规则继续生效。
- 如果服务端没有确认保持冻结，自动恢复执行单元。
- 生成 `freeze_expired_auto_resume` action event。

管理员可以在前端选择“保持冻结”，该操作必须审计。

### 12.5 Protected targets

至少保护：

- Aegis Agent 自身及其父 systemd unit。
- PID 1/systemd/init。
- kernel threads。
- dockerd/containerd/kubelet 默认不允许由普通 Agent Guard 事件整体 kill。
- PostgreSQL/Redis/Kafka 等 Aegis 控制面服务在同主机部署时按配置保护。

访问 runtime socket 的 Agent worker 可以被冻结，但不能误冻结宿主机 dockerd cgroup。

## 13. 配置与本地持久化

目录：

```text
/var/lib/aegis/agent-guard/
├── bundle.json
├── bundle.digest
├── profile-cache.json
├── state.json
└── actions/
```

要求：

- 使用临时文件 + fsync + rename 原子替换。
- 权限仅允许 root/Aegis Agent 读取。
- bundle 必须包含 version、digest、generated_at。
- 新 bundle 校验失败保留旧 bundle。
- 服务端发布空策略必须是显式合法 bundle，不能用缺失配置代表清空。
- 日志只记录策略 ID/version/digest，不记录命令正文、文件/网络内容或凭据。

## 14. 事件结构

### 14.1 统一行为事件

```json
{
  "schema": "aegis.agent_behavior.v1",
  "event_id": "uuid",
  "event_type": "agent_behavior",
  "host_id": "uuid",
  "host_boot_id": "uuid",
  "agent_sequence": 918273,
  "agent": {
    "type": "codex",
    "instance_id": "uuid",
    "asset_id": "uuid",
    "profile_key": "codex-linux",
    "profile_version": 1
  },
  "session": {
    "id": "uuid",
    "source": "execution_unit",
    "confidence": "inferred"
  },
  "execution_unit": {
    "id": "uuid",
    "type": "linux_namespace",
    "container_id": "",
    "cgroup_id": "12345"
  },
  "category": "file",
  "operation": "read",
  "outcome": "success",
  "actor": {
    "pid": 1234,
    "ppid": 1220,
    "start_time_ticks": 991827,
    "name": "python3",
    "exe": "/usr/bin/python3",
    "argv": ["python3", "script.py"],
    "cwd": "/workspace",
    "chain": []
  },
  "resource": {
    "type": "file",
    "identity": "/workspace/config.yaml",
    "classification": "configuration",
    "attributes": {
      "raw_path": "./config.yaml",
      "resolved_path": "/workspace/config.yaml",
      "device": 2049,
      "inode": 123,
      "content_observed": false
    }
  },
  "policy": {
    "id": "uuid",
    "version": 7,
    "rule_id": ""
  },
  "decision": "audit",
  "severity": "info",
  "collection": {
    "source": "ebpf",
    "sensor": "file_open",
    "visibility": "complete",
    "truncated_fields": [],
    "lost_events_since_last": 0
  },
  "occurred_at": "2026-07-30T10:00:00Z",
  "occurred_monotonic_ns": 99182700123
}
```

命令、网络和身份事件使用同一 envelope，只替换 `category`、`operation` 和 `resource`。完整字段约束见行为采集与分析设计。

### 14.2 逃逸专项事件

```json
{
  "schema": "aegis.agent_behavior.v1",
  "event_type": "agent_sandbox_violation",
  "agent": {},
  "execution_unit": {},
  "process": {},
  "violation": {
    "rule": "join_external_namespace",
    "operation": "setns",
    "target": "/proc/1/ns/mnt",
    "baseline": {},
    "actual": {},
    "state_changed": false,
    "return_code": -1
  },
  "decision": "deny_and_freeze",
  "evidence_event_ids": ["attempt-event-id", "drift-event-id"],
  "action": {
    "action_id": "uuid",
    "type": "freeze_execution_unit",
    "status": "success"
  }
}
```

## 15. 日志与指标

### 15.1 日志

必须记录：

- Agent Guard 初始化能力和覆盖等级。
- Profile/bundle 版本应用成功或失败。
- 实例/执行单元创建、状态变化和结束。
- BPF program/map 加载失败。
- 行为传感器、脱敏器、聚合器和本地 spool 降级。
- 高危 decision 和 freeze/resume/kill 结果。
- reconciler 修复数量和异常漂移。
- ringbuf/perf 丢失计数。

禁止记录：

- 文件内容、网络内容和 stdin/stdout/stderr。
- 环境变量值。
- token/password/hash。
- 未脱敏的完整远程凭据或 URL query。

### 15.2 指标

```text
aegis_agent_guard_instances
aegis_agent_guard_execution_units{type,status}
aegis_agent_guard_guarded_pids
aegis_agent_guard_guarded_cgroups
aegis_agent_behavior_events_total{category,operation,outcome}
aegis_agent_behavior_aggregated_total{category,operation}
aegis_agent_behavior_redactions_total{field,rule}
aegis_agent_behavior_visibility_total{visibility,reason}
aegis_agent_guard_denies_total{rule}
aegis_agent_guard_freezes_total{result}
aegis_agent_guard_event_drops_total{transport}
aegis_agent_guard_policy_version
aegis_agent_guard_reconcile_repairs_total{kind}
aegis_agent_guard_hook_latency_seconds
```

## 16. Agent 测试设计

### 16.1 单元测试

- Profile 组合证据匹配和误匹配。
- PID start time 防重用。
- fork 标签继承、exec 保持、exit 清理。
- 同机多 Agent 和同类型多 controller 分配独立 instance slot。
- Agent A 启动 Agent B 后切换主归属并保留 launched-by 证据。
- ambiguous 归属不复制事件且不触发自动 freeze。
- Native Hook 真实 session/correlation token 关联；无可信 session ID 时进入
  unattributed 索引，不创建可选的 inferred 官方 session（旧数据兼容除外）。
- cgroup v1/v2/containerd/Podman ID 解析。
- exact/prefix/glob 策略编译。
- 相对路径、cwd、dirfd、container root 解析。
- 命令 argv/URL/path secret 脱敏、截断和 visibility。
- file/network 高频事件聚合和高危证据优先级。
- 五个内置 rule key/version 加载、参数校验和不支持版本降级。
- 外部地址分类、DNS evidence source 和 trusted CIDR。
- 通用/自保护 executable 的 path/basename/argv 边界匹配；Agent Guard
  `AGB-BUILTIN-004` 工具命令匹配由 api-server 完成。
- privilege attempt/succeeded/inconclusive 和 container user namespace。
- Profile allow 与 policy deny 优先级。
- capability 降级产生 `would_deny`。
- protected target 不可 freeze/kill。
- bundle 原子替换和 last-known-good。

### 16.2 eBPF 测试

- 非 Agent 进程不进入 Agent Behavior 事件。
- Agent 子进程 fork/exec/exit 和 argv/cwd/exit result 正确。
- Agent 子进程 read/write/create/delete/rename/chmod 正确分类。
- connect/listen/Unix socket 和 setuid/capset/ptrace 正确分类。
- 敏感目录、外链、文件生成、敏感命令和提权分别产生完整必需证据。
- cgroup 成员无需 PPID 也能归属。
- LSM deny 返回 `EACCES/EPERM` 且文件状态不变。
- `setns`、mount、runtime socket、cgroupfs、ptrace 规则命中。
- ringbuf/perf 两种 transport。
- BPF LSM 不可用时加载 monitor program 并报告降级。

### 16.3 主机测试

- Codex → shell → Python 多层进程形成命令/文件/网络操作链。
- 下载测试文件 → chmod → execute 能提供完整原子事件，但 Agent 本地不擅自生成 AI 结论。
- Codex namespace worker 尝试加入测试 namespace。
- OpenClaw/Hermes Docker 容器访问只用于测试的 runtime socket。
- cgroup freeze 后所有 unit 进程停止，resume 后恢复。
- freeze 一个 execution unit 时，同机其他 Agent/instance 继续运行。
- PID reuse 后不会对新进程执行旧 action。
- Aegis Agent 自身不会被目标 Profile 归属或冻结。

测试只能使用临时 namespace、临时 cgroup、临时文件、测试网络端点和专用测试容器，不得访问真实生产敏感资源或外部未知地址。
