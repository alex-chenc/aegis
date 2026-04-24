package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"api-server/pkg/logger"

	"go.uber.org/zap"
)

type LLMClient struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	modelName  string
	maxRetries int
	timeout    time.Duration
}

type ChatCompletionRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Temperature    float64   `json:"temperature,omitempty"`
	MaxTokens      int       `json:"max_tokens,omitempty"`
	Stream         bool      `json:"stream,omitempty"`
	ReasoningSplit bool      `json:"reasoning_split,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	Choices []Choice `json:"choices"`
	Error   *Error   `json:"error,omitempty"`
}

type Choice struct {
	Message ChatCompletionMessage `json:"message"`
}

type ChatCompletionMessage struct {
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	ReasoningDetails []ReasoningDetail `json:"reasoning_details,omitempty"`
}

type ReasoningDetail struct {
	Text string `json:"text"`
}

type Error struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

type anthropicMessageRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMessageResponse struct {
	Content []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		Thinking string `json:"thinking,omitempty"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func NewLLMClient(apiKey, baseURL, modelName string, timeoutSeconds, maxRetries int) *LLMClient {
	return &LLMClient{
		// Don't set httpClient.Timeout - rely on context deadline instead
		// This ensures proper cancellation when context is canceled
		httpClient: &http.Client{},
		apiKey:     apiKey,
		baseURL:    baseURL,
		modelName:  modelName,
		maxRetries: maxRetries,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

func (c *LLMClient) prepareRequest(reqBody ChatCompletionRequest) ChatCompletionRequest {
	if !c.isMiniMaxM2() {
		return reqBody
	}

	reqBody.ReasoningSplit = true
	if reqBody.MaxTokens <= 0 || reqBody.MaxTokens > 2048 {
		reqBody.MaxTokens = 2048
	}
	if reqBody.Temperature <= 0 || reqBody.Temperature > 1 {
		reqBody.Temperature = 1
	}
	return reqBody
}

func (c *LLMClient) isMiniMaxM2() bool {
	model := strings.ToLower(c.modelName)
	return strings.Contains(model, "minimax-m2") || strings.Contains(model, "minimax m2")
}

func (c *LLMClient) isMiniMaxBaseURL() bool {
	baseURL := strings.ToLower(c.baseURL)
	return strings.Contains(baseURL, "minimaxi.com") || strings.Contains(baseURL, "minimax.io")
}

func (c *LLMClient) usesAnthropicAPI() bool {
	baseURL := strings.ToLower(c.baseURL)
	return strings.Contains(baseURL, "/anthropic") || (c.isMiniMaxM2() && c.isMiniMaxBaseURL())
}

func (c *LLMClient) chatCompletionsURL() string {
	return fmt.Sprintf("%s/chat/completions", strings.TrimRight(c.baseURL, "/"))
}

func (c *LLMClient) anthropicMessagesURL() string {
	baseURL := strings.TrimRight(c.baseURL, "/")
	if c.isMiniMaxM2() && c.isMiniMaxBaseURL() && !strings.Contains(strings.ToLower(baseURL), "/anthropic") {
		if parsed, err := url.Parse(baseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return fmt.Sprintf("%s://%s/anthropic/v1/messages", parsed.Scheme, parsed.Host)
		}
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return fmt.Sprintf("%s/messages", baseURL)
	}
	return fmt.Sprintf("%s/v1/messages", baseURL)
}

func (c *LLMClient) buildAnthropicRequest(messages []Message, temperature float64, maxTokens int, stream bool) anthropicMessageRequest {
	var systemParts []string
	anthropicMessages := make([]anthropicMessage, 0, len(messages))

	for _, msg := range messages {
		if msg.Role == "system" {
			systemParts = append(systemParts, msg.Content)
			continue
		}

		role := msg.Role
		if role != "assistant" {
			role = "user"
		}
		anthropicMessages = append(anthropicMessages, anthropicMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	if maxTokens <= 0 || maxTokens > 2048 {
		maxTokens = 2048
	}
	if temperature <= 0 || temperature > 1 {
		temperature = 1
	}

	return anthropicMessageRequest{
		Model:       c.modelName,
		Messages:    anthropicMessages,
		System:      strings.Join(systemParts, "\n\n"),
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stream:      stream,
	}
}

func (c *LLMClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error) {
	var messages []Message
	if systemPrompt != "" {
		messages = []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	} else {
		messages = []Message{
			{Role: "user", Content: userPrompt},
		}
	}

	reqBody := ChatCompletionRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   4096,
	}

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		response, err := c.sendRequest(ctx, reqBody)
		if err == nil && response != "" {
			return response, nil
		}

		// Handle empty response case
		if err == nil && response == "" {
			logger.Warn("LLM returned empty response, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", c.maxRetries))
			lastErr = fmt.Errorf("empty response from LLM")
		} else {
			lastErr = err
		}

		// Check if error is retryable (empty response is always retryable)
		isRetryable := c.isRetryableError(lastErr) || lastErr.Error() == "empty response from LLM"
		if !isRetryable {
			logger.Error("LLM request failed with non-retryable error",
				zap.Error(lastErr),
				zap.Int("attempt", attempt+1),
			)
			return "", lastErr
		}

		// Exponential backoff: 2s, 4s, 8s
		backoff := time.Duration(1<<uint(attempt)) * time.Second
		logger.Warn("LLM request failed, retrying with backoff",
			zap.Error(lastErr),
			zap.Int("attempt", attempt+1),
			zap.Duration("backoff", backoff),
		)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
			continue
		}
	}

	return "", fmt.Errorf("LLM request failed after %d attempts: %w", c.maxRetries, lastErr)
}

func (c *LLMClient) sendRequest(ctx context.Context, reqBody ChatCompletionRequest) (string, error) {
	if c.usesAnthropicAPI() {
		return c.sendAnthropicRequest(ctx, c.buildAnthropicRequest(reqBody.Messages, reqBody.Temperature, reqBody.MaxTokens, false))
	}

	reqBody = c.prepareRequest(reqBody)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.chatCompletionsURL()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				logger.Warn("LLM rate limited, waiting for Retry-After",
					zap.String("retry_after", retryAfter),
					zap.Int("sleep_seconds", seconds),
				)
				time.Sleep(time.Duration(seconds) * time.Second)
			} else {
				logger.Warn("LLM rate limited, invalid Retry-After value",
					zap.String("retry_after", retryAfter),
					zap.Error(err),
				)
				time.Sleep(time.Second)
			}
		}
		return "", fmt.Errorf("rate limited")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var completionResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if completionResp.Error != nil {
		return "", fmt.Errorf("API error: %s", completionResp.Error.Message)
	}

	if len(completionResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	message := completionResp.Choices[0].Message
	return message.Content, nil
}

func (c *LLMClient) sendAnthropicRequest(ctx context.Context, reqBody anthropicMessageRequest) (string, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.anthropicMessagesURL(), bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var completionResp anthropicMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if completionResp.Error != nil {
		return "", fmt.Errorf("API error: %s", completionResp.Error.Message)
	}

	var text strings.Builder
	for _, block := range completionResp.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		for _, block := range completionResp.Content {
			if block.Type == "thinking" {
				text.WriteString(block.Thinking)
			}
		}
	}
	if text.Len() == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return text.String(), nil
}

func (c *LLMClient) isRetryableError(err error) bool {
	errStr := err.Error()
	return containsAny(errStr, []string{
		"timeout",
		"connection",
		"500",
		"502",
		"503",
		"504",
		"rate limited",
		"dial tcp",
		"lookup",
		"dns",
		"server misbehaving",
		"no such host",
		"temporary failure",
		"unexpect",
		"decode",
		"end of JSON",
		"empty response",
	})
}

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SetAPIKey updates the API key
func (c *LLMClient) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// SetBaseURL updates the base URL
func (c *LLMClient) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

// SetModelName updates the model name
func (c *LLMClient) SetModelName(modelName string) {
	c.modelName = modelName
}

// ChatCompletionWithMessages performs a chat completion with full message history
func (c *LLMClient) ChatCompletionWithMessages(ctx context.Context, messages []Message, temperature float64) (string, error) {
	reqBody := ChatCompletionRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   4096,
	}

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		response, err := c.sendRequest(ctx, reqBody)
		if err == nil && response != "" {
			return response, nil
		}

		if err == nil && response == "" {
			logger.Warn("LLM returned empty response, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", c.maxRetries))
			lastErr = fmt.Errorf("empty response from LLM")
		} else {
			lastErr = err
		}

		isRetryable := c.isRetryableError(lastErr) || lastErr.Error() == "empty response from LLM"
		if !isRetryable {
			logger.Error("LLM request failed with non-retryable error",
				zap.Error(lastErr),
				zap.Int("attempt", attempt+1),
			)
			return "", lastErr
		}

		backoff := time.Duration(1<<uint(attempt)) * time.Second
		logger.Warn("LLM request failed, retrying with backoff",
			zap.Error(lastErr),
			zap.Int("attempt", attempt+1),
			zap.Duration("backoff", backoff),
		)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
			continue
		}
	}

	return "", fmt.Errorf("LLM request failed after %d attempts: %w", c.maxRetries, lastErr)
}

// ChatCompletionStreamWithMessages performs a streaming chat completion with full message history
func (c *LLMClient) ChatCompletionStreamWithMessages(ctx context.Context, messages []Message, temperature float64) (*ChatStream, error) {
	if c.usesAnthropicAPI() {
		return c.sendAnthropicStream(ctx, c.buildAnthropicRequest(messages, temperature, 2048, true))
	}

	reqBody := c.prepareRequest(ChatCompletionRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   4096,
		Stream:      true,
	})

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.chatCompletionsURL()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	logger.Info("streaming response", zap.Int("status", resp.StatusCode), zap.Any("headers", resp.Header))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return NewChatStream(resp), nil
}

func (c *LLMClient) sendAnthropicStream(ctx context.Context, reqBody anthropicMessageRequest) (*ChatStream, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.anthropicMessagesURL(), bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	logger.Info("anthropic streaming response", zap.Int("status", resp.StatusCode), zap.Any("headers", resp.Header))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return NewChatStream(resp), nil
}

// ChatCompletionStream performs a streaming chat completion
func (c *LLMClient) ChatCompletionStream(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (*ChatStream, error) {
	var messages []Message
	if systemPrompt != "" {
		messages = []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	} else {
		messages = []Message{
			{Role: "user", Content: userPrompt},
		}
	}

	if c.usesAnthropicAPI() {
		return c.ChatCompletionStreamWithMessages(ctx, messages, temperature)
	}

	reqBody := c.prepareRequest(ChatCompletionRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   4096,
		Stream:      true,
	})

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.chatCompletionsURL()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return NewChatStream(resp), nil
}
