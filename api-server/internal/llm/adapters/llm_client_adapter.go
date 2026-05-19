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
		Content: result.Content,
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
