package service

import (
	"time"

	"aegis-system/internal/repository"
	"aegis-system/pkg/logger"

	"go.uber.org/zap"
)

type SigmaRuleService struct {
	ruleRepo *repository.SigmaRuleRepository
}

func NewSigmaRuleService(ruleRepo *repository.SigmaRuleRepository) *SigmaRuleService {
	return &SigmaRuleService{ruleRepo: ruleRepo}
}

func (s *SigmaRuleService) ApproveRule(ruleID string) error {
	return s.ruleRepo.UpdateStatus(ruleID, "active")
}

func (s *SigmaRuleService) DisableRule(ruleID string) error {
	return s.ruleRepo.UpdateStatus(ruleID, "disabled")
}

func (s *SigmaRuleService) CheckAndPromoteRules() {
	rules, err := s.ruleRepo.GetActiveAndExperimental()
	if err != nil {
		logger.Error("failed to get active and experimental rules", zap.Error(err))
		return
	}

	for _, rule := range rules {
		if rule.Status == "experimental" && rule.ActivatedAt != nil {
			if time.Since(*rule.ActivatedAt) >= 7*24*time.Hour {
				if err := s.ruleRepo.UpdateStatus(rule.RuleID, "active"); err != nil {
					logger.Error("failed to promote rule",
						zap.String("rule_id", rule.RuleID),
						zap.Error(err),
					)
				} else {
					logger.Info("rule promoted to active",
						zap.String("rule_id", rule.RuleID),
					)
				}
			}
		}
	}
}

func (s *SigmaRuleService) CheckAndActivatePendingRules() {
	rules, _, err := s.ruleRepo.List(1, 1000, map[string]interface{}{"status": "pending"})
	if err != nil {
		logger.Error("failed to get pending rules", zap.Error(err))
		return
	}

	for _, rule := range rules {
		if time.Since(rule.CreatedAt) >= 24*time.Hour {
			if err := s.ruleRepo.UpdateStatus(rule.RuleID, "experimental"); err != nil {
				logger.Error("failed to activate pending rule",
					zap.String("rule_id", rule.RuleID),
					zap.Error(err),
				)
			} else {
				logger.Info("pending rule activated as experimental",
					zap.String("rule_id", rule.RuleID),
				)
			}
		}
	}
}
