# Agent详细设计文档 - V5.2 完整版

**版本**: 5.2
**状态**: 定稿
**日期**: 2026-03-26

---

## 1. 项目结构

```
/agent
├── cmd/agent/main.go            # 入口文件
├── internal/
│   ├── config/
│   │   └── config.go            # 配置管理
│   ├── client/
│   │   └── client.go            # gRPC客户端
│   ├── ebpf/
│   │   ├── loader.go            # eBPF加载器
│   │   ├── pipeline.go          # 事件处理管道
│   │   └── bpf/
│   │       ├── execve.bpf.c
│   │       └── fork.bpf.c
│   ├── rules/
│   │   └── loader.go            # Sigma规则加载
│   ├── logger/
│   │   └── logger.go            # 日志管理
│   └── tools/
│       └── file_tools.go        # 文件操作工具
├── dist/
│   ├── aegis-agent-linux-amd64
│   ├── aegis-agent-linux-arm64
│   ├── bpf/
│   │   ├── execve.bpf.o
│   │   └── fork.bpf.o
│   └── aegis-agent.tar.gz
├── Makefile
└── Dockerfile
```

---

## 2. 配置管理

### 2.1 配置文件

```toml
# /etc/aegis-agent/config.toml
ServerAddr = '192.168.1.100:19090'
AuthToken = 'a_very_secret_agent_token'
HostID = '53efa0f7-06c5-4b10-83c8-019327bcd0a2'
EventBufferSize = 10000
RuleDir = '/etc/aegis-agent/rules'
QuarantineDir = '/var/quarantine'
LogLevel = 'info'
```

### 2.2 配置结构

```go
type Config struct {
    ServerAddr     string `toml:"ServerAddr"`
    AuthToken      string `toml:"AuthToken"`
    HostID         string `toml:"HostID"`
    EventBufferSize int   `toml:"EventBufferSize"`
    RuleDir        string `toml:"RuleDir"`
    QuarantineDir  string `toml:"QuarantineDir"`
    LogLevel       string `toml:"LogLevel"`
}

func LoadConfig() (*Config, error) {
    cfg := &Config{
        EventBufferSize: 10000,
        LogLevel:        "info",
    }
    
    data, err := os.ReadFile("/etc/aegis-agent/config.toml")
    if err != nil {
        return nil, err
    }
    
    if err := toml.Unmarshal(data, cfg); err != nil {
        return nil, err
    }
    
    return cfg, nil
}
```

---

## 3. eBPF事件采集

### 3.1 事件类型

Agent只采集进程事件：

| 事件类型 | eBPF Hook | 说明 |
|----------|-----------|------|
| `process_exec` | sys_enter_execve | 进程执行 |
| `process_fork` | sched_process_fork | 进程创建 |

### 3.2 事件结构

```go
type Event struct {
    EventID     string `json:"event_id"`
    HostID      string `json:"host_id"`
    Hostname    string `json:"hostname"`
    Timestamp   int64  `json:"timestamp"`
    EventType   string `json:"event_type"`
    ProcessName string `json:"process_name"`
    PID         int    `json:"pid"`
    PPID        int    `json:"ppid"`
    UID         int    `json:"uid"`
    CommandLine string `json:"command_line"`
    FilePath    string `json:"file_path"`
}
```

### 3.3 eBPF加载器

```go
type Loader struct {
    collections map[string]*ebpf.Collection
    links       []link.Link
    readers     map[string]*ringbuf.Reader
    eventChan   chan Event
    hostID      string
    hostname    string
    seq         uint64
    done        chan struct{}
}

func (l *Loader) LoadAll() error {
    programs := []struct {
        name       string
        tracepoint string
        category   string
        mapName    string
    }{
        {"execve", "sys_enter_execve", "syscalls", "exec_events"},
        {"fork", "sched_process_fork", "sched", "fork_events"},
    }

    for _, prog := range programs {
        if err := l.loadProgram(prog.name, prog.tracepoint, prog.category, prog.mapName); err != nil {
            logger.Warn("Failed to load eBPF program", zap.Error(err))
            continue
        }
    }
    return nil
}

func (l *Loader) processExecEvent(data []byte) {
    var e ExecEvent
    if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
        return
    }

    filename := bytesToString(e.Filename[:])
    args := bytesToString(e.Args[:])
    cmdLine := filename + " " + args
    comm := bytesToString(e.Comm[:])

    // 如果cmdline为空，尝试从/proc读取
    emptyCmd := (cmdLine == " " || cmdLine == "" || filename == "")
    if emptyCmd {
        procPath := fmt.Sprintf("/proc/%d/cmdline", e.Pid)
        if procCmdline, err := os.ReadFile(procPath); err == nil {
            cmdLine = string(bytes.ReplaceAll(procCmdline, []byte{0}, []byte(" ")))
            cmdLine = strings.TrimSpace(cmdLine)
            // 使用Debug级别，避免日志刷屏
            logger.Debug("Read cmdline from /proc",
                zap.Int("pid", int(e.Pid)),
                zap.String("cmdline", cmdLine))
        } else {
            logger.Debug("Failed to read /proc cmdline",
                zap.Int("pid", int(e.Pid)),
                zap.Error(err))
        }
    }

    event := Event{
        EventID:     l.nextEventID(),
        HostID:      l.hostID,
        Hostname:    l.hostname,
        Timestamp:   time.Now().UnixMilli(),
        EventType:   "process_exec",
        ProcessName: comm,
        PID:         int(e.Pid),
        PPID:        int(e.Ppid),
        UID:         int(e.Uid),
        CommandLine: cmdLine,
        FilePath:    filename,
    }

    l.sendEvent(event)
}
```

---

## 4. Sigma规则匹配

### 4.1 规则加载

```go
type RuleLoader struct {
    rules map[string]*SigmaRule
    mu    sync.RWMutex
}

type SigmaRule struct {
    ID          string
    Title       string
    MitreID     string
    Severity    string
    Condition   string
    Selection   map[string]interface{}
}

func (r *RuleLoader) LoadFromDirectory(dir string) error {
    files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
    for _, file := range files {
        data, err := os.ReadFile(file)
        if err != nil {
            continue
        }
        
        var rule SigmaRule
        if err := yaml.Unmarshal(data, &rule); err != nil {
            continue
        }
        
        r.mu.Lock()
        r.rules[rule.ID] = &rule
        r.mu.Unlock()
    }
    return nil
}

func (r *RuleLoader) Match(event *Event) *SigmaRule {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for _, rule := range r.rules {
        if r.matchRule(event, rule) {
            return rule
        }
    }
    return nil
}

func (r *RuleLoader) matchRule(event *Event, rule *SigmaRule) bool {
    for field, patterns := range rule.Selection {
        var value string
        switch field {
        case "CommandLine":
            value = event.CommandLine
        case "ProcessName":
            value = event.ProcessName
        default:
            continue
        }

        matched := false
        for _, pattern := range patterns.([]interface{}) {
            if strings.Contains(value, pattern.(string)) {
                matched = true
                break
            }
        }
        if !matched {
            return false
        }
    }
    return true
}
```

---

## 5. gRPC客户端

### 5.1 连接管理

```go
type Client struct {
    conn       *grpc.ClientConn
    stream     pb.AgentService_ExecuteCommandClient
    eventChan  chan Event
    config     *Config
    ruleLoader *RuleLoader
}

func (c *Client) Connect() error {
    conn, err := grpc.Dial(c.config.ServerAddr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(),
        grpc.WithTimeout(10*time.Second))
    if err != nil {
        return err
    }
    c.conn = conn
    
    client := pb.NewAgentServiceClient(conn)
    stream, err := client.ExecuteCommand(context.Background())
    if err != nil {
        return err
    }
    c.stream = stream
    
    // 发送注册请求
    c.stream.Send(&pb.CommandRequest{
        Request: &pb.CommandRequest_Register{
            Register: &pb.RegisterRequest{
                HostId:      c.config.HostID,
                Hostname:    c.hostname,
                OsType:      runtime.GOOS,
                AgentVersion: "v5.2.0",
            },
        },
    })
    
    return nil
}
```

### 5.2 事件上报

```go
func (c *Client) ReportEvents(events []Event) error {
    pbEvents := make([]*pb.RuntimeEvent, 0, len(events))
    for _, e := range events {
        pbEvents = append(pbEvents, &pb.RuntimeEvent{
            EventId:     e.EventID,
            HostId:      c.config.HostID,
            Timestamp:   e.Timestamp,
            EventType:   e.EventType,
            ProcessName: e.ProcessName,
            Pid:         int32(e.PID),
            Ppid:        int32(e.PPID),
            CommandLine: e.CommandLine,
        })
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    resp, err := c.client.ReportEvent(ctx, &pb.ReportEventRequest{
        HostId: c.config.HostID,
        Events: pbEvents,
    })
    
    if err != nil {
        return err
    }
    
    logger.Debug("Events reported", zap.Int32("count", resp.ReceivedCount))
    return nil
}
```

### 5.3 规则同步

```go
func (c *Client) handleRuleUpdate(update *pb.RuleUpdateRequest) {
    switch update.Action {
    case "full_sync":
        // 全量同步
        c.ruleLoader.Clear()
        for _, rule := range update.Rules {
            c.ruleLoader.ApplyUpdate(rule.Action, rule.RuleId, []byte(rule.Content))
        }
        logger.Info("Full rule sync completed", zap.Int("count", len(update.Rules)))
        
    case "add":
        for _, rule := range update.Rules {
            c.ruleLoader.ApplyUpdate(rule.Action, rule.RuleId, []byte(rule.Content))
            logger.Info("Rule added", zap.String("rule_id", rule.RuleId))
        }
        
    case "delete":
        for _, rule := range update.Rules {
            c.ruleLoader.Remove(rule.RuleId)
            logger.Info("Rule deleted", zap.String("rule_id", rule.RuleId))
        }
    }
}
```

---

## 6. 阻断执行

### 6.1 阻断动作

```go
func (c *Client) executeBlock(cmd *pb.BlockCommand) error {
    switch cmd.Action {
    case "kill_process":
        return c.killProcess(cmd.Target)
    case "quarantine_file":
        return c.quarantineFile(cmd.Target)
    case "block_connection":
        return c.blockConnection(cmd.Target)
    case "disable_user":
        return c.disableUser(cmd.Target)
    default:
        return fmt.Errorf("unknown block action: %s", cmd.Action)
    }
}

func (c *Client) killProcess(pidStr string) error {
    pid, err := strconv.Atoi(pidStr)
    if err != nil {
        return err
    }
    
    process, err := os.FindProcess(pid)
    if err != nil {
        return err
    }
    
    if err := process.Kill(); err != nil {
        return err
    }
    
    logger.Info("Process killed", zap.Int("pid", pid))
    return nil
}

func (c *Client) quarantineFile(filePath string) error {
    quarantinePath := filepath.Join(c.config.QuarantineDir, filepath.Base(filePath))
    
    if err := os.Rename(filePath, quarantinePath); err != nil {
        return err
    }
    
    logger.Info("File quarantined", 
        zap.String("original", filePath),
        zap.String("quarantine", quarantinePath))
    return nil
}
```

---

## 7. 日志管理

### 7.1 日志级别

```go
func Init(logDir string, level string) error {
    var logLevel zapcore.Level
    switch level {
    case "debug":
        logLevel = zapcore.DebugLevel
    case "warn":
        logLevel = zapcore.WarnLevel
    case "error":
        logLevel = zapcore.ErrorLevel
    default:
        logLevel = zapcore.InfoLevel
    }
    
    // 使用lumberjack进行日志轮转
    lumberjackLogger := &lumberjack.Logger{
        Filename:   filepath.Join(logDir, "agent.log"),
        MaxSize:    10,   // 10MB
        MaxBackups: 5,    // 5个备份
        MaxAge:     7,    // 7天
        Compress:   true,
    }
    
    core := zapcore.NewCore(
        zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
        zapcore.AddSync(lumberjackLogger),
        logLevel,
    )
    
    Logger = zap.New(core, zap.AddCaller())
    return nil
}
```

### 7.2 日志输出示例

**Info级别**（生产环境）：
```json
{"level":"info","msg":"Rule matched","rule_id":"t1113_screen_capture","title":"Screen Capture","mitre_id":"t1113","severity":"medium"}
{"level":"info","msg":"Block command received","command_id":"BLK-xxx","action":"kill_process","target":"12345"}
```

**Debug级别**（调试环境）：
```json
{"level":"debug","msg":"Ringbuffer event received","program":"execve","size":544}
{"level":"debug","msg":"Event captured","type":"process_exec","cmd":"bash","pid":12345}
{"level":"debug","msg":"Read cmdline from /proc","pid":12345,"cmdline":"/bin/bash -i"}
```

---

## 8. 构建与部署

### 8.1 Makefile

```makefile
.PHONY: all build clean upload

BPF_DIR = internal/ebpf/bpf
DIST_DIR = dist

all: build-bpf build-agent package

build-bpf:
	@echo "Building eBPF programs..."
	clang -O2 -target bpf -c $(BPF_DIR)/execve.bpf.c -o $(BPF_DIR)/obj/execve.bpf.o
	clang -O2 -target bpf -c $(BPF_DIR)/fork.bpf.c -o $(BPF_DIR)/obj/fork.bpf.o

build-agent:
	@echo "Building agent..."
	GOOS=linux GOARCH=amd64 go build -o $(DIST_DIR)/aegis-agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 go build -o $(DIST_DIR)/aegis-agent-linux-arm64 ./cmd/agent

package:
	@echo "Packaging agent..."
	cd $(DIST_DIR) && tar -czf aegis-agent.tar.gz aegis-agent-linux-amd64 bpf/*.bpf.o

upload:
	mc cp $(DIST_DIR)/aegis-agent.tar.gz myminio/agent-artifacts/

clean:
	rm -rf $(DIST_DIR)
	rm -f $(BPF_DIR)/obj/*.bpf.o
```

### 8.2 安装脚本

```bash
# /opt/aegis-agent/uninstall.sh
#!/bin/bash
echo "=== Aegis Agent 卸载脚本 ==="

echo "[1/4] 停止 Agent 服务..."
systemctl stop aegis-agent 2>/dev/null || true

echo "[2/4] 删除 systemd 服务..."
systemctl disable aegis-agent 2>/dev/null || true
rm -f /etc/systemd/system/aegis-agent.service
systemctl daemon-reload

echo "[3/4] 删除安装目录..."
rm -rf /opt/aegis-agent

echo "[4/4] 删除配置目录..."
rm -rf /etc/aegis-agent

echo "=== 卸载完成 ==="
```

---

## 9. 系统服务

```ini
# /etc/systemd/system/aegis-agent.service
[Unit]
Description=Aegis Check Agent
After=network.target

[Service]
Type=simple
ExecStart=/opt/aegis-agent/aegis-agent
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

---

**文档结束**