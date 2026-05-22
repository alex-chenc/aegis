# Prompt 06: Builder 容器、打包与签名实现

你是 Aegis 构建与发布工程师。请实现 V5.8 动态 eBPF DetectionPackage 的 builder 和签名发布流程。

## 参考文档

- `docs/aegis_system_design_v5.8/overall_architecture_design_v5.8.md`
- `docs/aegis_system_design_v5.8/api_grpc_design_v5.8.md`
- `docs/aegis_system_design_v5.8/builder_release_design_v5.8.md`
- `agent/Makefile`
- `scripts/build_release_package.sh`

## 目标

新增 `aegis-agent-builder-ubi8:5.8.0` builder 镜像，agent release 和动态 eBPF 插件都使用同一个 builder image。

同时新增 `builder` 控制面内部服务，只允许 API Server 通过 gRPC 调用。Builder 不直接和 frontend、server、agent、dc 通信。

## gRPC 服务

实现内部 `BuilderService`：

```proto
rpc GetBuilderInfo(GetBuilderInfoRequest) returns (GetBuilderInfoResponse);
rpc StartPackageBuild(StartPackageBuildRequest) returns (StartPackageBuildResponse);
rpc GetPackageBuildStatus(GetPackageBuildStatusRequest) returns (GetPackageBuildStatusResponse);
rpc SignPackage(SignPackageRequest) returns (SignPackageResponse);
```

要求：

- 大文件不通过 gRPC 返回，builder 上传到 MinIO 后返回 object key。
- API Server 是业务状态源，builder 不直接写 PostgreSQL。
- `SignPackage` 只能签名 builder 产出的成功 build。

## 要求

- 基于 UBI8。
- 安装 Go、clang、llvm、make、bpftool、libbpf headers、linux uapi headers。
- 包含 agent 的 eBPF 公共头文件。
- 支持显式参数：
  - `BPF_TARGET_ARCH=x86|arm`
  - `BPF_TRANSPORT=perf|ringbuf`
- 每个插件默认生成：
  - `*.perf.bpf.o`
  - `*.ringbuf.bpf.o`

## 打包

生成 staging：

```text
package.yaml
plugin/plugin.yaml
plugin/*.perf.bpf.o
plugin/*.ringbuf.bpf.o
rules/atomic_sigma.yml
correlations/*.yml
```

打包：

```text
package.tar.gz
package.tar.gz.sig
```

## 签名

- 使用 Ed25519。
- 签名整个 tar.gz。
- 私钥编译进 V5.8 新增 builder 组件。
- 不设计独立 signer/KMS/HSM。
- API Server 只触发签名动作，不持有私钥。
- agent 只内置公钥。
- builder 禁止在日志、manifest、API 响应或对象存储元数据中输出私钥。

## 构建校验

- YAML 可解析。
- Correlation 引用的 Sigma rule_id 存在。
- plugin manifest hooks 和程序名齐全。
- perf/ringbuf artifact 都存在。
- 构建日志写回 api-server。

## 验收

- builder 镜像可重复构建。
- 同一输入生成 perf/ringbuf 两份 artifact。
- package 可签名。
- agent 可用内置公钥验签。
- 构建日志包含 builder digest 和 clang version。
