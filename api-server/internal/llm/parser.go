package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"api-server/internal/model"

	"api-server/pkg/logger"

	"go.uber.org/zap"
)

type ExtractedRule struct {
	Title        string `json:"title"`
	CheckContent string `json:"check_content"`
	FixContent   string `json:"fix_content"`
}

// ParseRules parses LLM response and extracts aegis rules
func ParseRules(llmResponse string) ([]*model.AegisRule, error) {
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
		repairedJSON := repairInvalidJSONStringEscapes(jsonStr)
		if repairedJSON == jsonStr {
			logger.Error("failed to unmarshal rules",
				zap.Error(err),
				zap.String("json", truncate(jsonStr, 500)),
			)
			return nil, fmt.Errorf("failed to parse rules: %w", err)
		}
		if retryErr := json.Unmarshal([]byte(repairedJSON), &extractedRules); retryErr != nil {
			logger.Error("failed to unmarshal repaired rules",
				zap.Error(retryErr),
				zap.NamedError("original_error", err),
				zap.String("json", truncate(repairedJSON, 500)),
			)
			return nil, fmt.Errorf("failed to parse rules: %w", retryErr)
		}
		logger.Warn("repaired invalid JSON escapes in LLM rule response",
			zap.Error(err),
		)
	}

	// Convert to model.AegisRule
	rules := make([]*model.AegisRule, len(extractedRules))
	for i, er := range extractedRules {
		rules[i] = &model.AegisRule{
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

	script = removeMarkdownCodeBlock(script)
	script = strings.TrimSpace(script)

	if script == "" {
		logger.Error("Script is empty after removing markdown markers")
		return "", fmt.Errorf("empty script after parsing")
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

func removeMarkdownCodeBlock(script string) string {
	if !strings.HasPrefix(script, "```") {
		return script
	}

	newlineIdx := strings.Index(script, "\n")
	if newlineIdx != -1 {
		script = script[newlineIdx+1:]
	} else {
		script = script[3:]
	}

	if strings.HasSuffix(script, "```") {
		script = script[:len(script)-3]
	}

	return script
}

// ValidateRules deduplicates rules by title
func ValidateRules(rules []*model.AegisRule) []*model.AegisRule {
	seen := make(map[string]bool)
	validated := make([]*model.AegisRule, 0, len(rules))

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

// extractJSON tries to extract JSON array/object from LLM response.
// It first scans for the first top-level [ or {; if none is found it falls
// back to extracting the content of a ```json (or plain ```) fenced code block,
// which models sometimes wrap their JSON in. Leading/trailing whitespace and
// invisible characters are trimmed before scanning.
func extractJSON(response string) string {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return ""
	}

	// Fast path: scan for the first top-level [ or {.
	start := -1
	for i, c := range trimmed {
		if c == '[' || c == '{' {
			start = i
			break
		}
	}

	if start != -1 {
		return scanToMatchingBracket(trimmed, start)
	}

	// Fallback: try to pull JSON out of a fenced code block (```json ... ```).
	if fenced := extractFencedJSON(trimmed); fenced != "" {
		return fenced
	}

	return ""
}

// scanToMatchingBracket extracts the substring from start (which must point at a
// '[' or '{') up to and including its matching closing bracket, honoring strings
// and escape sequences. Returns "" if no balanced match is found.
func scanToMatchingBracket(response string, start int) string {
	bracket := response[start]
	closeBracket := getCloseBracket(bracket)
	count := 0
	inString := false
	escaped := false

	for i := start; i < len(response); i++ {
		c := response[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == bracket {
			count++
		} else if c == closeBracket {
			count--
			if count == 0 {
				return response[start : i+1]
			}
		}
	}

	return ""
}

// extractFencedJSON tries to extract a JSON payload from a ```json or ```
// fenced code block. It returns the trimmed inner content (the caller's
// json.Unmarshal will validate it). Returns "" when no fenced block is found.
func extractFencedJSON(response string) string {
	const fence = "```"
	open := strings.Index(response, fence)
	if open == -1 {
		return ""
	}
	// Skip the opening fence line (e.g. "```json").
	afterOpen := open + len(fence)
	if nl := strings.IndexByte(response[afterOpen:], '\n'); nl != -1 {
		afterOpen += nl + 1
	}
	closeIdx := strings.Index(response[afterOpen:], fence)
	if closeIdx == -1 {
		return ""
	}
	inner := strings.TrimSpace(response[afterOpen : afterOpen+closeIdx])
	if inner == "" {
		return ""
	}
	// The fenced content might still be wrapped in [ ] / { }; let the normal
	// scanner handle bracket balancing.
	if inner[0] == '[' || inner[0] == '{' {
		return scanToMatchingBracket(inner, 0)
	}
	return inner
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

func repairInvalidJSONStringEscapes(jsonStr string) string {
	var b strings.Builder
	b.Grow(len(jsonStr))

	changed := false
	inString := false
	escaped := false

	for i := 0; i < len(jsonStr); i++ {
		c := jsonStr[i]
		if !inString {
			b.WriteByte(c)
			if c == '"' {
				inString = true
			}
			continue
		}

		if escaped {
			b.WriteByte('\\')
			if !isValidJSONEscape(c) {
				b.WriteByte('\\')
				changed = true
			}
			b.WriteByte(c)
			escaped = false
			continue
		}

		switch c {
		case '\\':
			escaped = true
		case '"':
			inString = false
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}

	if escaped {
		b.WriteByte('\\')
	}

	if !changed {
		return jsonStr
	}
	return b.String()
}

func isValidJSONEscape(c byte) bool {
	switch c {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	default:
		return false
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
