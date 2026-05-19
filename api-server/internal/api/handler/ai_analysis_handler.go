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
	"api-server/internal/llm/adapters"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"

	agentruntime "github.com/alex-chenc/agent-runtime"

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
	agentExecRepo *repository.AgentExecutionRepository
	sessions      map[string]*AISSESion
	sessionsMutex sync.RWMutex
	activeRuns    map[string]activeAnalysisRun
	activeRunsMu  sync.Mutex
}

type activeAnalysisRun struct {
	id     string
	cancel context.CancelFunc
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
	// 进程级别信息
	PID         int    `json:"pid"`
	PPID        int    `json:"ppid"`
	CommandLine string `json:"command_line,omitempty"`
	// 规则和 MITRE 详情
	MitreName string `json:"mitre_name,omitempty"`
	RuleID    string `json:"rule_id,omitempty"`
	// 告警统计
	HitCount int `json:"hit_count"`
	// 阻断状态
	AutoBlocked   bool   `json:"auto_blocked"`
	ManualBlocked bool   `json:"manual_blocked"`
	BlockStatus   string `json:"block_status,omitempty"`
	BlockMessage  string `json:"block_message,omitempty"`
	// 历史 AI 分析
	LLMDisposalStrategy string    `json:"llm_disposal_strategy,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
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
	defaultAnalysisMaxIterations = 500
	analysisMaxIterationsLimit   = 500
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
	c.thinking = appendTraceText(c.thinking, content)
	c.pendingThought = appendTraceText(c.pendingThought, content)
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

func appendTraceText(current, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	return current + "\n" + next
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

// AddThinking implements adapters.EventCollector
func (c *SSEResponseCollector) AddThinking(text string) {
	c.WriteThinking(text)
}

// AddToolCall implements adapters.EventCollector
func (c *SSEResponseCollector) AddToolCall(tool, callID string, args interface{}) {
	c.WriteToolCall(tool, callID, args)
}

// AddToolResult implements adapters.EventCollector
func (c *SSEResponseCollector) AddToolResult(callID string, result interface{}) {
	c.toolResults = append(c.toolResults, map[string]interface{}{
		"call_id": callID,
		"result":  result,
	})
	c.finishCurrentStep(formatCollectedPayload(result))
}

// AddToolError implements adapters.EventCollector
func (c *SSEResponseCollector) AddToolError(callID, errMsg string) {
	c.toolResults = append(c.toolResults, map[string]interface{}{
		"call_id": callID,
		"error":   errMsg,
	})
	c.finishCurrentStep("Error: " + errMsg)
}

// SetContent implements adapters.EventCollector
func (c *SSEResponseCollector) SetContent(content string) {
	c.content = content
}

// SetPlan implements adapters.EventCollector (no-op for now, plan data not persisted in messages)
func (c *SSEResponseCollector) SetPlan(plan interface{}) {}

// AddAudit implements adapters.EventCollector (no-op for now)
func (c *SSEResponseCollector) AddAudit(audit interface{}) {}

// AddReflection implements adapters.EventCollector (no-op for now)
func (c *SSEResponseCollector) AddReflection(reflection interface{}) {}

// AddCorrection implements adapters.EventCollector (no-op for now)
func (c *SSEResponseCollector) AddCorrection(correction interface{}) {}

func toStringMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	if typed, ok := value.(string); ok {
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return nil
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(typed), &decoded); err == nil {
			return decoded
		}
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
		blockStatus := ""
		if alert.BlockStatus != nil {
			blockStatus = *alert.BlockStatus
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
			// 进程级别信息
			PID:         alert.PID,
			PPID:        alert.PPID,
			CommandLine: alert.CommandLine,
			// 规则和 MITRE 详情
			MitreName: alert.MitreName,
			RuleID:    alert.RuleID,
			// 告警统计
			HitCount: alert.HitCount,
			// 阻断状态
			AutoBlocked:   alert.AutoBlocked,
			ManualBlocked: alert.ManualBlocked,
			BlockStatus:   blockStatus,
			BlockMessage:  alert.BlockMessage,
			// 历史 AI 分析
			LLMDisposalStrategy: alert.LLMDisposalStrategy,
			CreatedAt:           alert.CreatedAt,
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

// isAllFalsePositive checks whether all conclusions in the AI analysis are false positives.
// Returns true only when every conclusion's action is "mark_false_positive".
func isAllFalsePositive(content string) bool {
	result, err := extractFinalAnswerResult(content)
	if err != nil || len(result.Conclusions) == 0 {
		return false
	}
	for _, c := range result.Conclusions {
		if c.Action != "mark_false_positive" {
			return false
		}
	}
	return true
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

type AnalysisControlResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	ActiveRun bool   `json:"active_run"`
	Message   string `json:"message"`
}

func NewAIAnalysisHandler(
	alertRepo *repository.AlertRepository,
	configRepo *repository.ConfigRepository,
	vectorService *service.VectorService,
	serverClient *grpc.ServerClient,
	sessionRepo *repository.AISessionRepository,
	messageRepo *repository.AIMessageRepository,
	agentExecRepo *repository.AgentExecutionRepository,
) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		alertRepo:     alertRepo,
		configRepo:    configRepo,
		vectorService: vectorService,
		serverClient:  serverClient,
		sessionRepo:   sessionRepo,
		messageRepo:   messageRepo,
		agentExecRepo: agentExecRepo,
		sessions:      make(map[string]*AISSESion),
		activeRuns:    make(map[string]activeAnalysisRun),
	}
}

func (h *AIAnalysisHandler) setActiveRun(sessionID string, cancel context.CancelFunc) string {
	h.activeRunsMu.Lock()
	defer h.activeRunsMu.Unlock()
	runID := uuid.New().String()
	h.activeRuns[sessionID] = activeAnalysisRun{
		id:     runID,
		cancel: cancel,
	}
	return runID
}

func (h *AIAnalysisHandler) clearActiveRun(sessionID, runID string) {
	h.activeRunsMu.Lock()
	defer h.activeRunsMu.Unlock()
	if current, ok := h.activeRuns[sessionID]; ok && current.id == runID {
		delete(h.activeRuns, sessionID)
	}
}

func (h *AIAnalysisHandler) popActiveRun(sessionID string) (context.CancelFunc, bool) {
	h.activeRunsMu.Lock()
	defer h.activeRunsMu.Unlock()
	run, ok := h.activeRuns[sessionID]
	if ok {
		delete(h.activeRuns, sessionID)
	}
	return run.cancel, ok
}

func (h *AIAnalysisHandler) hasActiveRun(sessionID string) bool {
	h.activeRunsMu.Lock()
	defer h.activeRunsMu.Unlock()
	_, ok := h.activeRuns[sessionID]
	return ok
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

	// Build context for the session
	alertCtx := h.buildSessionContext(session)

	logger.Info("StreamMessage: using agent-runtime", zap.String("session_id", sessionID))

	// Create experience provider
	var experienceProvider agentruntime.ExperienceProvider
	if h.vectorService != nil && h.agentExecRepo != nil {
		reflectionQuerier := adapters.NewReflectionQuerierAdapter(h.agentExecRepo)
		experienceProvider = adapters.NewExperienceProviderAdapter(h.vectorService, reflectionQuerier, 5)
	}

	// Create agent-runtime
	runtime, err := adapters.NewAegisRuntime(
		session.LLMClient,
		h.serverClient,
		sseWriter,
		responseCollector,
		session.HostIDs,
		alertCtx,
		session.MaxIterations,
		experienceProvider,
	)
	if err != nil {
		logger.Error("failed to create agent runtime", zap.Error(err), zap.String("session_id", sessionID))
		sseWriter.WriteError(fmt.Sprintf("failed to create agent runtime: %v", err))
		sseWriter.WriteDone()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	runID := h.setActiveRun(sessionID, cancel)
	defer h.clearActiveRun(sessionID, runID)

	// Start keepalive goroutine to prevent reverse proxy timeout during long LLM/tool calls.
	// SSE comments (lines starting with ":") are ignored by EventSource but keep the TCP connection active.
	keepaliveCtx, keepaliveCancel := context.WithCancel(ctx)
	defer keepaliveCancel()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-keepaliveCtx.Done():
				return
			case <-ticker.C:
				_ = sseWriter.WriteComment("keepalive")
			}
		}
	}()

	taskResult, err := runtime.Run(ctx, agentruntime.TaskInput{
		UserInput:   message,
		UserContext: alertCtx,
		Metadata:    map[string]string{"session_id": sessionID},
	})
	if err != nil {
		logger.Error("agent runtime error", zap.Error(err), zap.String("session_id", sessionID))
		sseWriter.WriteError(fmt.Sprintf("agent runtime error: %v", err))
		sseWriter.WriteDone()
		return
	}
	logger.Info("StreamMessage: agent-runtime.Run returned", zap.String("session_id", sessionID))

	// Persist agent execution results before deciding whether a final answer is safe
	// to show so history can replay failed, paused, or limited runs.
	if taskResult != nil && h.agentExecRepo != nil {
		h.persistAgentResult(sessionID, taskResult)
	}

	// Get the full AI response content
	aiResponseContent := responseCollector.GetContent()
	if aiResponseContent == "" && taskResult != nil {
		aiResponseContent = taskResult.FinalAnswer
		responseCollector.SetContent(aiResponseContent)
	}
	logger.Info("AI response collected", zap.String("session_id", sessionID), zap.Int("content_len", len(aiResponseContent)))

	if taskResult != nil && !isConclusiveTaskResult(taskResult) {
		errMsg := buildTaskResultFailureMessage(taskResult)
		logger.Warn("agent runtime task did not reach conclusive completion",
			zap.String("status", string(taskResult.Status)),
			zap.String("reason", string(taskResult.ExitReason)),
			zap.String("session_id", sessionID))
		h.persistStreamMessages(session, message, "AI 分析失败: "+errMsg, responseCollector)
		sseWriter.WriteError(errMsg)
		sseWriter.WriteDone()
		return
	}

	h.persistAnalysisOutcome(session, aiResponseContent)

	if aiResponseContent != "" {
		sseWriter.WriteContent(aiResponseContent)
	}

	// Generate flowchart image from final content (skip if all conclusions are false positives)
	if aiResponseContent != "" && !isAllFalsePositive(aiResponseContent) {
		_ = h.writeFlowchartImageEvent(c.Request.Context(), sseWriter, aiResponseContent)
	}

	// Signal the SSE stream is complete
	sseWriter.WriteDone()

	h.persistStreamMessages(session, message, aiResponseContent, responseCollector)
}

func isConclusiveTaskResult(result *agentruntime.TaskResult) bool {
	if result == nil {
		return true
	}
	if result.Status != agentruntime.StatusCompleted {
		return false
	}
	return !planHasUnfinishedSteps(result.FinalPlan)
}

func planHasUnfinishedSteps(plan *agentruntime.Plan) bool {
	if plan == nil {
		return false
	}
	for _, step := range plan.Steps {
		switch step.Status {
		case agentruntime.StepCompleted, agentruntime.StepFailed, agentruntime.StepSkipped, agentruntime.StepReplaced, agentruntime.StepInvalidated:
			continue
		default:
			return true
		}
	}
	return false
}

func buildTaskResultFailureMessage(result *agentruntime.TaskResult) string {
	if result == nil {
		return "AI 分析未返回执行结果"
	}
	if planHasUnfinishedSteps(result.FinalPlan) {
		return "AI 分析未完成全部执行计划，已停止输出结论。请缩小告警范围、补充上下文或稍后重试。"
	}
	if len(result.Errors) > 0 {
		return result.Errors[len(result.Errors)-1].Message
	}
	if result.ExitReason != "" {
		return fmt.Sprintf("执行状态为 %s，退出原因: %s", result.Status, result.ExitReason)
	}
	return fmt.Sprintf("执行状态为 %s，未生成可应用结论", result.Status)
}

func (h *AIAnalysisHandler) persistStreamMessages(session *AISSESion, userContent, assistantContent string, collector *SSEResponseCollector) {
	if session == nil {
		return
	}
	sessionID := session.SessionID
	if h.messageRepo != nil {
		userMsg := &model.AIMessage{
			SessionID: sessionID,
			MessageID: uuid.New().String(),
			Role:      "user",
			Content:   userContent,
		}
		if err := h.messageRepo.Create(userMsg); err != nil {
			logger.Warn("failed to persist user message", zap.Error(err))
		} else if h.sessionRepo != nil {
			h.sessionRepo.IncrementMessageCount(sessionID)
		}

		if collector != nil && (collector.HasAssistantTrace() || assistantContent != "") {
			aiMsg := &model.AIMessage{
				SessionID:   sessionID,
				MessageID:   uuid.New().String(),
				Role:        "assistant",
				Content:     assistantContent,
				Thinking:    collector.GetThinking(),
				ToolCalls:   collector.ToolCallsJSONB(),
				ToolResults: collector.ToolResultsJSONB(),
				Steps:       collector.StepsJSONB(),
			}
			if err := h.messageRepo.Create(aiMsg); err != nil {
				logger.Warn("failed to persist AI response", zap.Error(err))
			} else {
				logger.Info("persisted AI response", zap.String("session_id", sessionID), zap.Int("content_len", len(assistantContent)))
				if h.sessionRepo != nil {
					h.sessionRepo.IncrementMessageCount(sessionID)
					for range collector.GetToolCalls() {
						h.sessionRepo.IncrementToolCallCount(sessionID)
					}
				}
			}
		}
	}

	session.Messages = append(session.Messages, &llm.AIMessage{
		Role:    "user",
		Content: userContent,
	})
	if assistantContent != "" {
		session.Messages = append(session.Messages, &llm.AIMessage{
			Role:    "assistant",
			Content: assistantContent,
		})
	}
}

func (h *AIAnalysisHandler) persistAnalysisOutcome(session *AISSESion, finalContent string) {
	if session == nil {
		return
	}

	result, err := extractFinalAnswerResult(finalContent)
	if err != nil {
		// Fallback: use parseConclusionFromAnswer to extract verdict via keyword matching
		logger.Info("structured JSON parse failed, falling back to keyword-based conclusion",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
		conclusionMap := parseConclusionFromAnswer(finalContent)
		session.Status = "completed"
		if h.sessionRepo != nil {
			conclusionJSON := model.JSONB(conclusionMap)
			if updateErr := h.sessionRepo.UpdateConclusion(session.SessionID, conclusionJSON); updateErr != nil {
				logger.Warn("failed to persist fallback AI session conclusion",
					zap.String("session_id", session.SessionID),
					zap.Error(updateErr))
			}
		}
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

func (h *AIAnalysisHandler) persistAgentResult(sessionID string, result *agentruntime.TaskResult) {
	if h.agentExecRepo == nil {
		return
	}

	// 1. Save execution record
	// Merge Metrics with ContextBudget and CompressionRecords into a single JSONB
	metricsMap := make(map[string]interface{})
	if metricsData, err := json.Marshal(result.Metrics); err == nil {
		_ = json.Unmarshal(metricsData, &metricsMap)
	}
	if result.ContextBudget != nil {
		metricsMap["context_budget"] = result.ContextBudget
	}
	if len(result.CompressionRecords) > 0 {
		metricsMap["compression_records"] = result.CompressionRecords
	}
	metricsJSONB := model.JSONB(metricsMap)

	exec := &model.AgentExecution{
		SessionID:       sessionID,
		TaskID:          result.TaskID,
		Status:          string(result.Status),
		ExitReason:      string(result.ExitReason),
		FinalAnswer:     result.FinalAnswer,
		InitialPlan:     marshalJSONB(result.InitialPlan),
		FinalPlan:       marshalJSONB(result.FinalPlan),
		Completion:      marshalJSONB(result.Completion),
		Metrics:         metricsJSONB,
		StartedAt:       result.StartedAt,
		EndedAt:         result.EndedAt,
		TotalDurationMs: result.Metrics.TotalDuration.Milliseconds(),
	}
	if err := h.agentExecRepo.CreateExecution(exec); err != nil {
		logger.Warn("failed to persist agent execution", zap.Error(err))
		return
	}

	// 2. Save step executions
	for _, step := range result.StepExecutions {
		h.agentExecRepo.CreateStepExecution(&model.AgentStepExecution{
			ExecutionID: exec.ID,
			TaskID:      result.TaskID,
			StepID:      step.StepID,
			Attempt:     step.Attempt,
			Status:      string(step.Status),
			Result:      step.Result,
			Evidence:    marshalJSONB(step.Evidence),
			Error:       marshalJSONB(step.Error),
			ReactTurns:  marshalJSONB(step.ReactTurns),
			StartedAt:   step.StartedAt,
			EndedAt:     step.EndedAt,
			DurationMs:  step.EndedAt.Sub(step.StartedAt).Milliseconds(),
		})
	}

	// 3. Save reflections
	for _, refl := range result.Reflections {
		h.agentExecRepo.CreateReflection(&model.AgentReflection{
			ExecutionID:    exec.ID,
			TaskID:         result.TaskID,
			ReflectionID:   refl.ReflectionID,
			Trigger:        refl.Trigger,
			RootCause:      refl.RootCause,
			Impact:         refl.Impact,
			Recoverable:    refl.Recoverable,
			Recommendation: string(refl.Recommendation),
			DisableTools:   marshalJSONB(refl.DisableTools),
			CorrectionHint: refl.CorrectionHint,
			ReusableLesson: refl.ReusableLesson,
			CreatedAt:      refl.CreatedAt,
		})
	}

	// 4. Save audits
	for _, aud := range result.Audits {
		h.agentExecRepo.CreateAudit(&model.AgentAudit{
			ExecutionID:    exec.ID,
			TaskID:         result.TaskID,
			AuditID:        aud.AuditID,
			Trigger:        aud.Trigger,
			Drifted:        aud.Drifted,
			RiskLevel:      string(aud.RiskLevel),
			Findings:       marshalJSONB(aud.Findings),
			Decision:       string(aud.Decision),
			CorrectionHint: aud.CorrectionHint,
			ShouldExit:     aud.ShouldExit,
			ExitReason:     string(aud.ExitReason),
			CreatedAt:      aud.CreatedAt,
		})
	}

	// 5. Save corrections
	for _, corr := range result.Corrections {
		h.agentExecRepo.CreateCorrection(&model.AgentCorrection{
			ExecutionID:      exec.ID,
			TaskID:           result.TaskID,
			CorrectionID:     corr.CorrectionID,
			Trigger:          corr.Trigger,
			FromPlanVersion:  corr.FromPlanVersion,
			ToPlanVersion:    corr.ToPlanVersion,
			Reason:           corr.Reason,
			Actions:          marshalJSONB(corr.Actions),
			Valid:            corr.Valid,
			ValidationErrors: marshalJSONB(corr.ValidationErrors),
			CreatedAt:        corr.CreatedAt,
		})
	}

	// 6. Save tool calls
	for _, tc := range result.ToolCalls {
		h.agentExecRepo.CreateToolCall(&model.AgentToolCallRecord{
			ExecutionID:   exec.ID,
			TaskID:        result.TaskID,
			StepID:        tc.StepID,
			CallID:        tc.CallID,
			ToolName:      tc.ToolName,
			Reason:        tc.Reason,
			ArgsSummary:   tc.ArgsSummary,
			Status:        string(tc.Status),
			ResultSummary: tc.ResultSummary,
			ErrorMessage:  tc.ErrorMessage,
			RiskLevel:     string(tc.RiskLevel),
			DurationMs:    tc.EndedAt.Sub(tc.StartedAt).Milliseconds(),
			StartedAt:     tc.StartedAt,
			EndedAt:       tc.EndedAt,
		})
	}

	// 7. Save model errors
	for _, mc := range result.ModelCalls {
		if mc.Error != "" {
			h.agentExecRepo.CreateModelError(&model.AgentModelError{
				ExecutionID: exec.ID,
				TaskID:      result.TaskID,
				StepID:      mc.StepID,
				CallID:      mc.CallID,
				Purpose:     string(mc.Purpose),
				Message:     mc.Error,
				Model:       mc.Model,
				TokensUsed:  mc.TokensUsed,
				LatencyMs:   mc.Latency.Milliseconds(),
				OccurredAt:  mc.OccurredAt,
			})
		}
	}

	logger.Info("persisted agent execution results",
		zap.String("session_id", sessionID),
		zap.String("task_id", result.TaskID),
		zap.String("status", string(result.Status)))

	// Save RAG record for future experience retrieval (async)
	go h.saveAnalysisRecordForRAG(sessionID, result)
}

// saveAnalysisRecordForRAG builds an analysis summary from the task result and
// saves it with a vector embedding for future similarity search.
func (h *AIAnalysisHandler) saveAnalysisRecordForRAG(sessionID string, result *agentruntime.TaskResult) {
	if h.vectorService == nil {
		return
	}

	h.sessionsMutex.RLock()
	session, exists := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()
	if !exists {
		return
	}

	summary := buildAnalysisSummary(result)

	record := &service.AIAnalysisRecord{
		ID:           uuid.New().String(),
		SessionID:    sessionID,
		AlertIDs:     marshalJSONString(session.AlertIDs),
		HostFilter:   marshalJSONString(session.HostFilter),
		InitialQuery: session.InitialQuery,
		Summary:      summary,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := h.vectorService.GenerateAndSaveEmbedding(ctx, record); err != nil {
		logger.Warn("failed to save RAG record for experience",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// buildAnalysisSummary extracts key information from a TaskResult into a
// human-readable summary string for embedding generation.
func buildAnalysisSummary(result *agentruntime.TaskResult) string {
	var b strings.Builder
	if result.InitialPlan != nil {
		b.WriteString("目标: ")
		b.WriteString(result.InitialPlan.Goal)
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("完成: %d/%d 步骤\n", result.Completion.CompletedSteps, len(result.StepExecutions)))
	b.WriteString(fmt.Sprintf("工具调用: %d次, 模型调用: %d次\n", result.Completion.ToolCalls, result.Completion.ModelCalls))
	if result.Metrics.TotalPromptTokens > 0 || result.Metrics.TotalCompletionTokens > 0 {
		b.WriteString(fmt.Sprintf("Token: prompt=%d, completion=%d\n", result.Metrics.TotalPromptTokens, result.Metrics.TotalCompletionTokens))
	}
	if len(result.CompressionRecords) > 0 {
		b.WriteString(fmt.Sprintf("上下文压缩: %d次\n", len(result.CompressionRecords)))
	}
	if len(result.Reflections) > 0 {
		b.WriteString(fmt.Sprintf("反思: %d次\n", len(result.Reflections)))
	}
	if len(result.Audits) > 0 {
		b.WriteString(fmt.Sprintf("审计: %d次\n", len(result.Audits)))
	}
	return b.String()
}

func marshalJSONString(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func marshalJSONB(v interface{}) model.JSONB {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var result model.JSONB
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
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

// loadExecutionPlan loads the execution plan from agent execution records for a session.
func (h *AIAnalysisHandler) loadExecutionPlan(sessionID string) interface{} {
	if h.agentExecRepo == nil {
		return nil
	}
	exec, err := h.agentExecRepo.FindBySessionID(sessionID)
	if err != nil || exec == nil {
		return nil
	}
	if exec.FinalPlan != nil {
		return exec.FinalPlan
	}
	return exec.InitialPlan
}

type executionHistoryArtifacts struct {
	ExecutionPlan interface{}
	Audits        interface{}
	Reflections   interface{}
	Corrections   interface{}
}

func (h *AIAnalysisHandler) loadExecutionHistoryArtifacts(sessionID string) executionHistoryArtifacts {
	if h.agentExecRepo == nil {
		return executionHistoryArtifacts{}
	}
	exec, err := h.agentExecRepo.FindBySessionID(sessionID)
	if err != nil || exec == nil {
		return executionHistoryArtifacts{}
	}

	artifacts := executionHistoryArtifacts{ExecutionPlan: exec.InitialPlan}
	if exec.FinalPlan != nil {
		artifacts.ExecutionPlan = exec.FinalPlan
	}
	if audits, err := h.agentExecRepo.FindAuditsByExecutionID(exec.ID); err == nil {
		artifacts.Audits = audits
	}
	if reflections, err := h.agentExecRepo.FindReflectionsByExecutionID(exec.ID); err == nil {
		artifacts.Reflections = reflections
	}
	if corrections, err := h.agentExecRepo.FindCorrectionsByExecutionID(exec.ID); err == nil {
		artifacts.Corrections = corrections
	}
	return artifacts
}

// GetSessionHistory gets the message history for a session
// GET /api/v1/detection/alerts/ai-analysis/{session_id}/history
func (h *AIAnalysisHandler) GetSessionHistory(c *gin.Context) {
	sessionID := c.Param("session_id")

	// 获取会话信息（用于状态、结论和告警快照）
	var displayStatus string
	var conclusion model.JSONB
	var alertSnapshots []AlertContextSnapshot
	if h.sessionRepo != nil {
		session, err := h.sessionRepo.FindBySessionID(sessionID)
		if err == nil {
			displayStatus = repository.GetDisplayStatus(session)
			conclusion = session.Conclusion
			// 加载会话关联的告警快照
			if h.alertRepo != nil && len(session.AlertIDs) > 0 {
				alerts, err := h.alertRepo.FindByIDs(session.AlertIDs)
				if err != nil {
					logger.Warn("failed to load alerts for session history", zap.Error(err), zap.Strings("alert_ids", session.AlertIDs))
				} else {
					alertSnapshots = buildAlertSnapshots(alerts)
				}
			}
		}
	}

	// Try to read from database first
	if h.messageRepo != nil {
		messages, err := h.messageRepo.FindBySessionID(sessionID)
		if err == nil && len(messages) > 0 {
			artifacts := h.loadExecutionHistoryArtifacts(sessionID)
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"session_id":     sessionID,
					"messages":       messages,
					"execution_plan": artifacts.ExecutionPlan,
					"audits":         artifacts.Audits,
					"reflections":    artifacts.Reflections,
					"corrections":    artifacts.Corrections,
					"status":         displayStatus,
					"conclusion":     conclusion,
					"alerts":         alertSnapshots,
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

	artifacts := h.loadExecutionHistoryArtifacts(sessionID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"session_id":     sessionID,
			"messages":       session.Messages,
			"execution_plan": artifacts.ExecutionPlan,
			"audits":         artifacts.Audits,
			"reflections":    artifacts.Reflections,
			"corrections":    artifacts.Corrections,
			"status":         displayStatus,
			"conclusion":     conclusion,
			"alerts":         session.AlertSnapshots,
		},
	})
}

// GetExecutionResult gets the structured execution result for a session
// GET /api/v1/detection/alerts/ai-analysis/{session_id}/execution-result
func (h *AIAnalysisHandler) GetExecutionResult(c *gin.Context) {
	sessionID := c.Param("session_id")

	if h.agentExecRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "执行记录服务不可用"})
		return
	}

	exec, err := h.agentExecRepo.FindBySessionID(sessionID)
	if err != nil || exec == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "执行记录不存在"})
		return
	}

	steps, err := h.agentExecRepo.FindStepsByExecutionID(exec.ID)
	if err != nil {
		steps = []*model.AgentStepExecution{}
	}

	response := buildExecutionResultResponse(exec, steps)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func buildExecutionResultResponse(exec *model.AgentExecution, steps []*model.AgentStepExecution) map[string]interface{} {
	statusMap := map[string]string{
		"completed":   "已完成",
		"failed":      "失败",
		"interrupted": "已中断",
		"limited":     "已限制",
		"running":     "执行中",
		"pending":     "等待中",
	}

	exitReasonMap := map[string]string{
		"normal_completed":  "正常完成",
		"max_iterations":    "达到最大轮次",
		"timeout":           "执行超时",
		"user_cancelled":    "用户取消",
		"error":             "执行错误",
		"audit_rejected":    "审计拒绝",
		"drift_detected":    "检测到计划漂移",
		"rate_limit":        "速率限制",
		"context_overflow":  "上下文窗口溢出",
	}

	statusDisplay := exec.Status
	if v, ok := statusMap[exec.Status]; ok {
		statusDisplay = v
	}

	exitReasonDisplay := exec.ExitReason
	if v, ok := exitReasonMap[exec.ExitReason]; ok {
		exitReasonDisplay = v
	}

	stepResponses := make([]map[string]interface{}, 0, len(steps))
	for _, step := range steps {
		stepStatus := step.Status
		if v, ok := statusMap[step.Status]; ok {
			stepStatus = v
		}
		stepResponses = append(stepResponses, map[string]interface{}{
			"step_id":     step.StepID,
			"status":      stepStatus,
			"result":      step.Result,
			"started_at":  step.StartedAt,
			"ended_at":    step.EndedAt,
			"duration_ms": step.DurationMs,
		})
	}

	conclusion := parseConclusionFromAnswer(exec.FinalAnswer)

	// Extract context budget and token metrics from stored metrics
	var contextBudget interface{}
	var compressionRecords interface{}
	var totalPromptTokens, totalCompletionTokens int
	if exec.Metrics != nil {
		if cb, ok := exec.Metrics["context_budget"]; ok {
			contextBudget = cb
		}
		if cr, ok := exec.Metrics["compression_records"]; ok {
			compressionRecords = cr
		}
		totalPromptTokens = jsonbInt(exec.Metrics["total_prompt_tokens"])
		totalCompletionTokens = jsonbInt(exec.Metrics["total_completion_tokens"])
	}

	return map[string]interface{}{
		"execution_id":          exec.ID.String(),
		"task_id":               exec.TaskID,
		"session_id":            exec.SessionID,
		"status":                statusDisplay,
		"exit_reason":           exitReasonDisplay,
		"started_at":            exec.StartedAt,
		"ended_at":              exec.EndedAt,
		"total_duration_ms":     exec.TotalDurationMs,
		"steps":                 stepResponses,
		"errors":                extractErrorsFromExecution(exec),
		"conclusion":            conclusion,
		"context_budget":        contextBudget,
		"compression_records":   compressionRecords,
		"total_prompt_tokens":   totalPromptTokens,
		"total_completion_tokens": totalCompletionTokens,
	}
}

func parseConclusionFromAnswer(finalAnswer string) map[string]interface{} {
	if finalAnswer == "" {
		return map[string]interface{}{
			"verdict":   "unknown",
			"summary":   "未生成结论",
			"reasoning": "",
		}
	}

	// Step 1: Try structured JSON extraction (handles LLM's summarizePromptTemplate output)
	if result, err := extractFinalAnswerResult(finalAnswer); err == nil && len(result.Conclusions) > 0 {
		return buildVerdictFromConclusions(result, finalAnswer)
	}

	// Step 2: Fallback to keyword matching (English + Chinese, deterministic with severity priority)
	type keywordRule struct {
		keyword  string
		verdict  string
		severity int
	}
	keywordRules := []keywordRule{
		{"Malicious", "malicious", 2}, {"恶意", "malicious", 2}, {"Threat", "malicious", 2},
		{"Suspicious", "suspicious", 1}, {"可疑", "suspicious", 1},
		{"Benign", "benign", 0}, {"False Positive", "benign", 0}, {"误报", "benign", 0}, {"良性", "benign", 0},
	}

	verdict := "unknown"
	summary := finalAnswer
	worstSeverity := -1
	for _, rule := range keywordRules {
		if strings.Contains(finalAnswer, rule.keyword) && rule.severity > worstSeverity {
			verdict = rule.verdict
			worstSeverity = rule.severity
		}
	}

	verdictDisplayMap := map[string]string{
		"benign":    "良性/误报",
		"malicious": "恶意",
		"suspicious": "可疑",
		"unknown":   "未知",
	}

	if v, ok := verdictDisplayMap[verdict]; ok {
		summary = v
	}

	// Step 3: If still unknown but has content, use truncated text as summary
	if verdict == "unknown" && len(finalAnswer) > 0 {
		runes := []rune(finalAnswer)
		if len(runes) > 200 {
			summary = string(runes[:200]) + "..."
		} else {
			summary = finalAnswer
		}
	}

	return map[string]interface{}{
		"verdict":   verdict,
		"summary":   summary,
		"reasoning": finalAnswer,
	}
}

// buildVerdictFromConclusions maps structured JSON conclusions to a verdict.
// When multiple conclusions exist, the most severe verdict is used:
// malicious > suspicious > benign.
func buildVerdictFromConclusions(result *finalAnswerResult, originalText string) map[string]interface{} {
	actionVerdictMap := map[string]string{
		"mark_false_positive": "benign",
		"confirm_threat":      "malicious",
		"generate_rule":       "suspicious",
	}

	severityOrder := map[string]int{
		"benign":    0,
		"suspicious": 1,
		"malicious": 2,
	}

	worstVerdict := "unknown"
	worstSeverity := -1
	summary := ""

	for _, c := range result.Conclusions {
		v, ok := actionVerdictMap[c.Action]
		if !ok {
			continue
		}
		if severityOrder[v] > worstSeverity {
			worstSeverity = severityOrder[v]
			worstVerdict = v
		}
		if summary == "" && c.Summary != "" {
			summary = c.Summary
		}
	}

	if summary == "" {
		summary = attackGraphStringField(result.AttackGraph, "summary")
	}

	return map[string]interface{}{
		"verdict":   worstVerdict,
		"summary":   summary,
		"reasoning": originalText,
	}
}

func extractErrorsFromExecution(exec *model.AgentExecution) []string {
	errors := []string{}

	if exec.Completion != nil {
		if errMsgs, ok := exec.Completion["errors"].([]interface{}); ok {
			for _, e := range errMsgs {
				if s, ok := e.(string); ok {
					errors = append(errors, s)
				}
			}
		}
	}

	if exec.Metrics != nil {
		if errMsgs, ok := exec.Metrics["errors"].([]interface{}); ok {
			for _, e := range errMsgs {
				if s, ok := e.(string); ok {
					errors = append(errors, s)
				}
			}
		}
	}

	return errors
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

	// 转换状态为显示状态：只有conclusion不为空才是completed
	for _, session := range sessions {
		session.Status = repository.GetDisplayStatus(session)
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

// PauseAnalysis requests cooperative cancellation of the current stream run while
// preserving the session for a follow-up user message.
// POST /api/v1/detection/alerts/ai-analysis/{session_id}/pause
func (h *AIAnalysisHandler) PauseAnalysis(c *gin.Context) {
	h.stopActiveAnalysis(c, "paused", "AI 分析已暂停")
}

// CancelAnalysis cancels the current stream run and marks the session cancelled.
// POST /api/v1/detection/alerts/ai-analysis/{session_id}/cancel
func (h *AIAnalysisHandler) CancelAnalysis(c *gin.Context) {
	h.stopActiveAnalysis(c, "cancelled", "AI 分析已取消")
}

func (h *AIAnalysisHandler) stopActiveAnalysis(c *gin.Context, status, message string) {
	sessionID := c.Param("session_id")
	cancel, ok := h.popActiveRun(sessionID)
	if ok {
		cancel()
	}

	h.sessionsMutex.Lock()
	if session, exists := h.sessions[sessionID]; exists {
		session.Status = status
	}
	h.sessionsMutex.Unlock()

	if h.sessionRepo != nil {
		if err := h.sessionRepo.UpdateStatus(sessionID, status); err != nil {
			logger.Warn("failed to update AI analysis session status",
				zap.String("session_id", sessionID),
				zap.String("status", status),
				zap.Error(err))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": AnalysisControlResponse{
			SessionID: sessionID,
			Status:    status,
			ActiveRun: ok,
			Message:   message,
		},
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
		if !ok || isPlaceholderToolValue(hostID) {
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

// jsonbInt safely extracts an int from a JSONB value that may be float64, int, int64, or json.Number.
func jsonbInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

func isPlaceholderToolValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	switch strings.ToLower(value) {
	case "...", "your_host_id", "host_id", "<host_id>", "[host_id]", "[the host id]":
		return true
	default:
		return strings.HasPrefix(value, "[") || strings.HasPrefix(value, "<")
	}
}
