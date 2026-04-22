package handler

import (
	"context"
	"encoding/json"
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
	alertRepo     *repository.AlertRepository
	configRepo     *repository.ConfigRepository
	vectorService  *service.VectorService
	serverClient  *grpc.ServerClient
	sessions       map[string]*AISSESion
	sessionsMutex  sync.RWMutex
}

type AISSESion struct {
	SessionID     string
	AlertIDs      []string
	HostIDs       []string  // Extracted from alerts for tool routing
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

func NewAIAnalysisHandler(alertRepo *repository.AlertRepository, configRepo *repository.ConfigRepository, vectorService *service.VectorService, serverClient *grpc.ServerClient) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		alertRepo:     alertRepo,
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

	// Look up alerts to extract host_ids for tool routing
	var hostIDs []string
	if h.alertRepo != nil && len(req.AlertIDs) > 0 {
		alerts, err := h.alertRepo.FindByIDs(req.AlertIDs)
		if err != nil {
			logger.Warn("failed to look up alerts for host_ids", zap.Error(err))
		} else {
			// Extract unique host_ids from alerts
			hostIDSet := make(map[string]bool)
			for _, alert := range alerts {
				if alert.HostID.String() != "00000000-0000-0000-0000-000000000000" {
					hostIDSet[alert.HostID.String()] = true
				}
			}
			for hostID := range hostIDSet {
				hostIDs = append(hostIDs, hostID)
			}
			logger.Info("extracted host_ids from alerts",
				zap.Int("alert_count", len(alerts)),
				zap.Int("host_count", len(hostIDs)),
				zap.Strings("host_ids", hostIDs))
		}
	}

	// Create LLM client
	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 60, 3)

	sessionID := uuid.New().String()

	session := &AISSESion{
		SessionID:     sessionID,
		AlertIDs:      req.AlertIDs,
		HostIDs:      hostIDs,
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
		zap.Int("alert_count", len(req.AlertIDs)),
		zap.Int("host_count", len(hostIDs)))

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
		session.ReActAgent = llm.NewReActAgent(session.LLMClient, NewToolExecutor(h.serverClient, session.HostIDs), sessionID)
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
		session.ReActAgent = llm.NewReActAgent(session.LLMClient, NewToolExecutor(h.serverClient, session.HostIDs), sessionID)
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
	if len(session.HostIDs) > 0 {
		context["host_ids"] = session.HostIDs
	}
	if len(session.HostFilter) > 0 {
		context["host_filter"] = session.HostFilter
	}
	if session.TimeRange != nil {
		// Provide time range in multiple formats for clarity
		context["time_range"] = session.TimeRange
		// Also provide as separate fields for easier extraction by LLM
		context["start_time"] = session.TimeRange.Start.Format(time.RFC3339)
		context["end_time"] = session.TimeRange.End.Format(time.RFC3339)
	}

	return context
}

// ToolExecutor implements llm.ToolExecutor for ReAct agent
type ToolExecutor struct {
	serverClient   *grpc.ServerClient
	defaultHostIDs []string  // Fallback host_ids from session
}

// NewToolExecutor creates a new tool executor with gRPC client
func NewToolExecutor(serverClient *grpc.ServerClient, defaultHostIDs []string) *ToolExecutor {
	return &ToolExecutor{
		serverClient:   serverClient,
		defaultHostIDs: defaultHostIDs,
	}
}

// Execute executes a tool call by forwarding to the agent via gRPC
func (e *ToolExecutor) Execute(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error) {
	logger.Info("tool execution requested", zap.String("tool", tool), zap.Any("args", args), zap.Strings("default_hosts", e.defaultHostIDs))

	// Try to forward to agent via gRPC
	if e.serverClient != nil {
		// Ensure args is not nil
		if args == nil {
			args = make(map[string]interface{})
		}

		// Normalize parameter names: convert camelCase to snake_case for agent compatibility
		normalizedArgs := normalizeArgs(args)

		// Get host_id from args, or use first default host_id if available
		hostID, ok := normalizedArgs["host_id"].(string)
		if !ok || hostID == "" {
			// Use first available host_id from session as default
			if len(e.defaultHostIDs) > 0 {
				hostID = e.defaultHostIDs[0]
				normalizedArgs["host_id"] = hostID
				logger.Info("using default host_id from session", zap.String("host_id", hostID))
			} else {
				// No host_id available - return error
				return nil, fmt.Errorf("tool %s requires host_id parameter for routing to target agent, but no host_id was provided in args and no default host_id available from session", tool)
			}
		}

		// For QueryHistoricalLogs, ensure time parameters are provided
		if tool == "QueryHistoricalLogs" {
			// Check for top-level start_time/end_time OR nested time_range object
			_, hasStartTime := normalizedArgs["start_time"]
			_, hasEndTime := normalizedArgs["end_time"]

			// Also check for nested time_range: {start: ..., end: ...}
			if timeRange, ok := normalizedArgs["time_range"].(map[string]interface{}); ok {
				if startTime, ok := timeRange["start"].(string); ok && startTime != "" {
					normalizedArgs["start_time"] = startTime
					hasStartTime = true
				}
				if endTime, ok := timeRange["end"].(string); ok && endTime != "" {
					normalizedArgs["end_time"] = endTime
					hasEndTime = true
				}
			}

			if !hasStartTime {
				return nil, fmt.Errorf("QueryHistoricalLogs requires start_time parameter in RFC3339 format (e.g., '2026-04-14T10:00:00Z')")
			}
			if !hasEndTime {
				return nil, fmt.Errorf("QueryHistoricalLogs requires end_time parameter in RFC3339 format (e.g., '2026-04-14T11:00:00Z')")
			}
		}

		// Format arguments as JSON
		argsJSON, err := json.Marshal(normalizedArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool arguments: %w", err)
		}

		// Generate call ID for tracking
		callID := fmt.Sprintf("tool_%d", time.Now().UnixNano())

		logger.Info("executing tool via ExecuteTool", zap.String("host_id", hostID), zap.String("tool", tool), zap.String("call_id", callID))

		// Execute tool synchronously and wait for result
		resp, err := e.serverClient.ExecuteTool(ctx, callID, hostID, tool, string(argsJSON), 60)
		if err != nil {
			logger.Error("ExecuteTool RPC failed", zap.Error(err), zap.String("call_id", callID))
			return nil, fmt.Errorf("failed to execute tool via agent: %w", err)
		}

		if !resp.Success {
			logger.Warn("tool execution returned error", zap.String("error", resp.Error), zap.String("call_id", callID))
			return nil, fmt.Errorf("tool execution failed: %s", resp.Error)
		}

		logger.Info("tool execution succeeded", zap.String("result", resp.Result), zap.Int64("exec_time_ms", resp.ExecutionTimeMs), zap.String("call_id", callID))

		// Parse the result JSON if present
		if resp.Result != "" {
			var result interface{}
			if err := json.Unmarshal([]byte(resp.Result), &result); err != nil {
				// If not JSON, return as string
				return resp.Result, nil
			}
			return result, nil
		}

		return nil, nil
	}

	// No gRPC client available - return error instead of mock data
	// This ensures the AI knows the tool failed and can reason accordingly
	logger.Warn("ToolExecutor has no serverClient - returning error instead of mock data")
	return nil, fmt.Errorf("tool execution unavailable: no connection to agent server (gRPC client not initialized)")
}

// normalizeArgs converts camelCase keys to snake_case for agent compatibility
func normalizeArgs(args map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Mapping of common camelCase to snake_case
	camelToSnake := map[string]string{
		"hostId":        "host_id",
		"startTime":     "start_time",
		"endTime":       "end_time",
		"processName":   "process_name",
		"commandLine":   "command_line",
		"filePath":      "file_path",
		"maxSize":       "max_size",
		"filter":        "filter",
		"callId":        "call_id",
		"timeRange":     "time_range",
		"alertIds":      "alert_ids",
		"hostFilter":    "host_filter",
		"pageSize":      "page_size",
		"page":          "page",
		"sessionId":     "session_id",
		"ruleId":        "rule_id",
		"alertId":       "alert_id",
		"userId":        "user_id",
		"processId":     "process_id",
	}

	for k, v := range args {
		if snakeKey, ok := camelToSnake[k]; ok {
			result[snakeKey] = v
		} else {
			result[k] = v
		}
	}

	return result
}