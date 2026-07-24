package tools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"
)

// RuleGenerationServiceForTools 规则生成服务接口
type RuleGenerationServiceForTools interface {
	GenerateRule(ctx context.Context, req *service.GenerateRuleRequest) (*service.GenerateRuleResponse, error)
}

type SigmaRuleContextReader interface {
	ListBySession(ctx context.Context, sessionID string) ([]model.AssistantContextRef, error)
}

type SigmaRuleLifecycleServiceForTools interface {
	UploadRules(file io.Reader, fileName string, fileSize int64) (*service.UploadResult, error)
	ApproveRule(ruleID string, targetHostIDs []string) error
}

// SigmaRuleToolDeps Sigma规则工具依赖
type SigmaRuleToolDeps struct {
	SigmaRuleRepo    *repository.SigmaRuleRepository
	RuleGenService   RuleGenerationServiceForTools
	ContextRefReader SigmaRuleContextReader
	LifecycleService SigmaRuleLifecycleServiceForTools
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
		Name:               "SigmaRule.Import",
		Domain:             assistant.DomainSigmaRule,
		Operation:          assistant.OpCreate,
		Capability:         "import_sigma_rule",
		Description:        "Import and validate a Sigma detection rule from an attached YAML file in the current assistant session.",
		Aliases:            []string{"导入 Sigma 规则", "解析规则文件", "解析异常检测规则"},
		Tags:               []string{"sigma", "sigma_rule", "detection", "rule", "file", "yaml", "import"},
		ObjectTypes:        []string{"file", "rule", "sigma_rule"},
		PageRoutes:         []string{"/assistant", "/detection/rules", "/detection"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact attached assistant file ID from the current session context.",
				},
			},
			"required": []string{"file_id"},
		},
		ResultSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_id":       map[string]interface{}{"type": "string", "description": "Persisted Sigma rule ID."},
				"filename":      map[string]interface{}{"type": "string", "description": "Imported attachment filename."},
				"parsed_count":  map[string]interface{}{"type": "integer", "description": "Number of newly imported rules."},
				"skipped_count": map[string]interface{}{"type": "integer", "description": "Number of duplicate rules reused without reimporting."},
			},
		},
		ResultContract: assistant.ToolResultContract{
			OperationRefFields: []string{"rule_id"},
			ArtifactRefFields:  []string{"rule_id"},
		},
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "internal/service/sigma_rule_upload_service.go",
			Function:  "SigmaRuleUploadService.UploadRules",
		},
		Handler: makeSigmaRuleImportHandler(deps.ContextRefReader, deps.LifecycleService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "SigmaRule.Enable",
		Domain:             assistant.DomainSigmaRule,
		Operation:          assistant.OpUpdate,
		Capability:         "enable_sigma_rule",
		Description:        "Enable a persisted Sigma detection rule and dispatch it to all hosts or an explicit host set.",
		Aliases:            []string{"启用 Sigma 规则", "开启规则检测", "下发异常检测规则"},
		Tags:               []string{"sigma", "sigma_rule", "detection", "rule", "enable", "dispatch"},
		ObjectTypes:        []string{"rule", "sigma_rule", "host"},
		PageRoutes:         []string{"/assistant", "/detection/rules", "/detection"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact persisted Sigma rule ID returned by SigmaRule.Import or supplied by the user.",
				},
				"target_host_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional exact host UUIDs. Omit to dispatch to all hosts.",
				},
			},
			"required": []string{"rule_id"},
		},
		ResultSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_id":          map[string]interface{}{"type": "string", "description": "Enabled Sigma rule ID."},
				"operation_status": map[string]interface{}{"type": "string", "description": "Terminal enable operation status."},
				"target_scope":     map[string]interface{}{"type": "string", "description": "All hosts or explicit hosts."},
			},
		},
		ResultContract: assistant.ToolResultContract{
			OperationStatusField: "operation_status",
			SuccessValues:        []string{"completed"},
			FailureValues:        []string{"failed"},
			OperationRefFields:   []string{"rule_id"},
			SideEffectRefFields:  []string{"rule_id"},
		},
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "internal/service/sigma_rule_upload_service.go",
			Function:  "SigmaRuleUploadService.ApproveRule",
		},
		Handler: makeSigmaRuleEnableHandler(deps.LifecycleService),
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

func makeSigmaRuleImportHandler(contextReader SigmaRuleContextReader, lifecycle SigmaRuleLifecycleServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if contextReader == nil || lifecycle == nil {
			return nil, fmt.Errorf("Sigma rule import service is not configured")
		}
		fileID := getStringArg(args, "file_id", "")
		if fileID == "" {
			return nil, fmt.Errorf("file_id is required")
		}
		invocation, ok := assistant.ToolInvocationFromContext(ctx)
		if !ok || strings.TrimSpace(invocation.SessionID) == "" {
			return nil, fmt.Errorf("assistant session context is required")
		}
		refs, err := contextReader.ListBySession(ctx, invocation.SessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to load attached files: %w", err)
		}
		var attachment *model.AssistantContextRef
		for index := range refs {
			if refs[index].ObjectType == "file" && refs[index].ObjectID == fileID {
				attachment = &refs[index]
				break
			}
		}
		if attachment == nil {
			return nil, fmt.Errorf("attached file %s was not found in the current session", fileID)
		}
		content := strings.TrimSpace(attachment.Summary)
		if content == "" {
			return nil, fmt.Errorf("attached file %s has no parsed content", fileID)
		}
		if strings.HasSuffix(content, "\n...") {
			return nil, fmt.Errorf("attached file %s content is truncated; upload it with the Sigma rule purpose", fileID)
		}

		result, err := lifecycle.UploadRules(strings.NewReader(content), attachment.Title, int64(len([]byte(content))))
		if err != nil {
			return nil, fmt.Errorf("failed to import attached Sigma rule: %w", err)
		}
		if result == nil || !result.Success {
			if result != nil && strings.TrimSpace(result.Error) != "" {
				return nil, fmt.Errorf("failed to import attached Sigma rule: %s", result.Error)
			}
			return nil, fmt.Errorf("failed to import attached Sigma rule")
		}
		ruleID := ""
		if len(result.Rules) > 0 {
			ruleID = strings.TrimSpace(result.Rules[0].RuleID)
		}
		if ruleID == "" {
			return nil, fmt.Errorf("Sigma rule import returned no persisted rule ID")
		}
		return map[string]interface{}{
			"rule_id":       ruleID,
			"filename":      attachment.Title,
			"parsed_count":  result.ParsedCount,
			"failed_count":  result.FailedCount,
			"skipped_count": result.SkippedCount,
			"rules":         result.Rules,
		}, nil
	}
}

func makeSigmaRuleEnableHandler(lifecycle SigmaRuleLifecycleServiceForTools) assistant.ToolHandler {
	return func(_ context.Context, args map[string]interface{}) (interface{}, error) {
		if lifecycle == nil {
			return nil, fmt.Errorf("Sigma rule lifecycle service is not configured")
		}
		ruleID := getStringArg(args, "rule_id", "")
		if ruleID == "" {
			return nil, fmt.Errorf("rule_id is required")
		}
		targetHostIDs, err := getStringSliceArg(args, "target_host_ids")
		if err != nil {
			return nil, fmt.Errorf("invalid target_host_ids: %w", err)
		}
		if err := lifecycle.ApproveRule(ruleID, targetHostIDs); err != nil {
			return nil, fmt.Errorf("failed to enable Sigma rule: %w", err)
		}
		targetScope := "all_hosts"
		if len(targetHostIDs) > 0 {
			targetScope = "explicit_hosts"
		}
		return map[string]interface{}{
			"rule_id":          ruleID,
			"operation_status": "completed",
			"target_scope":     targetScope,
			"target_host_ids":  targetHostIDs,
		}, nil
	}
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
