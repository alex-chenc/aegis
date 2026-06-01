package dynpkg

import (
	"aegis-agent/internal/sigma"
)

// SigmaMatcherAdapter wraps sigma.Loader to implement the SigmaMatcher interface
type SigmaMatcherAdapter struct {
	loader *sigma.Loader
}

func NewSigmaMatcherAdapter(loader *sigma.Loader) *SigmaMatcherAdapter {
	return &SigmaMatcherAdapter{loader: loader}
}

func (a *SigmaMatcherAdapter) Match(event map[string]interface{}) []SigmaMatch {
	if a.loader == nil {
		return nil
	}

	compiledRules := a.loader.MatchAll(event)
	matches := make([]SigmaMatch, 0, len(compiledRules))
	for _, rule := range compiledRules {
		matches = append(matches, SigmaMatch{
			RuleID:    rule.ID,
			Title:     rule.Title,
			Severity:  rule.Severity,
			MitreID:   rule.MitreID,
			EventType: rule.Logsource.Category,
		})
	}
	return matches
}

func (a *SigmaMatcherAdapter) AddRules(rules []byte) error {
	if a.loader == nil {
		return nil
	}
	return a.loader.ApplyUpdate("add", "", rules)
}

func (a *SigmaMatcherAdapter) RemoveRules(ruleIDs []string) {
	if a.loader == nil {
		return
	}
	for _, id := range ruleIDs {
		a.loader.ApplyUpdate("delete", id, nil)
	}
}
