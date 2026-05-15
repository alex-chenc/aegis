# V5.7 eBPF文件事件与网络事件采集设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 现状分析

| 程序 | C代码 | 编译 | Go加载 | Pipeline映射 |
|:---|:---|:---|:---|:---|
| execve | 完成 | 完成 | 完成 | process_creation |
| fork | 完成 | 完成 | 完成 | process_creation |
| **openat** | **完成** | **完成** | **未接入** | **已预留** file_event |
| **connect** | **完成** | **完成** | **未接入** | **已预留** network_connection |

**关键发现**: openat和connect的C代码和编译产物已就绪，pipeline已预留映射，只需接入Go层。

### 1.1 C代码缺陷

- `connect.bpf.c`: 仅IPv4，仅目标地址/端口，无源地址，无协议标识
- `openat.bpf.c`: 相对完整，flags解析可增强
- `execve.bpf.c`（2026-05-15补充）: 当前仅读取 `argv[1]`，且 Go 侧曾在 syscall entry 阶段优先读取 `/proc/{pid}/cmdline`，会拿到旧进程 cmdline，导致 `process_exec` 命令行误报为 `-bash` 等旧值。

### 1.2 process_exec 参数模型修正（2026-05-15）

参考 Cilium/Tetragon 的 `process_exec` 事件模型，Aegis v5.7 将执行文件与参数拆分表达：

| 字段 | 来源优先级 | 语义 |
|:---|:---|:---|
| `FilePath` / `image` / `exe` | execve `filename` → `/proc/{pid}/exe` | 执行文件路径 |
| `ProcessName` | `filename` basename → `argv[0]` basename → `comm` | 进程短名称 |
| `Args` / `CommandLine` | eBPF `argv[0..N]` → `/proc/{pid}/cmdline` 兜底 | 执行参数与完整命令行 |

设计原则：

- eBPF 在 `sys_enter_execve` 使用 bounded loop 读取 `argv[0..N]`，写入固定长度 `NUL-separated` args buffer。
- Go 侧优先解码 eBPF argv，并用空格重建 `CommandLine`；`/proc/{pid}/cmdline` 仅在 eBPF `filename` 与 `argv` 都缺失时兜底。
- 不使用 `task comm` 代替 `arguments/cmdline`。`comm` 只作为 `ProcessName` 的最后兜底。
- `FilePath` 使用 execve `filename` 或 `/proc/{pid}/exe`，避免把参数字符串写入 `image/exe`。
- argv buffer 达到 512 字节或 20 个参数上限时设置 `args_truncated`，供后续 Sigma/AI 分析识别命令行可能不完整。
- `execve` eBPF 程序是 `process_exec` 的必需采集项；加载失败必须返回错误并触发 Collector fallback，不能只打日志后继续运行。

待执行验证：

| 验证项 | 验证内容 |
|:---|:---|
| argv完整性 | `argv[0]`、`argv[1]`、后续参数均能被 eBPF buffer 解码 |
| /proc时序 | 在交互 shell 中执行新命令，不应将旧 `/proc/{pid}/cmdline` 误报为 `-bash` |
| 字段分离 | `image/exe` 为执行文件路径，`commandline` 为 argv 重建命令行 |
| 兜底路径 | eBPF filename/argv 都为空时，才使用 `/proc/{pid}/cmdline` |
| 截断标记 | argv buffer 或参数个数达到上限时，Go 事件包含 `args_truncated=true` |
| 加载失败 | `execve` verifier/load 失败时，`LoadAll()` 返回错误，Collector 进入 fallback |

2026-05-15 已执行的窄验证：

- `env GOCACHE=/tmp/aegis-go-cache go test ./internal/ebpf -count=1`
- `make bpf`
- `env GOCACHE=/tmp/aegis-go-cache make build`

未执行本机 root/eBPF 加载验证；该步骤需要替换或启动 `/opt/aegis-agent` 并具备内核 eBPF 权限。本次修改不涉及 HTTP API，因此未执行 curl 接口验证。

---

## 2. 文件事件采集（openat）

### 2.1 Go事件结构体

```go
// FileEvent 文件访问事件 (events.go新增)
type FileEvent struct {
    PID      uint32
    UID      uint32
    Flags    int32    // openat flags
    Comm     [16]byte
    Filename [256]byte
}

const (
    O_RDONLY = 0x0000
    O_WRONLY = 0x0001
    O_RDWR   = 0x0002
    O_CREAT  = 0x0040
    O_TRUNC  = 0x0200
)
```

### 2.2 Loader扩展

```go
// loader.go LoadAll() 扩展
func (l *Loader) LoadAll() error {
    programs := map[string]string{
        "execve":  "tracepoint/syscalls/sys_enter_execve",
        "fork":    "tracepoint/sched/sched_process_fork",
        "openat":  "tracepoint/syscalls/sys_enter_openat",  // V5.7
        "connect": "tracepoint/syscalls/sys_enter_connect", // V5.7
    }
    for name, tp := range programs {
        if err := l.loadProgram(name, tp); err != nil {
            logger.Warn("eBPF加载失败", "program", name, "error", err)
            continue
        }
    }
    return nil
}

// processEvent() 扩展
func (l *Loader) processEvent(name string, raw []byte) {
    switch name {
    case "execve":  l.processExecEvent(raw)
    case "fork":    l.processForkEvent(raw)
    case "openat":  l.processFileEvent(raw)    // V5.7
    case "connect": l.processConnEvent(raw)    // V5.7
    }
}

func (l *Loader) processFileEvent(raw []byte) {
    var fe FileEvent
    binary.Read(bytes.NewReader(raw), binary.LittleEndian, &fe)
    l.sendEvent(Event{
        EventType:   "file_access",
        PID:         fe.PID,
        UID:         fe.UID,
        FilePath:    cstrToString(fe.Filename[:]),
        ProcessName: cstrToString(fe.Comm[:]),
        FileFlags:   parseOpenFlags(fe.Flags),
        Timestamp:   time.Now().UnixMicro(),
    })
}
```

### 2.3 Pipeline扩展

```go
// pipeline.go buildEventMap() 扩展
case "file_access":
    m["category"] = "file_event"
    m["file_path"] = event.FilePath
    m["file_flags"] = event.FileFlags
```

### 2.4 内核态过滤（性能关键）

openat是高频调用，必须在内核态过滤：

```c
// openat.bpf.c 增强
SEC("tracepoint/syscalls/sys_enter_openat")
int trace_openat(struct trace_event_raw_sys_enter *ctx) {
    char filename[256];
    bpf_probe_read_user_str(filename, sizeof(filename), (void *)ctx->args[1]);

    // 只捕获敏感路径
    if (!is_sensitive_path(filename))
        return 0;

    // ... 提交事件 ...
}

static __always_inline int is_sensitive_path(const char *path) {
    // /etc/, /root/, /var/, /tmp/ 前缀匹配
    if (path[0]=='/' && path[1]=='e' && path[2]=='t' && path[3]=='c' && path[4]=='/') return 1;
    if (path[0]=='/' && path[1]=='r' && path[2]=='o' && path[3]=='o' && path[4]=='t' && path[5]=='/') return 1;
    if (path[0]=='/' && path[1]=='v' && path[2]=='a' && path[3]=='r' && path[4]=='/') return 1;
    if (path[0]=='/' && path[1]=='t' && path[2]=='m' && path[3]=='p' && path[4]=='/') return 1;
    return 0;
}
```

### 2.5 用户态采样与去重

```go
// Agent config.toml
[ebpf]
file_event_sampling_rate = 10   # 10%采样
conn_event_sampling_rate = 100  # 全量

// 事件去重器
type FileEventDeduplicator struct {
    cache    map[string]time.Time // "pid:filepath"
    interval time.Duration        // 5秒窗口
}
```

---

## 3. 网络事件采集（connect）

### 3.1 C代码增强

```c
// connect.bpf.c 增强：IPv6支持 + 源地址
struct conn_event {
    u32 pid;
    u32 uid;
    char comm[16];
    u16 family;       // AF_INET(2) / AF_INET6(10)
    u8  saddr[16];
    u8  daddr[16];
    u16 sport;
    u16 dport;
};

SEC("tracepoint/syscalls/sys_enter_connect")
int trace_connect(struct trace_event_raw_sys_enter *ctx) {
    struct sockaddr *addr = (struct sockaddr *)ctx->args[1];
    u16 family = 0;
    bpf_probe_read_user(&family, sizeof(family), &addr->sa_family);

    // 过滤回环
    if (family == AF_INET) {
        struct sockaddr_in *in = (struct sockaddr_in *)addr;
        u32 daddr;
        bpf_probe_read_user(&daddr, 4, &in->sin_addr);
        if (daddr == bpf_htonl(0x7f000001)) return 0;
    } else if (family == AF_INET6) {
        // ::1 检测
    } else {
        return 0;
    }

    struct conn_event *evt = bpf_ringbuf_reserve(&conn_events, sizeof(*evt), 0);
    evt->pid = bpf_get_current_pid_tgid() >> 32;
    evt->uid = bpf_get_current_uid_gid();
    evt->family = family;
    bpf_get_current_comm(&evt->comm, sizeof(evt->comm));

    if (family == AF_INET) {
        struct sockaddr_in *in = (struct sockaddr_in *)addr;
        bpf_probe_read_user(evt->daddr, 4, &in->sin_addr);
        bpf_probe_read_user(&evt->dport, 2, &in->sin_port);
        evt->dport = bpf_ntohs(evt->dport);
    } else if (family == AF_INET6) {
        struct sockaddr_in6 *in6 = (struct sockaddr_in6 *)addr;
        bpf_probe_read_user(evt->daddr, 16, &in6->sin6_addr);
        bpf_probe_read_user(&evt->dport, 2, &in6->sin6_port);
        evt->dport = bpf_ntohs(evt->dport);
    }

    bpf_ringbuf_submit(evt, 0);
    return 0;
}
```

### 3.2 Go事件结构体

```go
type ConnEvent struct {
    PID    uint32
    UID    uint32
    Comm   [16]byte
    Family uint16    // AF_INET(2) / AF_INET6(10)
    SAddr  [16]byte
    DAddr  [16]byte
    SPort  uint16
    DPort  uint16
}

func (l *Loader) processConnEvent(raw []byte) {
    var ce ConnEvent
    binary.Read(bytes.NewReader(raw), binary.LittleEndian, &ce)
    event := Event{
        EventType:   "network_connect",
        PID:         ce.PID,
        UID:         ce.UID,
        ProcessName: cstrToString(ce.Comm[:]),
        SrcPort:     ce.SPort,
        DstPort:     ce.DPort,
    }
    if ce.Family == 2 {
        event.SrcAddr = net.IP(ce.SAddr[:4]).String()
        event.DstAddr = net.IP(ce.DAddr[:4]).String()
    } else {
        event.SrcAddr = net.IP(ce.SAddr[:]).String()
        event.DstAddr = net.IP(ce.DAddr[:]).String()
    }
    l.sendEvent(event)
}
```

### 3.3 Pipeline扩展

```go
case "network_connect":
    m["category"] = "network_connection"
    m["src_addr"] = event.SrcAddr
    m["dst_addr"] = event.DstAddr
    m["src_port"] = event.SrcPort
    m["dst_port"] = event.DstPort
```

### 3.4 高风险端口标记

```go
var highRiskPorts = map[uint16]string{
    4444:  "Metasploit",
    5555:  "ADB",
    31337: "Back Orifice",
    1234:  "Common Backdoor",
}
```

---

## 4. 统一Event结构体扩展

```go
type Event struct {
    // 现有字段...
    EventID     string
    EventType   string // execve/fork/file_access/network_connect
    ProcessName string
    PID         uint32
    FilePath    string

    // V5.7新增: 文件事件
    FileFlags   string

    // V5.7新增: 网络事件
    SrcAddr     string
    DstAddr     string
    SrcPort     uint16
    DstPort     uint16
    ConnFamily  string
}
```

---

## 5. Sigma规则适配

### 5.1 文件事件规则

```yaml
title: 敏感文件写入
logsource:
    category: file_event
detection:
    selection:
        file_flags|contains: 'O_WRONLY'
        file_path|re: '/etc/(passwd|shadow|sudoers)'
    condition: selection
level: high
```

### 5.2 网络事件规则

```yaml
title: 高风险端口外联
logsource:
    category: network_connection
detection:
    selection:
        dst_port: [4444, 5555, 31337]
    filter_local:
        dst_addr|startswith: '127.'
    condition: selection and not filter_local
level: high
```

---

## 6. 测试计划

| 测试项 | 验证内容 |
|:---|:---|
| openat采集 | 创建/修改/etc/下文件，验证事件上报 |
| connect采集 | 发起外部连接，验证地址和端口正确 |
| IPv6 | IPv6连接，验证解析正确 |
| 内核态过滤 | 高频openat（find /），CPU < 5% |
| 事件去重 | 重复访问同一文件，验证只上报一次 |
| Sigma匹配 | 文件/网络Sigma规则匹配正确 |
| 降级 | 不支持ringbuf的内核，降级到/proc |
