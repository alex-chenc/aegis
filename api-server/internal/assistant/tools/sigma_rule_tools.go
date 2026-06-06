package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/repository"
	"api-server/internal/service"
)

// RuleGenerationServiceForTools 规则生成服务接口
type RuleGenerationServiceForTools interface {
	GenerateRule(ctx context.Context, req *service.GenerateRuleRequest) (*service.GenerateRuleResponse, error)
}

// SigmaRuleToolDeps Sigma规则工具依赖
type SigmaRuleToolDeps struct {
	SigmaRuleRepo    *repository.SigmaRuleRepository
	RuleGenService   RuleGenerationServiceForTools
}

// RegisterSigmaRuleTools 注册Sigma规则域工具
func RegisterSigmaRuleTools(registry *assistant.ToolRegistry, deps SigmaRuleToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:        "SigmaRule.List",
		Domain:      "sigma_rule",
		Operation:   "list",
		Description: "列出Sigma规则，支持按状态、关键字筛选",
		Risk:        assistant.ToolRiskReadonly,
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量"},
				"status":    map[string]interface{}{"type": "string", "description": "状态筛选（active/experimental/pending/disabled）"},
				"query":     map[string]interface{}{"type": "string", "description": "搜索关键字"},
			},
		},
		Handler: makeSigmaRuleListHandler(deps.SigmaRuleRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:        "SigmaRule.Generate",
		Domain:      "sigma_rule",
		Operation:   "generate",
		Description: "基于告警样本使用AI生成Sigma规则",
		Risk:        assistant.ToolRiskMedium,
		Enabled:     true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mitre_id": map[string]interface{}{
					"type":        "string",
					"description": "MITRE技术ID",
				},
				"sample_alert_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "样本告警ID列表",
				},
				"conservatism": map[string]interface{}{
					"type":        "number",
					"description": "保守度（0.0-1.0），越低越保守，默认0.5",
				},
			},
			"required": []string{"mitre_id"},
		},
		Handler: makeSigmaRuleGenerateHandler(deps.RuleGenService),
	}); err != nil {
		return err
	}

	return nil
}

func makeSigmaRuleListHandler(repo *repository.SigmaRuleRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)

		filters := make(map[string]interface{})
		if status := getStringArg(args, "status", ""); status != "" {
			filters["status"] = status
		}
		if query := getStringArg(args, "query", ""); query != "" {
			filters["query"] = query
		}

		rules, total, err := repo.List(page, pageSize, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to list sigma rules: %w", err)
		}

		return map[string]interface{}{
			"data":  rules,
			"total": total,
		}, nil
	}
}

func makeSigmaRuleGenerateHandler(svc RuleGenerationServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		mitreID := getStringArg(args, "mitre_id", "")
		if mitreID == "" {
			return nil, fmt.Errorf("mitre_id is required")
		}

		sampleAlertIDs, _ := getStringSliceArg(args, "sample_alert_ids")

		req := &service.GenerateRuleRequest{
			MitreID:      mitreID,
			SampleAlerts: sampleAlertIDs,
			Conservatism: getFloatArg(args, "conservatism", 0.5),
		}

		result, err := svc.GenerateRule(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to generate sigma rule: %w", err)
		}

		return result, nil
	}
}

// getFloatArg extracts a float argument from the args map with a default value.
func getFloatArg(args map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	return defaultVal
}
