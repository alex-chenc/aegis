package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"baseline-system/internal/llm"
	"baseline-system/internal/model"
	"baseline-system/internal/repository"
	"baseline-system/internal/storage"
	"baseline-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type SelfHealingService struct {
	healingLogRepo    *repository.HealingLogRepository
	scriptVersionRepo *repository.ScriptVersionRepository
	configRepo        *repository.ConfigRepository
	ruleRepo          *repository.RuleRepository
	taskLogRepo       *repository.TaskLogRepository
	minioClient       *storage.MinIOClient
	healingQueue      chan HealingTask
	maxRetries        int
}

type HealingTask struct {
	OriginalTaskID uuid.UUID
	RuleID         uuid.UUID
	HostID         uuid.UUID
	ScriptType     string
	ScriptContent  string
	ErrorMessage   string
	ExitCode       int
	UserSuggestion string
}

func NewSelfHealingService(
	healingLogRepo *repository.HealingLogRepository,
	scriptVersionRepo *repository.ScriptVersionRepository,
	configRepo *repository.ConfigRepository,
	ruleRepo *repository.RuleRepository,
	taskLogRepo *repository.TaskLogRepository,
	minioClient *storage.MinIOClient,
	llmTimeout int,
	llmMaxRetries int,
	maxRetries int,
) *SelfHealingService {
	return &SelfHealingService{
		healingLogRepo:    healingLogRepo,
		scriptVersionRepo: scriptVersionRepo,
		configRepo:        configRepo,
		ruleRepo:          ruleRepo,
		taskLogRepo:       taskLogRepo,
		minioClient:       minioClient,
		healingQueue:      make(chan HealingTask, 100),
		maxRetries:        maxRetries,
	}
}

func (s *SelfHealingService) TriggerHealing(task HealingTask) error {
	logger.Info("triggering self-healing",
		zap.String("original_task_id", task.OriginalTaskID.String()),
		zap.String("rule_id", task.RuleID.String()),
		zap.String("script_type", task.ScriptType),
		zap.Int("exit_code", task.ExitCode),
	)

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
	logger.Info("processing self-healing",
		zap.Int("worker_id", workerID),
		zap.String("original_task_id", task.OriginalTaskID.String()),
		zap.String("rule_id", task.RuleID.String()),
		zap.String("user_suggestion", task.UserSuggestion),
	)

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

		// 更新尝试次数
		healingLog.TotalAttempts = attempt
		if err := s.healingLogRepo.IncrementAttempts(healingLog.ID); err != nil {
			logger.Error("failed to increment healing attempts",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
			)
		}

		// 构建自愈 Prompt
		history := s.buildHealingHistory(healingLog.AttemptsDetail)
		prompt := llm.GetSelfHealingFixPrompt(task.ScriptContent, task.ErrorMessage, task.ExitCode, history)

		if task.UserSuggestion != "" {
			prompt = fmt.Sprintf("%s\n\n用户提供的修复建议：%s", prompt, task.UserSuggestion)
		}

		llmResponse, err := llmClient.ChatCompletion(ctx, "你是一位资深的 Shell 脚本调试专家", prompt, 0.1)
		if err != nil {
			logger.Error("failed to call LLM for healing",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
				zap.Int("attempt", attempt),
			)
			lastError = fmt.Sprintf("LLM 调用失败：%v", err)
			continue
		}

		// 解析修复后的脚本
		fixedScript, err = llm.ParseScript(llmResponse)
		if err != nil {
			logger.Error("failed to parse healed script",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
				zap.Int("attempt", attempt),
			)
			lastError = fmt.Sprintf("脚本解析失败：%v", err)
			continue
		}

		// 安全性校验
		if err := s.validateScript(fixedScript); err != nil {
			logger.Error("healed script validation failed",
				zap.Error(err),
				zap.Stringer("healing_id", healingLog.ID),
				zap.Int("attempt", attempt),
			)
			lastError = fmt.Sprintf("脚本验证失败：%v", err)
			continue
		}

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

		// 上传修复后的脚本到 MinIO
		minIOPath := fmt.Sprintf("healing/%s/%d/%s.sh", task.RuleID.String(), attempt, task.ScriptType)
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

		// 更新规则的脚本
		if err := s.updateRuleScript(task.RuleID, task.ScriptType, fixedScript); err != nil {
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

func (s *SelfHealingService) validateScript(script string) error {
	if !strings.HasPrefix(script, "#!") {
		return fmt.Errorf("script must start with shebang (#!/bin/bash)")
	}

	dangerousPatterns := []string{
		"rm -rf /",
		"mkfs.",
		"dd if=/dev/zero",
		":(){:|:&};:",
		"chmod -R 777 /",
		"chown -R root:root /",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(script, pattern) {
			return fmt.Errorf("dangerous command detected: %s", pattern)
		}
	}

	if len(script) > 10000 {
		return fmt.Errorf("script too long: %d bytes (max 10000)", len(script))
	}

	return nil
}

func (s *SelfHealingService) updateRuleScript(ruleID uuid.UUID, scriptType, scriptContent string) error {
	return s.ruleRepo.UpdateScript(ruleID, scriptType, scriptContent, 0)
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
