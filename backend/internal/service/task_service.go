package service

import (
	"context"
	"fmt"
	"time"

	"baseline-system/internal/grpc_server"
	"baseline-system/internal/model"
	"baseline-system/internal/repository"
	"baseline-system/internal/storage"
	"baseline-system/pkg/api/v1"
	"baseline-system/pkg/logger"

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

func (s *TaskService) CreateAndDispatchTasks(ctx context.Context, ruleIDs, hostIDs []string, taskType string) (*TaskCreateResult, error) {
	taskGroupID := uuid.New()
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
		if taskType == "check" {
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
				if taskType == "check" {
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
				RuleID:        ruleID,
				HostID:        hostID,
				TaskType:      taskType,
				Status:        "pending",
				ScriptContent: &scriptContent,
				CreatedAt:     now,
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

		s.ProcessTaskResult(taskID, "", err.Error(), -1, "failed")
		return
	}

	s.ProcessTaskResult(taskID, "", "", 0, "running")
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
				s.ProcessTaskResult(task.ID, "", "任务执行超时（超过5分钟）", -1, "failed")
			}
		}
	}
}

func (s *TaskService) TriggerSelfHealing(taskID uuid.UUID) error {
	taskLog, err := s.taskLogRepo.FindByID(taskID)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	if taskLog.Status != "failed" {
		return fmt.Errorf("task is not in failed state")
	}

	if s.scriptGenService == nil {
		return fmt.Errorf("script generation service not available")
	}

	go s.scriptGenService.GenerateFixScript(context.Background(), taskLog.RuleID)

	return nil
}
