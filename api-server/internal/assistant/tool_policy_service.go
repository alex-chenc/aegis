package assistant

import (
	"context"
	"fmt"

	"api-server/internal/model"
	"api-server/internal/repository"
	"go.uber.org/zap"
)

// ToolPolicyService 工具策略服务
type ToolPolicyService struct {
	policyRepo repository.AssistantToolPolicyRepository
	registry   *ToolRegistry
	logger     *zap.Logger
}

// NewToolPolicyService 创建工具策略服务
func NewToolPolicyService(
	policyRepo repository.AssistantToolPolicyRepository,
	registry *ToolRegistry,
	logger *zap.Logger,
) *ToolPolicyService {
	return &ToolPolicyService{
		policyRepo: policyRepo,
		registry:   registry,
		logger:     logger,
	}
}

// SyncCatalogTools 同步工具目录到策略表
func (s *ToolPolicyService) SyncCatalogTools(ctx context.Context) error {
	tools := s.registry.List()
	var policies []model.AssistantToolPolicy

	for _, tool := range tools {
		policies = append(policies, model.AssistantToolPolicy{
			ToolName:          tool.Name,
			Domain:            tool.Domain,
			Operation:         tool.Operation,
			RiskLevel:         tool.RiskLevel,
			Description:       tool.Description,
			DefaultWhitelisted: tool.DefaultWhitelisted,
			Whitelisted:       tool.DefaultWhitelisted,
			Enabled:           tool.Enabled,
			Source:            "builtin",
		})
	}

	if err := s.policyRepo.BatchUpsert(ctx, policies); err != nil {
		return fmt.Errorf("failed to sync catalog tools: %w", err)
	}

	s.logger.Info("synced catalog tools to policy table", zap.Int("count", len(policies)))
	return nil
}

// GetApprovalMode 获取当前审批模式
func (s *ToolPolicyService) GetApprovalMode(ctx context.Context) (string, error) {
	// Read from system config
	// Default to whitelist mode
	return "whitelist", nil
}

// SetApprovalMode 设置审批模式
func (s *ToolPolicyService) SetApprovalMode(ctx context.Context, mode string) error {
	validModes := map[string]bool{
		"request_approval": true,
		"whitelist":        true,
		"full_access":      true,
	}
	if !validModes[mode] {
		return fmt.Errorf("invalid approval mode: %s", mode)
	}
	// Save to system config
	return nil
}

// IsToolWhitelisted 检查工具是否在白名单中
func (s *ToolPolicyService) IsToolWhitelisted(ctx context.Context, toolName string) (bool, error) {
	policy, err := s.policyRepo.FindByToolName(ctx, toolName)
	if err != nil {
		// If not found, default to not whitelisted
		return false, nil
	}
	return policy.Whitelisted && policy.Enabled, nil
}

// ListTools 列出所有工具策略
func (s *ToolPolicyService) ListTools(ctx context.Context, query repository.ToolPolicyQuery) ([]model.AssistantToolPolicy, int64, error) {
	return s.policyRepo.List(ctx, query)
}

// UpdateWhitelist 更新工具白名单状态
func (s *ToolPolicyService) UpdateWhitelist(ctx context.Context, toolName string, whitelisted bool, operator string) error {
	return s.policyRepo.UpdateWhitelist(ctx, toolName, whitelisted, operator)
}

// BatchUpdateWhitelist 批量更新白名单
func (s *ToolPolicyService) BatchUpdateWhitelist(ctx context.Context, items []repository.WhitelistUpdateItem, operator string) error {
	return s.policyRepo.BatchUpdateWhitelist(ctx, items, operator)
}

// ResetDefaultWhitelist 重置默认白名单
func (s *ToolPolicyService) ResetDefaultWhitelist(ctx context.Context, operator string) error {
	return s.policyRepo.ResetDefaultWhitelist(ctx, operator)
}
