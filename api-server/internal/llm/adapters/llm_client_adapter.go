package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"

	"api-server/internal/llm"
)

// LLMClientAdapter wraps the existing llm.LLMClient to satisfy the
// agent-runtime LLMClient interface.
type LLMClientAdapter struct {
	client   *llm.LLMClient
	alertCtx map[string]interface{}
}

// NewLLMClientAdapter creates a new adapter that bridges api-server's LLMClient
// with the agent-runtime LLMClient interface. The alertCtx, when non-nil, is
// injected into system messages for plan/react/summarize purposes.
func NewLLMClientAdapter(client *llm.LLMClient, alertCtx map[string]interface{}) *LLMClientAdapter {
	return &LLMClientAdapter{
		client:   client,
		alertCtx: alertCtx,
	}
}

// Complete implements the agent-runtime LLMClient interface. It translates the
// request into the format expected by the underlying LLM client and returns the
// response.
func (a *LLMClientAdapter) Complete(ctx context.Context, req agentruntime.LLMRequest) (agentruntime.LLMResponse, error) {
	// Convert agent-runtime messages to llm.Message slice.
	messages := make([]llm.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, llm.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Determine temperature based on purpose.
	temperature := a.temperatureForPurpose(req.Purpose)

	// Allow the caller to override temperature explicitly.
	if req.Temperature != nil {
		temperature = float64(*req.Temperature)
	}

	// For plan/react/summarize purposes, inject alert context into the first
	// system message when available.
	if a.alertCtx != nil && isContextualPurpose(req.Purpose) {
		messages = a.injectAlertContext(messages)
	}

	// Auto-enable JSON mode for DashScope when the request expects structured output
	// and the prompt contains the required "json" keyword.
	var responseFormat *llm.ResponseFormat
	if req.ResponseSchema != "" && a.client.IsDashScope() && containsJSONKeyword(messages) {
		responseFormat = &llm.ResponseFormat{Type: "json_object"}
	}

	result, err := a.client.ChatCompletionWithMessagesFormatResult(ctx, messages, temperature, responseFormat)
	if err != nil && responseFormat != nil {
		// Fallback: thinking mode models (e.g., qwen3.5-plus) do not support json_object.
		// Retry without response_format.
		result, err = a.client.ChatCompletionWithMessagesFormatResult(ctx, messages, temperature, nil)
	}
	if err != nil {
		return agentruntime.LLMResponse{}, fmt.Errorf("llm completion failed: %w", err)
	}

	return agentruntime.LLMResponse{
		Content: normalizeToolCallFormat(cleanLLMResponse(result.Content)),
		Model:   result.Model,
		Usage: agentruntime.LLMUsage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
	}, nil
}

// temperatureForPurpose returns the default temperature for each LLM purpose.
func (a *LLMClientAdapter) temperatureForPurpose(purpose agentruntime.LLMPurpose) float64 {
	switch purpose {
	case agentruntime.PurposePlan:
		return 0.4
	case agentruntime.PurposeReact:
		return 0.7
	case agentruntime.PurposeAudit:
		return 0.3
	case agentruntime.PurposeReflect:
		return 0.3
	case agentruntime.PurposeCorrect:
		return 0.4
	case agentruntime.PurposeSummarize:
		return 0.3
	case agentruntime.PurposeCompress:
		return 0.3
	default:
		return 0.7
	}
}

// isContextualPurpose returns true for purposes that should receive the alert
// context injection.
func isContextualPurpose(purpose agentruntime.LLMPurpose) bool {
	switch purpose {
	case agentruntime.PurposePlan, agentruntime.PurposeReact, agentruntime.PurposeSummarize:
		return true
	default:
		return false
	}
}

// injectAlertContext appends the serialized alert context to the first system
// message. If no system message exists, one is prepended.
func (a *LLMClientAdapter) injectAlertContext(messages []llm.Message) []llm.Message {
	if a.alertCtx == nil {
		return messages
	}
	ctxJSON, err := json.Marshal(a.alertCtx)
	if err != nil {
		// If serialization fails, skip injection rather than breaking the call.
		return messages
	}

	suffix := fmt.Sprintf("\n\n## 告警上下文\n%s", string(ctxJSON))

	for i, m := range messages {
		if m.Role == "system" {
			messages[i].Content = m.Content + suffix
			return messages
		}
	}

	// No system message found; prepend one.
	return append([]llm.Message{
		{Role: "system", Content: suffix},
	}, messages...)
}

// containsJSONKeyword checks if any message contains the word "json" (case-insensitive).
// DashScope requires this when using json_object response format.
func containsJSONKeyword(messages []llm.Message) bool {
	for _, m := range messages {
		if strings.Contains(strings.ToLower(m.Content), "json") {
			return true
		}
	}
	return false
}

// cleanLLMResponse cleans the LLM response by removing markdown code blocks,
// BOM characters, and other formatting that would break JSON parsing.
func cleanLLMResponse(content string) string {
	// Remove BOM character (UTF-8 BOM: EF BB BF)
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")

	// Remove markdown code blocks
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	// Find the first { or [ to start of JSON
	if idx := strings.IndexAny(content, "{["); idx > 0 {
		content = content[idx:]
	}

	// Find the last } or ] to end of JSON
	if idx := strings.LastIndexAny(content, "}]"); idx >= 0 && idx < len(content)-1 {
		content = content[:idx+1]
	}

	return content
}

// normalizeToolCallFormat converts alternative LLM tool call formats into the
// standard agent-runtime format: {"action":"tool_call","tool_call":{"tool_name":"X","args":{...}}}
//
// Handles:
//   - {"name":"X","arguments":{...}}          (OpenAI function calling style)
//   - {"name":"X","args":{...}}               (variant)
//   - {"tool":"X","input":{...}}              (Anthropic style)
//   - {"function":{"name":"X","arguments":{}}} (OpenAI function call style)
//   - Natural language tool call descriptions  (LLM fallback)
func normalizeToolCallFormat(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return content
	}

	// Quick check: if it already has "action" field and looks like valid JSON, no normalization needed
	if strings.HasPrefix(content, "{") && strings.Contains(content, `"action"`) {
		return content
	}

	// Try to parse as JSON first
	if strings.HasPrefix(content, "{") {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(content), &raw); err == nil {
			// Valid JSON, try to extract tool call
			toolName := extractToolName(raw)
			if toolName != "" {
				args := extractToolArgs(raw)
				if args == nil {
					args = json.RawMessage("{}")
				}
				normalized := map[string]interface{}{
					"action":  "tool_call",
					"summary": fmt.Sprintf("调用 %s", toolName),
					"tool_call": map[string]interface{}{
						"tool_name": toolName,
						"args":      json.RawMessage(args),
					},
				}
				result, err := json.Marshal(normalized)
				if err == nil {
					return string(result)
				}
			}
		}
	}

	// 不再从自然语言文本中提取工具调用（会产生空参数的不可靠调用）
	// 如果 LLM 返回自然语言而非 JSON，让 ReAct 执行器处理解析错误并重试
	return content
}

// extractToolCallFromText attempts to extract a tool call from natural language text.
// This is a fallback for when the LLM returns descriptions instead of JSON.
func extractToolCallFromText(content string) string {
	// Known tool names that the system supports (must match tool_registry.go registrations)
	knownTools := []string{
		"Host.List", "Host.Get", "Host.AgentStatus.Get",
		"Detection.Alert.List", "Detection.Alert.Get", "Detection.Statistics.Get", "Detection.Trend.Get",
		"Vulnerability.List", "Vulnerability.AffectedHosts", "Software.Installed.Search",
		"Task.List", "Task.GetDetail", "Task.RunCheck", "Task.RunFix",
		"Block.Policy.List", "Block.Policy.Update",
		"Audit.Log.List",
		"Package.List", "Package.Get",
		"SigmaRule.List", "SigmaRule.Generate",
		"Config.Get",
		"Agent.Process.List", "Agent.Process.Tree", "Agent.Network.List", "Agent.File.OpenList", "Agent.Log.Query",
		"Investigation.HostAttack.Analyze", "Investigation.HostAttack.Plan",
	}

	// Look for known tool names in the content
	for _, toolName := range knownTools {
		if strings.Contains(content, toolName) {
			// Found a tool name, construct a JSON tool call
			normalized := map[string]interface{}{
				"action":  "tool_call",
				"summary": fmt.Sprintf("调用 %s", toolName),
				"tool_call": map[string]interface{}{
					"tool_name": toolName,
					"reason":    "从用户请求中检测到需要调用此工具",
					"args":      map[string]interface{}{},
				},
			}
			result, err := json.Marshal(normalized)
			if err == nil {
				return string(result)
			}
		}
	}

	return ""
}

// extractToolName tries to extract the tool name from various JSON formats.
func extractToolName(raw map[string]json.RawMessage) string {
	// Format: {"name":"X",...}
	if v, ok := raw["name"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil && s != "" {
			return s
		}
	}

	// Format: {"tool":"X",...}
	if v, ok := raw["tool"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil && s != "" {
			return s
		}
	}

	// Format: {"function":{"name":"X",...}}
	if v, ok := raw["function"]; ok {
		var fn struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(v, &fn) == nil && fn.Name != "" {
			return fn.Name
		}
	}

	return ""
}

// extractToolArgs tries to extract the tool arguments from various JSON formats.
func extractToolArgs(raw map[string]json.RawMessage) json.RawMessage {
	// Format: {"args":{...}}
	if v, ok := raw["args"]; ok {
		return v
	}

	// Format: {"arguments":{...}}
	if v, ok := raw["arguments"]; ok {
		return v
	}

	// Format: {"input":{...}}
	if v, ok := raw["input"]; ok {
		return v
	}

	// Format: {"parameters":{...}}
	if v, ok := raw["parameters"]; ok {
		return v
	}

	// Format: {"function":{"arguments":"..."}}
	if v, ok := raw["function"]; ok {
		var fn struct {
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(v, &fn) == nil && fn.Arguments != nil {
			// OpenAI sometimes returns arguments as a JSON string
			var s string
			if json.Unmarshal(fn.Arguments, &s) == nil {
				return json.RawMessage(s)
			}
			return fn.Arguments
		}
	}

	return nil
}
