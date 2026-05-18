# 阿里云百炼 (DashScope) JSON Mode 适配设计文档

**版本**: 5.7
**状态**: 设计方案
**适用范围**: api-server LLM 客户端、agent-runtime 适配器、dc LLM 客户端

## 1. 背景与问题

### 1.1 现状

当前 Aegis 的 LLM 客户端完全依赖 prompt 工程来获取结构化 JSON 输出，然后通过后处理的括号匹配算法 (`extractJSON()`) 提取 JSON。这种方式存在以下问题：

- LLM 可能在 JSON 前后输出额外文本，导致提取失败
- 嵌套括号匹配算法在复杂 JSON 结构下可能出错
- 不同 LLM 提供商对 JSON 输出的遵循程度不一

### 1.2 机会

阿里云百炼 (DashScope) 的 OpenAI 兼容 API 支持 `response_format` 参数：

```json
{"type": "text"}          // 默认，纯文本
{"type": "json_object"}   // 保证输出合法 JSON
{"type": "json_schema", "json_schema": {...}}  // 指定 JSON Schema 的结构化输出
```

使用 `json_object` 类型时，API **保证**返回合法的 JSON 字符串，无需后处理提取。

### 1.3 DashScope 约束

- 使用 `json_object` 时，prompt 消息中必须包含 "json" 关键词（不区分大小写），否则返回错误
- 思考模式模型（如 qwen-plus-latest 开启 thinking）不支持 `json_object`，需使用 `json_schema`

## 2. 设计方案

### 2.1 整体架构

```
agent-runtime (ResponseFormat 类型)
    ↓
api-server LLM client (ChatCompletionWithMessagesFormat)
    ↓
LLMClientAdapter.Complete() (自动检测 DashScope + JSON 关键词)
    ↓
DashScope API (response_format: json_object)
```

### 2.2 触发条件

JSON Mode 自动启用需同时满足三个条件：

1. **Provider 为 DashScope**: 通过 URL 检测 (`contains "dashscope"` 或 `"aliyuncs"`)
2. **请求期望 JSON 输出**: `LLMRequest.ResponseSchema != ""` (agent-runtime 已为 PurposePlan 和 PurposeSummarize 设置)
3. **Prompt 包含 JSON 关键词**: 防止 DashScope API 报错

### 2.3 降级策略

思考模式模型（如 `qwen3.5-plus`）不支持 `json_object` 类型的 `response_format`，DashScope 会返回错误。为此，适配器实现了自动降级：

1. 首次尝试使用 `response_format: {"type": "json_object"}` 发送请求
2. 如果失败（且使用了 response_format），自动去掉 `response_format` 重试
3. 此时退回到原有的 prompt 工程 + 括号匹配提取模式

### 2.4 类型定义

#### agent-runtime (`core/types.go`)

新增 `ResponseFormat` 类型：

```go
type ResponseFormat struct {
    Type       string                `json:"type"`
    JSONSchema *ResponseFormatSchema `json:"json_schema,omitempty"`
}

type ResponseFormatSchema struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    Schema      json.RawMessage `json:"schema,omitempty"`
    Strict      bool            `json:"strict,omitempty"`
}
```

在 `LLMRequest` 中新增字段：

```go
ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
```

#### api-server LLM client (`client.go`)

新增本地 `ResponseFormat` 结构体（与 agent-runtime 解耦），新增到 `ChatCompletionRequest`：

```go
type ChatCompletionRequest struct {
    // ... existing fields ...
    ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}
```

### 2.5 Provider 检测

遵循现有 `isMiniMaxM2()`/`usesAnthropicAPI()` 模式：

```go
func (c *LLMClient) isDashScope() bool {
    baseURL := strings.ToLower(c.baseURL)
    return strings.Contains(baseURL, "dashscope") || strings.Contains(baseURL, "aliyuncs")
}
```

### 2.6 方法设计

新增 `ChatCompletionWithMessagesFormat` 方法，现有 `ChatCompletionWithMessages` 委托调用：

```go
func (c *LLMClient) ChatCompletionWithMessages(ctx, messages, temperature) (string, error) {
    return c.ChatCompletionWithMessagesFormat(ctx, messages, temperature, nil)
}

func (c *LLMClient) ChatCompletionWithMessagesFormat(ctx, messages, temperature, responseFormat) (string, error) {
    // ... 原有重试逻辑，reqBody 增加 ResponseFormat ...
}
```

## 3. 修改范围

| 文件 | 修改内容 |
|------|---------|
| `/code/agent-runtime/core/types.go` | 新增 `ResponseFormat` 类型，`LLMRequest` 新增字段 |
| `/code/aegis/api-server/internal/llm/client.go` | 新增 `ResponseFormat` 结构体、`isDashScope()`、`ChatCompletionWithMessagesFormat` |
| `/code/aegis/api-server/internal/llm/adapters/llm_client_adapter.go` | `Complete()` 中自动检测并设置 `ResponseFormat` |
| `/code/aegis/dc/internal/llm/client.go` | 新增 `isDashScope()`，`Analyze()` 中注入 `response_format` |
| `/code/aegis/api-server/go.mod` | 更新 agent-runtime 依赖版本 |

## 4. 测试策略

### 4.1 单元测试 (TDD)

- **agent-runtime**: `ResponseFormat` JSON 序列化/反序列化
- **api-server LLM client**: `isDashScope()` URL 检测、`ChatCompletionRequest` 序列化
- **adapter**: `containsJSONKeyword()` 关键词检测
- **dc client**: `isDashScope()` 检测

### 4.2 集成测试

使用 curl 调用 API，验证 DashScope provider 配置后 LLM 调用返回合法 JSON。

## 5. 已知限制

- 当前仅支持 `json_object` 类型，未实现 `json_schema` 结构化输出
- 思考模式模型需使用 `json_schema` 而非 `json_object`，当前未处理此场景
- Anthropic 协议路径不支持 `response_format`，不在此设计范围内
