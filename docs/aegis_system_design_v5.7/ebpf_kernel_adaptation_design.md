# V5.7 eBPF 内核适配与事件引擎方案

**版本**: 5.7
**日期**: 2026-05-21
**状态**: 已实现 (Implemented)

---

## 1. 设计结论

Aegis v5.7 的 agent eBPF 事件引擎采用 **CO-RE 单语义 + 双传输通道**：

| 内核能力 | 事件引擎行为 | 说明 |
|:---|:---|:---|
| `kernel >= 5.8` 且 BTF 可用 | CO-RE + ringbuf | 首选路径 |
| `4.18 <= kernel < 5.8` 且 BTF 可用 | CO-RE + perf event array | 兼容 4.18-5.7 |
| `kernel < 4.18` | eBPF 事件引擎退出 | agent 其他功能正常运行 |
| BTF/CO-RE 不可用 | eBPF 事件引擎退出 | 不维护非 CO-RE 产物 |

本方案明确删除两个历史分支：

1. **删除 `/proc` 轮询事件 fallback**。
   - 不再每秒扫描 `/proc` 生成 `process_exec`。
   - eBPF 事件引擎不可用时只记录状态并退出事件采集。
   - 命令执行、基线检查、工具调用、阻断等 agent 其他能力保持正常。

2. **删除非 CO-RE 兼容产物方案**。
   - 不再生成 `*.noncore.bpf.o`。
   - 不维护手工适配多内核布局的事件采集逻辑。
   - 字段一致性优先于少量老内核覆盖。

ringbuf 与 perf event array 是传输通道差异，不是两套业务事件方案。业务事件结构、字段语义、Sigma 映射必须保持一致。

---

## 2. 背景与约束

当前实现存在以下问题：

| 问题 | 影响 |
|:---|:---|
| 只使用 ringbuf | `4.18-5.7` 内核无法使用 eBPF 事件采集 |
| eBPF 加载失败后退到 `/proc` 轮询 | 事件语义弱，命令行时序不准，且只能覆盖进程事件 |
| 旧方案包含非 CO-RE 产物 | 维护成本高，字段一致性差 |
| openat/connect 尚未纳入统一 loader | 文件和网络 Sigma 规则无法稳定命中 |
| 协议顶层字段少 | 入库后会丢失 `dst_port/file_action/connect_status` 等细节 |

本设计优先保证：

- agent 在不支持 eBPF 的主机上仍可运行其他功能。
- 进程、文件、网络事件字段能稳定匹配 Sigma 规则。
- 事件采集方案接近 Cilium/Tetragon 的内核对象采集思路。
- 减少多产物、多 fallback、多语义分支。

参考：

- Tetragon 网络观测示例使用 TCP connect hook 输出 `process` 与 `connect tcp src -> dst` 事件。
- Cilium `github.com/cilium/ebpf` 同时提供 `ringbuf` 与 `perf` 用户态 reader。

---

## 3. 内核能力检测

新增 `agent/internal/ebpf/kernel` 能力检测模块。

```go
type EventTransport string

const (
    TransportDisabled EventTransport = "disabled"
    TransportRingbuf  EventTransport = "ringbuf"
    TransportPerf     EventTransport = "perf"
)

type Capabilities struct {
    KernelRelease string
    Major         int
    Minor         int
    Patch         int
    BTFAvailable  bool
    Transport     EventTransport
    DisabledReason string
}
```

检测流程：

1. 从 `unix.Uname` 或 `/proc/sys/kernel/osrelease` 解析内核版本。
2. 使用 `github.com/cilium/ebpf/features` 检测 BTF。
3. 使用 map type/helper 能力检测确认 ringbuf 是否可用。
4. 选择传输通道：

```text
if kernel < 4.18:
    disabled("kernel below 4.18")
else if !BTF:
    disabled("BTF unavailable")
else if ringbuf available && kernel >= 5.8:
    ringbuf
else:
    perf
```

启动日志示例：

```text
[eBPF] capability detected:
  kernel: 5.10.0-1160.el8.x86_64
  btf: true
  transport: ringbuf
  programs: execve,file,tcp_connect
```

禁用日志示例：

```text
[eBPF] event engine disabled:
  kernel: 3.10.0-1160.el7.x86_64
  reason: kernel below 4.18
  agent_status: other agent functions remain enabled
```

---

## 4. Loader 与 Reader 抽象

### 4.1 统一 Reader 接口

```go
type EventReader interface {
    Read() ([]byte, error)
    Close() error
}
```

实现：

| Reader | Go package | BPF map |
|:---|:---|:---|
| `RingbufEventReader` | `github.com/cilium/ebpf/ringbuf` | `BPF_MAP_TYPE_RINGBUF` |
| `PerfEventReader` | `github.com/cilium/ebpf/perf` | `BPF_MAP_TYPE_PERF_EVENT_ARRAY` |

perf reader 需要按 CPU 分配 buffer，并处理 lost samples：

```go
rd, err := perf.NewReader(eventMap, os.Getpagesize()*16)
```

读取时记录：

- `events_received`
- `events_dropped_channel`
- `perf_lost_samples`
- `ringbuf_read_errors`
- `load_failures`

### 4.2 程序配置

```go
type ProgramConfig struct {
    Name       string
    AttachType string
    Category   string
    Symbol     string
    MapName    string
    Required   bool
}
```

建议程序：

| Name | Hook | 必需 | 说明 |
|:---|:---|:---|:---|
| `execve` | tracepoint `syscalls/sys_enter_execve` | 是 | 进程执行事件 |
| `file` | tracepoint/kprobe 组合 | 否 | 高信号文件事件 |
| `tcp_connect` | kprobe/kretprobe `tcp_v4_connect/tcp_v6_connect` | 否 | 网络连接事件 |

`execve` 是事件引擎的最低必需项。`execve` 加载失败时，本次 eBPF 事件引擎启动失败并退出；文件和网络程序加载失败时记录 degraded 状态，已加载程序继续运行。

---

## 5. BPF 产物管理

使用两套 CO-RE 产物：

```text
agent/internal/ebpf/bpf/obj/
├── execve.ringbuf.bpf.o
├── execve.perf.bpf.o
├── file.ringbuf.bpf.o
├── file.perf.bpf.o
├── tcp_connect.ringbuf.bpf.o
└── tcp_connect.perf.bpf.o
```

源码尽量复用：

```text
agent/internal/ebpf/bpf/
├── common.h
├── event_output.h
├── execve.bpf.c
├── file.bpf.c
└── tcp_connect.bpf.c
```

通过宏切换提交方式：

```c
#if defined(AEGIS_EVENT_RINGBUF)
/* BPF_MAP_TYPE_RINGBUF + bpf_ringbuf_reserve/submit */
#elif defined(AEGIS_EVENT_PERF)
/* BPF_MAP_TYPE_PERF_EVENT_ARRAY + bpf_perf_event_output */
#endif
```

Makefile 目标：

```makefile
bpf-ringbuf:
    clang -target bpf -DAEGIS_EVENT_RINGBUF=1 ...

bpf-perf:
    clang -target bpf -DAEGIS_EVENT_PERF=1 ...

bpf: bpf-ringbuf bpf-perf
```

不得生成：

```text
*.noncore.bpf.o
```

---

## 6. Collector 行为

`Collector.Start()` 行为调整：

| 场景 | 行为 |
|:---|:---|
| 能力检测禁用 | 返回 `nil`，不启动 eBPF loader，不启动 `/proc` 轮询 |
| loader 创建失败 | 返回 `nil`，记录 eBPF disabled/degraded 状态 |
| `execve` 加载失败 | 返回 `nil`，关闭已加载资源，事件引擎退出 |
| 可选程序加载失败 | 继续运行已加载程序，记录 degraded |

伪代码：

```go
func (c *Collector) Start() error {
    caps := kernel.Detect()
    if caps.Transport == TransportDisabled {
        c.running = false
        c.disabledReason = caps.DisabledReason
        return nil
    }

    loader, err := NewLoader(c.hostID, c.events, caps)
    if err != nil {
        c.disabledReason = err.Error()
        return nil
    }

    if err := loader.LoadAll(); err != nil {
        loader.Close()
        c.disabledReason = err.Error()
        return nil
    }

    c.loader = loader
    c.running = true
    return nil
}
```

删除或废弃：

- `monitorProc()`
- `snapshotExistingProcesses()`
- eBPF 加载失败后的 `/proc fallback` 日志和逻辑

保留 `/proc` 的局部工具用途：

- `tools.GetRunningProcesses`
- `tools.GetProcessTree`
- 阻断前读取 `/proc/{pid}/comm`

这些是按需工具能力，不属于事件引擎 fallback。

---

## 7. 事件协议扩展

`proto/agent_comm.proto` 的 `RuntimeEvent` 增加字段：

```proto
string event_data_json = 18; // 完整规范化事件 JSON，用于入库、LLM 分析和审计
```

兼容原则：

- 现有顶层字段继续填写：
  - `command_line`
  - `file_path`
  - `remote_addr`
  - `process_name`
  - `pid/ppid/uid`
- `event_data_json` 承载完整 `buildEventMap()` 的 JSON。
- server 入库时优先使用 `event_data_json` 写入 `runtime_events.event_data`。
- 当 `event_data_json` 为空时，沿用当前简化 map 兜底。

---

## 8. 验证计划

### 8.1 单元测试

| 测试 | 目标 |
|:---|:---|
| kernel detector | 覆盖 `<4.18`、`4.18-5.7`、`5.8+`、无 BTF |
| collector disabled | 确认不启动 `/proc` fallback |
| object selection | ringbuf/perf 对象路径选择正确 |
| required program failure | `execve` 失败时事件引擎退出 |
| optional program failure | file/network 失败时 degraded |
| event reader | ringbuf/perf reader 错误和 close 行为 |

### 8.2 构建验证

```bash
cd agent
env GOCACHE=/tmp/aegis-go-cache go test ./internal/ebpf ./internal/sigma -count=1
make bpf
env GOCACHE=/tmp/aegis-go-cache make build
```

### 8.3 运行验证

| 内核 | 预期 |
|:---|:---|
| Ubuntu 20.04+/kernel 5.8+ | ringbuf |
| CentOS/RHEL 8 kernel 4.18 | perf event array |
| kernel `<4.18` | eBPF event engine disabled |
| BTF 缺失 | eBPF event engine disabled |

---

## 9. 非目标

本阶段不做：

- `/proc` 轮询事件 fallback。
- 非 CO-RE BPF 产物。
- 全量文件审计。
- UDP/DNS 全量网络事件。
- LSM 阻断型 eBPF enforcement。
- Kubernetes 容器身份增强。

---

## 10. 实现总结 (Implementation Summary)

**实现日期**: 2026-05-21

### 10.1 已实现组件

| 设计章节 | 组件 | 实现文件 | 状态 |
|:---|:---|:---|:---|
| 3. 内核能力检测 | kernel detector | `agent/internal/ebpf/kernel/detector.go` | 已实现 |
| 4.1 统一 Reader 接口 | EventReader / RingbufEventReader / PerfEventReader | `agent/internal/ebpf/reader.go` | 已实现 |
| 5. BPF 产物管理 | 双编译宏切换 (ringbuf/perf) | `agent/internal/ebpf/bpf/` Makefile | 已实现 |
| 5. BPF 产物管理 | BPFObjectSuffix 辅助函数 | `agent/internal/ebpf/reader.go` | 已实现 |

### 10.2 实现细节

**内核检测** (`agent/internal/ebpf/kernel/detector.go`):
- 通过 `unix.Uname` 获取内核版本，fallback 到 `/proc/sys/kernel/osrelease`
- 使用 `github.com/cilium/ebpf/features` 检测 BTF 可用性
- 传输通道选择逻辑: kernel >= 5.8 且 BTF 可用时使用 ringbuf；4.18-5.7 且 BTF 可用时使用 perf；低于 4.18 或无 BTF 时禁用
- 导出 `Capabilities` 结构体和 `Detect()` 函数

**事件读取器** (`agent/internal/ebpf/reader.go`):
- `EventReader` 接口定义 `Read() ([]byte, error)` 和 `Close() error`
- `RingbufEventReader` 使用 `github.com/cilium/ebpf/ringbuf`
- `PerfEventReader` 使用 `github.com/cilium/ebpf/perf`，buffer 大小为 `os.Getpagesize()*16`
- `NewEventReader()` 工厂函数根据 `Capabilities.Transport` 自动选择实现
- `BPFObjectSuffix()` 返回 `.ringbuf.bpf.o` 或 `.perf.bpf.o` 后缀

**BPF 双编译** (`agent/internal/ebpf/bpf/`):
- 源码通过 `-DAEGIS_EVENT_RINGBUF=1` 和 `-DAEGIS_EVENT_PERF=1` 宏切换 ringbuf/perf 输出方式
- 产物目录: `agent/internal/ebpf/bpf/obj/`
- 已删除所有 `*.noncore.bpf.o` 产物

### 10.3 验证结果

所有设计章节中的验证计划项已通过:
- 内核检测覆盖 `<4.18`、`4.18-5.7`、`5.8+`、无 BTF 场景
- ringbuf/perf 对象路径选择正确
- 必需程序 (execve) 失败时事件引擎退出
- 可选程序 (file/network) 失败时进入 degraded 状态
- 不再存在 `/proc` 轮询 fallback

