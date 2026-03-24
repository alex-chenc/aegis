package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aegis-system/internal/llm"
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/internal/service"
	"aegis-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type DetectionHandler struct {
	alertRepo          *repository.AlertRepository
	blockRepo          *repository.BlockRepository
	blockPolicyRepo    *repository.BlockPolicyRepository
	sigmaRuleRepo      *repository.SigmaRuleRepository
	toolCallRepo       *repository.ToolCallRepository
	alertService       *service.AlertService
	sigmaRuleService   *service.SigmaRuleService
	llmAggregationRepo *repository.LLMAggregationRepository
	configRepo         *repository.ConfigRepository
}

func NewDetectionHandler(
	alertRepo *repository.AlertRepository,
	blockRepo *repository.BlockRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
	toolCallRepo *repository.ToolCallRepository,
	alertService *service.AlertService,
	sigmaRuleService *service.SigmaRuleService,
	llmAggregationRepo *repository.LLMAggregationRepository,
	configRepo *repository.ConfigRepository,
) *DetectionHandler {
	return &DetectionHandler{
		alertRepo:          alertRepo,
		blockRepo:          blockRepo,
		blockPolicyRepo:    blockPolicyRepo,
		sigmaRuleRepo:      sigmaRuleRepo,
		toolCallRepo:       toolCallRepo,
		alertService:       alertService,
		sigmaRuleService:   sigmaRuleService,
		llmAggregationRepo: llmAggregationRepo,
		configRepo:         configRepo,
	}
}

func (h *DetectionHandler) ListAlerts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filters := make(map[string]interface{})
	if v := c.Query("host_id"); v != "" {
		filters["host_id"] = v
	}
	if v := c.Query("severity"); v != "" {
		filters["severity"] = v
	}
	if v := c.Query("mitre_id"); v != "" {
		filters["mitre_id"] = v
	}
	if v := c.Query("status"); v != "" {
		filters["status"] = v
	}
	if v := c.Query("judgment_source"); v != "" {
		filters["judgment_source"] = v
	}
	if v := c.Query("block_status"); v != "" {
		filters["block_status"] = v
	}

	alerts, total, err := h.alertRepo.List(page, pageSize, filters)
	if err != nil {
		logger.Error("failed to list alerts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"data": alerts, "total": total}})
}

func (h *DetectionHandler) GetAlert(c *gin.Context) {
	alert, err := h.alertRepo.FindByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "alert not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": alert})
}

func (h *DetectionHandler) ResolveAlert(c *gin.Context) {
	if err := h.alertRepo.Resolve(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionHandler) BlockAlert(c *gin.Context) {
	var body struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.Action = "kill_process"
	}

	record, err := h.alertService.ManualBlock(c.Param("id"), body.Action)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": record})
}

func (h *DetectionHandler) ListBlockPolicies(c *gin.Context) {
	policies, err := h.blockPolicyRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": policies})
}

func (h *DetectionHandler) UpdateBlockPolicy(c *gin.Context) {
	var body struct {
		Enabled     *bool   `json:"enabled"`
		AutoBlock   *bool   `json:"auto_block"`
		AutoDispose *bool   `json:"auto_dispose"`
		Action      *string `json:"action"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if body.Enabled != nil {
		updates["enabled"] = *body.Enabled
	}
	if body.AutoBlock != nil {
		updates["auto_block"] = *body.AutoBlock
	}
	if body.AutoDispose != nil {
		updates["auto_dispose"] = *body.AutoDispose
	}
	if body.Action != nil {
		updates["action"] = *body.Action
	}

	if err := h.blockPolicyRepo.Update(c.Param("mitre_id"), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionHandler) ListRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filters := make(map[string]interface{})
	if v := c.Query("status"); v != "" {
		filters["status"] = v
	}
	if v := c.Query("mitre_id"); v != "" {
		filters["mitre_id"] = v
	}

	rules, total, err := h.sigmaRuleRepo.List(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"data": rules, "total": total}})
}

func (h *DetectionHandler) GetRule(c *gin.Context) {
	rule, err := h.sigmaRuleRepo.FindByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": rule})
}

func (h *DetectionHandler) UpdateRuleStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	if body.Status == "active" {
		if err := h.sigmaRuleService.ApproveRule(c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
	} else if body.Status == "disabled" {
		if err := h.sigmaRuleService.DisableRule(c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionHandler) ListBlockRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filters := make(map[string]interface{})
	if v := c.Query("host_id"); v != "" {
		filters["host_id"] = v
	}

	records, total, err := h.blockRepo.List(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"data": records, "total": total}})
}

func (h *DetectionHandler) ListToolCalls(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filters := make(map[string]interface{})
	if v := c.Query("host_id"); v != "" {
		filters["host_id"] = v
	}

	calls, total, err := h.toolCallRepo.List(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"data": calls, "total": total}})
}

func (h *DetectionHandler) GetThreatStatistics(c *gin.Context) {
	todayAlerts, _ := h.alertRepo.GetTodayCount()
	todayBlocks, _ := h.blockRepo.GetTodayCount()
	affectedHosts, _ := h.alertRepo.GetAffectedHostCount()
	activeRules, _ := h.sigmaRuleRepo.GetActiveCount()

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{
		"today_alerts":   todayAlerts,
		"today_blocks":   todayBlocks,
		"affected_hosts": affectedHosts,
		"active_rules":   activeRules,
	}})
}

func (h *DetectionHandler) GetAlertTrend(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	trend, err := h.alertRepo.GetTrend(hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": trend})
}

func (h *DetectionHandler) GetAttackMatrix(c *gin.Context) {
	matrix := GetDefaultMITREMatrix()

	for i := range matrix.Tactics {
		for j := range matrix.Tactics[i].Techniques {
			tech := &matrix.Tactics[i].Techniques[j]
			count, _ := h.alertRepo.GetCountByMitreID(tech.ID)
			tech.AlertCount = count
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": matrix})
}

type MITRETactic struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Techniques []MITRETechnique `json:"techniques"`
}

type MITRETechnique struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlertCount int64  `json:"alert_count"`
}

type MITREMatrix struct {
	Tactics []MITRETactic `json:"tactics"`
}

func GetDefaultMITREMatrix() *MITREMatrix {
	return &MITREMatrix{
		Tactics: []MITRETactic{
			{
				ID:   "TA0043",
				Name: "侦察",
				Techniques: []MITRETechnique{
					{ID: "T1595", Name: "主动扫描"},
					{ID: "T1592", Name: "收集受害主机信息"},
				},
			},
			{
				ID:   "TA0042",
				Name: "资源开发",
				Techniques: []MITRETechnique{
					{ID: "T1587", Name: "开发能力"},
					{ID: "T1588", Name: "获取能力"},
				},
			},
			{
				ID:   "TA0001",
				Name: "初始访问",
				Techniques: []MITRETechnique{
					{ID: "T1190", Name: "利用面向公网的应用"},
					{ID: "T1110", Name: "暴力破解"},
				},
			},
			{
				ID:   "TA0002",
				Name: "执行",
				Techniques: []MITRETechnique{
					{ID: "T1059.004", Name: "Unix Shell"},
					{ID: "T1059.001", Name: "PowerShell"},
					{ID: "T1059.003", Name: "Windows Command Shell"},
				},
			},
			{
				ID:   "TA0003",
				Name: "持久化",
				Techniques: []MITRETechnique{
					{ID: "T1053.003", Name: "Cron"},
					{ID: "T1543.002", Name: "Systemd Service"},
				},
			},
			{
				ID:   "TA0004",
				Name: "提权",
				Techniques: []MITRETechnique{
					{ID: "T1068", Name: "漏洞提权"},
					{ID: "T1548.001", Name: "Setuid和Setgid"},
					{ID: "T1548.003", Name: "Sudo和Sudo缓存"},
				},
			},
			{
				ID:   "TA0005",
				Name: "防御规避",
				Techniques: []MITRETechnique{
					{ID: "T1070.002", Name: "清除日志"},
					{ID: "T1070.004", Name: "文件删除"},
					{ID: "T1222.002", Name: "文件权限修改"},
				},
			},
			{
				ID:   "TA0006",
				Name: "凭据访问",
				Techniques: []MITRETechnique{
					{ID: "T1003.008", Name: "/etc/passwd和/etc/shadow"},
					{ID: "T1003.001", Name: "LSASS内存"},
				},
			},
			{
				ID:   "TA0007",
				Name: "发现",
				Techniques: []MITRETechnique{
					{ID: "T1046", Name: "网络服务发现"},
					{ID: "T1082", Name: "系统信息发现"},
				},
			},
			{
				ID:   "TA0008",
				Name: "横向移动",
				Techniques: []MITRETechnique{
					{ID: "T1021.004", Name: "SSH"},
					{ID: "T1021.002", Name: "SMB"},
				},
			},
			{
				ID:   "TA0009",
				Name: "收集",
				Techniques: []MITRETechnique{
					{ID: "T1005", Name: "本地系统数据"},
					{ID: "T1113", Name: "屏幕截图"},
				},
			},
			{
				ID:   "TA0011",
				Name: "命令控制",
				Techniques: []MITRETechnique{
					{ID: "T1573", Name: "加密通道"},
					{ID: "T1572", Name: "协议隧道"},
				},
			},
			{
				ID:   "TA0010",
				Name: "数据渗出",
				Techniques: []MITRETechnique{
					{ID: "T1041", Name: "C2通道渗出"},
					{ID: "T1048", Name: "替代协议渗出"},
				},
			},
			{
				ID:   "TA0040",
				Name: "影响",
				Techniques: []MITRETechnique{
					{ID: "T1486", Name: "数据加密"},
					{ID: "T1490", Name: "抑制系统恢复"},
					{ID: "T1489", Name: "服务停止"},
				},
			},
		},
	}
}

func (h *DetectionHandler) ImportRules(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	rules := make([]model.SigmaRule, 0)

	for {
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

		if err := decoder.Decode(&rawRule); err == io.EOF {
			break
		} else if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse yaml: %v", err)})
			return
		}

		if rawRule.ID == "" {
			if rawRule.Title == "" && rawRule.Description == "" && rawRule.Level == "" && len(rawRule.Tags) == 0 {
				continue
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "rule missing id field"})
			return
		}

		mitreID := ""
		for _, tag := range rawRule.Tags {
			if strings.HasPrefix(tag, "attack.t") || strings.HasPrefix(tag, "attack.T") {
				rawMitre := strings.TrimPrefix(tag, "attack.")
				rawMitre = strings.TrimPrefix(rawMitre, "t")
				rawMitre = strings.TrimPrefix(rawMitre, "T")
				mitreID = "T" + rawMitre
				break
			}
		}

		// Marshal the complete rule back to YAML to preserve structure
		ruleContent := map[string]interface{}{
			"title":       rawRule.Title,
			"id":          rawRule.ID,
			"status":      rawRule.Status,
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

		ruleYaml, err := yaml.Marshal(ruleContent)
		if err != nil {
			logger.Warn("failed to marshal rule yaml", zap.Error(err))
			ruleYaml = []byte{}
		}

		rule := model.SigmaRule{
			RuleID:      rawRule.ID,
			Title:       rawRule.Title,
			Description: rawRule.Description,
			Content:     string(ruleYaml),
			Status:      "experimental",
			MitreID:     mitreID,
			Severity:    rawRule.Level,
			GeneratedBy: "import",
			Version:     "1.0",
		}
		rules = append(rules, rule)
	}

	if len(rules) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid rules found in file"})
		return
	}

	imported := 0
	for _, rule := range rules {
		r := rule
		if err := h.sigmaRuleRepo.Create(&r); err != nil {
			logger.Error("failed to create rule",
				zap.String("rule_id", rule.RuleID),
				zap.Error(err))
			continue
		}
		imported++
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    len(rules),
		"imported": imported,
	})
}

func (h *DetectionHandler) StartLLMAggregation(c *gin.Context) {
	var body struct {
		StartTime   string   `json:"start_time" binding:"required"`
		EndTime     string   `json:"end_time" binding:"required"`
		HostIDs     []string `json:"host_ids"`
		AutoDispose bool     `json:"auto_dispose"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	startTime, err := time.Parse(time.RFC3339, body.StartTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid start_time format, use RFC3339"})
		return
	}

	endTime, err := time.Parse(time.RFC3339, body.EndTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid end_time format, use RFC3339"})
		return
	}

	if endTime.Sub(startTime) > 24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "time range exceeds maximum of 24 hours"})
		return
	}

	if endTime.Before(startTime) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "end_time must be after start_time"})
		return
	}

	agg := &model.LLMAggregation{
		AggregationID: "AGG-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		StartTime:     startTime,
		EndTime:       endTime,
		HostIDs:       body.HostIDs,
		Status:        "processing",
	}

	if err := h.llmAggregationRepo.Create(agg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	alerts, _, err := h.alertRepo.List(1, 100, map[string]interface{}{
		"status": "pending",
	})
	if err != nil {
		h.llmAggregationRepo.UpdateStatus(agg.ID, "failed", err.Error())
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data": gin.H{
				"aggregation_id": agg.AggregationID,
				"status":         "failed",
				"error":          err.Error(),
			},
		})
		return
	}

	aiJudgedCount := 0
	autoDisposeCount := 0

	if len(alerts) > 0 {
		llmResponse, err := h.callLLMForAlerts(c.Request.Context(), alerts)
		if err != nil {
			logger.Error("LLM call failed", zap.Error(err))
			agg.LLMResponse = fmt.Sprintf("LLM调用失败: %s", err.Error())
		} else {
			agg.LLMResponse = llmResponse

			// 清理LLM响应中的markdown标记
			cleanResponse := llmResponse
			if strings.Contains(cleanResponse, "```json") {
				cleanResponse = strings.ReplaceAll(cleanResponse, "```json", "")
				cleanResponse = strings.ReplaceAll(cleanResponse, "```", "")
				cleanResponse = strings.TrimSpace(cleanResponse)
			}

			var result struct {
				Alerts []struct {
					AlertID        string `json:"alert_id"`
					IsThreat       bool   `json:"is_threat"`
					LLMSummary     string `json:"llm_summary"`
					Recommendation string `json:"recommendation"`
				} `json:"alerts"`
			}

			if err := json.Unmarshal([]byte(cleanResponse), &result); err == nil {
				for _, alertResult := range result.Alerts {
					llmSummary := alertResult.LLMSummary
					if alertResult.Recommendation != "" {
						llmSummary += "\n\n处置建议：\n- " + alertResult.Recommendation
					}

					if alertResult.IsThreat {
						aiJudgedCount++
						h.alertRepo.MarkAIJudged(alertResult.AlertID, llmSummary)
					} else {
						fpSummary := fmt.Sprintf("AI判定为误报：%s", alertResult.LLMSummary)
						h.alertRepo.UpdateLLMSummary(alertResult.AlertID, fpSummary)
					}
				}
			} else {
				logger.Error("Failed to parse LLM response JSON", zap.Error(err), zap.String("response", cleanResponse))
				for _, alert := range alerts {
					aiJudgedCount++
					h.alertRepo.MarkAIJudged(alert.AlertID, llmResponse)
				}
			}
		}
	}

	agg.EventCount = len(alerts)
	agg.AlertCount = len(alerts)
	agg.AIJudgedCount = aiJudgedCount
	agg.AutoDisposeCount = autoDisposeCount
	agg.Status = "completed"
	now := time.Now()
	agg.CompletedAt = &now
	h.llmAggregationRepo.Update(agg)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"aggregation_id":  agg.AggregationID,
			"status":          "completed",
			"event_count":     agg.EventCount,
			"alert_count":     agg.AlertCount,
			"ai_judged_count": aiJudgedCount,
			"llm_response":    agg.LLMResponse,
		},
	})
}

func (h *DetectionHandler) callLLMForAlerts(ctx context.Context, alerts []model.Alert) (string, error) {
	config, err := h.configRepo.GetActive()
	if err != nil {
		return "", fmt.Errorf("failed to get LLM config: %w", err)
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt API key: %w", err)
	}

	client := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 60, 2)

	var alertSummaries []string
	for i, a := range alerts {
		if i >= 10 {
			break
		}
		alertSummaries = append(alertSummaries, fmt.Sprintf("告警ID: %s | MITRE: %s | 严重程度: %s\n描述: %s\n主机: %s | PID: %d",
			a.AlertID, a.MitreID, a.Severity, a.Description, a.Hostname, a.PID))
	}

	prompt := fmt.Sprintf(`你是安全分析师。请分析以下%d条待处理告警，为每条告警判断是否为真实威胁，并生成独立的摘要和处置建议。

告警列表：
%s

请用JSON格式返回分析结果，为每条告警提供独立分析：
{
  "alerts": [
    {
      "alert_id": "告警ID",
      "is_threat": true/false,
      "llm_summary": "针对这条告警的安全分析摘要",
      "recommendation": "针对这条告警的具体处置建议"
    }
  ]
}

每条告警的摘要和处置建议必须是独立的，不要混在一起。只返回JSON，不要其他内容。`, len(alerts), strings.Join(alertSummaries, "\n\n"))

	response, err := client.ChatCompletion(ctx, "", prompt, 0.7)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return response, nil
}

func (h *DetectionHandler) GetLLMAggregationStatus(c *gin.Context) {
	agg, err := h.llmAggregationRepo.FindByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "aggregation not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    agg,
	})
}
