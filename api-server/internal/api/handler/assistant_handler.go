package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/repository"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AssistantHandler 智能体处理器
type AssistantHandler struct {
	assistantService *assistant.Service
	approvalGate     *assistant.ApprovalGate
	policyService    *assistant.ToolPolicyService
	investigationSvc *assistant.HostAttackInvestigationService
	mcpService       *assistant.ExternalMCPSourceService
	logger           *zap.Logger
}

// NewAssistantHandler 创建智能体处理器
func NewAssistantHandler(
	assistantService *assistant.Service,
	approvalGate *assistant.ApprovalGate,
	policyService *assistant.ToolPolicyService,
	investigationSvc *assistant.HostAttackInvestigationService,
	mcpService *assistant.ExternalMCPSourceService,
	logger *zap.Logger,
) *AssistantHandler {
	return &AssistantHandler{
		assistantService: assistantService,
		approvalGate:     approvalGate,
		policyService:    policyService,
		investigationSvc: investigationSvc,
		mcpService:       mcpService,
		logger:           logger,
	}
}

// RegisterRoutes 注册路由
func (h *AssistantHandler) RegisterRoutes(group *gin.RouterGroup) {
	// Sessions
	group.GET("/sessions", h.ListSessions)
	group.POST("/sessions", h.CreateSession)
	group.GET("/sessions/:session_id", h.GetSession)
	group.GET("/sessions/:session_id/messages", h.GetMessages)
	group.POST("/sessions/:session_id/message", h.SendMessage)
	group.GET("/sessions/:session_id/stream", h.StreamSession)
	group.POST("/sessions/:session_id/cancel", h.CancelRun)
	group.GET("/sessions/:session_id/context-refs", h.ListContextRefs)
	group.GET("/sessions/:session_id/tool-calls", h.ListToolCalls)
	group.GET("/sessions/:session_id/approvals", h.ListApprovals)

	// Tools
	group.GET("/tools", h.ListTools)
	group.GET("/tool-approval-policy", h.GetToolApprovalPolicy)
	group.PUT("/tool-approval-policy", h.UpdateToolApprovalPolicy)
	group.PUT("/tools/:tool_name/whitelist", h.UpdateToolWhitelist)
	group.POST("/tools/whitelist/batch", h.BatchUpdateToolWhitelist)
	group.POST("/tools/whitelist/reset-defaults", h.ResetToolWhitelistDefaults)

	// Approvals
	group.GET("/approvals/:approval_id", h.GetApproval)
	group.POST("/approvals/:approval_id/approve", h.Approve)
	group.POST("/approvals/:approval_id/reject", h.Reject)

	// Investigations
	group.POST("/investigations/host-attack", h.CreateHostAttackInvestigation)
	group.GET("/investigations/:investigation_id", h.GetInvestigation)
	group.GET("/investigations/:investigation_id/evidence", h.ListInvestigationEvidence)

	// MCP Sources
	group.GET("/mcp-sources", h.ListMCPSources)
	group.POST("/mcp-sources", h.CreateMCPSource)
	group.GET("/mcp-sources/:source_id", h.GetMCPSource)
	group.PUT("/mcp-sources/:source_id", h.UpdateMCPSource)
	group.DELETE("/mcp-sources/:source_id", h.DeleteMCPSource)
	group.POST("/mcp-sources/:source_id/test", h.TestMCPSource)
	group.POST("/mcp-sources/:source_id/sync-schema", h.SyncMCPSchema)
}

// ListSessions 列出会话
func (h *AssistantHandler) ListSessions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	query := assistant.SessionQuery{
		Status:    c.Query("status"),
		CreatedBy: c.Query("created_by"),
		Keyword:   c.Query("keyword"),
		Page:      page,
		PageSize:  pageSize,
	}

	sessions, total, err := h.assistantService.ListSessions(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"sessions": sessions,
			"total":    total,
		},
	})
}

// CreateSession 创建会话
func (h *AssistantHandler) CreateSession(c *gin.Context) {
	var req assistant.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	operator := c.GetString("username")
	session, err := h.assistantService.CreateSession(c.Request.Context(), req, operator)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": session})
}

// GetSession 获取会话
func (h *AssistantHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	session, err := h.assistantService.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": session})
}

// GetMessages 获取消息历史
func (h *AssistantHandler) GetMessages(c *gin.Context) {
	sessionID := c.Param("session_id")

	messages, err := h.assistantService.GetMessages(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": messages})
}

// SendMessage 发送消息
func (h *AssistantHandler) SendMessage(c *gin.Context) {
	sessionID := c.Param("session_id")

	var req assistant.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	operator := c.GetString("username")
	handle, err := h.assistantService.SendMessage(c.Request.Context(), sessionID, req, operator)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": handle})
}

// StreamSession SSE 流式输出
func (h *AssistantHandler) StreamSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	writer := &sseWriter{gin: c}
	if err := h.assistantService.Stream(c.Request.Context(), sessionID, writer); err != nil {
		h.logger.Error("stream error", zap.Error(err))
	}
}

// CancelRun 取消运行
func (h *AssistantHandler) CancelRun(c *gin.Context) {
	sessionID := c.Param("session_id")
	operator := c.GetString("username")

	if err := h.assistantService.CancelRun(c.Request.Context(), sessionID, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "run cancelled"})
}

// ListContextRefs 列出上下文引用
func (h *AssistantHandler) ListContextRefs(c *gin.Context) {
	sessionID := c.Param("session_id")

	refs, err := h.assistantService.ListContextRefs(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": refs})
}

// ListToolCalls 列出工具调用
func (h *AssistantHandler) ListToolCalls(c *gin.Context) {
	sessionID := c.Param("session_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	calls, total, err := h.assistantService.ListToolCalls(c.Request.Context(), sessionID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"items": calls,
			"total": total,
		},
	})
}

// ListApprovals 列出审批
func (h *AssistantHandler) ListApprovals(c *gin.Context) {
	sessionID := c.Param("session_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	approvals, total, err := h.assistantService.ListApprovals(c.Request.Context(), sessionID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"items": approvals,
			"total": total,
		},
	})
}

// ListTools 列出工具
func (h *AssistantHandler) ListTools(c *gin.Context) {
	domain := c.Query("domain")
	riskLevel := c.Query("risk_level")
	keyword := c.Query("keyword")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	whitelistedStr := c.Query("whitelisted")
	var whitelisted *bool
	if whitelistedStr == "true" {
		t := true
		whitelisted = &t
	} else if whitelistedStr == "false" {
		f := false
		whitelisted = &f
	}

	query := repository.ToolPolicyQuery{
		Domain:      domain,
		RiskLevel:   riskLevel,
		Whitelisted: whitelisted,
		Keyword:     keyword,
		Page:        page,
		PageSize:    pageSize,
	}

	tools, total, err := h.policyService.ListTools(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"tools": tools,
			"total": total,
		},
	})
}

// GetToolApprovalPolicy 获取工具审批策略
func (h *AssistantHandler) GetToolApprovalPolicy(c *gin.Context) {
	mode, err := h.policyService.GetApprovalMode(c.Request.Context())
	if err != nil {
		mode = "whitelist"
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"mode": mode},
	})
}

// UpdateToolApprovalPolicy 更新工具审批策略
func (h *AssistantHandler) UpdateToolApprovalPolicy(c *gin.Context) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.policyService.SetApprovalMode(c.Request.Context(), req.Mode); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "policy updated"})
}

// UpdateToolWhitelist 更新工具白名单
func (h *AssistantHandler) UpdateToolWhitelist(c *gin.Context) {
	toolName := c.Param("tool_name")

	var req struct {
		Whitelisted bool `json:"whitelisted"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	operator := c.GetString("username")
	if err := h.policyService.UpdateWhitelist(c.Request.Context(), toolName, req.Whitelisted, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "whitelist updated"})
}

// BatchUpdateToolWhitelist 批量更新工具白名单
func (h *AssistantHandler) BatchUpdateToolWhitelist(c *gin.Context) {
	var req struct {
		Items []repository.WhitelistUpdateItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	operator := c.GetString("username")
	if err := h.policyService.BatchUpdateWhitelist(c.Request.Context(), req.Items, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "whitelist updated"})
}

// ResetToolWhitelistDefaults 重置工具白名单默认值
func (h *AssistantHandler) ResetToolWhitelistDefaults(c *gin.Context) {
	operator := c.GetString("username")
	if err := h.policyService.ResetDefaultWhitelist(c.Request.Context(), operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "whitelist reset to defaults"})
}

// GetApproval 获取审批详情
func (h *AssistantHandler) GetApproval(c *gin.Context) {
	approvalID := c.Param("approval_id")

	approval, err := h.approvalGate.GetApproval(c.Request.Context(), approvalID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": approval})
}

// Approve 批准
func (h *AssistantHandler) Approve(c *gin.Context) {
	approvalID := c.Param("approval_id")

	var req struct {
		Comment string `json:"comment"`
	}
	_ = c.ShouldBindJSON(&req)

	operator := c.GetString("username")
	result, err := h.approvalGate.Approve(c.Request.Context(), approvalID, operator, req.Comment)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// Reject 拒绝
func (h *AssistantHandler) Reject(c *gin.Context) {
	approvalID := c.Param("approval_id")

	var req struct {
		Comment string `json:"comment"`
	}
	_ = c.ShouldBindJSON(&req)

	operator := c.GetString("username")
	approval, err := h.approvalGate.Reject(c.Request.Context(), approvalID, operator, req.Comment)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": approval})
}

// CreateHostAttackInvestigation 创建主机攻击研判
func (h *AssistantHandler) CreateHostAttackInvestigation(c *gin.Context) {
	if h.investigationSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "investigation service not available"})
		return
	}

	var req struct {
		HostID   string   `json:"host_id"`
		AlertIDs []string `json:"alert_ids"`
		CVEIDs   []string `json:"cve_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	operator := c.GetString("username")
	input := model.HostAttackInvestigationInput{
		HostID:   req.HostID,
		AlertIDs: req.AlertIDs,
		CVEIDs:   req.CVEIDs,
	}

	result, err := h.investigationSvc.CreateInvestigation(c.Request.Context(), input, operator)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetInvestigation 获取研判报告
func (h *AssistantHandler) GetInvestigation(c *gin.Context) {
	if h.investigationSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "investigation service not available"})
		return
	}

	investigationID := c.Param("investigation_id")
	report, err := h.investigationSvc.GetInvestigation(c.Request.Context(), investigationID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "investigation not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": report})
}

// ListInvestigationEvidence 列出研判证据
func (h *AssistantHandler) ListInvestigationEvidence(c *gin.Context) {
	if h.investigationSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "investigation service not available"})
		return
	}

	investigationID := c.Param("investigation_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	sourceType := c.Query("source_type")

	query := repository.EvidenceQuery{
		SourceType: sourceType,
		Page:       page,
		PageSize:   pageSize,
	}

	evidence, total, err := h.investigationSvc.ListEvidence(c.Request.Context(), investigationID, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"items": evidence,
			"total": total,
		},
	})
}

// ListMCPSources 列出 MCP 数据源
func (h *AssistantHandler) ListMCPSources(c *gin.Context) {
	if h.mcpService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP service not available"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	query := repository.MCPSourceQuery{
		SourceType: c.Query("source_type"),
		Keyword:    c.Query("keyword"),
		Page:       page,
		PageSize:   pageSize,
	}

	sources, total, err := h.mcpService.ListSources(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"sources": sources,
			"total":   total,
		},
	})
}

// CreateMCPSource 创建 MCP 数据源
func (h *AssistantHandler) CreateMCPSource(c *gin.Context) {
	if h.mcpService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP service not available"})
		return
	}

	var source model.ExternalMCPSource
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	operator := c.GetString("username")
	source.CreatedBy = operator

	if err := h.mcpService.CreateSource(c.Request.Context(), &source); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": source})
}

// GetMCPSource 获取 MCP 数据源
func (h *AssistantHandler) GetMCPSource(c *gin.Context) {
	if h.mcpService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP service not available"})
		return
	}

	sourceID := c.Param("source_id")
	source, err := h.mcpService.GetSource(c.Request.Context(), sourceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": source})
}

// UpdateMCPSource 更新 MCP 数据源
func (h *AssistantHandler) UpdateMCPSource(c *gin.Context) {
	if h.mcpService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP service not available"})
		return
	}

	sourceID := c.Param("source_id")
	var source model.ExternalMCPSource
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	source.SourceID = sourceID
	operator := c.GetString("username")
	source.UpdatedBy = operator

	if err := h.mcpService.UpdateSource(c.Request.Context(), &source); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "updated"})
}

// DeleteMCPSource 删除 MCP 数据源
func (h *AssistantHandler) DeleteMCPSource(c *gin.Context) {
	if h.mcpService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP service not available"})
		return
	}

	sourceID := c.Param("source_id")
	if err := h.mcpService.DeleteSource(c.Request.Context(), sourceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "deleted"})
}

// TestMCPSource 测试 MCP 数据源连接
func (h *AssistantHandler) TestMCPSource(c *gin.Context) {
	if h.mcpService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP service not available"})
		return
	}

	sourceID := c.Param("source_id")
	result, err := h.mcpService.TestConnection(c.Request.Context(), sourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// SyncMCPSchema 同步 MCP Schema
func (h *AssistantHandler) SyncMCPSchema(c *gin.Context) {
	if h.mcpService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP service not available"})
		return
	}

	sourceID := c.Param("source_id")
	result, err := h.mcpService.SyncSchema(c.Request.Context(), sourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// SSE Writer
type sseWriter struct {
	gin *gin.Context
}

func (w *sseWriter) Write(event assistant.AssistantEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	fmt.Fprintf(w.gin.Writer, "data: %s\n\n", string(data))
	if f, ok := w.gin.Writer.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
