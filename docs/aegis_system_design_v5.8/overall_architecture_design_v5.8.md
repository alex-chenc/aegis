# V5.8 总体架构设计: 动态 eBPF DetectionPackage 与 Builder 组件

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. 文档目标

本文补齐 V5.8 动态 eBPF 检测能力的整体系统视图，重点说明新增 `builder` 组件的定位、通信对象、职责边界和 gRPC 接口关系。

V5.8 的核心变化是新增一条“检测包生产链路”：

```text
AI/人工草稿 -> API Server 编排 -> builder 构建与签名 -> MinIO 存储
  -> API Server 启用 -> server 转发 -> agent 验签加载 -> agent 本地检测
```

`builder` 是控制面内部组件，不属于 agent 数据面，也不直接参与运行时告警处理。

---

## 2. 组件总览

| 组件 | V5.8 职责 | 是否新增 | 是否持有私钥 |
|:---|:---|:---:|:---:|
| `frontend` | DetectionPackage 草稿编辑、构建、签名、启用、状态查看页面 | 否 | 否 |
| `api-server` | DetectionPackage 元数据、草稿、构建任务、发布状态、全局配置、下发编排 | 否 | 否 |
| `builder` | eBPF 源码编译、perf/ringbuf artifact 生成、package 打包、整包签名 | 是 | 是 |
| `server` | agent hub，负责把安装、卸载、配置同步命令转发给 agent | 否 | 否 |
| `agent` | 下载包、验签、校验 hook allowlist、加载 eBPF 插件、Sigma + Correlation 检测 | 否 | 否，只有公钥 |
| `dc` | 消费运行时告警事件，聚合、入库、WebSocket 推送 | 否 | 否 |
| `postgres` | package、build、release、host status、allowlist、审计记录 | 否 | 否 |
| `minio` | DetectionPackage 构建产物、签名包、签名文件存储 | 否 | 否 |
| `kafka` | agent 告警事件进入 dc 的事件总线 | 否 | 否 |

---

## 3. Builder 组件定位

`builder` 的定位是 **内部构建与签名服务**：

- 只服务于动态 eBPF DetectionPackage 的生产发布链路。
- 使用与 agent release 相同的 builder image 和 eBPF 公共头文件。
- 负责编译两种 artifact：`*.perf.bpf.o` 与 `*.ringbuf.bpf.o`。
- 负责把 staging 目录打成 `package.tar.gz`。
- 使用编译进 builder 二进制的 Ed25519 私钥，对整个 `package.tar.gz` 生成 `package.tar.gz.sig`。
- 不把 eBPF 源码放入最终 DetectionPackage。

`builder` 不做：

- 不直接接收 frontend 请求。
- 不直接和 `server`、`agent`、`dc` 通信。
- 不向 agent 下发包。
- 不判断某个 agent 是否应该安装包。
- 不执行 runtime 检测。
- 不保存业务最终状态，业务状态以 `api-server` 数据库为准。

---

## 4. 通信关系

### 4.1 总通信图

```text
frontend
  | HTTP
  v
api-server
  | gRPC: BuilderService
  v
builder
  | S3 API
  v
minio

api-server
  | gRPC: APIServerToServer
  v
server
  | bidirectional gRPC command stream
  v
agent

agent
  | gRPC ReportEvent / ReportDetectionPackageStatus
  v
server
  | Kafka
  v
dc
  | PostgreSQL / WebSocket
  v
frontend
```

### 4.2 通信矩阵

| 发起方 | 接收方 | 协议 | 用途 | 说明 |
|:---|:---|:---|:---|:---|
| `frontend` | `api-server` | HTTP REST | 草稿、构建、签名、启用、状态查询 | 只面向管理页面 |
| `api-server` | `builder` | gRPC | 启动构建、查询构建、触发签名、查询 builder 信息 | builder 唯一业务调用方是 API Server |
| `builder` | `minio` | S3 API | 上传 unsigned package、signed package、signature、构建日志 | 大文件不走 gRPC |
| `api-server` | `minio` | S3 API | 生成下载 URL、读取 artifact 元数据、清理对象 | API Server 负责发布对象管理 |
| `api-server` | `server` | gRPC | 下发安装、卸载、回滚、配置同步命令 | server 只转发和记录 agent 在线状态 |
| `server` | `agent` | 双向 gRPC | agent 注册、命令流、状态上报 | 复用现有 agent command stream |
| `agent` | `server` | gRPC | 告警、DetectionPackage 状态 | agent 不和 builder 通信 |
| `server` | `kafka` | Kafka Producer | 转发 runtime security events | 进入 dc 消费链路 |
| `dc` | `postgres` | SQL | 告警、聚合结果入库 | 复用当前告警链路 |

---

## 5. 端到端流程

### 5.1 草稿生成与编辑

```text
frontend -> api-server
```

1. 管理员输入 CVE、漏洞描述、检测目标。
2. API Server 调用 AI 生成草稿。
3. 草稿包含 HookPlan、eBPF source、Sigma atomic rules、Correlation DetectionSpec。
4. 管理员可以在页面修改草稿。
5. 草稿保存到 `detection_package_drafts`。

### 5.2 构建

```text
frontend -> api-server -> builder -> minio
```

1. 管理员点击“提交构建”。
2. API Server 创建 build 记录，调用 `builder.StartPackageBuild`。
3. Builder 在统一 builder image 内编译 eBPF 源码。
4. Builder 同时生成 perf 与 ringbuf 两份 `.bpf.o`。
5. Builder 组装 staging 目录并打出 unsigned `package.tar.gz`。
6. Builder 上传 unsigned package 和 build log 到 MinIO staging 路径。
7. API Server 通过 `builder.GetPackageBuildStatus` 获取结果并写入数据库。

### 5.3 签名发布

```text
frontend -> api-server -> builder -> minio
```

1. 构建成功后页面展示 hook、schema、规则、构建日志和风险信息。
2. 管理员点击“签名发布”。
3. API Server 校验 build 状态为 `success`，调用 `builder.SignPackage`。
4. Builder 从 MinIO staging 路径读取 unsigned `package.tar.gz`。
5. Builder 使用内置 Ed25519 私钥对整个 tar.gz 签名。
6. Builder 上传 signed package 和 `.sig` 到正式发布路径。
7. API Server 写入 release 元数据和审计记录。

### 5.4 启用与下发

```text
frontend -> api-server -> server -> agent
```

1. 管理员点击“启用”。
2. API Server 确认该版本已签名。
3. 同一 `package_id` 只保留一个 active/enabled 版本。
4. API Server 调用 server gRPC 下发 install command。
5. Server 对在线 agent 立即转发，对离线 agent 在上线后补发。

### 5.5 Agent 运行时

```text
agent -> server -> kafka -> dc
```

1. Agent 下载 `package.tar.gz` 和 `.sig`。
2. Agent 使用内置公钥验签整个包。
3. Agent 校验当前全局 hook allowlist。
4. Agent 优先加载 ringbuf artifact，失败后可尝试 perf artifact。
5. eBPF 插件只采集事件和轻量过滤。
6. Agent 用户态执行 Sigma atomic rules。
7. Sigma 命中形成 AtomicFinding。
8. AtomicFinding 进入本地 Correlation Engine。
9. Correlation 命中后上报告警和 evidence chain。

---

## 6. Builder gRPC 总览

Builder 暴露内部 gRPC 服务 `BuilderService`，只允许 API Server 调用。

```proto
service BuilderService {
  rpc GetBuilderInfo(GetBuilderInfoRequest) returns (GetBuilderInfoResponse);
  rpc StartPackageBuild(StartPackageBuildRequest) returns (StartPackageBuildResponse);
  rpc GetPackageBuildStatus(GetPackageBuildStatusRequest) returns (GetPackageBuildStatusResponse);
  rpc SignPackage(SignPackageRequest) returns (SignPackageResponse);
}
```

接口分工：

| RPC | 作用 |
|:---|:---|
| `GetBuilderInfo` | 返回 builder 版本、image digest、clang/libbpf/bpftool 版本、公钥指纹 |
| `StartPackageBuild` | 接收草稿内容或草稿对象 key，启动构建任务 |
| `GetPackageBuildStatus` | 查询构建状态、日志摘要、artifact、schema、hook summary |
| `SignPackage` | 对成功构建的 unsigned package 进行整包签名并发布 |

详细 proto 草案见 `api_grpc_design_v5.8.md`。

---

## 7. 安全边界

### 7.1 私钥边界

- Ed25519 私钥只编译进 `builder`。
- API Server、server、agent、dc、frontend、MinIO、PostgreSQL 都不持有私钥。
- Builder 日志、manifest、HTTP 响应、gRPC 响应、MinIO metadata 禁止输出私钥或可恢复私钥的材料。
- Builder 镜像和二进制按敏感制品管理。

### 7.2 调用边界

- Builder gRPC 不暴露公网。
- Builder gRPC 只接受 API Server 调用。
- 推荐使用服务间 mTLS 或内网 allowlist。
- `SignPackage` 只允许签名 builder 自己产出的、状态为 `success` 的 build。
- 签名动作必须来自人工点击，不允许 AI 或定时任务自动签名。

### 7.3 Agent 边界

- Agent 不编译源码。
- Agent 不加载未签名包。
- Agent 不依赖 builder。
- Agent 只使用页面下发的全局 hook allowlist。
- 插件加载失败只禁用该插件或 package，不退出 agent 主进程。

---

## 8. 状态归属

| 状态 | 权威组件 | 存储 |
|:---|:---|:---|
| draft 内容 | API Server | PostgreSQL |
| build 状态 | API Server | PostgreSQL |
| build 执行过程 | Builder | 本地临时工作目录 + MinIO 日志 |
| unsigned package | Builder 生成 | MinIO staging bucket |
| signed package | Builder 生成 | MinIO release bucket |
| package enabled 状态 | API Server | PostgreSQL |
| agent 安装状态 | Agent 上报，API Server 汇总 | PostgreSQL |
| runtime alert | Agent 产生，dc 处理 | Kafka + PostgreSQL |

Builder 可以保存短期本地工作目录，但不能作为最终状态源。API Server 需要把 build/release 状态落库，便于重启恢复和审计。

---

## 9. 与现有组件的关系

### 9.1 与现有 Sigma 的关系

动态 DetectionPackage 内的 Sigma atomic rules 进入 agent 侧 Sigma matcher。Correlation Engine 只消费 Sigma 命中的 AtomicFinding，不直接消费全部 eBPF 原始事件。

### 9.2 与现有 agent eBPF loader 的关系

现有 agent 固定 eBPF 程序继续存在。V5.8 新增动态 plugin manager，负责加载 DetectionPackage 内的 `.bpf.o`，并和固定采集能力并行运行。

### 9.3 与现有 release packaging 的关系

`builder` 镜像需要与 agent release 使用同一套构建环境。动态插件构建和 agent release 不共用运行时流程，但共用 builder image、BPF 公共头文件、Makefile 规则和 clang/libbpf 工具链。

---

## 10. 第一版边界

V5.8 第一版不做：

- Builder 直接对接 agent。
- 独立 signer、KMS、HSM。
- 多 builder 调度。
- 主机组灰度发布。
- agent 本地源码编译。
- 未签名 package 加载。
- 原始插件事件默认上报服务端。
- 跨主机关联检测。
