package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// Service 智能体服务
type Service struct {
	sessionRepo   repository.AssistantSessionRepository
	messageRepo   repository.AssistantMessageRepository
	contextRepo   repository.AssistantContextRefRepository
	toolCallRepo  repository.AssistantToolCallRepository
	approvalRepo  repository.AssistantApprovalRepository
	memoryRepo    repository.AssistantMemoryRepository
	contextLoader *ContextLoader
	orchestrator  *Orchestrator
	runManager    *RunManager
	logger        *zap.Logger
}

// ServiceDeps 服务依赖
type ServiceDeps struct {
	SessionRepo    repository.AssistantSessionRepository
	MessageRepo    repository.AssistantMessageRepository
	ContextRefRepo repository.AssistantContextRefRepository
	ToolCallRepo   repository.AssistantToolCallRepository
	ApprovalRepo   repository.AssistantApprovalRepository
	MemoryRepo     repository.AssistantMemoryRepository
	ContextLoader  *ContextLoader
	Orchestrator   *Orchestrator
	RunManager     *RunManager
	Logger         *zap.Logger
}

// NewService 创建智能体服务
func NewService(deps ServiceDeps) *Service {
	return &Service{
		sessionRepo:   deps.SessionRepo,
		messageRepo:   deps.MessageRepo,
		contextRepo:   deps.ContextRefRepo,
		toolCallRepo:  deps.ToolCallRepo,
		approvalRepo:  deps.ApprovalRepo,
		memoryRepo:    deps.MemoryRepo,
		contextLoader: deps.ContextLoader,
		orchestrator:  deps.Orchestrator,
		runManager:    deps.RunManager,
		logger:        deps.Logger,
	}
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	Title          string            `json:"title"`
	TaskType       string            `json:"task_type"`
	InitialMessage string            `json:"initial_message"`
	ContextRefs    []ContextRefInput `json:"context_refs"`
	Locale         string            `json:"locale,omitempty"`
}

// ContextRefInput 上下文引用输入
type ContextRefInput struct {
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	// Title and Summary are populated from server-side session context before
	// intent classification. AttachContextRefs deliberately ignores
	// client-supplied values so callers cannot forge trusted object metadata.
	Title   string `json:"title,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// SendMessageRequest 发送消息请求
type SendMessageRequest struct {
	Content     string            `json:"content"`
	ContextRefs []ContextRefInput `json:"context_refs"`
	Locale      string            `json:"locale,omitempty"`
}

// RunHandle 运行句柄
type RunHandle struct {
	RunID     string `json:"run_id"`
	MessageID string `json:"message_id"`
}

// SessionQuery 会话查询参数
type SessionQuery = repository.SessionQuery

// ListSessions 列出会话
func (s *Service) ListSessions(ctx context.Context, q SessionQuery) ([]model.AssistantSession, int64, error) {
	return s.sessionRepo.List(ctx, q)
}

// DeleteSession 删除会话
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	return s.sessionRepo.Delete(ctx, sessionID)
}

// CreateSession 创建会话
func (s *Service) CreateSession(ctx context.Context, req CreateSessionRequest, operator string) (*model.AssistantSession, error) {
	sessionID := newAssistantSessionID()
	title := req.Title
	if title == "" {
		title = inferTitle(req.InitialMessage, req.ContextRefs)
	}

	metadata, _ := json.Marshal(map[string]interface{}{"locale": NormalizeLocale(req.Locale)})
	session := &model.AssistantSession{
		ID:         uuid.New(),
		SessionID:  sessionID,
		Title:      title,
		TaskType:   defaultTaskType(req.TaskType),
		ModeSource: inferModeSource(req.ContextRefs),
		Status:     model.SessionStatusActive,
		CreatedBy:  operator,
		Metadata:   datatypes.JSON(metadata),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Attach context refs
	if len(req.ContextRefs) > 0 {
		if _, err := s.AttachContextRefs(ctx, sessionID, req.ContextRefs); err != nil {
			s.logger.Warn("failed to attach context refs", zap.Error(err))
		}
	}

	// Create initial message if provided
	if strings.TrimSpace(req.InitialMessage) != "" {
		msg := &model.AssistantMessage{
			ID:        uuid.New(),
			SessionID: sessionID,
			MessageID: newMessageID(),
			Role:      "user",
			Content:   req.InitialMessage,
		}
		if err := s.messageRepo.Create(ctx, msg); err != nil {
			s.logger.Warn("failed to create initial message", zap.Error(err))
		} else {
			_ = s.sessionRepo.IncrementMessageCount(ctx, sessionID)
		}
	}

	return session, nil
}

// GetSession 获取会话
func (s *Service) GetSession(ctx context.Context, sessionID string) (*model.AssistantSession, error) {
	return s.sessionRepo.FindBySessionID(ctx, sessionID)
}

// GetMessages 获取消息历史
func (s *Service) GetMessages(ctx context.Context, sessionID string) ([]model.AssistantMessage, error) {
	return s.messageRepo.ListBySession(ctx, sessionID, 0)
}

// AttachContextRefs 附加上下文引用
func (s *Service) AttachContextRefs(ctx context.Context, sessionID string, refs []ContextRefInput) ([]model.AssistantContextRef, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	var contextRefs []model.AssistantContextRef
	for _, ref := range refs {
		cr := model.AssistantContextRef{
			ID:         uuid.New(),
			SessionID:  sessionID,
			ObjectType: ref.ObjectType,
			ObjectID:   ref.ObjectID,
		}

		// Try to resolve context object
		if s.contextLoader != nil {
			obj, err := s.contextLoader.Resolve(ctx, ref.ObjectType, ref.ObjectID)
			if err == nil && obj != nil {
				cr.Title = obj.Title
				cr.Summary = obj.Summary
				cr.RoutePath = obj.RoutePath
			}
		}

		contextRefs = append(contextRefs, cr)
	}

	if err := s.contextRepo.UpsertMany(ctx, contextRefs); err != nil {
		return nil, fmt.Errorf("failed to attach context refs: %w", err)
	}

	return contextRefs, nil
}

// SendMessage 发送消息并启动运行
func (s *Service) SendMessage(ctx context.Context, sessionID string, req SendMessageRequest, operator string) (*RunHandle, error) {
	session, err := s.sessionRepo.FindBySessionID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if session.Status == model.SessionStatusRunning {
		return nil, fmt.Errorf("session is already running")
	}
	req.Locale = NormalizeLocale(req.Locale)
	metadata := unmarshalJSON(session.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["locale"] = req.Locale
	session.Metadata = mustMarshalJSON(metadata)
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		s.logger.Warn("failed to persist assistant run locale",
			zap.String("session_id", sessionID),
			zap.String("locale", req.Locale),
			zap.Error(err),
		)
	}

	// Attach new context refs if provided
	if len(req.ContextRefs) > 0 {
		if _, err := s.AttachContextRefs(ctx, sessionID, req.ContextRefs); err != nil {
			s.logger.Warn("failed to attach context refs", zap.Error(err))
		}
	}

	// Create user message
	userMsg := &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		MessageID: newMessageID(),
		Role:      "user",
		Content:   req.Content,
	}
	if err := s.messageRepo.Create(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}
	_ = s.sessionRepo.IncrementMessageCount(ctx, sessionID)

	// Start run
	run := s.runManager.Start(sessionID)
	_ = s.sessionRepo.UpdateStatus(ctx, sessionID, model.SessionStatusRunning)

	// Run orchestrator in background
	go func() {
		// 防止 panic 导致 session 永远卡在 running 状态
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("orchestrator panic recovered",
					zap.String("session_id", sessionID),
					zap.String("run_id", run.RunID),
					zap.Any("panic", r),
				)
				s.completeRun(context.Background(), sessionID, run.RunID, nil,
					fmt.Errorf("internal error: orchestrator panic: %v", r))
			}
		}()

		runCtx := run.Context()

		// 加载上下文引用，传递给 Orchestrator（对齐设计文档 6 节 RunInput）
		var contextRefs []model.AssistantContextRef
		if s.contextLoader != nil {
			refs, _ := s.contextLoader.ResolveSession(runCtx, sessionID)
			for _, ref := range refs {
				contextRefs = append(contextRefs, model.AssistantContextRef{
					SessionID:  sessionID,
					ObjectType: ref.ObjectType,
					ObjectID:   ref.ObjectID,
					Title:      ref.Title,
					Summary:    ref.Summary,
					RoutePath:  ref.RoutePath,
					Snapshot:   mustMarshalJSON(ref.Data),
				})
			}
		}
		result, err := s.orchestrator.Run(runCtx, RunInput{
			RunID:       run.RunID,
			SessionID:   sessionID,
			MessageID:   userMsg.MessageID,
			UserID:      operator,
			UserMessage: req.Content,
			TaskType:    session.TaskType,
			ContextRefs: contextRefs,
			Locale:      req.Locale,
		})
		s.completeRun(context.Background(), sessionID, run.RunID, result, err)
	}()

	return &RunHandle{
		RunID:     run.RunID,
		MessageID: userMsg.MessageID,
	}, nil
}

// Stream 流式输出事件
func (s *Service) Stream(ctx context.Context, sessionID string, writer EventWriter) error {
	ch, unsubscribe, err := s.runManager.Subscribe(sessionID)
	if err != nil {
		return err
	}
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := writer.Write(event); err != nil {
				return err
			}
			if event.Type == EventDone || event.Type == EventError {
				return nil
			}
		}
	}
}

// CancelRun 取消运行
func (s *Service) CancelRun(ctx context.Context, sessionID string, operator string) error {
	if !s.runManager.IsRunning(sessionID) {
		return fmt.Errorf("no active run for session")
	}

	s.runManager.Cancel(sessionID)
	_ = s.sessionRepo.UpdateStatus(ctx, sessionID, model.SessionStatusCancelled)
	return nil
}

// CompleteRun 完成运行
func (s *Service) completeRun(ctx context.Context, sessionID, runID string, result *RunResult, err error) {
	locale := s.sessionLocale(ctx, sessionID)
	if errors.Is(err, context.Canceled) {
		if run, ok := s.runManager.Get(sessionID); ok {
			if decision := run.RejectedApproval(); decision != nil {
				msg := localized(locale, "审批已拒绝，运行已停止", "Approval was rejected and the run has stopped")
				if decision.Comment != "" {
					msg += ": " + decision.Comment
				}
				s.logger.Info("run stopped after approval rejection",
					zap.String("session_id", sessionID),
					zap.String("run_id", runID),
					zap.String("approval_id", decision.ApprovalID),
				)
				_ = s.sessionRepo.UpdateStatus(ctx, sessionID, model.SessionStatusFailed)
				s.persistRunOutcomeMetadata(ctx, sessionID, "approval_rejected", agentruntime.GoalFailed)
				s.runManager.Publish(sessionID, EventErrorPayload(sessionID, runID, msg))
				_ = s.messageRepo.Create(context.Background(), &model.AssistantMessage{
					ID:        uuid.New(),
					SessionID: sessionID,
					MessageID: "msg_" + runID,
					Role:      "assistant",
					Content:   msg,
					Thinking:  s.extractThinkingFromHistory(sessionID),
					Plan:      s.extractPlanFromHistory(sessionID),
				})
				s.runManager.Finish(sessionID)
				return
			}
		}
		s.logger.Info("run cancelled", zap.String("session_id", sessionID), zap.String("run_id", runID))
		_ = s.sessionRepo.UpdateStatus(ctx, sessionID, model.SessionStatusCancelled)
		s.persistRunOutcomeMetadata(ctx, sessionID, "cancelled", agentruntime.GoalFailed)
		s.runManager.Publish(sessionID, EventErrorPayload(sessionID, runID, localized(locale, "任务已取消", "Task cancelled")))
		s.runManager.Finish(sessionID)
		return
	}

	if err != nil {
		s.logger.Error("run failed", zap.String("session_id", sessionID), zap.Error(err))
		_ = s.sessionRepo.UpdateStatus(ctx, sessionID, model.SessionStatusFailed)
		s.persistRunOutcomeMetadata(ctx, sessionID, "failed", agentruntime.GoalFailed)

		// 错误时也保存一条助手消息，避免用户看到空白
		errMsg := fmt.Sprintf(localized(locale, "抱歉，执行过程中出现错误: %s", "Sorry, an error occurred during execution: %s"), err.Error())
		msgID := "msg_" + runID

		// 从事件历史中提取 thinking 和 plan 数据
		thinkingContent := s.extractThinkingFromHistory(sessionID)
		planData := s.extractPlanFromHistory(sessionID)

		// 使用 context.Background() 保存消息，避免上下文取消导致保存失败
		saveCtx := context.Background()
		_ = s.messageRepo.Create(saveCtx, &model.AssistantMessage{
			ID:        uuid.New(),
			SessionID: sessionID,
			MessageID: msgID,
			Role:      "assistant",
			Content:   errMsg,
			Thinking:  thinkingContent,
			Plan:      planData,
		})

		// 先推送错误消息内容，再推送错误事件：Stream 收到 EventError 会立即结束，
		// 若顺序颠倒，前端将只收到失败状态而收不到错误正文，只能刷新后才从库里加载。
		s.runManager.Publish(sessionID, EventMessageDeltaPayload(sessionID, runID, msgID, errMsg))
		s.runManager.Publish(sessionID, EventErrorPayload(sessionID, runID, err.Error()))
	} else {
		if result == nil {
			s.logger.Error("assistant run returned no result",
				zap.String("session_id", sessionID),
				zap.String("run_id", runID),
			)
			_ = s.sessionRepo.UpdateStatus(ctx, sessionID, model.SessionStatusFailed)
			s.persistRunOutcomeMetadata(ctx, sessionID, "failed", agentruntime.GoalFailed)
			s.runManager.Publish(sessionID, EventErrorPayload(sessionID, runID, "assistant run returned no result"))
		} else {
			sessionStatus := sessionStatusForGoalOutcome(result.GoalOutcome)
			_ = s.sessionRepo.UpdateStatus(ctx, sessionID, sessionStatus)
			s.persistRunOutcomeMetadata(ctx, sessionID, result.RunStatus, result.GoalOutcome)
			s.runManager.Publish(sessionID, EventDoneOutcomePayload(
				sessionID,
				runID,
				result.RunStatus,
				string(result.GoalOutcome),
			))
			s.logger.Info("assistant run completed with explicit outcome",
				zap.String("session_id", sessionID),
				zap.String("run_id", runID),
				zap.String("run_status", result.RunStatus),
				zap.String("goal_outcome", string(result.GoalOutcome)),
				zap.String("session_status", sessionStatus),
			)
		}
	}

	s.runManager.Finish(sessionID)
}

func (s *Service) sessionLocale(ctx context.Context, sessionID string) string {
	session, err := s.sessionRepo.FindBySessionID(ctx, sessionID)
	if err != nil || len(session.Metadata) == 0 {
		return LocaleZhCN
	}
	var metadata map[string]interface{}
	if json.Unmarshal(session.Metadata, &metadata) != nil {
		return LocaleZhCN
	}
	value, _ := metadata["locale"].(string)
	return NormalizeLocale(value)
}

func sessionStatusForGoalOutcome(outcome agentruntime.GoalOutcome) string {
	switch outcome {
	case agentruntime.GoalSucceeded, agentruntime.GoalPartiallySucceeded:
		return model.SessionStatusCompleted
	case agentruntime.GoalNeedsInput:
		return model.SessionStatusActive
	default:
		return model.SessionStatusFailed
	}
}

func (s *Service) persistRunOutcomeMetadata(ctx context.Context, sessionID, runStatus string, outcome agentruntime.GoalOutcome) {
	if s == nil || s.sessionRepo == nil || sessionID == "" {
		return
	}
	session, err := s.sessionRepo.FindBySessionID(ctx, sessionID)
	if err != nil || session == nil {
		s.logger.Warn("failed to load assistant session for outcome persistence",
			zap.String("session_id", sessionID),
			zap.String("run_status", runStatus),
			zap.Error(err),
		)
		return
	}
	metadata := unmarshalJSON(session.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["current_run_status"] = runStatus
	metadata["goal_outcome"] = outcome
	metadata["last_run_finished_at"] = time.Now().UTC().Format(time.RFC3339)
	session.Metadata = mustMarshalJSON(metadata)
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		s.logger.Warn("failed to persist assistant run outcome",
			zap.String("session_id", sessionID),
			zap.String("run_status", runStatus),
			zap.String("goal_outcome", string(outcome)),
			zap.Error(err),
		)
	}
}

// ListContextRefs 列出上下文引用
func (s *Service) ListContextRefs(ctx context.Context, sessionID string) ([]model.AssistantContextRef, error) {
	return s.contextRepo.ListBySession(ctx, sessionID)
}

// ListToolCalls 列出工具调用
func (s *Service) ListToolCalls(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantToolCall, int64, error) {
	return s.toolCallRepo.ListBySession(ctx, sessionID, page, pageSize)
}

// ListApprovals 列出审批
func (s *Service) ListApprovals(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantApproval, int64, error) {
	return s.approvalRepo.ListBySession(ctx, sessionID, page, pageSize)
}

// SignalApprovalApproved 将用户批准结果发送给正在等待的工具调用。
func (s *Service) SignalApprovalApproved(approval *model.AssistantApproval, operator string, comment string) bool {
	if approval == nil || s.runManager == nil {
		return false
	}
	run, ok := s.runManager.Get(approval.SessionID)
	if !ok {
		return false
	}
	return run.ResolveApproval(ApprovalDecision{
		ApprovalID: approval.ApprovalID,
		Approved:   true,
		Operator:   operator,
		Comment:    comment,
		DecidedAt:  time.Now(),
	})
}

// SignalApprovalRejected 将用户拒绝结果发送给正在等待的工具调用。
func (s *Service) SignalApprovalRejected(approval *model.AssistantApproval, operator string, comment string) bool {
	if approval == nil || s.runManager == nil {
		return false
	}
	run, ok := s.runManager.Get(approval.SessionID)
	if !ok {
		return false
	}
	return run.ResolveApproval(ApprovalDecision{
		ApprovalID: approval.ApprovalID,
		Approved:   false,
		Operator:   operator,
		Comment:    comment,
		DecidedAt:  time.Now(),
	})
}

// extractThinkingFromHistory 从事件历史中提取 thinking 内容
// 返回 JSON 数组，每个元素是一个思考步骤
func (s *Service) extractThinkingFromHistory(sessionID string) datatypes.JSON {
	run, ok := s.runManager.Get(sessionID)
	if !ok {
		return nil
	}

	var thinkingSteps []string
	for _, event := range run.History() {
		if event.Type == EventThinking {
			if payload, ok := event.Payload.(map[string]interface{}); ok {
				if content, ok := payload["content"].(string); ok && content != "" {
					thinkingSteps = append(thinkingSteps, content)
				}
			}
		}
	}

	if len(thinkingSteps) == 0 {
		return nil
	}

	jsonBytes, err := json.Marshal(thinkingSteps)
	if err != nil {
		return nil
	}
	return datatypes.JSON(jsonBytes)
}

// extractPlanFromHistory 从事件历史中提取 plan 数据
func (s *Service) extractPlanFromHistory(sessionID string) datatypes.JSON {
	run, ok := s.runManager.Get(sessionID)
	if !ok {
		return nil
	}
	return extractPlanFromEvents(run.History())
}

// Helper functions

func newAssistantSessionID() string {
	return "asst_" + uuid.New().String()[:8]
}

func newMessageID() string {
	return "msg_" + uuid.New().String()[:8]
}

func defaultTaskType(taskType string) string {
	if taskType == "" {
		return model.TaskTypeExplanation
	}
	return taskType
}

func inferTitle(message string, refs []ContextRefInput) string {
	if message != "" {
		// 取用户首次输入的前 5 个字作为会话标题
		runes := []rune(message)
		if len(runes) > 5 {
			return string(runes[:5])
		}
		return message
	}
	if len(refs) > 0 {
		return fmt.Sprintf("分析 %s", refs[0].ObjectType)
	}
	return "新会话"
}

func inferModeSource(refs []ContextRefInput) string {
	if len(refs) > 0 {
		return "page_context"
	}
	return "assistant"
}
