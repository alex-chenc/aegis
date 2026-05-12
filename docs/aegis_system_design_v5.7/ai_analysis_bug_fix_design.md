# AI 分析 Bug 修复设计文档

## 1. 问题描述

AI 分析功能在发起分析请求时失败，前端显示 "AI 分析连接中断，请稍后重试或查看服务日志"。

## 2. 根因分析

### 2.1 错误链条

```
前端 SSE onerror → "AI 分析连接中断"
    ↑
SSE 连接异常关闭
    ↑
api-server StreamHandler 返回错误
    ↑
agent-runtime.Run() 返回错误
    ↑
LLMClientAdapter.Complete() 返回错误
    ↑
LLM client sendRequest() 返回 400 错误
    ↑
GLM API 返回 {"error":{"code":"1214","message":"messages 参数非法"}}
```

### 2.2 根本原因

**消息格式不完整导致 GLM API 拒绝请求。**

在 Plan 阶段，消息构建流程存在缺陷：

1. `AegisPromptProvider.buildPlanPrompt()` 返回 `PromptBundle{SystemPrompt: planPromptTemplate}`，但 `Messages` 字段为空切片
2. Planner 的 `Generate()` 方法只使用 `prompt.Messages`（空），忽略了 `SystemPrompt` 字段
3. `LLMClientAdapter.injectAlertContext()` 只注入了包含告警上下文的 system 消息
4. 最终发送给 GLM API 的消息数组：`[{role: "system", content: "## 告警上下文\n..."}]`
5. GLM API（智谱AI）要求至少包含一条 user 角色消息，返回 400 错误

### 2.3 代码位置

| 文件 | 行号 | 问题 |
|------|------|------|
| `api-server/internal/llm/adapters/prompt_provider.go` | 52-68 | `buildPlanPrompt` 只设置 SystemPrompt，未构建 Messages |
| `agent-runtime/planner/planner.go` | 59-65 | `Generate` 只传递 `prompt.Messages`，忽略 `SystemPrompt` |
| `api-server/internal/llm/adapters/llm_client_adapter.go` | 33-65 | `Complete` 未处理空 Messages 场景 |
| `frontend/src/api/aiAnalysis.ts` | 249-256 | SSE onerror 显示误导性 "连接中断" 消息 |

## 3. 修复方案（已实施）

### 3.1 核心修复：PromptProvider 返回完整消息

修改 `AegisPromptProvider.buildPlanPrompt()` 返回包含完整消息的 `PromptBundle`：

```go
// prompt_provider.go:66-72
return agentruntime.PromptBundle{
    SystemPrompt: systemPrompt,
    Messages: []agentruntime.LLMMessage{
        {Role: "system", Content: systemPrompt},
        {Role: "user", Content: "请根据以上指令和告警上下文，制定详细的安全分析计划。"},
    },
}, nil
```

### 3.2 防御性修复：LLMClientAdapter nil 检查

在 `LLMClientAdapter.injectAlertContext()` 增加 nil 检查：

```go
// llm_client_adapter.go:101-103
if a.alertCtx == nil {
    return messages
}
```

### 3.3 前端错误提示优化（待后续迭代）

修改 `aiAnalysis.ts` 的 SSE onerror 处理：

- 区分连接中断和服务端错误
- 对 400 类错误显示更精确的提示
- 增加重试机制

## 4. 影响范围

- **后端**：`prompt_provider.go`, `llm_client_adapter.go`
- **前端**：`aiAnalysis.ts`（待后续迭代）
- **不涉及**：agent-runtime 库（通过 adapter 层修复）

## 5. 测试结果

### 5.1 单元测试

```
=== RUN   TestBuildPlanPrompt_ReturnsMessages        --- PASS
=== RUN   TestBuildPlanPrompt_SystemPromptInMessages  --- PASS
=== RUN   TestBuildPlanPrompt_MessagesHaveValidRoles  --- PASS
=== RUN   TestBuildReactPrompt_ReturnsSystemPrompt    --- PASS
=== RUN   TestBuildSummarizePrompt_ReturnsSystemPrompt --- PASS
=== RUN   TestInjectAlertContext_WithEmptyMessages     --- PASS
=== RUN   TestInjectAlertContext_WithExistingSystemMessage --- PASS
=== RUN   TestInjectAlertContext_NilAlertCtx          --- PASS
=== RUN   TestTemperatureForPurpose                   --- PASS
=== RUN   TestIsContextualPurpose                     --- PASS
PASS ok api-server/internal/llm/adapters 0.008s
```

### 5.2 集成测试（curl SSE）

修复前：
```
LLM request failed with non-retryable error: API returned status 400: {"error":{"code":"1214","message":"messages 参数非法"}}
```

修复后：
```
data: {"type":"thinking","content":"Creating analysis plan..."}
AI response collected: content_len=179
```

### 5.3 验证命令

```bash
go vet ./internal/llm/adapters/...    # Pass
go test ./internal/llm/adapters/...   # Pass (10/10)
go build ./...                        # Pass
```
