# V5.8 动态 eBPF DetectionPackage 总体设计

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. 背景

V5.7 agent 的 eBPF 事件面已经覆盖进程、文件、网络等基础场景，但固定探针无法保证覆盖未来漏洞。例如 CVE-2026-31431 这类本地提权利用链，需要观测 `AF_ALG`、`bind`、`splice`、后续 root exec 等特定行为。如果底层没有对应 hook 点，单纯新增上层 Sigma 规则无法命中。

V5.8 目标是在保持安全边界的前提下，引入动态 eBPF 检测能力。

---

## 2. 核心原则

1. **AI 可生成草稿，不可直接发布到 agent。**
   - AI 可以生成 HookPlan、Sigma atomic rules、Correlation DetectionSpec、eBPF 插件源码草稿。
   - 人工可修改草稿。
   - 构建、签名、启用均需要人工动作。

2. **采集与检测分层。**
   - HookPlan 描述采集能力。
   - eBPF 插件只做 hook、字段提取、轻量过滤、事件输出。
   - Sigma 负责单事件 atomic finding。
   - Correlation DetectionSpec 负责多事件漏洞利用链。

3. **DetectionPackage 是唯一发布单元。**
   - 插件 artifact、manifest、Sigma、Correlation 必须打成一个包。
   - 同一 `package_id` 只有一个 active version。
   - agent 以 package 为单位安装、启用、禁用、卸载。

4. **安全边界在签名包。**
   - agent 只验整个 `package.tar.gz` 的 Ed25519 签名。
   - Ed25519 私钥编译进 V5.8 新增 builder 组件。
   - 公钥编译进 agent。
   - 签名失败拒绝安装。

5. **动态能力受页面全局 hook allowlist 约束。**
   - agent 不内置 allowlist。
   - agent 收到页面下发的全局 allowlist 之前，不加载任何动态 eBPF package。
   - 默认页面配置只开放 tracepoint，kprobe/lsm/xdp/tc 默认关闭。

---

## 3. 角色与职责

| 组件 | 职责 |
|:---|:---|
| AI 规则生成服务 | 基于 CVE 情报生成 HookPlan、Sigma、Correlation、源码草稿 |
| 管理员 | 修改草稿、提交构建、审核构建结果、签名发布、启用 package |
| api-server | 管理草稿、构建任务、包元数据、全局 allowlist、发布状态、前端 API |
| ebpf-builder | 使用 agent 同源 builder 容器编译 `.bpf.o`，产出构建日志和 package staging，并在人工确认后使用内置私钥签名 |
| MinIO | 保存签名 package 与 `.sig` |
| server | 将 install/uninstall/config 指令转发给在线 agent |
| agent | 下载验签、校验 allowlist、加载插件、运行 Sigma 和 Correlation、上报状态和告警 |
| dc | 接收最终 correlation alert，入库、聚合、通知 |

---

## 4. DetectionPackage 目录结构

最终下发包不包含源码：

```text
cve-2026-31431-copyfail-1.0.0.tar.gz
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

签名文件：

```text
cve-2026-31431-copyfail-1.0.0.tar.gz.sig
```

---

## 5. package.yaml

```yaml
schema_version: "aegis.detection_package.v1"
package_id: "cve-2026-31431-copyfail"
version: "1.0.0"
title: "CVE-2026-31431 CopyFail Runtime Detector"
description: "Detect AF_ALG AEAD + splice + suspicious root exec chains."
min_agent_version: "5.8.0"
created_at: "2026-05-22T00:00:00Z"
detects:
  cves:
    - "CVE-2026-31431"
  tags:
    - "linux"
    - "privilege_escalation"
    - "copyfail"
plugin:
  manifest: "plugin/plugin.yaml"
artifacts:
  perf: "plugin/copyfail.perf.bpf.o"
  ringbuf: "plugin/copyfail.ringbuf.bpf.o"
sigma_rules:
  - "rules/atomic_sigma.yml"
correlation_rules:
  - "correlations/copyfail_correlation.yml"
limits:
  max_events_per_second: 1000
  max_events_per_pid_per_second: 100
  auto_disable_on_sustained_overflow: true
build:
  builder_image: "aegis-agent-builder-ubi8:5.8.0"
  builder_digest: "sha256:<digest>"
  clang_version: "recorded-by-builder"
```

约束：

- `package_id` 是检测能力稳定身份。
- `version` 使用 SemVer。
- 同 `package_id` 下默认只允许升级。
- `package_id` 不允许包含路径分隔符。

---

## 6. HookPlan

HookPlan 是构建输入，不直接下发到 agent。它只描述采集计划：

```yaml
schema_version: "aegis.hook_plan.v1"
package_id: "cve-2026-31431-copyfail"
hooks:
  - name: "af_alg_socket"
    attach_type: "tracepoint"
    attach: "syscalls/sys_enter_socket"
    extract:
      - { index: 0, name: "family", type: "int" }
      - { index: 1, name: "socket_type", type: "int" }
      - { index: 2, name: "protocol", type: "int" }
    filter:
      family: "AF_ALG"
    emit:
      event_type: "af_alg_socket"

  - name: "af_alg_bind"
    attach_type: "tracepoint"
    attach: "syscalls/sys_enter_bind"
    extract:
      - { index: 1, name: "sockaddr_alg", type: "sockaddr_alg" }
    filter:
      salg_type: "aead"
    emit:
      event_type: "af_alg_bind"

  - name: "splice_call"
    attach_type: "tracepoint"
    attach: "syscalls/sys_enter_splice"
    extract:
      - { index: 0, name: "fd_in", type: "int" }
      - { index: 2, name: "fd_out", type: "int" }
      - { index: 4, name: "len", type: "uint64" }
    emit:
      event_type: "splice_call"
```

HookPlan 不包含：

- correlation window
- sequence
- alert severity
- auto block
- 告警文案

---

## 7. 插件 manifest

`plugin/plugin.yaml` 是 agent 安装时读取的插件说明：

```yaml
schema_version: "aegis.ebpf_plugin.v1"
plugin_id: "copyfail_probe"
package_id: "cve-2026-31431-copyfail"
event_map: "plugin_events"
hooks:
  - name: "af_alg_socket"
    attach_type: "tracepoint"
    attach: "syscalls/sys_enter_socket"
    program: "trace_af_alg_socket"
  - name: "af_alg_bind"
    attach_type: "tracepoint"
    attach: "syscalls/sys_enter_bind"
    program: "trace_af_alg_bind"
  - name: "splice_call"
    attach_type: "tracepoint"
    attach: "syscalls/sys_enter_splice"
    program: "trace_splice"
event_schema:
  events:
    1001:
      name: "af_alg_socket"
      fields:
        1: { name: "family", type: "string" }
        2: { name: "socket_type", type: "int32" }
        3: { name: "protocol", type: "int32" }
    1002:
      name: "af_alg_bind"
      fields:
        1: { name: "alg_type", type: "string" }
        2: { name: "alg_name", type: "string" }
    1003:
      name: "splice_call"
      fields:
        1: { name: "fd_in", type: "int32" }
        2: { name: "fd_out", type: "int32" }
        3: { name: "len", type: "uint64" }
```

---

## 8. Sigma atomic rules

Atomic Sigma 规则只匹配单个事件。`rule_id` 必须稳定并使用 package 命名空间：

```yaml
title: CopyFail AF_ALG AEAD Bind
id: cve-2026-31431-copyfail.af_alg_bind_aead
status: experimental
description: AF_ALG bind to AEAD algorithm.
logsource:
  product: linux
  category: kernel_plugin
detection:
  selection:
    event_type: af_alg_bind
    alg_type: aead
  condition: selection
level: high
tags:
  - cve.2026-31431
```

约束：

- 第一版 correlation 只能引用同 package 内的 atomic rule。
- PATCH/MINOR 升级不改变 atomic rule ID。
- MAJOR 才允许破坏性 rule ID 变更。

---

## 9. Correlation DetectionSpec

Correlation 只做 agent 本地短窗口关联：

```yaml
schema_version: "aegis.correlation.v1"
id: "cve-2026-31431-copyfail.chain"
package_id: "cve-2026-31431-copyfail"
requires:
  - "cve-2026-31431-copyfail.af_alg_socket"
  - "cve-2026-31431-copyfail.af_alg_bind_aead"
  - "cve-2026-31431-copyfail.splice_call"
  - "cve-2026-31431-copyfail.suspicious_root_exec"
correlation:
  by: "pid_tree"
  window: "10s"
  ordered: true
  sequence:
    - rule_id: "cve-2026-31431-copyfail.af_alg_socket"
    - rule_id: "cve-2026-31431-copyfail.af_alg_bind_aead"
    - rule_id: "cve-2026-31431-copyfail.splice_call"
    - rule_id: "cve-2026-31431-copyfail.suspicious_root_exec"
alert:
  title: "Possible CVE-2026-31431 CopyFail Exploitation"
  severity: "critical"
  mitre_id: "T1068"
  cve_id: "CVE-2026-31431"
```

第一版仅支持：

- `ordered: true`
- `window <= 60s`
- `by: pid | pid_tree | host`
- 线性 `sequence`

不支持 count、absence、DAG、跨主机、复杂布尔。

---

## 10. Agent 安装事务

```text
1. 收到 install command
2. 下载 package.tar.gz 和 package.tar.gz.sig
3. 使用内置公钥验签整个 tar.gz
4. 解包到 staging 目录
5. 解析 package.yaml/plugin.yaml/Sigma/Correlation
6. 校验 package hooks 是当前全局 hook allowlist 子集
7. 根据本机能力优先加载 ringbuf，失败 fallback perf
8. 加载 Sigma 到 package-scoped rule loader
9. 加载 Correlation DetectionSpec
10. 切换 package active 指针
11. 上报 active 状态
```

失败处理：

- 验签失败：拒绝安装。
- hook 不在 allowlist：`blocked_by_hook_allowlist`。
- 插件加载失败：只禁用该 package，agent 继续运行。
- 新版本安装失败：保留旧 active version。

---

## 11. 状态机

```text
uploaded
  -> build_pending
  -> build_running
  -> build_failed
  -> built
  -> signed
  -> enabled
  -> installing
  -> active
  -> degraded
  -> load_failed
  -> disabled
  -> uninstalled
```

主机级状态：

```text
pending
downloading
signature_failed
blocked_by_hook_allowlist
installing
active
degraded
load_failed
disabled_by_policy
disabled_by_rate
rolled_back
uninstalled
```

---

## 12. 事件上报策略

动态插件原始事件默认不上报服务端。

上报条件：

- Sigma 命中后进入本地 Correlation。
- Correlation 命中后上报一条最终 runtime event/alert。
- `event_data_json` 带 evidence chain。

```json
{
  "package_id": "cve-2026-31431-copyfail",
  "correlation_id": "cve-2026-31431-copyfail.chain",
  "evidence": [
    {"rule_id": "cve-2026-31431-copyfail.af_alg_socket", "pid": 1234},
    {"rule_id": "cve-2026-31431-copyfail.af_alg_bind_aead", "pid": 1234, "alg_type": "aead"},
    {"rule_id": "cve-2026-31431-copyfail.splice_call", "pid": 1234},
    {"rule_id": "cve-2026-31431-copyfail.suspicious_root_exec", "pid": 1250, "uid": 0}
  ]
}
```

---

## 13. 资源保护

Correlation 默认限制：

| 项 | 默认 | 硬上限 |
|:---|:---|:---|
| window | 10s | 60s |
| events per package/key | 128 | 256 |
| global AtomicFinding cache | 10000 | 配置项 |

限速默认值：

| 维度 | 默认 |
|:---|:---|
| per plugin | 1000 events/s |
| per event_type | 500 events/s |
| per pid | 100 events/s |

持续超限：

- 丢弃原始 plugin events。
- 上报 `plugin_rate_limited`。
- 持续超阈值自动禁用插件并上报 `disabled_by_rate`。
