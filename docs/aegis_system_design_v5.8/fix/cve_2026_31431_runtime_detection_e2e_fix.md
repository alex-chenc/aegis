# CVE-2026-31431 动态检测包端到端修复与验证记录

## 问题对象

- Package ID: `b1c4300a-d050-4b12-8b0f-b41fce167b1e`
- Draft ID: `36edd0ac-314c-4f0b-8f71-4f3e36706669`
- Version: `1.0.0`
- Build ID: `54bdf67e-9073-4f20-bc49-01c23dcdb7a7`
- Host ID: `cf18f7f7-5b45-46e2-9889-160dddc4ee30`
- Hostname: `chenc-VMware-Virtual-Platform`

## 修复结论

本次问题不是单点失败，而是动态检测包从构建到运行的链路中连续暴露了五个缺口：

1. 原始 eBPF 草稿依赖 `bpf_get_current_task` 和 `struct task_struct`，不符合当前最小 eBPF 构建环境和 builder validation 约束。
2. validation 失败时 builder 没有填充 `BuildLogTail`，页面只能看到泛化错误，无法看到真实原因。
3. 草稿更新后仍保留旧的构建状态和 `last_build_id`，容易让前端和后续发布流程误用旧构建结果。
4. Agent 未配置 builder 的签名公钥，签名包下载后停在 `signature_failed`，导致 host 状态长时间为 `verifying`。
5. Runtime event 由 server 先入库并投递 Kafka，DC 消费后再次写入同一个 `event_id`，触发 `runtime_events_event_id_key` duplicate key 错误。

修复后，CVE-2026-31431 动态检测包已经完成构建、审核、签名、启用、Agent 激活和安全 PoC 触发验证。API 告警接口可查询到 `critical` 告警。

## 代码修复

### 1. Builder validation 失败日志

文件：`builder/internal/service/builder_service.go`

- validation 失败时生成 `=== validation ===` 日志。
- 调用 `storeBuildResult` 和 `populateFailedBuildLog`，确保 `BuildLogTail` 回传到 API Server。
- `populateFailedBuildLog` 增加 MinIO client nil guard，避免本地单测或无 MinIO 环境下诊断路径 panic。

覆盖测试：

- `builder/internal/service/builder_service_test.go`
- 新增 `TestStartBuildValidationFailureIncludesBuildLogTail`

### 2. AI 动态包生成约束

文件：`api-server/internal/llm/prompts.go`

- 明确要求 eBPF 草稿支持 `AEGIS_EVENT_PERF` 和 `AEGIS_EVENT_RINGBUF` 双 transport。
- 明确禁止 `bpf_get_current_task` 和直接解引用 `struct task_struct`。
- 明确统一事件 envelope 和 TLV payload 格式，避免生成的字段格式和 Agent 解析侧不一致。

### 3. 草稿更新后重置构建状态

文件：`api-server/internal/service/detection_package_service.go`

- `UpdateDraft` 在草稿内容变化后将状态重置为 `draft`。
- 清空 `LastBuildID`，避免草稿已经变化但仍引用旧构建。

覆盖测试：

- `api-server/internal/service/detection_package_service_test.go`
- 新增 `TestUpdateDraftResetsBuildStatus`

### 4. Allowlist 同步补齐 version

文件：`api-server/internal/service/detection_package_service.go`

- `UpdateAllowlist` 同步给 agent 时注入 allowlist `version`。
- 避免数据库已有版本号，但下发 JSON 中缺少版本字段。

覆盖测试：

- `TestAllowlistConfigPayloadAddsVersion`
- `TestUpdateAllowlistSyncsVersionedPayload`

### 5. DC 事件入库幂等

文件：`dc/internal/repository/runtime_event_repo.go`

- `Create`、`CreateWithContext`、`CreateBatch` 对 `event_id` 使用 `ON CONFLICT DO NOTHING`。
- 解决 server 已经入库 runtime event 后，DC 从 Kafka 消费同一个事件再次入库导致 duplicate key 的问题。
- 保留后续告警生成流程，不因重复事件写入中断 DC 处理。

## 检测包修复资产

目录：`docs/aegis_system_design_v5.8/fix/cve_2026_31431_runtime_detector/`

- `hook_plan.yaml`
- `plugin.c`
- `atomic_sigma.yml`
- `correlation.yml`

核心策略：

- hook `syscalls/sys_enter_socket`
- hook `syscalls/sys_enter_bind`
- hook `syscalls/sys_enter_splice`
- 按 pid 在 10 秒窗口内关联 AF_ALG socket、AF_ALG bind、splice 行为链。
- 触发后生成规则 `b1c4300a-d050-4b12-8b0f-b41fce167b1e.copyfail_chain`。
- MITRE 标记：`T1068`
- Severity：`critical`

eBPF 修复原则：

- 不使用 `bpf_get_current_task`。
- 不读取内核 `task_struct`。
- 使用 `bpf_get_current_pid_tgid` 获取 pid/tid。
- perf/ringbuf 通过 `AEGIS_EVENT_PERF` 和 `AEGIS_EVENT_RINGBUF` 条件编译。
- 事件输出符合 Agent 动态包统一 envelope。

## Agent 运行修复

Agent 初次安装签名包后，本机状态文件显示：

```text
state: signature_failed
reason: signing public key not configured, cannot verify signature
```

原因是 builder 使用 Ed25519 签名发布动态检测包，但本机 `aegis-agent` systemd 启动参数没有配置签名公钥。

已将 systemd 启动命令调整为：

```text
ExecStart=/opt/aegis-agent/aegis-agent --signing-public-key=82d0b16d992d9efff4b830ba011ccb74298aa42bfdcacea79aff85d81bcd691a
```

随后执行：

```bash
systemctl daemon-reload
systemctl restart aegis-agent
```

重新同步 allowlist，并对动态包执行 disable -> enable 后，Agent 本地状态变为：

```text
state: active
reason: package active
```

## 构建与发布验证

builder 运行环境已经切换到 UBI8 eBPF builder 基础镜像：

```text
go version go1.25.0 linux/amd64
clang version 21.1.8
AEGIS_EBPF_INCLUDE=/opt/aegis/ebpf/include
```

动态包构建结果：

```text
Build ID: 54bdf67e-9073-4f20-bc49-01c23dcdb7a7
Status: awaiting_review -> success -> signed -> enabled
Build log:
=== compile perf x86 ===
=== compile ringbuf x86 ===
```

构建产物：

```text
perf artifact sha256:   sha256:4d506c63a200b5ea3401ea47e741b19182fcfbd6b8cf5178045e731abe4a5beb
ringbuf artifact sha256: sha256:d2bd0d2faf7840a7a2780c0486fb0b09797f8493888544dc736c1b17e55739ed
signed package: detection-packages/b1c4300a-d050-4b12-8b0f-b41fce167b1e/1.0.0/signed/package.tar.gz
```

## 端到端运行验证

### 服务健康

`docker compose ps` 显示核心服务均处于 healthy/up：

```text
aegis-api-server   Up healthy
aegis-builder      Up
aegis-dc           Up healthy
aegis-frontend     Up healthy
aegis-kafka        Up healthy
aegis-minio        Up healthy
aegis-postgres     Up healthy
aegis-redis        Up healthy
aegis-server       Up healthy
```

API health：

```text
GET /health -> {"status":"ok"}
```

### 安全 PoC 触发

使用 `scripts/cve_2026_31431_safe_trigger.c` 编译了一个非破坏性触发程序。该程序只触发 AF_ALG socket/bind 和 splice 行为链，不执行提权、不覆盖文件、不修改系统敏感路径。

触发输出：

```text
bind(AF_ALG/aead/gcm(aes)) failed: No such file or directory
safe trigger completed, spliced=34
```

说明：本机内核未提供该 AF_ALG 算法时，`bind` 可能返回 `ENOENT`，但 syscall 行为已经发生，检测包仍可采集 socket/bind/splice 行为链。

### DC 日志

修复 DC 幂等写入后，第二次触发日志：

```text
Event persisted {"event_id": "EVT-31f3e6df", "host_id": "cf18f7f7-5b45-46e2-9889-160dddc4ee30", "event_type": "correlation_alert"}
Alert generated {"alert_id": "ALT-deed5cd9", "severity": "critical", "mitre_id": "T1068"}
```

未再出现 `duplicate key value violates unique constraint "runtime_events_event_id_key"`。

### API 告警结果

登录账号：

```text
admin / Admin@123
```

告警接口：

```text
GET /api/v1/detection/alerts?page=1&page_size=5
```

返回结果包含 2 条 CVE-2026-31431 行为链告警：

```text
total: 2
severity: critical
rule_id: b1c4300a-d050-4b12-8b0f-b41fce167b1e.copyfail_chain
rule_title: Possible CVE-2026-31431 CopyFail AF_ALG Exploitation Sequence
mitre_id: T1068
mitre_name: 权限提升漏洞利用
host: chenc-VMware-Virtual-Platform
status: pending
```

## 回归测试

已执行并通过：

```bash
docker run --rm -v /code/aegis:/workspace -w /workspace/builder aegis-agent-builder-ubi8:5.8.0 go test ./internal/service
docker run --rm -v /code/aegis:/workspace -w /workspace/api-server aegis-agent-builder-ubi8:5.8.0 go test ./internal/service -run 'DetectionPackage|Allowlist|UpdateDraft'
docker run --rm -v /code/aegis:/workspace -w /workspace/agent aegis-agent-builder-ubi8:5.8.0 go test ./internal/dynpkg ./internal/ebpf/plugin ./internal/sigma
docker run --rm -v /code/aegis:/workspace -w /workspace/dc aegis-agent-builder-ubi8:5.8.0 go test ./internal/repository ./internal/event_handler ./internal/alert_generator
```

结果：

```text
ok builder/internal/service
ok api-server/internal/service
ok aegis-agent/internal/sigma
ok dc/internal/repository
ok dc/internal/event_handler
ok dc/internal/alert_generator
```

## 当前结论

CVE-2026-31431 动态检测包已经可以在当前主机上完成完整闭环：

```text
草稿更新 -> builder 编译 -> 人工审核 -> 签名 -> 启用 -> allowlist 同步 -> agent 激活 -> 安全 PoC 触发 -> eBPF 采集 -> Sigma 命中 -> correlation 告警 -> server/DC 入库 -> API 可查询
```

本次问题已修复。后续如果希望把该动态包作为正式内置检测规则，应将 `docs/aegis_system_design_v5.8/fix/cve_2026_31431_runtime_detector/` 下的四份资产迁移到正式规则源，并把 Agent 签名公钥配置纳入部署模板或配置中心，避免新主机重复出现 `signature_failed`。
