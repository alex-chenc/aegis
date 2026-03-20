package handler

import (
	"net/http"
	"strconv"

	"aegis-system/internal/repository"
	"aegis-system/internal/service"
	"aegis-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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
