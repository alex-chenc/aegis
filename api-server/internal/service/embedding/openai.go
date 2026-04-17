package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Service generates text embeddings using OpenAI's embedding models
type Service struct {
	apiKey  string
	baseURL string
	model   string
}

// NewService creates a new embedding service
func NewService(apiKey, baseURL string) *Service {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Service{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   "text-embedding-3-small",
	}
}

// Generate generates an embedding for the given text
func (s *Service) Generate(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"model": s.model,
		"input": text,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", s.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return result.Data[0].Embedding, nil
}

// Dimension returns the dimension of the embedding model
func (s *Service) Dimension() int {
	return 1536
}
