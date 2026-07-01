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
	taskLogRepo      *repository.TaskLogRepository
	hostRepo         *repository.HostRepository
	ruleRepo         *repository.RuleRepository
	healingLogRepo   *repository.HealingLogRepository
	redisClient      *storage.RedisClient
	scriptGenService *ScriptGenerationService
	selfHealingSvc   *SelfHealingService
	autoVerifySvc    *AutoVerifyService
	auditService     *ScriptAuditService
	serverClient     *grpcclient.ServerClient
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

func (s *TaskService) SetSelfHealingService(service *SelfHealingService) {
	s.selfHealingSvc = service
}

func (s *TaskService) SetAutoVerifyService(service *AutoVerifyService) {
	s.autoVerifySvc = service
}

func (s *TaskService) SetAuditService(service *ScriptAuditService) {
	s.auditService = service
}

func (s *TaskService) GetHostByID(hostID uuid.UUID) (*model.Host, error) {
	return s.hostRepo.FindByID(hostID)
}

type DispatchOptions struct {
	AutoVerify bool
	MaxRounds  int
}

func (s *TaskService) CreateAndDispatchTasks(ctx context.Context, ruleIDs, hostIDs []string, taskType string, opts *DispatchOptions, existingGroupID ...uuid.UUID) (*TaskCreateResult, error) {
	var taskGroupID uuid.UUID
	if len(existingGroupID) > 0 && existingGroupID[0] != uuid.Nil {
		taskGroupID = existingGroupID[0]
	} else {
		taskGroupID = uuid.New()
	}
	var taskIDs []string
	now := time.Now()

	autoVerify := false
	maxRounds := 3
	if opts != nil {
		autoVerify = opts.AutoVerify
		if opts.MaxRounds > 0 {
			maxRounds = opts.MaxRounds
		}
	}

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
				AutoVerify:    autoVerify,
				VerifyRound:   0,
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

	taskLog, findErr := s.taskLogRepo.FindByID(taskID)
	normalizedStatus := strings.ToUpper(status)
	if findErr == nil {
		normalizedStatus = NormalizeTaskResultStatus(taskLog.TaskType, status, exitCode, stderr)
	} else {
		logger.Warn("failed to load task before processing result",
			zap.String("task_id", taskID.String()),
			zap.Error(findErr),
		)
	}

	if err := s.taskLogRepo.UpdateResult(taskID, stdoutPtr, stderrPtr, &exitCode, normalizedStatus, now); err != nil {
		logger.Error("failed to update task result",
			zap.String("task_id", taskID.String()),
			zap.Error(err),
		)
	}

	logger.Info("task result processed",
		zap.String("task_id", taskID.String()),
		zap.String("status", normalizedStatus),
		zap.Int("exit_code", exitCode),
	)

	if findErr == nil && IsTerminalTaskStatus(normalizedStatus) {
		s.maybeTriggerLargeModelRepair(taskLog, normalizedStatus, exitCode, stdout, stderr)
	}

	// Auto-verify: trigger detection-repair loop if enabled
	if findErr == nil && IsTerminalTaskStatus(normalizedStatus) && s.autoVerifySvc != nil {
		s.autoVerifySvc.HandleTaskResult(taskLog, normalizedStatus, exitCode)
	}
}

func (s *TaskService) maybeTriggerLargeModelRepair(taskLog *model.TaskLog, status string, exitCode int, stdout, stderr string) {
	if s.selfHealingSvc == nil {
		return
	}
	if taskLog.HealingID != nil {
		logger.Debug("task is already attached to large-model repair, skip auto trigger",
			zap.String("task_id", taskLog.ID.String()),
			zap.String("healing_id", taskLog.HealingID.String()),
		)
		return
	}
	if s.redisClient != nil {
		if status, err := s.redisClient.GetHealingStatusStruct(taskLog.ID.String()); err == nil && status != nil {
			logger.Debug("task already has large-model repair status, skip auto trigger",
				zap.String("task_id", taskLog.ID.String()),
				zap.String("healing_status", status.Status),
			)
			return
		}
	}
	if s.healingLogRepo != nil {
		healingLog, err := s.healingLogRepo.GetLatestByOriginalTaskID(taskLog.ID)
		if err == nil && healingLog != nil {
			logger.Debug("task already has large-model repair log, skip auto trigger",
				zap.String("task_id", taskLog.ID.String()),
				zap.String("healing_status", healingLog.Status),
			)
			return
		}
	}
	exitCodePtr := &exitCode
	if !IsLLMRepairableTask(taskLog.TaskType, status, exitCodePtr, stderr) {
		return
	}

	scriptContent := ""
	if taskLog.ScriptContent != nil {
		scriptContent = *taskLog.ScriptContent
	}
	if strings.TrimSpace(scriptContent) == "" {
		logger.Warn("skip large-model repair because task script is empty",
			zap.String("task_id", taskLog.ID.String()),
			zap.String("task_type", taskLog.TaskType),
		)
		return
	}

	errMsg := strings.TrimSpace(stderr)
	if errMsg == "" {
		errMsg = strings.TrimSpace(stdout)
	}
	if errMsg == "" {
		errMsg = fmt.Sprintf("task ended with status %s and exit code %d", status, exitCode)
	}

	scriptType := strings.ToUpper(taskLog.TaskType)
	if scriptType == "VULNERABILITY_FIX" {
		scriptType = "FIX"
	}
	if scriptType == "POC_VERIFY" {
		scriptType = "POC"
	}

	healingTask := HealingTask{
		OriginalTaskID:  taskLog.ID,
		RuleID:          taskLog.RuleID,
		VulnerabilityID: taskLog.VulnerabilityID,
		HostID:          taskLog.HostID,
		ScriptType:      scriptType,
		ScriptContent:   scriptContent,
		ErrorMessage:    errMsg,
		ExitCode:        exitCode,
		AttemptNo:       taskLog.AttemptNo,
		MaxRounds:       taskLog.MaxRounds,
	}
	if err := s.selfHealingSvc.TriggerHealing(healingTask); err != nil {
		logger.Error("failed to trigger large-model repair",
			zap.String("task_id", taskLog.ID.String()),
			zap.String("task_type", taskLog.TaskType),
			zap.Error(err),
		)
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

	if !IsLLMRepairableTask(taskLog.TaskType, taskLog.Status, taskLog.ExitCode, stringValue(taskLog.Stderr)) {
		return fmt.Errorf("task is not in a large-model repairable execution failure state")
	}

	if s.scriptGenService == nil {
		return fmt.Errorf("script generation service not available")
	}

	if taskLog.RuleID != nil {
		go s.scriptGenService.GenerateFixScript(context.Background(), *taskLog.RuleID)
	}

	return nil
}

func (s *TaskService) DispatchHealedTask(ctx context.Context, taskID uuid.UUID, scriptContent string, scriptVersion int, healingID uuid.UUID, attemptNo int) error {
	taskLog, err := s.taskLogRepo.FindByID(taskID)
	if err != nil {
		return fmt.Errorf("failed to find task for healing redispatch: %w", err)
	}
	if err := s.taskLogRepo.UpdateForHealingRedispatch(taskID, healingID, scriptContent, scriptVersion, attemptNo); err != nil {
		return err
	}

	var ruleID uuid.UUID
	if taskLog.RuleID != nil {
		ruleID = *taskLog.RuleID
	}
	logger.Info("redispatching task after large-model repair",
		zap.String("task_id", taskID.String()),
		zap.String("task_type", taskLog.TaskType),
		zap.Int("attempt_no", attemptNo),
	)
	go s.dispatchToAgent(context.Background(), taskID, taskLog.HostID, ruleID, scriptContent, taskLog.TaskType)
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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

func formatAuditBlockReason(hits []checker.BlacklistHit) string {
	var sb strings.Builder
	sb.WriteString("脚本存在恶意命令，下发已阻止。\n命中规则：")
	for i, hit := range hits {
		sb.WriteString(fmt.Sprintf("\n  %d. [%s] %s (第%d行, 匹配: %s)",
			i+1, hit.Severity, hit.RuleName, hit.LineNumber, hit.MatchedText))
	}
	return sb.String()
}
