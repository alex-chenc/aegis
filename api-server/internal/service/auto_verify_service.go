package service

import (
	"context"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AutoVerifyService handles automatic detection-repair-verification loops.
// Flow for CHECK tasks: CHECK fails → FIX → CHECK → ... until pass or max rounds.
// Flow for FIX tasks: FIX succeeds → CHECK → if fail → FIX → ... until pass or max rounds.
type AutoVerifyService struct {
	taskLogRepo  *repository.TaskLogRepository
	ruleRepo     *repository.RuleRepository
	taskService  *TaskService
}

func NewAutoVerifyService(
	taskLogRepo *repository.TaskLogRepository,
	ruleRepo *repository.RuleRepository,
	taskService *TaskService,
) *AutoVerifyService {
	return &AutoVerifyService{
		taskLogRepo: taskLogRepo,
		ruleRepo:    ruleRepo,
		taskService: taskService,
	}
}

// HandleTaskResult checks if auto-verification should be triggered after a task completes.
// Called from TaskService.ProcessTaskResult after updating the task result.
func (s *AutoVerifyService) HandleTaskResult(taskLog *model.TaskLog, normalizedStatus string, exitCode int) {
	if !taskLog.AutoVerify {
		return
	}

	taskType := strings.ToUpper(strings.TrimSpace(taskLog.TaskType))
	maxRounds := taskLog.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}
	currentRound := taskLog.VerifyRound

	logger.Info("auto-verify checking task result",
		zap.String("task_id", taskLog.ID.String()),
		zap.String("task_type", taskType),
		zap.String("status", normalizedStatus),
		zap.Int("exit_code", exitCode),
		zap.Int("verify_round", currentRound),
		zap.Int("max_rounds", maxRounds),
	)

	switch taskType {
	case "CHECK":
		s.handleCheckResult(taskLog, normalizedStatus, exitCode, currentRound, maxRounds)
	case "FIX":
		s.handleFixResult(taskLog, normalizedStatus, exitCode, currentRound, maxRounds)
	}
}

// handleCheckResult handles CHECK task completion in auto-verify mode.
// If CHECK passed (exit_code=0), verification is complete.
// If CHECK failed (exit_code=1, non-compliant), trigger FIX and continue.
func (s *AutoVerifyService) handleCheckResult(taskLog *model.TaskLog, status string, exitCode, currentRound, maxRounds int) {
	if status != "SUCCESS" {
		// CHECK task itself failed (execution error, timeout, etc.) - stop auto-verify
		logger.Info("auto-verify stopped: CHECK task execution failed",
			zap.String("task_id", taskLog.ID.String()),
			zap.String("status", status),
		)
		return
	}

	if exitCode == 0 {
		// CHECK passed - verification complete!
		logger.Info("auto-verify completed: CHECK passed",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("total_rounds", currentRound),
		)
		return
	}

	// CHECK failed (exit_code=1, non-compliant) - need to FIX
	if currentRound >= maxRounds {
		logger.Warn("auto-verify stopped: max rounds reached after CHECK",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("verify_round", currentRound),
			zap.Int("max_rounds", maxRounds),
		)
		return
	}

	logger.Info("auto-verify: CHECK failed, triggering FIX",
		zap.String("task_id", taskLog.ID.String()),
		zap.Int("next_round", currentRound+1),
	)

	go s.triggerFixForVerify(taskLog, currentRound+1)
}

// handleFixResult handles FIX task completion in auto-verify mode.
// If FIX succeeded (exit_code=0), trigger CHECK to verify.
// If FIX failed, trigger another FIX attempt (up to max rounds).
func (s *AutoVerifyService) handleFixResult(taskLog *model.TaskLog, status string, exitCode, currentRound, maxRounds int) {
	if status == "SUCCESS" && exitCode == 0 {
		// FIX succeeded - trigger CHECK to verify the fix
		logger.Info("auto-verify: FIX succeeded, triggering CHECK verification",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("verify_round", currentRound),
		)
		go s.triggerCheckForVerify(taskLog, currentRound)
		return
	}

	// FIX failed - try another FIX if rounds remain
	if currentRound >= maxRounds {
		logger.Warn("auto-verify stopped: max rounds reached after FIX failure",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("verify_round", currentRound),
			zap.Int("max_rounds", maxRounds),
		)
		return
	}

	logger.Info("auto-verify: FIX failed, triggering another FIX attempt",
		zap.String("task_id", taskLog.ID.String()),
		zap.Int("next_round", currentRound+1),
	)

	go s.triggerFixForVerify(taskLog, currentRound+1)
}

// triggerFixForVerify creates and dispatches a FIX task for auto-verification.
func (s *AutoVerifyService) triggerFixForVerify(originalTask *model.TaskLog, round int) {
	ctx := context.Background()

	if originalTask.RuleID == nil {
		logger.Error("auto-verify: cannot trigger FIX, no rule_id",
			zap.String("task_id", originalTask.ID.String()),
		)
		return
	}

	rule, err := s.ruleRepo.FindByID(*originalTask.RuleID)
	if err != nil {
		logger.Error("auto-verify: failed to find rule for FIX",
			zap.String("task_id", originalTask.ID.String()),
			zap.String("rule_id", originalTask.RuleID.String()),
			zap.Error(err),
		)
		return
	}

	var scriptContent string
	if rule.GeneratedFixScript != nil {
		scriptContent = *rule.GeneratedFixScript
	}
	if scriptContent == "" {
		logger.Error("auto-verify: no FIX script available",
			zap.String("rule_id", originalTask.RuleID.String()),
		)
		return
	}

	hostIDStr := originalTask.HostID.String()
	ruleIDStr := originalTask.RuleID.String()

	fixTask := &model.TaskLog{
		TaskGroupID:   originalTask.TaskGroupID,
		RuleID:        originalTask.RuleID,
		HostID:        originalTask.HostID,
		TaskType:      "FIX",
		Status:        "PENDING",
		ScriptContent: &scriptContent,
		AttemptNo:     1,
		MaxRounds:     1, // FIX itself doesn't need self-healing in auto-verify mode
		AutoVerify:    true,
		VerifyRound:   round,
		CreatedAt:     time.Now(),
		StartedAt:     autoVerifyTimePtr(time.Now()),
	}

	if err := s.taskLogRepo.Create(fixTask); err != nil {
		logger.Error("auto-verify: failed to create FIX task",
			zap.String("original_task_id", originalTask.ID.String()),
			zap.Error(err),
		)
		return
	}

	logger.Info("auto-verify: FIX task created",
		zap.String("fix_task_id", fixTask.ID.String()),
		zap.String("original_task_id", originalTask.ID.String()),
		zap.Int("round", round),
	)

	ruleID, _ := uuid.Parse(ruleIDStr)
	hostID, _ := uuid.Parse(hostIDStr)
	go s.taskService.dispatchToAgent(ctx, fixTask.ID, hostID, ruleID, scriptContent, "FIX")
}

// triggerCheckForVerify creates and dispatches a CHECK task for auto-verification.
func (s *AutoVerifyService) triggerCheckForVerify(originalTask *model.TaskLog, round int) {
	ctx := context.Background()

	if originalTask.RuleID == nil {
		logger.Error("auto-verify: cannot trigger CHECK, no rule_id",
			zap.String("task_id", originalTask.ID.String()),
		)
		return
	}

	rule, err := s.ruleRepo.FindByID(*originalTask.RuleID)
	if err != nil {
		logger.Error("auto-verify: failed to find rule for CHECK",
			zap.String("task_id", originalTask.ID.String()),
			zap.String("rule_id", originalTask.RuleID.String()),
			zap.Error(err),
		)
		return
	}

	var scriptContent string
	if rule.GeneratedCheckScript != nil {
		scriptContent = *rule.GeneratedCheckScript
	}
	if scriptContent == "" {
		logger.Error("auto-verify: no CHECK script available",
			zap.String("rule_id", originalTask.RuleID.String()),
		)
		return
	}

	hostIDStr := originalTask.HostID.String()
	ruleIDStr := originalTask.RuleID.String()

	checkTask := &model.TaskLog{
		TaskGroupID:   originalTask.TaskGroupID,
		RuleID:        originalTask.RuleID,
		HostID:        originalTask.HostID,
		TaskType:      "CHECK",
		Status:        "PENDING",
		ScriptContent: &scriptContent,
		AttemptNo:     1,
		MaxRounds:     1,
		AutoVerify:    true,
		VerifyRound:   round,
		CreatedAt:     time.Now(),
		StartedAt:     autoVerifyTimePtr(time.Now()),
	}

	if err := s.taskLogRepo.Create(checkTask); err != nil {
		logger.Error("auto-verify: failed to create CHECK task",
			zap.String("original_task_id", originalTask.ID.String()),
			zap.Error(err),
		)
		return
	}

	logger.Info("auto-verify: CHECK task created",
		zap.String("check_task_id", checkTask.ID.String()),
		zap.String("original_task_id", originalTask.ID.String()),
		zap.Int("round", round),
	)

	ruleID, _ := uuid.Parse(ruleIDStr)
	hostID, _ := uuid.Parse(hostIDStr)
	go s.taskService.dispatchToAgent(ctx, checkTask.ID, hostID, ruleID, scriptContent, "CHECK")
}

func autoVerifyTimePtr(t time.Time) *time.Time {
	return &t
}
