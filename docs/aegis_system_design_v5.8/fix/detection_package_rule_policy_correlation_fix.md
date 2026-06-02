# 动态检测包规则管理、MITRE、阻断策略与告警关联修复

## 问题现象

Package `b1c4300a-d050-4b12-8b0f-b41fce167b1e` 启用后出现以下问题：

1. 规则管理新增 3 条 atomic 规则，但 `MITRE` 为空。
2. 阻断策略页报错：

```text
rule b1c4300a-d050-4b12-8b0f-b41fce167b1e.splice_call has no MITRE ID; policy count cannot be bound one-to-one
```

3. 告警列表中的最终规则 `b1c4300a-d050-4b12-8b0f-b41fce167b1e.copyfail_chain` 在规则管理中不存在。
4. 阻断策略无法表达同一 MITRE 下的多条 atomic/correlation 规则。
5. 告警列表和详情没有展示进程数。

## 根因

V5.8 设计中，动态 DetectionPackage 的处理链路是：

```text
atomic Sigma rule -> local correlation -> final correlation alert
```

当前实现存在断点：

- `syncSigmaRules` 只同步 atomic Sigma，不同步 correlation final rule。
- atomic Sigma YAML 解析只读取 `id/title/description/level`，没有解析 `tags`，因此无法提取 `attack.t1068`。
- 当前 CVE-2026-31431 atomic 规则资产只有 `cve.2026-31431` tag，没有 `attack.t1068`。
- 阻断策略 reconciliation 错误地要求 `规则数 == 策略数`，并要求 MITRE 和 rule 一对一；实际应是一个 MITRE policy 对多条规则。
- 告警 rule_id 使用 correlation rule id，但规则管理没有登记 correlation rule，导致告警无法反查规则。
- 告警没有 `process_count` 派生字段；动态包 correlation evidence 中的 pid 没有被用于前端展示。

## 修复设计

### 1. 规则管理同步

动态包启用时同步两类规则：

- Atomic Sigma rules：进入 `sigma_rules`，来源为 `detection_package`。
- Correlation final rule：也进入 `sigma_rules`，来源为 `detection_package_correlation`，用于告警列表反查规则名、MITRE、严重级别。

同时把 correlation 元数据写入 `correlation_rules`。

MITRE 提取策略：

1. 优先从 atomic Sigma `tags` 中解析 `attack.txxxx`。
2. 如果 atomic 缺少 MITRE，则从同包 correlation alert 的 `mitre_id` 回填。
3. correlation final rule 使用 `alert.mitre_id`。

### 2. 阻断策略聚合

阻断策略按 MITRE 聚合：

```text
MITRE technique -> one block policy -> many rules
```

不再要求规则和策略一对一，也不再因为某条历史规则缺少 MITRE 导致整个策略页 409。缺少 MITRE 的规则会被跳过并记录日志。

策略列表新增：

- `rule_title`：优先显示 correlation final rule 标题。
- `rule_count`：显示当前 MITRE 关联规则数。

### 3. 告警规则反查

最终 correlation alert 的 `rule_id` 会登记进 `sigma_rules`，因此：

```text
alerts.rule_id -> sigma_rules.rule_id
```

告警列表、告警详情、AI 分析上下文都可以反查到规则标题。

### 4. 进程数展示

API 返回告警时新增只读派生字段：

```text
process_count
```

统计来源：

1. 优先从 `process_tree` 统计父进程、当前进程和子进程的唯一 pid。
2. 若动态包没有完整 process tree，则从 `runtime_events.event_data` 的 correlation evidence 中统计唯一 pid。
3. 如果仍无法统计，但告警有 pid，则返回 1。

### 5. 签名密钥

按 V5.8 设计和本次要求：

- builder 内置 Ed25519 私钥。
- agent 内置对应 Ed25519 公钥。
- `--use-key-file` 和 `--signing-public-key` 保留为调试覆盖入口。

## 修改文件

- `builder/cmd/main.go`
- `agent/cmd/agent/main.go`
- `api-server/internal/service/detection_package_service.go`
- `api-server/internal/api/handler/detection_handler.go`
- `api-server/internal/repository/block_policy_repo.go`
- `api-server/internal/repository/alert_repo.go`
- `api-server/internal/model/alert.go`
- `api-server/internal/llm/prompts.go`
- `frontend/src/views/detection/Policies.vue`
- `frontend/src/views/detection/Alerts.vue`
- `frontend/src/types/index.ts`
- `docs/aegis_system_design_v5.8/fix/cve_2026_31431_runtime_detector/atomic_sigma.yml`

## 回归测试

新增测试覆盖：

- 动态包同步 2 条 atomic + 1 条 correlation 规则，MITRE 均为 `T1068`。
- 多条规则共享同一个 MITRE 时，只生成 1 条阻断策略。
- 告警详情可从 correlation evidence 派生 `process_count`。

计划执行：

```bash
docker run --rm -v /code/aegis:/workspace -w /workspace/api-server aegis-agent-builder-ubi8:5.8.0 go test ./internal/service -run 'SyncDetectionPackageRules|Allowlist|UpdateDraft'
docker run --rm -v /code/aegis:/workspace -w /workspace/api-server aegis-agent-builder-ubi8:5.8.0 go test ./internal/api/handler -run 'ReconcileRulePolicyBindings|ImportRules'
docker run --rm -v /code/aegis:/workspace -w /workspace/api-server aegis-agent-builder-ubi8:5.8.0 go test ./internal/repository -run 'AlertRepo'
cd frontend && npm run type-check
```

当前会话中 Docker/Go 执行被环境额度限制拦截，尚未能完成实际测试运行。权限恢复后需要重跑上述命令，并重建 `api-server`、`builder`、`agent` 后重新签名启用动态包。

## 重新发布步骤

1. 重建 `builder`，使用内置私钥。
2. 重建并重装 `agent`，使用内置公钥，不再依赖 systemd `--signing-public-key` 参数。
3. 更新 CVE-2026-31431 草稿中的 atomic Sigma，补齐 `attack.t1068` tag。
4. 重新 build -> review -> sign -> enable。
5. 触发安全 PoC。
6. 验证：

```text
规则管理：3 条 atomic + 1 条 correlation 均存在，MITRE=T1068
阻断策略：T1068 一条策略，rule_count=4
告警列表：rule_id 能在规则管理中找到
告警列表/详情：process_count 有值
```

## 风险与回滚

- 规则同步只影响动态 DetectionPackage 启用流程，不改变 agent 本地匹配逻辑。
- 阻断策略从一对一改为 MITRE 聚合，符合现有 `block_policies.mitre_id` 表结构。
- 如需回滚，可恢复旧的 `syncSigmaRules` 和 `reconcileRulePolicyBindings`，但会再次暴露多规则同 MITRE 和 correlation rule 缺失问题。
