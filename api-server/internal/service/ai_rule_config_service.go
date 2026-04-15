package service

import (
	"fmt"

	"api-server/internal/model"
	"api-server/internal/repository"
)

type AIRuleConfigService struct {
	configRepo *repository.AIRuleConfigRepository
}

func NewAIRuleConfigService(configRepo *repository.AIRuleConfigRepository) *AIRuleConfigService {
	return &AIRuleConfigService{configRepo: configRepo}
}

// GetConfig 获取AI规则配置
func (s *AIRuleConfigService) GetConfig() (*model.AIConfigResponse, error) {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return nil, err
	}

	return config.ToResponse()
}

// UpdateConfig 更新AI规则配置
func (s *AIRuleConfigService) UpdateConfig(req *model.UpdateAIConfigRequest) (*model.AIConfigResponse, error) {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if req.Enabled != nil {
		config.Enabled = *req.Enabled
		updates["enabled"] = *req.Enabled
	}
	if req.Mode != nil {
		if *req.Mode != "suggest" && *req.Mode != "auto" {
			return nil, fmt.Errorf("mode must be 'suggest' or 'auto'")
		}
		config.Mode = *req.Mode
		updates["mode"] = *req.Mode
	}
	if req.Thresholds != nil {
		if err := config.SetThresholds(req.Thresholds); err != nil {
			return nil, err
		}
		updates["thresholds"] = config.Thresholds
	}
	if req.Conservatism != nil {
		if *req.Conservatism < 0 || *req.Conservatism > 1 {
			return nil, fmt.Errorf("conservatism must be between 0 and 1")
		}
		config.Conservatism = *req.Conservatism
		updates["conservatism"] = *req.Conservatism
	}
	if req.RequireApproval != nil {
		config.RequireApproval = *req.RequireApproval
		updates["require_approval"] = *req.RequireApproval
	}
	if req.AutoActivateAfterApproval != nil {
		config.AutoActivateAfterApproval = *req.AutoActivateAfterApproval
		updates["auto_activate_after_approval"] = *req.AutoActivateAfterApproval
	}
	if req.ActivationDelayHours != nil {
		if *req.ActivationDelayHours < 0 {
			return nil, fmt.Errorf("activation_delay_hours must be non-negative")
		}
		config.ActivationDelayHours = *req.ActivationDelayHours
		updates["activation_delay_hours"] = *req.ActivationDelayHours
	}
	if req.NotifyOnGeneration != nil {
		config.NotifyOnGeneration = *req.NotifyOnGeneration
		updates["notify_on_generation"] = *req.NotifyOnGeneration
	}
	if req.NotifyOnApproval != nil {
		config.NotifyOnApproval = *req.NotifyOnApproval
		updates["notify_on_approval"] = *req.NotifyOnApproval
	}
	if req.NotificationTargets != nil {
		if err := config.SetNotificationTargets(req.NotificationTargets); err != nil {
			return nil, err
		}
		updates["notification_targets"] = config.NotificationTargets
	}

	if err := s.configRepo.UpdateConfig(config); err != nil {
		return nil, err
	}

	return config.ToResponse()
}

// IncrementGeneratedCount 增加生成规则计数
func (s *AIRuleConfigService) IncrementGeneratedCount() error {
	return s.configRepo.IncrementGeneratedCount("default")
}

// IncrementApprovedCount 增加审核通过计数
func (s *AIRuleConfigService) IncrementApprovedCount() error {
	return s.configRepo.IncrementApprovedCount("default")
}

// IsEnabled 检查AI规则更新是否启用
func (s *AIRuleConfigService) IsEnabled() bool {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return false
	}
	return config.Enabled
}

// GetMode 获取模式
func (s *AIRuleConfigService) GetMode() string {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return "suggest"
	}
	return config.Mode
}

// GetThresholds 获取触发阈值
func (s *AIRuleConfigService) GetThresholds() *model.Thresholds {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return &model.Thresholds{
			HighFrequencyCount: 10,
			HighFrequencyHours: 1,
		}
	}
	thresholds, _ := config.GetThresholds()
	return thresholds
}

// GetConservatism 获取生成策略
func (s *AIRuleConfigService) GetConservatism() float64 {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return 0.5
	}
	return config.Conservatism
}

// RequireApproval 是否需要审核
func (s *AIRuleConfigService) RequireApproval() bool {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return true
	}
	return config.RequireApproval
}

// AutoActivateAfterApproval 审核后是否自动激活
func (s *AIRuleConfigService) AutoActivateAfterApproval() bool {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return false
	}
	return config.AutoActivateAfterApproval
}

// GetActivationDelayHours 获取激活延迟小时数
func (s *AIRuleConfigService) GetActivationDelayHours() int {
	config, err := s.configRepo.GetDefaultConfig()
	if err != nil {
		return 24
	}
	return config.ActivationDelayHours
}