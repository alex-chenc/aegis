package pipeline

import (
	"dc/internal/model"
	"encoding/json"
	"fmt"
	"strings"
)

type LLMResponseParser struct{}

func NewLLMResponseParser() *LLMResponseParser {
	return &LLMResponseParser{}
}

// Parse parses an LLM response into an LLMAnalysisResult
func (p *LLMResponseParser) Parse(response string) (*model.LLMAnalysisResult, error) {
	// Try to extract JSON from the response
	jsonStr := p.extractJSON(response)
	if jsonStr == "" {
		// If no JSON found, create a result from plain text
		return &model.LLMAnalysisResult{
			Summary:         response,
			Severity:        "medium",
			AnalysisDetails: response,
		}, nil
	}

	var result model.LLMAnalysisResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &result, nil
}

// extractJSON extracts JSON from a string that may contain extra text
func (p *LLMResponseParser) extractJSON(s string) string {
	// Find the first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")

	if start == -1 || end == -1 || end < start {
		return ""
	}

	return s[start : end+1]
}
