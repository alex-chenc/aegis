package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/repository"
	"github.com/google/uuid"
)

// TaskToolDeps 任务工具依赖
type TaskToolDeps struct {
	TaskLogRepo *repository.TaskLogRepository
}

// RegisterTaskTools 注册任务域工具
func RegisterTaskTools(registry *assistant.ToolRegistry, deps TaskToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Task.List",
		Domain:      "task",
		Operation:   "list",
		Description: "列出任务组，支持按状态、类型、时间范围筛选",
		Risk:        assistant.ToolRiskLow,
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":       map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size":  map[string]interface{}{"type": "integer", "description": "每页数量"},
				"status":     map[string]interface{}{"type": "string", "description": "任务状态筛选（success/failed/running/pending）"},
				"task_type":  map[string]interface{}{"type": "string", "description": "任务类型筛选（check/fix）"},
				"start_time": map[string]interface{}{"type": "string", "description": "开始时间（RFC3339格式）"},
				"end_time":   map[string]interface{}{"type": "string", "description": "结束时间（RFC3339格式）"},
				"search":     map[string]interface{}{"type": "string", "description": "搜索关键字"},
			},
		},
		Handler: makeTaskListHandler(deps.TaskLogRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Task.GetDetail",
		Domain:      "task",
		Operation:   "get_detail",
		Description: "根据任务ID获取任务详细信息",
		Risk:        assistant.ToolRiskLow,
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{"type": "string", "description": "任务ID（UUID）"},
			},
			"required": []string{"task_id"},
		},
		Handler: makeTaskGetDetailHandler(deps.TaskLogRepo),
	}); err != nil {
		return err
	}

	return nil
}

func makeTaskListHandler(repo *repository.TaskLogRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)
		status := getStringArg(args, "status", "")
		taskType := getStringArg(args, "task_type", "")
		search := getStringArg(args, "search", "")

		params := repository.ListTaskGroupsParams{
			Page:     page,
			PageSize: pageSize,
			Status:   status,
			TaskType: taskType,
			Search:   search,
		}

		groups, err := repo.ListTaskGroups(params)
		if err != nil {
			return nil, fmt.Errorf("failed to list task groups: %w", err)
		}

		total, err := repo.CountTaskGroups(params)
		if err != nil {
			return nil, fmt.Errorf("failed to count task groups: %w", err)
		}

		return map[string]interface{}{
			"data":  groups,
			"total": total,
		}, nil
	}
}

func makeTaskGetDetailHandler(repo *repository.TaskLogRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		taskIDStr := getStringArg(args, "task_id", "")
		if taskIDStr == "" {
			return nil, fmt.Errorf("task_id is required")
		}

		taskID, err := uuid.Parse(taskIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid task_id: %w", err)
		}

		task, err := repo.FindByID(taskID)
		if err != nil {
			return nil, fmt.Errorf("failed to find task: %w", err)
		}

		return task, nil
	}
}
