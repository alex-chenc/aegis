package sigma

import (
	"fmt"
	"regexp"
	"strings"
)

type CompiledRule struct {
	ID        string
	Title     string
	MitreID   string
	Severity  string
	Logsource Logsource
	Matchers  map[string][]PatternMatcher
}

type PatternMatcher struct {
	Pattern string
	IsRegex bool
	Regex   *regexp.Regexp
}

func CompileRule(rule *Rule) *CompiledRule {
	cr := &CompiledRule{
		ID:        rule.ID,
		Title:     rule.Title,
		MitreID:   extractMitreID(rule.Tags),
		Severity:  rule.Level,
		Logsource: rule.Logsource,
		Matchers:  make(map[string][]PatternMatcher),
	}

	for key, val := range rule.Detection.Selections {
		if key == "condition" {
			continue
		}
		if m, ok := val.(map[string]interface{}); ok {
			for field, values := range m {
				fieldKey := normalizeFieldName(field)
				isRegex := strings.Contains(field, "|re")

				if list, ok := values.([]interface{}); ok {
					for _, v := range list {
						if s, ok := v.(string); ok {
							pm := compilePattern(s, isRegex)
							cr.Matchers[fieldKey] = append(cr.Matchers[fieldKey], pm)
						}
					}
				} else if s, ok := values.(string); ok {
					pm := compilePattern(s, isRegex)
					cr.Matchers[fieldKey] = append(cr.Matchers[fieldKey], pm)
				}
			}
		}
	}

	return cr
}

func compilePattern(pattern string, isRegex bool) PatternMatcher {
	pm := PatternMatcher{
		Pattern: pattern,
		IsRegex: isRegex,
	}

	if isRegex {
		re, err := regexp.Compile(pattern)
		if err == nil {
			pm.Regex = re
		}
	}

	return pm
}

func normalizeFieldName(field string) string {
	// Strip Sigma modifiers like |re, |contains, etc.
	if idx := strings.Index(field, "|"); idx != -1 {
		field = field[:idx]
	}
	return strings.ToLower(field)
}

func (cr *CompiledRule) Match(event map[string]interface{}) bool {
	for field, patterns := range cr.Matchers {
		eventVal := ""
		if v, ok := event[field]; ok {
			eventVal = strings.ToLower(fmt.Sprint(v))
		}
		if eventVal == "" {
			continue
		}
		for _, pm := range patterns {
			if pm.IsRegex && pm.Regex != nil {
				if pm.Regex.MatchString(eventVal) {
					return true
				}
			} else {
				if strings.Contains(eventVal, strings.ToLower(pm.Pattern)) {
					return true
				}
			}
		}
	}
	return false
}

func extractMitreID(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "attack.t") || strings.HasPrefix(tag, "attack.T") {
			return strings.TrimPrefix(strings.TrimPrefix(tag, "attack."), "T")
		}
	}
	return ""
}
