package service

import (
	"context"
	"fmt"
	"time"

	"api-server/internal/model"
	"api-server/internal/queue"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"go.uber.org/zap"
)

// RuleService manages rule lifecycle and distribution
type RuleService struct {
	ruleRepo *repository.SigmaRuleRepository
	producer *queue.KafkaProducer
	logger   *zap.Logger
}

// NewRuleService creates a new rule service
func NewRuleService(ruleRepo *repository.SigmaRuleRepository, producer *queue.KafkaProducer) *RuleService {
	return &RuleService{
		ruleRepo: ruleRepo,
		producer: producer,
		logger:   logger.Logger,
	}
}

// DistributeAllRules sends all active/experimental rules to agents
func (s *RuleService) DistributeAllRules(ctx context.Context) error {
	rules, _, err := s.ruleRepo.List(1, 10000, map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("failed to list rules: %w", err)
	}

	count := 0
	for _, rule := range rules {
		if rule.Status == "active" || rule.Status == "experimental" {
			if err := s.publishRuleUpdate(ctx, "add", &rule); err != nil {
				s.logger.Error("failed to publish rule",
					zap.String("rule_id", rule.RuleID),
					zap.Error(err),
				)
			} else {
				count++
			}
		}
	}

	s.logger.Info("distributed rules", zap.Int("count", count))
	return nil
}

// DistributeRuleChange publishes a single rule change
func (s *RuleService) DistributeRuleChange(ctx context.Context, action string, rule *model.SigmaRule) error {
	return s.publishRuleUpdate(ctx, action, rule)
}

// publishRuleUpdate sends a rule update to Kafka
func (s *RuleService) publishRuleUpdate(ctx context.Context, action string, rule *model.SigmaRule) error {
	update := map[string]interface{}{
		"action":    action,
		"rule_id":   rule.RuleID,
		"content":   rule.Content,
		"status":    rule.Status,
		"mitre_id":  rule.MitreID,
		"severity":  rule.Severity,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	return s.producer.SendMessage(ctx, "rule-updates", rule.RuleID, update)
}

// CheckAndActivatePending activates pending rules after 24 hours
func (s *RuleService) CheckAndActivatePending() error {
	rules, _, err := s.ruleRepo.List(1, 10000, map[string]interface{}{"status": "pending"})
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if time.Since(rule.CreatedAt) >= 24*time.Hour {
			if err := s.ruleRepo.UpdateStatus(rule.RuleID, "experimental"); err != nil {
				s.logger.Error("failed to activate pending rule",
					zap.String("rule_id", rule.RuleID),
					zap.Error(err),
				)
			} else {
				s.logger.Info("pending rule activated as experimental",
					zap.String("rule_id", rule.RuleID),
				)
				s.DistributeRuleChange(context.Background(), "update", &rule)
			}
		}
	}

	return nil
}

// CheckAndPromoteExperimental promotes experimental rules after 7 days
func (s *RuleService) CheckAndPromoteExperimental() error {
	rules, err := s.ruleRepo.GetActiveAndExperimental()
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if rule.Status == "experimental" && rule.ActivatedAt != nil {
			if time.Since(*rule.ActivatedAt) >= 7*24*time.Hour {
				if err := s.ruleRepo.UpdateStatus(rule.RuleID, "active"); err != nil {
					s.logger.Error("failed to promote rule",
						zap.String("rule_id", rule.RuleID),
						zap.Error(err),
					)
				} else {
					s.logger.Info("rule promoted to active",
						zap.String("rule_id", rule.RuleID),
					)
					s.DistributeRuleChange(context.Background(), "update", &rule)
				}
			}
		}
	}

	return nil
}
