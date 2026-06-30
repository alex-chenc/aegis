package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/storage"
	"api-server/pkg/logger"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const baselineHealingWorkerCount = 3
const baselineHealingQueueSize = 100

type SelfHealingService struct {
	healingLogRepo     *repository.HealingLogRepository
	scriptVersionRepo  *repository.ScriptVersionRepository
	configRepo         *repository.ConfigRepository
	ruleRepo           *repository.RuleRepository
	taskLogRepo        *repository.TaskLogRepository
	minioClient        *storage.MinIOClient
	redisClient        *storage.RedisClient
	scriptAuditService *ScriptAuditService
	healingQueue       chan HealingTask
	maxRetries         int
	onHealedScript     func(context.Context, HealingTask, uuid.UUID, string, int) error
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
	TaskGroupID     uuid.UUID
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
		healingQueue:       make(chan HealingTask, baselineHealingQueueSize),
		maxRetries:         maxRetries,
	}
}

func (s *SelfHealingService) SetHealedScriptHandler(handler func(context.Context, HealingTask, uuid.UUID, string, int) error) {
	s.onHealedScript = handler
}

func (s *SelfHealingService) TriggerHealing(task HealingTask) error {
	ruleIDStr := ""
	if task.RuleID != nil {
		ruleIDStr = task.RuleID.String()
	}
	logger.Info("triggering self-healing",
		zap.String("original_task_id", task.OriginalTaskID.String()),
		zap.String("rule_id", ruleIDStr),
		zap.String("script_type", task.ScriptType),
		zap.Int("exit_code", task.ExitCode),
	)

	queuePosition := len(s.healingQueue) + 1

	// Store initial status in Redis
	if s.redisClient != nil {
		status := &storage.HealingStatus{
			TaskID:           task.OriginalTaskID.String(),
			Status:           "queued",
			StartedAt:        time.Now(),
			TotalAttempts:    0,
			MaxAttempts:      s.maxRetries,
			UserSuggestion:   task.UserSuggestion,
			ScriptType:       task.ScriptType,
			QueuePosition:    queuePosition,
			ConcurrencyLimit: baselineHealingWorkerCount,
			Steps: []storage.HealingStep{{
				Phase:     "queue",
				Status:    "queued",
				Summary:   fmt.Sprintf("已进入 ReAct 修复队列，当前位置 %d，并发上限 %d", queuePosition, baselineHealingWorkerCount),
				Timestamp: time.Now(),
			}},
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
		zap.Int("worker_count", baselineHealingWorkerCount),
	)

	for i := 0; i < baselineHealingWorkerCount; i++ {
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
	existing.QueuePosition = 0
	existing.ConcurrencyLimit = baselineHealingWorkerCount

	if err := s.redisClient.SetHealingStatusStruct(existing); err != nil {
		logger.Error("failed to update healing status in Redis",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
	}
}

func (s *SelfHealingService) appendHealingStep(taskID string, phase string, status string, summary string) {
	if s.redisClient == nil {
		return
	}
	existing, err := s.redisClient.GetHealingStatusStruct(taskID)
	if err != nil || existing == nil {
		existing = &storage.HealingStatus{
			TaskID:           taskID,
			Status:           "healing",
			StartedAt:        time.Now(),
			MaxAttempts:      s.maxRetries,
			ConcurrencyLimit: baselineHealingWorkerCount,
		}
	}
	existing.Steps = append(existing.Steps, storage.HealingStep{
		Phase:     phase,
		Status:    status,
		Summary:   summary,
		Timestamp: time.Now(),
	})
	if err := s.redisClient.SetHealingStatusStruct(existing); err != nil {
		logger.Error("failed to append healing step", zap.Error(err), zap.String("task_id", taskID))
	}
}

func (s *SelfHealingService) AppendHealingStep(taskID string, phase string, status string, summary string) {
	s.appendHealingStep(taskID, phase, status, summary)
}

func (s *SelfHealingService) AppendHealingStepByID(healingID uuid.UUID, phase string, status string, summary string) {
	healingLog, err := s.healingLogRepo.FindByID(healingID)
	if err != nil {
		logger.Error("failed to append healing step by healing id",
			zap.String("healing_id", healingID.String()),
			zap.Error(err))
		return
	}
	s.appendHealingStep(healingLog.OriginalTaskID.String(), phase, status, summary)
}

func (s *SelfHealingService) GetHealingOriginTaskID(healingID uuid.UUID) (uuid.UUID, error) {
	healingLog, err := s.healingLogRepo.FindByID(healingID)
	if err != nil {
		return uuid.Nil, err
	}
	return healingLog.OriginalTaskID, nil
}

func (s *SelfHealingService) MarkHealingSucceededByID(ctx context.Context, healingID uuid.UUID, attempt int, summary string) {
	healingLog, err := s.healingLogRepo.FindByID(healingID)
	if err != nil {
		logger.Error("failed to load healing log for success mark",
			zap.String("healing_id", healingID.String()),
			zap.Error(err))
		return
	}
	s.appendHealingStep(healingLog.OriginalTaskID.String(), "verify_check", "success", summary)
	s.updateHealingStatusInRedis(healingLog.OriginalTaskID.String(), "healed", attempt, "")

	scriptVersionID := uuid.Nil
	if healingLog.FinalScriptVersionID != nil {
		scriptVersionID = *healingLog.FinalScriptVersionID
	}
	if err := s.healingLogRepo.MarkCompleted(healingID, scriptVersionID); err != nil {
		logger.Error("failed to mark baseline healing completed",
			zap.String("healing_id", healingID.String()),
			zap.Error(err))
	}
}

func (s *SelfHealingService) MarkHealingFailedByID(ctx context.Context, healingID uuid.UUID, reason string) {
	healingLog, err := s.healingLogRepo.FindByID(healingID)
	if err != nil {
		logger.Error("failed to load healing log for failed mark",
			zap.String("healing_id", healingID.String()),
			zap.Error(err))
		return
	}
	s.appendHealingStep(healingLog.OriginalTaskID.String(), "final", "failed", reason)
	s.updateHealingStatusInRedis(healingLog.OriginalTaskID.String(), "failed", healingLog.TotalAttempts, reason)
	if err := s.healingLogRepo.MarkFailed(healingID); err != nil {
		logger.Error("failed to mark baseline healing failed",
			zap.String("healing_id", healingID.String()),
			zap.Error(err))
	}
	s.healingLogRepo.UpdateLastError(healingID, reason)
}

func (s *SelfHealingService) GetHealingStatus(taskID string) *storage.HealingStatus {
	if s.redisClient == nil {
		return nil
	}

	status, err := s.redisClient.GetHealingStatusStruct(taskID)
	if err != nil {
		logger.Error("failed to get healing status from Redis",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return nil
	}

	return status
}

const HealingTimeout = 5 * time.Minute

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
	s.updateHealingStatusInRedis(task.OriginalTaskID.String(), "healing", 0, "")
	s.appendHealingStep(task.OriginalTaskID.String(), "react_start", "running",
		fmt.Sprintf("Worker %d 开始执行 ReAct 修复循环", workerID))

	healingLog := &model.HealingLog{
		OriginalTaskID:  task.OriginalTaskID,
		RuleID:          task.RuleID,
		HostID:          task.HostID,
		ScriptType:      task.ScriptType,
		TriggerError:    task.ErrorMessage,
		TriggerExitCode: task.ExitCode,
		TotalAttempts:   0,
		MaxAttempts:     s.maxRetries,
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
		return
	}

	config, err := s.configRepo.GetActive()
	if err != nil {
		logger.Error("failed to get LLM config",
			zap.Error(err),
			zap.String("original_task_id", task.OriginalTaskID.String()),
		)
		return
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		logger.Error("failed to decrypt API key",
			zap.Error(err),
			zap.String("original_task_id", task.OriginalTaskID.String()),
		)
		return
	}

	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 120, 3)

	var lastError string
	var fixedScript string
	var scriptVersionID uuid.UUID

	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		logger.Info("healing attempt",
			zap.Stringer("healing_id", healingLog.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", s.maxRetries),
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
		s.appendHealingStep(task.OriginalTaskID.String(), "react_plan", "running",
			fmt.Sprintf("第 %d 轮：使用 agent-runtime 分析失败原因并生成修复策略", attempt))
		runtimePlan := s.runBaselineHealingRuntime(ctx, llmClient, task, history, attempt)
		if runtimePlan != "" {
			s.appendHealingStep(task.OriginalTaskID.String(), "react_plan", "success", truncateForStep(runtimePlan, 220))
		}
		prompt := llm.GetSelfHealingFixPrompt(task.ScriptContent, task.ErrorMessage, task.ExitCode, history)
		if runtimePlan != "" {
			prompt = fmt.Sprintf("%s\n\nagent-runtime ReAct 修复策略：\n%s", prompt, runtimePlan)
		}

		if task.UserSuggestion != "" {
			prompt = fmt.Sprintf("%s\n\n用户提供的修复建议：%s", prompt, task.UserSuggestion)
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
			s.appendHealingStep(task.OriginalTaskID.String(), "audit", "failed", lastError)
			continue
		}
		if !auditResult.Passed {
			logger.Error("healed script audit not passed",
				zap.String("error", auditResult.ErrorMsg),
				zap.Stringer("healing_id", healingLog.ID),
				zap.Int("attempt", attempt),
			)
			lastError = fmt.Sprintf("脚本审计未通过：%s", auditResult.ErrorMsg)
			s.appendHealingStep(task.OriginalTaskID.String(), "audit", "blocked", lastError)
			continue
		}

		fixedScript = auditResult.Script
		s.appendHealingStep(task.OriginalTaskID.String(), "audit", "success", "修复脚本已通过命令审计，准备进入下发流程")

		// 创建脚本版本记录
		version := attempt // 自愈版本从 1 开始
		scriptVersion := &model.ScriptVersion{
			RuleID:           task.RuleID,
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
		healingLog.FinalScriptVersionID = &scriptVersionID

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

		// 记录尝试详情
		healingLog.AttemptsDetail = append(healingLog.AttemptsDetail, model.AttemptDetail{
			Attempt:         attempt,
			ScriptVersionID: scriptVersionID.String(),
			ErrorInput:      task.ErrorMessage,
			LLMFixSummary:   "Script regenerated by LLM",
			ResultExitCode:  0, // 假设成功，实际执行由 Agent 完成
			ResultStderr:    "",
			Timestamp:       time.Now(),
		})

		// 更新自愈日志
		if err := s.healingLogRepo.Update(healingLog); err != nil {
			logger.Error("failed to update healing log",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
			)
		}

		logger.Info("healing attempt completed",
			zap.Stringer("healing_id", healingLog.ID),
			zap.Int("attempt", attempt),
			zap.String("script_version_id", scriptVersionID.String()),
		)

		// 更新规则的脚本（仅对基线任务）
		if task.RuleID != nil {
			if err := s.updateRuleScript(*task.RuleID, task.ScriptType, fixedScript); err != nil {
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

		if s.onHealedScript != nil && task.TaskGroupID != uuid.Nil && task.RuleID != nil && strings.EqualFold(task.ScriptType, "FIX") {
			s.appendHealingStep(task.OriginalTaskID.String(), "dispatch_fix", "running",
				fmt.Sprintf("第 %d 轮修复脚本已生成，开始创建并下发修复任务", task.AttemptNo))
			if err := s.onHealedScript(ctx, task, healingLog.ID, fixedScript, attempt); err != nil {
				lastError = fmt.Sprintf("下发修复任务失败：%v", err)
				s.appendHealingStep(task.OriginalTaskID.String(), "dispatch_fix", "failed", lastError)
				continue
			}
			s.appendHealingStep(task.OriginalTaskID.String(), "dispatch_fix", "success", "修复任务已下发，等待 Agent 返回执行结果")
			return
		}

		// 成功修复，标记为完成
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

func (s *SelfHealingService) updateRuleScript(ruleID uuid.UUID, scriptType, scriptContent string) error {
	return s.ruleRepo.UpdateScript(ruleID, scriptType, scriptContent, 0)
}

func (s *SelfHealingService) runBaselineHealingRuntime(ctx context.Context, llmClient *llm.LLMClient, task HealingTask, history string, attempt int) string {
	if llmClient == nil {
		return ""
	}

	runtimeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	runtime, err := agentruntime.New(
		agentruntime.WithLLMClient(&baselineHealingLLMAdapter{client: llmClient}),
		agentruntime.WithPromptProvider(&baselineHealingPromptProvider{}),
		agentruntime.WithConfig(agentruntime.RuntimeConfig{
			MaxTotalTurns:         8,
			MaxPlanSteps:          3,
			MaxStepReactTurns:     3,
			MaxToolCalls:          1,
			MaxToolCallsPerStep:   1,
			MaxToolFailures:       1,
			MaxModelFailures:      2,
			MaxParseFailures:      2,
			MaxNoProgressTurns:    2,
			TaskTimeout:           90 * time.Second,
			ModelTimeout:          60 * time.Second,
			ToolTimeout:           5 * time.Second,
			HookTimeout:           5 * time.Second,
			EnableReflection:      true,
			EnableAudit:           true,
			EnableCorrection:      true,
			MaxAudits:             2,
			MaxCorrections:        2,
			MaxReflections:        2,
			MaxStepRetries:        1,
			AllowDynamicNewSteps:  false,
			AllowSkipFailedStep:   false,
			AllowBestEffortAnswer: true,
			AllowHighRiskTools:    false,
			AllowDangerousTools:   false,
			MaxContextTokens:      32000,
			ReservedOutputTokens:  2048,
			RecentTurnsToKeep:     4,
		}),
	)
	if err != nil {
		logger.Warn("failed to create baseline healing agent-runtime", zap.Error(err))
		return ""
	}

	result, err := runtime.Run(runtimeCtx, agentruntime.TaskInput{
		TaskID: fmt.Sprintf("baseline-healing-%s-%d", task.OriginalTaskID.String(), attempt),
		UserInput: fmt.Sprintf(`请基于以下上下文输出本轮修复策略：
- 任务ID：%s
- 主机ID：%s
- 规则ID：%s
- 脚本类型：%s
- 当前轮次：%d/%d
- 退出码：%d
- 错误信息：%s
- 用户建议：%s
- 历史尝试：%s

要求：不要直接执行命令；只输出可用于生成 Shell 修复脚本的策略、风险点、验证方式和回滚注意事项。`,
			task.OriginalTaskID.String(),
			task.HostID.String(),
			uuidPtrString(task.RuleID),
			task.ScriptType,
			task.AttemptNo,
			task.MaxRounds,
			task.ExitCode,
			task.ErrorMessage,
			task.UserSuggestion,
			history,
		),
		UserContext: map[string]any{
			"baseline_healing": true,
			"host_id":          task.HostID.String(),
			"rule_id":          uuidPtrString(task.RuleID),
			"script_type":      task.ScriptType,
			"attempt_no":       task.AttemptNo,
			"max_rounds":       task.MaxRounds,
		},
		Metadata: map[string]string{
			"source": "baseline_react_healing",
		},
	})
	if err != nil {
		logger.Warn("baseline healing agent-runtime failed", zap.Error(err), zap.String("task_id", task.OriginalTaskID.String()))
		return ""
	}
	if result == nil {
		return ""
	}
	return result.FinalAnswer
}

type baselineHealingPromptProvider struct{}

func (p *baselineHealingPromptProvider) Build(_ context.Context, req agentruntime.PromptRequest) (agentruntime.PromptBundle, error) {
	switch req.Purpose {
	case agentruntime.PurposePlan:
		return agentruntime.PromptBundle{
			SystemPrompt: baselineHealingPlanPrompt,
		}, nil
	case agentruntime.PurposeReact:
		return agentruntime.PromptBundle{
			SystemPrompt: baselineHealingReactPrompt,
		}, nil
	case agentruntime.PurposeSummarize:
		return agentruntime.PromptBundle{
			SystemPrompt: baselineHealingSummarizePrompt,
		}, nil
	default:
		return agentruntime.PromptBundle{}, nil
	}
}

const baselineHealingPlanPrompt = `你是 Aegis 基线修复 ReAct 智能体，目标是分析一次基线检测/修复失败并制定下一轮修复策略。

边界：
- 不直接执行命令。
- 不绕过命令审计。
- 不建议破坏性命令，除非给出明确风险和回滚方式。
- 多条规则并发修复时，必须假设同一主机存在资源竞争，优先建议幂等、可重复执行、带锁或状态判断的脚本。

输出计划必须覆盖：
1. 失败原因假设。
2. 修复动作。
3. 下发前审计关注点。
4. 修复后检测验证方式。
5. 并发冲突与回滚注意事项。`

const baselineHealingReactPrompt = `你正在执行基线修复策略生成步骤。必须输出 JSON：
{"action":"step_result","summary":"一句话总结","step_result":{"result":"修复策略详情","evidence":["依据"],"confidence":"high/medium/low"}}

不要调用工具；不要输出真实执行命令；只给后续脚本生成器使用的策略。`

const baselineHealingSummarizePrompt = `请总结本次基线 ReAct 修复策略，输出中文短文，包含：失败原因、修复思路、审计重点、验证条件、并发注意事项。`

func uuidPtrString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func truncateForStep(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

type baselineHealingLLMAdapter struct {
	client *llm.LLMClient
}

func (a *baselineHealingLLMAdapter) Complete(ctx context.Context, req agentruntime.LLMRequest) (agentruntime.LLMResponse, error) {
	if a == nil || a.client == nil {
		return agentruntime.LLMResponse{}, fmt.Errorf("llm client is nil")
	}

	messages := make([]llm.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		messages = append(messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	temperature := baselineHealingTemperature(req.Purpose)
	if req.Temperature != nil {
		temperature = float64(*req.Temperature)
	}

	result, err := a.client.ChatCompletionWithMessagesFormatResult(ctx, messages, temperature, nil)
	if err != nil {
		return agentruntime.LLMResponse{}, err
	}

	return agentruntime.LLMResponse{
		Content: result.Content,
		Model:   result.Model,
		Usage: agentruntime.LLMUsage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
	}, nil
}

func baselineHealingTemperature(purpose agentruntime.LLMPurpose) float64 {
	switch purpose {
	case agentruntime.PurposePlan:
		return 0.3
	case agentruntime.PurposeReact:
		return 0.2
	case agentruntime.PurposeAudit, agentruntime.PurposeReflect, agentruntime.PurposeCorrect, agentruntime.PurposeSummarize:
		return 0.2
	default:
		return 0.3
	}
}

func (s *SelfHealingService) GetHealingLogByTaskID(taskID uuid.UUID) (*model.HealingLog, error) {
	return s.healingLogRepo.GetLatestByOriginalTaskID(taskID)
}

// ShouldTriggerHealing 判断是否应该触发自愈
func (s *SelfHealingService) ShouldTriggerHealing(scriptType string, exitCode int) bool {
	// 只有 FIX 脚本失败才触发自愈
	if scriptType != "FIX" {
		return false
	}

	// 非 0 退出码才触发
	return exitCode != 0
}

// Helper functions
func pointerToTime(t time.Time) *time.Time {
	return &t
}
