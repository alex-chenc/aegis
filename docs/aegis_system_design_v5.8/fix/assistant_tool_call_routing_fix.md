# 智能助手工具调用路由缺陷修复

## Bug 描述与症状

**症状**: 用户在智能助手会话中发送需要工具调用的查询（如 "查询当前主机的情况"），助手回复 "好的，我来为您查询当前主机的情况。首先我将调用 Host.List 工具来获取主机列表信息。正在为您查询主机列表..." 后卡住，不再有后续响应。

**影响范围**: 所有需要工具调用的查询，包括但不限于：
- 查询主机列表
- 查看告警信息
- 查询任务状态
- 查看漏洞信息

## 复现步骤

1. 打开智能助手界面
2. 发送消息 "查询当前主机的情况" 或 "帮我看看有哪些主机"
3. 助手回复描述性文字 "好的，我来为您查询当前主机的情况..."
4. 无后续工具调用和结果返回

## 根因分析

问题由**四层缺陷叠加**导致：

### 缺陷 1：任务分类遗漏

**文件**: `api-server/internal/assistant/orchestrator.go:225-261`

`isComplexTask` 函数将 "查询当前主机的情况" 分类为**简单任务**，因为：
- TaskType 不是 "investigation"、"host_attack_investigation" 或 "remediation"
- Intent.Action 是 "query"（不是 "analyze" 或 "investigate"）
- 消息不包含复杂关键词（"分析"、"调查"、"研判" 等）
- 消息长度不超过 100 字符

但这个查询**需要调用 Host.List 工具**才能完成，应该走 agent-runtime 路径。

### 缺陷 2：简单路径无法执行工具

**文件**: `api-server/internal/assistant/orchestrator.go:278-332`

`runDirectLLM` 路径：
1. 系统提示词告诉 LLM "所有操作必须通过工具调用完成，不能直接执行命令"
2. LLM 遵循指令，回复描述性文字而非实际数据
3. 这段文字被当作最终答案返回给用户
4. **没有真正的工具执行机制**

### 缺陷 3：Run Context 无超时

**文件**: `api-server/internal/assistant/run_manager.go:47`

`RunManager.Start` 创建的 context 使用 `context.WithCancel(context.Background())`，**没有超时**。如果 LLM API 挂起，goroutine 会永远阻塞，session 永远卡在 "running" 状态。

### 缺陷 4：Goroutine 无 Panic Recovery

**文件**: `api-server/internal/assistant/service.go:221-231`

运行 orchestrator 的 goroutine 没有 `recover()`，如果发生 panic，`completeRun` 永远不会被调用，session 永远卡在 "running" 状态。

### 缺陷 5：LLM 返回自然语言而非 JSON

**文件**: `api-server/internal/assistant/adapter_prompt_provider.go:110-142`

即使任务正确路由到 agent-runtime，LLM 仍然返回自然语言描述（如 "我来帮您查询主机的情况..."）而非 JSON 格式的工具调用。agent-runtime 的解析器无法解析自然语言文本，导致解析失败。

## 修复设计

### 修复 1：增强任务分类逻辑

在 `isComplexTask` 中添加工具选择检测：如果工具选择器选中了工具，说明查询需要工具调用，必须走 agent-runtime 路径。

```go
// 5. 如果选中了工具，说明需要工具调用，必须走 agent-runtime
if len(selectedTools) > 0 {
    return true
}
```

### 修复 2：简化简单路径的系统提示词

移除 `buildSimpleSystemPrompt` 中关于工具调用的指令，因为简单路径不支持工具执行：

```go
func (o *Orchestrator) buildSimpleSystemPrompt() string {
    return `你是 Aegis 智能安全助手...
当前对话不涉及具体的数据查询操作，请基于你的知识直接回答。
如果用户的问题需要查询系统数据，请告知用户你可以帮助查询哪些方面的信息。`
}
```

### 修复 3：添加工具调用描述检测

添加 `looksLikeToolCallDescription` 函数，检测 LLM 响应是否在描述工具调用而非直接回答。如果是且有可用工具，自动重新路由到 agent-runtime。

### 修复 4：添加 Run Context 超时

将 `RunManager.Start` 的 context 从 `WithCancel` 改为 `WithTimeout(5分钟)`：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
```

### 修复 5：添加 Panic Recovery

在 service.go 的 goroutine 中添加 `defer recover()`，确保 panic 时也能正确清理 session 状态。

### 修复 6：改进 ReAct 提示词

在 `adapter_prompt_provider.go` 中改进 `buildReactPrompt`，明确禁止自然语言描述，要求直接输出 JSON：

```
## 禁止事项
- 禁止输出自然语言描述（如"我来帮您查询..."），必须直接输出JSON
- 禁止解释你要做什么，直接输出工具调用JSON
```

### 修复 7：添加自然语言工具调用回退机制

在 `llm_client_adapter.go` 中添加 `extractToolCallFromText` 函数，当 LLM 返回自然语言文本时，尝试从中提取工具名称并构造 JSON 工具调用：

```go
func extractToolCallFromText(content string) string {
    knownTools := []string{"Host.List", "Host.GetDetail", ...}
    for _, toolName := range knownTools {
        if strings.Contains(content, toolName) {
            // 构造 JSON 工具调用
            ...
        }
    }
    return ""
}
```

## 代码变更

### 文件 1: `api-server/internal/assistant/orchestrator.go`

1. **`isComplexTask` 函数** (行 225-261):
   - 新增 `selectedTools []string` 参数
   - 添加工具选择检测逻辑

2. **`Run` 方法** (行 144):
   - 更新 `isComplexTask` 调用，传入 `selection.SelectedTools`

3. **`runDirectLLM` 方法** (行 278-332):
   - 使用简化的 `buildSimpleSystemPrompt()`（不传入工具描述）
   - 添加工具调用描述检测和重新路由逻辑

4. **`buildSimpleSystemPrompt` 方法** (行 487-515):
   - 移除工具描述和工具调用指令
   - 改为通用助手提示词

5. **新增 `looksLikeToolCallDescription` 函数**:
   - 检测 LLM 响应中的工具调用描述关键词
   - 用于安全回退机制

### 文件 2: `api-server/internal/assistant/run_manager.go`

1. **`Start` 方法** (行 47):
   - `context.WithCancel` → `context.WithTimeout(5分钟)`

### 文件 3: `api-server/internal/assistant/service.go`

1. **`SendMessage` 方法** (行 221-231):
   - 添加 `defer recover()` 处理 panic
   - panic 时调用 `completeRun` 清理状态

### 文件 4: `api-server/internal/assistant/adapter_prompt_provider.go`

1. **`buildReactPrompt` 方法** (行 110-142):
   - 添加明确禁止自然语言描述的指令
   - 添加同时调用多个工具的示例
   - 强化输出格式要求

### 文件 5: `api-server/internal/llm/adapters/llm_client_adapter.go`

1. **`normalizeToolCallFormat` 函数**:
   - 重构为先尝试 JSON 解析，再尝试自然语言提取
   - 添加 `extractToolCallFromText` 回退机制

2. **新增 `extractToolCallFromText` 函数**:
   - 从自然语言文本中提取已知工具名称
   - 构造标准 JSON 工具调用格式

## 验证步骤

1. **构建验证**:
   ```bash
   cd api-server && go build ./...
   ```

2. **功能验证**:
   - 发送 "查询当前主机的情况" → 应通过 agent-runtime 执行 Host.List 工具并返回结果
   - 发送 "你好" → 应通过简单路径直接回复
   - 发送 "分析最近的安全事件" → 应通过 agent-runtime 执行

3. **超时验证**:
   - 模拟 LLM API 超时 → 应在 5 分钟后自动恢复，session 状态变为 "failed"

4. **Panic Recovery 验证**:
   - 模拟 orchestrator panic → session 状态应变为 "failed"，不会永远卡在 "running"

## 影响组件

- **api-server/internal/assistant/orchestrator.go**: 任务分类和执行路径
- **api-server/internal/assistant/run_manager.go**: 运行上下文管理
- **api-server/internal/assistant/service.go**: 后台任务管理

## 风险与回滚计划

**风险**:
- 低风险：修改仅影响智能助手模块，不影响其他功能
- 工具调用描述检测可能有误判（极低概率）

**回滚计划**:
- 回滚 `orchestrator.go`、`run_manager.go`、`service.go` 的变更
- 重新构建 api-server
