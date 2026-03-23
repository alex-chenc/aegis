package sigma

import (
	"os"
	"path/filepath"
	"fmt"
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
		rules, err := ParseRules(content)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			l.rules[rule.ID] = &rule
			logger.Info("Sigma rule added", zap.String("rule_id", rule.ID))
		}
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

// LoadFromDisk loads all rules from the local rule directory
func (l *Loader) LoadFromDisk() error {
	if l.ruleDir == "" {
		return fmt.Errorf("rule directory not set")
	}

	entries, err := os.ReadDir(l.ruleDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("Rule directory does not exist", zap.String("dir", l.ruleDir))
			return nil
		}
		return fmt.Errorf("failed to read rule directory: %w", err)
	}

	var rules []Rule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(l.ruleDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("Failed to read rule file", zap.String("file", path), zap.Error(err))
			continue
		}

		parsed_rules, err := ParseRules(content)
		if err != nil {
			logger.Warn("Failed to parse rule file", zap.String("file", path), zap.Error(err))
			continue
		}
		for _, r := range parsed_rules {
			rules = append(rules, r)
		}
	}

	if len(rules) > 0 {
		return l.LoadAll(rules)
	}

	logger.Info("No rules found on disk")
	return nil
}
