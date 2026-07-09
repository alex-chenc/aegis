---
name: aegis-build-test
description: 构建、测试和验证 Aegis 的 api-server、server、dc、agent、frontend 或 Docker Compose 数据流。用于代码变更后的定向测试、组件构建、eBPF 构建、服务健康检查、API 冒烟、Agent 打包上传和跨服务集成验证。
---

# Aegis 构建与验证

以最窄且足以证明变更正确的验证完成任务。成功意味着受影响行为通过测试、受影响组件可构建；只有跨服务行为发生变化时才启动服务或验证完整数据流。

## 选择验证范围

| 变更范围 | 首选验证 | 构建 |
| --- | --- | --- |
| `api-server/` | 在该模块运行相关包测试；需要扩大覆盖时运行 `go test ./...` | `make build` |
| `server/` | 在该模块运行相关包测试；需要扩大覆盖时运行 `go test ./...` | `make build` |
| `dc/` | 在该模块运行相关包测试；需要扩大覆盖时运行 `go test ./...` | `make build` |
| `agent/` Go 代码 | 运行相关包测试 | `make build` |
| `agent/` eBPF 或打包链路 | 运行相关包测试 | `make bpf` 后运行 `make build`；需要制品时运行 `make package` |
| `frontend/` | `npm run test -- <file>`，按需补充 `npm run type-check` 和 `npm run lint` | `npm run build` |
| Compose、协议或跨服务链路 | 先验证各受影响组件，再做 Compose 冒烟或数据流验证 | 仅重建受影响服务 |

先读取对应 `Makefile`、`package.json` 和现有测试，避免假设不存在的命令。生成 protobuf 的变更还要确认 `proto/*.proto` 与生成代码一致。

## 工作流

1. 从变更文件、调用路径和失败现象确定受影响组件与行为。
2. 先运行能覆盖该行为的定向测试；失败时保留完整错误证据并定位原因。
3. 运行受影响组件的构建。Agent 的运行时采集变更必须先构建 eBPF。
4. 仅在组件边界、配置、协议、数据库或事件流改变时增加集成验证。
5. 记录已运行命令、结果和未运行检查的原因。

不要用全栈构建代替定向测试，也不要为了减少命令次数跳过必需的生成、构建或证据检查。

## 服务与数据流验证

需要运行服务时，优先重建最小集合：

```bash
docker compose up -d --build <service...>
docker compose ps
docker compose logs <service>
```

常用检查：

```bash
curl -fsS http://localhost:8082/health
nc -z localhost 19090
nc -z localhost 19092
docker compose exec postgres pg_isready -U aegis_user -d aegis_db
```

按变更涉及的边界检查数据流，不必机械执行全部步骤：

```text
Agent -> Server:19090 -> Kafka:aegis.security.events -> DC -> PostgreSQL/WebSocket
api-server -> Server:19094 -> Agent stream
```

可从 Server 日志、Kafka 主题、DC 日志和数据库结果中选择能够证明目标行为的检查点。空结果不等于“没有事件”；先确认时间范围、消费者位置、过滤条件和服务就绪状态。

## Agent 制品与 MinIO

需要上传 Agent 时，在 `agent/` 中运行 `make all` 和 `make upload`。从环境或用户批准的密钥来源读取 `MINIO_ENDPOINT`、`MINIO_ACCESS_KEY`、`MINIO_SECRET_KEY`；不得把凭证写入 skill、源码、命令输出或文档。上传后验证 `agent-artifacts/aegis-agent.tar.gz` 可读取且非空。

卸载目标机 Agent、覆盖远程制品、停止服务或执行 `docker compose down -v` 会改变外部状态或删除数据，执行前必须获得用户确认。

## 完成条件

- 定向测试覆盖了变更行为并通过。
- 受影响组件构建通过。
- 必要的协议、数据库、服务或数据流检查通过。
- 未执行的高成本或环境依赖检查已说明原因和下一步。
- 没有泄露凭证，也没有改动无关服务或数据。
