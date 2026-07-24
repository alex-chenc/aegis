# 修复：工具参数确定性绑定、Mapping 条件执行图、capability 启动强校验

## 问题

1. **参数未完全后端化**：`previous_step` 源参数只记录来源标记，无实际注入机制。`applyPlanArgs` 只返回编译参数、丢弃模型参数，导致轮询配套工具（`Operation.Get`、`Asset.Collection.Get`、`Vulnerability.Scan.Status` 等）的必填参数（`operation_id`、`task_id`、`scan_id`）永远缺失。`Asset.Collection.Trigger` 还缺少 `OperationRefFields: ["task_id"]`。

2. **Mapping 偏"能力集合"**：`ReadonlyCompanionToolNames` 把只读轮询配套工具作为无条件 plan step 注入；`ToolPlanStep.Condition` 存在但仅作 objective 文案，不参与执行门控。`validateToolPrerequisites` 已处理发现型配套（`PrerequisiteCapabilityEmptyResult`），但轮询型配套无此保障。

3. **capability 启动强校验缺失**：`Register` 只比较原始 `Capability` 字段（非空时），两个空 `Capability` + 同 `domain+operation` 的工具经 `syntheticToolCapability` 合成相同能力但未被拦截。无独立启动校验方法。

## 修复内容

### A. 运行时 previous_step 确定性绑定
**文件**：`api-server/internal/assistant/adapter_tool_gateway.go`

- `AssistantToolGatewayAdapter` 新增 per-run 并发安全的 `priorStepOutcomes map[string]capturedStepOutcome`，捕获每个 Mapping 步骤的 `OperationRef`/`SideEffects`/`terminal` 状态。
- 每次 `Call` 成功（含 Accepted/非终态/asyncPoll）后，`captureStepOutcome` 按 stepID 记录引用。
- `resolvePreviousStepArgs` 在 `Prepare` 中 `applyPlanArgs` 之后执行：按 `ArgSources.SourceType == "previous_step"` 的参数名，从已捕获的前序 outcome（倒序）匹配 `OperationRef`/`SideEffects` 字段名并注入。生产者值**始终覆盖**模型值（确定性绑定）。若必填 `previous_step` 参数无法解析，`Prepare` 返回明确的 skip 错误，阻止派发。

### B. 条件编译进 plan
**文件**：`api-server/internal/assistant/tool_decision_engine.go`

- `newToolPlanStep` 调用 `previousStepCondition(tool, argSources)`，对含必填 `previous_step` 参数的步骤写入 `Condition`（如 `"requires previous step to produce: scan_id"`），条件图显式进入 `ToolExecutionPlan`。
- 不移除配套 step（baseline 测试要求 `Operation.Get` 为第 3 步），仅由无条件改为条件。

### C. 生产者引用补全
**文件**：`api-server/internal/assistant/tools/asset_tools.go`

- `Asset.Collection.Trigger` 的 `ResultContract` 增加 `OperationRefFields: ["task_id"]`，使其 `task_id` 可被提取并绑定到下游 `Asset.Collection.Get`。

### D. 启动 capability 唯一性强校验
**文件**：`api-server/internal/assistant/tool_registry.go`、`api-server/cmd/main.go`

- `ValidateCapabilityUniqueness()` 使用 `BuildToolUseContract(tool).Capability` 解析后能力（含合成），断言无重复（大小写不敏感），冲突返回明确错误。`cmd/main.go` 在 `ValidateModelFacingEnglish()` 后调用，失败 Fatal。

## 新增测试

| 文件 | 测试 | 覆盖 |
|------|------|------|
| `adapter_tool_gateway_binding_test.go` | `TestAssistantToolGatewayBindsPreviousStepArgFromPriorOutcome` | 模型值被生产者值覆盖 |
| `adapter_tool_gateway_binding_test.go` | `TestAssistantToolGatewaySkipsStepWhenPreviousStepArgUnresolvable` | 必填 previous_step 无法解析时 skip |
| `adapter_tool_gateway_binding_test.go` | `TestAssistantToolGatewayDoesNotSkipWhenPreviousStepArgOptional` | 非必填 previous_step 不触发 skip |
| `tool_registry_test.go` | `TestValidateCapabilityUniquenessAcceptsDistinctCapabilities` | 不同 capability 通过 |
| `tool_registry_test.go` | `TestValidateCapabilityUniquenessRejectsExplicitDuplicate` | 显式重复被拦截 |
| `tool_registry_test.go` | `TestValidateCapabilityUniquenessRejectsSyntheticDuplicate` | 合成重复被拦截 |
| `tool_outcome_test.go` | `TestNormalizeToolOutcomeExtractsSynchronousProducerRef` | 同步生产者引用提取 |
| `tools/asset_tools_ref_test.go` | `TestAssetCollectionTriggerDeclaresOperationRefFields` | 声明 OperationRefFields |

## 验证

- `go build ./...` ✓
- `make build` ✓
- `go test ./internal/assistant/...` — 全部通过（含 7 个新测试）
- `docker compose build api-server` + `docker compose up -d api-server` — 容器重建，healthy
- 启动日志：`assistant tool capability uniqueness validated (total: 72)` ✓

## 不纳入（后续）

- `Host.Resolve` → `host_ids` 数组型发现绑定（需 Facts 数组聚合，属独立增强）。
- 发现型配套的更细粒度编排（已有 `validateToolPrerequisites` + `PrerequisiteCapabilityEmptyResult` 兜底）。
