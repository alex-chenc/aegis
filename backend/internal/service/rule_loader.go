package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/pkg/logger"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// SigmaRuleYAML represents a Sigma rule in YAML format
type SigmaRuleYAML struct {
	Title       string `yaml:"title"`
	ID          string `yaml:"id"`
	Status      string `yaml:"status"`
	Description string `yaml:"description"`
	Logsource   struct {
		Category string `yaml:"category"`
		Product  string `yaml:"product"`
	} `yaml:"logsource"`
	Detection struct {
		Selection map[string]interface{} `yaml:"selection"`
		Condition string                 `yaml:"condition"`
	} `yaml:"detection"`
	Level string   `yaml:"level"`
	Tags  []string `yaml:"tags"`
}

// RuleLoader loads Sigma rules from YAML files
type RuleLoader struct {
	ruleRepo *repository.SigmaRuleRepository
	logger   *zap.Logger
}

// NewRuleLoader creates a new rule loader
func NewRuleLoader(ruleRepo *repository.SigmaRuleRepository) *RuleLoader {
	return &RuleLoader{
		ruleRepo: ruleRepo,
		logger:   logger.Logger,
	}
}

// LoadFromDirectory loads all .yml/.yaml rules from a directory
func (l *RuleLoader) LoadFromDirectory(ctx context.Context, dirPath string) error {
	files, err := filepath.Glob(filepath.Join(dirPath, "*.yml"))
	if err != nil {
		return fmt.Errorf("failed to glob rules: %w", err)
	}

	yamlFiles, _ := filepath.Glob(filepath.Join(dirPath, "*.yaml"))
	files = append(files, yamlFiles...)

	count := 0
	for _, file := range files {
		if err := l.LoadFromFile(ctx, file); err != nil {
			l.logger.Error("failed to load rule",
				zap.String("file", file),
				zap.Error(err),
			)
		} else {
			count++
		}
	}

	l.logger.Info("rules loaded from directory",
		zap.String("dir", dirPath),
		zap.Int("count", count),
	)
	return nil
}

// LoadFromFile loads a single Sigma rule from a YAML file
func (l *RuleLoader) LoadFromFile(ctx context.Context, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var rule SigmaRuleYAML
	if err := yaml.Unmarshal(data, &rule); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Check if already exists
	existing, err := l.ruleRepo.FindByID(rule.ID)
	if err == nil && existing != nil {
		l.logger.Debug("rule already exists, skipping", zap.String("rule_id", rule.ID))
		return nil
	}

	// Create rule record
	sigmaRule := &model.SigmaRule{
		RuleID:      rule.ID,
		Title:       rule.Title,
		Description: rule.Description,
		Content:     string(data),
		Status:      "active",
		MitreID:     extractMitreID(rule.Tags),
		Severity:    rule.Level,
		GeneratedBy: "manual",
		Version:     "1.0",
	}

	if err := l.ruleRepo.Create(sigmaRule); err != nil {
		return fmt.Errorf("failed to create rule: %w", err)
	}

	l.logger.Info("rule loaded",
		zap.String("rule_id", rule.ID),
		zap.String("title", rule.Title),
	)

	return nil
}

// extractMitreID extracts MITRE ATT&CK ID from tags
func extractMitreID(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "attack.") {
			return strings.TrimPrefix(tag, "attack.")
		}
	}
	return ""
}
