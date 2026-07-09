package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"api-server/internal/checker"
	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"go.uber.org/zap"
)

type ScriptType string

const (
	ScriptTypeBaselineCheck    ScriptType = "baseline_check"
	ScriptTypeBaselineFix      ScriptType = "baseline_fix"
	ScriptTypeVulnerabilityFix ScriptType = "vulnerability_fix"
	ScriptTypePocVerify        ScriptType = "poc_verify"
	ScriptTypeSelfHealing      ScriptType = "self_healing"
)

type AuditSource string

const (
	AuditSourceGeneration AuditSource = "generation"
	AuditSourceDispatch   AuditSource = "dispatch"
	AuditSourceAgent      AuditSource = "agent"
)

type AuditRequest struct {
	ScriptContent string
	ScriptType    ScriptType
	Context       string
	TaskID        string
	RuleID        string
	Source        AuditSource
}

type AuditResult struct {
	Passed        bool                   `json:"passed"`
	RiskLevel     string                 `json:"risk_level"`
	BlacklistHits []checker.BlacklistHit `json:"blacklist_hits,omitempty"`
	AIAnalysis    *AIAnalysisResult      `json:"ai_analysis,omitempty"`
	AuditLogID    string                 `json:"audit_log_id,omitempty"`
	Attempt       int                    `json:"attempt"`
	Script        string                 `json:"script,omitempty"`
	ErrorMsg      string                 `json:"error_msg,omitempty"`
}

type AIAnalysisResult struct {
	Passed    bool         `json:"passed"`
	RiskLevel string       `json:"risk_level"`
	Issues    []AuditIssue `json:"issues"`
	Summary   string       `json:"summary"`
}

type AuditIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	LineRange   string `json:"line_range"`
	Suggestion  string `json:"suggestion"`
}

// ScriptGenerator is the interface for script generation used by AuditWithRetry
type ScriptGenerator interface {
	Generate(ctx context.Context, req *AuditRequest) (string, error)
}

// scriptGeneratorFunc adapts a closure to the ScriptGenerator interface
type scriptGeneratorFunc struct {
	fn func(ctx context.Context, req *AuditRequest) (string, error)
}

func (f *scriptGeneratorFunc) Generate(ctx context.Context, req *AuditRequest) (string, error) {
	return f.fn(ctx, req)
}

type ScriptAuditService struct {
	blacklistChecker *checker.BlacklistChecker
	auditLogRepo     *repository.AuditLogRepo
	configRepo       *repository.ConfigRepository
	sysConfigRepo    *repository.SystemConfigRepo
	ruleRepo         *repository.CommandAuditRuleRepo
	llmTimeout       int
	llmRetries       int
}

func NewScriptAuditService(
	blacklistChecker *checker.BlacklistChecker,
	auditLogRepo *repository.AuditLogRepo,
	configRepo *repository.ConfigRepository,
	sysConfigRepo *repository.SystemConfigRepo,
	ruleRepo *repository.CommandAuditRuleRepo,
	llmTimeout int,
	llmRetries int,
) *ScriptAuditService {
	return &ScriptAuditService{
		blacklistChecker: blacklistChecker,
		auditLogRepo:     auditLogRepo,
		configRepo:       configRepo,
		sysConfigRepo:    sysConfigRepo,
		ruleRepo:         ruleRepo,
		llmTimeout:       llmTimeout,
		llmRetries:       llmRetries,
	}
}

// ReloadRules reloads blacklist rules from database
func (s *ScriptAuditService) ReloadRules(ctx context.Context) error {
	rules, err := s.ruleRepo.FindAllEnabled()
	if err != nil {
		return fmt.Errorf("failed to load rules: %w", err)
	}
	return s.blacklistChecker.LoadRules(rules)
}

// AuditWithRetry performs blacklist + AI audit with retry on failure
func (s *ScriptAuditService) AuditWithRetry(ctx context.Context, generator ScriptGenerator, req *AuditRequest) (*AuditResult, error) {
	settings, err := s.sysConfigRepo.GetCommandAuditSettings()
	if err != nil {
		logger.Error("failed to get audit settings, using defaults", zap.Error(err))
		defaultSettings := model.DefaultCommandAuditSettings()
		settings = &defaultSettings
	}

	maxRetry := settings.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 3
	}

	var previousAudits []string

	for attempt := 1; attempt <= maxRetry; attempt++ {
		startTime := time.Now()

		// 1. Generate script
		scriptContent, err := generator.Generate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("script generation failed (attempt %d): %w", attempt, err)
		}

		parsedScript, parseErr := llm.ParseScript(scriptContent)
		if parseErr != nil {
			return nil, fmt.Errorf("script parse failed (attempt %d): %w", attempt, parseErr)
		}

		// 2. Blacklist audit
		blacklistResult, checkErr := s.blacklistChecker.Check(parsedScript, string(req.ScriptType))
		if checkErr != nil {
			logger.Error("blacklist check failed, retrying",
				zap.Error(checkErr),
				zap.Int("attempt", attempt),
			)
			previousAudits = append(previousAudits, fmt.Sprintf("黑名单检查内部错误: %v", checkErr))
			continue
		}

		logEntry := &model.ScriptAuditLog{
			TaskID:        req.TaskID,
			RuleID:        req.RuleID,
			ScriptType:    string(req.ScriptType),
			ScriptContent: parsedScript,
			AuditSource:   string(req.Source),
			Attempt:       attempt,
		}

		if settings.BlacklistEnabled && blacklistResult.HasViolation && blacklistResult.HasHardBlock() {
			logEntry.Passed = false
			logEntry.RiskLevel = "critical"
			hitsJSON, _ := json.Marshal(blacklistResult.Hits)
			logEntry.BlacklistHits = hitsJSON
			logEntry.DurationMs = time.Since(startTime).Milliseconds()
			s.auditLogRepo.Create(logEntry)

			previousAudits = append(previousAudits, formatBlacklistFailure(blacklistResult))
			continue
		}

		// 3. AI audit
		if settings.AIEnabled {
			aiResult, aiErr := s.auditAI(ctx, req, parsedScript, blacklistResult, previousAudits)
			if aiErr != nil {
				logger.Warn("AI audit failed, retrying",
					zap.Error(aiErr),
					zap.Int("attempt", attempt),
				)
				logEntry.Passed = false
				logEntry.RiskLevel = "unknown"
				logEntry.DurationMs = time.Since(startTime).Milliseconds()
				s.auditLogRepo.Create(logEntry)

				previousAudits = append(previousAudits, fmt.Sprintf("AI审计调用失败: %v", aiErr))
				continue
			}
			if !aiResult.Passed {
				logEntry.Passed = false
				logEntry.RiskLevel = aiResult.RiskLevel
				aiJSON, _ := json.Marshal(aiResult)
				logEntry.AIAnalysis = aiJSON
				logEntry.DurationMs = time.Since(startTime).Milliseconds()
				s.auditLogRepo.Create(logEntry)

				previousAudits = append(previousAudits, formatAIFailure(aiResult))
				continue
			}
		}

		// 4. Passed
		logEntry.Passed = true
		logEntry.RiskLevel = "safe"
		logEntry.DurationMs = time.Since(startTime).Milliseconds()
		s.auditLogRepo.Create(logEntry)

		return &AuditResult{
			Passed:    true,
			RiskLevel: "safe",
			Attempt:   attempt,
			Script:    parsedScript,
		}, nil
	}

	return &AuditResult{
		Passed:    false,
		RiskLevel: "critical",
		Attempt:   settings.MaxRetry,
		ErrorMsg:  fmt.Sprintf("audit failed after %d attempts", settings.MaxRetry),
	}, nil
}

// AuditForDispatch performs blacklist-only audit before dispatching to agent
func (s *ScriptAuditService) AuditForDispatch(ctx context.Context, content string, taskID string) (*AuditResult, error) {
	startTime := time.Now()

	blacklistResult, err := s.blacklistChecker.Check(content, "all")
	if err != nil {
		return nil, fmt.Errorf("blacklist check failed: %w", err)
	}

	logEntry := &model.ScriptAuditLog{
		TaskID:      taskID,
		ScriptType:  "all",
		AuditSource: string(AuditSourceDispatch),
		Attempt:     1,
	}

	if blacklistResult.HasViolation && blacklistResult.HasHardBlock() {
		logEntry.Passed = false
		logEntry.RiskLevel = "critical"
		logEntry.ScriptContent = content
		hitsJSON, _ := json.Marshal(blacklistResult.Hits)
		logEntry.BlacklistHits = hitsJSON
		logEntry.DurationMs = time.Since(startTime).Milliseconds()
		s.auditLogRepo.Create(logEntry)

		return &AuditResult{
			Passed:        false,
			RiskLevel:     "critical",
			BlacklistHits: blacklistResult.Hits,
			Attempt:       1,
		}, nil
	}

	logEntry.Passed = true
	logEntry.RiskLevel = "safe"
	logEntry.DurationMs = time.Since(startTime).Milliseconds()
	s.auditLogRepo.Create(logEntry)

	return &AuditResult{Passed: true, RiskLevel: "safe", Attempt: 1}, nil
}

func (s *ScriptAuditService) auditAI(ctx context.Context, req *AuditRequest, script string, blacklistResult *checker.CheckResult, previousAudits []string) (*AIAnalysisResult, error) {
	llmClient, err := s.getLLMClient(ctx)
	if err != nil {
		logger.Warn("LLM client unavailable, skipping AI audit", zap.Error(err))
		return nil, err
	}

	userPrompt := buildAuditUserPrompt(req, script, previousAudits)
	response, err := llmClient.ChatCompletion(ctx, llm.ScriptAuditSystemPrompt, userPrompt, 0.1)
	if err != nil {
		logger.Warn("AI audit failed", zap.Error(err))
		return nil, err
	}

	result, err := parseAIAnalysisResult(response)
	if err != nil {
		logger.Warn("failed to parse AI audit result", zap.Error(err))
		return nil, err
	}

	return result, nil
}

func (s *ScriptAuditService) getLLMClient(ctx context.Context) (*llm.LLMClient, error) {
	if s.configRepo == nil {
		return nil, fmt.Errorf("config repository not configured")
	}

	config, err := s.configRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM config: %w", err)
	}

	if config.APIKeyEncrypted == "" {
		return nil, fmt.Errorf("LLM API key not configured")
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	return llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmRetries), nil
}

func buildAuditUserPrompt(req *AuditRequest, script string, previousAudits []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Audit the security of this %s script.\n\n", req.ScriptType))

	if req.Context != "" {
		sb.WriteString("## Script context\n")
		sb.WriteString(req.Context)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Script\n```bash\n")
	sb.WriteString(script)
	sb.WriteString("\n```\n")

	if len(previousAudits) > 0 {
		sb.WriteString("\n## Previous failed audits\n")
		for i, audit := range previousAudits {
			sb.WriteString(fmt.Sprintf("Attempt %d: %s\n", i+1, audit))
		}
		sb.WriteString("The script was regenerated from previous audit feedback. Verify that earlier findings are resolved.\n")
	}

	sb.WriteString("\nReturn the audit result as strict JSON.")
	return sb.String()
}

func parseAIAnalysisResult(response string) (*AIAnalysisResult, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in AI response")
	}

	var result AIAnalysisResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI result: %w", err)
	}
	return &result, nil
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(s, "}")
	if end == -1 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func formatBlacklistFailure(result *checker.CheckResult) string {
	var sb strings.Builder
	sb.WriteString("黑名单审计未通过：")
	for _, hit := range result.Hits {
		sb.WriteString(fmt.Sprintf("\n- 第%d行: 包含 `%s`，违反规则\"%s\"(%s)",
			hit.LineNumber, hit.MatchedText, hit.RuleName, hit.Severity))
	}
	return sb.String()
}

func formatAIFailure(result *AIAnalysisResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("AI审计未通过（风险等级: %s）：", result.RiskLevel))
	for _, issue := range result.Issues {
		sb.WriteString(fmt.Sprintf("\n- [%s] %s (行%s): %s",
			issue.Type, issue.Description, issue.LineRange, issue.Suggestion))
	}
	return sb.String()
}
