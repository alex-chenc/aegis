package handler

import (
	"net/http"
	"regexp"
	"strconv"

	"api-server/internal/model"
	"api-server/internal/checker"
	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type CommandAuditHandler struct {
	ruleRepo      *repository.CommandAuditRuleRepo
	sysConfigRepo *repository.SystemConfigRepo
	auditService  *service.ScriptAuditService
}

func NewCommandAuditHandler(
	ruleRepo *repository.CommandAuditRuleRepo,
	sysConfigRepo *repository.SystemConfigRepo,
	auditService *service.ScriptAuditService,
) *CommandAuditHandler {
	return &CommandAuditHandler{
		ruleRepo:      ruleRepo,
		sysConfigRepo: sysConfigRepo,
		auditService:  auditService,
	}
}

// ListRules GET /api/v1/settings/command-audit/rules
func (h *CommandAuditHandler) ListRules(c *gin.Context) {
	category := c.Query("category")
	severity := c.Query("severity")
	matchType := c.Query("match_type")
	status := c.Query("status")
	search := c.Query("search")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	rules, total, err := h.ruleRepo.List(category, severity, matchType, status, search, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to list rules"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"rules":     rules,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// CreateRule POST /api/v1/settings/command-audit/rules
func (h *CommandAuditHandler) CreateRule(c *gin.Context) {
	var rule model.CommandAuditRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}

	if rule.Name == "" || rule.Pattern == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name and pattern are required"})
		return
	}

	if rule.RuleType == "" {
		rule.RuleType = "hard_block"
	}
	if rule.MatchType == "" {
		rule.MatchType = "regex"
	}
	if rule.Category == "" {
		rule.Category = "system"
	}
	if rule.Severity == "" {
		rule.Severity = "high"
	}
	rule.IsPreset = false

	if err := h.ruleRepo.Create(&rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to create rule"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"code": 0, "message": "rule created", "data": rule})
}

// UpdateRule PUT /api/v1/settings/command-audit/rules/:id
func (h *CommandAuditHandler) UpdateRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	existing, err := h.ruleRepo.FindByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "rule not found"})
		return
	}

	var req model.CommandAuditRule
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}

	existing.Name = req.Name
	existing.Description = req.Description
	existing.RuleType = req.RuleType
	existing.MatchType = req.MatchType
	existing.Pattern = req.Pattern
	existing.Category = req.Category
	existing.Severity = req.Severity
	existing.AppliesTo = req.AppliesTo

	if err := h.ruleRepo.Update(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to update rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "rule updated", "data": existing})
}

// DeleteRule DELETE /api/v1/settings/command-audit/rules/:id
func (h *CommandAuditHandler) DeleteRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	if err := h.ruleRepo.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "rule not found or is preset"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "rule deleted"})
}

// ToggleRule PUT /api/v1/settings/command-audit/rules/:id/toggle
func (h *CommandAuditHandler) ToggleRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	rule, err := h.ruleRepo.Toggle(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "rule toggled", "data": rule})
}

type TestRuleRequest struct {
	MatchType   string `json:"match_type"`
	Pattern     string `json:"pattern"`
	TestContent string `json:"test_content"`
}

type TestMatchResult struct {
	LineNumber int    `json:"line_number"`
	Matched    string `json:"matched_text"`
}

// TestRule POST /api/v1/settings/command-audit/rules/test
func (h *CommandAuditHandler) TestRule(c *gin.Context) {
	var req TestRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}

	if req.Pattern == "" || req.TestContent == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "pattern and test_content are required"})
		return
	}

	// Validate regex pattern if match_type is regex
	matchType := req.MatchType
	if matchType == "" {
		matchType = "regex"
	}
	if matchType == "regex" {
		if _, err := regexp.Compile(req.Pattern); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid regex pattern: " + err.Error()})
			return
		}
	}

	tempChecker := checker.NewBlacklistChecker()
	rule := model.CommandAuditRule{
		MatchType: req.MatchType,
		Pattern:   req.Pattern,
		AppliesTo: model.StringArray{"all"},
		IsEnabled: true,
		RuleType:  "soft_warn",
	}
	if rule.MatchType == "" {
		rule.MatchType = "regex"
	}
	tempChecker.LoadRules([]model.CommandAuditRule{rule})

	result, err := tempChecker.Check(req.TestContent, "all")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "check failed: " + err.Error()})
		return
	}

	matches := make([]TestMatchResult, 0)
	for _, hit := range result.Hits {
		matches = append(matches, TestMatchResult{
			LineNumber: hit.LineNumber,
			Matched:    hit.MatchedText,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"matched": result.HasViolation,
			"matches": matches,
		},
	})
}

// GetSettings GET /api/v1/settings/command-audit/settings
func (h *CommandAuditHandler) GetSettings(c *gin.Context) {
	settings, err := h.sysConfigRepo.GetCommandAuditSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to get settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": settings})
}

// UpdateSettings PUT /api/v1/settings/command-audit/settings
func (h *CommandAuditHandler) UpdateSettings(c *gin.Context) {
	var settings model.CommandAuditSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}

	if err := h.sysConfigRepo.UpdateCommandAuditSettings(&settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to update settings"})
		return
	}

	if err := h.auditService.ReloadRules(c.Request.Context()); err != nil {
		logger.Warn("failed to reload rules after settings update", zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "settings updated", "data": settings})
}
