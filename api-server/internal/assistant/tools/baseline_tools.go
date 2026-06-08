package tools

import (
	"context"
	"fmt"
	"strings"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"
	"github.com/google/uuid"
)

// TaskServiceForTools 任务服务接口（基线检查/修复）
type TaskServiceForTools interface {
	CreateAndDispatchTasks(ctx context.Context, ruleIDs, hostIDs []string, taskType string, existingGroupID ...uuid.UUID) (*service.TaskCreateResult, error)
}

type ScriptGenerationServiceForTools interface {
	BatchGenerateForTemplate(ctx context.Context, templateID uuid.UUID, scriptType string, maxConcurrency int) (*service.BatchGenerateResult, error)
	QueueScriptGeneration(ruleID uuid.UUID, scriptType string) error
}

type TemplateRepositoryForTools interface {
	FindAll(page, pageSize int) ([]model.Template, error)
	FindByID(id uuid.UUID) (*model.Template, error)
}

type RuleRepositoryForTools interface {
	FindByTemplateID(templateID uuid.UUID) ([]model.AegisRule, error)
	FindByID(id uuid.UUID) (*model.AegisRule, error)
}

// BaselineToolDeps 基线工具依赖
type BaselineToolDeps struct {
	TaskService      TaskServiceForTools
	TemplateRepo     TemplateRepositoryForTools
	RuleRepo         RuleRepositoryForTools
	ScriptGenService ScriptGenerationServiceForTools
}

// RegisterBaselineTools 注册基线写操作工具
func RegisterBaselineTools(registry *assistant.ToolRegistry, deps BaselineToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Baseline.Template.List",
		Domain:             assistant.DomainBaseline,
		Operation:          assistant.OpList,
		Capability:         "list_baseline_templates",
		Description:        "查询基线模板列表，包括解析状态和规则数量",
		Aliases:            []string{"基线模板", "基线上传记录", "模板列表"},
		Tags:               []string{"baseline", "template", "rule"},
		ObjectTypes:        []string{"baseline_template"},
		PageRoutes:         []string{"/baseline"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码，默认1"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量，默认20"},
			},
		},
		Handler: makeBaselineTemplateListHandler(deps.TemplateRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Baseline.Template.Status.Get",
		Domain:             assistant.DomainBaseline,
		Operation:          assistant.OpGet,
		Capability:         "get_baseline_template_status",
		Description:        "查询基线模板解析状态、错误信息和规则数量",
		Aliases:            []string{"模板解析状态", "基线识别状态", "基线解析进度"},
		Tags:               []string{"baseline", "template", "status"},
		ObjectTypes:        []string{"baseline_template"},
		PageRoutes:         []string{"/baseline"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"template_id": map[string]interface{}{"type": "string", "description": "基线模板ID"},
			},
			"required": []string{"template_id"},
		},
		Handler: makeBaselineTemplateStatusHandler(deps.TemplateRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Baseline.Template.Rules.List",
		Domain:             assistant.DomainBaseline,
		Operation:          assistant.OpList,
		Capability:         "list_baseline_template_rules",
		Description:        "查询基线模板识别出的规则列表和脚本生成状态",
		Aliases:            []string{"基线规则", "模板规则", "基线识别结果"},
		Tags:               []string{"baseline", "template", "rule", "script"},
		ObjectTypes:        []string{"baseline_template", "baseline_rule"},
		PageRoutes:         []string{"/baseline"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"template_id": map[string]interface{}{"type": "string", "description": "基线模板ID"},
			},
			"required": []string{"template_id"},
		},
		Handler: makeBaselineTemplateRulesHandler(deps.RuleRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Baseline.Script.Generate",
		Domain:             assistant.DomainBaseline,
		Operation:          assistant.OpGenerate,
		Capability:         "generate_baseline_scripts",
		Description:        "为基线模板或指定规则生成检测/修复脚本",
		Aliases:            []string{"生成基线脚本", "自动生成检测脚本", "自动生成修复脚本", "基线脚本生成"},
		Tags:               []string{"baseline", "script", "generate", "check", "fix"},
		ObjectTypes:        []string{"baseline_template", "baseline_rule"},
		PageRoutes:         []string{"/baseline"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"template_id": map[string]interface{}{"type": "string", "description": "基线模板ID；提供后按模板批量生成"},
				"rule_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "规则ID列表；未提供 template_id 时使用",
				},
				"script_type": map[string]interface{}{"type": "string", "description": "脚本类型：CHECK 或 FIX"},
			},
			"required": []string{"script_type"},
		},
		Handler: makeBaselineScriptGenerateHandler(deps.ScriptGenService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Task.RunCheck",
		Domain:             "baseline",
		Operation:          "run_check",
		Description:        "触发基线检查任务，将检查脚本下发到指定主机执行",
		Risk:               assistant.ToolRiskMedium,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "规则ID列表",
				},
				"host_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "主机ID列表",
				},
			},
			"required": []string{"rule_ids", "host_ids"},
		},
		Handler: makeRunCheckHandler(deps.TaskService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Task.RunFix",
		Domain:             "baseline",
		Operation:          "run_fix",
		Description:        "触发基线修复任务，将修复脚本下发到指定主机执行（高风险操作）",
		Risk:               assistant.ToolRiskHigh,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "规则ID列表",
				},
				"host_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "主机ID列表",
				},
			},
			"required": []string{"rule_ids", "host_ids"},
		},
		Handler: makeRunFixHandler(deps.TaskService),
	}); err != nil {
		return err
	}

	return nil
}

func makeBaselineTemplateListHandler(repo TemplateRepositoryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if repo == nil {
			return nil, fmt.Errorf("template repository not configured")
		}
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)
		templates, err := repo.FindAll(page, pageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to list baseline templates: %w", err)
		}
		return map[string]interface{}{
			"data":       templates,
			"total":      len(templates),
			"route_path": "/baseline",
		}, nil
	}
}

func makeBaselineTemplateStatusHandler(repo TemplateRepositoryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if repo == nil {
			return nil, fmt.Errorf("template repository not configured")
		}
		templateID, err := parseUUID(args, "template_id")
		if err != nil {
			return nil, err
		}
		template, err := repo.FindByID(templateID)
		if err != nil {
			return nil, fmt.Errorf("failed to get baseline template: %w", err)
		}
		return map[string]interface{}{
			"template":   template,
			"route_path": "/baseline",
		}, nil
	}
}

func makeBaselineTemplateRulesHandler(repo RuleRepositoryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if repo == nil {
			return nil, fmt.Errorf("rule repository not configured")
		}
		templateID, err := parseUUID(args, "template_id")
		if err != nil {
			return nil, err
		}
		rules, err := repo.FindByTemplateID(templateID)
		if err != nil {
			return nil, fmt.Errorf("failed to list baseline rules: %w", err)
		}
		return map[string]interface{}{
			"data":       rules,
			"total":      len(rules),
			"route_path": "/baseline",
		}, nil
	}
}

func makeBaselineScriptGenerateHandler(svc ScriptGenerationServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if svc == nil {
			return nil, fmt.Errorf("script generation service not configured")
		}
		scriptType := strings.ToUpper(strings.TrimSpace(getStringArg(args, "script_type", "")))
		if scriptType != "CHECK" && scriptType != "FIX" {
			return nil, fmt.Errorf("script_type must be CHECK or FIX")
		}

		templateIDStr := getStringArg(args, "template_id", "")
		if templateIDStr != "" {
			templateID, err := uuid.Parse(templateIDStr)
			if err != nil {
				return nil, fmt.Errorf("invalid template_id: %w", err)
			}
			result, err := svc.BatchGenerateForTemplate(ctx, templateID, scriptType, 2)
			if err != nil {
				return nil, fmt.Errorf("failed to generate baseline scripts by template: %w", err)
			}
			return map[string]interface{}{
				"template_id": templateID.String(),
				"script_type": scriptType,
				"result":      result,
				"route_path":  "/baseline",
			}, nil
		}

		ruleIDs, err := getStringSliceArg(args, "rule_ids")
		if err != nil || len(ruleIDs) == 0 {
			return nil, fmt.Errorf("rule_ids or template_id is required")
		}

		queued := make([]string, 0, len(ruleIDs))
		for _, ruleIDStr := range ruleIDs {
			ruleID, err := uuid.Parse(ruleIDStr)
			if err != nil {
				return nil, fmt.Errorf("invalid rule_id %s: %w", ruleIDStr, err)
			}
			if err := svc.QueueScriptGeneration(ruleID, scriptType); err != nil {
				return nil, fmt.Errorf("failed to queue script generation for rule %s: %w", ruleIDStr, err)
			}
			queued = append(queued, ruleID.String())
		}

		return map[string]interface{}{
			"script_type": scriptType,
			"queued":      queued,
			"total":       len(queued),
			"route_path":  "/baseline",
		}, nil
	}
}

func makeRunCheckHandler(svc TaskServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		ruleIDs, err := getStringSliceArg(args, "rule_ids")
		if err != nil {
			return nil, fmt.Errorf("rule_ids: %w", err)
		}
		hostIDs, err := getStringSliceArg(args, "host_ids")
		if err != nil {
			return nil, fmt.Errorf("host_ids: %w", err)
		}

		result, err := svc.CreateAndDispatchTasks(ctx, ruleIDs, hostIDs, "CHECK")
		if err != nil {
			return nil, fmt.Errorf("failed to create check tasks: %w", err)
		}

		return map[string]interface{}{
			"task_group_id": result.TaskGroupID.String(),
			"task_ids":      result.TaskIDs,
			"task_type":     "CHECK",
			"task_ref": buildTaskRef(
				"baseline_task",
				result.TaskGroupID.String(),
				result.TaskGroupID.String(),
				"/api/v1/tasks/"+result.TaskGroupID.String()+"/status",
				"/baseline",
			),
		}, nil
	}
}

func makeRunFixHandler(svc TaskServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		ruleIDs, err := getStringSliceArg(args, "rule_ids")
		if err != nil {
			return nil, fmt.Errorf("rule_ids: %w", err)
		}
		hostIDs, err := getStringSliceArg(args, "host_ids")
		if err != nil {
			return nil, fmt.Errorf("host_ids: %w", err)
		}

		result, err := svc.CreateAndDispatchTasks(ctx, ruleIDs, hostIDs, "FIX")
		if err != nil {
			return nil, fmt.Errorf("failed to create fix tasks: %w", err)
		}

		return map[string]interface{}{
			"task_group_id": result.TaskGroupID.String(),
			"task_ids":      result.TaskIDs,
			"task_type":     "FIX",
			"task_ref": buildTaskRef(
				"baseline_task",
				result.TaskGroupID.String(),
				result.TaskGroupID.String(),
				"/api/v1/tasks/"+result.TaskGroupID.String()+"/status",
				"/baseline",
			),
		}, nil
	}
}

// getStringSliceArg extracts a string slice argument from the args map.
func getStringSliceArg(args map[string]interface{}, key string) ([]string, error) {
	v, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	switch slice := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(slice))
		for i, item := range slice {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] is not a string", key, i)
			}
			result = append(result, s)
		}
		return result, nil
	case []string:
		return slice, nil
	default:
		return nil, fmt.Errorf("%s must be an array", key)
	}
}
