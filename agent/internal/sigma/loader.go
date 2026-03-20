package sigma

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

type Loader struct {
	mu      sync.RWMutex
	rules   map[string]*Rule
	index   *RuleIndex
	ruleDir string
}

func NewLoader(ruleDir string) *Loader {
	return &Loader{
		rules:   make(map[string]*Rule),
		index:   NewRuleIndex(),
		ruleDir: ruleDir,
	}
}

func (l *Loader) LoadAll(rules []Rule) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rules = make(map[string]*Rule)
	for _, rule := range rules {
		r := rule
		l.rules[r.ID] = &r
	}
	l.index.Rebuild(l.rules)

	logger.Info("Sigma rules loaded", zap.Int("count", len(l.rules)))
	return nil
}

func (l *Loader) ApplyUpdate(action, ruleID string, content []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	switch action {
	case "add", "update":
		rule, err := ParseRule(content)
		if err != nil {
			return err
		}
		l.rules[ruleID] = rule
		logger.Info("Sigma rule updated", zap.String("rule_id", ruleID), zap.String("action", action))
	case "delete":
		delete(l.rules, ruleID)
		logger.Info("Sigma rule deleted", zap.String("rule_id", ruleID))
	}
	l.index.Rebuild(l.rules)
	return nil
}

func (l *Loader) MatchAll(event map[string]interface{}) []*CompiledRule {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.index.Match(event)
}

func (l *Loader) RuleCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.rules)
}

func (l *Loader) SaveRuleToDisk(ruleID string, content []byte) error {
	path := filepath.Join(l.ruleDir, ruleID+".yml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create rule dir: %w", err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to save rule to disk: %w", err)
	}
	return nil
}
