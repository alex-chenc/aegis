package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"baseline-system/internal/model"

	"baseline-system/pkg/logger"

	"go.uber.org/zap"
)

type ExtractedRule struct {
	Title        string `json:"title"`
	CheckContent string `json:"check_content"`
	FixContent   string `json:"fix_content"`
}

// ParseRules parses LLM response and extracts baseline rules
func ParseRules(llmResponse string) ([]*model.BaselineRule, error) {
	// Try to extract JSON array from response
	jsonStr := extractJSON(llmResponse)
	if jsonStr == "" {
		logger.Error("failed to extract JSON from LLM response",
			zap.String("response", truncate(llmResponse, 200)),
		)
		return nil, fmt.Errorf("invalid LLM response format")
	}

	var extractedRules []ExtractedRule
	if err := json.Unmarshal([]byte(jsonStr), &extractedRules); err != nil {
		logger.Error("failed to unmarshal rules",
			zap.Error(err),
			zap.String("json", truncate(jsonStr, 500)),
		)
		return nil, fmt.Errorf("failed to parse rules: %w", err)
	}

	// Convert to model.BaselineRule
	rules := make([]*model.BaselineRule, len(extractedRules))
	for i, er := range extractedRules {
		rules[i] = &model.BaselineRule{
			Title:        strings.TrimSpace(er.Title),
			CheckContent: strings.TrimSpace(er.CheckContent),
			FixContent:   strings.TrimSpace(er.FixContent),
			ScriptStatus: "pending",
		}
	}

	logger.Info("successfully parsed rules from LLM response",
		zap.Int("count", len(rules)),
	)

	return rules, nil
}

// ParseScript parses LLM response and extracts generated script
func ParseScript(llmResponse string) (string, error) {
	script := strings.TrimSpace(llmResponse)

	if script == "" {
		logger.Error("LLM returned empty script")
		return "", fmt.Errorf("empty script")
	}

	if !strings.HasPrefix(script, "#!") {
		logger.Info("adding shebang to script without one",
			zap.String("first_chars", truncate(script, 20)),
		)
		script = "#!/bin/bash\n" + script
	}

	logger.Debug("successfully parsed script",
		zap.Int("length", len(script)),
	)

	return script, nil
}

// ValidateRule validates extracted rule for required fields
func ValidateRule(rule *ExtractedRule) error {
	if strings.TrimSpace(rule.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(rule.CheckContent) == "" {
		return fmt.Errorf("check_content is required")
	}
	if strings.TrimSpace(rule.FixContent) == "" {
		return fmt.Errorf("fix_content is required")
	}
	return nil
}

// ValidateRules deduplicates rules by title
func ValidateRules(rules []*model.BaselineRule) []*model.BaselineRule {
	seen := make(map[string]bool)
	validated := make([]*model.BaselineRule, 0, len(rules))

	for _, rule := range rules {
		if rule.Title == "" {
			continue
		}
		if !seen[rule.Title] {
			seen[rule.Title] = true
			validated = append(validated, rule)
		} else {
			logger.Debug("skipping duplicate rule",
				zap.String("title", rule.Title),
			)
		}
	}

	return validated
}

// extractJSON tries to extract JSON array/object from LLM response
func extractJSON(response string) string {
	// Find first [ or {
	start := -1
	for i, c := range response {
		if c == '[' || c == '{' {
			start = i
			break
		}
	}

	if start == -1 {
		return ""
	}

	// Find matching ] or }
	bracket := response[start]
	closeBracket := getCloseBracket(bracket)
	count := 0

	for i := start; i < len(response); i++ {
		if response[i] == bracket {
			count++
		} else if response[i] == closeBracket {
			count--
			if count == 0 {
				return response[start : i+1]
			}
		}
	}

	return ""
}

func getCloseBracket(b byte) byte {
	switch b {
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	case '(':
		return ')'
	default:
		return b
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
