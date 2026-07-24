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
	SigmaRuleRepo  *repository.SigmaRuleRepository
	RuleGenService RuleGenerationServiceForTools
}

// RegisterSigmaRuleTools 注册Sigma规则域工具
func RegisterSigmaRuleTools(registry *assistant.ToolRegistry, deps SigmaRuleToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "SigmaRule.List",
		Domain:             assistant.DomainSigmaRule,
		Operation:          assistant.OpList,
		Capability:         "list_sigma_rules",
		Description:        "List anomaly-detection Sigma rules with status and keyword filters.",
		Aliases:            []string{"规则识别", "Sigma 规则", "异常检测规则", "检测规则列表", "规则命中"},
		Tags:               []string{"sigma", "sigma_rule", "detection", "rule", "abnormal"},
		ObjectTypes:        []string{"sigma_rule", "detection", "alert"},
		PageRoutes:         []string{"/detection/rules", "/detection", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "One-based page number."},
				"page_size": map[string]interface{}{"type": "integer", "description": "Items per page."},
				"status":    map[string]interface{}{"type": "string", "description": "Status filter: active, experimental, pending, or disabled."},
				"query":     map[string]interface{}{"type": "string", "description": "Rule search keyword."},
			},
		},
		Handler: makeSigmaRuleListHandler(deps.SigmaRuleRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "SigmaRule.Generate",
		Domain:             assistant.DomainSigmaRule,
		Operation:          assistant.OpGenerate,
		Capability:         "generate_sigma_rule",
		Description:        "Generate a Sigma detection rule from anomaly-alert samples and a MITRE technique using AI.",
		Aliases:            []string{"生成 Sigma 规则", "规则自动生成", "异常检测规则生成", "AI 规则生成", "规则识别生成"},
		Tags:               []string{"sigma", "sigma_rule", "detection", "rule", "generate", "ai-analysis"},
		ObjectTypes:        []string{"sigma_rule", "detection", "alert"},
		PageRoutes:         []string{"/detection/rules", "/detection", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mitre_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact MITRE technique ID.",
				},
				"sample_alert_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Alert IDs used as generation samples.",
				},
				"conservatism": map[string]interface{}{
					"type":        "number",
					"description": "Conservatism from 0.0 to 1.0; lower is more conservative and the default is 0.5.",
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
