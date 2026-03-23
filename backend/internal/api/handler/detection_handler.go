package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/internal/service"
	"aegis-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type DetectionHandler struct {
	alertRepo        *repository.AlertRepository
	blockRepo        *repository.BlockRepository
	blockPolicyRepo  *repository.BlockPolicyRepository
	sigmaRuleRepo    *repository.SigmaRuleRepository
	toolCallRepo     *repository.ToolCallRepository
	alertService     *service.AlertService
	sigmaRuleService *service.SigmaRuleService
}

func NewDetectionHandler(
	alertRepo *repository.AlertRepository,
	blockRepo *repository.BlockRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
	toolCallRepo *repository.ToolCallRepository,
	alertService *service.AlertService,
	sigmaRuleService *service.SigmaRuleService,
) *DetectionHandler {
	return &DetectionHandler{
		alertRepo:        alertRepo,
		blockRepo:        blockRepo,
		blockPolicyRepo:  blockPolicyRepo,
		sigmaRuleRepo:    sigmaRuleRepo,
		toolCallRepo:     toolCallRepo,
		alertService:     alertService,
		sigmaRuleService: sigmaRuleService,
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
	record, err := h.alertService.ManualBlock(c.Param("id"))
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
		Enabled   *bool `json:"enabled"`
		AutoBlock *bool `json:"auto_block"`
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
