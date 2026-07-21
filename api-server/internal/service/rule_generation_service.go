package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
)

// RuleGenerationService AI规则自动更新服务（整合误报分析功能）
type RuleGenerationService struct {
	configService    *AIRuleConfigService
	llmConfigRepo    *repository.ConfigRepository
	sigmaRuleRepo    *repository.SigmaRuleRepository
	alertRepo        *repository.AlertRepository
	notificationRepo *repository.NotificationRepository
	sigmaRuleSvc     *SigmaRuleService
	serverClient     *grpcclient.ServerClient
	llmClient        *llm.LLMClient
	llmTimeout       int
	llmMaxRetries    int
	sampleSize       int
	enabled          bool
	stopCh           chan struct{}
	wg               sync.WaitGroup
}

// NewRuleGenerationService 创建规则生成服务
func NewRuleGenerationService(
	configService *AIRuleConfigService,
	llmConfigRepo *repository.ConfigRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
	alertRepo *repository.AlertRepository,
	notificationRepo *repository.NotificationRepository,
	sigmaRuleSvc *SigmaRuleService,
	serverClient *grpcclient.ServerClient,
	llmTimeout int,
	llmMaxRetries int,
) *RuleGenerationService {
	return &RuleGenerationService{
		configService:    configService,
		llmConfigRepo:    llmConfigRepo,
		sigmaRuleRepo:    sigmaRuleRepo,
		alertRepo:        alertRepo,
		notificationRepo: notificationRepo,
		sigmaRuleSvc:     sigmaRuleSvc,
		serverClient:     serverClient,
		llmTimeout:       llmTimeout,
		llmMaxRetries:    llmMaxRetries,
		sampleSize:       10,
		enabled:          true,
		stopCh:           make(chan struct{}),
	}
}

// InitLLMClient 初始化LLM客户端
func (s *RuleGenerationService) InitLLMClient(apiKey, baseURL, modelName string, timeout, maxRetries int) {
	s.llmClient = llm.NewLLMClient(apiKey, baseURL, modelName, timeout, maxRetries)
}

// Start 启动AI规则自动更新服务
func (s *RuleGenerationService) Start(ctx context.Context) {
	logger.Info("AI rule auto-update service starting")

	// Keep the scheduler alive so config changes made after startup take effect.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.TriggerConfiguredCheck(ctx)
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// 定时检查实验性规则升级
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.checkExperimentalRulesPromotion(ctx)
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	logger.Info("AI rule auto-update service started")
}

// Stop 停止AI规则自动更新服务
func (s *RuleGenerationService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	logger.Info("AI rule auto-update service stopped")
}

// TriggerConfiguredCheck checks the current configured alert window once.
func (s *RuleGenerationService) TriggerConfiguredCheck(ctx context.Context) {
	stats, err := s.collectConfiguredTriggerStats(time.Now())
	if err != nil {
		logger.Error("failed to collect configured rule trigger stats", zap.Error(err))
		return
	}

	if len(stats) == 0 {
		return
	}

	logger.Info("configured rule trigger stats collected",
		zap.Int("rules_over_threshold", len(stats)))

	for _, stat := range stats {
		go s.analyzeRule(ctx, stat, stat.TimeWindow)
	}
}

func (s *RuleGenerationService) collectConfiguredTriggerStats(now time.Time) ([]repository.RuleTriggerStats, error) {
	if !s.configService.IsEnabled() {
		return nil, nil
	}

	thresholds := s.configService.GetThresholds()
	if thresholds == nil {
		thresholds = &model.Thresholds{HighFrequencyCount: 10, HighFrequencyHours: 1}
	}
	if thresholds.HighFrequencyCount <= 0 {
		thresholds.HighFrequencyCount = 10
	}
	if thresholds.HighFrequencyHours <= 0 {
		thresholds.HighFrequencyHours = 1
	}

	startTime := now.Add(-time.Duration(thresholds.HighFrequencyHours) * time.Hour)
	stats, err := s.alertRepo.GetRuleTriggerStats(startTime, now, thresholds.HighFrequencyCount, s.sampleSize)
	if err != nil {
		return nil, err
	}

	timeWindow := fmt.Sprintf("%dh", thresholds.HighFrequencyHours)
	for i := range stats {
		stats[i].TimeWindow = timeWindow
	}

	return stats, nil
}

// analyzeRule 分析规则是否为误报，并进行紧收
func (s *RuleGenerationService) analyzeRule(ctx context.Context, stats repository.RuleTriggerStats, timeWindow string) {
	logger.Info("analyzing rule for AI auto-update",
		zap.String("rule_id", stats.RuleID),
		zap.Int("alert_count", stats.AlertCount),
		zap.String("time_window", timeWindow))

	rule, err := s.sigmaRuleRepo.FindByRuleID(stats.RuleID)
	if err != nil {
		logger.Error("failed to find rule", zap.Error(err), zap.String("rule_id", stats.RuleID))
		return
	}

	conservatism := s.configService.GetConservatism()
	if shouldSkipRuleForCooldown(rule, conservatism, time.Now()) {
		logger.Info("skipping rule tightening inside conservative cooldown",
			zap.String("rule_id", stats.RuleID),
			zap.Float64("conservatism", conservatism))
		return
	}

	result, err := s.callLLMForAnalysis(ctx, rule, stats, timeWindow)
	if err != nil {
		logger.Error("LLM analysis failed", zap.Error(err), zap.String("rule_id", stats.RuleID))
		return
	}

	logger.Info("LLM analysis completed",
		zap.String("rule_id", stats.RuleID),
		zap.Bool("is_false_positive", result.IsFalsePositive),
		zap.Float64("confidence", result.Confidence))

	confidenceThreshold := falsePositiveConfidenceThreshold(conservatism)
	if result.IsFalsePositive && result.Confidence >= confidenceThreshold {
		if err := s.applyRuleAdjustment(rule, result.RuleAdjustment, stats); err != nil {
			logger.Error("failed to apply rule adjustment", zap.Error(err), zap.String("rule_id", stats.RuleID))
			return
		}
		logger.Info("rule adjustment completed",
			zap.String("rule_id", stats.RuleID),
			zap.String("mode", s.configService.GetMode()))
	}
}

// callLLMForAnalysis 调用LLM分析规则是否为误报
func (s *RuleGenerationService) callLLMForAnalysis(ctx context.Context, rule *model.SigmaRule, stats repository.RuleTriggerStats, timeWindow string) (*FalsePositiveAnalysisResult, error) {
	config, err := s.llmConfigRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM config: %w", err)
	}

	apiKey, err := s.llmConfigRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	client := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmMaxRetries)

	alertSamples := s.buildAlertSamples(stats.Alerts)

	prompt := fmt.Sprintf(`You are a security analyst. This rule triggered %d alerts during %s. Determine whether the alerts are false positives.

## Rule
- Rule ID: %s
- Title: %s
- MITRE technique: %s
- Content:
%s

## Alert samples (first %d)
%s

Determine whether these alerts are false positives, give confidence from 0 to 1, explain the evidence, and propose precise rule changes when needed.

Return JSON only:
{
  "is_false_positive": true,
  "confidence": 0.9,
  "reason": "detailed rationale in Simplified Chinese",
  "rule_adjustments": {
    "rule_id": "%s",
    "action": "tighten",
    "reason": "adjustment rationale in Simplified Chinese",
    "add_conditions": ["new condition"],
    "exclude_patterns": ["excluded pattern"],
    "severity_change": ""
  }
}

Do not output Markdown or any text outside the JSON object.`,
		stats.AlertCount,
		timeWindow,
		rule.RuleID,
		rule.Title,
		rule.MitreID,
		rule.Content,
		len(stats.Alerts),
		alertSamples,
		rule.RuleID,
	)

	response, err := client.ChatCompletion(ctx, "", prompt, 0.7)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	cleanResponse := response
	if strings.Contains(cleanResponse, "```json") {
		cleanResponse = strings.ReplaceAll(cleanResponse, "```json", "")
		cleanResponse = strings.ReplaceAll(cleanResponse, "```", "")
		cleanResponse = strings.TrimSpace(cleanResponse)
	}

	var result FalsePositiveAnalysisResult
	if err := json.Unmarshal([]byte(cleanResponse), &result); err != nil {
		logger.Error("failed to parse LLM response", zap.Error(err), zap.String("response", cleanResponse))
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	result.RuleAdjustment.RuleID = rule.RuleID
	result.RuleAdjustment.Action = "tighten"

	return &result, nil
}

func (s *RuleGenerationService) buildAlertSamples(alerts []model.Alert) string {
	var samples []string
	for i, alert := range alerts {
		if i >= s.sampleSize {
			break
		}
		sample := fmt.Sprintf("Alert %d:\n- Host: %s\n- PID: %d\n- Command: %s\n- Severity: %s",
			i+1,
			alert.Hostname,
			alert.PID,
			truncateString(alert.CommandLine, 200),
			alert.Severity,
		)
		samples = append(samples, sample)
	}
	return strings.Join(samples, "\n\n")
}

// applyRuleAdjustment 紧收规则并发送通知
func (s *RuleGenerationService) applyRuleAdjustment(rule *model.SigmaRule, adjustment RuleAdjustment, stats repository.RuleTriggerStats) error {
	newContent, err := s.applyTightening(rule.Content, adjustment)
	if err != nil {
		return fmt.Errorf("failed to apply tightening: %w", err)
	}
	if strings.TrimSpace(newContent) == strings.TrimSpace(rule.Content) {
		return fmt.Errorf("tightening did not change rule content")
	}
	if err := validateTightenedRuleContent(newContent); err != nil {
		return fmt.Errorf("invalid tightened rule: %w", err)
	}

	if s.configService != nil && s.configService.GetMode() == "suggest" {
		return s.createRuleAdjustmentProposal(rule, newContent, adjustment, stats)
	}

	return s.applyRuleAdjustmentDirectly(rule, newContent, adjustment, stats)
}

func (s *RuleGenerationService) applyRuleAdjustmentDirectly(rule *model.SigmaRule, newContent string, adjustment RuleAdjustment, stats repository.RuleTriggerStats) error {
	rule.Content = newContent
	rule.Version = incrementVersion(rule.Version)
	rule.Status = "experimental"
	now := time.Now()
	rule.ActivatedAt = &now
	rule.UpdatedAt = now

	if severity := normalizeSeverityChange(adjustment.SeverityChange); severity != "" {
		rule.Severity = severity
	}

	if err := s.sigmaRuleRepo.Update(rule); err != nil {
		return fmt.Errorf("failed to update rule: %w", err)
	}

	if s.sigmaRuleSvc != nil {
		s.sigmaRuleSvc.broadcastRuleUpdate(rule.RuleID, rule.Status)
	}

	logger.Info("rule tightened and set to experimental",
		zap.String("rule_id", rule.RuleID),
		zap.String("version", rule.Version),
		zap.String("status", rule.Status))

	// 发送通知
	if config, err := s.configService.GetConfig(); err == nil && config.NotifyOnGeneration && s.notificationRepo != nil {
		metadataBytes, _ := json.Marshal(map[string]interface{}{
			"rule_id":     rule.RuleID,
			"mitre_id":    rule.MitreID,
			"action":      "tighten",
			"alert_count": stats.AlertCount,
			"time_window": stats.TimeWindow,
			"i18n_key":    "notifications.ruleAdjusted",
			"i18n_params": map[string]interface{}{
				"ruleTitle": rule.Title,
				"mitreId":   rule.MitreID,
				"version":   rule.Version,
			},
		})
		notification := &model.Notification{
			Title:    "AI规则更新通知",
			Content:  fmt.Sprintf("AI已自动更新规则: %s (MITRE: %s)，更新版本至 %s，减少误报", rule.Title, rule.MitreID, rule.Version),
			Severity: "info",
			Type:     "rule_generated",
			Link:     "/detection/rules?status=experimental",
			Metadata: datatypes.JSON(metadataBytes),
		}
		s.notificationRepo.Create(notification)
	}

	return nil
}

func (s *RuleGenerationService) createRuleAdjustmentProposal(rule *model.SigmaRule, newContent string, adjustment RuleAdjustment, stats repository.RuleTriggerStats) error {
	now := time.Now()
	proposalID := fmt.Sprintf("%s-ai-%s", rule.RuleID, uuid.NewString()[:8])
	severity := rule.Severity
	if normalized := normalizeSeverityChange(adjustment.SeverityChange); normalized != "" {
		severity = normalized
	}

	contextBytes, _ := json.Marshal(map[string]interface{}{
		"source_rule_id": rule.RuleID,
		"action":         adjustment.Action,
		"reason":         adjustment.Reason,
		"alert_count":    stats.AlertCount,
		"time_window":    stats.TimeWindow,
	})

	proposal := &model.SigmaRule{
		ID:                uuid.New(),
		RuleID:            proposalID,
		Title:             rule.Title,
		Description:       rule.Description,
		Content:           newContent,
		Status:            "pending",
		MitreID:           rule.MitreID,
		Severity:          severity,
		GeneratedBy:       "ai_generated",
		Version:           incrementVersion(rule.Version),
		CreatedAt:         now,
		UpdatedAt:         now,
		Source:            "ai_suggestion",
		AIGenerated:       true,
		ParentRuleID:      rule.RuleID,
		GenerationContext: string(contextBytes),
	}

	if err := s.sigmaRuleRepo.Create(proposal); err != nil {
		return fmt.Errorf("failed to create rule adjustment proposal: %w", err)
	}

	logger.Info("rule tightening proposal created",
		zap.String("rule_id", rule.RuleID),
		zap.String("proposal_rule_id", proposal.RuleID),
		zap.String("status", proposal.Status))

	if config, err := s.configService.GetConfig(); err == nil && config.NotifyOnGeneration && s.notificationRepo != nil {
		metadataBytes, _ := json.Marshal(map[string]interface{}{
			"rule_id":         proposal.RuleID,
			"parent_rule_id":  rule.RuleID,
			"mitre_id":        rule.MitreID,
			"action":          "tighten",
			"alert_count":     stats.AlertCount,
			"time_window":     stats.TimeWindow,
			"dispatch_status": "not_dispatched",
			"generation_mode": "suggest",
			"i18n_key":        "notifications.ruleAdjustmentSuggested",
			"i18n_params": map[string]interface{}{
				"ruleTitle": rule.Title,
				"mitreId":   rule.MitreID,
			},
		})
		notification := &model.Notification{
			Title:    "AI规则更新建议",
			Content:  fmt.Sprintf("AI已生成规则调整建议: %s (MITRE: %s)，请审核后下发", rule.Title, rule.MitreID),
			Severity: "info",
			Type:     "rule_generated",
			Link:     "/detection/rules?status=pending",
			Metadata: datatypes.JSON(metadataBytes),
		}
		s.notificationRepo.Create(notification)
	}

	return nil
}

func (s *RuleGenerationService) applyTightening(content string, adjustment RuleAdjustment) (string, error) {
	var sigmaRule map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &sigmaRule); err != nil {
		return content, nil
	}

	detection, ok := sigmaRule["detection"].(map[string]interface{})
	if !ok {
		detection = make(map[string]interface{})
		sigmaRule["detection"] = detection
	}

	condition, _ := detection["condition"].(string)

	for _, cond := range adjustment.AddConditions {
		if cond != "" {
			if condition == "" {
				condition = cond
			} else {
				condition = condition + " and " + cond
			}
		}
	}

	for _, pattern := range adjustment.ExcludePatterns {
		if pattern != "" {
			if condition == "" {
				condition = "not " + pattern
			} else {
				condition = condition + " and not " + pattern
			}
		}
	}

	detection["condition"] = condition

	newContent, err := yaml.Marshal(sigmaRule)
	if err != nil {
		return content, err
	}

	return string(newContent), nil
}

func falsePositiveConfidenceThreshold(conservatism float64) float64 {
	switch {
	case conservatism <= 0.2:
		return 0.90
	case conservatism <= 0.4:
		return 0.85
	case conservatism <= 0.6:
		return 0.80
	default:
		return 0.70
	}
}

func ruleTighteningCooldown(conservatism float64) time.Duration {
	switch {
	case conservatism <= 0.2:
		return 24 * time.Hour
	case conservatism <= 0.4:
		return 12 * time.Hour
	case conservatism <= 0.6:
		return 6 * time.Hour
	default:
		return time.Hour
	}
}

func shouldSkipRuleForCooldown(rule *model.SigmaRule, conservatism float64, now time.Time) bool {
	if rule == nil || rule.UpdatedAt.IsZero() {
		return false
	}
	return now.Sub(rule.UpdatedAt) < ruleTighteningCooldown(conservatism)
}

func validateTightenedRuleContent(content string) error {
	var sigmaRule map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &sigmaRule); err != nil {
		return err
	}

	detection, ok := sigmaRule["detection"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing detection block")
	}

	condition, _ := detection["condition"].(string)
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return fmt.Errorf("missing detection condition")
	}

	selectors := make(map[string]bool)
	for key := range detection {
		if key != "condition" {
			selectors[key] = true
		}
	}
	if len(selectors) == 0 {
		return fmt.Errorf("missing detection selectors")
	}

	return validateConditionReferences(condition, selectors)
}

func validateConditionReferences(condition string, selectors map[string]bool) error {
	tokens := tokenizeRuleCondition(condition)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		lower := strings.ToLower(token)
		switch lower {
		case "and", "or", "not", "(", ")":
			continue
		case "of":
			return fmt.Errorf("unexpected of in condition")
		case "all", "1":
			if i+2 >= len(tokens) || !strings.EqualFold(tokens[i+1], "of") {
				return fmt.Errorf("invalid selector group in condition")
			}
			if !selectorPatternExists(tokens[i+2], selectors) {
				return fmt.Errorf("condition references unknown selector pattern %q", tokens[i+2])
			}
			i += 2
		default:
			if strings.ContainsAny(token, "'\":|/\\") {
				return fmt.Errorf("condition contains unsupported fragment %q", token)
			}
			if strings.Contains(token, "*") {
				if !selectorPatternExists(token, selectors) {
					return fmt.Errorf("condition references unknown selector pattern %q", token)
				}
				continue
			}
			if !selectors[token] {
				return fmt.Errorf("condition references unknown selector %q", token)
			}
		}
	}
	return nil
}

func tokenizeRuleCondition(condition string) []string {
	condition = strings.ReplaceAll(condition, "(", " ( ")
	condition = strings.ReplaceAll(condition, ")", " ) ")
	return strings.Fields(condition)
}

func selectorPatternExists(pattern string, selectors map[string]bool) bool {
	if !strings.Contains(pattern, "*") {
		return selectors[pattern]
	}
	parts := strings.Split(pattern, "*")
	if len(parts) != 2 {
		return false
	}
	for selector := range selectors {
		if strings.HasPrefix(selector, parts[0]) && strings.HasSuffix(selector, parts[1]) {
			return true
		}
	}
	return false
}

// checkExperimentalRulesPromotion 检查实验性规则是否需要升级为active
func (s *RuleGenerationService) checkExperimentalRulesPromotion(ctx context.Context) {
	rules, err := s.sigmaRuleRepo.GetExperimentalRules()
	if err != nil {
		logger.Error("failed to get experimental rules", zap.Error(err))
		return
	}

	for _, rule := range rules {
		// Only promote rules that have been in experimental status for 7 days
		if rule.ActivatedAt != nil && time.Since(*rule.ActivatedAt) >= 7*24*time.Hour {
			if err := s.promoteRuleToActive(&rule); err != nil {
				logger.Error("failed to promote rule to active",
					zap.Error(err),
					zap.String("rule_id", rule.RuleID))
			}
		}
	}
}

// promoteRuleToActive 将规则升级为active状态
func (s *RuleGenerationService) promoteRuleToActive(rule *model.SigmaRule) error {
	if err := s.sigmaRuleRepo.UpdateStatus(rule.RuleID, "active"); err != nil {
		return err
	}

	// 更新统计
	s.configService.IncrementApprovedCount()

	// Broadcast rule update via server if available
	if s.serverClient != nil && s.sigmaRuleSvc != nil {
		s.sigmaRuleSvc.broadcastRuleUpdate(rule.RuleID, "active")
	}

	logger.Info("experimental rule promoted to active and broadcasted",
		zap.String("rule_id", rule.RuleID))

	// 发送通知
	if config, err := s.configService.GetConfig(); err == nil && config.NotifyOnApproval && s.notificationRepo != nil {
		metadataBytes, _ := json.Marshal(map[string]interface{}{
			"rule_id":  rule.RuleID,
			"mitre_id": rule.MitreID,
			"i18n_key": "notifications.ruleApproved",
			"i18n_params": map[string]interface{}{
				"ruleTitle": rule.Title,
			},
		})
		notification := &model.Notification{
			Title:    "规则审核通过",
			Content:  fmt.Sprintf("规则 %s 已审核通过并激活", rule.Title),
			Severity: "info",
			Type:     "system",
			Link:     "/detection/rules?status=active",
			Metadata: datatypes.JSON(metadataBytes),
		}
		s.notificationRepo.Create(notification)
	}

	return nil
}

// TriggerCheckResult 触发检查结果
type TriggerCheckResult struct {
	ShouldTrigger bool     `json:"should_trigger"`
	TriggerType   string   `json:"trigger_type"` // high_frequency, new_mitre, critical, manual
	MitreID       string   `json:"mitre_id"`
	AlertCount    int      `json:"alert_count"`
	Message       string   `json:"message"`
	SampleAlerts  []string `json:"sample_alerts"` // 样本告警ID列表
}

// CheckTriggers 检查是否满足触发条件
func (s *RuleGenerationService) CheckTriggers(mitreID string, severity string) (*TriggerCheckResult, error) {
	if !s.configService.IsEnabled() {
		return &TriggerCheckResult{ShouldTrigger: false, Message: "AI规则更新未启用"}, nil
	}

	thresholds := s.configService.GetThresholds()

	// 检查high_frequency触发
	if mitreID == "" {
		return &TriggerCheckResult{ShouldTrigger: false, Message: "MITRE ID为空"}, nil
	}

	count, err := s.alertRepo.CountByMitreIDInTimeRange(mitreID, thresholds.HighFrequencyHours)
	if err != nil {
		logger.Warn("failed to count alerts by mitre id", zap.String("mitre_id", mitreID), zap.Error(err))
		return nil, err
	}

	if count >= int64(thresholds.HighFrequencyCount) {
		// 获取样本告警ID
		alerts, _ := s.alertRepo.GetAlertIDsByMitreIDInTimeRange(mitreID, thresholds.HighFrequencyHours, 5)
		sampleAlerts := make([]string, len(alerts))
		for i, a := range alerts {
			sampleAlerts[i] = a.AlertID
		}

		return &TriggerCheckResult{
			ShouldTrigger: true,
			TriggerType:   "high_frequency",
			MitreID:       mitreID,
			AlertCount:    int(count),
			Message:       fmt.Sprintf("同一MITRE ID %d小时内告警次数(%d)超过阈值(%d)", thresholds.HighFrequencyHours, count, thresholds.HighFrequencyCount),
			SampleAlerts:  sampleAlerts,
		}, nil
	}

	return &TriggerCheckResult{ShouldTrigger: false, Message: "未满足触发条件"}, nil
}

// GenerateRuleRequest 生成规则请求
type GenerateRuleRequest struct {
	MitreID      string   `json:"mitre_id"`
	SampleAlerts []string `json:"sample_alerts"`
	Conservatism float64  `json:"conservatism"` // 0.0-1.0, 越低越保守
}

// GenerateRuleResponse 生成规则响应
type GenerateRuleResponse struct {
	RuleID      string `json:"rule_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	MitreID     string `json:"mitre_id"`
	Severity    string `json:"severity"`
	Content     string `json:"content"`
	Status      string `json:"status"`
}

// GenerateRule 生成规则
func (s *RuleGenerationService) GenerateRule(ctx context.Context, req *GenerateRuleRequest) (*GenerateRuleResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM client not initialized")
	}

	conservatism := req.Conservatism
	if conservatism == 0 {
		conservatism = s.configService.GetConservatism()
	}

	// 获取样本告警详情
	var alertDetails []string
	if len(req.SampleAlerts) > 0 {
		alerts, err := s.alertRepo.FindByAlertIDs(req.SampleAlerts)
		if err != nil {
			logger.Warn("failed to get sample alerts", zap.Error(err))
		} else {
			for _, a := range alerts {
				alertDetails = append(alertDetails, fmt.Sprintf(
					"Alert ID: %s | MITRE: %s | Severity: %s | Command line: %s | Host: %s",
					a.AlertID, a.MitreID, a.Severity, a.CommandLine, a.Hostname,
				))
			}
		}
	}

	// 构建LLM提示词
	prompt := s.buildRuleGenerationPrompt(req.MitreID, alertDetails, conservatism)

	// 调用LLM生成规则
	response, err := s.llmClient.ChatCompletion(ctx, "", prompt, 0.7)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 解析LLM响应
	rule, err := s.parseGeneratedRule(response, req.MitreID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated rule: %w", err)
	}
	s.applyGeneratedRuleMode(rule)

	// 保存规则
	if err := s.sigmaRuleRepo.Create(rule); err != nil {
		return nil, fmt.Errorf("failed to save rule: %w", err)
	}
	if rule.Status == "experimental" && s.sigmaRuleSvc != nil {
		s.sigmaRuleSvc.broadcastRuleUpdate(rule.RuleID, rule.Status)
	}

	// 更新统计
	s.configService.IncrementGeneratedCount()

	// 发送通知
	if config, err := s.configService.GetConfig(); err == nil && config.NotifyOnGeneration && s.notificationRepo != nil {
		metadataBytes, _ := json.Marshal(map[string]interface{}{
			"rule_id":  rule.RuleID,
			"mitre_id": rule.MitreID,
			"i18n_key": "notifications.ruleGenerated",
			"i18n_params": map[string]interface{}{
				"ruleTitle": rule.Title,
				"mitreId":   rule.MitreID,
				"status":    rule.Status,
			},
		})
		notification := &model.Notification{
			Title:    "AI规则生成通知",
			Content:  fmt.Sprintf("AI已自动生成新规则: %s (MITRE: %s)，当前状态为%s", rule.Title, rule.MitreID, rule.Status),
			Severity: "info",
			Type:     "rule_generated",
			Link:     fmt.Sprintf("/detection/rules?status=%s", rule.Status),
			Metadata: datatypes.JSON(metadataBytes),
		}
		s.notificationRepo.Create(notification)
	}

	return &GenerateRuleResponse{
		RuleID:      rule.RuleID,
		Title:       rule.Title,
		Description: rule.Description,
		MitreID:     rule.MitreID,
		Severity:    rule.Severity,
		Content:     rule.Content,
		Status:      rule.Status,
	}, nil
}

func (s *RuleGenerationService) applyGeneratedRuleMode(rule *model.SigmaRule) {
	if s.configService == nil || s.configService.GetMode() != "auto" {
		rule.Status = "pending"
		rule.ActivatedAt = nil
		return
	}

	now := time.Now()
	rule.Status = "experimental"
	rule.ActivatedAt = &now
	rule.UpdatedAt = now
}

// buildRuleGenerationPrompt 构建规则生成提示词
func (s *RuleGenerationService) buildRuleGenerationPrompt(mitreID string, alertDetails []string, conservatism float64) string {
	// 根据保守度调整提示词
	conditionDetail := "specific and precise detection conditions"
	if conservatism > 0.6 {
		conditionDetail = "broader conditions covering more detection patterns"
	} else if conservatism < 0.4 {
		conditionDetail = "strict precise conditions with a low false-positive rate"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`You are a security-rule expert. Generate one Sigma rule from this evidence.

## Detection target
- MITRE technique ID: %s
- Generation strategy: %.0f%% (lower is more conservative, higher is broader)

## Alert samples
`, mitreID, conservatism*100))

	if len(alertDetails) > 0 {
		sb.WriteString(strings.Join(alertDetails, "\n\n"))
	} else {
		sb.WriteString("(No alert sample is available. Generate a generic rule from the MITRE ID.)")
	}

	sb.WriteString(fmt.Sprintf(`

## Requirements
1. Return valid Sigma YAML.
2. Include title, id, status, description, level, logsource, and detection.
3. Generate a new UUID for id.
4. Set status to experimental.
5. Include the MITRE technique tag in attack.txxxx.xxx form.
6. Use %s.
7. Set level to high or critical according to evidence.

Return raw YAML only. Do not output Markdown or explanations.

`, conditionDetail))

	sb.WriteString(fmt.Sprintf(`

## Strategy
- Conservative (0-0.4): explicit malicious features and lower false positives.
- Balanced (0.4-0.6): balance precision and coverage.
- Broad (0.6-1.0): more possible threat patterns with higher false-positive risk.

Current value: %.0f%%

Generate the Sigma rule now.`, conservatism*100))

	return sb.String()
}

// parseGeneratedRule 解析LLM生成的规则
func (s *RuleGenerationService) parseGeneratedRule(response, mitreID string) (*model.SigmaRule, error) {
	// 清理响应，移除可能的markdown标记
	cleanResponse := response
	if strings.Contains(cleanResponse, "```yaml") {
		cleanResponse = strings.ReplaceAll(cleanResponse, "```yaml", "")
		cleanResponse = strings.ReplaceAll(cleanResponse, "```", "")
		cleanResponse = strings.TrimSpace(cleanResponse)
	}

	var rawRule struct {
		Title       string                 `yaml:"title"`
		ID          string                 `yaml:"id"`
		Status      string                 `yaml:"status"`
		Description string                 `yaml:"description"`
		Level       string                 `yaml:"level"`
		Tags        []string               `yaml:"tags"`
		Logsource   map[string]interface{} `yaml:"logsource"`
		Detection   map[string]interface{} `yaml:"detection"`
	}

	if err := yaml.Unmarshal([]byte(cleanResponse), &rawRule); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// 生成规则ID
	if rawRule.ID == "" {
		rawRule.ID = uuid.New().String()
	}

	// 如果没有title，使用默认值
	if rawRule.Title == "" {
		rawRule.Title = fmt.Sprintf("AI Generated Rule - %s", mitreID)
	}

	// 如果没有level，使用默认值
	if rawRule.Level == "" {
		rawRule.Level = "high"
	}

	// 提取MITRE ID
	if mitreID == "" {
		for _, tag := range rawRule.Tags {
			if strings.HasPrefix(strings.ToLower(tag), "attack.t") {
				rawMitre := strings.TrimPrefix(strings.ToLower(tag), "attack.")
				rawMitre = strings.ToUpper(rawMitre)
				if !strings.HasPrefix(rawMitre, "T") {
					rawMitre = "T" + rawMitre
				}
				mitreID = rawMitre
				break
			}
		}
	}

	// 构建规则内容
	ruleContent := map[string]interface{}{
		"title":       rawRule.Title,
		"id":          rawRule.ID,
		"status":      "experimental",
		"description": rawRule.Description,
		"level":       rawRule.Level,
		"tags":        rawRule.Tags,
	}
	if rawRule.Logsource != nil {
		ruleContent["logsource"] = rawRule.Logsource
	}
	if rawRule.Detection != nil {
		ruleContent["detection"] = rawRule.Detection
	}

	ruleYaml, _ := yaml.Marshal(ruleContent)

	rule := &model.SigmaRule{
		RuleID:      rawRule.ID,
		Title:       rawRule.Title,
		Description: rawRule.Description,
		Content:     string(ruleYaml),
		Status:      "pending", // 待审核
		MitreID:     mitreID,
		Severity:    rawRule.Level,
		GeneratedBy: "ai_generated",
		Version:     "1.0",
	}

	return rule, nil
}

func normalizeSeverityChange(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high", "medium", "low", "info":
		return strings.ToLower(strings.TrimSpace(severity))
	default:
		return ""
	}
}

// SubmitForApproval 提交规则进入审核队列
func (s *RuleGenerationService) SubmitForApproval(ruleID string) error {
	_, err := s.sigmaRuleRepo.FindByID(ruleID)
	if err != nil {
		return err
	}

	if s.configService.GetMode() != "auto" || s.configService.RequireApproval() {
		// suggest 模式始终保持 pending，不自动下发到 agent。
		return nil
	}

	// auto 模式且不要求审核时，规则进入实验性并下发。
	return s.sigmaRuleRepo.UpdateStatus(ruleID, "experimental")
}

// ActivateRule 激活规则
func (s *RuleGenerationService) ActivateRule(ruleID string) error {
	rule, err := s.sigmaRuleRepo.FindByID(ruleID)
	if err != nil {
		return err
	}

	// 更新状态为active
	if err := s.sigmaRuleRepo.UpdateStatus(ruleID, "active"); err != nil {
		return err
	}

	// 更新统计
	s.configService.IncrementApprovedCount()

	// 发送通知
	if config, err := s.configService.GetConfig(); err == nil && config.NotifyOnApproval && s.notificationRepo != nil {
		metadataBytes, _ := json.Marshal(map[string]interface{}{
			"rule_id":  ruleID,
			"mitre_id": rule.MitreID,
			"i18n_key": "notifications.ruleApproved",
			"i18n_params": map[string]interface{}{
				"ruleTitle": rule.Title,
			},
		})
		notification := &model.Notification{
			Title:    "规则审核通过",
			Content:  fmt.Sprintf("规则 %s 已审核通过并激活", rule.Title),
			Severity: "info",
			Type:     "system",
			Link:     "/detection/rules?status=active",
			Metadata: datatypes.JSON(metadataBytes),
		}
		s.notificationRepo.Create(notification)
	}

	logger.Info("rule activated",
		zap.String("rule_id", ruleID),
		zap.String("mitre_id", rule.MitreID))

	return nil
}

// GetRuleForReview 获取待审核规则列表
func (s *RuleGenerationService) GetRuleForReview() ([]model.SigmaRule, error) {
	rules, _, err := s.sigmaRuleRepo.List(1, 100, map[string]interface{}{
		"status":       "pending",
		"generated_by": "ai_generated",
	})
	return rules, err
}

// GetPendingRulesByGeneratedBy 获取AI生成的待审核规则
func (s *RuleGenerationService) GetPendingAIRules() ([]model.SigmaRule, error) {
	rules, err := s.sigmaRuleRepo.ListByGeneratedBy("ai_generated", "pending")
	return rules, err
}

// CountPendingByMitreID 统计同一MITRE ID的待审核规则数
func (s *RuleGenerationService) CountPendingByMitreID(mitreID string) (int64, error) {
	return s.sigmaRuleRepo.CountPendingByMitreID(mitreID)
}

// GenerateTestRule 测试规则生成（不保存到数据库）
func (s *RuleGenerationService) GenerateTestRule(ctx context.Context, req *GenerateRuleRequest) (*GenerateRuleResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM client not initialized")
	}

	conservatism := req.Conservatism
	if conservatism == 0 {
		conservatism = s.configService.GetConservatism()
	}

	// 获取样本告警详情
	var alertDetails []string
	if len(req.SampleAlerts) > 0 {
		alerts, err := s.alertRepo.FindByAlertIDs(req.SampleAlerts)
		if err != nil {
			logger.Warn("failed to get sample alerts", zap.Error(err))
		} else {
			for _, a := range alerts {
				alertDetails = append(alertDetails, fmt.Sprintf(
					"Alert ID: %s | MITRE: %s | Severity: %s | Command line: %s | Host: %s",
					a.AlertID, a.MitreID, a.Severity, a.CommandLine, a.Hostname,
				))
			}
		}
	}

	// 构建提示词
	prompt := s.buildRuleGenerationPrompt(req.MitreID, alertDetails, conservatism)

	// 调用LLM
	response, err := s.llmClient.ChatCompletion(ctx, "", prompt, 0.7)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 解析规则
	rule, err := s.parseGeneratedRule(response, req.MitreID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated rule: %w", err)
	}

	return &GenerateRuleResponse{
		RuleID:      rule.RuleID,
		Title:       rule.Title,
		Description: rule.Description,
		MitreID:     rule.MitreID,
		Severity:    rule.Severity,
		Content:     rule.Content,
		Status:      "test",
	}, nil
}

// FalsePositiveAnalysisResult LLM误报分析结果
type FalsePositiveAnalysisResult struct {
	IsFalsePositive bool           `json:"is_false_positive"`
	Confidence      float64        `json:"confidence"`
	Reason          string         `json:"reason"`
	RuleAdjustment  RuleAdjustment `json:"rule_adjustments"`
}

// RuleAdjustment 规则调整建议
type RuleAdjustment struct {
	RuleID          string   `json:"rule_id"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason"`
	AddConditions   []string `json:"add_conditions"`
	ExcludePatterns []string `json:"exclude_patterns"`
	SeverityChange  string   `json:"severity_change"`
}

// incrementVersion 增加版本号
func incrementVersion(version string) string {
	if version == "" {
		return "1.1"
	}

	var major, minor int
	fmt.Sscanf(version, "%d.%d", &major, &minor)
	minor++
	return fmt.Sprintf("%d.%d", major, minor)
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
