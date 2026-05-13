# AI 分析界面交互与结论优化设计文档

**版本**: 5.7
**状态**: 设计方案
**适用范围**: AI 分析对话框按钮交互、执行计划状态更新、agent-runtime 最终结论中文输出。

## 1. 背景与问题

### 问题 1：对话框按钮状态切换不正确

当前 AI 分析对话框中，当分析正在进行时（`isLoading=true`），按钮始终显示"暂停"。但用户输入文字后，期望按钮变为"发送"，发送完成后恢复为"暂停"。

### 问题 2：执行计划状态不更新

执行计划中所有步骤始终显示"待执行"状态，即使后端已通过 SSE 发送了 `step_started`、`step_completed`、`step_failed` 事件。

### 问题 3：最终结论未输出中文

`agent-runtime` 的 `generateFinalAnswer()` 使用英文系统 prompt（"Generate a concise final answer..."），绕过了 Aegis 的中文 prompt provider，导致最终结论可能输出英文。

## 2. 设计方案

### 2.1 按钮状态切换优化

**修改文件**: `frontend/src/views/detection/AIAnalysis.vue`

**当前逻辑**:

```html
<el-button v-if="isLoading" type="warning" @click="pauseAnalysis">暂停</el-button>
<el-button v-else type="primary" :disabled="!inputMessage.trim()" @click="sendMessage">发送 (Ctrl+Enter)</el-button>
```

**新逻辑**:

- 当 `isLoading=true` 且 `inputMessage` 有文字 → 显示"发送"按钮
- 当 `isLoading=true` 且 `inputMessage` 为空 → 显示"暂停"按钮
- 当 `isLoading=false` → 显示"发送"按钮（保持不变）

**交互流程**:

1. 分析开始 → `isLoading=true`，无输入文字 → 显示"暂停"
2. 用户输入文字 → 按钮自动切换为"发送"
3. 用户点击"发送" → 暂停当前分析 + 发送新消息 → 按钮恢复为"暂停"
4. 清空输入框 → 按钮恢复为"暂停"

### 2.2 执行计划状态更新排查与修复

**问题分析**:

后端 `hook_sink_sse.go` 已正确发出 `step_started`/`step_completed`/`step_failed` 事件。前端 `applyPlanStepStatus()` 也处理了这些事件。

**可能原因**:

1. SSE 事件中的 `call_id`（step ID）与前端 `executionPlan.steps` 中的 `step_id` 不匹配
2. `applyPlanStepStatus()` 中的 step 查找逻辑使用 `step.step_id || step.id`，但 plan 事件中的 step 可能没有正确的 ID 字段
3. SSE 事件解析时 `call_id` 字段未正确传递

**修复方案**:

- 在前端 `applyPlanStepStatus()` 中增加调试日志
- 确保 step ID 匹配逻辑正确
- 确保 SSE 事件的 `call_id` 字段正确解析

### 2.3 中文结论输出

**问题分析**:

`agent-runtime/runtime.go` 的 `generateFinalAnswer()` 函数（约第 742 行）使用硬编码的英文系统 prompt：

```
"Generate a concise final answer. Do not claim unexecuted work was completed. Respond as JSON: {\"final_answer\":\"...\"}."
```

而 Aegis 的 `prompt_provider.go` 中已有中文 `summarizePromptTemplate`，但 runtime 未使用。

**修复方案**:

修改 `agent-runtime/runtime.go` 的 `generateFinalAnswer()` 函数，使其通过 prompt provider 获取 summarize prompt，而非使用硬编码的英文 prompt。

**具体修改**:

1. 在 `agent-runtime/core/types.go` 中为 `PurposeSummarize` 定义 prompt provider 接口方法
2. 在 `runtime.go` 的 `generateFinalAnswer()` 中调用 `r.promptProvider.BuildPrompt(PurposeSummarize, ...)` 获取中文 prompt
3. 如果 prompt provider 未提供 summarize prompt，则回退到现有的英文 prompt（向后兼容）

## 3. 涉及文件

| 文件 | 修改内容 |
|------|---------|
| `frontend/src/views/detection/AIAnalysis.vue` | 按钮状态切换逻辑 |
| `frontend/src/utils/aiAnalysisRuntime.ts` | 执行计划状态更新调试 |
| `agent-runtime/runtime.go` | generateFinalAnswer 使用 prompt provider |
| `agent-runtime/core/types.go` | PurposeSummarize prompt 接口 |

## 4. 测试方案

### 4.1 按钮切换测试

- 启动 AI 分析，验证初始显示"暂停"
- 在输入框输入文字，验证按钮变为"发送"
- 清空输入框，验证按钮恢复为"暂停"
- 点击"发送"，验证暂停当前分析并发送新消息

### 4.2 执行计划更新测试

- 启动 AI 分析，观察执行计划
- 验证步骤状态从"待执行"变为"执行中"再变为"完成"
- 检查 SSE 事件日志确认事件正确发送

### 4.3 中文结论测试

- 启动 AI 分析，等待完成
- 验证最终结论为中文输出
- 检查 attack_graph 中的 title、summary、recommendations 等字段为中文

## 5. 风险与注意事项

1. **向后兼容**: `generateFinalAnswer` 修改需保持向后兼容，当 prompt provider 未提供 summarize prompt 时回退到英文
2. **SSE 事件解析**: 确保前端正确解析所有 SSE 事件字段
3. **按钮状态一致性**: 发送操作需要先暂停再发送，确保状态转换的原子性
