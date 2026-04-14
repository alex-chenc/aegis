# Aegis智能主机安全系统 V5.6 Agent详细设计文档

**版本**: 5.6
**日期**: 2026-04-14
**状态**: 设计中

---

## 1. Agent概述

### 1.1 V5.6 Agent定位

V5.6版本的Agent从"事件采集器"升级为"智能体"，具备以下核心能力：

| 能力 | 说明 |
|------|------|
| 工具调用 | 支持接收并执行来自Server的工具调用请求 |
| 进程管理 | 获取进程树、网络连接、打开文件等信息 |
| 日志查询 | 查询本地历史日志 |
| 用户会话 | 获取当前登录用户会话信息 |

### 1.2 Agent架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Agent架构                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      eBPF事件采集层                                    │   │
│  │  (execve, connect, openat, setuid, fork, clone)                     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Sigma规则匹配层                                  │   │
│  │  (本地规则匹配，事件优先级判断)                                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      智能通信层                                      │   │
│  │  (事件分级上报，批量聚合)                                             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      工具执行层 (V5.6新增)                            │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────┐  │   │
│  │  │ GetProcess  │  │ GetNetwork  │  │ QueryHistoricalLogs        │  │   │
│  │  │ Tree        │  │ Connections │  │                            │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      gRPC通信层                                      │   │
│  │  (双向流连接Server)                                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 工具执行模块

### 2.1 目录结构

```
agent/
├── internal/
│   ├── tool_executor.go        # 工具执行器主类
│   ├── tools/
│   │   ├── base.go            # 工具基类
│   │   ├── process_tree.go     # GetProcessTree
│   │   ├── network.go          # GetNetworkConnections
│   │   ├── files.go            # GetOpenFiles
│   │   ├── processes.go        # GetRunningProcesses
│   │   ├── sessions.go          # GetUserSessions
│   │   └── logs.go             # QueryHistoricalLogs
│   ├── gRPC/
│   │   └── client.go           # gRPC客户端
│   └── ...
```

### 2.2 工具执行器主类

```go
// agent/internal/tool_executor.go

package tool

import (
    "encoding/json"
    "fmt"
    "sync"
    "time"
)

// Executor 工具执行器
type Executor struct {
    tools map[string]ToolHandler
    mu    sync.RWMutex
}

type ToolHandler func(args map[string]interface{}) (interface{}, error)

type ToolResult struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   string      `json:"error,omitempty"`
    TimeMs  int64       `json:"time_ms"`
}

// NewExecutor 创建工具执行器
func NewExecutor() *Executor {
    e := &Executor{
        tools: make(map[string]ToolHandler),
    }
    e.registerDefaultTools()
    return e
}

// registerDefaultTools 注册默认工具
func (e *Executor) registerDefaultTools() {
    // 进程相关
    e.tools["GetProcessTree"] = e.getProcessTree
    e.tools["GetRunningProcesses"] = e.getRunningProcesses
    e.tools["GetOpenFiles"] = e.getOpenFiles

    // 网络相关
    e.tools["GetNetworkConnections"] = e.getNetworkConnections

    // 用户相关
    e.tools["GetUserSessions"] = e.getUserSessions

    // 日志相关
    e.tools["QueryHistoricalLogs"] = e.queryHistoricalLogs
}

// Execute 执行工具
func (e *Executor) Execute(toolName string, args map[string]interface{}) *ToolResult {
    start := time.Now()

    e.mu.RLock()
    handler, ok := e.tools[toolName]
    e.mu.RUnlock()

    if !ok {
        return &ToolResult{
            Success: false,
            Error:   fmt.Sprintf("unknown tool: %s", toolName),
            TimeMs:  time.Since(start).Milliseconds(),
        }
    }

    // 执行工具
    data, err := handler(args)

    if err != nil {
        return &ToolResult{
            Success: false,
            Error:   err.Error(),
            TimeMs:  time.Since(start).Milliseconds(),
        }
    }

    return &ToolResult{
        Success: true,
        Data:    data,
        TimeMs:  time.Since(start).Milliseconds(),
    }
}

// CanExecute 检查工具是否存在
func (e *Executor) CanExecute(toolName string) bool {
    e.mu.RLock()
    defer e.mu.RUnlock()
    _, ok := e.tools[toolName]
    return ok
}

// ListTools 列出所有可用工具
func (e *Executor) ListTools() []string {
    e.mu.RLock()
    defer e.mu.RUnlock()

    tools := make([]string, 0, len(e.tools))
    for name := range e.tools {
        tools = append(tools, name)
    }
    return tools
}
```

### 2.3 GetProcessTree工具

```go
// agent/internal/tools/process_tree.go

package tools

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"
)

// getProcessTree 获取进程树
func (e *Executor) getProcessTree(args map[string]interface{}) (interface{}, error) {
    pid, ok := args["pid"].(float64)
    if !ok {
        return nil, fmt.Errorf("pid is required and must be a number")
    }

    intPid := int(pid)

    // 检查进程是否存在
    procPath := fmt.Sprintf("/proc/%d", intPid)
    if _, err := os.Stat(procPath); os.IsNotExist(err) {
        return nil, fmt.Errorf("process %d not found", intPid)
    }

    // 获取根进程
    root := e.buildProcessTree(intPid)

    return map[string]interface{}{
        "pid":      intPid,
        "tree":     root,
        "captured": time.Now().Unix(),
    }, nil
}

// ProcessInfo 进程信息
type ProcessInfo struct {
    PID      int             `json:"pid"`
    PPID     int             `json:"ppid"`
    Name     string          `json:"name"`
    User     string          `json:"user"`
    Command  string          `json:"command"`
    ExePath  string          `json:"exe_path"`
    CWD      string          `json:"cwd"`
    Children []*ProcessInfo  `json:"children,omitempty"`
}

// buildProcessTree 构建进程树
func (e *Executor) buildProcessTree(pid int) *ProcessInfo {
    info := e.readProcessInfo(pid)
    if info == nil {
        return nil
    }

    // 查找子进程
    children := e.findChildProcesses(pid)
    for _, child := range children {
        childTree := e.buildProcessTree(child)
        if childTree != nil {
            info.Children = append(info.Children, childTree)
        }
    }

    return info
}

// readProcessInfo 读取进程信息
func (e *Executor) readProcessInfo(pid int) *ProcessInfo {
    procDir := fmt.Sprintf("/proc/%d", pid)

    info := &ProcessInfo{PID: pid}

    // 读取status获取PPID和User
    statusPath := filepath.Join(procDir, "status")
    file, err := os.Open(statusPath)
    if err != nil {
        return nil
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "PPid:") {
            parts := strings.Split(line, ":")
            if len(parts) == 2 {
                info.PPID, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
            }
        }
        if strings.HasPrefix(line, "Uid:") {
            // 获取UID后转换为用户名
            parts := strings.Split(line, ":")
            if len(parts) == 2 {
                uidStr := strings.TrimSpace(parts[1])
                uids := strings.Fields(uidStr)
                if len(uids) > 0 {
                    info.User = getUsername(uids[0])
                }
            }
        }
    }

    // 读取cmdline
    cmdlinePath := filepath.Join(procDir, "cmdline")
    if cmdData, err := os.ReadFile(cmdlinePath); err == nil {
        // cmdline以\0分隔
        cmdline := strings.ReplaceAll(string(cmdData), "\x00", " ")
        info.Command = strings.TrimSpace(cmdline)
    }

    // 读取exe链接
    exePath := filepath.Join(procDir, "exe")
    if exe, err := os.Readlink(exePath); err == nil {
        info.ExePath = exe
    }

    // 读取cwd链接
    cwdPath := filepath.Join(procDir, "cwd")
    if cwd, err := os.Readlink(cwdPath); err == nil {
        info.CWD = cwd
    }

    // 读取进程名
    info.Name = getProcessName(pid)

    return info
}

// findChildProcesses 查找子进程
func (e *Executor) findChildProcesses(ppid int) []int {
    children := []int{}

    entries, err := os.ReadDir("/proc")
    if err != nil {
        return children
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }

        pid, err := strconv.Atoi(entry.Name())
        if err != nil {
            continue
        }

        // 读取该进程的父进程ID
        statusPath := fmt.Sprintf("/proc/%d/status", pid)
        file, err := os.Open(statusPath)
        if err != nil {
            continue
        }

        scanner := bufio.NewScanner(file)
        foundPPID := false
        for scanner.Scan() {
            line := scanner.Text()
            if strings.HasPrefix(line, "PPid:") {
                parts := strings.Split(line, ":")
                if len(parts) == 2 {
                    childPPID, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
                    if childPPID == ppid {
                        children = append(children, pid)
                    }
                }
                foundPPID = true
                break
            }
        }
        file.Close()
    }

    return children
}

// getProcessName 获取进程名
func getProcessName(pid int) string {
    commPath := fmt.Sprintf("/proc/%d/comm", pid)
    if data, err := os.ReadFile(commPath); err == nil {
        return strings.TrimSpace(string(data))
    }
    return ""
}
```

### 2.4 GetNetworkConnections工具

```go
// agent/internal/tools/network.go

package tools

import (
    "bufio"
    "fmt"
    "os"
    "strings"
)

// getNetworkConnections 获取网络连接
func (e *Executor) getNetworkConnections(args map[string]interface{}) (interface{}, error) {
    pid, _ := args["pid"].(float64) // 可选参数

    var connections []map[string]interface{}

    if pid > 0 {
        // 获取指定进程的网络连接
        connections = e.getProcessNetworkConnections(int(pid))
    } else {
        // 获取所有网络连接
        connections = e.getAllNetworkConnections()
    }

    return map[string]interface{}{
        "connections": connections,
        "count":       len(connections),
        "captured":    time.Now().Unix(),
    }, nil
}

// ConnectionInfo 网络连接信息
type ConnectionInfo struct {
    LocalAddr  string `json:"local_addr"`
    RemoteAddr string `json:"remote_addr"`
    State      string `json:"state"`
    UID        int    `json:"uid"`
    PID        int    `json:"pid,omitempty"`
}

// getAllNetworkConnections 获取所有网络连接
func (e *Executor) getAllNetworkConnections() []map[string]interface{} {
    connections := []map[string]interface{}{}

    // 读取TCP连接
    connections = append(connections, e.readNetFile("tcp")...)
    connections = append(connections, e.readNetFile("tcp6")...)

    // 读取UDP连接
    connections = append(connections, e.readNetFile("udp")...)
    connections = append(connections, e.readNetFile("udp6")...)

    return connections
}

// getProcessNetworkConnections 获取指定进程的网络连接
func (e *Executor) getProcessNetworkConnections(pid int) []map[string]interface{} {
    // 读取进程的文件描述符
    fdDir := fmt.Sprintf("/proc/%d/fd", pid)
    dir, err := os.Open(fdDir)
    if err != nil {
        return []map[string]interface{}{}
    }
    defer dir.Close()

    connections := []map[string]interface{}{}

    entries, err := dir.ReadDir(-1)
    if err != nil {
        return connections
    }

    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }

        fdPath := fmt.Sprintf("%s/%s", fdDir, entry.Name())
        link, err := os.Readlink(fdPath)
        if err != nil {
            continue
        }

        // socket:[...] 格式
        if strings.HasPrefix(link, "socket:[") {
            socketID := strings.Trim(link, "socket:[]")
            // 根据socket ID查找网络连接
            conn := e.findConnectionBySocketID(socketID)
            if conn != nil {
                conn["pid"] = pid
                connections = append(connections, conn)
            }
        }
    }

    return connections
}

// readNetFile 读取/proc/net/文件
func (e *Executor) readNetFile(fileType string) []map[string]interface{} {
    connections := []map[string]interface{}{}

    path := fmt.Sprintf("/proc/net/%s", fileType)
    file, err := os.Open(path)
    if err != nil {
        return connections
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    // 跳过表头
    if scanner.Scan() {
        // 忽略表头行
    }

    for scanner.Scan() {
        line := scanner.Text()
        fields := strings.Fields(line)
        if len(fields) < 10 {
            continue
        }

        // 解析字段
        // 参考: https://www.tcpdump.org/papers/bpf-usenix93.pdf
        // 格式: sl local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode
        localAddr := e.parseIP(fields[1])
        remoteAddr := e.parseIP(fields[2])
        state := e.parseTCPState(fields[3])
        uid, _ := strconv.Atoi(fields[7])

        connections = append(connections, map[string]interface{}{
            "local_addr": localAddr,
            "remote_addr": remoteAddr,
            "state":      state,
            "uid":        uid,
        })
    }

    return connections
}

// parseIP 解析IP地址
func (e *Executor) parseIP hexIP string) string {
    // Linux /proc/net/* 文件中的IP地址是十六进制little-endian
    // 需要转换
    if len(hexIP) == 8 {
        // IPv4
        ip := fmt.Sprintf("%d.%d.%d.%d",
            parseHexByte(hexIP[6:8]),
            parseHexByte(hexIP[4:6]),
            parseHexByte(hexHex[2:4]),
            parseHexByte(hexIP[0:2]))
        return ip
    } else if len(hexIP) > 8 {
        // IPv6 (简化处理)
        return hexIP
    }
    return hexIP
}
```

### 2.5 QueryHistoricalLogs工具

```go
// agent/internal/tools/logs.go

package tools

import (
    "archive/tar"
    "bufio"
    "bytes"
    "compress/gzip"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// queryHistoricalLogs 查询历史日志
func (e *Executor) queryHistoricalLogs(args map[string]interface{}) (interface{}, error) {
    startTimeStr, ok := args["start_time"].(string)
    if !ok {
        return nil, fmt.Errorf("start_time is required")
    }

    endTimeStr, ok := args["end_time"].(string)
    if !ok {
        return nil, fmt.Errorf("end_time is required")
    }

    filter, _ := args["filter"].(string) // 可选过滤条件

    startTime, err := time.Parse(time.RFC3339, startTimeStr)
    if err != nil {
        return nil, fmt.Errorf("invalid start_time format, use RFC3339")
    }

    endTime, err := time.Parse(time.RFC3339, endTimeStr)
    if err != nil {
        return nil, fmt.Errorf("invalid end_time format, use RFC3339")
    }

    // 日志目录
    logDirs := []string{
        "/var/log",
        "/var/log/syslog",
        "/var/log/messages",
    }

    logs := []map[string]interface{}{}

    for _, logDir := range logDirs {
        entries, err := e.searchLogs(logDir, startTime, endTime, filter)
        if err == nil {
            logs = append(logs, entries...)
        }
    }

    return map[string]interface{}{
        "logs":   logs,
        "count":  len(logs),
        "captured": time.Now().Unix(),
    }, nil
}

// searchLogs 搜索日志文件
func (e *Executor) searchLogs(dir string, start, end time.Time, filter string) ([]map[string]interface{}, error) {
    logs := []map[string]interface{}{}

    filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return nil // 继续处理其他文件
        }

        if info.IsDir() {
            return nil
        }

        // 只处理文本日志文件
        ext := filepath.Ext(path)
        if ext != ".log" && ext != "" && !strings.Contains(path, "log.") {
            return nil
        }

        // 检查文件修改时间
        if info.ModTime().Before(start) {
            return nil
        }

        // 读取文件内容
        entries, err := e.readLogFile(path, start, end, filter)
        if err == nil {
            logs = append(logs, entries...)
        }

        return nil
    })

    return logs, nil
}

// readLogFile 读取单个日志文件
func (e *Executor) readLogFile(path string, start, end time.Time, filter string) ([]map[string]interface{}, error) {
    logs := []map[string]interface{}{}

    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := scanner.Text()

        // 应用过滤
        if filter != "" && !strings.Contains(line, filter) {
            continue
        }

        // 解析时间戳（假设日志格式：Jan 15 10:30:45）
        timestamp := parseLogTimestamp(line)
        if timestamp == nil {
            continue
        }

        // 检查时间范围
        if timestamp.Before(start) || timestamp.After(end) {
            continue
        }

        logs = append(logs, map[string]interface{}{
            "timestamp": timestamp.Format(time.RFC3339),
            "source":    path,
            "message":   line,
        })
    }

    return logs, nil
}
```

---

## 3. gRPC工具调用接口

### 3.1 Proto定义

```protobuf
// agent.proto

service AgentService {
    rpc Register(RegisterRequest) returns (RegisterResponse);
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
    rpc ExecuteCommand(stream CommandRequest) returns (stream CommandResponse);
    rpc ExecuteTool(ToolRequest) returns (ToolResponse);  // V5.6新增
    rpc ReportEvent(ReportEventRequest) returns (ReportEventResponse);
    rpc UpdateRules(RuleUpdateRequest) returns (RuleUpdateResponse);
}

// V5.6新增: 工具调用接口
message ToolRequest {
    string call_id = 1;
    string host_id = 2;
    string tool = 3;              // 工具名称: GetProcessTree, GetNetworkConnections, etc.
    string arguments = 4;          // JSON格式参数
    int32 timeout_seconds = 5;    // 超时时间，默认30秒
}

message ToolResponse {
    string call_id = 1;
    bool success = 2;
    string result = 3;            // JSON格式结果
    string error = 4;
    int64 execution_time_ms = 5;
}
```

### 3.2 Agent端实现

```go
// agent/internal/grpc_server.go

package internal

import (
    "context"
    "encoding/json"
    "fmt"

    "agent/internal/tool"
    pb "agent/pkg/api/v1"
)

// ExecuteTool 处理工具调用请求
func (s *GRPCServer) ExecuteTool(ctx context.Context, req *pb.ToolRequest) (*pb.ToolResponse, error) {
    logger.Info("tool request received",
        zap.String("call_id", req.CallId),
        zap.String("tool", req.Tool),
    )

    // 解析参数
    var args map[string]interface{}
    if req.Arguments != "" {
        if err := json.Unmarshal([]byte(req.Arguments), &args); err != nil {
            return &pb.ToolResponse{
                CallId:  req.CallId,
                Success: false,
                Error:   fmt.Sprintf("failed to parse arguments: %v", err),
            }, nil
        }
    }

    // 执行工具
    result := s.toolExecutor.Execute(req.Tool, args)

    // 序列化和返回结果
    resultJSON, err := json.Marshal(result)
    if err != nil {
        return &pb.ToolResponse{
            CallId:         req.CallId,
            Success:        false,
            Error:          fmt.Sprintf("failed to marshal result: %v", err),
            ExecutionTimeMs: result.TimeMs,
        }, nil
    }

    return &pb.ToolResponse{
        CallId:         req.CallId,
        Success:        result.Success,
        Result:         string(resultJSON),
        Error:          result.Error,
        ExecutionTimeMs: result.TimeMs,
    }, nil
}
```

---

## 4. 命令解析协议

### 4.1 工具调用命令格式

Agent通过双向流接收来自Server的命令，其中工具调用命令格式如下：

```
#TOOL:ToolName#{"arg1": "value1", "arg2": "value2"}
```

示例：

```
#TOOL:GetProcessTree#{"pid": 12345}
#TOOL:GetNetworkConnections#{}
#TOOL:QueryHistoricalLogs#{"start_time": "2026-04-14T10:00:00Z", "end_time": "2026-04-14T11:00:00Z", "filter": "ssh"}
```

### 4.2 命令解析器

```go
// agent/internal/command_parser.go

package internal

import (
    "encoding/json"
    "fmt"
    "strings"
)

const (
    ToolCommandPrefix = "#TOOL:"
    ToolCommandSuffix = "#"
)

// ToolCommand 工具调用命令
type ToolCommand struct {
    ToolName string
    Args     map[string]interface{}
}

// ParseToolCommand 解析工具调用命令
func ParseToolCommand(cmd string) (*ToolCommand, error) {
    // 检查前缀
    if !strings.HasPrefix(cmd, ToolCommandPrefix) {
        return nil, fmt.Errorf("not a tool command")
    }

    // 去除前缀
    cmd = strings.TrimPrefix(cmd, ToolCommandPrefix)

    // 查找分隔符位置
    sepIdx := strings.Index(cmd, ToolCommandSuffix)
    if sepIdx == -1 {
        return nil, fmt.Errorf("invalid tool command format")
    }

    toolName := cmd[:sepIdx]
    argsStr := cmd[sepIdx+len(ToolCommandSuffix):]

    var args map[string]interface{}
    if argsStr != "" {
        if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
            return nil, fmt.Errorf("failed to parse arguments: %w", err)
        }
    }

    return &ToolCommand{
        ToolName: toolName,
        Args:     args,
    }, nil
}

// IsToolCommand 检查是否为工具调用命令
func IsToolCommand(cmd string) bool {
    return strings.HasPrefix(cmd, ToolCommandPrefix)
}
```

---

## 5. 资源使用

### 5.1 资源占用估算

| 模块 | V5.5 | V5.6 | 增量 |
|------|------|------|------|
| eBPF采集 | 0.3核/30MB | 0.3核/30MB | - |
| Sigma匹配 | 0.1核/20MB | 0.1核/20MB | - |
| 工具执行器 | N/A | 0.1核/30MB | +0.1核/+30MB |
| gRPC通信 | 0.1核/20MB | 0.1核/20MB | - |
| **总计** | **~0.5核/~70MB** | **~0.6核/~100MB** | **+0.1核/+30MB** |

### 5.2 性能指标

| 指标 | 要求 |
|------|------|
| 工具调用响应时间 | <500ms |
| GetProcessTree (单进程) | <100ms |
| GetNetworkConnections | <200ms |
| QueryHistoricalLogs | <1s (受日志大小影响) |

---

**文档结束**
