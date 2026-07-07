package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
//
// Triggering: auto-verify is driven by two complementary paths:
//   1. Real-time: TaskService.ProcessTaskResult calls HandleTaskResult right
//      after a terminal result is persisted. This is reached both for results
//      the API Server derives itself (dispatch failure, timeout) and for agent
//      results pushed back by the Server service via the internal
//      POST /internal/task-result endpoint (SetTaskResultCallback in server).
//   2. Poll fallback: StartResultScanner polls FindAutoVerifyTerminalTasks
//      every 5s to catch anything the real-time path missed (e.g. after an
//      API Server restart). The handledResults map plus HasAutoVerifyFollowup
//      DB dedup keep re-processing idempotent.
type AutoVerifyService struct {
	taskLogRepo    *repository.TaskLogRepository
	ruleRepo       *repository.RuleRepository
	taskService    *TaskService
	handledResults sync.Map
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

const autoVerifyScanInterval = 5 * time.Second

func (s *AutoVerifyService) StartResultScanner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(autoVerifyScanInterval)
		defer ticker.Stop()

		logger.Info("auto-verify result scanner started", zap.Duration("interval", autoVerifyScanInterval))
		for {
			select {
			case <-ctx.Done():
				logger.Info("auto-verify result scanner stopped")
				return
			case <-ticker.C:
				s.scanCompletedTaskResults()
			}
		}
	}()
}

func (s *AutoVerifyService) scanCompletedTaskResults() {
	tasks, err := s.taskLogRepo.FindAutoVerifyTerminalTasks(200)
	if err != nil {
		logger.Error("auto-verify: failed to scan completed task results", zap.Error(err))
		return
	}

	for i := range tasks {
		task := &tasks[i]
		key := autoVerifyResultKey(task)
		if _, loaded := s.handledResults.LoadOrStore(key, struct{}{}); loaded {
			continue
		}

		exitCode := 0
		if task.ExitCode != nil {
			exitCode = *task.ExitCode
		}
		if handled := s.HandleTaskResult(task, strings.ToUpper(strings.TrimSpace(task.Status)), exitCode); !handled {
			s.handledResults.Delete(key)
		}
	}
	// Note: `handledResults` is in-process memory. After an API Server restart it
	// is cleared, so the poll may re-evaluate already-processed terminal tasks.
	// That is safe: triggerFixForVerify/triggerCheckForVerify de-dup via
	// HasAutoVerifyFollowup (DB), and a CHECK that already passed simply returns
	// true without creating new tasks. The poll is therefore idempotent and
	// serves as the fallback for both missed real-time pushes and restarts.
}

// HandleTaskResult checks if auto-verification should be triggered after a task completes.
// Called from TaskService.ProcessTaskResult after updating the task result.
func (s *AutoVerifyService) HandleTaskResult(taskLog *model.TaskLog, normalizedStatus string, exitCode int) bool {
	if !taskLog.AutoVerify {
		return false
	}
	if !IsTerminalTaskStatus(normalizedStatus) {
		return false
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
		return s.handleCheckResult(taskLog, normalizedStatus, exitCode, currentRound, maxRounds)
	case "FIX":
		return s.handleFixResult(taskLog, normalizedStatus, exitCode, currentRound, maxRounds)
	}
	return false
}

// handleCheckResult handles CHECK task completion in auto-verify mode.
// If CHECK passed (exit_code=0), verification is complete.
// If CHECK failed (exit_code=1, non-compliant), trigger FIX and continue.
//
// Note on status normalization: for CHECK tasks, NormalizeTaskResultStatus maps
// a non-compliant result (exit_code=1) to SUCCESS and only maps genuine
// execution errors (exit_code<0 or >=2, or stderr matching an error pattern)
// to FAILED. So the `status != "SUCCESS"` branch below is the defensive path
// for CHECK execution failures (it will not be hit for a merely non-compliant
// baseline item). Non-compliance is detected via `exitCode == 1` further down.
func (s *AutoVerifyService) handleCheckResult(taskLog *model.TaskLog, status string, exitCode, currentRound, maxRounds int) bool {
	if status != "SUCCESS" {
		// Defensive branch: CHECK task itself failed (execution error, timeout,
		// audit-blocked, etc.) - stop auto-verify. A merely non-compliant
		// baseline item is normalized to SUCCESS with exit_code=1 and handled below.
		logger.Info("auto-verify stopped: CHECK task execution failed",
			zap.String("task_id", taskLog.ID.String()),
			zap.String("status", status),
		)
		return true
	}

	if exitCode == 0 {
		// CHECK passed - verification complete!
		logger.Info("auto-verify completed: CHECK passed",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("total_rounds", currentRound),
		)
		return true
	}

	// CHECK failed (exit_code=1, non-compliant) - need to FIX
	if currentRound >= maxRounds {
		logger.Warn("auto-verify stopped: max rounds reached after CHECK",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("verify_round", currentRound),
			zap.Int("max_rounds", maxRounds),
		)
		return true
	}

	logger.Info("auto-verify: CHECK failed, triggering FIX",
		zap.String("task_id", taskLog.ID.String()),
		zap.Int("next_round", currentRound+1),
	)

	return s.triggerFixForVerify(taskLog, currentRound+1, maxRounds)
}

// handleFixResult handles FIX task completion in auto-verify mode.
// If FIX succeeded (exit_code=0), trigger CHECK to verify.
// If FIX failed, trigger another FIX attempt (up to max rounds).
func (s *AutoVerifyService) handleFixResult(taskLog *model.TaskLog, status string, exitCode, currentRound, maxRounds int) bool {
	if status == "SUCCESS" && exitCode == 0 {
		// FIX succeeded - trigger CHECK to verify the fix
		logger.Info("auto-verify: FIX succeeded, triggering CHECK verification",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("verify_round", currentRound),
		)
		return s.triggerCheckForVerify(taskLog, currentRound, maxRounds)
	}

	if currentRound >= maxRounds {
		logger.Warn("auto-verify stopped: max rounds reached after FIX failure",
			zap.String("task_id", taskLog.ID.String()),
			zap.Int("verify_round", currentRound),
			zap.Int("max_rounds", maxRounds),
		)
		return true
	}

	logger.Info("auto-verify: FIX failed, triggering large-model script repair",
		zap.String("task_id", taskLog.ID.String()),
		zap.Int("verify_round", currentRound),
	)
	if s.taskService == nil {
		logger.Warn("auto-verify: task service unavailable, cannot trigger script repair",
			zap.String("task_id", taskLog.ID.String()))
		return true
	}
	s.taskService.maybeTriggerLargeModelRepair(taskLog, status, exitCode, stringValue(taskLog.Stdout), stringValue(taskLog.Stderr))
	return true
}

// triggerFixForVerify creates and dispatches a FIX task for auto-verification.
func (s *AutoVerifyService) triggerFixForVerify(originalTask *model.TaskLog, round, maxRounds int) bool {
	ctx := context.Background()

	if originalTask.RuleID == nil {
		logger.Error("auto-verify: cannot trigger FIX, no rule_id",
			zap.String("task_id", originalTask.ID.String()),
		)
		return false
	}

	exists, err := s.taskLogRepo.HasAutoVerifyFollowup(originalTask.TaskGroupID, originalTask.RuleID, originalTask.HostID, "FIX", round)
	if err != nil {
		return false
	}
	if exists {
		logger.Debug("auto-verify: FIX followup already exists",
			zap.String("original_task_id", originalTask.ID.String()),
			zap.Int("round", round),
		)
		return true
	}

	rule, err := s.ruleRepo.FindByID(*originalTask.RuleID)
	if err != nil {
		logger.Error("auto-verify: failed to find rule for FIX",
			zap.String("task_id", originalTask.ID.String()),
			zap.String("rule_id", originalTask.RuleID.String()),
			zap.Error(err),
		)
		return false
	}

	var scriptContent string
	if rule.GeneratedFixScript != nil {
		scriptContent = *rule.GeneratedFixScript
	}
	if scriptContent == "" {
		logger.Error("auto-verify: no FIX script available",
			zap.String("rule_id", originalTask.RuleID.String()),
		)
		return false
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
		MaxRounds:     maxRounds,
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
		return false
	}

	logger.Info("auto-verify: FIX task created",
		zap.String("fix_task_id", fixTask.ID.String()),
		zap.String("original_task_id", originalTask.ID.String()),
		zap.Int("round", round),
		zap.Int("max_rounds", maxRounds),
	)

	ruleID, _ := uuid.Parse(ruleIDStr)
	hostID, _ := uuid.Parse(hostIDStr)
	if s.taskService != nil {
		go s.taskService.dispatchToAgent(ctx, fixTask.ID, hostID, ruleID, scriptContent, "FIX")
	}
	return true
}

// triggerCheckForVerify creates and dispatches a CHECK task for auto-verification.
func (s *AutoVerifyService) triggerCheckForVerify(originalTask *model.TaskLog, round, maxRounds int) bool {
	ctx := context.Background()

	if originalTask.RuleID == nil {
		logger.Error("auto-verify: cannot trigger CHECK, no rule_id",
			zap.String("task_id", originalTask.ID.String()),
		)
		return false
	}

	exists, err := s.taskLogRepo.HasAutoVerifyFollowup(originalTask.TaskGroupID, originalTask.RuleID, originalTask.HostID, "CHECK", round)
	if err != nil {
		return false
	}
	if exists {
		logger.Debug("auto-verify: CHECK followup already exists",
			zap.String("original_task_id", originalTask.ID.String()),
			zap.Int("round", round),
		)
		return true
	}

	rule, err := s.ruleRepo.FindByID(*originalTask.RuleID)
	if err != nil {
		logger.Error("auto-verify: failed to find rule for CHECK",
			zap.String("task_id", originalTask.ID.String()),
			zap.String("rule_id", originalTask.RuleID.String()),
			zap.Error(err),
		)
		return false
	}

	var scriptContent string
	if rule.GeneratedCheckScript != nil {
		scriptContent = *rule.GeneratedCheckScript
	}
	if scriptContent == "" {
		logger.Error("auto-verify: no CHECK script available",
			zap.String("rule_id", originalTask.RuleID.String()),
		)
		return false
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
		MaxRounds:     maxRounds,
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
		return false
	}

	logger.Info("auto-verify: CHECK task created",
		zap.String("check_task_id", checkTask.ID.String()),
		zap.String("original_task_id", originalTask.ID.String()),
		zap.Int("round", round),
		zap.Int("max_rounds", maxRounds),
	)

	ruleID, _ := uuid.Parse(ruleIDStr)
	hostID, _ := uuid.Parse(hostIDStr)
	if s.taskService != nil {
		go s.taskService.dispatchToAgent(ctx, checkTask.ID, hostID, ruleID, scriptContent, "CHECK")
	}
	return true
}

func autoVerifyTimePtr(t time.Time) *time.Time {
	return &t
}

func autoVerifyResultKey(task *model.TaskLog) string {
	exitCode := "nil"
	if task.ExitCode != nil {
		exitCode = fmt.Sprintf("%d", *task.ExitCode)
	}
	finishedAt := "nil"
	if task.FinishedAt != nil {
		finishedAt = fmt.Sprintf("%d", task.FinishedAt.UnixNano())
	}
	return strings.Join([]string{
		task.ID.String(),
		strings.ToUpper(strings.TrimSpace(task.Status)),
		exitCode,
		fmt.Sprintf("%d", task.AttemptNo),
		finishedAt,
	}, ":")
}
