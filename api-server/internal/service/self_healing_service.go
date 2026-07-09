package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/storage"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type SelfHealingService struct {
	healingLogRepo     *repository.HealingLogRepository
	scriptVersionRepo  *repository.ScriptVersionRepository
	configRepo         *repository.ConfigRepository
	ruleRepo           *repository.RuleRepository
	vulnRepo           *repository.VulnerabilityRepo
	vulnScriptRepo     *repository.VulnerabilityScriptRepository
	taskLogRepo        *repository.TaskLogRepository
	minioClient        *storage.MinIOClient
	redisClient        *storage.RedisClient
	scriptAuditService *ScriptAuditService
	taskRedispatcher   HealingTaskRedispatcher
	healingQueue       chan HealingTask
	llmTimeout         int
	llmMaxRetries      int
	maxRetries         int
}

type HealingTaskRedispatcher interface {
	DispatchHealedTask(ctx context.Context, taskID uuid.UUID, scriptContent string, scriptVersion int, healingID uuid.UUID, attemptNo int) error
}

type HealingTask struct {
	OriginalTaskID  uuid.UUID
	RuleID          *uuid.UUID
	VulnerabilityID *uuid.UUID
	HostID          uuid.UUID
	ScriptType      string
	ScriptContent   string
	ErrorMessage    string
	ExitCode        int
	UserSuggestion  string
	AttemptNo       int
	MaxRounds       int
}

func NewSelfHealingService(
	healingLogRepo *repository.HealingLogRepository,
	scriptVersionRepo *repository.ScriptVersionRepository,
	configRepo *repository.ConfigRepository,
	ruleRepo *repository.RuleRepository,
	taskLogRepo *repository.TaskLogRepository,
	minioClient *storage.MinIOClient,
	redisClient *storage.RedisClient,
	llmTimeout int,
	llmMaxRetries int,
	maxRetries int,
	scriptAuditService *ScriptAuditService,
) *SelfHealingService {
	return &SelfHealingService{
		healingLogRepo:     healingLogRepo,
		scriptVersionRepo:  scriptVersionRepo,
		configRepo:         configRepo,
		ruleRepo:           ruleRepo,
		taskLogRepo:        taskLogRepo,
		minioClient:        minioClient,
		redisClient:        redisClient,
		scriptAuditService: scriptAuditService,
		healingQueue:       make(chan HealingTask, 100),
		llmTimeout:         llmTimeout,
		llmMaxRetries:      llmMaxRetries,
		maxRetries:         maxRetries,
	}
}

func (s *SelfHealingService) SetTaskRedispatcher(redispatcher HealingTaskRedispatcher) {
	s.taskRedispatcher = redispatcher
}

func (s *SelfHealingService) SetVulnerabilityScriptRepositories(vulnRepo *repository.VulnerabilityRepo, vulnScriptRepo *repository.VulnerabilityScriptRepository) {
	s.vulnRepo = vulnRepo
	s.vulnScriptRepo = vulnScriptRepo
}

func (s *SelfHealingService) TriggerHealing(task HealingTask) error {
	ruleIDStr := ""
	if task.RuleID != nil {
		ruleIDStr = task.RuleID.String()
	}
	currentAttempt, maxAttempts := s.healingAttemptBounds(task)
	logger.Info("triggering self-healing",
		zap.String("original_task_id", task.OriginalTaskID.String()),
		zap.String("rule_id", ruleIDStr),
		zap.String("script_type", task.ScriptType),
		zap.Int("exit_code", task.ExitCode),
		zap.Int("attempt_no", currentAttempt),
		zap.Int("max_attempts", maxAttempts),
	)

	if currentAttempt > maxAttempts {
		return fmt.Errorf("large-model repair attempts exhausted: %d/%d", currentAttempt, maxAttempts)
	}

	// Store initial status in Redis
	if s.redisClient != nil {
		status := &storage.HealingStatus{
			TaskID:         task.OriginalTaskID.String(),
			Status:         "healing",
			StartedAt:      time.Now(),
			TotalAttempts:  currentAttempt - 1,
			MaxAttempts:    maxAttempts,
			UserSuggestion: task.UserSuggestion,
			ScriptType:     task.ScriptType,
		}
		if err := s.redisClient.SetHealingStatusStruct(status); err != nil {
			logger.Error("failed to store healing status in Redis",
				zap.Error(err),
				zap.String("task_id", task.OriginalTaskID.String()),
			)
		}
	}

	select {
	case s.healingQueue <- task:
		logger.Info("healing task queued",
			zap.String("original_task_id", task.OriginalTaskID.String()),
		)
		return nil
	default:
		logger.Error("healing queue is full",
			zap.String("original_task_id", task.OriginalTaskID.String()),
		)
		return fmt.Errorf("healing queue is full")
	}
}

// StartWorkers 启动自愈修复 Worker
func (s *SelfHealingService) StartWorkers(ctx context.Context) {
	logger.Info("starting self-healing workers",
		zap.Int("max_retries", s.maxRetries),
	)

	for i := 0; i < 3; i++ {
		go s.healingWorker(ctx, i)
	}

	// Start timeout checker
	go s.timeoutChecker(ctx)
}

func (s *SelfHealingService) updateHealingStatusInRedis(taskID string, status string, attempt int, lastError string) {
	if s.redisClient == nil {
		return
	}

	// Get existing status first to preserve started_at
	existing, err := s.redisClient.GetHealingStatusStruct(taskID)
	if err != nil {
		logger.Error("failed to get existing healing status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return
	}

	if existing == nil {
		existing = &storage.HealingStatus{
			TaskID:    taskID,
			StartedAt: time.Now(),
		}
	}

	existing.Status = status
	existing.TotalAttempts = attempt
	existing.LastError = lastError

	if err := s.redisClient.SetHealingStatusStruct(existing); err != nil {
		logger.Error("failed to update healing status in Redis",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
	}
}

func (s *SelfHealingService) GetHealingStatus(taskID string) *storage.HealingStatus {
	if s.redisClient != nil {
		status, err := s.redisClient.GetHealingStatusStruct(taskID)
		if err != nil {
			logger.Error("failed to get healing status from Redis",
				zap.Error(err),
				zap.String("task_id", taskID),
			)
		} else if status != nil {
			return status
		}
	}

	// Redis status is intentionally short-lived. Fall back to the persistent
	// healing log so task details still show the automatic repair process after
	// refresh, service restart, or TTL expiry.
	parsedTaskID, err := uuid.Parse(taskID)
	if err != nil || s.healingLogRepo == nil {
		return nil
	}
	healingLog, err := s.healingLogRepo.GetLatestByOriginalTaskID(parsedTaskID)
	if err != nil {
		logger.Warn("failed to restore healing status from database",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return nil
	}
	if healingLog == nil {
		return nil
	}
	status := &storage.HealingStatus{
		TaskID:         taskID,
		Status:         healingLog.Status,
		StartedAt:      healingLog.StartedAt,
		TotalAttempts:  healingLog.TotalAttempts,
		MaxAttempts:    healingLog.MaxAttempts,
		LastError:      healingLog.LastError,
		UserSuggestion: healingLog.UserSuggestion,
		ScriptType:     healingLog.ScriptType,
	}
	if healingLog.FinishedAt != nil {
		status.UpdatedAt = *healingLog.FinishedAt
	}
	return status
}

const HealingTimeout = 5 * time.Minute
const baselineHealingMinLLMTimeoutSeconds = 180
const redispatchPollInterval = 2 * time.Second

func (s *SelfHealingService) timeoutChecker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("healing timeout checker stopped")
			return
		case <-ticker.C:
			if s.redisClient == nil {
				continue
			}
			// Scan all healing status keys
			iter := s.redisClient.Client().Scan(s.redisClient.Context(), 0, storage.HealingStatusKeyPrefix+"*", 0).Iterator()
			for iter.Next(s.redisClient.Context()) {
				key := iter.Val()
				data, err := s.redisClient.Client().Get(s.redisClient.Context(), key).Bytes()
				if err != nil {
					continue
				}

				var status storage.HealingStatus
				if err := jsonUnmarshal(data, &status); err != nil {
					continue
				}

				// Check if timed out (status is "healing" and started more than 5 minutes ago)
				if status.Status == "healing" && time.Since(status.StartedAt) > HealingTimeout {
					logger.Warn("healing task timed out",
						zap.String("task_id", status.TaskID),
						zap.Duration("elapsed", time.Since(status.StartedAt)),
					)
					status.Status = "timeout"
					status.LastError = "修复超时（超过 5 分钟未返回结果）"
					s.redisClient.SetHealingStatusStruct(&status)
				}
			}
		}
	}
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (s *SelfHealingService) healingWorker(ctx context.Context, workerID int) {
	logger.Info("self-healing worker started",
		zap.Int("worker_id", workerID),
	)

	for {
		select {
		case <-ctx.Done():
			logger.Info("self-healing worker stopped",
				zap.Int("worker_id", workerID),
			)
			return
		case task := <-s.healingQueue:
			s.processHealing(ctx, workerID, task)
		}
	}
}

func (s *SelfHealingService) processHealing(ctx context.Context, workerID int, task HealingTask) {
	ruleIDStr := ""
	if task.RuleID != nil {
		ruleIDStr = task.RuleID.String()
	}
	vulnIDStr := ""
	if task.VulnerabilityID != nil {
		vulnIDStr = task.VulnerabilityID.String()
	}
	logger.Info("processing self-healing",
		zap.Int("worker_id", workerID),
		zap.String("original_task_id", task.OriginalTaskID.String()),
		zap.String("rule_id", ruleIDStr),
		zap.String("vulnerability_id", vulnIDStr),
		zap.String("user_suggestion", task.UserSuggestion),
	)

	currentAttempt, maxAttempts := s.healingAttemptBounds(task)
	healingLog := &model.HealingLog{
		OriginalTaskID:  task.OriginalTaskID,
		RuleID:          task.RuleID,
		VulnerabilityID: task.VulnerabilityID,
		HostID:          task.HostID,
		ScriptType:      task.ScriptType,
		TriggerError:    task.ErrorMessage,
		TriggerExitCode: task.ExitCode,
		TotalAttempts:   currentAttempt - 1,
		MaxAttempts:     maxAttempts,
		Status:          "healing",
		AttemptsDetail:  make(model.AttemptsDetail, 0),
		UserSuggestion:  task.UserSuggestion,
		StartedAt:       time.Now(),
	}

	if err := s.healingLogRepo.Create(healingLog); err != nil {
		logger.Error("failed to create healing log",
			zap.Error(err),
			zap.String("original_task_id", task.OriginalTaskID.String()),
		)
		s.updateHealingStatusInRedis(task.OriginalTaskID.String(), "failed", currentAttempt-1, err.Error())
		return
	}

	config, err := s.configRepo.GetActive()
	if err != nil {
		logger.Error("failed to get LLM config",
			zap.Error(err),
			zap.String("original_task_id", task.OriginalTaskID.String()),
		)
		s.failHealingLog(healingLog, "LLM配置未设置："+err.Error())
		return
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		logger.Error("failed to decrypt API key",
			zap.Error(err),
			zap.String("original_task_id", task.OriginalTaskID.String()),
		)
		s.failHealingLog(healingLog, "API密钥解密失败："+err.Error())
		return
	}

	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.effectiveLLMTimeoutSeconds(), s.llmMaxRetries)

	var lastError string
	var fixedScript string
	var scriptVersionID uuid.UUID

	for attempt := currentAttempt; attempt <= maxAttempts; attempt++ {
		logger.Info("healing attempt",
			zap.Stringer("healing_id", healingLog.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", maxAttempts),
		)

		healingLog.TotalAttempts = attempt
		if err := s.healingLogRepo.IncrementAttempts(healingLog.ID); err != nil {
			logger.Error("failed to increment healing attempts",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
			)
		}

		s.updateHealingStatusInRedis(task.OriginalTaskID.String(), "healing", attempt, "")

		// 构建自愈 Prompt
		history := s.buildHealingHistory(healingLog.AttemptsDetail)
		prompt := llm.GetSelfHealingFixPrompt(task.ScriptContent, task.ErrorMessage, task.ExitCode, history)

		if task.UserSuggestion != "" {
			prompt = fmt.Sprintf("%s\n\n用户提供的修复建议：%s", prompt, task.UserSuggestion)
		}
		if ruleContext := s.buildRuleContext(task.RuleID); ruleContext != "" {
			prompt = fmt.Sprintf("%s\n\n基线规则上下文：\n%s", prompt, ruleContext)
		}

		systemPrompt := "你是一位资深的 Shell 脚本调试专家"
		healingPrompt := prompt
		healingLLMClient := llmClient
		healingGenerator := &scriptGeneratorFunc{fn: func(genCtx context.Context, _ *AuditRequest) (string, error) {
			return healingLLMClient.ChatCompletion(genCtx, systemPrompt, healingPrompt, 0.1)
		}}

		ruleIDStr := ""
		if task.RuleID != nil {
			ruleIDStr = task.RuleID.String()
		}

		auditReq := &AuditRequest{
			ScriptContent: prompt,
			ScriptType:    ScriptTypeSelfHealing,
			TaskID:        task.OriginalTaskID.String(),
			RuleID:        ruleIDStr,
			Source:        AuditSourceGeneration,
		}

		auditResult, err := s.scriptAuditService.AuditWithRetry(ctx, healingGenerator, auditReq)
		if err != nil {
			logger.Error("healed script audit failed",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
				zap.Int("attempt", attempt),
			)
			lastError = fmt.Sprintf("脚本审计失败：%v", err)
			continue
		}
		if !auditResult.Passed {
			logger.Error("healed script audit not passed",
				zap.String("error", auditResult.ErrorMsg),
				zap.Stringer("healing_id", healingLog.ID),
				zap.Int("attempt", attempt),
			)
			lastError = fmt.Sprintf("脚本审计未通过：%s", auditResult.ErrorMsg)
			continue
		}

		fixedScript = auditResult.Script

		// 创建脚本版本记录
		version := attempt // 自愈版本从 1 开始
		scriptVersion := &model.ScriptVersion{
			RuleID:           task.RuleID,
			VulnerabilityID:  task.VulnerabilityID,
			ScriptType:       task.ScriptType,
			Version:          version,
			ScriptContent:    fixedScript,
			GenerationSource: "self_healing",
			IsCurrent:        false,
		}

		if err := s.scriptVersionRepo.Create(scriptVersion); err != nil {
			logger.Error("failed to create healed script version",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
				zap.Int("attempt", attempt),
			)
			lastError = fmt.Sprintf("创建版本记录失败：%v", err)
			continue
		}

		scriptVersionID = scriptVersion.ID

		// 上传修复后的脚本到 MinIO
		identifier := "unknown"
		if task.RuleID != nil {
			identifier = task.RuleID.String()
		} else if task.VulnerabilityID != nil {
			identifier = task.VulnerabilityID.String()
		}
		minIOPath := fmt.Sprintf("healing/%s/%d/%s.sh", identifier, attempt, task.ScriptType)
		_, err = s.minioClient.UploadFile("generated-scripts", minIOPath,
			strings.NewReader(fixedScript), int64(len(fixedScript)), "application/x-sh")
		if err != nil {
			logger.Error("failed to upload healed script to MinIO",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
			)
			lastError = fmt.Sprintf("上传脚本失败：%v", err)
			continue
		}

		// 更新规则的脚本（仅对基线任务）
		if task.RuleID != nil {
			if err := s.updateRuleScript(*task.RuleID, task.ScriptType, fixedScript, version); err != nil {
				logger.Error("failed to update rule script",
					zap.Error(err),
					zap.Stringer("rule_id", task.RuleID),
				)
			} else {
				logger.Info("rule script updated after healing",
					zap.Stringer("rule_id", task.RuleID),
					zap.String("script_type", task.ScriptType),
				)
			}
		}
		// Keep the CVE-level generic script aligned with the healed task, just as
		// baseline healing updates the rule script. The repository compares the
		// failed content first so a newer manual regeneration is never overwritten.
		if task.VulnerabilityID != nil {
			s.persistHealedVulnerabilityScript(task, fixedScript)
		}

		resultExitCode := 0
		resultStderr := ""
		if s.taskRedispatcher != nil {
			if err := s.taskRedispatcher.DispatchHealedTask(ctx, task.OriginalTaskID, fixedScript, version, healingLog.ID, attempt); err != nil {
				logger.Error("failed to redispatch healed task",
					zap.Error(err),
					zap.Stringer("healing_id", healingLog.ID),
					zap.Int("attempt", attempt),
				)
				lastError = fmt.Sprintf("重新下发失败：%v", err)
				s.recordHealingAttempt(healingLog, attempt, scriptVersionID, task.ErrorMessage, resultExitCode, lastError)
				continue
			}

			resultTask, err := s.waitForRedispatchedTask(ctx, task, attempt)
			if err != nil {
				logger.Error("waiting for redispatched task failed",
					zap.Error(err),
					zap.Stringer("healing_id", healingLog.ID),
					zap.Int("attempt", attempt),
				)
				lastError = fmt.Sprintf("等待重新下发结果失败：%v", err)
				s.recordHealingAttempt(healingLog, attempt, scriptVersionID, task.ErrorMessage, resultExitCode, lastError)
				continue
			}
			if resultTask.ExitCode != nil {
				resultExitCode = *resultTask.ExitCode
			}
			if resultTask.Stderr != nil {
				resultStderr = *resultTask.Stderr
			}
			if IsTaskExecutionSuccessful(resultTask.TaskType, resultTask.Status, resultTask.ExitCode, resultStderr) {
				s.recordHealingAttempt(healingLog, attempt, scriptVersionID, task.ErrorMessage, resultExitCode, resultStderr)
				if err := s.healingLogRepo.MarkCompleted(healingLog.ID, scriptVersionID); err != nil {
					logger.Error("failed to mark healing completed",
						zap.Error(err),
						zap.Stringer("healing_id", healingLog.ID),
					)
				}
				logger.Info("self-healing completed successfully after redispatch",
					zap.Stringer("healing_id", healingLog.ID),
					zap.Int("total_attempts", attempt),
					zap.String("script_version_id", scriptVersionID.String()),
				)
				s.updateHealingStatusInRedis(task.OriginalTaskID.String(), "healed", attempt, "")
				return
			}

			lastError = taskResultErrorMessage(resultTask)
			s.recordHealingAttempt(healingLog, attempt, scriptVersionID, task.ErrorMessage, resultExitCode, lastError)
			task.ScriptContent = fixedScript
			task.ErrorMessage = lastError
			task.ExitCode = resultExitCode
			continue
		}

		s.recordHealingAttempt(healingLog, attempt, scriptVersionID, task.ErrorMessage, resultExitCode, resultStderr)

		// 兼容未接入重新下发器的调用方：只完成脚本生成修复。
		if err := s.healingLogRepo.MarkCompleted(healingLog.ID, scriptVersionID); err != nil {
			logger.Error("failed to mark healing completed",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
			)
		}
		logger.Info("self-healing completed successfully",
			zap.Stringer("healing_id", healingLog.ID),
			zap.Int("total_attempts", attempt),
			zap.String("script_version_id", scriptVersionID.String()),
		)
		s.updateHealingStatusInRedis(task.OriginalTaskID.String(), "healed", attempt, "")
		return
	}

	// 所有尝试都失败
	healingLog.Status = "failed"
	healingLog.FinishedAt = pointerToTime(time.Now())
	healingLog.LastError = lastError

	if err := s.healingLogRepo.Update(healingLog); err != nil {
		logger.Error("failed to update failed healing log",
			zap.Error(err),
			zap.Stringer("healing_id", healingLog.ID),
		)
	}

	if err := s.healingLogRepo.MarkFailed(healingLog.ID); err != nil {
		logger.Error("failed to mark healing as failed",
			zap.Error(err),
			zap.Stringer("healing_id", healingLog.ID),
		)
	}

	s.healingLogRepo.UpdateLastError(healingLog.ID, lastError)

	logger.Error("self-healing failed after all attempts",
		zap.Stringer("healing_id", healingLog.ID),
		zap.Int("total_attempts", healingLog.TotalAttempts),
		zap.String("last_error", lastError),
	)

	s.updateHealingStatusInRedis(task.OriginalTaskID.String(), "failed", healingLog.TotalAttempts, lastError)
}

func (s *SelfHealingService) persistHealedVulnerabilityScript(task HealingTask, fixedScript string) {
	if task.VulnerabilityID == nil || s.vulnRepo == nil || s.vulnScriptRepo == nil {
		return
	}
	vuln, err := s.vulnRepo.FindByID(*task.VulnerabilityID)
	if err != nil {
		logger.Warn("failed to find vulnerability while persisting healed script",
			zap.Error(err),
			zap.String("vulnerability_id", task.VulnerabilityID.String()),
		)
		return
	}
	scriptType := model.ScriptTypeFix
	if strings.EqualFold(task.ScriptType, "POC") || strings.EqualFold(task.ScriptType, "POC_VERIFY") {
		scriptType = model.ScriptTypePoc
	}
	if err := s.vulnScriptRepo.UpdateHealedContent(vuln.CveID, scriptType, task.ScriptContent, fixedScript); err != nil {
		if errors.Is(err, repository.ErrStaleVulnerabilityScriptGeneration) {
			logger.Warn("healed generic vulnerability script not persisted because source changed",
				zap.String("vulnerability_id", task.VulnerabilityID.String()),
				zap.String("cve_id", vuln.CveID),
				zap.String("script_type", scriptType),
			)
			return
		}
		logger.Warn("failed to persist healed generic vulnerability script",
			zap.Error(err),
			zap.String("vulnerability_id", task.VulnerabilityID.String()),
			zap.String("cve_id", vuln.CveID),
			zap.String("script_type", scriptType),
		)
	}
}

func (s *SelfHealingService) recordHealingAttempt(healingLog *model.HealingLog, attempt int, scriptVersionID uuid.UUID, errorInput string, resultExitCode int, resultStderr string) {
	healingLog.AttemptsDetail = append(healingLog.AttemptsDetail, model.AttemptDetail{
		Attempt:         attempt,
		ScriptVersionID: scriptVersionID.String(),
		ErrorInput:      errorInput,
		LLMFixSummary:   "Script regenerated by LLM",
		ResultExitCode:  resultExitCode,
		ResultStderr:    resultStderr,
		Timestamp:       time.Now(),
	})
	healingLog.TotalAttempts = attempt

	if err := s.healingLogRepo.Update(healingLog); err != nil {
		logger.Error("failed to update healing log",
			zap.Error(err),
			zap.Stringer("healing_id", healingLog.ID),
		)
	}
}

func (s *SelfHealingService) waitForRedispatchedTask(ctx context.Context, task HealingTask, attempt int) (*model.TaskLog, error) {
	waitCtx, cancel := context.WithTimeout(ctx, HealingTimeout)
	defer cancel()

	ticker := time.NewTicker(redispatchPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return nil, waitCtx.Err()
		case <-ticker.C:
			current, err := s.taskLogRepo.FindByID(task.OriginalTaskID)
			if err != nil {
				return nil, err
			}
			if current.AttemptNo != attempt {
				continue
			}
			if IsTerminalTaskStatus(current.Status) {
				return current, nil
			}
		}
	}
}

func taskResultErrorMessage(task *model.TaskLog) string {
	if task == nil {
		return "任务执行失败"
	}
	if task.Stderr != nil && strings.TrimSpace(*task.Stderr) != "" {
		return strings.TrimSpace(*task.Stderr)
	}
	if task.Stdout != nil && strings.TrimSpace(*task.Stdout) != "" {
		return strings.TrimSpace(*task.Stdout)
	}
	exitCode := "unknown"
	if task.ExitCode != nil {
		exitCode = fmt.Sprintf("%d", *task.ExitCode)
	}
	return fmt.Sprintf("任务状态 %s，退出码 %s", task.Status, exitCode)
}

func (s *SelfHealingService) buildRuleContext(ruleID *uuid.UUID) string {
	if ruleID == nil || s.ruleRepo == nil {
		return ""
	}
	rule, err := s.ruleRepo.FindByID(*ruleID)
	if err != nil || rule == nil {
		return ""
	}
	return fmt.Sprintf("规则标题：%s\n检测内容：%s\n修复建议：%s", rule.Title, rule.CheckContent, rule.FixContent)
}

func (s *SelfHealingService) buildHealingHistory(attempts model.AttemptsDetail) string {
	if len(attempts) == 0 {
		return "无"
	}

	var history strings.Builder
	history.WriteString(fmt.Sprintf("历史修复尝试：%d 次\n\n", len(attempts)))

	for i, attempt := range attempts {
		history.WriteString(fmt.Sprintf("尝试 %d:\n", i+1))
		history.WriteString(fmt.Sprintf("  错误：%s\n", attempt.ErrorInput))
		history.WriteString(fmt.Sprintf("  修复：%s\n", attempt.LLMFixSummary))
		history.WriteString(fmt.Sprintf("  结果：退出码 %d\n", attempt.ResultExitCode))
		if attempt.ResultStderr != "" {
			history.WriteString(fmt.Sprintf("  错误输出：%s\n", attempt.ResultStderr))
		}
		history.WriteString("\n")
	}

	return history.String()
}

func (s *SelfHealingService) updateRuleScript(ruleID uuid.UUID, scriptType, scriptContent string, version int) error {
	return s.ruleRepo.UpdateScript(ruleID, scriptType, scriptContent, version)
}

func (s *SelfHealingService) GetHealingLogByTaskID(taskID uuid.UUID) (*model.HealingLog, error) {
	return s.healingLogRepo.GetLatestByOriginalTaskID(taskID)
}

// ShouldTriggerHealing 判断是否应该触发自愈
func (s *SelfHealingService) ShouldTriggerHealing(scriptType string, exitCode int) bool {
	switch strings.ToUpper(strings.TrimSpace(scriptType)) {
	case "CHECK":
		return isCheckExecutionError(exitCode, "")
	case "FIX", "VULNERABILITY_FIX", "POC_VERIFY":
		return exitCode != 0
	default:
		return false
	}
}

func (s *SelfHealingService) healingAttemptBounds(task HealingTask) (int, int) {
	maxAttempts := task.MaxRounds
	if maxAttempts <= 0 {
		maxAttempts = s.maxRetries
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	currentAttempt := task.AttemptNo
	if currentAttempt <= 0 {
		currentAttempt = 1
	}
	return currentAttempt, maxAttempts
}

func (s *SelfHealingService) effectiveLLMTimeoutSeconds() int {
	timeout := s.llmTimeout
	if timeout < baselineHealingMinLLMTimeoutSeconds {
		return baselineHealingMinLLMTimeoutSeconds
	}
	return timeout
}

func (s *SelfHealingService) failHealingLog(healingLog *model.HealingLog, lastError string) {
	if healingLog == nil {
		return
	}
	healingLog.Status = "failed"
	healingLog.FinishedAt = pointerToTime(time.Now())
	healingLog.LastError = lastError
	if err := s.healingLogRepo.Update(healingLog); err != nil {
		logger.Error("failed to update failed healing log",
			zap.Error(err),
			zap.Stringer("healing_id", healingLog.ID),
		)
	}
	if err := s.healingLogRepo.MarkFailed(healingLog.ID); err != nil {
		logger.Error("failed to mark healing as failed",
			zap.Error(err),
			zap.Stringer("healing_id", healingLog.ID),
		)
	}
	_ = s.healingLogRepo.UpdateLastError(healingLog.ID, lastError)
	s.updateHealingStatusInRedis(healingLog.OriginalTaskID.String(), "failed", healingLog.TotalAttempts, lastError)
}

// Helper functions
func pointerToTime(t time.Time) *time.Time {
	return &t
}
