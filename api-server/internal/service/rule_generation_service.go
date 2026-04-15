package service

import (
	"context"
	"fmt"
	"strings"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// RuleGenerationService AI规则生成服务
type RuleGenerationService struct {
	configService *AIRuleConfigService
	configRepo    *repository.AIRuleConfigRepository
	sigmaRuleRepo *repository.SigmaRuleRepository
	alertRepo     *repository.AlertRepository
	llmClient     *llm.LLMClient
}

// NewRuleGenerationService 创建规则生成服务
func NewRuleGenerationService(
	configService *AIRuleConfigService,
	configRepo *repository.AIRuleConfigRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
	alertRepo *repository.AlertRepository,
) *RuleGenerationService {
	return &RuleGenerationService{
		configService: configService,
		configRepo:    configRepo,
		sigmaRuleRepo: sigmaRuleRepo,
		alertRepo:    alertRepo,
	}
}

// InitLLMClient 初始化LLM客户端
func (s *RuleGenerationService) InitLLMClient(apiKey, baseURL, modelName string, timeout, maxRetries int) {
	s.llmClient = llm.NewLLMClient(apiKey, baseURL, modelName, timeout, maxRetries)
}

// TriggerCheckResult 触发检查结果
type TriggerCheckResult struct {
	ShouldTrigger bool     `json:"should_trigger"`
	TriggerType   string   `json:"trigger_type"`   // high_frequency, new_mitre, critical, manual
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
	MitreID       string   `json:"mitre_id"`
	SampleAlerts  []string `json:"sample_alerts"`
	Conservatism  float64  `json:"conservatism"` // 0.0-1.0, 越低越保守
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
					"告警ID: %s | MITRE: %s | 严重程度: %s | 命令行: %s | 主机: %s",
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

	// 保存规则
	if err := s.sigmaRuleRepo.Create(rule); err != nil {
		return nil, fmt.Errorf("failed to save rule: %w", err)
	}

	// 更新统计
	s.configService.IncrementGeneratedCount()

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

// buildRuleGenerationPrompt 构建规则生成提示词
func (s *RuleGenerationService) buildRuleGenerationPrompt(mitreID string, alertDetails []string, conservatism float64) string {
	// 根据保守度调整提示词
	conditionDetail := "具体的、精确的检测条件"
	if conservatism > 0.6 {
		conditionDetail = "包含更多检测模式的宽泛条件"
	} else if conservatism < 0.4 {
		conditionDetail = "严格的、误报率低的精确条件"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`你是一个安全规则专家。请根据以下信息生成一个Sigma规则。

## 检测目标
- MITRE技术ID: %s
- 生成策略: %.0f%% (越低越保守，越高越激进)

## 样本告警信息
`, mitreID, conservatism*100))

	if len(alertDetails) > 0 {
		sb.WriteString(strings.Join(alertDetails, "\n\n"))
	} else {
		sb.WriteString("(无样本告警，请基于MITRE ID生成通用规则)")
	}

	sb.WriteString(fmt.Sprintf(`

## 输出要求
1. 生成符合Sigma规则格式的YAML内容
2. 规则必须包含: title, id, status, description, level, logsource, detection
3. id字段使用新的UUID格式
4. status设为 experimental
5. 在tags中包含MITRE技术ID (格式: attack.txxxx.xxx)
6. detection部分需要包含具体的检测逻辑和条件，要求: %s
7. level设为 high 或 critical (根据告警严重程度)

## 输出格式
只输出YAML内容，不要有其他文字说明。绝对禁止使用markdown代码块标记，直接输出纯YAML字符串。

`, conditionDetail))

	sb.WriteString(fmt.Sprintf(`

## 生成策略说明
- 保守模式(0-0.4): 只检测明确的恶意行为特征，误报率低
- 平衡模式(0.4-0.6): 在准确率和覆盖率之间取得平衡
- 激进模式(0.6-1.0): 检测更多可能的威胁模式，覆盖率高但可能有更多误报

当前模式: %.0f%% 保守度

请生成Sigma规则:`, conservatism*100))

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

// SubmitForApproval 提交规则进入审核队列
func (s *RuleGenerationService) SubmitForApproval(ruleID string) error {
	_, err := s.sigmaRuleRepo.FindByID(ruleID)
	if err != nil {
		return err
	}

	if s.configService.RequireApproval() {
		// 需要审核，状态保持pending，24小时后自动变为experimental
		return nil
	}

	// 不需要审核，状态变为experimental
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

	logger.Info("rule activated",
		zap.String("rule_id", ruleID),
		zap.String("mitre_id", rule.MitreID))

	return nil
}

// GetRuleForReview 获取待审核规则列表
func (s *RuleGenerationService) GetRuleForReview() ([]model.SigmaRule, error) {
	rules, _, err := s.sigmaRuleRepo.List(1, 100, map[string]interface{}{
		"status":        "pending",
		"generated_by":  "ai_generated",
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
					"告警ID: %s | MITRE: %s | 严重程度: %s | 命令行: %s | 主机: %s",
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