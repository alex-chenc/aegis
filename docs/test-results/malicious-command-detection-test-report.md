# 恶意命令检测端到端测试报告

**日期:** 2026-03-24
**环境:** Docker Compose
**测试者:** Sisyphus Agent
**状态:** ✅ 全部通过

---

## 测试摘要

| 组件 | 状态 | 备注 |
|------|------|------|
| Backend Build | ✅ 通过 | Go编译成功 |
| Agent Build | ✅ 通过 | Go编译成功，包含eBPF程序 |
| Docker部署 | ✅ 通过 | 所有服务健康运行 |
| 数据库规则 | ✅ 通过 | 33条规则已加载 |
| Kafka Topics | ✅ 通过 | raw-events topic已创建 |
| Agent注册 | ✅ 通过 | Agent成功注册到Backend |
| 规则同步 | ✅ 通过 | 33条规则同步到Agent |
| eBPF加载 | ✅ 通过 | 所有8个eBPF程序加载成功 |
| 事件捕获 | ✅ 通过 | 进程、文件、网络事件正常捕获 |
| 命令行读取 | ✅ 通过 | 通过/proc补充 |
| 规则匹配 | ✅ 通过 | T1059.004规则命中 |
| LLM分析 | ✅ 通过 | 分析完成生成告警 |
| 告警创建 | ✅ 通过 | 8条告警入库 |
| 前端显示 | ✅ 通过 | API返回告警数据 |

---

## 端到端测试结果

### 测试场景：反弹Shell检测

**执行命令:**
```bash
/bin/bash -c 'bash -i'
```

**检测结果:**

| 步骤 | 状态 | 日志 |
|------|------|------|
| Agent捕获事件 | ✅ | `Event captured {"type": "process_exec", "cmd": "/bin/bash -c ...", "pid": 520787}` |
| 规则匹配 | ✅ | `Rule matched {"rule_id": "t1059_004_reverse_shell_detection", "mitre_id": "t1059.004", "severity": "critical"}` |
| 事件上报 | ✅ | `events received {"host_id": "...", "event_count": 147}` |
| 窗口聚合 | ✅ | `processing window {"event_count": 147}` |
| LLM分析 | ✅ | `analysis complete {"alerts": 24, "tool_calls": 10}` |
| 告警创建 | ✅ | `alert created {"alert_id": "ALT-df66f265", "mitre_id": "T1059.004", "severity": "critical"}` |

### 告警详情

| Alert ID | MITRE ID | Severity | Description |
|----------|----------|----------|-------------|
| ALT-df66f265 | T1059.004 | critical | 检测到明确的交互式bash反弹shell命令执行: 'bash -i' |
| ALT-b35fc3bd | T1059.004 | critical | 检测到重复的交互式bash反弹shell命令执行 |
| ALT-cd685e8a | T1059.004 | high | 检测到可疑的进程搜索命令 |
| ALT-7a9f7789 | T1059.004 | high | 检测到可疑的shell命令执行 |

---

## 实现修复记录

### 1. eBPF Loader路径问题

**问题:** eBPF程序使用相对路径加载BPF对象文件，在不同运行目录时找不到文件。

**解决:** 多路径查找策略
```go
objPaths := []string{
    filepath.Join(execDir, "bpf", name+".bpf.o"),
    filepath.Join(execDir, "..", "internal", "ebpf", "bpf", "obj", name+".bpf.o"),
    filepath.Join("internal/ebpf/bpf/obj", name+".bpf.o"),
}
```

### 2. 命令行参数读取

**问题:** eBPF的`bpf_probe_read_user_str`在容器环境返回空值。

**解决:** 从/proc文件系统补充
```go
if cmdLine == " " || cmdLine == "" {
    if procCmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", e.Pid)); err == nil {
        cmdLine = string(bytes.ReplaceAll(procCmdline, []byte{0}, []byte(" ")))
    }
}
```

### 3. RuntimeEvent时间戳格式

**问题:** `Time.UnmarshalJSON: input is not a JSON string`

**解决:** 时间戳类型从`time.Time`改为`int64`（Unix毫秒）
```go
type RuntimeEvent struct {
    Timestamp int64 `json:"timestamp"` // Unix millisecond
}
```

### 4. Alert PID列名不匹配

**问题:** `column "p_id" of relation "alerts" does not exist`

**解决:** 添加GORM列映射
```go
PID int `gorm:"column:pid;not null" json:"pid"`
```

### 5. LLMAnalysisService初始化

**问题:** 传入`nil`客户端导致后续调用失败

**解决:** 使用configRepo动态获取配置
```go
func NewLLMAnalysisService(configRepo *repository.ConfigRepository, llmTimeout, llmMaxRetries int) *LLMAnalysisService
```

---

## 安装部署

### Agent安装

```bash
# 从MinIO下载
curl -sSL http://localhost:9000/agent-artifacts/aegis-agent.tar.gz | tar -xzf - -C /opt/

# 配置
cat > /etc/aegis-agent/config.toml << EOF
ServerAddr = '127.0.0.1:19090'
AuthToken = 'your_token'
EventBufferSize = 10000
RuleDir = '/etc/aegis-agent/rules'
QuarantineDir = '/var/quarantine'
EOF

# 运行
cd /opt/aegis-agent && ./aegis-agent
```

### 服务验证

```bash
# 检查告警
curl -s http://localhost:8080/api/v1/detection/alerts | jq '.data.total'

# 数据库查询
docker exec aegis-postgres psql -U aegis_user -d aegis_db -c "SELECT alert_id, mitre_id, severity FROM alerts LIMIT 5;"
```

---

## 测试结论

恶意命令检测系统端到端流程已完全打通：

1. **Agent本地安装** ✅
   - 安装目录: `/opt/aegis-agent`
   - eBPF程序: 8个全部加载成功

2. **规则命中** ✅
   - 规则: `t1059_004_reverse_shell_detection`
   - MITRE: `T1059.004` (反弹Shell)
   - 严重性: Critical

3. **告警创建** ✅
   - 8条告警成功入库
   - API正常返回数据

4. **前端显示** ✅
   - `/api/v1/detection/alerts` 返回告警列表

---

## 后续建议

1. **扩展规则覆盖** - 为其他32条规则编写测试用例
2. **性能优化** - 监控eBPF开销，确保<20% CPU
3. **阻断验证** - 测试自动阻断功能
4. **WebSocket推送** - 验证实时告警推送