# AI 分析攻击溯源图降级显示设计文档

**版本**: 5.7
**状态**: 已实现
**适用范围**: `ai_analysis_handler.go` 执行结果 API，`AIAnalysis.vue` 历史会话加载逻辑。

## 1. 背景与问题

### 1.1 现象

AI 分析会话 `06b89c4b-4c1e-4a74-bf57-b6fe3e18e57e` 的告警被判定为"恶意"（malicious），但前端未显示攻击溯源图（AttackGraph 组件）。

### 1.2 影响范围

当 LLM 的 final answer 未输出结构化 `attack_graph` JSON（例如仅输出含"恶意"关键词的自由文本）时，系统能通过关键词降级正确判定 verdict 为 "malicious"，但前端无法渲染攻击溯源图。

### 1.3 设计文档参考

- `ai_analysis_verdict_fix_design.md`：verdict 解析降级链（JSON 结构化 → 关键词匹配 → 内容截断）
- `agent_optimization_design.md`：TaskResult 持久化，`final_answer` 字段定义
- `task_execution_result_ui_design.md`：ExecutionResult API 接口定义
- `backend_detailed_design_v5.6.md`：攻击溯源图 JSON 结构规范

## 2. 根因分析

### 2.1 数据流断裂点

```
LLM 输出 final_answer（可能不含结构化 attack_graph JSON）
  → extractFinalAnswerResult() 解析失败（无 attack_graph + conclusions JSON）
  → persistAnalysisOutcome() 降级到 parseConclusionFromAnswer()
    → 关键词匹配成功 → verdict = "malicious" ✓
  → buildExecutionResultResponse()
    → 调用 parseConclusionFromAnswer() → 返回 {verdict, summary, reasoning}
    → 不包含 final_answer 原始文本 ✗
  → 前端 loadSession()
    → applyStructuredFinalAnswer() → extractAttackGraphFinalAnswer() → null ✗
    → loadExecutionResultForSession() → 获取 execution result
      → conclusion.verdict = "malicious" ✓
      → 无 attack_graph 数据 ✗
    → attackGraph.value = null → AttackGraph 组件隐藏 ✗
```

### 2.2 根因总结

| 层级 | 组件 | 问题 |
|------|------|------|
| 后端 API | `buildExecutionResultResponse()` | 未返回 `final_answer` 原始文本 |
| 前端解析 | `loadSession()` | 未尝试从 execution result 的 `final_answer` 提取 attack_graph |
| 前端解析 | 流式 `done` 事件处理 | 未在 execution result 加载后尝试提取 attack_graph |

### 2.3 关键发现

`agent_executions.final_answer` 已持久化存储了 LLM 的完整输出文本，但 `GET /api/v1/detection/alerts/ai-analysis/:session_id/execution-result` API 的响应中**未包含此字段**。前端因此无法在历史会话加载时从 `final_answer` 中提取 attack_graph。

## 3. 设计方案

### 3.1 总体思路

1. **后端**：在执行结果 API 响应中新增 `final_answer` 字段
2. **前端**：当 verdict 为 malicious/suspicious 但无 attack_graph 时，从 `final_answer` 中提取

### 3.2 后端修改：`buildExecutionResultResponse()`

**文件**: `api-server/internal/api/handler/ai_analysis_handler.go`

在返回的 map 中新增 `final_answer` 字段：

```go
return map[string]interface{}{
    // ... 现有字段 ...
    "conclusion":            conclusion,
    "final_answer":          exec.FinalAnswer,  // 新增
    "context_budget":        contextBudget,
    // ...
}
```

**语义说明**：
- `final_answer` 为 LLM 输出的完整文本（可能包含结构化 JSON 或自由文本）
- 当 `agent_executions` 记录不存在时，该字段为空字符串
- 该字段仅用于前端 attack_graph 提取，不影响 verdict 判定逻辑

### 3.3 前端修改：`AIAnalysis.vue`

#### 3.3.1 历史会话加载（`loadSession()`）

在 `loadExecutionResultForSession()` 完成后，新增降级提取逻辑：

```typescript
const execResult = await loadExecutionResultForSession(session.session_id, true)
if (execResult) {
  // ... 现有状态更新逻辑 ...

  // 新增：当 verdict 为 malicious/suspicious 但无 attack_graph 时，从 final_answer 提取
  if (!attackGraph.value && execResult.final_answer && execResult.conclusion?.verdict &&
      (execResult.conclusion.verdict === 'malicious' || execResult.conclusion.verdict === 'suspicious')) {
    const extracted = extractAttackGraphFinalAnswer(execResult.final_answer)
    if (extracted) {
      attackGraph.value = extracted.graph
      upsertFinalAssistantMessage(buildAttackGraphDisplayText(extracted))
    }
  }
}
```

#### 3.3.2 流式分析完成（`done` 事件处理）

在 `loadExecutionResultForSession()` 的 `.then()` 回调中新增相同逻辑：

```typescript
case 'done':
  // ... 现有 cleanup 逻辑 ...
  if (sessionId.value) {
    void loadExecutionResultForSession(sessionId.value, true).then(result => {
      if (result && !attackGraph.value && result.final_answer &&
          result.conclusion?.verdict &&
          (result.conclusion.verdict === 'malicious' || result.conclusion.verdict === 'suspicious')) {
        const extracted = extractAttackGraphFinalAnswer(result.final_answer)
        if (extracted) {
          attackGraph.value = extracted.graph
          upsertFinalAssistantMessage(buildAttackGraphDisplayText(extracted))
        }
      }
    })
  }
```

### 3.4 前端接口修改：`aiAnalysis.ts`

在 `ExecutionResult` 接口中新增 `final_answer` 字段：

```typescript
export interface ExecutionResult {
  // ... 现有字段 ...
  final_answer?: string
}
```

## 4. 涉及文件

| 文件 | 修改内容 |
|------|---------|
| `api-server/internal/api/handler/ai_analysis_handler.go` | `buildExecutionResultResponse()` 新增 `final_answer` 字段 |
| `frontend/src/api/aiAnalysis.ts` | `ExecutionResult` 接口新增 `final_answer` 字段 |
| `frontend/src/views/detection/AIAnalysis.vue` | `loadSession()` 和 `done` 事件处理中新增 attack_graph 降级提取逻辑 |

## 5. 测试方案

### 5.1 后端测试

| 编号 | 测试场景 | 输入 | 预期结果 |
|------|---------|------|---------|
| T1 | 执行结果包含 final_answer | 存在 agent_execution 记录 | API 响应包含 `final_answer` 字段 |
| T2 | 执行结果不存在 | 不存在 agent_execution 记录 | API 返回 404 |
| T3 | final_answer 含结构化 JSON | `final_answer` 包含 `attack_graph` JSON | `final_answer` 原样返回 |

### 5.2 前端测试

| 编号 | 测试场景 | 输入 | 预期结果 |
|------|---------|------|---------|
| T4 | malicious verdict + 无 attack_graph + 有 final_answer | verdict=malicious, final_answer 含 attack_graph JSON | attackGraph.value 被设置 |
| T5 | malicious verdict + 无 attack_graph + final_answer 无 JSON | verdict=malicious, final_answer 为纯文本 | attackGraph.value 保持 null |
| T6 | benign verdict + 有 final_answer | verdict=benign | 不尝试提取 attack_graph |

### 5.3 集成测试

1. 使用 `aegis-build-test` 构建并部署
2. 使用 curl 测试执行结果 API：
   ```bash
   TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.data.token')
   curl -s http://localhost:8082/api/v1/detection/alerts/ai-analysis/<session_id>/execution-result \
     -H "Authorization: Bearer $TOKEN" | jq '.data.final_answer'
   ```
3. 在前端加载历史会话，验证恶意 verdict 的 attack_graph 显示

## 6. 风险与注意事项

1. **向后兼容**：`final_answer` 为新增字段，不影响现有 API 消费者
2. **数据大小**：`final_answer` 可能较大（含完整 LLM 输出），前端仅在需要时解析
3. **安全考虑**：`final_answer` 包含 LLM 原始输出，前端渲染时需注意 XSS 防护（现有 `buildAttackGraphDisplayText()` 已处理）
4. **降级链完整性**：此修复完善了 attack_graph 的降级链：流式解析 → 历史会话 JSON 提取 → execution result final_answer 提取

## 7. 实现记录

### 7.1 实现日期

2026-05-19

### 7.2 修改文件

| 文件 | 修改内容 |
|------|---------|
| `api-server/internal/api/handler/ai_analysis_handler.go` | `buildExecutionResultResponse()` 新增 `"final_answer": exec.FinalAnswer` 字段（1 行） |
| `api-server/internal/api/handler/ai_analysis_handler_test.go` | 新增 3 个测试用例：`TestBuildExecutionResultResponse_IncludesFinalAnswer`、`TestBuildExecutionResultResponse_FinalAnswerWithStructuredJSON`、`TestBuildExecutionResultResponse_EmptyFinalAnswer` |
| `frontend/src/api/aiAnalysis.ts` | `ExecutionResult` 接口新增 `final_answer?: string` 字段 |
| `frontend/src/views/detection/AIAnalysis.vue` | `loadSession()` 和流式 `done` 事件处理中新增 attack_graph 降级提取逻辑 |

### 7.3 测试结果

| 测试 | 结果 |
|------|------|
| Go handler 测试（19 个） | 全部通过 |
| Frontend attackGraph 测试（5 个） | 全部通过 |
| Frontend aiAnalysis 测试（5 个） | 全部通过 |
| API curl 测试 | `final_answer` 字段正确返回（1656 字符，verdict=malicious） |

### 7.4 验证的会话

- 会话 `06b89c4b-4c1e-4a74-bf57-b6fe3e18e57e`：`final_answer` 长度 1656，verdict=malicious
- 会话 `704cd6a8-d667-4ee4-bbc8-36d2a88ef615`：`final_answer` 长度 3221，verdict=malicious

## 8. Phase 2：Agent-Runtime 根因修复

### 8.1 问题

Phase 1 添加了 `final_answer` 字段和前端降级提取逻辑，但攻击溯源图仍未显示。根因在 `agent-runtime/runtime.go` 的 `parseFinalAnswer()` 函数：

当 LLM 输出结构化 JSON（如 `{"attack_graph":{...},"conclusions":[...]}`）时，`parseFinalAnswer()` 只提取 `"final_answer"` 字段。由于该字段不存在，返回 `""`，导致使用 `buildFinalAnswer()` 的纯文本降级内容。

### 8.2 修改文件

| 文件 | 修改内容 |
|------|---------|
| `/code/agent-runtime/runtime.go` | `parseFinalAnswer()`：当 JSON 中无 `final_answer` 字段时返回完整 JSON 字符串（而非空字符串） |
| `/code/agent-runtime/runtime_test.go` | 新增 `TestParseFinalAnswer`：4 个测试用例覆盖标准 JSON、纯文本、结构化 JSON、空 final_answer |

### 8.3 修复逻辑

```go
if raw.FinalAnswer == "" {
    // LLM 返回了结构化 JSON（如 attack_graph + conclusions）
    // 但没有 final_answer 字段。保留完整 JSON 以便调用方提取结构化数据。
    return jsonStr
}
```

### 8.4 测试结果

| 测试 | 结果 |
|------|------|
| `TestParseFinalAnswer/standard_final_answer_JSON` | 通过 |
| `TestParseFinalAnswer/plain_text` | 通过 |
| `TestParseFinalAnswer/structured_JSON_without_final_answer_field` | 通过 |
| `TestParseFinalAnswer/empty_final_answer_falls_through_to_raw_JSON` | 通过 |
| Go handler 测试（8 个） | 全部通过 |

### 8.5 部署信息

- agent-runtime commit: `d3f8855` (pushed to `github.com/alex-chenc/agent-runtime`)
- aegis go.mod 更新: `v0.0.0-20260519071420-d3f8855f0609`
- api-server Docker 镜像已重建并部署

### 8.6 注意事项

- 已有会话（修复前创建）的 `final_answer` 仍为纯文本，攻击溯源图不会自动显示
- 新会话（修复后创建）的 `final_answer` 将包含完整的结构化 JSON，前端可正确提取 attack_graph
- 向后兼容：当 LLM 输出 `{"final_answer":"text"}` 时行为不变
