package sigma

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
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
	Pattern    string
	IsRegex    bool
	Regex      *regexp.Regexp
	StartsWith bool
	EndsWith   bool
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
		fmt.Printf("[SigmaCompile] selector=%q type=%T\n", key, val)
		if m, ok := val.(map[string]interface{}); ok {
			selector := make(map[string][]PatternMatcher)
			for field, values := range m {
				fieldKey := normalizeFieldName(field)
				isRegex := strings.Contains(field, "|re")
				isStartsWith := strings.Contains(field, "|startswith")
				isEndsWith := strings.Contains(field, "|endswith")
				logger.Debug("[SigmaCompile] field modifier",
					zap.String("selector", key),
					zap.String("field", field),
					zap.String("normalized", fieldKey),
					zap.Bool("isEndsWith", isEndsWith),
					zap.Bool("isStartsWith", isStartsWith),
					zap.Bool("isRegex", isRegex))

				if list, ok := values.([]interface{}); ok {
					for _, v := range list {
						s := fmt.Sprintf("%v", v)
						pm := compilePattern(s, isRegex)
						if isStartsWith {
							pm.StartsWith = true
						}
						if isEndsWith {
							pm.EndsWith = true
						}
						cr.Matchers[fieldKey] = append(cr.Matchers[fieldKey], pm)
						selector[fieldKey] = append(selector[fieldKey], pm)
					}
				} else {
					// Handle string, int, float64, bool, and any other scalar types
					s := fmt.Sprintf("%v", values)
					pm := compilePattern(s, isRegex)
					if isStartsWith {
						pm.StartsWith = true
					}
					if isEndsWith {
						pm.EndsWith = true
					}
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

func matchValue(eventVal string, eventRaw interface{}, pm PatternMatcher) bool {
	if pm.IsRegex && pm.Regex != nil {
		return pm.Regex.MatchString(eventVal)
	}
	if pm.StartsWith {
		return strings.HasPrefix(eventVal, strings.ToLower(pm.Pattern))
	}
	if pm.EndsWith {
		result := strings.HasSuffix(eventVal, strings.ToLower(pm.Pattern))
		fmt.Printf("[SigmaMatch] EndsWith check: eventVal=%q pattern=%q result=%v\n", eventVal, pm.Pattern, result)
		return result
	}
	// Try numeric comparison for integer fields
	if eventRaw != nil {
		if isNumericType(eventRaw) && isNumericPattern(pm.Pattern) {
			result := numericMatch(eventRaw, pm.Pattern)
			fmt.Printf("[SigmaMatch] NumericMatch: eventRaw=%v(%T) pattern=%q result=%v\n", eventRaw, eventRaw, pm.Pattern, result)
			return result
		}
	}
	return strings.Contains(eventVal, strings.ToLower(pm.Pattern))
}

func isNumericType(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

func isNumericPattern(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func numericMatch(eventRaw interface{}, pattern string) bool {
	patternVal, err := strconv.ParseFloat(pattern, 64)
	if err != nil {
		return false
	}
	eventVal := toFloat64(eventRaw)
	return eventVal == patternVal
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float64:
		return val
	case float32:
		return float64(val)
	}
	return 0
}

func normalizeFieldName(field string) string {
	// Strip Sigma modifiers like |re, |contains, etc.
	if idx := strings.Index(field, "|"); idx != -1 {
		field = field[:idx]
	}
	return strings.ToLower(field)
}

func (cr *CompiledRule) Match(event map[string]interface{}) bool {
	// Debug: log match attempt
	fmt.Printf("[SigmaMatch] rule=%s condition=%q selectors=%v event_keys=%v\n", cr.ID, cr.Condition, selectorNames(cr.Selectors), eventKeys(event))
	if cr.Condition != "" {
		matched, ok := cr.evaluateCondition(event)
		fmt.Printf("[SigmaMatch] rule=%s condition_result=%v ok=%v\n", cr.ID, matched, ok)
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
	logger.Debug("[SigmaMatch] selectorMatches called",
		zap.String("selector", selectorName),
		zap.Bool("found", ok),
		zap.Int("fields", len(selector)))
	if !ok || len(selector) == 0 {
		return false
	}

	for field, patterns := range selector {
		eventVal := ""
		var eventRaw interface{}
		// Try case-insensitive lookup
		if v, found := lookupFieldCaseInsensitive(event, field); found {
			eventRaw = v
			eventVal = strings.ToLower(fmt.Sprint(v))
			logger.Debug("[SigmaMatch] field lookup",
				zap.String("selector", selectorName),
				zap.String("field", field),
				zap.Any("raw", eventRaw),
				zap.String("eventVal", eventVal))
		}
		if eventVal == "" {
			logger.Debug("[SigmaMatch] field NOT_FOUND",
				zap.String("selector", selectorName),
				zap.String("field", field))
			return false
		}
		fieldMatched := false
		for _, pm := range patterns {
			if matchValue(eventVal, eventRaw, pm) {
				fieldMatched = true
				break
			}
		}
		if !fieldMatched {
			fmt.Printf("[SigmaMatch] selector=%s field=%q eventVal=%q patterns=%v NO_MATCH\n", selectorName, field, eventVal, patterns)
			return false
		}
	}
	return true
}

// lookupFieldCaseInsensitive does case-insensitive field name lookup in event map
func lookupFieldCaseInsensitive(event map[string]interface{}, field string) (interface{}, bool) {
	// Try exact match first (fast path)
	if v, ok := event[field]; ok {
		return v, true
	}
	// Try case-insensitive match
	lowerField := strings.ToLower(field)
	for k, v := range event {
		if strings.ToLower(k) == lowerField {
			return v, true
		}
	}
	return nil, false
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
		lower := strings.ToLower(tag)
		if strings.HasPrefix(lower, "attack.t") {
			id := strings.TrimPrefix(lower, "attack.")
			return strings.ToUpper(id)
		}
	}
	return ""
}


func selectorNames(selectors map[string]map[string][]PatternMatcher) []string {
	var names []string
	for k := range selectors {
		names = append(names, k)
	}
	return names
}

func eventKeys(event map[string]interface{}) []string {
	var keys []string
	for k := range event {
		keys = append(keys, k)
	}
	return keys
}
