package handler

import (
	"net/http"
	"strconv"

	"github.com/alex-chenc/aegis/api-server/internal/assistant"
	"github.com/alex-chenc/aegis/api-server/internal/api/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AssistantHandler 智能体处理器
type AssistantHandler struct {
	assistantService *assistant.Service
	approvalGate     *assistant.ApprovalGate
	logger           *zap.Logger
}

// NewAssistantHandler 创建智能体处理器
func NewAssistantHandler(
	assistantService *assistant.Service,
	approvalGate *assistant.ApprovalGate,
	logger *zap.Logger,
) *AssistantHandler {
	return &AssistantHandler{
		assistantService: assistantService,
		approvalGate:     approvalGate,
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

	operator := middleware.GetUsername(c)
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

	operator := middleware.GetUsername(c)
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
	operator := middleware.GetUsername(c)

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
	// TODO: Implement tool listing from ToolCatalog
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"tools": []interface{}{}, "total": 0}})
}

// GetToolApprovalPolicy 获取工具审批策略
func (h *AssistantHandler) GetToolApprovalPolicy(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"mode": "whitelist"}})
}

// UpdateToolApprovalPolicy 更新工具审批策略
func (h *AssistantHandler) UpdateToolApprovalPolicy(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "policy updated"})
}

// UpdateToolWhitelist 更新工具白名单
func (h *AssistantHandler) UpdateToolWhitelist(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "whitelist updated"})
}

// BatchUpdateToolWhitelist 批量更新工具白名单
func (h *AssistantHandler) BatchUpdateToolWhitelist(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "whitelist updated"})
}

// ResetToolWhitelistDefaults 重置工具白名单默认值
func (h *AssistantHandler) ResetToolWhitelistDefaults(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "whitelist reset"})
}

// GetApproval 获取审批
func (h *AssistantHandler) GetApproval(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}

// Approve 批准
func (h *AssistantHandler) Approve(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "approved"})
}

// Reject 拒绝
func (h *AssistantHandler) Reject(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "rejected"})
}

// CreateHostAttackInvestigation 创建主机攻击研判
func (h *AssistantHandler) CreateHostAttackInvestigation(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}

// GetInvestigation 获取研判
func (h *AssistantHandler) GetInvestigation(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}

// ListInvestigationEvidence 列出研判证据
func (h *AssistantHandler) ListInvestigationEvidence(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": []interface{}{}, "total": 0}})
}

// ListMCPSources 列出 MCP 数据源
func (h *AssistantHandler) ListMCPSources(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"sources": []interface{}{}, "total": 0}})
}

// CreateMCPSource 创建 MCP 数据源
func (h *AssistantHandler) CreateMCPSource(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}

// GetMCPSource 获取 MCP 数据源
func (h *AssistantHandler) GetMCPSource(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}

// UpdateMCPSource 更新 MCP 数据源
func (h *AssistantHandler) UpdateMCPSource(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "updated"})
}

// DeleteMCPSource 删除 MCP 数据源
func (h *AssistantHandler) DeleteMCPSource(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "deleted"})
}

// TestMCPSource 测试 MCP 数据源
func (h *AssistantHandler) TestMCPSource(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"success": true}})
}

// SyncMCPSchema 同步 MCP Schema
func (h *AssistantHandler) SyncMCPSchema(c *gin.Context) {
	// TODO: Implement
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}

// SSE Writer
type sseWriter struct {
	gin *gin.Context
}

func (w *sseWriter) Write(event assistant.AssistantEvent) error {
	w.gin.Writer.Write([]byte("data: "))
	if err := w.gin.Writer.Encode(event); err != nil {
		return err
	}
	w.gin.Writer.Write([]byte("\n\n"))
	w.gin.Writer.Flush()
	return nil
}
