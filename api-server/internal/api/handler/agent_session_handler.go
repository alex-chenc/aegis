package handler

import (
	"net/http"
	"strconv"
	"strings"

	"api-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AgentSessionHandler struct {
	service *service.AgentSessionService
	logger  *zap.Logger
}

func NewAgentSessionHandler(svc *service.AgentSessionService, logger *zap.Logger) *AgentSessionHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentSessionHandler{service: svc, logger: logger}
}

func (h *AgentSessionHandler) RegisterRoutes(api *gin.RouterGroup, read gin.HandlerFunc, content gin.HandlerFunc, analysis gin.HandlerFunc) {
	register := func(group *gin.RouterGroup) {
		if read != nil {
			group.Use(read)
		}
		group.GET("", h.List)
		group.GET("/:id", h.Detail)
		group.GET("/:id/rule-hits", h.RuleHits)
		group.GET("/:id/ai-analysis", h.GetAIAnalysis)
		if content != nil {
			group.GET("/:id/items", content, h.Items)
		} else {
			group.GET("/:id/items", h.Items)
		}
		if analysis != nil {
			group.POST("/:id/ai-analysis", analysis, h.AIAnalysis)
		} else {
			group.POST("/:id/ai-analysis", h.AIAnalysis)
		}
	}
	awareness := api.Group("/agent-guard/session-awareness")
	if read != nil {
		awareness.Use(read)
	}
	awareness.POST("/agents/:host_id/collect", h.Collect)
	awareness.GET("/rules", h.Rules)
	register(awareness.Group("/sessions"))
	// Kept as a short compatibility alias for early V6.3 clients.
	group := api.Group("/agent-sessions")
	if read != nil {
		group.Use(read)
	}
	group.POST("/agents/:host_id/collect", h.Collect)
	// Preserve the original short alias while also exposing the explicit
	// /agent-sessions/sessions hierarchy for clients that mirror the canonical
	// route.
	group.GET("", h.List)
	group.GET("/:id", h.Detail)
	group.GET("/:id/rule-hits", h.RuleHits)
	group.GET("/:id/ai-analysis", h.GetAIAnalysis)
	if content != nil {
		group.GET("/:id/items", content, h.Items)
	} else {
		group.GET("/:id/items", h.Items)
	}
	if analysis != nil {
		group.POST("/:id/ai-analysis", analysis, h.AIAnalysis)
	} else {
		group.POST("/:id/ai-analysis", h.AIAnalysis)
	}
	sessions := group.Group("/sessions")
	sessions.GET("", h.List)
	sessions.GET("/:id", h.Detail)
	sessions.GET("/:id/rule-hits", h.RuleHits)
	sessions.GET("/:id/ai-analysis", h.GetAIAnalysis)
	if content != nil {
		sessions.GET("/:id/items", content, h.Items)
	} else {
		sessions.GET("/:id/items", h.Items)
	}
	if analysis != nil {
		sessions.POST("/:id/ai-analysis", analysis, h.AIAnalysis)
	} else {
		sessions.POST("/:id/ai-analysis", h.AIAnalysis)
	}
}

func (h *AgentSessionHandler) Collect(c *gin.Context) {
	hostID, err := uuid.Parse(strings.TrimSpace(c.Param("host_id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host_id"})
		return
	}
	agentType := strings.TrimSpace(c.Query("agent_type"))
	if agentType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_type is required"})
		return
	}
	result, err := h.service.RequestCollection(c.Request.Context(), hostID, agentType)
	if err != nil {
		h.logger.Warn("agent_session_collection_request_failed", zap.String("host_id", hostID.String()), zap.String("agent_type", agentType), zap.Error(err))
		errorCode := "AGENT_SESSION_COLLECTION_FAILED"
		message := err.Error()
		if strings.Contains(strings.ToLower(message), "not connected") {
			errorCode = "AGENT_NOT_CONNECTED"
			message = "agent is not connected"
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error_code": errorCode, "message": message})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": result})
}

func (h *AgentSessionHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	var hostID *uuid.UUID
	if value := strings.TrimSpace(c.Query("host_id")); value != "" {
		id, err := uuid.Parse(value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host_id"})
			return
		}
		hostID = &id
	}
	result, err := h.service.List(c.Request.Context(), hostID, c.Query("agent_type"), c.Query("risk_level"), page, size)
	if err != nil {
		h.logger.Error("agent_session_list_failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agent sessions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *AgentSessionHandler) Rules(c *gin.Context) {
	rules := h.service.BuiltinRules()
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": rules, "total": len(rules)}})
}

func (h *AgentSessionHandler) Detail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	session, _, err := h.service.Detail(c.Request.Context(), id, false)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": session})
}

func (h *AgentSessionHandler) Items(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	session, items, err := h.service.Detail(c.Request.Context(), id, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"session": session, "items": items}})
}

func (h *AgentSessionHandler) RuleHits(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	hits, err := h.service.RuleHits(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("agent_session_rule_hits_failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list rule hits"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": hits})
}

func (h *AgentSessionHandler) AIAnalysis(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	result, err := h.service.Analyze(c.Request.Context(), id)
	if err != nil {
		h.logger.Warn("agent_session_ai_analysis_failed", zap.String("session_id", id.String()), zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *AgentSessionHandler) GetAIAnalysis(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	result, err := h.service.GetAIAnalysis(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("agent_session_ai_analysis_get_failed", zap.String("session_id", id.String()), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "agent session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
