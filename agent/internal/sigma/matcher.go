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
	Selectors map[string]map[string][]PatternMatcher
	Condition string
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
		Selectors: make(map[string]map[string][]PatternMatcher),
		Condition: strings.TrimSpace(rule.Detection.Condition),
	}

	for key, val := range rule.Detection.Selections {
		if key == "condition" {
			continue
		}
		if m, ok := val.(map[string]interface{}); ok {
			selector := make(map[string][]PatternMatcher)
			for field, values := range m {
				fieldKey := normalizeFieldName(field)
				isRegex := strings.Contains(field, "|re")

				if list, ok := values.([]interface{}); ok {
					for _, v := range list {
						if s, ok := v.(string); ok {
							pm := compilePattern(s, isRegex)
							cr.Matchers[fieldKey] = append(cr.Matchers[fieldKey], pm)
							selector[fieldKey] = append(selector[fieldKey], pm)
						}
					}
				} else if s, ok := values.(string); ok {
					pm := compilePattern(s, isRegex)
					cr.Matchers[fieldKey] = append(cr.Matchers[fieldKey], pm)
					selector[fieldKey] = append(selector[fieldKey], pm)
				}
			}
			cr.Selectors[key] = selector
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
	if cr.Condition != "" {
		matched, ok := cr.evaluateCondition(event)
		return ok && matched
	}

	for selectorName := range cr.Selectors {
		if cr.selectorMatches(selectorName, event) {
			return true
		}
	}
	return false
}

func (cr *CompiledRule) selectorMatches(selectorName string, event map[string]interface{}) bool {
	selector, ok := cr.Selectors[selectorName]
	if !ok || len(selector) == 0 {
		return false
	}

	for field, patterns := range selector {
		eventVal := ""
		if v, ok := event[field]; ok {
			eventVal = strings.ToLower(fmt.Sprint(v))
		}
		if eventVal == "" {
			return false
		}
		fieldMatched := false
		for _, pm := range patterns {
			if pm.IsRegex && pm.Regex != nil {
				if pm.Regex.MatchString(eventVal) {
					fieldMatched = true
					break
				}
			} else {
				if strings.Contains(eventVal, strings.ToLower(pm.Pattern)) {
					fieldMatched = true
					break
				}
			}
		}
		if !fieldMatched {
			return false
		}
	}
	return true
}

func (cr *CompiledRule) evaluateCondition(event map[string]interface{}) (bool, bool) {
	parser := &conditionParser{
		rule:   cr,
		event:  event,
		tokens: tokenizeCondition(cr.Condition),
	}
	if len(parser.tokens) == 0 {
		return false, false
	}
	result, ok := parser.parseExpression()
	if !ok || parser.pos != len(parser.tokens) {
		return false, false
	}
	return result, true
}

func tokenizeCondition(condition string) []string {
	condition = strings.ReplaceAll(condition, "(", " ( ")
	condition = strings.ReplaceAll(condition, ")", " ) ")
	return strings.Fields(condition)
}

type conditionParser struct {
	rule   *CompiledRule
	event  map[string]interface{}
	tokens []string
	pos    int
}

func (p *conditionParser) parseExpression() (bool, bool) {
	left, ok := p.parseTerm()
	if !ok {
		return false, false
	}
	for p.match("or") {
		right, ok := p.parseTerm()
		if !ok {
			return false, false
		}
		left = left || right
	}
	return left, true
}

func (p *conditionParser) parseTerm() (bool, bool) {
	left, ok := p.parseFactor()
	if !ok {
		return false, false
	}
	for p.match("and") {
		right, ok := p.parseFactor()
		if !ok {
			return false, false
		}
		left = left && right
	}
	return left, true
}

func (p *conditionParser) parseFactor() (bool, bool) {
	if p.match("not") {
		value, ok := p.parseFactor()
		return !value, ok
	}
	return p.parsePrimary()
}

func (p *conditionParser) parsePrimary() (bool, bool) {
	if p.match("(") {
		value, ok := p.parseExpression()
		if !ok || !p.match(")") {
			return false, false
		}
		return value, true
	}

	token, ok := p.next()
	if !ok {
		return false, false
	}

	switch strings.ToLower(token) {
	case "all":
		if !p.match("of") {
			return false, false
		}
		pattern, ok := p.next()
		if !ok {
			return false, false
		}
		return p.rule.matchSelectorPattern(pattern, p.event, true)
	case "1":
		if !p.match("of") {
			return false, false
		}
		pattern, ok := p.next()
		if !ok {
			return false, false
		}
		return p.rule.matchSelectorPattern(pattern, p.event, false)
	case "and", "or", "of", ")":
		return false, false
	default:
		if strings.Contains(token, "*") {
			return p.rule.matchSelectorPattern(token, p.event, false)
		}
		if _, exists := p.rule.Selectors[token]; !exists {
			return false, false
		}
		return p.rule.selectorMatches(token, p.event), true
	}
}

func (p *conditionParser) match(expected string) bool {
	if p.pos >= len(p.tokens) {
		return false
	}
	if strings.EqualFold(p.tokens[p.pos], expected) {
		p.pos++
		return true
	}
	return false
}

func (p *conditionParser) next() (string, bool) {
	if p.pos >= len(p.tokens) {
		return "", false
	}
	token := p.tokens[p.pos]
	p.pos++
	return token, true
}

func (cr *CompiledRule) matchSelectorPattern(pattern string, event map[string]interface{}, requireAll bool) (bool, bool) {
	matchedSelectors := 0
	matchedEvents := 0
	for selectorName := range cr.Selectors {
		if !selectorNameMatches(pattern, selectorName) {
			continue
		}
		matchedSelectors++
		if cr.selectorMatches(selectorName, event) {
			matchedEvents++
		}
	}
	if matchedSelectors == 0 {
		return false, false
	}
	if requireAll {
		return matchedEvents == matchedSelectors, true
	}
	return matchedEvents > 0, true
}

func selectorNameMatches(pattern, selectorName string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == selectorName
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		return strings.HasPrefix(selectorName, parts[0]) && strings.HasSuffix(selectorName, parts[1])
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
