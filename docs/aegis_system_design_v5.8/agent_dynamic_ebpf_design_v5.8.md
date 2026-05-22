# V5.8 Agent 动态 eBPF 设计

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. 目标

agent 支持动态 DetectionPackage：

- 收到全局 hook allowlist 后才允许加载动态 eBPF。
- 下载完整签名 package。
- 使用内置 Ed25519 公钥验签整个 tar.gz。
- 解包后根据本机能力选择 ringbuf 或 perf artifact。
- 加载插件事件，解析统一事件信封 + TLV payload。
- 将插件事件转成 event map，复用 Sigma atomic rules。
- Sigma 命中后生成 AtomicFinding。
- 本地 Correlation Engine 做 ordered sequence + window + by。
- 命中后上报 correlation alert + evidence。

---

## 2. 新增模块

建议目录：

```text
agent/internal/dynpkg/
├── manager.go          # package 生命周期
├── verifier.go         # Ed25519 验签
├── installer.go        # 下载、解包、校验、安装事务
├── manifest.go         # package.yaml/plugin.yaml 解析
├── allowlist.go        # hook allowlist 校验
├── state.go            # 本地状态和状态上报
└── storage.go          # /var/lib/aegis/detection-packages

agent/internal/ebpf/plugin/
├── loader.go           # 动态 .bpf.o 加载与 attach
├── event.go            # aegis_plugin_event 解析
├── tlv.go              # TLV payload 解码
└── schema.go           # event_schema 解析

agent/internal/correlation/
├── engine.go
├── spec.go
├── finding.go
├── process_context.go
└── cache.go
```

---

## 3. 启动流程

```text
agent 启动
  -> 建立 command stream
  -> 上报 online/capabilities
  -> 等待 ConfigSync(dynamic_ebpf_hook_allowlist)
  -> 收到 allowlist 后保存到内存
  -> 如果已有 enabled package 指令，则安装/加载
```

约束：

- 未收到 allowlist 前，不加载任何动态 eBPF package。
- 内置基础 eBPF 引擎不受动态 allowlist 影响。
- 动态 package 加载失败只影响该 package。

---

## 4. 本地目录

```text
/var/lib/aegis/detection-packages/
  cve-2026-31431-copyfail/
    1.0.0/
      package.yaml
      plugin/
      rules/
      correlations/
    active -> 1.0.0
```

卸载时删除对应 package 目录。

---

## 5. 签名校验

agent 编译时内置公钥：

```go
var detectionPackagePublicKey = []byte{...}
```

流程：

```text
download package.tar.gz
download package.tar.gz.sig
ed25519.Verify(publicKey, packageBytes, sigBytes)
失败则拒绝安装
成功后才允许解包
```

第一版不支持企业额外公钥。

---

## 6. Hook allowlist

ConfigSync 类型：

```text
dynamic_ebpf_hook_allowlist
```

agent 使用远端配置：

```go
type HookAllowlist struct {
    Version     int64    `json:"version"`
    Tracepoints []string `json:"tracepoints"`
    Kprobes      []string `json:"kprobes"`
    LSM          []string `json:"lsm"`
    XDP          []string `json:"xdp"`
    TC           []string `json:"tc"`
}
```

校验：

```text
plugin manifest 中每个 hook 必须在 allowlist 中
不满足则 package status = blocked_by_hook_allowlist
```

allowlist 更新后：

```text
agent 重新评估 active packages
不满足新 allowlist 的 package 自动停用
```

---

## 7. 动态插件加载

### 7.1 Artifact 选择

每个 package 都包含：

```text
*.ringbuf.bpf.o
*.perf.bpf.o
```

加载策略：

```text
if ringbuf available:
    try ringbuf
    if failed:
        try perf
else:
    try perf

if both failed:
    package status = load_failed
```

### 7.2 Attach

只根据 `plugin.yaml` 中 hook 列表 attach：

```yaml
hooks:
  - attach_type: tracepoint
    attach: syscalls/sys_enter_socket
    program: trace_af_alg_socket
```

tracepoint attach：

```go
link.Tracepoint(category, symbol, prog, nil)
```

kprobe/lsm/xdp/tc 第一版即使页面可配置，也建议 agent 代码先仅实现 tracepoint；其他类型返回 unsupported，后续版本扩展。

---

## 8. 统一插件事件

所有插件输出统一信封：

```c
#define AEGIS_PLUGIN_PAYLOAD_MAX 256

struct aegis_plugin_event {
    __u64 timestamp_ns;
    __u32 plugin_id_hash;
    __u32 event_type;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __u32 payload_len;
    __u8  payload[AEGIS_PLUGIN_PAYLOAD_MAX];
};
```

payload 使用 TLV：

```c
struct aegis_tlv {
    __u16 field_id;
    __u8  field_type;
    __u8  length;
    __u8  value[];
};
```

字段类型：

```text
1 = string
2 = int32
3 = uint32
4 = int64
5 = uint64
6 = bool
7 = bytes
```

agent 解码：

```text
plugin_id + event_type -> plugin manifest event_schema -> field_id -> name/type
```

---

## 9. Event Map

插件事件解析后转换为 Sigma event map：

```go
map[string]any{
    "category": "kernel_plugin",
    "event_type": "af_alg_bind",
    "package_id": "cve-2026-31431-copyfail",
    "plugin_id": "copyfail_probe",
    "pid": 1234,
    "uid": 1000,
    "alg_type": "aead",
    "alg_name": "gcm(aes)",
}
```

必须补齐：

- host_id
- hostname
- timestamp
- process context
- package_id
- plugin_id

---

## 10. Sigma AtomicFinding

当前 pipeline 是“事件 -> Sigma MatchAll -> RuntimeEvent”。V5.8 对动态插件增加分支：

```text
plugin event -> event map -> package scoped Sigma -> AtomicFinding -> Correlation
```

AtomicFinding：

```go
type AtomicFinding struct {
    PackageID    string
    PackageVer   string
    RuleID       string
    EventType    string
    Timestamp    int64
    HostID       string
    Hostname     string
    PID          int
    PPID         int
    UID          int
    Process      ProcessContext
    EventMap     map[string]any
}
```

---

## 11. ProcessContext

所有进入 correlation 的 finding 必须固化进程上下文：

```go
type ProcessContext struct {
    PID          int      `json:"pid"`
    PPID         int      `json:"ppid"`
    UID          int      `json:"uid"`
    ProcessName  string   `json:"process_name"`
    CommandLine  string   `json:"command_line"`
    AncestorPIDs []int    `json:"ancestor_pids"`
    TreeKey      string   `json:"tree_key"`
}
```

`by: pid_tree` 使用 `TreeKey`，不能等 correlation 时再临时查 `/proc`。

第一版 TreeKey：

```text
host_id + nearest stable ancestor pid
```

如果无法解析祖先：

```text
fallback = host_id + pid
```

---

## 12. Correlation Engine

支持语义：

```yaml
correlation:
  by: pid_tree
  window: 10s
  ordered: true
  sequence:
    - rule_id: a
    - rule_id: b
    - rule_id: c
```

缓存限制：

```go
type CorrelationLimits struct {
    DefaultWindow        time.Duration // 10s
    MaxWindow            time.Duration // 60s
    MaxEventsPerKey      int           // 128
    MaxEventsGlobal      int           // 10000
}
```

命中后输出：

```go
type CorrelationAlert struct {
    PackageID     string
    PackageVer    string
    RuleID        string
    Title         string
    Severity      string
    MITREID       string
    CVEID         string
    Findings      []AtomicFinding
}
```

---

## 13. 限速和自动禁用

默认：

```text
per plugin: 1000 events/s
per event_type: 500 events/s
per pid: 100 events/s
```

持续超限：

```text
drop plugin events
increment metrics
if sustained_overflow:
    disable package
    report disabled_by_rate
```

---

## 14. 状态上报

状态变化时上报：

- install started
- signature failed
- blocked by allowlist
- active
- degraded
- load failed
- disabled by policy
- disabled by rate
- uninstalled

上报内容：

```json
{
  "package_id": "cve-2026-31431-copyfail",
  "version": "1.0.0",
  "status": "active",
  "active_artifact": "ringbuf",
  "loaded_hooks": ["syscalls/sys_enter_socket"],
  "kernel_release": "5.10.0",
  "arch": "amd64"
}
```

---

## 15. 卸载

```text
stop correlation
remove package scoped Sigma rules
close event readers
detach links
close eBPF collections/maps
delete local package directory
report uninstalled
```

---

## 16. 兼容当前 Agent

当前 agent 已具备：

- `ConfigSync`
- `RuleUpdate`
- `ReportEvents`
- ringbuf/perf reader 抽象
- Sigma loader
- eBPF loader

V5.8 在此基础上新增：

- dynamic package manager
- plugin loader
- TLV decoder
- package scoped Sigma loader
- local correlation engine
- package status reporter

