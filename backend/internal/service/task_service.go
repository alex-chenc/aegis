package service

import (
	"context"
	"fmt"
	"time"

	"aegis-system/internal/grpc_server"
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/internal/storage"
	pb "aegis-system/pkg/api/v1"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TaskService struct {
	taskLogRepo      *repository.TaskLogRepository
	hostRepo         *repository.HostRepository
	ruleRepo         *repository.RuleRepository
	healingLogRepo   *repository.HealingLogRepository
	redisClient      *storage.RedisClient
	scriptGenService *ScriptGenerationService
	grpcServer       *grpc_server.GRPCServer
}

type TaskCreateResult struct {
	TaskGroupID uuid.UUID
	TaskIDs     []string
}

func NewTaskService(
	taskLogRepo *repository.TaskLogRepository,
	hostRepo *repository.HostRepository,
	ruleRepo *repository.RuleRepository,
	healingLogRepo *repository.HealingLogRepository,
	redisClient *storage.RedisClient,
	grpcServer *grpc_server.GRPCServer,
) *TaskService {
	return &TaskService{
		taskLogRepo:    taskLogRepo,
		hostRepo:       hostRepo,
		ruleRepo:       ruleRepo,
		healingLogRepo: healingLogRepo,
		redisClient:    redisClient,
		grpcServer:     grpcServer,
	}
}

func (s *TaskService) SetGRPCServer(server *grpc_server.GRPCServer) {
	s.grpcServer = server
}

func (s *TaskService) SetScriptGenService(service *ScriptGenerationService) {
	s.scriptGenService = service
}

func (s *TaskService) GetHostByID(hostID uuid.UUID) (*model.Host, error) {
	return s.hostRepo.FindByID(hostID)
}

func (s *TaskService) CreateAndDispatchTasks(ctx context.Context, ruleIDs, hostIDs []string, taskType string, existingGroupID ...uuid.UUID) (*TaskCreateResult, error) {
	var taskGroupID uuid.UUID
	if len(existingGroupID) > 0 && existingGroupID[0] != uuid.Nil {
		taskGroupID = existingGroupID[0]
	} else {
		taskGroupID = uuid.New()
	}
	var taskIDs []string
	now := time.Now()

	for _, ruleIDStr := range ruleIDs {
		ruleID, err := uuid.Parse(ruleIDStr)
		if err != nil {
			logger.Error("invalid rule_id", zap.String("rule_id", ruleIDStr), zap.Error(err))
			continue
		}

		rule, err := s.ruleRepo.FindByID(ruleID)
		if err != nil {
			logger.Error("failed to find rule", zap.String("rule_id", ruleIDStr), zap.Error(err))
			continue
		}

		var scriptContent string
		if taskType == "CHECK" {
			if rule.GeneratedCheckScript != nil {
				scriptContent = *rule.GeneratedCheckScript
			}
		} else {
			if rule.GeneratedFixScript != nil {
				scriptContent = *rule.GeneratedFixScript
			}
		}

		if scriptContent == "" {
			if s.scriptGenService != nil {
				logger.Info("script not generated, queueing generation",
					zap.String("rule_id", ruleIDStr),
					zap.String("task_type", taskType),
				)
				if taskType == "CHECK" {
					go s.scriptGenService.GenerateCheckScript(context.Background(), ruleID)
				} else {
					go s.scriptGenService.GenerateFixScript(context.Background(), ruleID)
				}
			}
			scriptContent = fmt.Sprintf("# Script generation in progress for %s\n# Please retry in a few seconds\n", taskType)
		}

		for _, hostIDStr := range hostIDs {
			hostID, err := uuid.Parse(hostIDStr)
			if err != nil {
				logger.Error("invalid host_id", zap.String("host_id", hostIDStr), zap.Error(err))
				continue
			}

			taskLog := &model.TaskLog{
				TaskGroupID:   taskGroupID,
				RuleID:        &ruleID,
				HostID:        hostID,
				TaskType:      taskType,
				Status:        "PENDING",
				ScriptContent: &scriptContent,
				CreatedAt:     now,
				StartedAt:     &now,
			}

			if err := s.taskLogRepo.Create(taskLog); err != nil {
				logger.Error("failed to create task log",
					zap.String("rule_id", ruleIDStr),
					zap.String("host_id", hostIDStr),
					zap.Error(err),
				)
				continue
			}

			taskIDs = append(taskIDs, taskLog.ID.String())

			go s.dispatchToAgent(ctx, taskLog.ID, hostID, ruleID, scriptContent, taskType)
		}
	}

	return &TaskCreateResult{
		TaskGroupID: taskGroupID,
		TaskIDs:     taskIDs,
	}, nil
}

func (s *TaskService) RedispatchTask(ctx context.Context, originalTaskID uuid.UUID) (*model.TaskLog, error) {
	originalTask, err := s.taskLogRepo.FindByID(originalTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to find original task: %w", err)
	}

	// Validate task is in a redispatchable state
	// Only allow redispatch for tasks that have reached a terminal state (failed, timeout, success)
	// or are currently pending (can be retried)
	switch originalTask.Status {
	case "FAILED", "TIMEOUT", "SUCCESS":
		// Terminal states - allow redispatch
	case "PENDING", "RUNNING":
		// These states mean the task is still being processed
		// For safety, we still allow redispatch but log a warning
		logger.Warn("redispatching task that may still be running",
			zap.String("task_id", originalTaskID.String()),
			zap.String("current_status", originalTask.Status))
	default:
		return nil, fmt.Errorf("task is not in a redispatchable state: %s", originalTask.Status)
	}

	if originalTask.RuleID == nil {
		return nil, fmt.Errorf("task has no rule_id")
	}

	rule, err := s.ruleRepo.FindByID(*originalTask.RuleID)
	if err != nil {
		return nil, fmt.Errorf("failed to find rule: %w", err)
	}

	var scriptContent string
	if originalTask.TaskType == "CHECK" {
		if rule.GeneratedCheckScript != nil {
			scriptContent = *rule.GeneratedCheckScript
		}
	} else {
		if rule.GeneratedFixScript != nil {
			scriptContent = *rule.GeneratedFixScript
		}
	}

	if scriptContent == "" && originalTask.ScriptContent != nil {
		scriptContent = *originalTask.ScriptContent
	}

	if scriptContent == "" {
		return nil, fmt.Errorf("script content is empty, please regenerate the script first")
	}

	newVersion := 0
	if originalTask.TaskType == "CHECK" {
		newVersion = rule.CheckScriptVersion
	} else {
		newVersion = rule.FixScriptVersion
	}
	if newVersion <= 0 {
		if originalTask.ScriptVersion != nil {
			newVersion = *originalTask.ScriptVersion
		} else {
			newVersion = 1
		}
	}

	if err := s.taskLogRepo.UpdateForRedispatch(originalTask.ID, scriptContent, newVersion); err != nil {
		return nil, fmt.Errorf("failed to update original task for redispatch: %w", err)
	}

	if s.healingLogRepo != nil {
		s.healingLogRepo.DeleteByOriginalTaskIDs([]uuid.UUID{originalTask.ID})
	}
	if s.redisClient != nil {
		s.redisClient.DeleteHealingStatus(originalTask.ID.String())
	}

	updatedTask, err := s.taskLogRepo.FindByID(originalTask.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload redispatched task: %w", err)
	}

	var ruleID uuid.UUID
	if updatedTask.RuleID != nil {
		ruleID = *updatedTask.RuleID
	}
	go s.dispatchToAgent(context.Background(), updatedTask.ID, updatedTask.HostID, ruleID, scriptContent, updatedTask.TaskType)

	return updatedTask, nil
}

func (s *TaskService) dispatchToAgent(ctx context.Context, taskID, hostID, ruleID uuid.UUID, scriptContent, taskType string) {
	logger.Info("dispatching task to agent",
		zap.String("task_id", taskID.String()),
		zap.String("host_id", hostID.String()),
	)

	execute := &pb.CommandExecute{
		TaskId:         taskID.String(),
		RuleId:         ruleID.String(),
		HostId:         hostID.String(),
		ScriptContent:  scriptContent,
		TimeoutSeconds: 300,
	}

	if s.grpcServer == nil {
		logger.Error("grpc server not set", zap.String("task_id", taskID.String()))
		return
	}

	if err := s.grpcServer.SendCommand(hostID, execute); err != nil {
		logger.Error("failed to send command to agent",
			zap.String("task_id", taskID.String()),
			zap.String("host_id", hostID.String()),
			zap.Error(err),
		)

		s.ProcessTaskResult(taskID, "", err.Error(), -1, "FAILED")
		return
	}

	s.ProcessTaskResult(taskID, "", "", 0, "RUNNING")
}

func (s *TaskService) ProcessTaskResult(taskID uuid.UUID, stdout, stderr string, exitCode int, status string) {
	now := time.Now()
	var stdoutPtr, stderrPtr *string
	if stdout != "" {
		stdoutPtr = &stdout
	}
	if stderr != "" {
		stderrPtr = &stderr
	}

	if err := s.taskLogRepo.UpdateResult(taskID, stdoutPtr, stderrPtr, &exitCode, status, now); err != nil {
		logger.Error("failed to update task result",
			zap.String("task_id", taskID.String()),
			zap.Error(err),
		)
	}

	logger.Info("task result processed",
		zap.String("task_id", taskID.String()),
		zap.String("status", status),
		zap.Int("exit_code", exitCode),
	)
}

func (s *TaskService) CheckTimeoutTasks() {
	tasks, err := s.taskLogRepo.FindRunningTasks()
	if err != nil {
		logger.Error("failed to find running tasks", zap.Error(err))
		return
	}

	timeout := 5 * time.Minute
	now := time.Now()

	for _, task := range tasks {
		if task.StartedAt != nil {
			elapsed := now.Sub(*task.StartedAt)
			if elapsed > timeout {
				logger.Warn("task timeout, marking as failed",
					zap.String("task_id", task.ID.String()),
					zap.Duration("elapsed", elapsed),
				)
				s.ProcessTaskResult(task.ID, "", "任务执行超时（超过5分钟）", -1, "TIMEOUT")
			}
		}
	}
}

func (s *TaskService) TriggerSelfHealing(taskID uuid.UUID) error {
	taskLog, err := s.taskLogRepo.FindByID(taskID)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	if taskLog.Status != "FAILED" {
		return fmt.Errorf("task is not in failed state")
	}

	if s.scriptGenService == nil {
		return fmt.Errorf("script generation service not available")
	}

	if taskLog.RuleID != nil {
		go s.scriptGenService.GenerateFixScript(context.Background(), *taskLog.RuleID)
	}

	return nil
}

const TaskTimeout = 5 * time.Minute

func (s *TaskService) StartTimeoutChecker() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			s.checkAndMarkTimedOutTasks()
		}
	}()
	logger.Info("task timeout checker started", zap.Duration("timeout", TaskTimeout))
}

func (s *TaskService) checkAndMarkTimedOutTasks() {
	tasks, err := s.taskLogRepo.FindTimedOutTasks(TaskTimeout)
	if err != nil {
		logger.Error("failed to check timed out tasks", zap.Error(err))
		return
	}

	if len(tasks) == 0 {
		return
	}

	var taskIDs []uuid.UUID
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}

	count, err := s.taskLogRepo.MarkAsTimedOut(taskIDs)
	if err != nil {
		logger.Error("failed to mark tasks as timed out", zap.Error(err))
		return
	}

	logger.Info("marked tasks as timed out", zap.Int64("count", count))
}
