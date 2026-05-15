# AI 分析结论判定修复设计文档

**版本**: 5.7
**状态**: 设计方案
**适用范围**: `ai_analysis_handler.go` 结论解析逻辑，AI 分析执行结果的 verdict 字段生成。

## 1. 背景与问题

### 1.1 现象

AI 分析完成后，执行结果页面显示：

- **结论**: "未知"
- **处置建议**: "建议人工复核分析结果，结合上下文信息进行判断"

实际上 LLM 已正确输出结构化分析结论（包含 `conclusions[].action` 字段），但后端未能正确解析。

### 1.2 影响范围

所有 AI 分析任务完成后均出现此问题，导致前端无法展示有效结论，用户体验严重下降。

## 2. 根因分析

### 2.1 代码调用链

```
AI 分析完成
  → agent-runtime generateFinalAnswer() 输出 JSON
    → 含 attack_graph + conclusions[].action
  → api-server 接收 finalAnswer 字符串
    → parseConclusionFromAnswer(finalAnswer) (ai_analysis_handler.go:1795)
      → verdictMap 英文关键词匹配
      → 匹配失败 → verdict = "unknown"
  → 前端显示 "未知"
```

### 2.2 断裂点详解

**问题 1：`parseConclusionFromAnswer()` 使用英文关键词匹配**

`ai_analysis_handler.go:1804-1810` 定义的 `verdictMap`：

```go
verdictMap := map[string]string{
    "Benign":         "benign",
    "False Positive": "benign",
    "Malicious":      "malicious",
    "Threat":         "malicious",
    "Suspicious":     "suspicious",
}
```

此映射期望 LLM 在 `finalAnswer` 中直接输出英文关键词（如 "Malicious"、"Benign"），但实际 LLM 输出的是结构化 JSON。

**问题 2：`summarizePromptTemplate` 指示 LLM 输出结构化 JSON**

`prompt_provider.go:157-177` 中的 summarize prompt 指示 LLM 输出：

```json
{
  "attack_graph": { ... },
  "conclusions": [
    {
      "alert_id": "...",
      "action": "mark_false_positive | confirm_threat | generate_rule",
      "summary": "..."
    }
  ]
}
```

LLM 输出的 `action` 值为 `mark_false_positive`、`confirm_threat`、`generate_rule`，与 `verdictMap` 的英文关键词完全不匹配。

**问题 3：`extractFinalAnswerResult()` 已正确解析但未被使用**

`ai_analysis_handler.go:417` 的 `extractFinalAnswerResult()` 能正确解析 JSON 中的 `conclusions` 字段，返回 `*finalAnswerResult`。该函数已被 `isAllFalsePositive()`（line 436）和 `buildAlertWritebacks()`（line 449）成功使用，但 `parseConclusionFromAnswer()` 完全未调用它。

**问题 4：前端 verdict 显示链正确**

前端 `sessionStatus.ts` 中的 `getVerdictText()`、`getRemediationSuggestion()`、`getVerdictType()` 已正确处理所有 verdict 值（benign、malicious、suspicious、unknown），无需修改。

### 2.3 根因总结

| 层级 | 组件 | 问题 |
|------|------|------|
| Prompt | `summarizePromptTemplate` | 指示 LLM 输出 `action` 字段（mark_false_positive 等） |
| 解析 | `parseConclusionFromAnswer()` | 使用英文关键词匹配，无法识别 action 值 |
| 桥接 | `extractFinalAnswerResult()` | 已存在但未被 `parseConclusionFromAnswer()` 调用 |
| 显示 | 前端 `getVerdictText()` | 正确，无需修改 |

## 3. 设计方案

### 3.1 总体思路

`parseConclusionFromAnswer()` 增加 JSON 结构化解析优先路径，通过 `extractFinalAnswerResult()` 提取 `conclusions`，将 `action` 映射为 verdict。保留现有关键词匹配作为降级兜底。

### 3.2 Action → Verdict 映射表

| LLM action | verdict | 前端显示 | 严重等级 |
|------------|---------|---------|---------|
| `confirm_threat` | `malicious` | 恶意 | 3（最高） |
| `generate_rule` | `suspicious` | 可疑 | 2 |
| `mark_false_positive` | `benign` | 良性/误报 | 1（最低） |

**多结论取最严重原则**：当 `conclusions` 包含多条记录时，取严重等级最高的 verdict。例如同时存在 `mark_false_positive` 和 `confirm_threat`，最终 verdict 为 `malicious`。

### 3.3 新增函数：`buildVerdictFromConclusions()`

**函数签名**：

```go
func buildVerdictFromConclusions(result *finalAnswerResult) map[string]interface{}
```

**输入**：`extractFinalAnswerResult()` 返回的 `*finalAnswerResult`

**处理逻辑**：

```
1. 遍历 result.Conclusions
2. 对每个 conclusion.action 进行映射：
   - mark_false_positive → benign (severity=1)
   - confirm_threat      → malicious (severity=3)
   - generate_rule       → suspicious (severity=2)
   - 其他值              → 跳过
3. 跟踪最高严重等级对应的 verdict
4. 收集所有 conclusion.summary 作为综合摘要
5. 返回 map[string]interface{}{
     "verdict":   最严重verdict,
     "summary":   合并摘要文本,
     "reasoning": 原始conclusions JSON序列化,
   }
```

**空结论处理**：若 `result.Conclusions` 为空或所有 action 均无法映射，返回 nil，由调用方走降级路径。

### 3.4 修改函数：`parseConclusionFromAnswer()`

**修改后的处理步骤**：

```
Step 1: 空值检查（不变）
  - finalAnswer 为空 → 返回 verdict=unknown, summary="未生成结论"

Step 2: JSON 结构化解析（新增）
  - 调用 extractFinalAnswerResult(finalAnswer)
  - 成功 → 调用 buildVerdictFromConclusions(result)
  - 返回有效结果 → 直接返回

Step 3: 中英文关键词匹配（增强）
  - 扩展 verdictMap，增加中文关键词：
    "良性": benign, "误报": benign, "误告": benign,
    "恶意": malicious, "威胁": malicious, "危险": malicious,
    "可疑": suspicious, "异常": suspicious, "疑似": suspicious
  - 保留原有英文关键词

Step 4: 内容摘要兜底（新增）
  - verdict 仍为 unknown 时
  - 将 finalAnswer 截断为前 200 字符作为 summary
  - 返回 verdict=unknown, summary=截断文本, reasoning=原文
```

### 3.5 中英文关键词扩展表

| 关键词 | verdict | 来源 |
|--------|---------|------|
| Benign | benign | 原有 |
| False Positive | benign | 原有 |
| 良性 | benign | 新增 |
| 误报 | benign | 新增 |
| 误告 | benign | 新增 |
| Malicious | malicious | 原有 |
| Threat | malicious | 原有 |
| 恶意 | malicious | 新增 |
| 威胁 | malicious | 新增 |
| 危险 | malicious | 新增 |
| Suspicious | suspicious | 原有 |
| 可疑 | suspicious | 新增 |
| 异常 | suspicious | 新增 |
| 疑似 | suspicious | 新增 |

## 4. 涉及文件

| 文件 | 修改内容 |
|------|---------|
| `api-server/internal/api/handler/ai_analysis_handler.go` | 新增 `buildVerdictFromConclusions()`，修改 `parseConclusionFromAnswer()` |
| `api-server/internal/api/handler/ai_analysis_handler_test.go` | 新增 8 个测试用例 |

**前端无需修改**：`frontend/src/utils/sessionStatus.ts` 已正确处理所有 verdict 值。

## 5. 测试方案

### 5.1 新增测试用例

| 编号 | 测试场景 | 输入 | 预期 verdict | 预期 summary |
|------|---------|------|-------------|-------------|
| T1 | 结构化 JSON + confirm_threat | `{"conclusions":[{"action":"confirm_threat","summary":"确认威胁"}]}` | malicious | 确认威胁 |
| T2 | 结构化 JSON + mark_false_positive | `{"conclusions":[{"action":"mark_false_positive","summary":"误报"}]}` | benign | 误报 |
| T3 | 结构化 JSON + generate_rule | `{"conclusions":[{"action":"generate_rule","summary":"需生成规则"}]}` | suspicious | 需生成规则 |
| T4 | 多结论取最严重 | `conclusions: [mark_false_positive, confirm_threat]` | malicious | 合并摘要 |
| T5 | 中文关键词 "恶意" | `finalAnswer = "该行为属于恶意攻击"` | malicious | 恶意 |
| T6 | 中文关键词 "可疑" | `finalAnswer = "行为可疑，建议进一步观察"` | suspicious | 可疑 |
| T7 | 空 finalAnswer | `""` | unknown | 未生成结论 |
| T8 | 无法识别内容 | `finalAnswer = "今天天气不错"` | unknown | 截断文本 |

### 5.2 回归验证

- 确认 `isAllFalsePositive()` 行为不变（独立使用 `extractFinalAnswerResult()`）
- 确认 `buildAlertWritebacks()` 行为不变
- 确认前端 verdict 显示、颜色、建议文案均正确

## 6. 风险与注意事项

1. **向后兼容**：修改后的 `parseConclusionFromAnswer()` 保留完整的降级路径，当 JSON 解析失败时回退到关键词匹配，不影响已有功能。
2. **截断长度**：Step 4 中 finalAnswer 截断为 200 字符，避免前端展示过长文本。该值可按需调整。
3. **多结论策略**：当前设计取"最严重"结论。若未来业务需要展示每条 alert 的独立结论，需扩展 API 返回值结构。
4. **LLM 输出格式变更**：若未来 `summarizePromptTemplate` 修改 action 值命名，需同步更新 `buildVerdictFromConclusions()` 中的映射表。
