package service

import (
	"context"
	"fmt"
	"strings"

	"baseline-system/internal/llm"
	"baseline-system/internal/model"
	"baseline-system/internal/repository"
	"baseline-system/internal/storage"
	"baseline-system/pkg/logger"

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

	rule, err := s.ruleRepo.FindByID(task.RuleID)
	if err != nil {
		logger.Error("failed to find rule",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
	}

	config, err := s.configRepo.GetActive()
	if err != nil {
		logger.Error("failed to get LLM config",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		logger.Error("failed to decrypt API key",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
	}

	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 30, 3)

	var prompt string
	if task.ScriptType == "CHECK" {
		prompt = llm.GetCheckScriptGenerationPrompt(rule.CheckContent)
	} else {
		prompt = llm.GetFixScriptGenerationPrompt(rule.FixContent)
	}

	llmResponse, err := llmClient.ChatCompletion(ctx, "你是一位资深的 Shell 脚本工程师", prompt, 0.1)
	if err != nil {
		logger.Error("failed to call LLM for script generation",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
			zap.String("script_type", task.ScriptType),
		)
		return
	}

	// 解析脚本
	script, err := llm.ParseScript(llmResponse)
	if err != nil {
		logger.Error("failed to parse generated script",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
	}

	// 安全性校验
	if err := s.validateScript(script); err != nil {
		logger.Error("script validation failed",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
			zap.String("script_type", task.ScriptType),
		)
		return
	}

	// 获取当前版本号
	var version int
	if task.ScriptType == "CHECK" {
		version = rule.CheckScriptVersion + 1
	} else {
		version = rule.FixScriptVersion + 1
	}

	// 创建脚本版本记录
	scriptVersion := &model.ScriptVersion{
		RuleID:           task.RuleID,
		ScriptType:       task.ScriptType,
		Version:          version,
		ScriptContent:    script,
		GenerationSource: "llm",
		IsCurrent:        true,
	}

	if err := s.scriptVersionRepo.Create(scriptVersion); err != nil {
		logger.Error("failed to create script version",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
	}

	// 上传脚本到 MinIO
	minIOPath := fmt.Sprintf("%s/%d/%s.sh", task.RuleID.String(), version, strings.ToLower(task.ScriptType))
	_, err = s.minioClient.UploadFile("generated-scripts", minIOPath, strings.NewReader(script), int64(len(script)), "application/x-sh")
	if err != nil {
		logger.Error("failed to upload script to MinIO",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
	}

	// 更新规则表的脚本版本
	if err := s.ruleRepo.UpdateScript(task.RuleID, task.ScriptType, script, version); err != nil {
		logger.Error("failed to update rule script",
			zap.Error(err),
			zap.String("rule_id", task.RuleID.String()),
		)
		return
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

// GenerateFixScript 生成修复脚本
func (s *ScriptGenerationService) GenerateFixScript(ctx context.Context, ruleID uuid.UUID) error {
	return s.QueueScriptGeneration(ruleID, "FIX")
}
