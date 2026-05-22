# V5.8 Builder 与发布设计

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. 目标

V5.8 动态 eBPF 插件必须在与 agent 同源的 builder 容器中编译，保证编译器、headers、公共 BPF 头文件和 Makefile 规则一致。

明确禁止：

- agent 本机编译 eBPF 源码。
- 未经 builder 的 `.bpf.o` 进入签名发布。
- 未签名 package 下发到 agent。

签名私钥策略：

- Ed25519 私钥编译进 V5.8 新增 builder 组件。
- 不设计独立 release signer、KMS 或 HSM。
- API Server 只触发签名动作，不持有私钥。
- builder 日志、构建产物和 package manifest 中禁止输出私钥或可恢复私钥的材料。

---

## 2. 组件定位与通信关系

`builder` 是 V5.8 新增的控制面内部组件，定位为 **动态 eBPF DetectionPackage 构建与签名服务**。

组件边界：

- `builder` 只接受 `api-server` 的内部 gRPC 调用。
- `builder` 不直接接收 `frontend` 请求。
- `builder` 不直接向 `server` 或 `agent` 下发 DetectionPackage。
- `builder` 不参与运行时事件采集、Sigma 匹配、Correlation 检测或告警处理。
- `builder` 不直接写 PostgreSQL，业务状态由 `api-server` 入库。

通信关系：

| 发起方 | 接收方 | 协议 | 用途 |
|:---|:---|:---|:---|
| `api-server` | `builder` | gRPC `BuilderService` | 启动构建、查询构建、触发签名、查询 builder 信息 |
| `builder` | `minio` | S3 API | 上传 unsigned package、signed package、signature、构建日志 |
| `api-server` | `minio` | S3 API | 生成下载 URL、读取发布对象元数据、清理对象 |
| `api-server` | `server` | gRPC | 启用后下发 install/uninstall/rollback 命令 |

Builder 对外暴露的 gRPC 服务：

```proto
service BuilderService {
  rpc GetBuilderInfo(GetBuilderInfoRequest) returns (GetBuilderInfoResponse);
  rpc StartPackageBuild(StartPackageBuildRequest) returns (StartPackageBuildResponse);
  rpc GetPackageBuildStatus(GetPackageBuildStatusRequest) returns (GetPackageBuildStatusResponse);
  rpc SignPackage(SignPackageRequest) returns (SignPackageResponse);
}
```

详细 proto 草案见 `api_grpc_design_v5.8.md`。

---

## 3. Builder 镜像

推荐镜像：

```text
aegis-agent-builder-ubi8:5.8.0
```

基础镜像：

```text
registry.access.redhat.com/ubi8/ubi:8
```

要求：

- 与 agent release 使用同一个 builder image。
- 记录 image digest。
- 构建产物记录 builder image 和 digest。
- 不用真实 4.18 机器验证，运行时由 agent 尝试加载，失败仅禁用插件。

镜像包含：

```text
go
clang
llvm
make
bpftool
libbpf headers
linux uapi headers
agent/internal/ebpf/bpf/common.h
agent/internal/ebpf/bpf/event_output.h
agent/internal/ebpf/bpf/vmlinux.h
package assembly tools
signature tools
```

---

## 4. 构建输入

来自 `detection_package_drafts` 当前草稿：

```text
package metadata
HookPlan YAML
eBPF source
Sigma atomic YAML
Correlation YAML
build params
```

源码不进入最终 DetectionPackage。

---

## 5. 构建输出

每个插件必须生成：

```text
copyfail.perf.bpf.o
copyfail.ringbuf.bpf.o
```

最终 staging：

```text
staging/
├── package.yaml
├── plugin/
│   ├── plugin.yaml
│   ├── copyfail.perf.bpf.o
│   └── copyfail.ringbuf.bpf.o
├── rules/
│   └── atomic_sigma.yml
└── correlations/
    └── copyfail_correlation.yml
```

---

## 6. 编译参数

当前 agent Makefile 使用 `uname -m` 推导 target arch。V5.8 builder 必须改成显式参数：

```text
BPF_TARGET_ARCH=x86
BPF_TARGET_ARCH=arm
BPF_TRANSPORT=perf
BPF_TRANSPORT=ringbuf
```

第一版发布包可以只构建当前支持架构，但 Makefile 接口必须为多架构预留。

示例：

```bash
clang -g -O2 -c -target bpf \
  -DAEGIS_EVENT_PERF=1 \
  -D__TARGET_ARCH_x86 \
  -I/usr/include \
  -Iagent/internal/ebpf/bpf \
  -o copyfail.perf.bpf.o \
  copyfail.bpf.c

clang -g -O2 -c -target bpf \
  -DAEGIS_EVENT_RINGBUF=1 \
  -D__TARGET_ARCH_x86 \
  -I/usr/include \
  -Iagent/internal/ebpf/bpf \
  -o copyfail.ringbuf.bpf.o \
  copyfail.bpf.c
```

---

## 7. 构建校验

构建阶段必须校验：

| 校验项 | 说明 |
|:---|:---|
| YAML 解析 | HookPlan、Sigma、Correlation 必须合法 |
| Package ID | 文件和规则必须属于同一 package_id |
| Rule 引用 | Correlation 引用的 rule_id 必须存在于当前 package |
| Event schema | plugin event_schema 和 eBPF event_type 对齐 |
| Artifact 存在 | perf/ringbuf 两份必须生成 |
| ELF section | hook program section 与 plugin manifest 对齐 |
| Map 名称 | event map 名必须存在 |
| 源码扫描 | 禁止明显危险或无关 helper 使用 |

不做真实内核加载验证。

---

## 8. 人工审核

构建成功后页面展示：

- package_id/version/title
- 源码摘要
- hook 列表
- 是否命中当前 allowlist
- event_schema
- Sigma atomic rules
- Correlation DetectionSpec
- artifact 文件名、大小
- builder image digest
- clang version
- 构建日志
- 风险提示

人工点击“签名发布”后，api-server 调用 builder 的签名动作，由 builder 使用内置私钥完成整包签名。

---

## 9. 签名

签名对象：

```text
DetectionPackage tar.gz 整包
```

签名文件：

```text
package.tar.gz.sig
```

算法：

```text
Ed25519
```

私钥：

- 编译进 V5.8 新增 builder 组件。
- builder 是唯一持有签名私钥的组件。
- API Server、server、agent、MinIO 均不持有私钥。
- 不进入 agent。
- 不写入构建日志、package.yaml、plugin.yaml、MinIO 对象元数据或前端响应。

builder 组件约束：

- 签名动作必须由人工点击“签名发布”触发。
- 签名前必须确认对应 build 状态为 `success`。
- 签名输入只能是 builder 产出的 package tar.gz。
- 签名输出只能是 `package.tar.gz.sig`。
- builder 镜像和二进制需要按生产敏感组件管理，限制拉取、运行和日志访问。

公钥：

- 编译进 agent。

---

## 10. 分发

签名后上传到 MinIO：

```text
minio://detection-packages/cve-2026-31431-copyfail/1.0.0/package.tar.gz
minio://detection-packages/cve-2026-31431-copyfail/1.0.0/package.tar.gz.sig
```

api-server 下发给 server/agent 的是：

```json
{
  "package_id": "cve-2026-31431-copyfail",
  "version": "1.0.0",
  "package_url": "...",
  "signature_url": "...",
  "package_size": 123456
}
```

---

## 11. 版本与回滚

- `version` 使用 SemVer。
- 默认禁止降级。
- 显式 rollback 指令允许安装旧版本。
- 新版本安装失败保留旧 active。
- 运行中失败尝试回滚到同 `package_id` 上一 active 版本。

同一 `package_id`：

```text
active version <= 1
```

---

## 12. 离线包

离线部署时可将 DetectionPackage 放入 release 包：

```text
release/detection-packages/
  cve-2026-31431-copyfail-1.0.0.tar.gz
  cve-2026-31431-copyfail-1.0.0.tar.gz.sig
```

即使本地路径安装，也必须验签。
