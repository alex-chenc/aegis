package llm

import (
	"bytes"
	"context"
	"dc/config"
	"dc/pkg/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

type Client struct {
	cfg    *config.LLMConfig
	client *http.Client
}

func NewClient(cfg *config.LLMConfig) (*Client, error) {
	return &Client{
		cfg:    cfg,
		// Don't set client.Timeout - rely on context deadline instead
		client: &http.Client{},
	}, nil
}

func (c *Client) isDashScope() bool {
	base := strings.ToLower(c.cfg.BaseURL)
	return strings.Contains(base, "dashscope") || strings.Contains(base, "aliyuncs")
}

func (c *Client) Analyze(ctx context.Context, prompt string) (string, error) {
	if c.cfg.APIKey == "" {
		return "", fmt.Errorf("LLM API key not configured")
	}

	reqBody := map[string]interface{}{
		"model": c.cfg.ModelName,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
	}

	// Enable JSON mode for DashScope when prompt mentions JSON
	if c.isDashScope() && strings.Contains(strings.ToLower(prompt), "json") {
		reqBody["response_format"] = map[string]string{"type": "json_object"}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("unexpected response format")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected choice format")
	}

	message, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected message format")
	}

	content, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected content format")
	}

	logger.Debug("LLM analysis completed",
		zap.String("model", c.cfg.ModelName),
		zap.Int("prompt_length", len(prompt)),
		zap.Int("response_length", len(content)),
	)

	return content, nil
}