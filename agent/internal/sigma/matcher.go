package sigma

import (
	"fmt"
	"strings"
)

type CompiledRule struct {
	ID        string
	Title     string
	MitreID   string
	Severity  string
	Logsource Logsource
	Matchers  map[string][]string
}

func CompileRule(rule *Rule) *CompiledRule {
	cr := &CompiledRule{
		ID:        rule.ID,
		Title:     rule.Title,
		MitreID:   extractMitreID(rule.Tags),
		Severity:  rule.Level,
		Logsource: rule.Logsource,
		Matchers:  make(map[string][]string),
	}

	for key, val := range rule.Detection.Selections {
		if key == "condition" {
			continue
		}
		if m, ok := val.(map[string]interface{}); ok {
			for field, values := range m {
				if list, ok := values.([]interface{}); ok {
					for _, v := range list {
						if s, ok := v.(string); ok {
							fieldKey := strings.ToLower(field)
							cr.Matchers[fieldKey] = append(cr.Matchers[fieldKey], s)
						}
					}
				} else if s, ok := values.(string); ok {
					fieldKey := strings.ToLower(field)
					cr.Matchers[fieldKey] = append(cr.Matchers[fieldKey], s)
				}
			}
		}
	}

	return cr
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
		for _, pattern := range patterns {
			if strings.Contains(eventVal, strings.ToLower(pattern)) {
				return true
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
