package service

import (
	"time"

	"aegis-system/internal/grpc_server"
	"aegis-system/internal/repository"
	pb "aegis-system/pkg/api/v1"
	"aegis-system/pkg/logger"

	"go.uber.org/zap"
)

type SigmaRuleService struct {
	ruleRepo   *repository.SigmaRuleRepository
	grpcServer *grpc_server.GRPCServer
}

func NewSigmaRuleService(ruleRepo *repository.SigmaRuleRepository) *SigmaRuleService {
	return &SigmaRuleService{ruleRepo: ruleRepo}
}

func (s *SigmaRuleService) SetGRPCServer(server *grpc_server.GRPCServer) {
	s.grpcServer = server
}

func (s *SigmaRuleService) ApproveRule(ruleID string) error {
	if err := s.ruleRepo.UpdateStatus(ruleID, "active"); err != nil {
		return err
	}
	s.broadcastRuleUpdate(ruleID, "active")
	return nil
}

func (s *SigmaRuleService) DisableRule(ruleID string) error {
	if err := s.ruleRepo.UpdateStatus(ruleID, "disabled"); err != nil {
		return err
	}
	s.broadcastRuleUpdate(ruleID, "disabled")
	return nil
}

func (s *SigmaRuleService) broadcastRuleUpdate(ruleID, status string) {
	if s.grpcServer == nil {
		return
	}

	action := "update"
	if status == "disabled" {
		action = "delete"
	}

	rule, err := s.ruleRepo.FindByID(ruleID)
	if err != nil {
		logger.Warn("failed to get rule for broadcast", zap.String("rule_id", ruleID), zap.Error(err))
		return
	}

	content := rule.Content
	if status == "disabled" {
		content = ""
	}

	s.grpcServer.BroadcastRuleUpdate(&pb.RuleUpdate{
		RuleId:  ruleID,
		Action:  action,
		Content: content,
	})

	logger.Info("rule update broadcasted", zap.String("rule_id", ruleID), zap.String("action", action))
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
