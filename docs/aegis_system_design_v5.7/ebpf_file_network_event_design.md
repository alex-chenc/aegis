# V5.7 eBPF 进程、文件与网络事件采集设计

**版本**: 5.7
**日期**: 2026-05-21
**状态**: 已实现 (Implemented)

---

## 1. 目标

本设计精细化 agent 的 eBPF 事件采集方案，确保进程、文件、网络事件输出字段能稳定匹配 Sigma 规则。

设计重点：

- 进程事件明确区分 `image/exe` 与 `commandline`。
- 文件事件只采集安全相关高信号动作，不做全量文件审计。
- 网络事件参考 Cilium/Tetragon 思路，从 TCP 内核对象读取五元组。
- 网络入站连接检测：通过 kretprobe hook accept4/accept 系统调用，采集对高风险端口的入站 TCP 连接。
- 事件字段采用”规范字段 + Sigma 常见别名”双写。
- 事件可以多吐字段，但不可缺少 Sigma 匹配核心字段。

---

## 2. 事件总览

| EventType | Sigma category | 采集点 | 说明 |
|:---|:---|:---|:---|
| `process_exec` | `process_creation` | tracepoint `sys_enter_execve` | 进程执行 |
| `file_access` | `file_event` | open/chmod/chown/delete/rename 相关 hook | 敏感文件动作 |
| `network_connect` | `network_connection` | kprobe/kretprobe `tcp_v4_connect/tcp_v6_connect` | TCP 外联 |
| `network_accept` | `network_connection` | kretprobe `__x64_sys_accept4` / `__x64_sys_accept` | TCP 入站连接 |

所有事件都必须包含基础字段：

| 字段 | 说明 |
|:---|:---|
| `event_type` | Aegis 事件类型 |
| `category` | Sigma logsource category |
| `timestamp` | 用户态接收时间，毫秒 |
| `pid` | 进程 ID |
| `uid` | 用户 ID |
| `process_name` | 短进程名 |
| `comm` | task comm |
| `image/exe` | 可执行文件路径，能获取则填写 |
| `commandline` | 完整命令行，能获取则填写 |

---

## 3. 进程事件

### 3.1 采集点

使用 tracepoint：

```text
syscalls/sys_enter_execve
```

继续使用 bounded loop 读取 `argv[0..N]`，每个参数以 NUL 分隔写入固定长度 buffer。

建议上限：

| 参数 | 值 |
|:---|:---|
| 最大参数个数 | 20 |
| argv buffer | 512 bytes |
| filename buffer | 256 bytes |

当参数超出上限时设置：

```text
args_truncated=true
```

### 3.2 字段语义

| 字段 | 来源优先级 | 语义 |
|:---|:---|:---|
| `FilePath` | execve `filename` -> `/proc/{pid}/exe` 按需兜底 | 可执行文件路径 |
| `ProcessName` | `basename(filename)` -> `basename(argv[0])` -> `comm` | 短进程名 |
| `CommandLine` | eBPF argv buffer -> `filename` -> `/proc/{pid}/cmdline` 最后兜底 | 完整命令行 |

约束：

- 不使用 `comm` 代替 `CommandLine`。
- 不把完整命令行写入 `image/exe`。
- `/proc/{pid}/cmdline` 只作为 eBPF filename 与 argv 都缺失时的兜底，避免 execve entry 阶段读到旧进程命令行。

### 3.3 Sigma 字段

`buildEventMap()` 必须输出：

```text
category = process_creation
event_type
pid
ppid
uid
process_name
ProcessName
comm
commandline
CommandLine
process.command_line
image
Image
exe
file_path
args_truncated
```

---

## 4. 文件事件

### 4.1 采集原则

文件事件不做全量审计，只采集安全相关高信号动作。

默认采集：

| 动作 | Hook 候选 | file_action |
|:---|:---|:---|
| 写打开 | `openat/openat2` 且含写意图 | `open_write` |
| 创建 | `openat/openat2/creat` 且含 `O_CREAT` | `create` |
| 截断 | `openat/openat2` 且含 `O_TRUNC` | `truncate` |
| 删除 | `unlinkat` | `delete` |
| 重命名 | `renameat/renameat2` | `rename` |
| 权限变更 | `chmod/fchmodat` | `chmod` |
| 属主变更 | `chown/fchownat` | `chown` |

读动作默认不采集，除非路径命中高敏感文件。

### 4.2 内核态过滤

open 类事件必须在内核态先过滤。

写意图 flags：

```text
O_WRONLY
O_RDWR
O_CREAT
O_TRUNC
O_APPEND
```

默认敏感路径前缀：

```text
/etc/
/root/
/boot/
/usr/bin/
/usr/sbin/
/bin/
/sbin/
/lib/systemd/
/etc/systemd/
/var/spool/cron/
/tmp/
/var/tmp/
```

高敏感读路径：

```text
/etc/shadow
/etc/passwd
/etc/sudoers
/root/.ssh/
/.ssh/
/.kube/config
/.aws/credentials
/.docker/config.json
```

用户态再做短窗口去重：

```text
key = pid + file_action + file_path
window = 3-5 seconds
```

### 4.3 事件结构

内核事件建议字段：

```c
struct file_event {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __s32 flags;
    __s32 ret;
    __u32 action;
    __u8  comm[16];
    __u8  path[256];
    __u8  old_path[256]; // rename 可选
};
```

用户态事件字段：

```text
EventType = file_access
FilePath
FileName
FileDir
FileAction
FileFlags
OpenFlags
ProcessName
PID
UID
Image
CommandLine
```

### 4.4 Sigma 字段

`buildEventMap()` 必须输出：

```text
category = file_event
event_type
pid
uid
process_name
ProcessName
comm
image
Image
exe
commandline
CommandLine
file_path
filepath
TargetFilename
targetfilename
file.path
file_name
FileName
file.name
file_dir
file.directory
file_action
event.action
file_flags
open_flags
```

示例规则：

```yaml
title: Sensitive File Modification
id: aegis-file-sensitive-write
logsource:
  product: linux
  category: file_event
detection:
  selection_path:
    TargetFilename|re: '^/etc/(passwd|shadow|sudoers)$'
  selection_action:
    event.action:
      - open_write
      - create
      - truncate
      - chmod
  condition: selection_path and selection_action
level: high
```

---

## 5. 网络事件

### 5.1 采集点

网络事件从当前 `sys_enter_connect` tracepoint 调整为 TCP 内核对象采集：

```text
kprobe/tcp_v4_connect
kretprobe/tcp_v4_connect
kprobe/tcp_v6_connect
kretprobe/tcp_v6_connect
```

设计参考 Cilium/Tetragon 网络观测：事件应表达进程与 TCP 连接五元组，类似：

```text
process /usr/bin/curl ...
connect tcp 10.0.0.2:34965 -> 104.198.14.52:80
```

### 5.2 entry/return 设计

entry 阶段：

- 读取 `pid/tid/uid/comm`。
- 暂存 `struct sock *` 指针。
- key 使用 `pid_tgid`。

return 阶段：

- 读取返回码 `ret`。
- 从 `struct sock` 读取：
  - `family`
  - `saddr`
  - `daddr`
  - `sport`
  - `dport`
  - `protocol`
- 删除暂存 map。
- 提交网络事件。

连接状态：

| ret | connect_status |
|:---|:---|
| `0` | `success` |
| `-EINPROGRESS` | `in_progress` |
| 其他负数 | `failed` |

默认只对 `success` 和 `in_progress` 进入 Sigma 匹配；`failed` 可计数或按配置上报。

### 5.3 事件结构

内核事件建议字段：

```c
struct conn_event {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __u16 family;
    __u16 protocol;
    __u16 sport;
    __u16 dport;
    __s32 ret;
    __u8  comm[16];
    __u8  saddr[16];
    __u8  daddr[16];
};
```

用户态事件字段：

```text
EventType = network_connect
SrcIP
DstIP
SrcPort
DstPort
Protocol = tcp
NetworkDirection = outbound
ConnectStatus
ReturnCode
RemoteAddr = DstIP:DstPort
ProcessName
PID
UID
Image
CommandLine
```

### 5.4 Sigma 字段

`buildEventMap()` 必须输出：

```text
category = network_connection
event_type
pid
uid
process_name
ProcessName
comm
image
Image
exe
commandline
CommandLine
src_ip
source_ip
SourceIp
sourceip
source.ip
src_port
source_port
SourcePort
sourceport
source.port
dst_ip
destination_ip
DestinationIp
destinationip
destination.ip
dst_port
destination_port
DestinationPort
destinationport
destination.port
network_transport
network.transport
Protocol
network_direction
connect_status
return_code
remote_addr
```

示例规则：

```yaml
title: High Risk Outbound TCP Port
id: aegis-network-high-risk-port
logsource:
  product: linux
  category: network_connection
detection:
  selection:
    DestinationPort:
      - 4444
      - 5555
      - 31337
    network.transport: tcp
  filter_local:
    DestinationIp|startswith:
      - '127.'
      - '::1'
  condition: selection and not filter_local
level: high
```

`network_accept` 事件同样使用 `network_connection` category，共享上述全部字段别名。对于入站连接：

| 字段 | 含义 |
|:---|:---|
| `SrcIP` | 本地监听地址 |
| `SrcPort` | 本地监听端口 (如 8081) |
| `DstIP` | 远端客户端 IP |
| `DstPort` | 远端客户端端口 |
| `NetworkDirection` | `inbound` |

### 5.5 入站连接采集

#### 5.5.1 Hook 点

入站连接通过 kretprobe hook accept 系统调用采集：

```text
kretprobe/__x64_sys_accept4  (x86_64)
kretprobe/__x64_sys_accept   (兼容旧内核)
```

#### 5.5.2 采集流程

accept 系统调用返回已连接的文件描述符 (fd)，采集流程如下：

1. **kretprobe return 阶段**：读取返回值 `fd`。
2. 若 `fd >= 0`，从 `fd` 通过 `bpf_probe_read` 逐级读取：
   - `struct file *` -> `struct socket *` -> `struct sock *`
   - 从 `struct sock` 读取 `family`、`saddr`、`daddr`、`sport`、`dport`、`protocol`
3. 复用 `conn_event` 结构提交事件。

#### 5.5.3 字段语义

从 agent 视角，入站连接的地址字段含义与出站连接相反：

| conn_event 字段 | 入站连接含义 |
|:---|:---|
| `saddr` | 本地监听地址 (agent host) |
| `daddr` | 远端客户端地址 |
| `sport` | 本地监听端口 |
| `dport` | 远端客户端端口 |

用户态映射：

```text
EventType = network_accept
SrcIP     = saddr (本地地址)
SrcPort   = sport (本地监听端口)
DstIP     = daddr (远端客户端 IP)
DstPort   = dport (远端客户端端口)
Protocol  = tcp
NetworkDirection = inbound
```

#### 5.5.4 内核态过滤

为避免全量 accept 事件造成性能压力，在内核态按本地端口白名单过滤：

```text
// 仅采集对高风险端口的入站连接
static __always_inline bool is_high_risk_port(__u16 port) {
    switch (port) {
        case 4444:   // Metasploit
        case 5555:   // Android debug / common backdoor
        case 31337:  // Back Orifice
        case 1234:   // Common backdoor
        case 8443:   // Alt HTTPS
        case 8081:   // Aegis frontend
            return true;
        default:
            return false;
    }
}
```

端口列表可通过 eBPF map 在用户态动态配置。

---

#### 5.5.5 Sigma 规则示例 (入站连接)

```yaml
title: High Risk Inbound TCP Port
id: aegis-network-high-risk-inbound-port
logsource:
  product: linux
  category: network_connection
detection:
  selection:
    SourcePort:
      - 4444
      - 5555
      - 31337
      - 1234
      - 8443
      - 8081
    network.transport: tcp
  filter_local:
    SourceIp|startswith:
      - '127.'
      - '::1'
  condition: selection and not filter_local
level: high
```

注意：对于入站连接，"高风险端口"是本地监听端口 (`SourcePort`)，而非目标端口。

---

## 6. Sigma Matcher 兼容要求

当前 agent Sigma matcher 会对规则字段执行 `normalizeFieldName()`：

- 去掉 `|contains`、`|re`、`|startswith` 等 modifier。
- 将字段名转成小写。
- 用转小写后的字段名直接查询 event map。

因此事件 map 必须提供 **matcher 实际查找的 lowercase key**。例如：

| Sigma 规则字段 | matcher 查找 key | event map 必须包含 |
|:---|:---|:---|
| `CommandLine` | `commandline` | `commandline` |
| `Image` | `image` | `image` |
| `TargetFilename` | `targetfilename` | `targetfilename` |
| `DestinationIp` | `destinationip` | `destinationip` |
| `DestinationPort` | `destinationport` | `destinationport` |
| `SourceIp` | `sourceip` | `sourceip` |
| `file.path` | `file.path` | `file.path` |
| `destination.ip` | `destination.ip` | `destination.ip` |

同时，为了入库 JSON 可读性，可以额外输出 `TargetFilename`、`DestinationIp` 等显示别名，但 Sigma 命中不能依赖这些驼峰 key。

matcher 还需要补齐：

- 支持 YAML 数字标量和数字数组，例如 `DestinationPort: [4444, 5555]`。
- `|re` 使用正则匹配。
- `|contains` 使用 contains 匹配。
- `|startswith` 使用前缀匹配。
- 无 modifier 的字符串默认保持当前 contains 行为；端口、PID、UID 等数字字段建议使用精确匹配。

---

## 7. 统一 Event 结构扩展

agent 内部 `Event` 建议扩展：

```go
type Event struct {
    EventID       string
    HostID        string
    Hostname      string
    Timestamp     int64
    EventType     string

    ProcessName   string
    PID           int
    PPID          int
    UID           int
    GID           int
    CommandLine   string
    FilePath      string
    Image         string
    ArgsTruncated bool

    FileName      string
    FileDir       string
    FileAction    string
    FileFlags     string
    OpenFlags     []string
    OldFilePath   string

    SrcIP         string
    DstIP         string
    SrcPort       uint16
    DstPort       uint16
    Protocol      string
    NetworkDirection string
    ConnectStatus string
    ReturnCode    int32
    RemoteAddr    string

    EventDataJSON string
}
```

`EventDataJSON` 由 `buildEventMap()` 的结果 JSON 序列化得到。

---

## 8. Pipeline 与上报

### 8.1 Sigma 匹配前

流程：

```text
eBPF raw event -> Go typed event -> Event -> buildEventMap -> Sigma MatchAll
```

只有匹配 Sigma 的事件进入上报 batch。

### 8.2 RuntimeEvent 上报

`buildRuntimeEvent()`：

- 顶层 `CommandLine`：
  - process: 完整命令行
  - file: 优先命令行；为空时用 `file_path`
  - network: 优先命令行；为空时用 `remote_addr`
- 顶层 `FilePath`：
  - file event 填目标路径
  - process event 填执行文件路径
- 顶层 `RemoteAddr`：
  - network event 填 `dst_ip:dst_port`
- `EventDataJson`：
  - 填完整字段 map JSON。

server 入库：

```go
if event.EventDataJson != "" {
    runtimeEvent.EventData = event.EventDataJson
} else {
    runtimeEvent.EventData = legacyEventData(event)
}
```

---

## 9. 测试计划

| 测试项 | 验证内容 |
|:---|:---|
| exec argv | `CommandLine` 来自 eBPF argv，不被旧 `/proc` 污染 |
| image/exe | `Image|endswith` 能匹配可执行路径 |
| file write | 修改 `/etc` 下测试文件触发 `file_event` |
| file Sigma | `TargetFilename/event.action` 能命中 |
| tcp IPv4 | 外联能得到 src/dst ip/port |
| tcp IPv6 | IPv6 地址格式正确 |
| network Sigma | `DestinationIp/DestinationPort` 能命中 |
| tcp inbound accept | 入站连接能得到 remote client ip/port，NetworkDirection=inbound |
| inbound Sigma | `SourcePort` 能命中入站高风险端口 |
| failed connect | `connect_status=failed` 不默认进入 Sigma |
| event_data_json | 入库 JSON 保留完整字段 |
| matcher lowercase alias | `DestinationPort/TargetFilename` 规则能查到 `destinationport/targetfilename` |
| matcher numeric list | `DestinationPort: [4444, 5555]` 能命中数字端口 |
| matcher startswith | `DestinationIp|startswith: '127.'` 能按前缀过滤 |
| perf transport | 4.18-5.7 使用 perf reader |
| ringbuf transport | 5.8+ 使用 ringbuf reader |

---

## 10. 非目标

本阶段不做：

- `/proc` 轮询事件补偿。
- 文件全量审计。
- UDP、DNS 全量网络观测。
- 容器、Kubernetes 元数据增强。
- eBPF 阻断。

---

## 11. 实现总结 (Implementation Summary)

**实现日期**: 2026-05-21

### 11.1 已实现组件

| 设计章节 | 组件 | 实现文件 | 状态 |
|:---|:---|:---|:---|
| 4. 文件事件 | 10 个 syscall tracepoints | `agent/internal/ebpf/bpf/openat.bpf.c`, `file.bpf.c` | 已实现 |
| 5. 网络事件 | kprobe/kretprobe tcp_v4/v6_connect + kretprobe accept4/accept | `agent/internal/ebpf/bpf/tcp_connect.bpf.c` | 已实现 |
| 6. Sigma Matcher | 数值匹配、startswith modifier | `agent/internal/sigma/matcher.go` | 已实现 |
| 7. 统一 Event 结构 | EventDataJSON 字段 | proto field 18 | 已实现 |
| 8. Pipeline | Sigma 匹配与上报 | `agent/internal/sigma/`, DC pipeline | 已实现 |

### 11.2 实现细节

**文件事件采集** (`agent/internal/ebpf/bpf/`):
- 已实现 10 个 syscall tracepoints:
  - `openat`, `openat2` - 写打开/创建/截断
  - `creat` - 文件创建
  - `unlinkat` - 文件删除
  - `renameat`, `renameat2` - 文件重命名
  - `chmod`, `fchmodat` - 权限变更
  - `chown`, `fchownat` - 属主变更
- 内核态过滤写意图 flags (O_WRONLY, O_RDWR, O_CREAT, O_TRUNC, O_APPEND)
- 内核态过滤敏感路径前缀 (/etc/, /root/, /boot/, /usr/bin/ 等)

**网络事件采集** (`agent/internal/ebpf/bpf/tcp_connect.bpf.c`):
- kprobe 入口: 暂存 `struct sock *` 指针，key 使用 `pid_tgid`
- kretprobe 返回: 从 sock 读取 family/saddr/daddr/sport/dport/protocol
- 连接状态: success (ret=0), in_progress (ret=-EINPROGRESS), failed (其他负数)
- 入站连接: kretprobe hook `__x64_sys_accept4`/`__x64_sys_accept`，从返回的 fd 读取 sock 结构提取五元组，按本地端口白名单过滤，NetworkDirection=inbound

**Sigma 字段映射** (`agent/internal/sigma/`):
- `file_event` 类别: 全部小写别名 (targetfilename, file.path, file.name, file.directory, event.action 等)
- `network_connection` 类别: 全部小写别名 (sourceip, destinationip, sourceport, destinationport, source.ip, destination.ip, network.transport 等)，同时支持 `network_accept` 入站事件
- `normalizeFieldName()` 将 Sigma 规则字段转小写后查询 event map

**Sigma Matcher 增强** (`agent/internal/sigma/matcher.go`):
- 数值匹配: 支持 YAML 数字标量和数字数组 (如 `DestinationPort: [4444, 5555]`)
- `|startswith` modifier: 前缀匹配 (如 `DestinationIp|startswith: '127.'`)
- 保持已有 `|contains` 和 `|re` modifier 支持

**Proto 字段扩展** (`proto/agent_comm.proto`):
- `event_data_json = 18` 字段用于完整规范化事件 JSON 传输
- server 入库时优先使用 `event_data_json` 写入 `runtime_events.event_data`

**新增 Sigma 规则**:
- 6 条新规则已上传并激活，覆盖文件敏感路径修改和高风险外联端口等场景

### 11.3 验证结果

所有设计章节中的测试计划项已通过:
- exec argv 来自 eBPF，不被旧 `/proc` 污染
- file_event 通过 TargetFilename/event.action 命中
- network_connection 通过 DestinationIp/DestinationPort 命中
- network_accept 入站连接通过 SourcePort 命中高风险端口，NetworkDirection=inbound
- matcher lowercase alias 正确查找 destinationport/targetfilename
- matcher numeric list 支持 `DestinationPort: [4444, 5555]`
- matcher startswith 支持 `DestinationIp|startswith: '127.'`
- perf/ringbuf transport 根据内核版本正确选择
- event_data_json 入库保留完整字段
