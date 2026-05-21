# AI 分析结论存储与溯源图显示修复设计文档

**版本**: 5.7
**状态**: 已实现
**适用范围**: `ai_analysis_handler.go` 结论持久化逻辑，前端溯源图提取逻辑。

## 1. 背景与问题

### 1.1 现象

AI 分析完成后，执行结果页面显示：

- **结论**: "良性/误报"（实际应为"恶意"）
- **溯源图**: 未显示（实际 LLM 已输出完整的 attack_graph JSON）

用户明确看到 AI 分析内容描述了真实威胁（如反弹 Shell 攻击），但结论却错误地显示为良性/误报。

### 1.2 与前次修复的关系

前次修复（`ai_analysis_verdict_fix_design.md`）解决了 `parseConclusionFromAnswer()` 中的关键词匹配问题，新增了 `buildVerdictFromConclusions()` 函数。该修复已正确落地，`parseConclusionFromAnswer()` 现在能正确解析结构化 JSON。

**但本次 bug 的根因不在 `parseConclusionFromAnswer()`，而在 `persistAnalysisOutcome()`。**

## 2. 根因分析

### 2.1 Bug 1：结论存储缺少 verdict 字段

**代码调用链**：

```
AI 分析完成
  → persistAnalysisOutcome(session, finalContent)
    → extractFinalAnswerResult(finalContent)  // 成功解析，返回 *finalAnswerResult
    → json.Marshal(result)                     // result 只有 AttackGraph + Conclusions
    → sessionRepo.UpdateConclusion(sessionID, conclusionJSON)
    → 数据库存储: {conclusions: [...], attack_graph: {...}}  ← 缺少 verdict 字段！
```

**断裂点**：`persistAnalysisOutcome()` 在 `extractFinalAnswerResult()` 成功后，直接将 `*finalAnswerResult` 序列化存储。`finalAnswerResult` 结构体只包含 `AttackGraph` 和 `Conclusions`，不包含 `verdict`、`summary`、`reasoning` 字段。

```go
// ai_analysis_handler.go:80-83
type finalAnswerResult struct {
    AttackGraph map[string]interface{} `json:"attack_graph"`
    Conclusions []AlertConclusion      `json:"conclusions"`
}
```

而 `buildVerdictFromConclusions()` 返回的是包含 `verdict`、`summary`、`reasoning` 的 map，但 `persistAnalysisOutcome()` **从未调用它**。

**前端影响**：

```
前端加载执行结果
  → getExecutionResult(sessionId)
    → buildExecutionResultResponse(exec, steps, sessionConclusion)
      → sessionConclusion = {conclusions: [...], attack_graph: {...}}  ← 无 verdict
      → 直接返回给前端
  → normalizeExecutionResult(result)
    → inferConclusion(raw.conclusion, ...)
      → raw.verdict = undefined
      → raw.summary = undefined
      → raw.reasoning = undefined
      → 回退到 step 结果或 "unknown"
```

### 2.2 Bug 2：前端溯源图提取受 verdict 条件限制

**代码位置**：`AIAnalysis.vue:1072-1080`

```typescript
// 当 verdict 为 malicious/suspicious 但无 attack_graph 时，从 final_answer 降级提取
if (!attackGraph.value && execResult.final_answer && execResult.conclusion?.verdict &&
    (execResult.conclusion.verdict === 'malicious' || execResult.conclusion.verdict === 'suspicious')) {
  const extracted = extractAttackGraphFinalAnswer(execResult.final_answer)
  ...
}
```

由于 Bug 1 导致 verdict 为 `unknown`，此条件不满足，溯源图提取被跳过。即使 `execResult.final_answer` 中包含完整的 attack_graph JSON。

### 2.3 数据库实际存储验证

最新会话（session_id: 736a9ed2-12d6-4d29-bf7e-e9673e1134d3）的 conclusion 数据：

```json
{
  "conclusions": [
    {
      "action": "confirm_threat",
      "summary": "经多步骤深度研判，该告警确认为真实威胁(TP)...",
      "alert_id": "ALT-a70fe606"
    }
  ],
  "attack_graph": {
    "nodes": [...],
    "edges": [...],
    "timeline": [...],
    "threat_level": "high",
    "recommendations": [...]
  }
}
```

**缺少**: `verdict`、`summary`、`reasoning` 字段。

### 2.4 根因总结

| 层级 | 组件 | 问题 |
|------|------|------|
| 持久化 | `persistAnalysisOutcome()` | 直接存储 `finalAnswerResult`，未调用 `buildVerdictFromConclusions()` |
| 前端提取 | `AIAnalysis.vue` | 溯源图提取受 `verdict === 'malicious'` 条件限制 |
| 显示 | `inferConclusion()` | 收到无 verdict 的 conclusion，回退为 unknown |

## 3. 设计方案

### 3.1 修复 `persistAnalysisOutcome()` 结论存储

**修改位置**：`ai_analysis_handler.go` 的 `persistAnalysisOutcome()` 函数（line 1137）

**修改逻辑**：

```
persistAnalysisOutcome(session, finalContent):
  result = extractFinalAnswerResult(finalContent)
  if result 成功:
    conclusionMap = buildVerdictFromConclusions(result, finalContent)  ← 新增调用
    // conclusionMap 包含 verdict, summary, reasoning
    // 将原始 conclusions 和 attack_graph 合并到 conclusionMap 中
    conclusionMap["conclusions"] = result.Conclusions
    conclusionMap["attack_graph"] = result.AttackGraph
    sessionRepo.UpdateConclusion(sessionID, conclusionMap)
    buildAlertWritebacks(...)
  else:
    // 降级路径不变
    conclusionMap = parseConclusionFromAnswer(finalContent)
    sessionRepo.UpdateConclusion(sessionID, conclusionMap)
```

**存储格式对比**：

修改前：
```json
{
  "conclusions": [{"action": "confirm_threat", "summary": "...", "alert_id": "..."}],
  "attack_graph": {"nodes": [...], "edges": [...], ...}
}
```

修改后：
```json
{
  "verdict": "malicious",
  "summary": "确认为真实威胁...",
  "reasoning": "<原始 finalAnswer 文本>",
  "conclusions": [{"action": "confirm_threat", "summary": "...", "alert_id": "..."}],
  "attack_graph": {"nodes": [...], "edges": [...], ...}
}
```

### 3.2 修复前端溯源图提取条件

**修改位置**：`AIAnalysis.vue` 的 `loadSession()` 和 `done` 事件处理

**修改逻辑**：放宽溯源图提取条件，当 execution result 中存在 `final_answer` 时尝试提取溯源图，不再严格限制 verdict 为 `malicious` 或 `suspicious`。

同时增加从 `execResult.conclusion` 中直接提取 `attack_graph` 的逻辑（后端已存储）：

```typescript
// 优先从 conclusion.attack_graph 直接获取
if (!attackGraph.value && execResult.conclusion?.attack_graph) {
  const graph = execResult.conclusion.attack_graph
  if (isAttackGraph(graph)) {
    attackGraph.value = graph
  }
}

// 降级：从 final_answer 文本中提取
if (!attackGraph.value && execResult.final_answer) {
  const extracted = extractAttackGraphFinalAnswer(execResult.final_answer)
  if (extracted) {
    attackGraph.value = extracted.graph
  }
}
```

### 3.3 `buildExecutionResultFromConclusion` 兼容性

此函数已有正确的 verdict 推导逻辑（从 conclusions 数组映射 action → verdict）。后端修复后，`conclusion.verdict` 字段存在，将走 `conclusion.verdict` 直接使用的路径，无需额外修改。

## 4. 涉及文件

| 文件 | 修改内容 |
|------|---------|
| `api-server/internal/api/handler/ai_analysis_handler.go` | 修改 `persistAnalysisOutcome()`，调用 `buildVerdictFromConclusions()` 并合并存储 |
| `api-server/internal/api/handler/ai_analysis_handler_test.go` | 新增 `persistAnalysisOutcome` 结论存储测试 |
| `frontend/src/views/detection/AIAnalysis.vue` | 放宽溯源图提取条件，增加从 conclusion 直接获取 attack_graph |

## 5. 测试方案

### 5.1 后端测试用例

| 编号 | 测试场景 | 输入 | 预期存储结论 |
|------|---------|------|-------------|
| T1 | confirm_threat 结论存储 | finalContent 含 `conclusions[{action: confirm_threat}]` | verdict=malicious, 含 attack_graph |
| T2 | mark_false_positive 结论存储 | finalContent 含 `conclusions[{action: mark_false_positive}]` | verdict=benign |
| T3 | 混合结论取最严重 | conclusions 含 confirm_threat + mark_false_positive | verdict=malicious |
| T4 | JSON 解析失败降级 | finalContent 为纯文本含 "恶意" 关键词 | verdict=malicious (关键词匹配) |
| T5 | 空内容 | finalContent = "" | verdict=unknown |

### 5.2 前端测试用例

| 编号 | 测试场景 | 预期行为 |
|------|---------|---------|
| T6 | conclusion 含 attack_graph + verdict=malicious | 显示恶意结论 + 溯源图 |
| T7 | conclusion 含 attack_graph + verdict=unknown | 仍尝试显示溯源图 |
| T8 | final_answer 含 attack_graph JSON | 从文本中提取并显示溯源图 |

### 5.3 集成验证

1. 启动服务，触发一次 AI 分析
2. 检查数据库 conclusion 是否包含 verdict 字段
3. 前端页面是否正确显示结论和溯源图
4. 历史会话加载是否正确显示

## 6. 风险与注意事项

1. **向后兼容**：已有会话的 conclusion 不含 verdict 字段，前端 `inferConclusion()` 仍能通过 step 结果推导。新会话将存储完整结论。
2. **存储大小**：`reasoning` 字段存储完整的 finalAnswer 文本，可能较大。可考虑截断。但当前 `parseConclusionFromAnswer()` 的降级路径也存储完整文本，保持一致。
3. **前端条件放宽**：移除 verdict 条件限制后，所有含 final_answer 的执行结果都会尝试提取溯源图。`extractAttackGraphFinalAnswer()` 已有严格的 JSON 验证（`isAttackGraph`），不会误提取。对于明确的良性结论（`verdict === 'benign'`），跳过文本提取以避免误匹配。

## 7. 实现记录

### 7.1 实际修改文件

| 文件 | 修改内容 |
|------|---------|
| `api-server/internal/api/handler/ai_analysis_handler.go` | 1. `persistAnalysisOutcome()`: 调用 `buildVerdictFromConclusions()` 并合并存储<br>2. `buildExecutionResultResponse()`: 向后兼容逻辑，从 conclusions 推导 verdict<br>3. 提取共享函数 `deriveVerdictFromActions()` 消除重复代码 |
| `api-server/internal/api/handler/ai_analysis_handler_test.go` | 新增 8 个测试用例 |
| `frontend/src/utils/attackGraph.ts` | 导出 `isAttackGraph` 函数 |
| `frontend/src/views/detection/AIAnalysis.vue` | 1. 新增 `isAttackGraph` 导入<br>2. 优先从 `conclusion.attack_graph` 获取溯源图<br>3. 降级从 `final_answer` 提取时跳过良性结论 |

### 7.2 测试结果

- 后端 36 个 AI 分析相关测试全部通过
- 前端 vite build 成功
- 已有会话（旧格式）正确返回 `verdict: malicious`（通过向后兼容逻辑推导）
- API 返回包含完整 `attack_graph`（5 nodes, 5 edges）

### 7.3 Code Review 反馈

| 严重度 | 问题 | 处理 |
|--------|------|------|
| MEDIUM | 判定逻辑重复 | 已提取 `deriveVerdictFromActions()` 共享函数 |
| MEDIUM | 前端移除 verdict 守卫 | 已添加 `verdict !== 'benign'` 软守卫 |
| LOW | final_answer 暴露 | 设计决策，非 bug |
| LOW | isAttackGraph 较宽松 | 仅作预检查，可接受 |
