package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
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
	Message Message `json:"message"`
}

type Error struct {
	Message string `json:"message"`
	Code    string `json:"code"`
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

func (c *LLMClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error) {
	reqBody := ChatCompletionRequest{
		Model: c.modelName,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
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
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
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
			logger.Warn("LLM rate limited, waiting for Retry-After",
				zap.String("retry_after", retryAfter),
			)
			time.Sleep(time.Duration(len(retryAfter)) * time.Second)
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

	return completionResp.Choices[0].Message.Content, nil
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
