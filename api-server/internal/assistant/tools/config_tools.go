package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/repository"
)

// ConfigToolDeps 配置工具依赖
type ConfigToolDeps struct {
	SystemConfigRepo *repository.SystemConfigRepo
	LLMConfigRepo    *repository.ConfigRepository
}

// RegisterConfigTools 注册配置域工具
func RegisterConfigTools(registry *assistant.ToolRegistry, deps ConfigToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Config.Get",
		Domain:             "config",
		Operation:          "get",
		Capability:         "get_system_config",
		Description:        "获取系统配置信息，支持按配置键查询",
		ModelDescription:   "Get system configuration values, optionally restricted to one configuration key.",
		Risk:               assistant.ToolRiskLow,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"config_key": map[string]interface{}{"type": "string", "description": "配置键名称"},
			},
			"required": []string{"config_key"},
		},
		Handler: makeConfigGetHandler(deps.SystemConfigRepo),
	}); err != nil {
		return err
	}

	return nil
}

func makeConfigGetHandler(repo *repository.SystemConfigRepo) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		configKey := getStringArg(args, "config_key", "")
		if configKey == "" {
			return nil, fmt.Errorf("config_key is required")
		}

		config, err := repo.GetByKey(configKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get config: %w", err)
		}

		return config, nil
	}
}
