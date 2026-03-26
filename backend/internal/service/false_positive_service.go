package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aegis-system/internal/grpc_server"
	"aegis-system/internal/llm"
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	pb "aegis-system/pkg/api/v1"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type FalsePositiveAnalysisResult struct {
	IsFalsePositive bool           `json:"is_false_positive"`
	Confidence      float64        `json:"confidence"`
	Reason          string         `json:"reason"`
	RuleAdjustment  RuleAdjustment `json:"rule_adjustments"`
}

type RuleAdjustment struct {
	RuleID          string   `json:"rule_id"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason"`
	AddConditions   []string `json:"add_conditions"`
	ExcludePatterns []string `json:"exclude_patterns"`
	SeverityChange  string   `json:"severity_change"`
}

type RuleAdjustmentHistory struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	RuleID          string    `gorm:"type:varchar(128);index" json:"rule_id"`
	TriggerCount    int       `gorm:"not null" json:"trigger_count"`
	TimeWindow      string    `gorm:"type:varchar(10)" json:"time_window"`
	IsFalsePositive bool      `gorm:"not null" json:"is_false_positive"`
	LLMReason       string    `gorm:"type:text" json:"llm_reason"`
	OldContent      string    `gorm:"type:text" json:"old_content"`
	NewContent      string    `gorm:"type:text" json:"new_content"`
	AppliedAt       time.Time `gorm:"default:now()" json:"applied_at"`
}

func (RuleAdjustmentHistory) TableName() string {
	return "rule_adjustment_histories"
}

type FalsePositiveDetectionService struct {
	alertRepo     *repository.AlertRepository
	sigmaRuleRepo *repository.SigmaRuleRepository
	configRepo    *repository.ConfigRepository
	grpcServer    *grpc_server.GRPCServer
	sigmaRuleSvc  *SigmaRuleService
	llmTimeout    int
	llmMaxRetries int
	sampleSize    int

	thresholds map[string]int
	enabled    bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewFalsePositiveDetectionService(
	alertRepo *repository.AlertRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
	configRepo *repository.ConfigRepository,
	grpcServer *grpc_server.GRPCServer,
	sigmaRuleSvc *SigmaRuleService,
	llmTimeout int,
	llmMaxRetries int,
) *FalsePositiveDetectionService {
	return &FalsePositiveDetectionService{
		alertRepo:     alertRepo,
		sigmaRuleRepo: sigmaRuleRepo,
		configRepo:    configRepo,
		grpcServer:    grpcServer,
		sigmaRuleSvc:  sigmaRuleSvc,
		llmTimeout:    llmTimeout,
		llmMaxRetries: llmMaxRetries,
		sampleSize:    10,
		thresholds: map[string]int{
			"10m": 10,
			"30m": 30,
			"60m": 60,
		},
		enabled: true,
		stopCh:  make(chan struct{}),
	}
}

func (s *FalsePositiveDetectionService) SetThresholds(thresholds map[string]int) {
	s.thresholds = thresholds
}

func (s *FalsePositiveDetectionService) SetEnabled(enabled bool) {
	s.enabled = enabled
}

func (s *FalsePositiveDetectionService) SetGRPCServer(server *grpc_server.GRPCServer) {
	s.grpcServer = server
}

func (s *FalsePositiveDetectionService) Start(ctx context.Context) {
	if !s.enabled {
		logger.Info("false positive detection service is disabled")
		return
	}

	logger.Info("false positive detection service starting",
		zap.Any("thresholds", s.thresholds))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.checkTimeWindow("10m", s.thresholds["10m"])
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.checkTimeWindow("30m", s.thresholds["30m"])
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(60 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.checkTimeWindow("60m", s.thresholds["60m"])
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

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

	logger.Info("false positive detection service started")
}

func (s *FalsePositiveDetectionService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	logger.Info("false positive detection service stopped")
}

func (s *FalsePositiveDetectionService) checkTimeWindow(window string, threshold int) {
	if !s.enabled {
		return
	}

	ctx := context.Background()

	var startTime time.Time
	switch window {
	case "10m":
		startTime = time.Now().Add(-10 * time.Minute)
	case "30m":
		startTime = time.Now().Add(-30 * time.Minute)
	case "60m":
		startTime = time.Now().Add(-60 * time.Minute)
	default:
		return
	}

	stats, err := s.alertRepo.GetRuleTriggerStats(startTime, time.Now(), threshold, s.sampleSize)
	if err != nil {
		logger.Error("failed to get rule trigger stats", zap.Error(err), zap.String("window", window))
		return
	}

	logger.Info("rule trigger stats collected",
		zap.String("window", window),
		zap.Int("rules_over_threshold", len(stats)))

	for _, stat := range stats {
		if stat.AlertCount > threshold {
			go s.analyzeRule(ctx, stat, window)
		}
	}
}

func (s *FalsePositiveDetectionService) analyzeRule(ctx context.Context, stats repository.RuleTriggerStats, timeWindow string) {
	logger.Info("analyzing rule for false positive detection",
		zap.String("rule_id", stats.RuleID),
		zap.Int("alert_count", stats.AlertCount),
		zap.String("time_window", timeWindow))

	rule, err := s.sigmaRuleRepo.FindByRuleID(stats.RuleID)
	if err != nil {
		logger.Error("failed to find rule", zap.Error(err), zap.String("rule_id", stats.RuleID))
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

	if result.IsFalsePositive && result.Confidence >= 0.7 {
		if err := s.applyRuleAdjustment(rule, result.RuleAdjustment, stats); err != nil {
			logger.Error("failed to apply rule adjustment", zap.Error(err), zap.String("rule_id", stats.RuleID))
			return
		}
		logger.Info("rule adjusted successfully",
			zap.String("rule_id", stats.RuleID),
			zap.String("status", "experimental"))
	}
}

func (s *FalsePositiveDetectionService) callLLMForAnalysis(ctx context.Context, rule *model.SigmaRule, stats repository.RuleTriggerStats, timeWindow string) (*FalsePositiveAnalysisResult, error) {
	config, err := s.configRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM config: %w", err)
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	client := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmMaxRetries)

	alertSamples := s.buildAlertSamples(stats.Alerts)

	prompt := fmt.Sprintf(`你是安全分析师。以下规则在%s内触发了%d条告警，需要判断是否为误报。

## 规则信息
- 规则ID: %s
- 规则名称: %s
- MITRE技术: %s
- 当前规则内容:
%s

## 告警样本（前%d条）
%s

## 分析要求
请判断这些告警是否为误报，并提供以下信息：
1. 是否为误报？给出置信度(0-1)
2. 如果是误报，原因是什么？
3. 如何修改规则来减少误报？

## 返回格式（JSON）
{
  "is_false_positive": true/false,
  "confidence": 0.0-1.0,
  "reason": "判断原因的详细说明",
  "rule_adjustments": {
    "rule_id": "%s",
    "action": "tighten",
    "reason": "调整原因",
    "add_conditions": ["新条件1"],
    "exclude_patterns": ["排除模式1"],
    "severity_change": ""
  }
}

只返回JSON，不要其他内容。`,
		timeWindow,
		stats.AlertCount,
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

func (s *FalsePositiveDetectionService) buildAlertSamples(alerts []model.Alert) string {
	var samples []string
	for i, alert := range alerts {
		if i >= s.sampleSize {
			break
		}
		sample := fmt.Sprintf("告警%d:\n- 主机: %s\n- PID: %d\n- 命令: %s\n- 严重程度: %s",
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

func (s *FalsePositiveDetectionService) applyRuleAdjustment(rule *model.SigmaRule, adjustment RuleAdjustment, stats repository.RuleTriggerStats) error {
	newContent, err := s.applyTightening(rule.Content, adjustment)
	if err != nil {
		return fmt.Errorf("failed to apply tightening: %w", err)
	}

	rule.Content = newContent
	rule.Version = s.incrementVersion(rule.Version)
	rule.Status = "experimental"
	now := time.Now()
	rule.ActivatedAt = &now
	rule.UpdatedAt = now

	if adjustment.SeverityChange != "" {
		rule.Severity = adjustment.SeverityChange
	}

	if err := s.sigmaRuleRepo.Update(rule); err != nil {
		return fmt.Errorf("failed to update rule: %w", err)
	}

	logger.Info("rule tightened and set to experimental",
		zap.String("rule_id", rule.RuleID),
		zap.String("version", rule.Version),
		zap.String("status", rule.Status))

	return nil
}

func (s *FalsePositiveDetectionService) applyTightening(content string, adjustment RuleAdjustment) (string, error) {
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

func (s *FalsePositiveDetectionService) incrementVersion(version string) string {
	if version == "" {
		return "1.1"
	}

	var major, minor int
	fmt.Sscanf(version, "%d.%d", &major, &minor)
	minor++
	return fmt.Sprintf("%d.%d", major, minor)
}

func (s *FalsePositiveDetectionService) checkExperimentalRulesPromotion(ctx context.Context) {
	rules, err := s.sigmaRuleRepo.GetExperimentalRules()
	if err != nil {
		logger.Error("failed to get experimental rules", zap.Error(err))
		return
	}

	for _, rule := range rules {
		if rule.ActivatedAt != nil && time.Since(*rule.ActivatedAt) >= 1*time.Hour {
			if err := s.promoteRuleToActive(&rule); err != nil {
				logger.Error("failed to promote rule to active",
					zap.Error(err),
					zap.String("rule_id", rule.RuleID))
			}
		}
	}
}

func (s *FalsePositiveDetectionService) promoteRuleToActive(rule *model.SigmaRule) error {
	if err := s.sigmaRuleRepo.UpdateStatus(rule.RuleID, "active"); err != nil {
		return err
	}

	if s.grpcServer != nil {
		s.grpcServer.BroadcastRuleUpdate(&pb.RuleUpdate{
			RuleId:  rule.RuleID,
			Action:  "update",
			Content: rule.Content,
		})
	}

	logger.Info("experimental rule promoted to active and broadcasted",
		zap.String("rule_id", rule.RuleID))

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
