package service

import (
	"context"
	"fmt"
	"strings"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/storage"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var _ ScriptGenerator = (*scriptGeneratorFunc)(nil)

type ScriptGenerationService struct {
	ruleRepo           *repository.RuleRepository
	scriptVersionRepo  *repository.ScriptVersionRepository
	configRepo         *repository.ConfigRepository
	minioClient        *storage.MinIOClient
	scriptAuditService *ScriptAuditService
	generateQueue      chan GenerateTask
	workerCount        int
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
	scriptAuditService *ScriptAuditService,
) *ScriptGenerationService {
	return &ScriptGenerationService{
		ruleRepo:           ruleRepo,
		scriptVersionRepo:  scriptVersionRepo,
		configRepo:         configRepo,
		minioClient:        minioClient,
		scriptAuditService: scriptAuditService,
		generateQueue:      make(chan GenerateTask, 100),
		workerCount:        workerCount,
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
		prompt = llm.GetFixScriptGenerationPrompt(rule.CheckContent, rule.FixContent)
	}

	systemPrompt := "你是一位资深的 Shell 脚本工程师"
	generator := &scriptGeneratorFunc{fn: func(genCtx context.Context, _ *AuditRequest) (string, error) {
		return llmClient.ChatCompletion(genCtx, systemPrompt, prompt, 0.1)
	}}

	scriptType := ScriptTypeBaselineCheck
	if task.ScriptType == "FIX" {
		scriptType = ScriptTypeBaselineFix
	}

	auditReq := &AuditRequest{
		ScriptContent: prompt,
		ScriptType:    scriptType,
		TaskID:        task.RuleID.String(),
		RuleID:        task.RuleID.String(),
		Source:        AuditSourceGeneration,
	}

	auditResult, err := s.scriptAuditService.AuditWithRetry(ctx, generator, auditReq)
	if err != nil {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("脚本审计失败: %v", err))
		return
	}
	if !auditResult.Passed {
		s.ruleRepo.UpdateScriptStatusWithError(task.RuleID, task.ScriptType, "failed", fmt.Sprintf("脚本审计未通过: %s", auditResult.ErrorMsg))
		return
	}

	script := auditResult.Script

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
		GenerationSource: "llm_generated",
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
