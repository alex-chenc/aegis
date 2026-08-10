package agentsession

import (
	"encoding/json"
	"regexp"
	"strings"
)

type Redactor struct {
	patterns []*regexp.Regexp
}

func NewRedactor() *Redactor {
	return &Redactor{patterns: []*regexp.Regexp{
		regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer\s+)?|cookie\s*[:=]\s*|(?:api[_-]?key|token|secret|password|passwd)\s*[:=]\s*)([^\s,;"']+)`),
		regexp.MustCompile(`\b(?:sk|ghp|github_pat|xox[baprs])_[A-Za-z0-9_\-]{12,}\b`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	}}
}

func (r *Redactor) Text(input string) (string, int) {
	if r == nil || input == "" {
		return input, 0
	}
	count := 0
	output := input
	for _, pattern := range r.patterns {
		output = pattern.ReplaceAllStringFunc(output, func(match string) string {
			count++
			if strings.Contains(match, "PRIVATE KEY") {
				return "[REDACTED:PRIVATE_KEY]"
			}
			if strings.Contains(strings.ToLower(match), "authorization") {
				return "[REDACTED:AUTHORIZATION]"
			}
			if strings.Contains(strings.ToLower(match), "cookie") {
				return "[REDACTED:COOKIE]"
			}
			return "[REDACTED:SECRET]"
		})
	}
	return output, count
}

func (r *Redactor) JSONValue(value any) (any, int) {
	switch typed := value.(type) {
	case string:
		return r.Text(typed)
	case []any:
		result := make([]any, len(typed))
		count := 0
		for i, item := range typed {
			result[i], _ = r.JSONValue(item)
			if s, ok := result[i].(string); ok && s != typed[i] {
				count++
			}
		}
		return result, count
	case map[string]any:
		result := make(map[string]any, len(typed))
		count := 0
		for key, item := range typed {
			redacted, itemCount := r.JSONValue(item)
			result[key] = redacted
			count += itemCount
		}
		return result, count
	default:
		return value, 0
	}
}

func redactJSON(r *Redactor, value any) (string, int) {
	redacted, count := r.JSONValue(value)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "[REDACTED:SERIALIZATION_ERROR]", count
	}
	return string(encoded), count
}
