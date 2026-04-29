package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type DetectionHandler struct {
	alertRepo              *repository.AlertRepository
	blockRepo              *repository.BlockRepository
	blockPolicyRepo        *repository.BlockPolicyRepository
	sigmaRuleRepo          *repository.SigmaRuleRepository
	toolCallRepo           *repository.ToolCallRepository
	alertService           *service.AlertService
	sigmaRuleService       *service.SigmaRuleService
	sigmaRuleUploadService *service.SigmaRuleUploadService
	llmAggregationRepo     *repository.LLMAggregationRepository
	runtimeEventRepo       *repository.RuntimeEventRepository
	configRepo             *repository.ConfigRepository
	serverClient           *grpcclient.ServerClient
	wsService              *service.WebSocketService
	aiRuleConfigService    *service.AIRuleConfigService
	ruleGenService         *service.RuleGenerationService
}

func NewDetectionHandler(
	alertRepo *repository.AlertRepository,
	blockRepo *repository.BlockRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
	toolCallRepo *repository.ToolCallRepository,
	alertService *service.AlertService,
	sigmaRuleService *service.SigmaRuleService,
	sigmaRuleUploadService *service.SigmaRuleUploadService,
	llmAggregationRepo *repository.LLMAggregationRepository,
	runtimeEventRepo *repository.RuntimeEventRepository,
	configRepo *repository.ConfigRepository,
	serverClient *grpcclient.ServerClient,
	wsService *service.WebSocketService,
	aiRuleConfigService *service.AIRuleConfigService,
	ruleGenService *service.RuleGenerationService,
) *DetectionHandler {
	return &DetectionHandler{
		alertRepo:              alertRepo,
		blockRepo:              blockRepo,
		blockPolicyRepo:        blockPolicyRepo,
		sigmaRuleRepo:          sigmaRuleRepo,
		toolCallRepo:           toolCallRepo,
		alertService:           alertService,
		sigmaRuleService:       sigmaRuleService,
		sigmaRuleUploadService: sigmaRuleUploadService,
		llmAggregationRepo:     llmAggregationRepo,
		runtimeEventRepo:       runtimeEventRepo,
		configRepo:             configRepo,
		serverClient:           serverClient,
		wsService:              wsService,
		aiRuleConfigService:    aiRuleConfigService,
		ruleGenService:         ruleGenService,
	}
}

func normalizeDetectionMitreID(mitreID string) string {
	normalized := strings.ToUpper(strings.TrimSpace(mitreID))
	if normalized != "" && !strings.HasPrefix(normalized, "T") {
		normalized = "T" + normalized
	}
	return normalized
}

func (h *DetectionHandler) createPolicyForRule(rule *model.SigmaRule) error {
	mitreID := normalizeDetectionMitreID(rule.MitreID)
	if mitreID == "" {
		return fmt.Errorf("rule %s has no MITRE ID; cannot create one-to-one block policy", rule.RuleID)
	}
	if mitreID != rule.MitreID {
		rule.MitreID = mitreID
		if err := h.sigmaRuleRepo.Update(rule); err != nil {
			return fmt.Errorf("failed to normalize rule MITRE ID: %w", err)
		}
	}

	existingPolicy, _ := h.blockPolicyRepo.FindByMitreID(mitreID)
	if existingPolicy != nil {
		return nil
	}

	policy := &model.BlockPolicy{
		MitreID:     mitreID,
		MitreName:   rule.Title,
		Enabled:     true,
		AutoBlock:   false,
		AutoDispose: false,
		Action:      "kill_process",
	}
	if err := h.blockPolicyRepo.Create(policy); err != nil {
		return fmt.Errorf("failed to create one-to-one block policy for rule %s: %w", rule.RuleID, err)
	}
	return nil
}

func (h *DetectionHandler) reconcileRulePolicyBindings() (gin.H, error) {
	rules, _, err := h.sigmaRuleRepo.List(1, 100000, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list sigma rules: %w", err)
	}

	seen := make(map[string]string, len(rules))
	mitreIDs := make([]string, 0, len(rules))
	created := 0
	for i := range rules {
		rule := &rules[i]
		mitreID := normalizeDetectionMitreID(rule.MitreID)
		if mitreID == "" {
			return nil, fmt.Errorf("rule %s has no MITRE ID; policy count cannot be bound one-to-one", rule.RuleID)
		}
		if previousRuleID, ok := seen[mitreID]; ok {
			return nil, fmt.Errorf("rules %s and %s share MITRE ID %s; one-to-one policy binding requires unique MITRE IDs", previousRuleID, rule.RuleID, mitreID)
		}
		seen[mitreID] = rule.RuleID
		mitreIDs = append(mitreIDs, mitreID)

		existingPolicy, _ := h.blockPolicyRepo.FindByMitreID(mitreID)
		if existingPolicy == nil {
			if err := h.createPolicyForRule(rule); err != nil {
				return nil, err
			}
			created++
		} else if existingPolicy.MitreName == "" || existingPolicy.MitreName != rule.Title {
			if err := h.blockPolicyRepo.Update(mitreID, map[string]interface{}{"mitre_name": rule.Title}); err != nil {
				return nil, fmt.Errorf("failed to align policy title for %s: %w", mitreID, err)
			}
		}
	}

	deleted, err := h.blockPolicyRepo.DeleteExceptMitreIDs(mitreIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to delete orphan block policies: %w", err)
	}

	policies, err := h.blockPolicyRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list block policies: %w", err)
	}
	if len(policies) != len(rules) {
		return nil, fmt.Errorf("rule/policy binding mismatch: rules=%d policies=%d", len(rules), len(policies))
	}

	return gin.H{
		"created":        created,
		"deleted_orphan": deleted,
		"total_rules":    len(rules),
		"total_policies": len(policies),
	}, nil
}

func (h *DetectionHandler) ListAlerts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSizeValue := c.DefaultQuery("pageSize", c.DefaultQuery("page_size", "20"))
	pageSize, _ := strconv.Atoi(pageSizeValue)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	filters := make(map[string]interface{})
	if v := c.Query("host_id"); v != "" {
		filters["host_id"] = v
	}
	if hostnames := parseCSVQuery(c.Query("hostnames")); len(hostnames) > 0 {
		filters["hostnames"] = hostnames
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
	if startTime, ok := parseAlertTime(c.Query("start_time")); ok {
		filters["start_time"] = startTime
	}
	if endTime, ok := parseAlertTime(c.Query("end_time")); ok {
		filters["end_time"] = endTime
	}

	alerts, total, err := h.alertRepo.List(page, pageSize, filters)
	if err != nil {
		logger.Error("failed to list alerts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"data": alerts, "total": total}})
}

func parseCSVQuery(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func parseAlertTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func (h *DetectionHandler) GetAlert(c *gin.Context) {
	alert, err := h.alertRepo.FindByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "alert not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": alert})
}

func (h *DetectionHandler) GetProcessTree(c *gin.Context) {
	alertID := c.Param("id")
	alert, err := h.alertRepo.FindByID(alertID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "alert not found"})
		return
	}

	// Return cached process tree if available
	if alert.ProcessTree != "" {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"process_tree": alert.ProcessTree}})
		return
	}

	// Process tree requires agent connectivity via Server service
	// Currently not supported through ServerClient API
	c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "process tree retrieval not available through this endpoint"})
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

func (h *DetectionHandler) DeleteAlerts(c *gin.Context) {
	var body struct {
		AlertIDs []string `json:"alert_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "alert_ids is required"})
		return
	}

	if len(body.AlertIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no alerts selected"})
		return
	}

	if err := h.alertRepo.DeleteByIDs(body.AlertIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"deleted_count": len(body.AlertIDs)}})
}

func (h *DetectionHandler) ListBlockPolicies(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	query := c.Query("query")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	if _, err := h.reconcileRulePolicyBindings(); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	policies, total, err := h.blockPolicyRepo.ListPaginatedWithRuleTitle(page, pageSize, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"data":       policies,
			"total":      total,
			"page":       page,
			"page_size":  pageSize,
			"total_page": (int(total) + pageSize - 1) / pageSize,
		},
	})
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

	mitreID := c.Param("mitre_id")

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

	if err := h.blockPolicyRepo.Update(mitreID, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	policy, err := h.blockPolicyRepo.FindByMitreID(mitreID)
	if err == nil && policy != nil && h.wsService != nil {
		h.wsService.BroadcastPolicyUpdate(policy)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionHandler) SyncBlockPolicies(c *gin.Context) {
	result, err := h.reconcileRulePolicyBindings()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

func (h *DetectionHandler) DeleteBlockPolicy(c *gin.Context) {
	mitreID := c.Param("mitre_id")
	if mitreID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "mitre_id is required"})
		return
	}

	// Delete all alerts associated with this MITRE ID
	alertCount, err := h.alertRepo.DeleteByMitreID(mitreID)
	if err != nil {
		logger.Error("failed to delete alerts for mitre_id", zap.String("mitre_id", mitreID), zap.Error(err))
		// Continue anyway - we'll still try to delete the rule and policy
	} else {
		logger.Info("deleted alerts for block policy", zap.String("mitre_id", mitreID), zap.Int("count", alertCount))
	}

	// Delete all rules associated with this MITRE ID
	deletedRules, err := h.sigmaRuleRepo.DeleteByMitreID(mitreID)
	if err != nil {
		logger.Error("failed to delete rules for mitre_id", zap.String("mitre_id", mitreID), zap.Error(err))
		// Continue anyway - we'll still try to delete the policy
	} else {
		logger.Info("deleted rules for block policy", zap.String("mitre_id", mitreID), zap.Int64("count", deletedRules))
	}

	// Delete the block policy
	deleted, err := h.blockPolicyRepo.DeleteByMitreID(mitreID)
	if err != nil {
		logger.Error("failed to delete block policy", zap.String("mitre_id", mitreID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "block policy not found"})
		return
	}

	logger.Info("block policy deleted", zap.String("mitre_id", mitreID))

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "block policy and associated rules/alerts deleted successfully",
	})
}

func (h *DetectionHandler) NormalizeMitreIDs(c *gin.Context) {
	ctx := c.Request.Context()

	rulesUpdated, err := h.sigmaRuleRepo.NormalizeMitreIDs(ctx)
	if err != nil {
		logger.Error("failed to normalize rule mitre_ids", zap.Error(err))
	}

	policiesUpdated, err := h.blockPolicyRepo.NormalizeMitreIDs(ctx)
	if err != nil {
		logger.Error("failed to normalize block policy mitre_ids", zap.Error(err))
	}

	alertsUpdated, err := h.alertRepo.NormalizeMitreIDs(ctx)
	if err != nil {
		logger.Error("failed to normalize alert mitre_ids", zap.Error(err))
	}

	logger.Info("mitre_ids normalized",
		zap.Int("rules_updated", rulesUpdated),
		zap.Int("policies_updated", policiesUpdated),
		zap.Int("alerts_updated", alertsUpdated))

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"rules_updated":    rulesUpdated,
			"policies_updated": policiesUpdated,
			"alerts_updated":   alertsUpdated,
		},
	})
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
	if v := c.Query("query"); v != "" {
		filters["query"] = v
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
		Status        string   `json:"status" binding:"required"`
		TargetHostIDs []string `json:"target_host_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	if body.Status == "active" {
		// 如果提供了target_host_ids，使用精确下发
		if len(body.TargetHostIDs) > 0 && h.sigmaRuleUploadService != nil {
			if err := h.sigmaRuleUploadService.ApproveRule(c.Param("id"), body.TargetHostIDs); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
				return
			}
		} else {
			// 使用旧的广播方式
			if err := h.sigmaRuleService.ApproveRule(c.Param("id")); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
				return
			}
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
				rawMitre = strings.ToUpper(rawMitre)
				if !strings.HasPrefix(rawMitre, "T") {
					rawMitre = "T" + rawMitre
				}
				mitreID = rawMitre
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
	skipped := 0
	for _, rule := range rules {
		r := rule

		if r.MitreID != "" {
			upperMitreID := normalizeDetectionMitreID(r.MitreID)
			r.MitreID = upperMitreID

			exists, err := h.sigmaRuleRepo.ExistsByMitreID(upperMitreID)
			if err != nil {
				logger.Error("failed to check mitre_id existence", zap.String("mitre_id", upperMitreID), zap.Error(err))
			} else if exists {
				logger.Warn("skipping rule with duplicate mitre_id", zap.String("mitre_id", upperMitreID), zap.String("title", r.Title))
				skipped++
				continue
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("rule %s has no MITRE ID; every rule must map to exactly one block policy", r.RuleID)})
			return
		}

		if err := h.sigmaRuleRepo.Create(&r); err != nil {
			logger.Error("failed to create rule",
				zap.String("rule_id", rule.RuleID),
				zap.Error(err))
			continue
		}
		imported++

		if err := h.createPolicyForRule(&r); err != nil {
			_ = h.sigmaRuleRepo.DeleteByRuleID(r.RuleID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    len(rules),
		"imported": imported,
		"skipped":  skipped,
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

	startTs := startTime.UnixMilli()
	endTs := endTime.UnixMilli()
	events, err := h.runtimeEventRepo.FindUnaggregated(startTs, endTs, body.HostIDs)
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

	agg.EventCount = len(events)
	h.llmAggregationRepo.Update(agg)

	aiJudgedCount := 0
	autoDisposeCount := 0

	alerts, err := h.alertRepo.FindPendingByTimeRange(startTime, endTime, body.HostIDs)
	if err != nil {
		logger.Error("Failed to get alerts for LLM", zap.Error(err))
	}

	agg.AlertCount = len(alerts)
	h.llmAggregationRepo.Update(agg)

	logger.Info("AI降噪请求开始", zap.Time("start_time", startTime), zap.Time("end_time", endTime),
		zap.Int("event_count", len(events)), zap.Int("alert_count", len(alerts)))

	if len(alerts) > 0 {
		logger.Info("开始调用LLM进行告警分析", zap.Int("alert_count", len(alerts)))
		llmResponse, err := h.callLLMForAlerts(c.Request.Context(), alerts)
		if err != nil {
			logger.Error("LLM call failed", zap.Error(err))
			agg.LLMResponse = fmt.Sprintf("LLM调用失败: %s", err.Error())
		} else {
			agg.LLMResponse = llmResponse

			cleanResponse := extractJSONFromResponse(llmResponse)

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
					h.alertRepo.MarkAIJudged(alert.AlertID, cleanResponse)
				}
			}
		}
	} else {
		logger.Warn("AI降噪请求范围内没有待处理告警", zap.Time("start_time", startTime),
			zap.Time("end_time", endTime), zap.Int("alert_count", 0))
	}

	agg.EventCount = len(events)
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

// extractJSONFromResponse extracts clean JSON from LLM response that may contain markdown fences
func extractJSONFromResponse(response string) string {
	// Try regex extraction first for ```json ... ``` blocks
	re := regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")
	matches := re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: find first { and last }
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(response[start : end+1])
	}

	// Last resort: return original trimmed
	return strings.TrimSpace(response)
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

	logger.Info("创建LLM客户端", zap.String("base_url", config.BaseURL),
		zap.String("model", config.ModelName))
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

每条告警的摘要和处置建议必须是独立的，不要混在一起。只返回JSON，不要其他内容。绝对禁止使用markdown代码块标记，直接输出纯JSON字符串。`, len(alerts), strings.Join(alertSummaries, "\n\n"))

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

func (h *DetectionHandler) GenerateSigmaRule(c *gin.Context) {
	var req struct {
		Event    string `json:"event" binding:"required"`
		Method   string `json:"method"`
		MitreID  string `json:"mitre_id"`
		Severity string `json:"severity"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "event is required"})
		return
	}

	startTime := time.Now()

	config, err := h.configRepo.GetActive()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "LLM config not found"})
		return
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to decrypt API key"})
		return
	}

	client := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 120, 2)

	severity := req.Severity
	if severity == "" {
		severity = "medium"
	}

	prompt := fmt.Sprintf(`你是一个安全规则专家。请根据用户描述生成一个Sigma规则。

## 用户需求
- 检测事件: %s
- 检测方式: %s
- MITRE技术ID: %s
- 严重程度: %s

## 输出要求
1. 生成符合Sigma规则格式的YAML内容
2. 规则必须包含: title, id, status, description, level, logsource, detection
3. id字段使用uuid格式生成一个新的规则ID
4. status设为 experimental
5. 在tags中包含MITRE技术ID（如果有）
6. detection部分需要包含具体的检测逻辑和条件

## 输出格式
只输出YAML内容，不要有其他文字说明。

示例格式:
title: 规则标题
id: 生成的UUID
status: experimental
description: 规则描述
level: high
tags:
  - attack.t1059.004
logsource:
  category: process_creation
  product: linux
detection:
  selection:
    CommandLine|contains:
      - 'bash -i'
      - 'nc -e'
  condition: selection

请生成Sigma规则:`, req.Event, req.Method, req.MitreID, severity)

	response, err := client.ChatCompletion(c.Request.Context(), "", prompt, 0.7)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": fmt.Sprintf("LLM call failed: %v", err)})
		return
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": fmt.Sprintf("failed to parse LLM response: %v", err)})
		return
	}

	if rawRule.ID == "" {
		rawRule.ID = uuid.New().String()
	}

	mitreID := req.MitreID
	if mitreID == "" {
		for _, tag := range rawRule.Tags {
			if strings.HasPrefix(tag, "attack.t") || strings.HasPrefix(tag, "attack.T") {
				rawMitre := strings.TrimPrefix(tag, "attack.")
				rawMitre = strings.ToUpper(rawMitre)
				if !strings.HasPrefix(rawMitre, "T") {
					rawMitre = "T" + rawMitre
				}
				mitreID = rawMitre
				break
			}
		}
	}

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

	ruleYaml, _ := yaml.Marshal(ruleContent)

	rule := &model.SigmaRule{
		RuleID:      rawRule.ID,
		Title:       rawRule.Title,
		Description: rawRule.Description,
		Content:     string(ruleYaml),
		Status:      "experimental",
		MitreID:     mitreID,
		Severity:    rawRule.Level,
		GeneratedBy: "llm",
		Version:     "1.0",
	}

	if rule.Title == "" {
		rule.Title = req.Event
	}
	if rule.Severity == "" {
		rule.Severity = severity
	}

	if rule.MitreID != "" {
		upperMitreID := normalizeDetectionMitreID(rule.MitreID)
		rule.MitreID = upperMitreID

		exists, err := h.sigmaRuleRepo.ExistsByMitreID(upperMitreID)
		if err != nil {
			logger.Error("failed to check mitre_id existence", zap.String("mitre_id", upperMitreID), zap.Error(err))
		} else if exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": fmt.Sprintf("MITRE ID %s already exists, cannot create duplicate rule", upperMitreID),
			})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "MITRE ID is required; every rule must map to exactly one block policy",
		})
		return
	}

	if err := h.sigmaRuleRepo.Create(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": fmt.Sprintf("failed to save rule: %v", err)})
		return
	}

	if err := h.createPolicyForRule(rule); err != nil {
		_ = h.sigmaRuleRepo.DeleteByRuleID(rule.RuleID)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	duration := time.Since(startTime).Seconds()

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"rule_id":  rule.RuleID,
			"title":    rule.Title,
			"mitre_id": rule.MitreID,
			"severity": rule.Severity,
			"content":  rule.Content,
			"duration": int(duration),
		},
	})
}

func (h *DetectionHandler) CheckRulesBeforeDelete(c *gin.Context) {
	var req struct {
		RuleIDs []string `json:"rule_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "rule_ids is required"})
		return
	}

	rulesWithAlerts := make([]map[string]interface{}, 0)
	totalAlerts := 0

	for _, ruleID := range req.RuleIDs {
		alertCount, err := h.alertRepo.CountByRuleID(ruleID)
		if err != nil {
			logger.Error("failed to count alerts for rule", zap.String("rule_id", ruleID), zap.Error(err))
			continue
		}

		if alertCount > 0 {
			rule, err := h.sigmaRuleRepo.FindByRuleID(ruleID)
			title := ""
			if err == nil && rule != nil {
				title = rule.Title
			}
			rulesWithAlerts = append(rulesWithAlerts, map[string]interface{}{
				"rule_id":     ruleID,
				"title":       title,
				"alert_count": alertCount,
			})
			totalAlerts += alertCount
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"has_alerts":        len(rulesWithAlerts) > 0,
			"rules_with_alerts": rulesWithAlerts,
			"total_alerts":      totalAlerts,
		},
	})
}

func (h *DetectionHandler) DeleteRules(c *gin.Context) {
	var req struct {
		RuleIDs []string `json:"rule_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "rule_ids is required"})
		return
	}

	deletedRules := 0
	deletedAlerts := 0
	deletedPolicies := 0

	for _, ruleID := range req.RuleIDs {
		rule, err := h.sigmaRuleRepo.FindByRuleID(ruleID)
		if err != nil {
			continue
		}

		alertCount, _ := h.alertRepo.DeleteByRuleID(ruleID)
		deletedAlerts += alertCount

		if err := h.sigmaRuleRepo.DeleteByRuleID(ruleID); err != nil {
			logger.Error("failed to delete rule", zap.String("rule_id", ruleID), zap.Error(err))
			continue
		}
		deletedRules++

		if rule.MitreID != "" {
			policyDeleted, _ := h.blockPolicyRepo.DeleteByMitreID(rule.MitreID)
			if policyDeleted {
				deletedPolicies++
			}
		}
	}

	logger.Info("rules deleted",
		zap.Int("deleted_rules", deletedRules),
		zap.Int("deleted_alerts", deletedAlerts),
		zap.Int("deleted_policies", deletedPolicies))

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"deleted_rules":    deletedRules,
			"deleted_alerts":   deletedAlerts,
			"deleted_policies": deletedPolicies,
		},
	})
}

// GetAIConfig 获取AI规则配置
func (h *DetectionHandler) GetAIConfig(c *gin.Context) {
	config, err := h.aiRuleConfigService.GetConfig()
	if err != nil {
		logger.Error("failed to get AI config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": config})
}

// UpdateAIConfig 更新AI规则配置
func (h *DetectionHandler) UpdateAIConfig(c *gin.Context) {
	var req model.UpdateAIConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	config, err := h.aiRuleConfigService.UpdateConfig(&req)
	if err != nil {
		logger.Error("failed to update AI config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": config})
}

// GenerateTestRule 测试规则生成（不保存到数据库）
func (h *DetectionHandler) GenerateTestRule(c *gin.Context) {
	var req struct {
		MitreID      string   `json:"mitre_id" binding:"required"`
		SampleAlerts []string `json:"sample_alerts"`
		Conservatism float64  `json:"conservatism"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "mitre_id is required"})
		return
	}

	if req.Conservatism == 0 {
		req.Conservatism = 0.5
	}

	// 初始化LLM客户端
	config, err := h.configRepo.GetActive()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "LLM config not found"})
		return
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to decrypt API key"})
		return
	}

	h.ruleGenService.InitLLMClient(apiKey, config.BaseURL, config.ModelName, 120, 2)

	result, err := h.ruleGenService.GenerateTestRule(c.Request.Context(), &service.GenerateRuleRequest{
		MitreID:      req.MitreID,
		SampleAlerts: req.SampleAlerts,
		Conservatism: req.Conservatism,
	})

	if err != nil {
		logger.Error("failed to generate test rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": result})
}

// UploadRules 上传Sigma规则文件
func (h *DetectionHandler) UploadRules(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no file uploaded"})
		return
	}
	defer file.Close()

	// 检查文件大小
	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "file size exceeds maximum limit of 10 MB"})
		return
	}

	// 执行上传
	result, err := h.sigmaRuleUploadService.UploadRules(file, header.Filename, header.Size)
	if err != nil {
		logger.Error("failed to upload sigma rules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	if !result.Success {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": result.Error,
			"data":    result,
		})
		return
	}

	// 为每个成功上传的规则创建阻断策略（如果不存在）
	for _, parsedRule := range result.Rules {
		if parsedRule.MitreID != "" && parsedRule.Status != "skipped_duplicate" {
			// 获取完整的规则信息以检查是否需要创建阻断策略
			rule, err := h.sigmaRuleService.GetRuleByID(parsedRule.RuleID)
			if err == nil && rule != nil {
				if err := h.createPolicyForRule(rule); err != nil {
					_ = h.sigmaRuleRepo.DeleteByRuleID(rule.RuleID)
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
					return
				}
			}
		}
	}

	if _, err := h.reconcileRulePolicyBindings(); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}
