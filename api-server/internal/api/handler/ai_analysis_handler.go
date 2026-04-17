package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"api-server/internal/grpc"
	"api-server/internal/llm"
	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AIAnalysisHandler struct {
	configRepo     *repository.ConfigRepository
	vectorService  *service.VectorService
	serverClient  *grpc.ServerClient
	sessions       map[string]*AISSESion
	sessionsMutex  sync.RWMutex
}

type AISSESion struct {
	SessionID     string
	AlertIDs      []string
	HostFilter    []string
	TimeRange     *TimeRange
	InitialQuery  string
	Status        string
	CreatedAt     time.Time
	Messages      []*llm.AIMessage
	LLMClient     *llm.LLMClient
	ReActAgent    *llm.ReActAgent
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type CreateSessionRequest struct {
	AlertIDs   []string   `json:"alert_ids"`
	TimeRange  *TimeRange `json:"time_range"`
	HostFilter []string   `json:"host_filter"`
}

type CreateSessionResponse struct {
	SessionID      string `json:"session_id"`
	Status         string `json:"status"`
	SelectedAlerts int    `json:"selected_alerts"`
}

type SendMessageRequest struct {
	Content string `json:"content"`
}

type SendMessageResponse struct {
	MessageID  string              `json:"message_id"`
	Role      string              `json:"role"`
	Content   string              `json:"content"`
	ToolCalls []*llm.ToolCall     `json:"tool_calls,omitempty"`
	Steps     []llm.AgentStep     `json:"steps,omitempty"`
}

type SimilarCaseRequest struct {
	Query     string  `json:"query"`
	AlertType string  `json:"alert_type"`
	Threshold float64 `json:"threshold"`
	Limit     int     `json:"limit"`
}

type SimilarCaseResponse struct {
	SimilarCases []service.SimilarAnalysis `json:"similar_cases"`
}

func NewAIAnalysisHandler(configRepo *repository.ConfigRepository, vectorService *service.VectorService, serverClient *grpc.ServerClient) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		configRepo:    configRepo,
		vectorService: vectorService,
		serverClient:  serverClient,
		sessions:      make(map[string]*AISSESion),
	}
}

// CreateSession creates a new AI analysis session
// POST /api/v1/detection/alerts/ai-analysis/session
func (h *AIAnalysisHandler) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Get LLM config
	config, err := h.configRepo.GetActive()
	if err != nil {
		logger.Error("failed to get LLM config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get LLM config"})
		return
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		logger.Error("failed to decrypt API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decrypt API key"})
		return
	}

	// Create LLM client
	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 60, 3)

	sessionID := uuid.New().String()

	session := &AISSESion{
		SessionID:     sessionID,
		AlertIDs:      req.AlertIDs,
		HostFilter:    req.HostFilter,
		TimeRange:     req.TimeRange,
		InitialQuery:  "",
		Status:        "active",
		CreatedAt:     time.Now(),
		Messages:      make([]*llm.AIMessage, 0),
		LLMClient:     llmClient,
		ReActAgent:    nil,
	}

	h.sessionsMutex.Lock()
	h.sessions[sessionID] = session
	h.sessionsMutex.Unlock()

	logger.Info("AI analysis session created",
		zap.String("session_id", sessionID),
		zap.Int("alert_count", len(req.AlertIDs)))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": CreateSessionResponse{
			SessionID:      sessionID,
			Status:         "active",
			SelectedAlerts: len(req.AlertIDs),
		},
	})
}

// StreamMessage handles SSE streaming for AI analysis
// GET /api/v1/detection/alerts/ai-analysis/{session_id}/stream
func (h *AIAnalysisHandler) StreamMessage(c *gin.Context) {
	sessionID := c.Param("session_id")
	message := c.Query("message")

	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	h.sessionsMutex.RLock()
	session, exists := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	// Create SSE writer
	sseWriter := llm.NewSSEWriter(c.Writer)

	// Initialize ReAct agent if not exists
	if session.ReActAgent == nil {
		session.ReActAgent = llm.NewReActAgent(session.LLMClient, NewToolExecutor(h.serverClient), sessionID)
	}

	// Build context for the session
	context := h.buildSessionContext(session)

	// Stream the response
	ctx := c.Request.Context()
	if err := session.ReActAgent.Stream(ctx, message, session.Messages, sseWriter, context); err != nil {
		logger.Error("stream error", zap.Error(err), zap.String("session_id", sessionID))
		sseWriter.WriteError(fmt.Sprintf("stream error: %v", err))
	}

	// Add user message to history
	session.Messages = append(session.Messages, &llm.AIMessage{
		Role:    "user",
		Content: message,
	})
}

// SendMessage handles regular (non-streaming) message sending
// POST /api/v1/detection/alerts/ai-analysis/{session_id}/message
func (h *AIAnalysisHandler) SendMessage(c *gin.Context) {
	sessionID := c.Param("session_id")

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	h.sessionsMutex.RLock()
	session, exists := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Initialize ReAct agent if not exists
	if session.ReActAgent == nil {
		session.ReActAgent = llm.NewReActAgent(session.LLMClient, NewToolExecutor(h.serverClient), sessionID)
	}

	// Build context
	context := h.buildSessionContext(session)

	// Invoke the agent
	ctx := c.Request.Context()
	resp, err := session.ReActAgent.Invoke(ctx, req.Content, session.Messages, context)
	if err != nil {
		logger.Error("invoke error", zap.Error(err), zap.String("session_id", sessionID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("invoke error: %v", err)})
		return
	}

	// Add user message to history
	session.Messages = append(session.Messages, &llm.AIMessage{
		Role:    "user",
		Content: req.Content,
	})

	// Add assistant response to history
	session.Messages = append(session.Messages, &llm.AIMessage{
		Role:    "assistant",
		Content: resp.Content,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": SendMessageResponse{
			MessageID:  uuid.New().String(),
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
			Steps:     resp.Steps,
		},
	})
}

// FindSimilarCases finds similar analysis cases using vector search
// POST /api/v1/detection/alerts/ai-analysis/similar
func (h *AIAnalysisHandler) FindSimilarCases(c *gin.Context) {
	var req SimilarCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Threshold == 0 {
		req.Threshold = 0.75
	}
	if req.Limit == 0 {
		req.Limit = 5
	}

	ctx := c.Request.Context()
	similar, err := h.vectorService.FindSimilarAnalysis(ctx, req.Query, req.AlertType, req.Threshold, req.Limit)
	if err != nil {
		logger.Error("find similar analysis error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("search error: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": SimilarCaseResponse{
			SimilarCases: similar,
		},
	})
}

// GetRAGContext builds RAG context for a query
// POST /api/v1/detection/alerts/ai-analysis/rag-context
func (h *AIAnalysisHandler) GetRAGContext(c *gin.Context) {
	var req struct {
		Query     string `json:"query" binding:"required"`
		AlertType string `json:"alert_type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()
	ragContext, err := h.vectorService.BuildRAGContext(ctx, req.Query, req.AlertType)
	if err != nil {
		logger.Error("build RAG context error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("RAG context error: %v", err)})
		return
	}

	var caseCount int
	if ragContext != "" {
		caseCount = 3 // BuildRAGContext returns up to 3 similar cases
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"context":     ragContext,
			"case_count": caseCount,
		},
	})
}

// GetSessionHistory gets the message history for a session
// GET /api/v1/detection/alerts/ai-analysis/{session_id}/history
func (h *AIAnalysisHandler) GetSessionHistory(c *gin.Context) {
	sessionID := c.Param("session_id")

	h.sessionsMutex.RLock()
	session, exists := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"session_id": sessionID,
			"messages":   session.Messages,
		},
	})
}

// buildSessionContext builds the context map for a session
func (h *AIAnalysisHandler) buildSessionContext(session *AISSESion) map[string]interface{} {
	context := make(map[string]interface{})

	if len(session.AlertIDs) > 0 {
		context["alert_ids"] = session.AlertIDs
	}
	if len(session.HostFilter) > 0 {
		context["host_filter"] = session.HostFilter
	}
	if session.TimeRange != nil {
		context["time_range"] = session.TimeRange
	}

	return context
}

// ToolExecutor implements llm.ToolExecutor for ReAct agent
type ToolExecutor struct {
	serverClient *grpc.ServerClient
}

// NewToolExecutor creates a new tool executor with gRPC client
func NewToolExecutor(serverClient *grpc.ServerClient) *ToolExecutor {
	return &ToolExecutor{
		serverClient: serverClient,
	}
}

// Execute executes a tool call by forwarding to the agent via gRPC
func (e *ToolExecutor) Execute(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error) {
	logger.Info("tool execution requested", zap.String("tool", tool), zap.Any("args", args))

	// For now, return mock data since we need to implement gRPC forwarding
	// TODO: Implement actual gRPC forwarding to agents via Server hub
	return e.getMockToolResult(tool, args)
}

// getMockToolResult returns mock data for testing
func (e *ToolExecutor) getMockToolResult(tool string, args map[string]interface{}) (interface{}, error) {
	switch tool {
	case "GetRunningProcesses":
		return map[string]interface{}{
			"status": "success",
			"tool":  tool,
			"processes": []map[string]interface{}{
				{"pid": 1, "name": "systemd", "user": "root", "cmdline": "/sbin/init"},
				{"pid": 123, "name": "sshd", "user": "root", "cmdline": "/usr/sbin/sshd"},
				{"pid": 456, "name": "nginx", "user": "www-data", "cmdline": "/usr/sbin/nginx"},
			},
		}, nil
	case "GetProcessTree":
		pid := 1
		if p, ok := args["pid"].(float64); ok {
			pid = int(p)
		}
		return map[string]interface{}{
			"status": "success",
			"tool":   tool,
			"pid":    pid,
			"tree": []map[string]interface{}{
				{"pid": pid, "name": "systemd", "ppid": 0, "children": []map[string]interface{}{
					{"pid": 123, "name": "sshd", "ppid": 1},
					{"pid": 456, "name": "nginx", "ppid": 1},
				}},
			},
		}, nil
	case "GetNetworkConnections":
		return map[string]interface{}{
			"status": "success",
			"tool":   tool,
			"connections": []map[string]interface{}{
				{"protocol": "tcp", "local_addr": "0.0.0.0:22", "remote_addr": "192.168.1.100:54321", "state": "ESTABLISHED"},
				{"protocol": "tcp", "local_addr": "0.0.0.0:80", "remote_addr": "0.0.0.0:0", "state": "LISTEN"},
			},
		}, nil
	case "GetOpenFiles":
		return map[string]interface{}{
			"status": "success",
			"tool":   tool,
			"files": []string{
				"/etc/passwd",
				"/etc/shadow",
				"/var/log/syslog",
			},
		}, nil
	case "GetUserSessions":
		return map[string]interface{}{
			"status": "success",
			"tool":   tool,
			"sessions": []map[string]interface{}{
				{"user": "root", "tty": "pts/0", "from": "192.168.1.100", "login_time": "2026-04-17T10:00:00Z"},
				{"user": "admin", "tty": "pts/1", "from": "192.168.1.101", "login_time": "2026-04-17T09:30:00Z"},
			},
		}, nil
	case "QueryHistoricalLogs":
		return map[string]interface{}{
			"status": "success",
			"tool":   tool,
			"logs": []string{
				"2026-04-17T10:00:00Z sshd[123]: Accepted publickey for root from 192.168.1.100",
				"2026-04-17T09:45:00Z sudo: admin : TTY=pts/1 ; PWD=/home/admin ; USER=root ; COMMAND=/bin systemctl status nginx",
			},
		}, nil
	default:
		return map[string]interface{}{
			"status":  "error",
			"tool":    tool,
			"message": fmt.Sprintf("unknown tool: %s", tool),
		}, nil
	}
}