package checker

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"api-server/internal/model"
	"api-server/pkg/logger"

	"go.uber.org/zap"
)

const (
	MaxRegexLength    = 1000
	MaxRegexMatchTime = 10 * time.Millisecond
)

type CompiledRule struct {
	ID         string
	Name       string
	RuleType   string
	MatchType  string
	Pattern    string
	CompiledRe *regexp.Regexp
	Category   string
	Severity   string
	AppliesTo  []string
	IsEnabled  bool
}

func (r *CompiledRule) AppliesToType(scriptType string) bool {
	for _, t := range r.AppliesTo {
		if t == "all" || t == scriptType {
			return true
		}
	}
	return false
}

type BlacklistHit struct {
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	Pattern     string `json:"pattern"`
	MatchedText string `json:"matched_text"`
	LineNumber  int    `json:"line_number"`
	Severity    string `json:"severity"`
	RuleType    string `json:"rule_type"`
}

type CheckResult struct {
	HasViolation bool           `json:"has_violation"`
	Hits         []BlacklistHit `json:"hits"`
}

func (r *CheckResult) HasHardBlock() bool {
	for _, hit := range r.Hits {
		if hit.RuleType == "hard_block" {
			return true
		}
	}
	return false
}

type BlacklistChecker struct {
	rules      []CompiledRule
	regexCache map[string]*regexp.Regexp
	mu         sync.RWMutex
}

func NewBlacklistChecker() *BlacklistChecker {
	return &BlacklistChecker{
		regexCache: make(map[string]*regexp.Regexp),
	}
}

func (c *BlacklistChecker) LoadRules(rules []model.CommandAuditRule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	compiled := make([]CompiledRule, 0, len(rules))
	c.regexCache = make(map[string]*regexp.Regexp)

	for _, rule := range rules {
		cr := CompiledRule{
			ID:        rule.ID.String(),
			Name:      rule.Name,
			RuleType:  rule.RuleType,
			MatchType: rule.MatchType,
			Pattern:   rule.Pattern,
			Category:  rule.Category,
			Severity:  rule.Severity,
			AppliesTo: []string(rule.AppliesTo),
			IsEnabled: rule.IsEnabled,
		}
		if len(cr.AppliesTo) == 0 {
			cr.AppliesTo = []string{"all"}
		}

		if rule.MatchType == "regex" {
			re, err := c.validateRegex(rule.Pattern)
			if err != nil {
				logger.Warn("skipping invalid regex rule",
					zap.String("rule_id", rule.ID.String()),
					zap.String("pattern", rule.Pattern),
					zap.Error(err),
				)
				continue
			}
			cr.CompiledRe = re
			c.regexCache[rule.ID.String()] = re
		}

		compiled = append(compiled, cr)
	}

	c.rules = compiled
	logger.Info("blacklist rules loaded", zap.Int("count", len(compiled)))
	return nil
}

func (c *BlacklistChecker) Check(content string, scriptType string) (*CheckResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lines := strings.Split(content, "\n")
	result := &CheckResult{}

	for _, rule := range c.rules {
		if !rule.IsEnabled {
			continue
		}
		if !rule.AppliesToType(scriptType) {
			continue
		}
		for lineNum, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			matched := false
			var matchedText string

			switch rule.MatchType {
			case "exact":
				if strings.Contains(line, rule.Pattern) {
					matched = true
					matchedText = rule.Pattern
				}
			case "regex":
				if rule.CompiledRe != nil {
					loc := rule.CompiledRe.FindStringIndex(line)
					if loc != nil {
						matched = true
						matchedText = line[loc[0]:loc[1]]
					}
				}
			}

			if matched {
				result.HasViolation = true
				result.Hits = append(result.Hits, BlacklistHit{
					RuleID:      rule.ID,
					RuleName:    rule.Name,
					Pattern:     rule.Pattern,
					MatchedText: matchedText,
					LineNumber:  lineNum + 1,
					Severity:    rule.Severity,
					RuleType:    rule.RuleType,
				})
				if rule.RuleType == "hard_block" {
					return result, nil
				}
			}
		}
	}
	return result, nil
}

func (c *BlacklistChecker) validateRegex(pattern string) (*regexp.Regexp, error) {
	if len(pattern) > MaxRegexLength {
		return nil, fmt.Errorf("regex length exceeds limit (%d)", MaxRegexLength)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regex compile failed: %w", err)
	}
	start := time.Now()
	re.MatchString("test string for performance check")
	if time.Since(start) > MaxRegexMatchTime {
		return nil, fmt.Errorf("regex match timeout, potential ReDoS risk")
	}
	return re, nil
}
