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
		Name:               "Task.List",
		Domain:             "task",
		Operation:          "list",
		Capability:         "list_tasks",
		Description:        "List task groups with status, task-type, time-range, and keyword filters.",
		ModelDescription:   "List task groups with optional status, type, time-range, and keyword filters.",
		Risk:               assistant.ToolRiskLow,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":       map[string]interface{}{"type": "integer", "description": "One-based page number."},
				"page_size":  map[string]interface{}{"type": "integer", "description": "Items per page."},
				"status":     map[string]interface{}{"type": "string", "enum": []interface{}{"success", "failed", "running", "pending"}, "description": "Task-group status filter."},
				"task_type":  map[string]interface{}{"type": "string", "description": "Task type filter such as check or fix."},
				"start_time": map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive RFC3339 start time."},
				"end_time":   map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive RFC3339 end time."},
				"search":     map[string]interface{}{"type": "string", "description": "Task-group search keyword."},
			},
		},
		Handler: makeTaskListHandler(deps.TaskLogRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Task.GetDetail",
		Domain:             "task",
		Operation:          "get_detail",
		Capability:         "get_task_detail",
		Description:        "Get one task by exact task UUID; do not pass a task-group UUID.",
		ModelDescription:   "Get detailed execution information for one exact task ID.",
		Risk:               assistant.ToolRiskLow,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact individual task UUID, not a task-group UUID."},
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
