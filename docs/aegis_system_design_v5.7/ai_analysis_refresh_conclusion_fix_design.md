# AI 分析刷新后结论丢失修复设计文档

**版本**: 5.7
**状态**: 已实现
**适用范围**: `ai_analysis_handler.go` execution-result API 结论优先级；`AIAnalysis.vue` localStorage 恢复逻辑

## 1. 背景与问题

### 1.1 现象

AI 分析会话完成后，页面显示正确的分析结论（如"恶意"verdict + 处置建议）。刷新页面后，底部 UI 从"建议"变为"错误"：
- verdict 变为 "未知"
- summary 变为 "执行完成，但存在采集或检查错误，需要结合错误信息复核"
- 原有的处置建议（【标准化处置建议】）消失，取而代之的是执行错误列表

### 1.2 影响范围

所有已完成的 AI 分析会话在刷新页面后均出现此问题。

## 2. 根因分析

### 2.1 数据流概览

```
会话完成时:
  persistAnalysisOutcome() → 结论存入 ai_analysis_session.conclusion (JSONB)
  persistAgentResult()     → 执行记录存入 agent_executions (含 FinalAnswer)

刷新页面时:
  loadConversation() 从 localStorage 恢复消息
    → applyStructuredFinalAnswer() → upsertFinalAssistantMessage() → executionResult = null
    → applyParsedExecutionResultFromContent() → 从文本正则解析 → 生成劣质 ExecutionResult
  loadExecutionResultForSession(savedId, false) → API 返回权威结果但不附加到消息
```

### 2.2 两个独立问题

**问题 1: 后端 execution-result API 重新推导结论**

`GetExecutionResult` handler 调用 `buildExecutionResultResponse`，其中通过 `parseConclusionFromAnswer(exec.FinalAnswer)` 从原始文本重新推导结论，而非使用 `ai_analysis_session.conclusion` 中已持久化的权威结论。

当 FinalAnswer 包含复杂格式时，`parseConclusionFromAnswer` 可能无法正确解析，返回 `verdict: "unknown"`。

**问题 2: 前端 loadConversation 恢复流程缺陷**

`loadConversation()` 中的 `applyParsedExecutionResultFromContent()` 从纯文本正则解析生成劣质 ExecutionResult，覆盖了 API 返回的权威结果。

`loadExecutionResultForSession(savedId, false)` 的 `attachToMessage=false` 参数导致 API 结果只写入顶层 ref，不附加到消息对象。

## 3. 修复方案

### 3.1 后端修复: execution-result API 优先使用会话存储结论

**文件**: `api-server/internal/api/handler/ai_analysis_handler.go`

修改 `GetExecutionResult` handler，在构建响应前获取会话的存储结论：

```go
// 修改前
response := buildExecutionResultResponse(exec, steps)

// 修改后
var sessionConclusion model.JSONB
if h.sessionRepo != nil {
    if session, sessErr := h.sessionRepo.FindBySessionID(sessionID); sessErr == nil {
        sessionConclusion = session.Conclusion
    }
}
response := buildExecutionResultResponse(exec, steps, sessionConclusion)
```

修改 `buildExecutionResultResponse` 函数，优先使用会话结论：

```go
// 修改前
conclusion := parseConclusionFromAnswer(exec.FinalAnswer)

// 修改后
var conclusion map[string]interface{}
if sessionConclusion != nil && len(sessionConclusion) > 0 {
    conclusion = map[string]interface{}(sessionConclusion)
} else {
    conclusion = parseConclusionFromAnswer(exec.FinalAnswer)
}
```

### 3.2 前端修复: loadConversation 正确恢复 ExecutionResult

**文件**: `frontend/src/views/detection/AIAnalysis.vue`

**修改 1**: `loadConversation()` 中移除 `applyParsedExecutionResultFromContent()` 调用，避免生成劣质结果覆盖 API 权威结果。

**修改 2**: `onMounted` 中将 `loadExecutionResultForSession(savedId, false)` 改为 `loadExecutionResultForSession(savedId, true)`，确保 API 结果附加到消息。

**修改 3**: `upsertFinalAssistantMessage()` 中增加守卫，不覆盖已有有效 verdict 的 executionResult：

```typescript
// 修改前
if (append || content) {
    messages.value[index].executionResult = null
}

// 修改后
if (append || content) {
    const existing = messages.value[index].executionResult
    if (!existing || existing.conclusion?.verdict === 'unknown') {
        messages.value[index].executionResult = null
    }
}
```

## 4. 测试验证

### 4.1 后端单元测试

新增 3 个测试用例：
- `TestBuildExecutionResultResponse_SessionConclusionTakesPrecedence`: 验证会话结论优先
- `TestBuildExecutionResultResponse_NilSessionConclusionFallsBack`: 验证 nil 回退到文本解析
- `TestBuildExecutionResultResponse_EmptySessionConclusionFallsBack`: 验证空结论回退

### 4.2 集成测试

```bash
# 获取 token
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Cc&324511"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

# 测试 execution-result API 返回会话存储结论
curl -s "http://localhost:8082/api/v1/detection/alerts/ai-analysis/{session_id}/execution-result" \
  -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
data = json.load(sys.stdin)['data']
c = data['conclusion']
assert c['verdict'] == 'malicious', f'Expected malicious, got {c[\"verdict\"]}'
print('PASS: Session conclusion takes precedence')
"
```

## 5. 关联文件

| 文件 | 修改内容 |
|------|---------|
| `api-server/internal/api/handler/ai_analysis_handler.go` | `GetExecutionResult`: 获取会话结论; `buildExecutionResultResponse`: 优先使用会话结论 |
| `api-server/internal/api/handler/ai_analysis_handler_test.go` | 新增 3 个测试用例 |
| `frontend/src/views/detection/AIAnalysis.vue` | `loadConversation`: 移除劣质解析; `onMounted`: attachToMessage=true; `upsertFinalAssistantMessage`: 守卫有效结果 |
