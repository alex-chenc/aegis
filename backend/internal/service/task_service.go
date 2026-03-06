package service

import (
	"context"
	
	"time"

	"baseline-system/internal/model"
	"baseline-system/internal/repository"
	"baseline-system/internal/storage"
	"baseline-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TaskService 任务编排服务
type TaskService struct {
	taskLogRepo        *repository.TaskLogRepository
	hostRepo           *repository.HostRepository
	ruleRepo           *repository.RuleRepository
	healingLogRepo     *repository.HealingLogRepository
	redisClient        *storage.RedisClient
	selfHealingService *SelfHealingService
}

// TaskDispatchRequest 任务下发请求
type TaskDispatchRequest struct {
	RuleIDs    []uuid.UUID
	HostIDs    []uuid.UUID
	ScriptType string // CHECK or FIX
}

// TaskDispatchResponse 任务下发响应
type TaskDispatchResponse struct {
	TaskGroupID uuid.UUID
	TaskCount   int
}

// NewTaskService 创建任务编排服务
func NewTaskService(
	taskLogRepo *repository.TaskLogRepository,
	hostRepo *repository.HostRepository,
	ruleRepo *repository.RuleRepository,
	healingLogRepo *repository.HealingLogRepository,
	redisClient *storage.RedisClient,
	selfHealingService *SelfHealingService,
) *TaskService {
	return &TaskService{
		taskLogRepo:        taskLogRepo,
		hostRepo:           hostRepo,
		ruleRepo:           ruleRepo,
		healingLogRepo:     healingLogRepo,
		redisClient:        redisClient,
		selfHealingService: selfHealingService,
	}
}

// DispatchTask 下发任务
func (s *TaskService) DispatchTask(ctx context.Context, req TaskDispatchRequest) (*TaskDispatchResponse, error) {
	logger.Info("dispatching task",
		zap.Int("rule_count", len(req.RuleIDs)),
		zap.Int("host_count", len(req.HostIDs)),
		zap.String("script_type", req.ScriptType),
	)

	// 生成任务组 ID
	taskGroupID := uuid.New()

	taskCount := 0

	// 为每个规则 - 主机对创建子任务
	for _, ruleID := range req.RuleIDs {
		// 获取规则
		rule, err := s.ruleRepo.FindByID(ruleID)
		if err != nil {
			logger.Error("failed to find rule",
				zap.Error(err),
				zap.String("rule_id", ruleID.String()),
			)
			continue
		}

		// 获取脚本内容
		var scriptContent *string
		var scriptVersion *int
		if req.ScriptType == "CHECK" {
			if rule.GeneratedCheckScript != nil {
				scriptContent = rule.GeneratedCheckScript
				scriptVersion = &rule.CheckScriptVersion
			}
		} else {
			if rule.GeneratedFixScript != nil {
				scriptContent = rule.GeneratedFixScript
				scriptVersion = &rule.FixScriptVersion
			}
		}

		if scriptContent == nil {
			logger.Warn("script not available",
				zap.String("rule_id", ruleID.String()),
				zap.String("script_type", req.ScriptType),
			)
			continue
		}

		// 为每个主机创建任务
		for _, hostID := range req.HostIDs {
			// 检查主机是否在线
			online, err := s.redisClient.IsOnline(hostID.String())
			if err != nil {
				logger.Error("failed to check host online status",
					zap.Error(err),
					zap.String("host_id", hostID.String()),
				)
				continue
			}

			if !online {
				logger.Warn("host is offline, skipping",
					zap.String("host_id", hostID.String()),
				)
				// 离线主机直接标记为失败
				s.markTaskAsFailed(taskGroupID, ruleID, hostID, req.ScriptType, "主机离线")
				continue
			}

			// 创建任务日志记录
			taskLog := &model.TaskLog{
				TaskGroupID:   taskGroupID,
				RuleID:        ruleID,
				HostID:        hostID,
				TaskType:      req.ScriptType,
				Status:        "pending",
				ScriptContent: scriptContent,
				ScriptVersion: scriptVersion,
				StartedAt:     pointerToTime(time.Now()),
			}

			if err := s.taskLogRepo.Create(taskLog); err != nil {
				logger.Error("failed to create task log",
					zap.Error(err),
					zap.String("rule_id", ruleID.String()),
					zap.String("host_id", hostID.String()),
				)
				continue
			}

			taskCount++

			logger.Info("task created",
				zap.String("task_id", taskLog.ID.String()),
				zap.String("task_group_id", taskGroupID.String()),
				zap.String("rule_id", ruleID.String()),
				zap.String("host_id", hostID.String()),
			)

			// TODO: 通过 gRPC 下发任务到 Agent
			// 这里先记录日志，实际下发由 gRPC 服务器完成
		}
	}

	logger.Info("task dispatch completed",
		zap.String("task_group_id", taskGroupID.String()),
		zap.Int("total_tasks", taskCount),
	)

	return &TaskDispatchResponse{
		TaskGroupID: taskGroupID,
		TaskCount:   taskCount,
	}, nil
}

// ProcessTaskResult 处理任务执行结果
func (s *TaskService) ProcessTaskResult(ctx context.Context, taskID uuid.UUID, exitCode int, stdout, stderr string) error {
	logger.Info("processing task result",
		zap.String("task_id", taskID.String()),
		zap.Int("exit_code", exitCode),
	)

	// 获取任务日志
	taskLog, err := s.taskLogRepo.FindByID(taskID)
	if err != nil {
		logger.Error("failed to find task log",
			zap.Error(err),
			zap.String("task_id", taskID.String()),
		)
		return err
	}

	// 更新任务结果
	finishedAt := time.Now()
	if err := s.taskLogRepo.UpdateResult(taskID, &stdout, &stderr, &exitCode, getTaskStatus(exitCode), finishedAt); err != nil {
		logger.Error("failed to update task result",
			zap.Error(err),
			zap.String("task_id", taskID.String()),
		)
		return err
	}

	// 判断是否需要触发自愈
	if s.selfHealingService.ShouldTriggerHealing(taskLog.TaskType, exitCode) {
		logger.Info("triggering self-healing",
			zap.String("task_id", taskID.String()),
			zap.String("rule_id", taskLog.RuleID.String()),
			zap.String("host_id", taskLog.HostID.String()),
		)

		// 创建自愈任务
		healingTask := HealingTask{
			OriginalTaskID: taskID,
			RuleID:         taskLog.RuleID,
			HostID:         taskLog.HostID,
			ScriptType:     taskLog.TaskType,
			ScriptContent:  *taskLog.ScriptContent,
			ErrorMessage:   stderr,
			ExitCode:       exitCode,
		}

		// 触发自愈
		if err := s.selfHealingService.TriggerHealing(healingTask); err != nil {
			logger.Error("failed to trigger self-healing",
				zap.Error(err),
				zap.String("task_id", taskID.String()),
			)
			return err
		}
	}

	logger.Info("task result processed",
		zap.String("task_id", taskID.String()),
		zap.Int("exit_code", exitCode),
		zap.Bool("healing_triggered", s.selfHealingService.ShouldTriggerHealing(taskLog.TaskType, exitCode)),
	)

	return nil
}

// GetTaskGroupStatus 获取任务组状态
func (s *TaskService) GetTaskGroupStatus(taskGroupID uuid.UUID) (map[string]int, error) {
	// 获取任务组所有任务
	taskLogs, err := s.taskLogRepo.FindByGroupID(taskGroupID)
	if err != nil {
		return nil, err
	}

	// 统计各状态数量
	statusCount := make(map[string]int)
	for _, task := range taskLogs {
		statusCount[task.Status]++
	}

	return statusCount, nil
}

// CheckHostOnline 检查主机是否在线
func (s *TaskService) CheckHostOnline(hostID uuid.UUID) (bool, error) {
	return s.redisClient.IsOnline(hostID.String())
}

// markTaskAsFailed 标记任务为失败
func (s *TaskService) markTaskAsFailed(taskGroupID, ruleID, hostID uuid.UUID, scriptType, reason string) {
	now := time.Now()
	status := "failed"

	taskLog := &model.TaskLog{
		TaskGroupID: taskGroupID,
		RuleID:      ruleID,
		HostID:      hostID,
		TaskType:    scriptType,
		Status:      status,
		Stderr:      &reason,
		StartedAt:   &now,
		FinishedAt:  &now,
	}

	if err := s.taskLogRepo.Create(taskLog); err != nil {
		logger.Error("failed to create failed task log",
			zap.Error(err),
			zap.String("rule_id", ruleID.String()),
			zap.String("host_id", hostID.String()),
		)
	}
}

// Helper functions
func getTaskStatus(exitCode int) string {
	if exitCode == 0 {
		return "success"
	}
	return "failed"
}

