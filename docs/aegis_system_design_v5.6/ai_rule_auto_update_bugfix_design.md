# AI Rule Auto-Update Bugfix Design V5.6

## Problem Statement

1. **AI自动更新功能失常**: 配置了最保守规则生成后，告警未减少，反而持续增加。
2. **事件持续告警**: 规则已收紧为`experimental`状态，但事件仍在源源不断地触发告警。用户困惑"实验性"状态的含义。

## Root Cause Analysis

### Root Cause 1: 广播顺序错误（Critical）

`api-server/internal/service/rule_generation_service.go:313-336` 中 `applyRuleAdjustment` 的执行顺序：

```
1. applyTightening()      -> 修改内存中的 rule.Content
2. sigmaRuleRepo.Update()  -> 写入数据库（line 330）
3. broadcastRuleUpdate()   -> 从数据库读取并广播（line 335）
```

**问题**: 虽然数据库已更新，但 `broadcastRuleUpdate` 发送 `action: "incremental"` 时，Agent端可能未正确处理增量更新。且 `broadcastRuleUpdate` 调用链中 `FindByID` 可能找到错误的记录。

### Root Cause 2: DC BlockManager 逻辑错误（Critical）

`dc/internal/block_manager/block_manager.go:34-40`:

```go
func (m *BlockManager) ShouldAutoBlock(mitreID string) bool {
    policy, ok := m.LoadPolicy(mitreID)
    if !ok {
        return false
    }
    return policy.Enabled && policy.AutoDispose  // BUG: 应该检查 AutoBlock
}
```

`ShouldAutoBlock` 检查的是 `AutoDispose` 而非 `AutoBlock`，导致：
- 当 `AutoBlock=true, AutoDispose=false` 时，DC不会自动阻断
- 当 `AutoBlock=false, AutoDispose=true` 时，DC错误地标记为自动处置

### Root Cause 3: CheckAndAutoDispose 从未被调用（Critical）

`api-server/internal/service/alert_service.go:126-150` 中定义了 `CheckAndAutoDispose()` 方法，但在 `api-server/internal/service/runtime_pipeline_service.go:102-169` 的 `onWindowFlush` 中：

```go
// 只调用了 CheckAndAutoBlock，从未调用 CheckAndAutoDispose
if err := s.alertService.CheckAndAutoBlock(createdAlert); err != nil {
    ...
}
// Missing: s.alertService.CheckAndAutoDispose(createdAlert)
```

即使配置了 `AutoDispose=true` 的策略，告警也永远不会被自动解决。

### Root Cause 4: 实验性规则24小时静默期导致更新延迟（已调整）

`api-server/internal/repository/sigma_rule_repo.go:97-108`:

```go
func (r *SigmaRuleRepository) GetActiveAndExperimental() ([]model.SigmaRule, error) {
    err := r.db.Where(
        "status = ? OR (status = ? AND activated_at IS NOT NULL AND activated_at <= ?)",
        "active",
        "experimental",
        time.Now().Add(-24*time.Hour),  // 实验性规则需等待24小时
    ).Find(&rules).Error
    return rules, err
}
```

原设计中，AI收紧的规则被设置为 `experimental` 后，全量同步需等待24小时才会包含它。这会导致在线广播和重连全量同步行为不一致。

新设计要求：`experimental` 也要下发。全量同步直接包含 `active` 和全部 `experimental` 规则，不再按24小时静默期过滤。

### Root Cause 5: promoteRuleToActive 广播内容正确性

`api-server/internal/service/rule_generation_service.go:431-465`:

`broadcastRuleUpdate` 从数据库读取完整规则内容发送，但Agent收到 `action: "update"` + `status: "active"` 时，需要确认Agent端正确替换本地规则缓存。

## Solution Design

### Fix 1: 修正 DC BlockManager.ShouldAutoBlock

修改 `dc/internal/block_manager/block_manager.go`:

```go
func (m *BlockManager) ShouldAutoBlock(mitreID string) bool {
    policy, ok := m.LoadPolicy(mitreID)
    if !ok {
        return false
    }
    return policy.Enabled && policy.AutoBlock  // 修正：检查 AutoBlock
}
```

### Fix 2: 在 RuntimePipelineService 中调用 CheckAndAutoDispose

修改 `api-server/internal/service/runtime_pipeline_service.go`，在 `onWindowFlush` 中 `CheckAndAutoBlock` 之后添加 `CheckAndAutoDispose` 调用。

### Fix 3: 修正 applyRuleAdjustment 确保广播成功

修改 `api-server/internal/service/rule_generation_service.go`，确保：
1. 数据库更新成功后再广播
2. 广播失败时记录更详细的错误日志
3. 使用 `full` 模式而非 `incremental` 确保Agent收到完整规则

### Fix 4: 实验性规则参与全量下发

修改 `GetActiveAndExperimental`，使其返回所有 `active` 和 `experimental` 规则。AI收紧后的即时广播继续保留，Agent 重连后的全量同步也能拿到刚进入实验性的规则。

## Files to Modify

| File | Change |
|------|--------|
| `dc/internal/block_manager/block_manager.go` | Fix `ShouldAutoBlock` to check `AutoBlock` |
| `api-server/internal/service/runtime_pipeline_service.go` | Add `CheckAndAutoDispose` call |
| `api-server/internal/service/rule_generation_service.go` | Fix broadcast order and mode |
| `api-server/internal/service/rule_generation_service_test.go` | Unit tests (new file) |

## Verification Plan

1. Unit tests for each fix
2. Build using `aegis-build-test` skill
3. curl API smoke tests
4. Monitor alert counts before/after fixes
