---
name: aegis-build-test
description: 构建、测试和验证 Aegis 的 api-server、server、dc、agent、builder、frontend 或 Docker Compose 数据流。用于代码变更后的定向测试、组件构建、eBPF 镜像内构建、服务健康检查、API 冒烟、Agent 打包上传和跨服务集成验证。
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
| `agent/` eBPF 或打包链路 | 运行相关包测试 | 在 `aegis-agent-builder-ubi8:5.8.0` 内 `docker run` 编译打包（见下，不生成镜像） |
| `builder/` | 运行相关包测试 | 在 `aegis-agent-builder-ubi8:5.8.0` 内构建（见下） |
| `frontend/` | `npm run test -- <file>`，按需补充 `npm run type-check` 和 `npm run lint` | `npm run build` |
| Compose、协议或跨服务链路 | 先验证各受影响组件，再做 Compose 冒烟或数据流验证 | 仅重建受影响服务 |

先读取对应 `Makefile`、`package.json` 和现有测试，避免假设不存在的命令。生成 protobuf 的变更还要确认 `proto/*.proto` 与生成代码一致。

## 工作流

1. 从变更文件、调用路径和失败现象确定受影响组件与行为。
2. 先运行能覆盖该行为的定向测试；失败时保留完整错误证据并定位原因。
3. 运行受影响组件的构建。Agent 的 eBPF/打包与 `builder/` 服务都在共享 builder 镜像内构建，先确认 `aegis-agent-builder-ubi8:5.8.0` 存在（见下）。
4. 仅在组件边界、配置、协议、数据库或事件流改变时增加集成验证。
5. 记录已运行命令、结果和未运行检查的原因。

不要用全栈构建代替定向测试，也不要为了减少命令次数跳过必需的生成、构建或证据检查。

## 共享 eBPF Builder 镜像

Agent 的 eBPF 程序和 `builder/` 服务都依赖 clang、llvm、kernel-headers 与 eBPF 头文件，宿主机通常不具备。两者都必须在共享 builder 镜像 `aegis-agent-builder-ubi8:5.8.0` 内构建；不要直接在宿主机运行 `make bpf`，除非已确认本地具备完整 eBPF 工具链与 `/opt/aegis/ebpf/include` 头文件。

builder 镜像本身由 `docker/ebpf-builder-base/Dockerfile` 构建（基于 Red Hat UBI 8.10，内含 Go、clang/llvm、kernel-headers 与 eBPF include），是 Agent 与 builder 的共同前置依赖。构建前先确认镜像存在：

```bash
docker image inspect aegis-agent-builder-ubi8:5.8.0 >/dev/null 2>&1 \
  || docker build -f docker/ebpf-builder-base/Dockerfile -t aegis-agent-builder-ubi8:5.8.0 .
# 等价：cd agent && make docker-base-image
```

### Agent 制品（在 builder 镜像内编译，不生成 Docker 镜像）

Agent 的 eBPF 程序必须在 `aegis-agent-builder-ubi8:5.8.0` 内编译，但**打包发布只产出 `aegis-agent.tar.gz`，不要构建 Docker 镜像**。用 `docker run` 把源码挂进 builder 镜像执行 `make all`（= `bpf` + `build` + `package`），产物经 bind mount 直接落到宿主机 `agent/dist/`，不留下任何镜像：

```bash
cd agent
docker run --rm \
  -v "$(pwd):/src/agent" \
  -w /src/agent \
  aegis-agent-builder-ubi8:5.8.0 \
  make BPF_TARGET_ARCH=x86 BPF_TRANSPORT=all all
# 产物：dist/aegis-agent-linux-amd64、dist/aegis-agent-linux-arm64、dist/aegis-agent.tar.gz、dist/bpf/*.bpf.o
```

`make all` 中的 `build` 会交叉编译 amd64 与 arm64 两个 Go 二进制；`BPF_TARGET_ARCH` 只决定 eBPF 编译时的架构宏（`x86`/`arm`），`package` 默认把 amd64 二进制与 eBPF 对象打成 `dist/aegis-agent.tar.gz`。

不要用 `make docker-build` 或 `docker build -f agent/Dockerfile` 产出发布制品——它们会构建并留下 Docker 镜像 `aegis-agent-artifacts:local`，仅在确实需要容器镜像（而非上传 MinIO 的 tarball）时才用。宿主机不具备 eBPF 工具链时也不要直接在宿主机跑 `make bpf`。

### Builder 服务（镜像内构建）

`builder/Dockerfile` 同样以 builder 镜像作为构建与运行基础，编译 `builder/` Go 服务并暴露 19096：

```bash
docker build -f builder/Dockerfile \
  --build-arg EBPF_BASE_IMAGE=aegis-agent-builder-ubi8:5.8.0 \
  -t aegis-system/builder:latest .
# 或经 Compose（builder 服务无 profile，可直接构建）：
docker compose up -d --build builder
```

Compose 中 `ebpf-builder-base` 服务带 `profiles: [build]`，需要单独重建基础镜像时用 `docker compose --profile build up -d --build ebpf-builder-base`。

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

需要发布 Agent 时，先用上一节 `docker run` 方法在 `aegis-agent-builder-ubi8:5.8.0` 内编译打包，确认 `agent/dist/aegis-agent.tar.gz` 为最新且非空，再上传到 MinIO 的 `agent-artifacts` 桶。

从环境或用户批准的密钥来源读取 `MINIO_ENDPOINT`、`MINIO_ACCESS_KEY`、`MINIO_SECRET_KEY`（例如运行中的 MinIO 容器环境 `docker compose exec -T minio printenv MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD`，或 `.env` / `.env.example`），不要硬编码或写入文件。注意 `make upload` 的 recipe 会回显 `mc alias set ... <密钥>` 命令行，导致凭证进入输出，因此用静默模式执行：

```bash
cd agent
export MINIO_ENDPOINT="http://localhost:9000"
export MINIO_ACCESS_KEY="$(docker compose exec -T minio printenv MINIO_ROOT_USER)"
export MINIO_SECRET_KEY="$(docker compose exec -T minio printenv MINIO_ROOT_PASSWORD)"
make -s upload     # -s 抑制 recipe 回显，避免凭证出现在命令输出
```

上传后验证对象可读且非空（`local` 别名已由 `make upload` 建立）：

```bash
mc ls local/agent-artifacts/aegis-agent.tar.gz
```

覆盖远程制品会改变外部状态（目标机会拉取到新版本），卸载目标机 Agent、停止服务或执行 `docker compose down -v` 同样会改变外部状态或删除数据，执行前必须获得用户确认。

## 完成条件

- 定向测试覆盖了变更行为并通过。
- 受影响组件构建通过。
- 必要的协议、数据库、服务或数据流检查通过。
- 未执行的高成本或环境依赖检查已说明原因和下一步。
- 没有泄露凭证，也没有改动无关服务或数据。
