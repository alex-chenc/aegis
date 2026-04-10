---
name: aegis-build-test
description: Aegis 系统构建测试技能 - agent/server/dc/api-server 构建部署和 gRPC 数据流测试
version: 1.0.0
source: manual-creation
---

# Aegis 系统构建测试技能

本技能用于 AI 学习 Aegis 系统架构、构建部署流程和 API 测试。

## 服务端口

| 服务 | HTTP 端口 | gRPC 端口 |
|------|-----------|-----------|
| api-server | 8082 | 19093 |
| server | - | 19090, 19094 |
| dc | - | 19092 |

## 一、AI 必读文档

### 架构设计文档 (v5.5)
```bash
cat aegis_system_design_v5.5/README.md
cat aegis_system_design_v5.5/architecture_design_v5.5.md
cat aegis_system_design_v5.5/communication_protocol_design_v5.5.md
cat aegis_system_design_v5.5/backend_detailed_design_v5.5_complete.md
cat aegis_system_design_v5.5/agent_detailed_design_v5.5_complete.md
cat aegis_system_design_v5.5/frontend_detailed_design_v5.5_complete.md
cat aegis_system_design_v5.5/database_structure_design_v5.5_complete.md
```

## 二、构建命令

### Agent
```bash
cd agent && make all && make upload
```

### API Server
```bash
cd api-server && make build
```

### Server
```bash
cd server && make build
```

### DC
```bash
cd dc && make build
```

### Docker 构建并启动
```bash
docker compose up -d --build
```

## 三、Agent 卸载重装

```bash
# 1. 构建并上传
cd agent && make all && make upload

# 2. 卸载
sudo /opt/aegis-agent/uninstall.sh

# 3. 重新安装
curl -sSL http://<SERVER_IP>:8082/api/v1/agent/install.sh | sudo bash

# 4. 验证
sudo systemctl status aegis-agent
```

## 四、API 测试 (curl)

### 健康检查
```bash
curl http://localhost:8082/health
```

### 示例：创建漏洞扫描任务
```bash
curl -X POST http://localhost:8082/api/v1/vulnerability/scan \
  -H "Content-Type: application/json" \
  -d '{"host_id":1,"scan_type":"full"}'
```

## 五、数据流测试

```
Agent → Server (19090) → Kafka → DC (19092) → PostgreSQL
```

**测试检查点:**
1. Agent 是否能连接 Server (端口 19090)
2. Server 是否发送消息到 Kafka topic `aegis.security.events`
3. DC 是否消费 Kafka 消息
4. 数据是否正确存储到 PostgreSQL
```