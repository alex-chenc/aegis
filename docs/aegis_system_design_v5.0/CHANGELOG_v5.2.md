# Aegis V5.2 版本变更日志

**版本**: 5.2
**日期**: 2026-03-26
**状态**: 已完成

---

## 1. 版本概述

V5.2版本在V5.1基础上，修复了多项功能问题并增强用户体验：

- 阻断策略页面实时更新（WebSocket广播）
- 自动处置功能完整实现
- 事件持久化支持AI降噪
- MITRE中文描述映射
- Agent日志级别优化
- 前端交互体验优化

---

## 2. 功能修复

### 2.1 阻断策略实时更新

#### 问题描述
阻断策略页面的启用/自动阻断/自动处置开关点击后，需要刷新页面才能看到状态变化。

#### 解决方案

**后端**: 在`UpdateBlockPolicy` API中添加WebSocket广播

```go
// backend/internal/api/handler/detection_handler.go
func (h *DetectionHandler) UpdateBlockPolicy(c *gin.Context) {
    // ... 更新逻辑
    
    policy, err := h.blockPolicyRepo.FindByMitreID(mitreID)
    if err == nil && policy != nil && h.wsService != nil {
        h.wsService.BroadcastPolicyUpdate(policy)
    }
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}
```

**后端**: WebSocket服务新增`BroadcastPolicyUpdate`方法

```go
// backend/internal/service/websocket_service.go
func (s *WebSocketService) BroadcastPolicyUpdate(policy *model.BlockPolicy) {
    s.Broadcast(WSMessage{
        Type: "policy_update",
        Data: policy,
    })
}
```

**前端**: 添加WebSocket监听和本地状态更新

```typescript
// frontend/src/views/detection/Policies.vue
function connectWebSocket() {
    const wsUrl = `${wsProtocol}//${wsHost}/api/v1/detection/runtime/ws`
    ws = new WebSocket(wsUrl)
    
    ws.onmessage = (event) => {
        const message = JSON.parse(event.data)
        if (message.type === 'policy_update' && message.data) {
            const updatedPolicy = message.data
            const index = blockPolicies.value.findIndex(p => p.mitre_id === updatedPolicy.mitre_id)
            if (index !== -1) {
                blockPolicies.value[index] = { ...blockPolicies.value[index], ...updatedPolicy }
            }
        }
    }
}

// 按钮点击后立即更新本地状态
async function handleToggleEnabled(mitreId: string, enabled: boolean) {
    await api.updateBlockPolicy(mitreId, { enabled })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
        blockPolicies.value[index].enabled = enabled
    }
    ElMessage.success('策略启用状态已更新')
}
```

#### WebSocket消息格式

```json
{
    "type": "policy_update",
    "data": {
        "id": "95ca8b84-a154-4eb6-b00d-007f8c6c252a",
        "mitre_id": "t1003.001",
        "mitre_name": "Lsass Dump",
        "enabled": true,
        "auto_block": false,
        "auto_dispose": false,
        "action": "kill_process",
        "updated_at": "2026-03-26T10:46:59Z"
    }
}
```

---

### 2.2 自动处置功能实现

#### 问题描述
开启自动处置后，告警状态不会自动变更为"已处置"。

#### 解决方案

**后端**: 在`createAlertFromEvent`中添加自动处置检查

```go
// backend/internal/grpc_server/server.go
func (s *GRPCServer) createAlertFromEvent(hostIDStr string, event *pb.RuntimeEvent) {
    // ... 创建告警
    
    // 添加MITRE中文描述
    mitreName, mitreDesc := model.GetMITREChineseDescription(event.MitreId)
    if mitreName != "" {
        alert.MitreName = mitreName
        alert.Description = mitreDesc
    }
    
    if err := s.alertRepo.Create(alert); err != nil {
        return
    }
    
    // 检查自动处置
    s.checkAutoActions(alert)
    
    if s.wsBroadcaster != nil {
        s.wsBroadcaster.BroadcastAlert(alert)
    }
}

func (s *GRPCServer) checkAutoActions(alert *model.Alert) {
    if s.blockPolicyRepo == nil {
        return
    }
    
    policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
    if err != nil || !policy.Enabled {
        return
    }
    
    if policy.AutoDispose {
        alert.AutoDispose = true
        alert.Status = "resolved"
        s.alertRepo.Update(alert)
        s.broadcastPolicyUpdate(policy)
    }
}
```

#### 自动处置流程

```
Agent上报事件
    ↓
Backend创建Alert
    ↓
查询对应MITRE的阻断策略
    ↓
策略已启用 && auto_dispose=true
    ↓
Alert.status = "resolved"
Alert.auto_dispose = true
    ↓
广播Alert更新
```

#### 验证结果

```json
{
    "alert_id": "ALT-91e9cd8e",
    "mitre_id": "t1113",
    "status": "resolved",
    "auto_dispose": true
}
```

---

### 2.3 事件持久化支持AI降噪

#### 问题描述
AI降噪显示0事件，因为`ReportEvent`只发送到Kafka，没有持久化到数据库。

#### 解决方案

**后端**: 在`ReportEvent`中添加事件持久化

```go
// backend/internal/grpc_server/server.go
func (s *GRPCServer) ReportEvent(ctx context.Context, req *pb.ReportEventRequest) (*pb.ReportEventResponse, error) {
    hostID, _ := uuid.Parse(req.HostId)
    
    for _, event := range req.Events {
        // 发送到Kafka
        if s.kafkaProducer != nil {
            s.kafkaProducer.SendRawEvent(ctx, req.HostId, event)
        }
        
        // 持久化到runtime_events表
        if s.runtimeEventRepo != nil {
            runtimeEvent := &model.RuntimeEvent{
                EventID:       event.EventId,
                HostID:        hostID,
                EventType:     event.EventType,
                EventData:     eventDataJSON,
                MatchedRuleID: event.MatchedRuleId,
                MitreID:       event.MitreId,
                Severity:      event.Severity,
                PID:           int(event.Pid),
                CommandLine:   event.CommandLine,
                Timestamp:     event.Timestamp,
                Aggregated:    false,
            }
            s.runtimeEventRepo.Create(runtimeEvent)
        }
        
        // 创建告警
        if s.alertRepo != nil && event.MatchedRuleId != "" {
            s.createAlertFromEvent(req.HostId, event)
        }
    }
    
    return &pb.ReportEventResponse{Success: true, ReceivedCount: int32(len(req.Events))}, nil
}
```

**修复**: `runtime_events`表字段映射

```go
// backend/internal/model/runtime_event.go
type RuntimeEvent struct {
    // ...
    PID int `gorm:"column:pid" json:"pid"`  // 添加column标签
    // ...
}
```

---

### 2.4 MITRE中文描述映射

#### 问题描述
告警的MITRE名称和描述显示为英文，不符合中国用户习惯。

#### 解决方案

**新增文件**: `backend/internal/model/mitre_mapping.go`

```go
package model

import "strings"

var mitreChineseMapping = map[string]struct {
    Name        string
    Description string
}{
    "T1003.001": {
        Name:        "LSASS内存转储",
        Description: "攻击者从LSASS进程内存中提取凭据，可能使用procdump、mimikatz等工具",
    },
    "T1059.004": {
        Name:        "Unix Shell反向Shell",
        Description: "攻击者使用bash/sh建立反向Shell连接，实现远程控制",
    },
    "T1053.003": {
        Name:        "Cron任务持久化",
        Description: "攻击者通过cron计划任务建立持久化",
    },
    "T1113": {
        Name:        "屏幕截图",
        Description: "攻击者截取屏幕内容以窃取敏感信息",
    },
    "T1082": {
        Name:        "系统信息收集",
        Description: "攻击者收集操作系统、版本、配置等系统信息",
    },
    // ... 共32个MITRE技术
}

func GetMITREChineseDescription(mitreID string) (name string, description string) {
    upperID := strings.ToUpper(mitreID)
    if mapping, ok := mitreChineseMapping[upperID]; ok {
        return mapping.Name, mapping.Description
    }
    return "", ""
}
```

**覆盖的MITRE技术** (32个):
- T1003.001, T1003.008 - 凭证窃取
- T1005 - 本地数据收集
- T1021.002, T1021.004 - 横向移动
- T1041, T1046, T1048 - 数据渗出
- T1053.003 - 计划任务
- T1059.001, T1059.003, T1059.004 - 命令执行
- T1068 - 权限提升
- T1070.002, T1070.004 - 日志清除
- T1082 - 系统信息收集
- T1110 - 暴力破解
- T1113 - 屏幕截图
- T1190 - 应用漏洞利用
- T1222.002 - 文件权限修改
- T1486, T1489, T1490 - 破坏性攻击
- T1543.002, T1547.001 - 持久化
- T1548.001, T1548.003 - 权限滥用
- T1572, T1573 - 隐蔽通道
- T1587, T1588 - 能力开发/获取
- T1592, T1595 - 信息收集/扫描

#### 验证结果

```json
{
    "mitre_id": "t1113",
    "mitre_name": "屏幕截图",
    "description": "攻击者截取屏幕内容以窃取敏感信息"
}
```

---

### 2.5 Agent日志级别优化

#### 问题描述
Agent的cmdline日志过多，使用`logger.Info`级别，影响日志查看。

#### 解决方案

```go
// agent/internal/ebpf/loader.go
// 将logger.Info改为logger.Debug
if emptyCmd {
    procPath := fmt.Sprintf("/proc/%d/cmdline", e.Pid)
    if procCmdline, err := os.ReadFile(procPath); err == nil {
        cmdLine = string(bytes.ReplaceAll(procCmdline, []byte{0}, []byte(" ")))
        cmdLine = strings.TrimSpace(cmdLine)
        logger.Debug("Read cmdline from /proc",  // 改为Debug
            zap.Int("pid", int(e.Pid)),
            zap.String("comm", comm),
            zap.String("cmdline", cmdLine))
    } else {
        logger.Debug("Failed to read /proc cmdline",  // 改为Debug
            zap.Int("pid", int(e.Pid)),
            zap.String("comm", comm),
            zap.Error(err))
    }
}
```

---

### 2.6 前端交互优化

#### 分页默认值

```typescript
// 修改前
const pageSize = ref(20)

// 修改后
const pageSize = ref(10)  // 默认10条每页
```

#### 按钮点击后立即更新状态

```typescript
// 每个handle函数添加本地状态更新
async function handleToggleAutoBlock(mitreId: string, autoBlock: boolean) {
    try {
        await api.updateBlockPolicy(mitreId, { auto_block: autoBlock })
        const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
        if (index !== -1) {
            blockPolicies.value[index].auto_block = autoBlock
        }
        ElMessage.success('自动阻断状态已更新')
    } catch (e: any) {
        ElMessage.error(e.message || '更新失败')
    }
}
```

---

## 3. 文件变更清单

### 后端 (Backend)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/model/mitre_mapping.go` | 新增 | MITRE中文映射 |
| `internal/model/runtime_event.go` | 修改 | 添加PID字段column标签 |
| `internal/service/websocket_service.go` | 修改 | 添加BroadcastPolicyUpdate方法 |
| `internal/api/handler/detection_handler.go` | 修改 | 添加wsService依赖和广播逻辑 |
| `internal/grpc_server/server.go` | 修改 | 添加事件持久化和自动处置逻辑 |
| `cmd/server/main.go` | 修改 | 更新DetectionHandler初始化参数 |

### 前端 (Frontend)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `src/views/detection/Policies.vue` | 修改 | WebSocket监听、本地状态更新、分页默认值 |

### Agent

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/ebpf/loader.go` | 修改 | cmdline日志改为Debug级别 |

---

## 4. 数据库变更

无需新增表或字段，使用现有`runtime_events`表。

---

## 5. API变更

无新增API，现有API行为变更：

| API | 变更 |
|-----|------|
| `PUT /api/v1/detection/block-policies/:mitre_id` | 更新后通过WebSocket广播策略变更 |
| `POST /api/v1/agent/events` (gRPC) | 新增事件持久化逻辑 |

---

## 6. WebSocket消息类型

| 类型 | 说明 | 触发条件 |
|------|------|----------|
| `alert` | 告警更新 | 新告警创建/告警状态变更 |
| `policy_update` | 策略更新 | 策略配置变更 |

---

## 7. 测试验证

### 7.1 API测试

```bash
# 运行API测试脚本
./tests/api/run-tests.sh

# 结果: 正向测试 12/12 通过
```

### 7.2 实时更新测试

```bash
# WebSocket连接测试
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  -H "Sec-WebSocket-Version: 13" \
  http://localhost:8080/api/v1/detection/runtime/ws

# 预期: 返回101 Switching Protocols
# 收到消息: {"type":"policy_update","data":{...}}
```

### 7.3 自动处置测试

```bash
# 开启t1113的自动处置
curl -X PUT http://localhost:8080/api/v1/detection/block-policies/t1113 \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "auto_dispose": true}'

# 触发相关事件后检查告警
curl http://localhost:8080/api/v1/detection/alerts | jq '.data.data[0]'

# 预期: status="resolved", auto_dispose=true
```

### 7.4 中文描述测试

```bash
curl http://localhost:8080/api/v1/detection/alerts | jq '.data.data[] | {mitre_id, mitre_name, description}'

# 预期输出:
# {
#   "mitre_id": "t1113",
#   "mitre_name": "屏幕截图",
#   "description": "攻击者截取屏幕内容以窃取敏感信息"
# }
```

---

## 8. 部署说明

### 8.1 构建步骤

```bash
# 后端
cd backend && make build

# 前端
cd frontend && npm run build

# Agent
cd agent && make all

# Docker镜像
docker build -t aegis-system/backend:latest -f backend/Dockerfile .
docker build -t aegis-system/frontend:latest -f frontend/Dockerfile frontend/
```

### 8.2 部署命令

```bash
docker compose up -d
```

### 8.3 Agent更新

```bash
# 上传到MinIO
mc cp agent/dist/aegis-agent.tar.gz myminio/agent-artifacts/

# 卸载旧版本
sudo /opt/aegis-agent/uninstall.sh

# 安装新版本
curl -sSL http://SERVER_IP:8080/api/v1/agent/install.sh | sudo bash
```

---

## 9. 已知问题

1. **WebSocket重连**: 前端WebSocket断开后会自动重连，间隔3秒。如果网络不稳定可能看到短暂的状态同步延迟。

2. **MITRE大小写**: 所有MITRE ID统一使用小写存储，历史数据中的大写MITRE ID需要在查询时转换。

3. **AI降噪依赖runtime_events**: 如果Agent版本较旧没有上报事件到runtime_events表，AI降噪可能显示0事件。

---

## 10. 后续计划

1. **V5.3**: 
   - 支持更多MITRE技术的中文描述
   - 前端国际化支持（中英文切换）

2. **V6.0**:
   - 多主机关联分析
   - 横向移动检测
   - 威胁狩猎工作流