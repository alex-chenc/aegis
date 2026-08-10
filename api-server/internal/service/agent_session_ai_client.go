package service

import (
	"context"
	"errors"

	"api-server/internal/llm"
	"api-server/internal/repository"
)

type configuredAgentSessionAIClient struct {
	client *ConfiguredAgentGuardAnalysisClient
}

func NewConfiguredAgentSessionAIClient(config *repository.ConfigRepository, timeoutSeconds, maxRetries int) AgentSessionAIClient {
	if config == nil {
		return nil
	}
	return configuredAgentSessionAIClient{client: NewConfiguredAgentGuardAnalysisClient(config, timeoutSeconds, maxRetries)}
}

func (c configuredAgentSessionAIClient) Complete(ctx context.Context, messages []Message, _ string) (string, string, string, TokenUsage, error) {
	if c.client == nil {
		return "", "", "", TokenUsage{}, errors.New("agent session AI client unavailable")
	}
	llmMessages := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		llmMessages = append(llmMessages, llm.Message{Role: message.Role, Content: message.Content})
	}
	content, provider, modelName, err := c.client.Complete(ctx, llmMessages, nil)
	return content, provider, modelName, TokenUsage{Input: estimateTokenInt(messages), Output: int(estimateTokens(content)), Total: estimateTokenInt(messages) + int(estimateTokens(content))}, err
}

func estimateTokenInt(messages []Message) int {
	total := 0
	for _, message := range messages {
		total += int(estimateTokens(message.Content))
	}
	return total
}
