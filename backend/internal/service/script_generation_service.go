package service

import (
	"context"
	"fmt"
	"strings"

	"aegis-system/internal/llm"
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/internal/storage"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ScriptGenerationService struct {
	ruleRepo          *repository.RuleRepository
	scriptVersionRepo *repository.ScriptVersionRepository
	configRepo        *repository.ConfigRepository
	minioClient       *storage.MinIOClient
	generateQueue     chan GenerateTask
	workerCount       int
}

type GenerateTask struct {
	RuleID     uuid.UUID
	ScriptType string
}

func NewScriptGenerationService(
	ruleRepo *repository.RuleRepository,
	scriptVersionRepo *repository.ScriptVersionRepository,
	configRepo *repository.ConfigRepository,
	minioClient *storage.MinIOClient,
	llmTimeout int,
	llmMaxRetries int,
	workerCount int,
) *ScriptGenerationService {
	return &ScriptGenerationService{
		ruleRepo:          ruleRepo,
		scriptVersionRepo: scriptVersionRepo,
		configRepo:        configRepo,
		minioClient:       minioClient,
		generateQueue:     make(chan GenerateTask, 100),
		workerCount:       workerCount,
	}
}

// StartWorkers 启动脚本生成 Worker
func (s *ScriptGenerationService) StartWorkers(ctx context.Context) {
	logger.Info("starting script generation workers",
		zap.Int("count", s.workerCount),
	)

	for i := 0; i < s.workerCount; i++ {
		go s.generateWorker(ctx, i)
	}
}

func (s *ScriptGenerationService) generateWorker(ctx context.Context, workerID int) {
	logger.Info("script generation worker started",
		zap.Int("worker_id", workerID),
	)

	for {
		select {
		case <-ctx.Done():
			logger.Info("script generation worker stopped",
				zap.Int("worker_id", workerID),
			)
			return
		case task := <-s.generateQueue:
			s.processScriptGeneration(ctx, workerID, task)
		}
	}
}

func (s *ScriptGenerationService) processScriptGeneration(ctx context.Context, workerID int, task GenerateTask) {
	logger.Info("processing script generation",
		zap.Int("worker_id", workerID),
		zap.String("rule_id", task.RuleID.String()),
		zap.String("script_type", task.ScriptType),
	)

	if err := s.ruleRepo.UpdateScriptStatusByType(task.RuleID, task.ScriptType, "generating"); err != nil {
		logger.Error("failed to set generating status",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
	}

	rule, err := s.ruleRepo.FindByID(task.RuleID)
	if err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", "规则不存在")
		return
	}

	config, err := s.configRepo.GetActive()
	if err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", "LLM配置未设置")
		return
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", "API密钥解密失败")
		return
	}

	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 120, 3)

	var prompt string
	if task.ScriptType == "CHECK" {
		prompt = llm.GetCheckScriptGenerationPrompt(rule.CheckContent)
	} else {
		prompt = llm.GetFixScriptGenerationPrompt(rule.FixContent)
	}

	llmResponse, err := llmClient.ChatCompletion(ctx, "你是一位资深的 Shell 脚本工程师", prompt, 0.1)
	if err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("LLM调用失败: %v", err))
		return
	}

	script, err := llm.ParseScript(llmResponse)
	if err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("脚本解析失败: %v", err))
		return
	}

	if err := s.validateScript(script); err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("脚本安全校验失败: %v", err))
		return
	}

	var version int
	if task.ScriptType == "CHECK" {
		version = rule.CheckScriptVersion + 1
	} else {
		version = rule.FixScriptVersion + 1
	}

	scriptVersion := &model.ScriptVersion{
		RuleID:           &task.RuleID,
		ScriptType:       task.ScriptType,
		Version:          version,
		ScriptContent:    script,
		GenerationSource: "llm",
		IsCurrent:        true,
	}

	if err := s.scriptVersionRepo.Create(scriptVersion); err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("版本记录创建失败: %v", err))
		return
	}

	minIOPath := fmt.Sprintf("%s/%d/%s.sh", task.RuleID.String(), version, strings.ToLower(task.ScriptType))
	_, err = s.minioClient.UploadFile("generated-scripts", minIOPath, strings.NewReader(script), int64(len(script)), "application/x-sh")
	if err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("MinIO上传失败: %v", err))
		return
	}

	if err := s.ruleRepo.UpdateScript(task.RuleID, task.ScriptType, script, version); err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("规则更新失败: %v", err))
		return
	}

	if err := s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "generated", ""); err != nil {
		logger.Error("failed to set generated status", zap.Error(err), zap.String("rule_id", task.RuleID.String()))
	}

	logger.Info("script generated successfully",
		zap.String("rule_id", task.RuleID.String()),
		zap.String("script_type", task.ScriptType),
		zap.Int("version", version),
	)
}

// validateScript 脚本安全性校验
func (s *ScriptGenerationService) validateScript(script string) error {
	// 1. Shebang 检查
	if !strings.HasPrefix(script, "#!") {
		return fmt.Errorf("script must start with shebang (#!/bin/bash)")
	}

	// 2. 危险命令检测
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

	// 3. 网络外联检测（可选，根据安全策略）
	// 如果脚本包含 curl/wget 访问外部地址，记录警告
	externalPatterns := []string{
		"curl http",
		"wget http",
	}

	for _, pattern := range externalPatterns {
		if strings.Contains(script, pattern) {
			logger.Warn("script contains external network access",
				zap.String("pattern", pattern),
			)
		}
	}

	// 4. 长度检查
	if len(script) > 10000 {
		return fmt.Errorf("script too long: %d bytes (max 10000)", len(script))
	}

	return nil
}

// QueueScriptGeneration 将脚本生成任务加入队列
func (s *ScriptGenerationService) QueueScriptGeneration(ruleID uuid.UUID, scriptType string) error {
	task := GenerateTask{
		RuleID:     ruleID,
		ScriptType: scriptType,
	}

	select {
	case s.generateQueue <- task:
		logger.Info("script generation queued",
			zap.String("rule_id", ruleID.String()),
			zap.String("script_type", scriptType),
		)
		return nil
	default:
		logger.Error("script generation queue is full",
			zap.String("rule_id", ruleID.String()),
		)
		return fmt.Errorf("script generation queue is full")
	}
}

// GenerateCheckScript 生成检查脚本
func (s *ScriptGenerationService) GenerateCheckScript(ctx context.Context, ruleID uuid.UUID) error {
	return s.QueueScriptGeneration(ruleID, "CHECK")
}

type BatchGenerateResult struct {
	Total     int `json:"total"`
	Queued    int `json:"queued"`
	Skipped   int `json:"skipped"`
	Generated int `json:"generated"`
}

func (s *ScriptGenerationService) BatchGenerateForTemplate(ctx context.Context, templateID uuid.UUID, scriptType string, maxConcurrency int) (*BatchGenerateResult, error) {
	rules, err := s.ruleRepo.FindByTemplateID(templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rules: %w", err)
	}

	result := &BatchGenerateResult{Total: len(rules)}
	var queued, skipped, generated int

	for _, rule := range rules {
		var hasScript bool
		var status string

		if scriptType == "CHECK" {
			hasScript = rule.GeneratedCheckScript != nil && *rule.GeneratedCheckScript != ""
			status = rule.CheckScriptStatus
		} else {
			hasScript = rule.GeneratedFixScript != nil && *rule.GeneratedFixScript != ""
			status = rule.FixScriptStatus
		}

		if hasScript {
			generated++
			continue
		}

		if status == "generating" {
			skipped++
			continue
		}

		if err := s.QueueScriptGeneration(rule.ID, scriptType); err != nil {
			logger.Error("failed to queue script generation",
				zap.Error(err),
				zap.String("rule_id", rule.ID.String()),
			)
			continue
		}
		queued++
	}

	result.Queued = queued
	result.Skipped = skipped
	result.Generated = generated

	logger.Info("batch script generation queued",
		zap.String("template_id", templateID.String()),
		zap.String("script_type", scriptType),
		zap.Int("total", result.Total),
		zap.Int("queued", result.Queued),
		zap.Int("skipped", result.Skipped),
		zap.Int("generated", result.Generated),
	)

	return result, nil
}

// GenerateFixScript 生成修复脚本
func (s *ScriptGenerationService) GenerateFixScript(ctx context.Context, ruleID uuid.UUID) error {
	return s.QueueScriptGeneration(ruleID, "FIX")
}
