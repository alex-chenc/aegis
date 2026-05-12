# Code Review: AI 分析 Bug 修复

**Reviewed**: 2026-05-12
**Branch**: develop
**Decision**: APPROVE

## Summary

修复了 AI 分析功能因消息格式不完整导致 GLM API 返回 400 错误的问题。修改范围小且精确，测试覆盖充分。

## Findings

### CRITICAL
None

### HIGH
None

### MEDIUM

1. **重复的系统消息内容** — `prompt_provider.go:66-71`
   - `SystemPrompt` 字段和 `Messages[0].Content` 包含相同的 `planPromptTemplate` 内容
   - 如果未来修改模板，需要同步修改两处
   - 建议：提取为变量引用，或在 adapter 层自动从 SystemPrompt 构建 system message
   - 当前可接受，因为这是最小修复方案

### LOW

1. **硬编码的中文提示** — `prompt_provider.go:70`
   - `"请根据以上指令和告警上下文，制定详细的安全分析计划。"` 是硬编码的用户消息
   - 可以考虑提取为常量或配置
   - 当前可接受，因为其他提示词模板也是硬编码的

## Validation Results

| Check | Result |
|---|---|
| go vet | Pass |
| go test | Pass (9/9) |
| go build | Pass |

## Files Reviewed

| File | Change Type | Description |
|---|---|---|
| `api-server/internal/llm/adapters/prompt_provider.go` | Modified | buildPlanPrompt 返回包含 system+user 消息的 PromptBundle |
| `api-server/internal/llm/adapters/llm_client_adapter.go` | Modified | injectAlertContext 增加 nil 检查 |
| `api-server/internal/llm/adapters/prompt_provider_test.go` | Added | 5 个测试用例覆盖消息格式 |
| `api-server/internal/llm/adapters/llm_client_adapter_test.go` | Added | 4 个测试用例覆盖上下文注入 |
| `docs/aegis_system_design_v5.7/ai_analysis_bug_fix_design.md` | Added | Bug 修复设计文档 |
