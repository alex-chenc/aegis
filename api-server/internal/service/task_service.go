package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api-server/internal/checker"
	grpcclient "api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/storage"
	pb "api-server/pkg/api/v1"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TaskService struct {
	taskLogRepo        *repository.TaskLogRepository
	hostRepo           *repository.HostRepository
	ruleRepo           *repository.RuleRepository
	healingLogRepo     *repository.HealingLogRepository
	redisClient        *storage.RedisClient
	scriptGenService   *ScriptGenerationService
	auditService       *ScriptAuditService
	selfHealingService *SelfHealingService
	serverClient       *grpcclient.ServerClient
}

type TaskCreateResult struct {
	TaskGroupID uuid.UUID
	TaskIDs     []string
}

type TaskDispatchOptions struct {
	TaskType        string
	ExistingGroupID uuid.UUID
	MaxRounds       int
}

const (
	minBaselineTaskRounds = 1
	maxBaselineTaskRounds = 10
)

func NewTaskService(
	taskLogRepo *repository.TaskLogRepository,
	hostRepo *repository.HostRepository,
	ruleRepo *repository.RuleRepository,
	healingLogRepo *repository.HealingLogRepository,
	redisClient *storage.RedisClient,
	serverClient *grpcclient.ServerClient,
) *TaskService {
	return &TaskService{
		taskLogRepo:    taskLogRepo,
		hostRepo:       hostRepo,
		ruleRepo:       ruleRepo,
		healingLogRepo: healingLogRepo,
		redisClient:    redisClient,
		serverClient:   serverClient,
	}
}

func (s *TaskService) SetScriptGenService(service *ScriptGenerationService) {
	s.scriptGenService = service
}

func (s *TaskService) SetAuditService(service *ScriptAuditService) {
	s.auditService = service
}

func (s *TaskService) SetSelfHealingService(service *SelfHealingService) {
	s.selfHealingService = service
}

func (s *TaskService) GetHostByID(hostID uuid.UUID) (*model.Host, error) {
	return s.hostRepo.FindByID(hostID)
}

func (s *TaskService) CreateAndDispatchTasks(ctx context.Context, ruleIDs, hostIDs []string, taskType string, existingGroupID ...uuid.UUID) (*TaskCreateResult, error) {
	options := TaskDispatchOptions{TaskType: taskType, MaxRounds: 1}
	if len(existingGroupID) > 0 && existingGroupID[0] != uuid.Nil {
		options.ExistingGroupID = existingGroupID[0]
	}
	return s.CreateAndDispatchTasksWithOptions(ctx, ruleIDs, hostIDs, options)
}

func (s *TaskService) CreateAndDispatchTasksWithOptions(ctx context.Context, ruleIDs, hostIDs []string, options TaskDispatchOptions) (*TaskCreateResult, error) {
	var taskGroupID uuid.UUID
	if options.ExistingGroupID != uuid.Nil {
		taskGroupID = options.ExistingGroupID
	} else {
		taskGroupID = uuid.New()
	}
	taskType := strings.ToUpper(options.TaskType)
	maxRounds := normalizeTaskRounds(options.MaxRounds)
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
				AttemptNo:     1,
				MaxRounds:     maxRounds,
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

			go s.dispatchToAgent(context.Background(), taskLog.ID, hostID, ruleID, scriptContent, taskType)
		}
	}

	return &TaskCreateResult{
		TaskGroupID: taskGroupID,
		TaskIDs:     taskIDs,
	}, nil
}

func normalizeTaskRounds(rounds int) int {
	if rounds < minBaselineTaskRounds {
		return minBaselineTaskRounds
	}
	if rounds > maxBaselineTaskRounds {
		return maxBaselineTaskRounds
	}
	return rounds
}

func (s *TaskService) dispatchNextRound(ctx context.Context, previous model.TaskLog) error {
	if previous.AttemptNo <= 0 {
		previous.AttemptNo = 1
	}
	if previous.MaxRounds <= 0 {
		previous.MaxRounds = 1
	}
	nextAttempt := previous.AttemptNo + 1
	if nextAttempt > previous.MaxRounds {
		return nil
	}

	exists, err := s.taskLogRepo.ExistsRound(previous, nextAttempt)
	if err != nil {
		return fmt.Errorf("failed to check next round existence: %w", err)
	}
	if exists {
		return nil
	}

	scriptContent := ""
	if previous.ScriptContent != nil {
		scriptContent = *previous.ScriptContent
	}
	if scriptContent == "" {
		return fmt.Errorf("task %s has empty script content", previous.ID.String())
	}

	now := time.Now()
	nextTask := &model.TaskLog{
		TaskGroupID:     previous.TaskGroupID,
		RuleID:          previous.RuleID,
		HostID:          previous.HostID,
		VulnerabilityID: previous.VulnerabilityID,
		TaskType:        previous.TaskType,
		Status:          "PENDING",
		ScriptContent:   &scriptContent,
		ScriptVersion:   previous.ScriptVersion,
		AttemptNo:       nextAttempt,
		MaxRounds:       previous.MaxRounds,
		CreatedAt:       now,
		StartedAt:       &now,
	}
	if err := s.taskLogRepo.Create(nextTask); err != nil {
		return fmt.Errorf("failed to create next round task: %w", err)
	}

	var ruleID uuid.UUID
	if previous.RuleID != nil {
		ruleID = *previous.RuleID
	}
	logger.Info("dispatching baseline task next round",
		zap.String("previous_task_id", previous.ID.String()),
		zap.String("next_task_id", nextTask.ID.String()),
		zap.String("task_group_id", previous.TaskGroupID.String()),
		zap.String("task_type", previous.TaskType),
		zap.Int("attempt_no", nextAttempt),
		zap.Int("max_rounds", previous.MaxRounds))
	go s.dispatchToAgent(ctx, nextTask.ID, previous.HostID, ruleID, scriptContent, previous.TaskType)
	return nil
}

func (s *TaskService) CreateAndDispatchLoopTask(ctx context.Context, taskGroupID uuid.UUID, hostID uuid.UUID, ruleID *uuid.UUID, taskType string, scriptContent string, attemptNo int, maxRounds int, healingID *uuid.UUID) (*model.TaskLog, error) {
	taskType = strings.ToUpper(taskType)
	if taskType != "CHECK" && taskType != "FIX" {
		return nil, fmt.Errorf("unsupported baseline loop task type: %s", taskType)
	}
	if ruleID == nil {
		return nil, fmt.Errorf("baseline loop task requires rule_id")
	}
	if attemptNo <= 0 {
		attemptNo = 1
	}
	maxRounds = normalizeTaskRounds(maxRounds)

	if strings.TrimSpace(scriptContent) == "" {
		rule, err := s.ruleRepo.FindByID(*ruleID)
		if err != nil {
			return nil, fmt.Errorf("failed to find rule script: %w", err)
		}
		if taskType == "CHECK" && rule.GeneratedCheckScript != nil {
			scriptContent = *rule.GeneratedCheckScript
		}
		if taskType == "FIX" && rule.GeneratedFixScript != nil {
			scriptContent = *rule.GeneratedFixScript
		}
	}
	if strings.TrimSpace(scriptContent) == "" {
		return nil, fmt.Errorf("%s script content is empty", taskType)
	}

	exists, err := s.loopRoundExists(taskGroupID, hostID, ruleID, nil, taskType, attemptNo)
	if err != nil {
		return nil, err
	}
	if exists {
		logger.Info("baseline loop task already exists",
			zap.String("task_group_id", taskGroupID.String()),
			zap.String("host_id", hostID.String()),
			zap.String("rule_id", ruleID.String()),
			zap.String("task_type", taskType),
			zap.Int("attempt_no", attemptNo),
		)
		return nil, nil
	}

	now := time.Now()
	taskLog := &model.TaskLog{
		TaskGroupID:   taskGroupID,
		RuleID:        ruleID,
		HostID:        hostID,
		TaskType:      taskType,
		Status:        "PENDING",
		ScriptContent: &scriptContent,
		AttemptNo:     attemptNo,
		MaxRounds:     maxRounds,
		HealingID:     healingID,
		CreatedAt:     now,
		StartedAt:     &now,
	}
	if err := s.taskLogRepo.Create(taskLog); err != nil {
		return nil, fmt.Errorf("failed to create baseline loop task: %w", err)
	}

	logger.Info("dispatching baseline loop task",
		zap.String("task_id", taskLog.ID.String()),
		zap.String("task_group_id", taskGroupID.String()),
		zap.String("host_id", hostID.String()),
		zap.String("rule_id", ruleID.String()),
		zap.String("task_type", taskType),
		zap.Int("attempt_no", attemptNo),
		zap.Int("max_rounds", maxRounds),
	)
	go s.dispatchToAgent(ctx, taskLog.ID, hostID, *ruleID, scriptContent, taskType)
	return taskLog, nil
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

	var scriptContent string
	var newVersion int

	// For CHECK tasks, we need a rule_id to get the script
	// For POC_VERIFY and VULNERABILITY_FIX tasks, we use the original task's ScriptContent
	if originalTask.TaskType == "CHECK" {
		if originalTask.RuleID == nil {
			return nil, fmt.Errorf("CHECK task has no rule_id")
		}

		rule, err := s.ruleRepo.FindByID(*originalTask.RuleID)
		if err != nil {
			return nil, fmt.Errorf("failed to find rule: %w", err)
		}

		if rule.GeneratedCheckScript != nil {
			scriptContent = *rule.GeneratedCheckScript
		}

		newVersion = rule.CheckScriptVersion
	} else {
		// For POC_VERIFY and VULNERABILITY_FIX, use the original task's ScriptContent
		if originalTask.ScriptContent != nil {
			scriptContent = *originalTask.ScriptContent
		}

		if originalTask.ScriptVersion != nil {
			newVersion = *originalTask.ScriptVersion
		} else {
			newVersion = 1
		}
	}

	if scriptContent == "" {
		return nil, fmt.Errorf("script content is empty, please regenerate the script first")
	}

	if newVersion <= 0 {
		newVersion = 1
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

	if s.serverClient == nil {
		logger.Error("server gRPC client not set", zap.String("task_id", taskID.String()))
		s.ProcessTaskResult(taskID, "", "gRPC client not configured", -1, "FAILED")
		return
	}

	// V5.7: Pre-dispatch blacklist audit
	if s.auditService != nil {
		auditResult, err := s.auditService.AuditForDispatch(ctx, scriptContent, taskID.String())
		if err != nil {
			logger.Error("pre-dispatch audit error, proceeding (fail-open)",
				zap.String("task_id", taskID.String()),
				zap.Error(err),
			)
		} else if auditResult != nil && !auditResult.Passed {
			blockReason := formatAuditBlockReason(auditResult.BlacklistHits)
			logger.Warn("script blocked by pre-dispatch audit",
				zap.String("task_id", taskID.String()),
				zap.String("reason", blockReason),
			)
			now := time.Now()
			s.taskLogRepo.UpdateResult(taskID, nil, &blockReason, nil, "AUDIT_BLOCKED", now)
			go s.handleBaselineRepairLoop(context.Background(), taskID)
			return
		}
	}

	// Use background context for async dispatch - the HTTP request context
	// will be canceled when the handler returns, but we need the gRPC
	// call to continue running until completion
	bgCtx := context.Background()

	_, err := s.serverClient.ForwardCommand(bgCtx, &pb.ForwardCommandRequest{
		TaskId:         taskID.String(),
		HostId:         hostID.String(),
		RuleId:         ruleID.String(),
		ScriptContent:  scriptContent,
		TimeoutSeconds: 300,
		TaskType:       taskType,
	})

	if err != nil {
		logger.Error("failed to forward command to server",
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

	if isTerminalTaskStatus(status) {
		go s.handleBaselineRepairLoop(context.Background(), taskID)
	}
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

func (s *TaskService) StartRoundRetryChecker() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			s.checkAndDispatchNextRounds()
		}
	}()
	logger.Info("baseline task round retry checker started")
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

func (s *TaskService) checkAndDispatchNextRounds() {
	tasks, err := s.taskLogRepo.FindRoundRetryCandidates(50)
	if err != nil {
		logger.Error("failed to find baseline round retry candidates", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	for _, task := range tasks {
		if err := s.dispatchNextRound(context.Background(), task); err != nil {
			logger.Error("failed to dispatch next baseline task round",
				zap.String("task_id", task.ID.String()),
				zap.String("task_group_id", task.TaskGroupID.String()),
				zap.Int("attempt_no", task.AttemptNo),
				zap.Int("max_rounds", task.MaxRounds),
				zap.Error(err))
		}
	}
}

func (s *TaskService) handleBaselineRepairLoop(ctx context.Context, taskID uuid.UUID) {
	if s.selfHealingService == nil {
		return
	}

	task, err := s.taskLogRepo.FindByID(taskID)
	if err != nil {
		logger.Error("failed to reload task for baseline repair loop",
			zap.String("task_id", taskID.String()),
			zap.Error(err))
		return
	}
	if task.RuleID == nil {
		return
	}
	taskType := strings.ToUpper(task.TaskType)
	if taskType != "CHECK" && taskType != "FIX" {
		return
	}
	if task.AttemptNo <= 0 {
		task.AttemptNo = 1
	}
	if task.MaxRounds <= 0 {
		task.MaxRounds = 1
	}
	if !isTerminalTaskStatus(task.Status) {
		return
	}

	originTaskID := task.ID
	if task.HealingID != nil {
		if origin, originErr := s.selfHealingService.GetHealingOriginTaskID(*task.HealingID); originErr == nil {
			originTaskID = origin
		}
	}

	switch taskType {
	case "CHECK":
		if isTaskExecutionPassed(task) {
			if task.HealingID != nil {
				s.selfHealingService.MarkHealingSucceededByID(ctx, *task.HealingID, task.AttemptNo, "复检通过，基线修复闭环完成")
			}
			return
		}

		if task.HealingID != nil {
			s.selfHealingService.AppendHealingStepByID(*task.HealingID, "verify_check", "failed", fmt.Sprintf("第 %d 轮复检仍未通过：%s", task.AttemptNo, summarizeTaskFailure(task)))
		}
		if task.AttemptNo >= task.MaxRounds {
			if task.HealingID != nil {
				s.selfHealingService.MarkHealingFailedByID(ctx, *task.HealingID, "复检失败且已达到最大轮数")
			}
			return
		}

		nextAttempt := task.AttemptNo + 1
		exists, err := s.loopRoundExists(task.TaskGroupID, task.HostID, task.RuleID, task.VulnerabilityID, "FIX", nextAttempt)
		if err != nil {
			logger.Error("failed to check existing next fix round", zap.Error(err), zap.String("task_id", task.ID.String()))
			return
		}
		if exists {
			return
		}
		s.triggerBaselineHealing(ctx, *task, originTaskID, nextAttempt)

	case "FIX":
		if isTaskExecutionPassed(task) {
			if task.HealingID != nil {
				s.selfHealingService.AppendHealingStepByID(*task.HealingID, "fix_result", "success", fmt.Sprintf("第 %d 轮修复脚本执行成功，开始下发复检任务", task.AttemptNo))
			}
			rule, err := s.ruleRepo.FindByID(*task.RuleID)
			if err != nil {
				if task.HealingID != nil {
					s.selfHealingService.MarkHealingFailedByID(ctx, *task.HealingID, fmt.Sprintf("修复成功但读取检测脚本失败：%v", err))
				}
				return
			}
			checkScript := ""
			if rule.GeneratedCheckScript != nil {
				checkScript = *rule.GeneratedCheckScript
			}
			if strings.TrimSpace(checkScript) == "" {
				if task.HealingID != nil {
					s.selfHealingService.MarkHealingFailedByID(ctx, *task.HealingID, "修复成功但检测脚本为空，无法复检")
				}
				return
			}
			if _, err := s.CreateAndDispatchLoopTask(ctx, task.TaskGroupID, task.HostID, task.RuleID, "CHECK", checkScript, task.AttemptNo, task.MaxRounds, task.HealingID); err != nil {
				logger.Error("failed to dispatch baseline verification check",
					zap.String("task_id", task.ID.String()),
					zap.Error(err))
				if task.HealingID != nil {
					s.selfHealingService.MarkHealingFailedByID(ctx, *task.HealingID, fmt.Sprintf("复检任务下发失败：%v", err))
				}
			}
			return
		}

		if task.HealingID != nil {
			s.selfHealingService.AppendHealingStepByID(*task.HealingID, "fix_result", "failed", fmt.Sprintf("第 %d 轮修复执行失败：%s", task.AttemptNo, summarizeTaskFailure(task)))
		}
		if task.AttemptNo >= task.MaxRounds {
			if task.HealingID != nil {
				s.selfHealingService.MarkHealingFailedByID(ctx, *task.HealingID, "修复执行失败且已达到最大轮数")
			}
			return
		}

		nextAttempt := task.AttemptNo + 1
		exists, err := s.loopRoundExists(task.TaskGroupID, task.HostID, task.RuleID, task.VulnerabilityID, "FIX", nextAttempt)
		if err != nil {
			logger.Error("failed to check existing next fix round", zap.Error(err), zap.String("task_id", task.ID.String()))
			return
		}
		if exists {
			return
		}
		s.triggerBaselineHealing(ctx, *task, originTaskID, nextAttempt)
	}
}

func (s *TaskService) triggerBaselineHealing(ctx context.Context, source model.TaskLog, originTaskID uuid.UUID, nextAttempt int) {
	if s.selfHealingService == nil {
		return
	}
	if source.RuleID == nil {
		return
	}
	scriptContent := ""
	if source.ScriptContent != nil {
		scriptContent = *source.ScriptContent
	}
	exitCode := -1
	if source.ExitCode != nil {
		exitCode = *source.ExitCode
	}
	healingTask := HealingTask{
		OriginalTaskID:  originTaskID,
		RuleID:          source.RuleID,
		VulnerabilityID: source.VulnerabilityID,
		HostID:          source.HostID,
		ScriptType:      "FIX",
		ScriptContent:   scriptContent,
		ErrorMessage:    summarizeTaskFailure(&source),
		ExitCode:        exitCode,
		TaskGroupID:     source.TaskGroupID,
		AttemptNo:       nextAttempt,
		MaxRounds:       source.MaxRounds,
	}
	if source.HealingID != nil {
		s.selfHealingService.AppendHealingStepByID(*source.HealingID, "next_round", "queued", fmt.Sprintf("准备进入第 %d/%d 轮 ReAct 修复", nextAttempt, source.MaxRounds))
	}
	if err := s.selfHealingService.TriggerHealing(healingTask); err != nil {
		logger.Error("failed to trigger baseline ReAct healing",
			zap.String("task_id", source.ID.String()),
			zap.String("origin_task_id", originTaskID.String()),
			zap.Int("next_attempt", nextAttempt),
			zap.Error(err))
		if source.HealingID != nil {
			s.selfHealingService.MarkHealingFailedByID(ctx, *source.HealingID, fmt.Sprintf("触发下一轮 ReAct 修复失败：%v", err))
		}
	}
}

func (s *TaskService) loopRoundExists(taskGroupID uuid.UUID, hostID uuid.UUID, ruleID *uuid.UUID, vulnerabilityID *uuid.UUID, taskType string, attemptNo int) (bool, error) {
	task := model.TaskLog{
		TaskGroupID:     taskGroupID,
		HostID:          hostID,
		RuleID:          ruleID,
		VulnerabilityID: vulnerabilityID,
		TaskType:        strings.ToUpper(taskType),
	}
	return s.taskLogRepo.ExistsRound(task, attemptNo)
}

func isTerminalTaskStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "SUCCESS", "FAILED", "TIMEOUT", "AUDIT_BLOCKED":
		return true
	default:
		return false
	}
}

func isTaskExecutionPassed(task *model.TaskLog) bool {
	if task == nil || strings.ToUpper(task.Status) != "SUCCESS" {
		return false
	}
	exitCode := 0
	if task.ExitCode != nil {
		exitCode = *task.ExitCode
	}
	return exitCode == 0
}

func summarizeTaskFailure(task *model.TaskLog) string {
	if task == nil {
		return "未知失败"
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("状态=%s", task.Status))
	if task.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("退出码=%d", *task.ExitCode))
	}
	if task.Stderr != nil && strings.TrimSpace(*task.Stderr) != "" {
		parts = append(parts, "stderr="+truncateTaskLogText(*task.Stderr, 600))
	}
	if task.Stdout != nil && strings.TrimSpace(*task.Stdout) != "" {
		parts = append(parts, "stdout="+truncateTaskLogText(*task.Stdout, 600))
	}
	return strings.Join(parts, "；")
}

func truncateTaskLogText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func formatAuditBlockReason(hits []checker.BlacklistHit) string {
	var sb strings.Builder
	sb.WriteString("脚本存在恶意命令，下发已阻止。\n命中规则：")
	for i, hit := range hits {
		sb.WriteString(fmt.Sprintf("\n  %d. [%s] %s (第%d行, 匹配: %s)",
			i+1, hit.Severity, hit.RuleName, hit.LineNumber, hit.MatchedText))
	}
	return sb.String()
}
