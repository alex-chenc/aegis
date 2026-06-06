package service

import (
	"context"
	"fmt"

	"api-server/internal/model"
	"api-server/internal/repository"
)

// SigmaRuleManagementService Sigma 规则管理服务（对齐设计文档第 11 节）
type SigmaRuleManagementService struct {
	sigmaRuleRepo   *repository.SigmaRuleRepository
	blockPolicyRepo *repository.BlockPolicyRepository
	alertRepo       *repository.AlertRepository
}

// NewSigmaRuleManagementService 创建 Sigma 规则管理服务
func NewSigmaRuleManagementService(
	sigmaRuleRepo *repository.SigmaRuleRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	alertRepo *repository.AlertRepository,
) *SigmaRuleManagementService {
	return &SigmaRuleManagementService{
		sigmaRuleRepo:   sigmaRuleRepo,
		blockPolicyRepo: blockPolicyRepo,
		alertRepo:       alertRepo,
	}
}

// GenerateSigmaRuleRequest 生成 Sigma 规则请求
type GenerateSigmaRuleRequest struct {
	Description string `json:"description"`
	HostID      string `json:"host_id,omitempty"`
	AlertID     string `json:"alert_id,omitempty"`
}

// DeleteRulesResult 删除规则结果
type DeleteRulesResult struct {
	Deleted int      `json:"deleted"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// RuleDeleteCheckResult 删除前检查结果
type RuleDeleteCheckResult struct {
	CanDelete        bool     `json:"can_delete"`
	AffectedAlerts   int      `json:"affected_alerts"`
	AffectedPolicies int     `json:"affected_policies"`
	Warnings         []string `json:"warnings,omitempty"`
}

// UpdateContent 更新规则内容（对齐 SigmaRule.Content.Update 工具）
func (s *SigmaRuleManagementService) UpdateContent(ctx context.Context, ruleID string, content string, operator string) (*model.SigmaRule, error) {
	// TODO: 实现规则内容更新
	return nil, fmt.Errorf("not implemented")
}

// UpdateStatus 更新规则状态（对齐 SigmaRule.Status.Update 工具）
func (s *SigmaRuleManagementService) UpdateStatus(ctx context.Context, ruleID string, status string, targetHostIDs []string, operator string) error {
	// TODO: 实现规则状态更新
	return fmt.Errorf("not implemented")
}

// CheckBeforeDelete 删除前检查（对齐 SigmaRule.Delete.Check 工具）
func (s *SigmaRuleManagementService) CheckBeforeDelete(ctx context.Context, ruleIDs []string) (*RuleDeleteCheckResult, error) {
	// TODO: 实现删除前检查
	return &RuleDeleteCheckResult{CanDelete: true}, nil
}

// DeleteRules 删除规则（对齐 SigmaRule.Delete 工具）
func (s *SigmaRuleManagementService) DeleteRules(ctx context.Context, ruleIDs []string, operator string) (*DeleteRulesResult, error) {
	// TODO: 实现规则删除
	return &DeleteRulesResult{}, nil
}
