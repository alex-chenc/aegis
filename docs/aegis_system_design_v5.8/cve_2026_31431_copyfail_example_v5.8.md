# V5.8 示例: CVE-2026-31431 CopyFail DetectionPackage

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 示例设计

---

## 1. 检测思路

CVE-2026-31431 类型的本地提权利用链需要观测：

```text
AF_ALG socket
  -> bind 到 AEAD 算法
  -> splice 调用
  -> 后续 root exec 或异常权限变化
```

单个事件误报较高，最终告警必须依赖 agent 本地 correlation。

---

## 2. package.yaml

```yaml
schema_version: "aegis.detection_package.v1"
package_id: "cve-2026-31431-copyfail"
version: "1.0.0"
title: "CVE-2026-31431 CopyFail Runtime Detector"
description: "Detect AF_ALG AEAD and splice exploitation chain with suspicious root execution."
min_agent_version: "5.8.0"
detects:
  cves:
    - "CVE-2026-31431"
  tags:
    - "linux"
    - "local_privilege_escalation"
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
```

---

## 3. HookPlan

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

---

## 4. plugin.yaml

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
    program: "trace_splice_call"
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

## 5. Atomic Sigma

```yaml
---
title: CopyFail AF_ALG Socket
id: cve-2026-31431-copyfail.af_alg_socket
status: experimental
description: AF_ALG socket creation observed.
logsource:
  product: linux
  category: kernel_plugin
detection:
  selection:
    event_type: af_alg_socket
    family: AF_ALG
  condition: selection
level: medium
tags:
  - cve.2026-31431

---
title: CopyFail AF_ALG AEAD Bind
id: cve-2026-31431-copyfail.af_alg_bind_aead
status: experimental
description: AF_ALG bind to AEAD algorithm observed.
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

---
title: CopyFail Splice Call
id: cve-2026-31431-copyfail.splice_call
status: experimental
description: splice syscall observed in CopyFail candidate chain.
logsource:
  product: linux
  category: kernel_plugin
detection:
  selection:
    event_type: splice_call
  condition: selection
level: medium
tags:
  - cve.2026-31431

---
title: Suspicious Root Exec After User Process
id: cve-2026-31431-copyfail.suspicious_root_exec
status: experimental
description: Root shell or privileged utility execution observed.
logsource:
  product: linux
  category: process_creation
detection:
  selection_uid:
    uid: 0
  selection_image:
    image|re:
      - '(/bin/(ba)?sh)$'
      - '(/usr/bin/(su|sudo|passwd|newgrp))$'
  condition: selection_uid and selection_image
level: high
tags:
  - cve.2026-31431
```

---

## 6. Correlation

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

---

## 7. 告警 evidence

命中后上报：

```json
{
  "package_id": "cve-2026-31431-copyfail",
  "package_version": "1.0.0",
  "correlation_rule_id": "cve-2026-31431-copyfail.chain",
  "evidence": [
    {
      "rule_id": "cve-2026-31431-copyfail.af_alg_socket",
      "event_type": "af_alg_socket",
      "pid": 1234,
      "uid": 1000,
      "family": "AF_ALG"
    },
    {
      "rule_id": "cve-2026-31431-copyfail.af_alg_bind_aead",
      "event_type": "af_alg_bind",
      "pid": 1234,
      "uid": 1000,
      "alg_type": "aead",
      "alg_name": "gcm(aes)"
    },
    {
      "rule_id": "cve-2026-31431-copyfail.splice_call",
      "event_type": "splice_call",
      "pid": 1234,
      "uid": 1000
    },
    {
      "rule_id": "cve-2026-31431-copyfail.suspicious_root_exec",
      "event_type": "process_exec",
      "pid": 1250,
      "ppid": 1234,
      "uid": 0,
      "image": "/bin/bash"
    }
  ]
}
```

---

## 8. 预期效果

| 场景 | 行为 |
|:---|:---|
| 只出现 AF_ALG socket | 只产生本地 AtomicFinding，不上报最终告警 |
| AF_ALG + AEAD + splice，无 root exec | 不上报最终告警 |
| 完整链路在 10 秒内出现 | 上报 critical correlation alert |
| 插件加载失败 | package load_failed，agent 继续运行 |
| hook 不在 allowlist | package blocked_by_hook_allowlist |

