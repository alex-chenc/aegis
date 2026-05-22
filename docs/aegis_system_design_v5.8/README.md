# Aegis V5.8 动态 eBPF 检测包设计文档

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. 版本目标

V5.8 引入 **动态 eBPF DetectionPackage** 能力，用于解决固定 agent 采集面无法覆盖新型漏洞利用链的问题。系统允许 AI 生成检测草稿，人工修改后通过统一 builder 容器编译，生成签名检测包，由管理员在页面显式启用后全局下发到 agent。

核心目标：

- 通过动态 eBPF 插件扩展运行时采集面。
- 通过 HookPlan、Sigma atomic rules、Correlation DetectionSpec 分离采集、单事件匹配和多事件关联。
- 保留人工审核、人工签名、人工启用的安全边界。
- agent 只加载完整包签名验证通过的 DetectionPackage。
- 动态插件加载失败只禁用该插件，agent 主进程继续运行。

---

## 2. 文档索引

| 文档 | 说明 |
|:---|:---|
| `overall_architecture_design_v5.8.md` | 总体架构、builder 组件定位、组件通信关系、端到端链路 |
| `dynamic_ebpf_detection_package_design.md` | DetectionPackage 核心决策、包结构、生命周期 |
| `frontend_prd_dynamic_ebpf_v5.8.md` | 前端 PRD、页面、交互、状态展示 |
| `database_structure_design_v5.8.md` | 数据库表结构、索引、状态枚举 |
| `api_grpc_design_v5.8.md` | HTTP API 与 gRPC 扩展设计 |
| `agent_dynamic_ebpf_design_v5.8.md` | agent 动态加载、验签、插件管理、关联引擎设计 |
| `builder_release_design_v5.8.md` | builder 容器、编译、签名、对象存储分发 |
| `code_interfaces_v5.8.md` | Go/Proto/TypeScript 关键接口草案 |
| `cve_2026_31431_copyfail_example_v5.8.md` | CVE-2026-31431 示例包设计 |
| `prompt/` | 各组件实现提示词 |

---

## 3. 已确认设计决策

| 决策点 | 结论 |
|:---|:---|
| AI 是否直接下发 eBPF 代码 | 不允许。AI 产出草稿，最终必须人工构建、签名、启用 |
| HookPlan 与 DetectionSpec | 分离。HookPlan 只描述采集，DetectionSpec 只描述关联检测 |
| 插件事件格式 | 统一事件信封 `aegis_plugin_event` + TLV payload |
| TLV 字段定义 | 插件 manifest 自带 schema，基础字段固定在事件信封 |
| 单事件检测 | 复用当前 Sigma matcher 作为 AtomicFinding 生成层 |
| 多事件检测 | agent 本地 Correlation Engine，支持 ordered sequence + window + by |
| Correlation 输入 | 只消费 Sigma 命中的 AtomicFinding |
| DetectionPackage 启用范围 | 第一版全局启用，全部 agent 下发 |
| DetectionPackage 签名 | Ed25519 公私钥签名整个 tar.gz 包 |
| 签名私钥 | 编译进 V5.8 新增 builder 组件，不设计独立 signer/KMS |
| 公钥管理 | agent 编译时内置官方公钥 |
| 签名包分发 | MinIO/对象存储保存包和签名，控制面下发 URL |
| Hook allowlist | agent 不内置，完全使用页面全局配置，下发后才允许加载动态插件 |
| 默认 allowlist | 页面初始化默认只开放 tracepoint，不默认开放 kprobe/lsm/xdp/tc |
| perf/ringbuf | 每个插件默认生成两份 `.perf.bpf.o` 与 `.ringbuf.bpf.o`，agent 自行选择 |
| 插件加载失败 | 只禁用该插件或 package，agent 继续运行 |
| package 版本 | `package_id + SemVer`，同 package_id 同时只能一个 active |
| 回滚 | 默认禁止降级，显式 rollback 指令例外；安装失败保留旧 active |
| 卸载 | 删除 agent 本地 artifact |
| 原始事件上报 | 默认不上报，命中 correlation 后上报告警和 evidence chain |

---

## 4. 核心链路

```text
AI 生成草稿
  -> 人工修改 HookPlan / Sigma / Correlation / eBPF 源码
  -> 人工提交构建
  -> api-server 调用 builder gRPC
  -> builder 使用 aegis-agent-builder 容器编译 perf/ringbuf .bpf.o
  -> 页面展示构建结果、hook、schema、规则、风险信息
  -> 人工触发 builder 使用内置私钥签名发布 DetectionPackage
  -> 人工启用 package
  -> api-server 通过 server 下发全局安装指令
  -> agent 下载 package.tar.gz 和 .sig
  -> agent 用内置公钥验签整个包
  -> agent 校验 hook allowlist
  -> agent 加载 ringbuf 或 perf artifact
  -> 插件事件进入 Sigma atomic rules
  -> AtomicFinding 进入本地 Correlation Engine
  -> 命中后上报 correlation alert + evidence
```

---

## 5. 非目标

V5.8 第一版不做：

- agent 本机编译 eBPF 源码。
- agent 加载未签名 DetectionPackage。
- 自动签名、自动启用、自动全链路发布。
- 主机组灰度发布。第一版管理员启用后全局下发。
- 跨主机关联检测。
- 复杂 CEP，包括 count、absence、DAG、多分支负条件。
- 插件源码打入最终 DetectionPackage。
- agent 内置 hook allowlist。
