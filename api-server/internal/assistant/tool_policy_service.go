package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"api-server/internal/model"
	"api-server/internal/repository"
	"go.uber.org/zap"
)

// ToolPolicyService 工具策略服务（对齐设计文档 8.1 节）
type ToolPolicyService struct {
	policyRepo   repository.AssistantToolPolicyRepository
	registry     *ToolRegistry
	systemConfig *repository.SystemConfigRepo
	logger       *zap.Logger
}

// ToolPolicyServiceDeps 工具策略服务依赖
type ToolPolicyServiceDeps struct {
	PolicyRepo   repository.AssistantToolPolicyRepository
	Registry     *ToolRegistry
	SystemConfig *repository.SystemConfigRepo
	Logger       *zap.Logger
}

// NewToolPolicyService 创建工具策略服务
func NewToolPolicyService(deps ToolPolicyServiceDeps) *ToolPolicyService {
	return &ToolPolicyService{
		policyRepo:   deps.PolicyRepo,
		registry:     deps.Registry,
		systemConfig: deps.SystemConfig,
		logger:       deps.Logger,
	}
}

// SyncCatalogTools 同步工具目录到策略表
func (s *ToolPolicyService) SyncCatalogTools(ctx context.Context) error {
	tools := s.registry.List()
	var policies []model.AssistantToolPolicy

	for _, tool := range tools {
		policies = append(policies, model.AssistantToolPolicy{
			ToolName:           tool.Name,
			Domain:             string(tool.Domain),
			Operation:          string(tool.Operation),
			RiskLevel:          string(tool.Risk),
			Description:        tool.Description,
			DefaultWhitelisted: tool.DefaultWhitelisted,
			Whitelisted:        tool.DefaultWhitelisted,
			Enabled:            tool.Enabled,
			Source:             "builtin",
		})
	}

	if err := s.policyRepo.BatchUpsert(ctx, policies); err != nil {
		return fmt.Errorf("failed to sync catalog tools: %w", err)
	}

	s.logger.Info("synced catalog tools to policy table", zap.Int("count", len(policies)))
	return nil
}

// GetApprovalMode 获取当前审批模式（从 system_config 表读取，默认 whitelist）
func (s *ToolPolicyService) GetApprovalMode(ctx context.Context) (string, error) {
	if s.systemConfig == nil {
		return model.ApprovalModeWhitelist, nil
	}
	cfg, err := s.systemConfig.GetByKey("assistant.tool_approval_mode")
	if err != nil || cfg == nil {
		return model.ApprovalModeWhitelist, nil
	}
	// ConfigValue 是 JSON 编码的字符串，如 "\"whitelist\""
	var mode string
	if err := json.Unmarshal(cfg.ConfigValue, &mode); err != nil {
		// 兼容直接存储非 JSON 值
		mode = strings.Trim(string(cfg.ConfigValue), "\"")
	}
	validModes := map[string]bool{
		model.ApprovalModeRequestApproval: true,
		model.ApprovalModeWhitelist:       true,
		model.ApprovalModeFullAccess:      true,
	}
	if !validModes[mode] {
		return model.ApprovalModeWhitelist, nil
	}
	return mode, nil
}

// SetApprovalMode 设置审批模式（持久化到 system_config 表）
func (s *ToolPolicyService) SetApprovalMode(ctx context.Context, mode string, operator string) error {
	validModes := map[string]bool{
		model.ApprovalModeRequestApproval: true,
		model.ApprovalModeWhitelist:       true,
		model.ApprovalModeFullAccess:      true,
	}
	if !validModes[mode] {
		return fmt.Errorf("invalid approval mode: %s", mode)
	}
	if s.systemConfig == nil {
		return fmt.Errorf("system config not available")
	}
	return s.systemConfig.Upsert("assistant.tool_approval_mode", mode, "智能体工具审批模式", "assistant")
}

// IsToolWhitelisted 检查工具是否在白名单中
func (s *ToolPolicyService) IsToolWhitelisted(ctx context.Context, toolName string) (bool, error) {
	if s.policyRepo == nil {
		if s.registry == nil {
			return false, nil
		}
		tool, ok := s.registry.Get(toolName)
		if !ok {
			return false, nil
		}
		return tool.DefaultWhitelisted && tool.Enabled, nil
	}
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
