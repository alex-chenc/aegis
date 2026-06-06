package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/service"
	"github.com/google/uuid"
)

// TaskServiceForTools 任务服务接口（基线检查/修复）
type TaskServiceForTools interface {
	CreateAndDispatchTasks(ctx context.Context, ruleIDs, hostIDs []string, taskType string, existingGroupID ...uuid.UUID) (*service.TaskCreateResult, error)
}

// BaselineToolDeps 基线工具依赖
type BaselineToolDeps struct {
	TaskService TaskServiceForTools
}

// RegisterBaselineTools 注册基线写操作工具
func RegisterBaselineTools(registry *assistant.ToolRegistry, deps BaselineToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Task.RunCheck",
		Domain:      "baseline",
		Operation:   "run_check",
		Description: "触发基线检查任务，将检查脚本下发到指定主机执行",
		Risk:        assistant.ToolRiskMedium,
		Enabled:     true,
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
		Name:        "Task.RunFix",
		Domain:      "baseline",
		Operation:   "run_fix",
		Description: "触发基线修复任务，将修复脚本下发到指定主机执行（高风险操作）",
		Risk:        assistant.ToolRiskHigh,
		Enabled:     true,
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
