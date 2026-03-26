# Aegis V5.2 更新总结

**更新日期**: 2026-03-26

---

## 已修复问题

| # | 问题 | 状态 |
|---|------|------|
| 1 | 阻断策略页面按钮不会实时更新，需要刷新页面 | ✅ 已修复 |
| 2 | 开启自动处置后告警状态不会自动变更 | ✅ 已修复 |
| 3 | AI降噪显示0事件 | ✅ 已修复 |
| 4 | MITRE无中文描述 | ✅ 已修复 |
| 5 | Agent cmdline日志过多 | ✅ 已修复 |
| 6 | 分页默认值过大（20改为10） | ✅ 已修复 |

---

## 主要变更

### 1. WebSocket实时广播

- 新增`policy_update`消息类型
- 策略变更后自动广播到所有连接的前端

### 2. 自动处置功能

- Alert创建时检查对应策略的`auto_dispose`字段
- 自动将状态设为`resolved`

### 3. 事件持久化

- `ReportEvent` gRPC方法新增持久化逻辑
- AI降噪可正确统计`runtime_events`表中的事件

### 4. MITRE中文映射

- 新增32个MITRE技术的中文名称和描述
- Alert创建时自动填充

### 5. Agent日志优化

- cmdline相关日志从`Info`改为`Debug`

---

## 文件变更

```
backend/internal/model/mitre_mapping.go        [新增]
backend/internal/model/runtime_event.go        [修改]
backend/internal/service/websocket_service.go  [修改]
backend/internal/api/handler/detection_handler.go [修改]
backend/internal/grpc_server/server.go         [修改]
backend/cmd/server/main.go                     [修改]
frontend/src/views/detection/Policies.vue      [修改]
agent/internal/ebpf/loader.go                  [修改]
```

---

## 部署命令

```bash
# 构建后端
cd backend && make build

# 构建前端
cd frontend && npm run build

# 构建Docker镜像
docker build -t aegis-system/backend:latest -f backend/Dockerfile .
docker build -t aegis-system/frontend:latest -f frontend/Dockerfile frontend/

# 部署
docker compose up -d

# 更新Agent
mc cp agent/dist/aegis-agent.tar.gz myminio/agent-artifacts/
sudo /opt/aegis-agent/uninstall.sh
curl -sSL http://SERVER_IP:8080/api/v1/agent/install.sh | sudo bash
```

---

## 验证测试

```bash
# API测试
./tests/api/run-tests.sh

# WebSocket测试
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Key: test" -H "Sec-WebSocket-Version: 13" \
  http://localhost:8080/api/v1/detection/runtime/ws

# 自动处置测试
curl -X PUT http://localhost:8080/api/v1/detection/block-policies/t1113 \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "auto_dispose": true}'
```

---

## 详细文档

- [CHANGELOG_v5.2.md](./CHANGELOG_v5.2.md) - 完整变更日志
- [DESIGN_UPDATE_V5.1.md](./DESIGN_UPDATE_V5.1.md) - 设计更新文档