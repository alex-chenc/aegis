# Aegis System Design Update - V5.2

**版本**: 5.2  
**日期**: 2026-03-26  
**类型**: 功能修复与体验优化  

---

## 更新概述

本次更新主要修复V5.1版本遗留的功能问题，优化用户交互体验，完善事件持久化和中文本地化支持。

详细变更日志请参考: [CHANGELOG_v5.2.md](./CHANGELOG_v5.2.md)

---

## 1. 阻断策略实时更新

### 1.1 问题描述

阻断策略页面的开关按钮（启用/自动阻断/自动处置）点击后，需要刷新页面才能看到状态变化，用户体验不佳。

### 1.2 技术方案

**架构设计**:

```
前端Policies.vue                Backend                    WebSocket服务
      │                            │                            │
      │  1. PUT /block-policies   │                            │
      │ ─────────────────────────>│                            │
      │                            │  2. 更新数据库              │
      │                            │ ──────>                    │
      │                            │                            │
      │                            │  3. BroadcastPolicyUpdate  │
      │                            │ ──────────────────────────>│
      │                            │                            │
      │  4. WebSocket消息          │                            │
      │ <─────────────────────────────────────────────────────│
      │                            │                            │
      │  5. 更新本地状态            │                            │
      │ ──────>                    │                            │
```

**消息类型**:

```typescript
interface WSMessage {
    type: 'alert' | 'policy_update' | 'rule_update'
    data: any
}
```

### 1.3 实现要点

1. **后端广播**: `UpdateBlockPolicy` API完成后调用`wsService.BroadcastPolicyUpdate(policy)`
2. **前端监听**: WebSocket连接建立后监听`policy_update`消息类型
3. **本地更新**: 按钮点击时立即更新本地状态，同时等待WebSocket确认

### 1.4 容错处理

- WebSocket断开后自动重连（间隔3秒）
- 本地状态更新失败不影响API调用
- 广播失败不影响API响应

---

## 2. 自动处置功能

### 2.1 问题描述

开启自动处置后，符合MITRE的告警不会自动变更为"已处置"状态。

### 2.2 业务流程

```
Agent上报事件
    │
    ▼
Backend匹配规则，创建Alert
    │
    ▼
查询BlockPolicy (WHERE mitre_id = alert.mitre_id)
    │
    ├── 策略不存在 → 跳过
    ├── 策略未启用 → 跳过
    └── 策略已启用 && auto_dispose=true
            │
            ▼
        Alert.status = "resolved"
        Alert.auto_dispose = true
            │
            ▼
        更新数据库
        广播Alert更新
```

### 2.3 与自动阻断的区别

| 功能 | 触发条件 | 执行动作 |
|------|----------|----------|
| 自动阻断 | `enabled=true && auto_block=true` | 下发阻断指令给Agent |
| 自动处置 | `enabled=true && auto_dispose=true` | 直接更新Alert状态为resolved |

---

## 3. 事件持久化

### 3.1 数据流变更

**变更前**:
```
Agent → gRPC ReportEvent → Kafka → (无持久化)
```

**变更后**:
```
Agent → gRPC ReportEvent → Kafka
                            └→ runtime_events表 (持久化)
```

### 3.2 runtime_events表结构

```sql
CREATE TABLE runtime_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(64) UNIQUE NOT NULL,
    host_id UUID NOT NULL,
    event_type VARCHAR(32) NOT NULL,
    event_data JSONB NOT NULL,
    matched_rule_id VARCHAR(128),
    rule_title VARCHAR(255),
    mitre_id VARCHAR(20),
    severity VARCHAR(16),
    pid INTEGER,
    command_line TEXT,
    timestamp BIGINT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    aggregated BOOLEAN DEFAULT FALSE
);

CREATE INDEX idx_runtime_events_timestamp ON runtime_events(timestamp);
CREATE INDEX idx_runtime_events_aggregated ON runtime_events(aggregated);
CREATE INDEX idx_runtime_events_host_id ON runtime_events(host_id);
```

### 3.3 AI降噪查询

```go
func (r *RuntimeEventRepository) FindUnaggregated(startTime, endTime int64, hostIDs []string) ([]model.RuntimeEvent, error) {
    query := r.db.Where("aggregated = ? AND timestamp >= ? AND timestamp <= ?", false, startTime, endTime)
    if len(hostIDs) > 0 {
        query = query.Where("host_id IN ?", hostIDs)
    }
    return query.Order("timestamp ASC").Find(&events).Error
}
```

---

## 4. MITRE中文映射

### 4.1 设计原则

1. **映射数据**: 覆盖现有规则中使用的32个MITRE技术
2. **存储方式**: Go代码中硬编码（初始化快、查询快）
3. **大小写兼容**: 统一转换为大写后匹配

### 4.2 使用方式

```go
// 创建告警时自动填充
mitreName, mitreDesc := model.GetMITREChineseDescription(event.MitreId)
if mitreName != "" {
    alert.MitreName = mitreName
    alert.Description = mitreDesc
}
```

### 4.3 扩展方式

如需添加新的MITRE技术映射，编辑`backend/internal/model/mitre_mapping.go`:

```go
var mitreChineseMapping = map[string]struct {
    Name        string
    Description string
}{
    // 添加新映射
    "T1566.001": {
        Name:        "钓鱼附件",
        Description: "攻击者通过钓鱼邮件附件投放恶意软件",
    },
}
```

---

## 5. 前端优化

### 5.1 分页默认值

```typescript
// 阻断策略页面
const pageSize = ref(10)  // 从20改为10

// 告警列表页面
const pageSize = ref(10)  // 保持10
```

### 5.2 按钮交互优化

**变更前**: 调用API后只显示Toast提示，需刷新页面看到变化

**变更后**: 调用API成功后立即更新本地数据，同时监听WebSocket确认

```typescript
async function handleToggleEnabled(mitreId: string, enabled: boolean) {
    try {
        await api.updateBlockPolicy(mitreId, { enabled })
        // 立即更新本地状态
        const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
        if (index !== -1) {
            blockPolicies.value[index].enabled = enabled
        }
        ElMessage.success('策略启用状态已更新')
    } catch (e: any) {
        ElMessage.error(e.message || '更新失败')
    }
}
```

---

## 6. Agent日志优化

### 6.1 变更内容

| 日志内容 | 变更前 | 变更后 |
|----------|--------|--------|
| cmdline读取成功 | `logger.Info` | `logger.Debug` |
| cmdline读取失败 | `logger.Info` | `logger.Debug` |

### 6.2 效果

生产环境（`LogLevel=info`）不再输出大量cmdline日志，仅保留规则匹配和命令执行日志。

---

## 7. 测试覆盖

### 7.1 自动化测试

```bash
# API正向测试
./tests/api/positive-tests.sh
# 覆盖: health, config, hosts, templates, alerts, block-policies, blocks, rules, tool-calls, tasks, agent

# API反向测试
./tests/api/negative-tests.sh
# 覆盖: 无效端点、无效方法、无效参数、无效请求体、边界情况
```

### 7.2 手动测试清单

- [ ] 阻断策略页面开关点击后状态立即更新
- [ ] 开启自动处置后新告警自动变为"已处置"
- [ ] AI降噪显示正确的事件数量
- [ ] 告警显示中文MITRE名称和描述
- [ ] Agent日志不再刷屏

---

## 8. 部署检查清单

- [ ] 后端构建: `cd backend && make build`
- [ ] 前端构建: `cd frontend && npm run build`
- [ ] Agent构建: `cd agent && make all`
- [ ] Docker镜像构建
- [ ] Agent上传到MinIO
- [ ] 数据库检查: `runtime_events`表存在
- [ ] API测试通过
- [ ] WebSocket连接正常

---

## 9. 总结

V5.2版本主要修复了用户反馈的功能问题，提升了系统的可用性和用户体验：

1. **实时更新** - WebSocket广播策略变更，无需刷新页面
2. **自动处置** - 完整实现自动处置功能
3. **事件持久化** - AI降噪可正确统计事件数量
4. **中文本地化** - MITRE技术中文描述
5. **日志优化** - 减少不必要的日志输出
6. **交互优化** - 分页默认值、按钮状态同步

---

## 1. Agent日志级别配置

### 1.1 问题描述

V5.0版本Agent日志级别硬编码为Info，生产环境调试困难，开发环境日志不足。

### 1.2 解决方案

添加`LogLevel`配置项，支持在配置文件中动态调整日志级别。

**配置文件**: `/etc/aegis-agent/config.toml`

```toml
ServerAddr = '127.0.0.1:19090'
AuthToken = 'a_very_secret_agent_token'
HostID = '53efa0f7-06c5-4b10-83c8-019327bcd0a2'
EventBufferSize = 10000
RuleDir = '/etc/aegis-agent/rules'
QuarantineDir = '/var/quarantine'
LogLevel = 'info'  # 新增配置项
```

**支持的日志级别**:
- `debug`: 详细调试信息（包括每个捕获的事件）
- `info`: 常规信息（仅规则匹配和命令接收）
- `warn`: 警告信息
- `error`: 错误信息

### 1.3 代码修改

**文件**: `agent/internal/config/config.go`
```go
type Config struct {
    // ... 其他字段
    LogLevel string `toml:"LogLevel"`
}

func LoadConfig() (*Config, error) {
    // ...
    if cfg.LogLevel == "" {
        cfg.LogLevel = "info"  // 默认info级别
        updated = true
    }
    // ...
}
```

**文件**: `agent/internal/logger/logger.go`
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
    // ...
}
```

### 1.4 日志输出示例

**Info级别**（生产环境推荐）:
```json
{"level":"info","timestamp":"2026-03-25T16:32:07.121+0800","caller":"client/client.go:202","msg":"Received block command via stream","command_id":"BLK-9973b971","action":"kill_process","target":"2069406"}
{"level":"info","timestamp":"2026-03-25T16:32:07.122+0800","caller":"client/client.go:215","msg":"Block command failed","command_id":"BLK-9973b971","error":"failed to kill process 2069406: os: process already finished"}
```

**Debug级别**（开发/调试环境）:
```json
{"level":"debug","timestamp":"2026-03-25T16:24:18.571+0800","caller":"ebpf/loader.go:150","msg":"Ringbuffer event received","program":"execve","size":544}
{"level":"debug","timestamp":"2026-03-25T16:24:18.574+0800","caller":"ebpf/pipeline.go:88","msg":"Event captured","type":"process_exec","cmd":"runc init","pid":1933506}
```

---

## 2. 阻断策略初始化机制

### 2.1 问题描述

V5.0版本阻断策略需要手动创建，新部署环境无默认策略，导致阻断功能无法使用。

### 2.2 解决方案

创建Seed机制，服务启动时自动初始化36条默认阻断策略。

### 2.3 默认策略列表

| MITRE ID | 策略名称 | 默认动作 | 自动阻断 | 自动处置 |
|----------|----------|----------|----------|----------|
| t1003 | OS Credential Dumping | quarantine_file | ❌ | ✅ |
| t1003.001 | LSASS Memory Dump | kill_process | ❌ | ❌ |
| t1003.008 | /etc/passwd and /etc/shadow Access | kill_process | ❌ | ❌ |
| t1005 | Local Data Collection | kill_process | ❌ | ❌ |
| t1021.002 | SMB Lateral Movement | kill_process | ❌ | ❌ |
| t1021.004 | SSH Lateral Movement | kill_process | ❌ | ❌ |
| t1041 | C2 Exfiltration | block_connection | ❌ | ❌ |
| t1046 | Network Service Discovery | kill_process | ❌ | ❌ |
| t1048 | Alternative Protocol Exfiltration | block_connection | ❌ | ❌ |
| t1053.003 | Cron Job Persistence | kill_process | ❌ | ❌ |
| t1059.001 | PowerShell Suspicious Commands | kill_process | ❌ | ❌ |
| t1059.003 | Windows Command Shell | kill_process | ❌ | ❌ |
| t1059.004 | Unix Shell | kill_process | ❌ | ❌ |
| t1068 | Exploitation for Privilege Escalation | kill_process | ❌ | ❌ |
| t1070.002 | Log Clearing | kill_process | ❌ | ❌ |
| t1082 | System Information Discovery | kill_process | ❌ | ❌ |
| t1110 | Brute Force | kill_process | ❌ | ❌ |
| t1113 | Screen Capture | kill_process | ❌ | ❌ |
| t1190 | Exploit Public-Facing Application | kill_process | ❌ | ❌ |
| t1222.002 | File Permission Modification | kill_process | ❌ | ❌ |
| t1486 | Data Encrypted for Impact | kill_process | ❌ | ❌ |
| t1489 | Service Stop | kill_process | ❌ | ❌ |
| t1490 | Inhibit System Recovery | kill_process | ❌ | ❌ |
| t1547.001 | Registry Run Keys Persistence | kill_process | ❌ | ❌ |
| t1548 | Abuse Elevation Control Mechanism | kill_process | ❌ | ❌ |
| t1548.001 | Setuid and Setgid Abuse | kill_process | ❌ | ❌ |
| t1548.003 | Sudo Abuse | kill_process | ❌ | ❌ |
| t1572 | Protocol Tunneling | block_connection | ❌ | ❌ |
| t1573 | Encrypted Channel | kill_process | ❌ | ❌ |
| t1587 | Capability Development | kill_process | ❌ | ❌ |
| t1588 | Capability Obtainment | kill_process | ❌ | ❌ |
| t1592 | Host Information Gathering | kill_process | ❌ | ❌ |
| t1595 | Active Scanning | kill_process | ❌ | ❌ |

### 2.4 代码实现

**文件**: `backend/internal/seed/block_policy.go`
```go
package seed

var DefaultBlockPolicies = []model.BlockPolicy{
    {MitreID: "t1003", MitreName: "OS Credential Dumping", Enabled: true, AutoBlock: false, AutoDispose: true, Action: "quarantine_file"},
    // ... 其他35条策略
}

func SeedBlockPolicies(db *gorm.DB) {
    repo := repository.NewBlockPolicyRepository(db)
    for _, policy := range DefaultBlockPolicies {
        var existing model.BlockPolicy
        err := db.Where("mitre_id = ?", policy.MitreID).First(&existing).Error
        if err == gorm.ErrRecordNotFound {
            repo.Create(&policy)
        }
    }
}
```

**文件**: `backend/cmd/server/main.go`
```go
func main() {
    // ... 初始化代码
    
    // Load detection rules on startup
    if err := ruleLoader.LoadFromDirectory(ctx, "config/rules"); err != nil {
        logger.Warn("failed to load rules from directory", zap.Error(err))
    }
    
    // Seed default block policies
    seed.SeedBlockPolicies(db)
    
    // ... 启动服务
}
```

### 2.5 持久化保证

- 策略数据存储在PostgreSQL `block_policies`表
- 重启服务不会丢失已配置的策略
- 仅当策略不存在时才插入，不会覆盖用户自定义配置

---

## 3. 阻断策略分页功能

### 3.1 问题描述

V5.0版本阻断策略列表无分页，36条策略一次性加载，影响页面性能。

### 3.2 API设计

**请求**:
```
GET /api/v1/detection/block-policies?page=1&page_size=20&query=t1003
```

**响应**:
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "data": [...],
        "total": 36,
        "page": 1,
        "page_size": 20,
        "total_page": 2
    }
}
```

### 3.3 代码实现

**文件**: `backend/internal/repository/block_policy_repo.go`
```go
func (r *BlockPolicyRepository) ListPaginated(page, pageSize int, query string) ([]model.BlockPolicy, int64, error) {
    var policies []model.BlockPolicy
    var total int64
    
    db := r.db.Model(&model.BlockPolicy{})
    if query != "" {
        db = db.Where("mitre_id ILIKE ? OR mitre_name ILIKE ?", "%"+query+"%", "%"+query+"%")
    }
    
    if err := db.Count(&total).Error; err != nil {
        return nil, 0, err
    }
    
    offset := (page - 1) * pageSize
    err := db.Order("mitre_id").Offset(offset).Limit(pageSize).Find(&policies).Error
    return policies, total, err
}
```

**文件**: `backend/internal/api/handler/detection_handler.go`
```go
func (h *DetectionHandler) ListBlockPolicies(c *gin.Context) {
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
    query := c.Query("query")
    
    policies, total, err := h.blockPolicyRepo.ListPaginated(page, pageSize, query)
    
    c.JSON(http.StatusOK, gin.H{
        "code":    0,
        "message": "success",
        "data": gin.H{
            "data":       policies,
            "total":      total,
            "page":       page,
            "page_size":  pageSize,
            "total_page": (int(total) + pageSize - 1) / pageSize,
        },
    })
}
```

### 3.4 前端实现

**文件**: `frontend/src/views/detection/Policies.vue`
```vue
<template>
  <div class="detection-policies-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>阻断策略 (共 {{ total }} 条)</span>
          <el-button size="small" @click="loadPolicies">刷新</el-button>
        </div>
      </template>
      
      <el-table :data="blockPolicies" border stripe>
        <!-- 列定义 -->
      </el-table>
      
      <div class="pagination-container">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="loadPolicies"
          @current-change="loadPolicies"
        />
      </div>
    </el-card>
  </div>
</template>
```

---

## 4. AI降噪事件数量修正

### 4.1 问题描述

V5.0版本AI降噪始终显示100个事件，实际时间段内可能没有100个事件，导致用户困惑。

### 4.2 原因分析

原代码硬编码查询100条pending状态的告警：
```go
alerts, _, err := h.alertRepo.List(1, 100, map[string]interface{}{
    "status": "pending",
})
```

### 4.3 修复方案

改为从`runtime_events`表查询实际时间段内的事件数量：

**文件**: `backend/internal/api/handler/detection_handler.go`
```go
func (h *DetectionHandler) StartLLMAggregation(c *gin.Context) {
    var body struct {
        StartTime   string   `json:"start_time"`
        EndTime     string   `json:"end_time"`
        HostIDs     []string `json:"host_ids"`
        AutoDispose bool     `json:"auto_dispose"`
    }
    c.ShouldBindJSON(&body)
    
    startTime, _ := time.Parse(time.RFC3339, body.StartTime)
    endTime, _ := time.Parse(time.RFC3339, body.EndTime)
    
    // 查询实际时间段内的事件
    startTs := startTime.UnixMilli()
    endTs := endTime.UnixMilli()
    events, err := h.runtimeEventRepo.FindUnaggregated(startTs, endTs, body.HostIDs)
    
    agg.EventCount = len(events)  // 使用实际事件数量
    h.llmAggregationRepo.Update(agg)
    
    // ... 后续处理
}
```

### 4.4 前端显示

**文件**: `frontend/src/views/detection/Alerts.vue`
```vue
<el-descriptions>
  <el-descriptions-item label="事件数量">{{ aiDenoiseResult?.event_count }}</el-descriptions-item>
  <el-descriptions-item label="告警数量">{{ aiDenoiseResult?.alert_count }}</el-descriptions-item>
  <el-descriptions-item label="AI判定">{{ aiDenoiseResult?.ai_judged_count }}</el-descriptions-item>
  <el-descriptions-item label="自动处置">{{ aiDenoiseResult?.auto_dispose_count }}</el-descriptions-item>
</el-descriptions>
```

---

## 5. MITRE ID与阻断策略关联

### 5.1 问题描述

用户困惑告警列表的MITRE列是什么，误以为是阻断策略ID，但发现告警没有相应的阻断策略。

### 5.2 设计说明

**MITRE ID含义**: MITRE ATT&CK技术编号，标识攻击技术类型，如：
- `t1059.004` - Unix Shell (命令执行)
- `t1003` - OS Credential Dumping (凭证窃取)
- `t1548.003` - Sudo Abuse (权限滥用)

**与阻断策略的关系**:
- 每条阻断策略对应一个MITRE ID
- 告警的`mitre_id`字段与阻断策略的`mitre_id`关联
- 当告警命中时，系统查找对应MITRE ID的阻断策略，根据策略配置决定是否自动阻断

**关联查询**:
```sql
SELECT a.*, b.action, b.auto_block 
FROM alerts a
LEFT JOIN block_policies b ON LOWER(a.mitre_id) = LOWER(b.mitre_id)
WHERE a.alert_id = 'ALT-xxx';
```

### 5.3 大小写统一

所有MITRE ID统一为小写存储和比较，避免`t1003`与`T1003`不匹配问题。

**修复代码**:
```sql
-- 数据库修复
UPDATE block_policies SET mitre_id = LOWER(mitre_id);
UPDATE sigma_rules SET mitre_id = LOWER(mitre_id);
UPDATE alerts SET mitre_id = LOWER(mitre_id);
```

---

## 6. 规则同步机制优化

### 6.1 问题描述

V5.0版本Agent连接Backend后需要手动触发规则同步，禁用规则后Agent仍可能匹配已禁用的规则。

### 6.2 解决方案

1. **连接时自动推送**: Agent连接成功后，Backend自动推送所有active状态的规则
2. **禁用时实时同步**: 规则状态变更时，Backend通过双向流实时通知Agent删除规则

### 6.3 实现流程

```
Agent连接Backend (gRPC双向流)
    ↓
Backend检测新连接
    ↓
推送所有active规则 (full_sync)
    ↓
Agent接收并加载规则
    ↓
管理员禁用规则
    ↓
Backend通过双向流发送删除通知 (delete)
    ↓
Agent移除规则
```

### 6.4 代码实现

**Backend推送规则**:
```go
// backend/internal/grpc_server/server.go
func (s *GRPCServer) pushActiveRulesToAgent(hostID uuid.UUID, conn *AgentConnection) {
    time.Sleep(1 * time.Second)  // 等待连接稳定
    
    rules, _ := s.sigmaRuleRepo.GetActiveAndExperimental()
    
    updates := make([]*pb.RuleUpdate, 0, len(rules))
    for _, rule := range rules {
        updates = append(updates, &pb.RuleUpdate{
            RuleId:  rule.RuleID,
            Action:  "add",
            Content: rule.Content,
        })
    }
    
    conn.Stream.Send(&pb.CommandRequest{
        Request: &pb.CommandRequest_RuleUpdate{
            RuleUpdate: &pb.RuleUpdateRequest{
                Action: "full_sync",
                Rules:  updates,
            },
        },
    })
}

func (s *GRPCServer) BroadcastRuleUpdate(update *pb.RuleUpdate) {
    s.agentConnections.Range(func(key, value interface{}) bool {
        conn := value.(*AgentConnection)
        conn.Stream.Send(&pb.CommandRequest{
            Request: &pb.CommandRequest_RuleUpdate{
                RuleUpdate: update,
            },
        })
        return true
    })
}
```

**Agent接收规则**:
```go
// agent/internal/client/client.go
func (c *Client) run() {
    for {
        req, err := c.stream.Recv()
        if err != nil {
            return
        }
        
        if ruleUpdate := req.GetRuleUpdate(); ruleUpdate != nil {
            c.applyRuleUpdate(ruleUpdate)
        }
        
        if block := req.GetBlock(); block != nil {
            c.handleBlockCommand(block)
        }
    }
}

func (c *Client) applyRuleUpdate(req *pb.RuleUpdateRequest) {
    for _, rule := range req.Rules {
        c.ruleLoader.ApplyUpdate(rule.Action, rule.RuleId, []byte(rule.Content))
        if rule.Action == "delete" {
            logger.Info("Rule deleted", zap.String("rule_id", rule.RuleId))
        }
    }
}
```

### 6.5 Proto定义

```protobuf
message CommandRequest {
  oneof request {
    CommandExecute execute = 1;
    CommandResult result = 2;
    RuleUpdateRequest rule_update = 3;
    BlockCommand block = 4;
  }
}
```

---

## 7. 阻断功能验证

### 7.1 阻断流程

```
用户点击阻断按钮
    ↓
Backend调用ExecuteBlockCommand
    ↓
通过双向流发送BlockCommand到Agent
    ↓
Agent接收并执行阻断
    ↓
Agent记录执行结果（日志）
    ↓
Backend更新阻断状态和消息
```

### 7.2 日志验证

**Backend日志**:
```json
{"level":"info","timestamp":"2026-03-25T08:23:54.703+0000","msg":"block command sent via stream","command_id":"BLK-de1e729a"}
```

**Agent日志**:
```json
{"level":"info","timestamp":"2026-03-25T16:32:07.121+0800","caller":"client/client.go:202","msg":"Received block command via stream","command_id":"BLK-9973b971","action":"kill_process","target":"2069406"}
{"level":"error","timestamp":"2026-03-25T16:32:07.122+0800","caller":"client/client.go:215","msg":"Block command failed","command_id":"BLK-9973b971","error":"failed to kill process 2069406: os: process already finished"}
```

### 7.3 状态说明

| 状态 | 说明 |
|------|------|
| `blocking` | 阻断指令已下发，等待Agent执行 |
| `success` | Agent执行成功 |
| `failed` | Agent执行失败，显示失败原因 |

---

## 8. 测试验证

### 8.1 API测试

```bash
# 阻断策略分页测试
curl "http://localhost:8080/api/v1/detection/block-policies?page=1&page_size=10"

# 响应
{
    "code": 0,
    "data": {
        "data": [...],
        "total": 36,
        "page": 1,
        "page_size": 10,
        "total_page": 4
    }
}
```

### 8.2 阻断功能测试

```bash
# 阻断告警
curl -X POST "http://localhost:8080/api/v1/detection/alerts/ALT-xxx/block" \
  -H "Content-Type: application/json" \
  -d '{"action": "kill_process"}'

# 响应
{
    "code": 0,
    "data": {
        "success": true,
        "message": "阻断成功"
    }
}
```

### 8.3 规则同步测试

```bash
# 禁用规则
curl -X PUT "http://localhost:8080/api/v1/detection/rules/t1059_004_reverse_shell_detection/status" \
  -H "Content-Type: application/json" \
  -d '{"status": "disabled"}'

# Agent日志应显示
{"level":"info","msg":"Sigma rule deleted","rule_id":"t1059_004_reverse_shell_detection"}
```

---

## 9. 部署说明

### 9.1 Agent部署

1. 下载最新Agent包
2. 编辑配置文件`/etc/aegis-agent/config.toml`
3. 设置`LogLevel = 'info'`（生产环境）或`LogLevel = 'debug'`（调试环境）
4. 启动Agent: `systemctl start aegis-agent`

### 9.2 Backend部署

1. Backend启动时自动初始化36条默认阻断策略
2. 如已有自定义策略，不会被覆盖
3. 首次部署后检查`block_policies`表确认策略已创建

### 9.3 数据库迁移

```sql
-- 统一MITRE ID大小写（仅首次部署需要）
UPDATE block_policies SET mitre_id = LOWER(mitre_id);
UPDATE sigma_rules SET mitre_id = LOWER(mitre_id);
UPDATE alerts SET mitre_id = LOWER(mitre_id);

-- 添加分页所需索引
CREATE INDEX IF NOT EXISTS idx_block_policies_mitre_name ON block_policies(mitre_name);
```

---

## 10. 总结

V5.1版本主要增强了生产环境的可维护性和用户体验：

1. **日志级别配置** - 支持灵活调整日志详细程度
2. **默认策略初始化** - 新部署环境开箱即用
3. **分页功能** - 提升大数据量下的页面性能
4. **AI降噪修正** - 显示真实事件数量
5. **规则同步优化** - 连接自动推送，禁用实时同步
6. **阻断验证** - 完整日志链路，状态清晰

所有修改均已通过完整测试，可安全部署到生产环境。
