package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"api-server/internal/grpc"
	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AIAnalysisHandler struct {
	alertRepo     *repository.AlertRepository
	configRepo    *repository.ConfigRepository
	vectorService *service.VectorService
	serverClient  *grpc.ServerClient
	sessionRepo   *repository.AISessionRepository
	messageRepo   *repository.AIMessageRepository
	sessions      map[string]*AISSESion
	sessionsMutex sync.RWMutex
}

type AlertContextSnapshot struct {
	ID          string    `json:"id"`
	AlertID     string    `json:"alert_id"`
	HostID      string    `json:"host_id"`
	Hostname    string    `json:"hostname"`
	RuleTitle   string    `json:"rule_title"`
	MitreID     string    `json:"mitre_id"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	ProcessTree string    `json:"process_tree,omitempty"`
	LLMSummary  string    `json:"llm_summary,omitempty"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

type finalAnswerResult struct {
	AttackGraph map[string]interface{} `json:"attack_graph"`
	Conclusions []AlertConclusion      `json:"conclusions"`
}

type alertWriteback struct {
	AlertID          string
	Summary          string
	DisposalStrategy string
}

type AISSESion struct {
	SessionID      string
	AlertIDs       []string
	AlertSnapshots []AlertContextSnapshot
	HostIDs        []string // Extracted from alerts for tool routing
	HostFilter     []string
	TimeRange      *TimeRange
	InitialQuery   string
	Status         string
	CreatedAt      time.Time
	MaxIterations  int // Maximum ReAct iterations
	Messages       []*llm.AIMessage
	LLMClient      *llm.LLMClient
	ReActAgent     *llm.ReActAgent
}

const (
	defaultAnalysisMaxIterations = 15
	analysisMaxIterationsLimit   = 20
	maxToolResultEventBytes      = 20000
	maxToolResultArrayItems      = 20
	maxToolResultStringBytes     = 800
)

// SSEResponseCollector collects SSE content events to build the full AI response
type SSEResponseCollector struct {
	content        string
	thinking       string
	pendingThought string
	toolCalls      []map[string]interface{}
	toolResults    []map[string]interface{}
	steps          []llm.AgentStep
	currentStep    *llm.AgentStep
}

func (c *SSEResponseCollector) WriteContent(chunk string) error {
	c.content += chunk
	return nil
}

func (c *SSEResponseCollector) WriteThinking(content string) {
	c.thinking += content
	c.pendingThought += content
}

func (c *SSEResponseCollector) WriteToolCall(tool, callID string, args interface{}) {
	c.toolCalls = append(c.toolCalls, map[string]interface{}{
		"call_id": callID,
		"tool":    tool,
		"args":    args,
	})
	c.currentStep = &llm.AgentStep{
		Thought:     strings.TrimSpace(c.pendingThought),
		Action:      tool,
		ActionInput: toStringMap(args),
	}
	c.pendingThought = ""
}

func (c *SSEResponseCollector) WriteToolResult(callID string, result interface{}, timeMs int64) {
	observation := formatCollectedPayload(result)
	c.toolResults = append(c.toolResults, map[string]interface{}{
		"call_id": callID,
		"result":  result,
		"time_ms": timeMs,
	})
	c.finishCurrentStep(observation)
}

func (c *SSEResponseCollector) WriteToolError(callID, errMsg string) {
	c.toolResults = append(c.toolResults, map[string]interface{}{
		"call_id": callID,
		"error":   errMsg,
	})
	c.finishCurrentStep("Error: " + errMsg)
}

func (c *SSEResponseCollector) GetContent() string {
	return c.content
}

func (c *SSEResponseCollector) GetThinking() string {
	return c.thinking
}

func (c *SSEResponseCollector) GetToolCalls() []map[string]interface{} {
	return c.toolCalls
}

func (c *SSEResponseCollector) GetToolResults() []map[string]interface{} {
	return c.toolResults
}

func (c *SSEResponseCollector) GetSteps() []llm.AgentStep {
	return c.steps
}

func (c *SSEResponseCollector) ToolCallsJSONB() model.JSONB {
	if len(c.toolCalls) == 0 {
		return nil
	}
	return model.JSONB{"items": c.toolCalls}
}

func (c *SSEResponseCollector) ToolResultsJSONB() model.JSONB {
	if len(c.toolResults) == 0 {
		return nil
	}
	return model.JSONB{"items": c.toolResults}
}

func (c *SSEResponseCollector) StepsJSONB() model.JSONB {
	if len(c.steps) == 0 {
		return nil
	}
	return model.JSONB{"items": c.steps}
}

func (c *SSEResponseCollector) HasAssistantTrace() bool {
	return c.content != "" || c.thinking != "" || len(c.toolCalls) > 0 || len(c.toolResults) > 0 || len(c.steps) > 0
}

func (c *SSEResponseCollector) finishCurrentStep(observation string) {
	if c.currentStep == nil {
		c.currentStep = &llm.AgentStep{
			Thought: strings.TrimSpace(c.pendingThought),
		}
		c.pendingThought = ""
	}
	c.currentStep.Observation = observation
	c.steps = append(c.steps, *c.currentStep)
	c.currentStep = nil
}

func toStringMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil
	}
	return decoded
}

func formatCollectedPayload(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func normalizeAnalysisMaxIterations(value int) int {
	if value <= 0 {
		return defaultAnalysisMaxIterations
	}
	if value > analysisMaxIterationsLimit {
		return analysisMaxIterationsLimit
	}
	return value
}

func buildAlertSnapshots(alerts []model.Alert) []AlertContextSnapshot {
	snapshots := make([]AlertContextSnapshot, 0, len(alerts))
	for _, alert := range alerts {
		hostID := ""
		if alert.HostID != uuid.Nil {
			hostID = alert.HostID.String()
		}
		snapshots = append(snapshots, AlertContextSnapshot{
			ID:          alert.ID.String(),
			AlertID:     alert.AlertID,
			HostID:      hostID,
			Hostname:    alert.Hostname,
			RuleTitle:   alert.RuleTitle,
			MitreID:     alert.MitreID,
			Severity:    alert.Severity,
			Status:      alert.Status,
			Description: alert.Description,
			ProcessTree: alert.ProcessTree,
			LLMSummary:  alert.LLMSummary,
			FirstSeenAt: alert.FirstSeenAt,
			LastSeenAt:  alert.LastSeenAt,
		})
	}
	return snapshots
}

func collectJSONObjectCandidates(content string) []string {
	candidates := make([]string, 0)
	depth := 0
	start := -1
	inString := false
	escaped := false

	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				candidates = append(candidates, content[start:i+1])
				start = -1
			}
		}
	}

	return candidates
}

func extractFinalAnswerResult(content string) (*finalAnswerResult, error) {
	normalized := strings.ReplaceAll(content, "```json", "")
	normalized = strings.ReplaceAll(normalized, "```", "")
	for _, candidate := range collectJSONObjectCandidates(normalized) {
		var result finalAnswerResult
		if err := json.Unmarshal([]byte(candidate), &result); err != nil {
			continue
		}
		if len(result.AttackGraph) == 0 && len(result.Conclusions) == 0 {
			continue
		}
		return &result, nil
	}

	return nil, fmt.Errorf("no final answer JSON found")
}

func buildAlertWritebacks(session *AISSESion, result *finalAnswerResult) []alertWriteback {
	if session == nil || result == nil {
		return nil
	}

	disposalStrategy := strings.TrimSpace(strings.Join(attackGraphRecommendations(result.AttackGraph), "；"))
	snapshotByAnyID := make(map[string]AlertContextSnapshot, len(session.AlertSnapshots)*2)
	for _, snapshot := range session.AlertSnapshots {
		if snapshot.ID != "" {
			snapshotByAnyID[snapshot.ID] = snapshot
		}
		if snapshot.AlertID != "" {
			snapshotByAnyID[snapshot.AlertID] = snapshot
		}
	}

	writebacks := make([]alertWriteback, 0, len(result.Conclusions))
	for _, conclusion := range result.Conclusions {
		snapshot, ok := snapshotByAnyID[conclusion.AlertID]
		if !ok || snapshot.AlertID == "" {
			continue
		}
		summary := strings.TrimSpace(conclusion.Summary)
		if summary == "" {
			summary = strings.TrimSpace(attackGraphStringField(result.AttackGraph, "summary"))
		}
		writebacks = append(writebacks, alertWriteback{
			AlertID:          snapshot.AlertID,
			Summary:          summary,
			DisposalStrategy: disposalStrategy,
		})
	}

	return writebacks
}

func attackGraphStringField(graph map[string]interface{}, key string) string {
	if graph == nil {
		return ""
	}
	value, ok := graph[key].(string)
	if !ok {
		return ""
	}
	return value
}

func attackGraphRecommendations(graph map[string]interface{}) []string {
	if graph == nil {
		return nil
	}

	switch value := graph["recommendations"].(type) {
	case []string:
		return value
	case []interface{}:
		recommendations := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				recommendations = append(recommendations, strings.TrimSpace(text))
			}
		}
		return recommendations
	default:
		return nil
	}
}

func compactToolResultPayload(value interface{}) interface{} {
	data, err := json.Marshal(value)
	if err == nil && len(data) <= maxToolResultEventBytes {
		return value
	}

	compact := compactJSONValue(value, 0)
	if data, err := json.Marshal(compact); err == nil && len(data) <= maxToolResultEventBytes {
		return compact
	}

	preview := formatCollectedPayload(value)
	originalBytes := len(preview)
	if len(preview) > maxToolResultEventBytes {
		preview = preview[:maxToolResultEventBytes]
	}
	return map[string]interface{}{
		"truncated":           true,
		"original_size_bytes": originalBytes,
		"preview":             preview,
		"notice":              "tool result was too large and was truncated for UI/history display",
	}
}

func compactJSONValue(value interface{}, depth int) interface{} {
	if depth > 6 {
		return map[string]interface{}{
			"truncated": true,
			"notice":    "nested value omitted",
		}
	}

	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed)+1)
		for k, v := range typed {
			out[k] = compactJSONValue(v, depth+1)
		}
		out["truncated"] = true
		out["notice"] = "large tool result compacted for UI/history display"
		return out
	case []interface{}:
		limit := len(typed)
		if limit > maxToolResultArrayItems {
			limit = maxToolResultArrayItems
		}
		out := make([]interface{}, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, compactJSONValue(typed[i], depth+1))
		}
		if len(typed) > limit {
			out = append(out, map[string]interface{}{
				"truncated":       true,
				"omitted_items":   len(typed) - limit,
				"displayed_items": limit,
			})
		}
		return out
	case string:
		if len(typed) > maxToolResultStringBytes {
			return typed[:maxToolResultStringBytes] + fmt.Sprintf("\n... [truncated %d bytes for UI/history display]", len(typed)-maxToolResultStringBytes)
		}
		return typed
	default:
		return value
	}
}

// collectingSSEWriter wraps SSEWriter to also collect content for persistence
type collectingSSEWriter struct {
	writer      *llm.SSEWriter
	collector   *SSEResponseCollector
	beforeDone  func(content string) error
	doneHandled bool
}

func (w *collectingSSEWriter) Write(event llm.SSEEvent) error {
	// Collect content events
	if event.Type == "content" {
		w.collector.WriteContent(event.Content)
	}
	// Write to actual SSE writer
	return w.writer.Write(event)
}

func (w *collectingSSEWriter) WriteThinking(content string) error {
	w.collector.WriteThinking(content)
	return w.writer.WriteThinking(content)
}

func (w *collectingSSEWriter) WriteToolCall(tool, callID string, args interface{}) error {
	w.collector.WriteToolCall(tool, callID, args)
	return w.writer.WriteToolCall(tool, callID, args)
}

func (w *collectingSSEWriter) WriteToolResult(callID string, result interface{}, timeMs int64) error {
	displayResult := compactToolResultPayload(result)
	w.collector.WriteToolResult(callID, displayResult, timeMs)
	return w.writer.WriteToolResult(callID, displayResult, timeMs)
}

func (w *collectingSSEWriter) WriteToolError(callID, errMsg string) error {
	w.collector.WriteToolError(callID, errMsg)
	return w.writer.WriteToolError(callID, errMsg)
}

func (w *collectingSSEWriter) WriteContent(content string) error {
	// First collect the content
	w.collector.WriteContent(content)
	// Then write to SSE writer
	return w.writer.WriteContent(content)
}

func (w *collectingSSEWriter) WriteDone() error {
	if w.beforeDone != nil && !w.doneHandled {
		w.doneHandled = true
		if err := w.beforeDone(w.collector.GetContent()); err != nil {
			return err
		}
	}
	return w.writer.WriteDone()
}

func (w *collectingSSEWriter) WriteError(errMsg string) error {
	return w.writer.WriteError(errMsg)
}

func (w *collectingSSEWriter) Flush() {
	w.writer.Flush()
}

// truncate truncates a string to a maximum length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type CreateSessionRequest struct {
	AlertIDs      []string   `json:"alert_ids"`
	TimeRange     *TimeRange `json:"time_range"`
	HostFilter    []string   `json:"host_filter"`
	MaxIterations int        `json:"max_iterations"`
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
	MessageID string          `json:"message_id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls []*llm.ToolCall `json:"tool_calls,omitempty"`
	Steps     []llm.AgentStep `json:"steps,omitempty"`
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

type AlertConclusion struct {
	AlertID string `json:"alert_id"`
	Action  string `json:"action"`  // mark_false_positive, confirm_threat, generate_rule
	Summary string `json:"summary"` // AI分析摘要
}

type ApplyConclusionRequest struct {
	Conclusions []AlertConclusion `json:"conclusions"`
}

func NewAIAnalysisHandler(
	alertRepo *repository.AlertRepository,
	configRepo *repository.ConfigRepository,
	vectorService *service.VectorService,
	serverClient *grpc.ServerClient,
	sessionRepo *repository.AISessionRepository,
	messageRepo *repository.AIMessageRepository,
) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		alertRepo:     alertRepo,
		configRepo:    configRepo,
		vectorService: vectorService,
		serverClient:  serverClient,
		sessionRepo:   sessionRepo,
		messageRepo:   messageRepo,
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

	// Look up alerts to extract host_ids and build real alert context for analysis
	var hostIDs []string
	var alertSnapshots []AlertContextSnapshot
	if h.alertRepo != nil && len(req.AlertIDs) > 0 {
		alerts, err := h.alertRepo.FindByIDs(req.AlertIDs)
		if err != nil {
			logger.Warn("failed to look up alerts for host_ids", zap.Error(err))
		} else {
			alertSnapshots = buildAlertSnapshots(alerts)
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
		SessionID:      sessionID,
		AlertIDs:       req.AlertIDs,
		AlertSnapshots: alertSnapshots,
		HostIDs:        hostIDs,
		HostFilter:     req.HostFilter,
		TimeRange:      req.TimeRange,
		InitialQuery:   "",
		Status:         "active",
		CreatedAt:      time.Now(),
		MaxIterations:  normalizeAnalysisMaxIterations(req.MaxIterations),
		Messages:       make([]*llm.AIMessage, 0),
		LLMClient:      llmClient,
		ReActAgent:     nil,
	}

	h.sessionsMutex.Lock()
	h.sessions[sessionID] = session
	h.sessionsMutex.Unlock()

	// Persist session to database
	if h.sessionRepo != nil {
		timeRangeJSON := model.JSONB{}
		if req.TimeRange != nil {
			timeRangeJSON = model.JSONB{
				"start": req.TimeRange.Start,
				"end":   req.TimeRange.End,
			}
		}
		dbSession := &model.AISession{
			SessionID:     sessionID,
			AlertIDs:      model.StringArray(req.AlertIDs),
			HostIDs:       model.StringArray(hostIDs),
			HostFilter:    model.StringArray(req.HostFilter),
			TimeRange:     timeRangeJSON,
			Status:        "active",
			MaxIterations: normalizeAnalysisMaxIterations(req.MaxIterations),
		}
		if err := h.sessionRepo.Create(dbSession); err != nil {
			logger.Warn("failed to persist session", zap.Error(err))
		}
	}

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
		var err error
		session, err = h.restoreSessionFromDB(sessionID)
		if err != nil {
			logger.Warn("failed to restore AI session for stream",
				zap.String("session_id", sessionID),
				zap.Error(err))
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	// Create SSE writer and response collector
	sseWriter := llm.NewSSEWriter(c.Writer)
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("AI analysis stream interrupted: %v", r)
			logger.Error("AI analysis stream panic recovered",
				zap.Any("error", r),
				zap.String("session_id", sessionID))
			_ = sseWriter.WriteError(errMsg)
			_ = sseWriter.WriteDone()
		}
	}()

	responseCollector := &SSEResponseCollector{}

	// Create a wrapper that captures content while writing SSE
	collectingWriter := &collectingSSEWriter{
		writer:    sseWriter,
		collector: responseCollector,
		beforeDone: func(content string) error {
			return h.writeFlowchartImageEvent(c.Request.Context(), sseWriter, content)
		},
	}

	// Initialize ReAct agent if not exists
	if session.ReActAgent == nil {
		session.ReActAgent = llm.NewReActAgent(session.LLMClient, NewToolExecutor(h.serverClient, session.HostIDs), sessionID, session.MaxIterations)
	}

	// Build context for the session
	context := h.buildSessionContext(session)

	logger.Info("StreamMessage: calling ReActAgent.Stream", zap.String("session_id", sessionID))

	// Stream the response - use collectingWriter instead of sseWriter
	ctx := c.Request.Context()
	if err := session.ReActAgent.Stream(ctx, message, session.Messages, collectingWriter, context); err != nil {
		logger.Error("stream error", zap.Error(err), zap.String("session_id", sessionID))
		sseWriter.WriteError(fmt.Sprintf("stream error: %v", err))
	}
	logger.Info("StreamMessage: ReActAgent.Stream returned", zap.String("session_id", sessionID))

	// Get the full AI response content
	aiResponseContent := responseCollector.GetContent()
	logger.Info("AI response collected", zap.String("session_id", sessionID), zap.Int("content_len", len(aiResponseContent)))

	h.persistAnalysisOutcome(session, aiResponseContent)

	// Persist user message to database
	if h.messageRepo != nil {
		userMsg := &model.AIMessage{
			SessionID: sessionID,
			MessageID: uuid.New().String(),
			Role:      "user",
			Content:   message,
		}
		if err := h.messageRepo.Create(userMsg); err != nil {
			logger.Warn("failed to persist user message", zap.Error(err))
		}
		if h.sessionRepo != nil {
			h.sessionRepo.IncrementMessageCount(sessionID)
		}

		// Persist AI response and the complete ReAct trace collected from the SSE stream.
		if responseCollector.HasAssistantTrace() {
			aiMsg := &model.AIMessage{
				SessionID:   sessionID,
				MessageID:   uuid.New().String(),
				Role:        "assistant",
				Content:     aiResponseContent,
				Thinking:    responseCollector.GetThinking(),
				ToolCalls:   responseCollector.ToolCallsJSONB(),
				ToolResults: responseCollector.ToolResultsJSONB(),
				Steps:       responseCollector.StepsJSONB(),
			}
			if err := h.messageRepo.Create(aiMsg); err != nil {
				logger.Warn("failed to persist AI response", zap.Error(err))
			} else {
				logger.Info("persisted AI response", zap.String("session_id", sessionID), zap.Int("content_len", len(aiResponseContent)))
				if h.sessionRepo != nil {
					h.sessionRepo.IncrementMessageCount(sessionID)
					for range responseCollector.GetToolCalls() {
						h.sessionRepo.IncrementToolCallCount(sessionID)
					}
				}
			}
		}
	}

	// Add messages to in-memory session history in the same order users saw them.
	session.Messages = append(session.Messages, &llm.AIMessage{
		Role:    "user",
		Content: message,
	})
	if aiResponseContent != "" {
		session.Messages = append(session.Messages, &llm.AIMessage{
			Role:    "assistant",
			Content: aiResponseContent,
		})
	}
}

func (h *AIAnalysisHandler) persistAnalysisOutcome(session *AISSESion, finalContent string) {
	if session == nil {
		return
	}

	result, err := extractFinalAnswerResult(finalContent)
	if err != nil {
		logger.Warn("failed to parse AI final answer result",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
		return
	}

	session.Status = "completed"
	if h.sessionRepo != nil {
		var conclusionJSON model.JSONB
		if payload, marshalErr := json.Marshal(result); marshalErr == nil {
			_ = json.Unmarshal(payload, &conclusionJSON)
			if updateErr := h.sessionRepo.UpdateConclusion(session.SessionID, conclusionJSON); updateErr != nil {
				logger.Warn("failed to persist AI session conclusion",
					zap.String("session_id", session.SessionID),
					zap.Error(updateErr))
			}
		}
	}

	if h.alertRepo == nil {
		return
	}

	for _, writeback := range buildAlertWritebacks(session, result) {
		if err := h.alertRepo.UpdateAIAnalysisResult(writeback.AlertID, writeback.Summary, writeback.DisposalStrategy); err != nil {
			logger.Warn("failed to persist AI analysis result to alert",
				zap.String("session_id", session.SessionID),
				zap.String("alert_id", writeback.AlertID),
				zap.Error(err))
		}
	}
}

func (h *AIAnalysisHandler) writeFlowchartImageEvent(ctx context.Context, writer *llm.SSEWriter, finalContent string) error {
	finalContent = strings.TrimSpace(finalContent)
	if finalContent == "" {
		return nil
	}

	config, err := h.configRepo.GetActiveImageModel()
	if err != nil {
		return writer.Write(llm.SSEEvent{
			Type:  "flowchart_image",
			Error: "图片模型配置不可用: " + err.Error(),
		})
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return writer.Write(llm.SSEEvent{
			Type:  "flowchart_image",
			Error: "图片模型密钥解密失败: " + err.Error(),
		})
	}

	req := ImageModelConfigRequest{
		APIKey:    apiKey,
		Provider:  config.Provider,
		BaseURL:   config.BaseURL,
		ModelName: config.ModelName,
	}
	normalizeImageModelConfigRequest(&req)

	imageURL, err := generateImageModel(ctx, req, buildFlowchartImagePrompt(finalContent))
	if err != nil {
		return writer.Write(llm.SSEEvent{
			Type:  "flowchart_image",
			Error: "图片模型生成溯源图失败: " + err.Error(),
		})
	}
	if imageURL == "" {
		return writer.Write(llm.SSEEvent{
			Type:  "flowchart_image",
			Error: "图片模型未返回图片 URL",
		})
	}

	return writer.Write(llm.SSEEvent{
		Type: "flowchart_image",
		Result: map[string]interface{}{
			"url":        imageURL,
			"provider":   req.Provider,
			"model_name": req.ModelName,
		},
	})
}

func buildFlowchartImagePrompt(finalContent string) string {
	const maxPromptContentBytes = 4000
	if len(finalContent) > maxPromptContentBytes {
		finalContent = finalContent[:maxPromptContentBytes]
	}
	return "请根据以下 AI 安全分析最终结论生成一张攻击溯源流程图。要求：白底，清晰的节点和箭头，突出攻击入口、执行过程、横向移动、影响范围和最终处置建议；不要生成写实人物；使用中文标签。\n\n" + finalContent
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
		var err error
		session, err = h.restoreSessionFromDB(sessionID)
		if err != nil {
			logger.Warn("failed to restore AI session for message",
				zap.String("session_id", sessionID),
				zap.Error(err))
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
	}

	// Initialize ReAct agent if not exists
	if session.ReActAgent == nil {
		session.ReActAgent = llm.NewReActAgent(session.LLMClient, NewToolExecutor(h.serverClient, session.HostIDs), sessionID, session.MaxIterations)
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
			MessageID: uuid.New().String(),
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
			"context":    ragContext,
			"case_count": caseCount,
		},
	})
}

// GetSessionHistory gets the message history for a session
// GET /api/v1/detection/alerts/ai-analysis/{session_id}/history
func (h *AIAnalysisHandler) GetSessionHistory(c *gin.Context) {
	sessionID := c.Param("session_id")

	// Try to read from database first
	if h.messageRepo != nil {
		messages, err := h.messageRepo.FindBySessionID(sessionID)
		if err == nil && len(messages) > 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"session_id": sessionID,
					"messages":   messages,
				},
			})
			return
		}
		logger.Warn("no messages found in DB or error, falling back to memory", zap.Error(err))
	}

	// Fallback to memory
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

// GetSessionList gets the list of sessions with pagination
// GET /api/v1/detection/alerts/ai-analysis/sessions
func (h *AIAnalysisHandler) GetSessionList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if h.sessionRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session repository not available"})
		return
	}

	sessions, total, err := h.sessionRepo.FindList(page, pageSize, status)
	if err != nil {
		logger.Error("failed to get session list", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"sessions":  sessions,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// DeleteSession deletes a session and its messages
// DELETE /api/v1/detection/alerts/ai-analysis/{session_id}
func (h *AIAnalysisHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	// Delete from database first
	if h.sessionRepo != nil {
		if err := h.sessionRepo.Delete(sessionID); err != nil {
			logger.Error("failed to delete session from DB", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete session"})
			return
		}
	}

	// Remove from memory after DB delete succeeds
	h.sessionsMutex.Lock()
	delete(h.sessions, sessionID)
	h.sessionsMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "session deleted",
	})
}

// ApplyConclusion applies AI analysis conclusions to alerts
// POST /api/v1/detection/alerts/ai-analysis/{session_id}/conclusion
func (h *AIAnalysisHandler) ApplyConclusion(c *gin.Context) {
	sessionID := c.Param("session_id")

	var req ApplyConclusionRequest
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

	// Apply conclusions to each alert
	for _, conclusion := range req.Conclusions {
		summary := conclusion.Summary
		if summary == "" {
			summary = fmt.Sprintf("AI分析判定: %s", conclusion.Action)
		}

		switch conclusion.Action {
		case "mark_false_positive":
			if h.alertRepo != nil {
				h.alertRepo.UpdateLLMSummary(conclusion.AlertID, fmt.Sprintf("AI判定为误报：%s", summary))
			}
		case "confirm_threat":
			if h.alertRepo != nil {
				h.alertRepo.MarkAIJudged(conclusion.AlertID, summary)
			}
		case "generate_rule":
			// TODO: Generate rule from alert
			logger.Info("generate_rule not implemented yet", zap.String("alert_id", conclusion.AlertID))
		}
	}

	// Update session status
	session.Status = "completed"

	logger.Info("applied conclusions", zap.String("session_id", sessionID), zap.Int("count", len(req.Conclusions)))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("已应用 %d 个结论", len(req.Conclusions)),
	})
}

func (h *AIAnalysisHandler) restoreSessionFromDB(sessionID string) (*AISSESion, error) {
	if h.sessionRepo == nil || h.configRepo == nil {
		return nil, fmt.Errorf("session restore dependencies unavailable")
	}

	dbSession, err := h.sessionRepo.FindBySessionID(sessionID)
	if err != nil {
		return nil, err
	}

	config, err := h.configRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM config: %w", err)
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	maxIterations := normalizeAnalysisMaxIterations(dbSession.MaxIterations)
	alertSnapshots := make([]AlertContextSnapshot, 0)
	if h.alertRepo != nil && len(dbSession.AlertIDs) > 0 {
		if alerts, alertErr := h.alertRepo.FindByIDs([]string(dbSession.AlertIDs)); alertErr != nil {
			logger.Warn("failed to restore alert snapshots for AI session",
				zap.String("session_id", sessionID),
				zap.Error(alertErr))
		} else {
			alertSnapshots = buildAlertSnapshots(alerts)
		}
	}

	session := &AISSESion{
		SessionID:      dbSession.SessionID,
		AlertIDs:       []string(dbSession.AlertIDs),
		AlertSnapshots: alertSnapshots,
		HostIDs:        []string(dbSession.HostIDs),
		HostFilter:     []string(dbSession.HostFilter),
		TimeRange:      restoreTimeRange(dbSession.TimeRange),
		Status:         dbSession.Status,
		CreatedAt:      dbSession.CreatedAt,
		MaxIterations:  maxIterations,
		Messages:       h.restoreLLMMessages(sessionID),
		LLMClient:      llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 60, 3),
		ReActAgent:     nil,
	}

	h.sessionsMutex.Lock()
	defer h.sessionsMutex.Unlock()
	if existing, ok := h.sessions[sessionID]; ok {
		return existing, nil
	}
	h.sessions[sessionID] = session

	logger.Info("restored AI analysis session from DB",
		zap.String("session_id", sessionID),
		zap.Int("history_messages", len(session.Messages)))

	return session, nil
}

func (h *AIAnalysisHandler) restoreLLMMessages(sessionID string) []*llm.AIMessage {
	if h.messageRepo == nil {
		return make([]*llm.AIMessage, 0)
	}

	dbMessages, err := h.messageRepo.FindBySessionID(sessionID)
	if err != nil {
		logger.Warn("failed to restore AI message history",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return make([]*llm.AIMessage, 0)
	}

	messages := make([]*llm.AIMessage, 0, len(dbMessages))
	for _, msg := range dbMessages {
		content := strings.TrimSpace(msg.Content)
		if content == "" && msg.Role == "assistant" {
			content = strings.TrimSpace(msg.Thinking)
		}
		if content == "" {
			continue
		}
		if len(content) > 12000 {
			content = content[:12000] + "\n... [restored history truncated]"
		}
		messages = append(messages, &llm.AIMessage{
			Role:    msg.Role,
			Content: content,
		})
	}

	return messages
}

func restoreTimeRange(value model.JSONB) *TimeRange {
	if len(value) == 0 {
		return nil
	}

	start, okStart := parseJSONBTime(value["start"])
	end, okEnd := parseJSONBTime(value["end"])
	if !okStart || !okEnd {
		return nil
	}

	return &TimeRange{Start: start, End: end}
}

func parseJSONBTime(value interface{}) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case string:
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	default:
		return time.Time{}, false
	}
}

// buildSessionContext builds the context map for a session
func (h *AIAnalysisHandler) buildSessionContext(session *AISSESion) map[string]interface{} {
	context := make(map[string]interface{})

	if len(session.AlertIDs) > 0 {
		context["alert_ids"] = session.AlertIDs
	}
	if len(session.AlertSnapshots) > 0 {
		context["alerts"] = session.AlertSnapshots
		context["selected_alert_count"] = len(session.AlertSnapshots)
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
	defaultHostIDs []string // Fallback host_ids from session
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
		"hostId":      "host_id",
		"startTime":   "start_time",
		"endTime":     "end_time",
		"processName": "process_name",
		"commandLine": "command_line",
		"filePath":    "file_path",
		"maxSize":     "max_size",
		"filter":      "filter",
		"callId":      "call_id",
		"timeRange":   "time_range",
		"alertIds":    "alert_ids",
		"hostFilter":  "host_filter",
		"pageSize":    "page_size",
		"page":        "page",
		"sessionId":   "session_id",
		"ruleId":      "rule_id",
		"alertId":     "alert_id",
		"userId":      "user_id",
		"processId":   "process_id",
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
