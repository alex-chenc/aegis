package checker

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

type Rule struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	RuleType   string   `json:"rule_type"`
	MatchType  string   `json:"match_type"`
	Pattern    string   `json:"pattern"`
	Category   string   `json:"category"`
	Severity   string   `json:"severity"`
	AppliesTo  []string `json:"applies_to"`
	IsEnabled  bool     `json:"is_enabled"`
	compiledRe *regexp.Regexp
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

type BlacklistChecker struct {
	rules []Rule
	mu    sync.RWMutex
}

func NewBlacklistChecker(rulesPath string) (*BlacklistChecker, error) {
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules file: %w", err)
	}

	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse rules: %w", err)
	}

	c := &BlacklistChecker{}
	for _, rule := range rules {
		if !rule.IsEnabled {
			continue
		}
		if rule.MatchType == "regex" {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				continue // skip invalid regex
			}
			rule.compiledRe = re
		}
		c.rules = append(c.rules, rule)
	}
	return c, nil
}

func (c *BlacklistChecker) RuleCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.rules)
}

func (c *BlacklistChecker) Check(content string) *CheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lines := strings.Split(content, "\n")
	result := &CheckResult{}

	for _, rule := range c.rules {
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
				if rule.compiledRe != nil {
					loc := rule.compiledRe.FindStringIndex(line)
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
					return result
				}
			}
		}
	}
	return result
}
