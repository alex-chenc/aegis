package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"api-server/internal/llm"
)

const llmJSONParseMaxAttempts = 3

const jsonOnlyRetryReminder = "The previous response was not valid parseable JSON. Return exactly one JSON object this time. " +
	"Do not output reasoning, explanations, or Markdown fences. Start with { and end with }."

func jsonObjectResponseFormat(client *llm.LLMClient) *llm.ResponseFormat {
	if client != nil && client.SupportsJSONObjectResponseFormat() {
		return &llm.ResponseFormat{Type: "json_object"}
	}
	return nil
}

func requestLLMJSONWithRetry(
	ctx context.Context,
	target interface{},
	baseMessages []llm.Message,
	call func(ctx context.Context, messages []llm.Message, temperature float64) (string, error),
) error {
	var lastErr error
	for attempt := 0; attempt < llmJSONParseMaxAttempts; attempt++ {
		messages := baseMessages
		temperature := 0.1
		if attempt > 0 {
			messages = append(append([]llm.Message{}, baseMessages...), llm.Message{
				Role:    "user",
				Content: jsonOnlyRetryReminder,
			})
			temperature = 0.35
		}
		response, err := call(ctx, messages, temperature)
		if err != nil {
			return err
		}
		if parseErr := unmarshalFirstJSONObject(response, target); parseErr != nil {
			lastErr = parseErr
			continue
		}
		return nil
	}
	return lastErr
}

func unmarshalFirstJSONObject(raw string, target interface{}) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return fmt.Errorf("no json object found in llm response")
	}
	return json.Unmarshal([]byte(raw[start:end+1]), target)
}

func encodeJSON(value interface{}) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
