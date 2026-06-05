package assistant

import (
	"context"
	"fmt"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/alex-chenc/aegis/api-server/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Orchestrator 编排器
type Orchestrator struct {
	configRepo    repository.ConfigRepository
	messageRepo   repository.AssistantMessageRepository
	toolCallRepo  repository.AssistantToolCallRepository
	runManager    *RunManager
	logger        *zap.Logger
}

// OrchestratorDeps 编排器依赖
type OrchestratorDeps struct {
	ConfigRepo   repository.ConfigRepository
	MessageRepo  repository.AssistantMessageRepository
	ToolCallRepo repository.AssistantToolCallRepository
	RunManager   *RunManager
	Logger       *zap.Logger
}

// RunInput 运行输入
type RunInput struct {
	RunID       string
	SessionID   string
	MessageID   string
	UserID      string
	UserMessage string
	TaskType    string
	ContextRefs []interface{}
}

// NewOrchestrator 创建编排器
func NewOrchestrator(deps OrchestratorDeps) *Orchestrator {
	return &Orchestrator{
		configRepo:   deps.ConfigRepo,
		messageRepo:  deps.MessageRepo,
		toolCallRepo: deps.ToolCallRepo,
		runManager:   deps.RunManager,
		logger:       deps.Logger,
	}
}

// Run 运行编排
func (o *Orchestrator) Run(ctx context.Context, input RunInput) (*RunResult, error) {
	o.logger.Info("starting orchestration",
		zap.String("session_id", input.SessionID),
		zap.String("run_id", input.RunID),
	)

	// Emit thinking event
	o.runManager.Publish(input.SessionID, EventThinkingPayload(input.SessionID, input.RunID, "正在分析您的问题..."))

	// TODO: Implement full orchestration with agent-runtime
	// For now, return a simple response
	response := fmt.Sprintf("感谢您的提问。您的问题是：\n\n%s\n\n这是一个 V6.0 智能体的占位响应。完整的 agent-runtime 集成将在后续阶段实现。", input.UserMessage)

	// Create assistant message
	msgID := "msg_" + input.RunID
	if err := o.messageRepo.Create(ctx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
	}); err != nil {
		o.logger.Error("failed to save assistant message", zap.Error(err))
	}

	// Emit done event
	o.runManager.Publish(input.SessionID, EventDonePayload(input.SessionID, input.RunID))

	return &RunResult{
		MessageID:   assistantMsg.MessageID,
		FinalAnswer: response,
	}, nil
}

// CreateMessageInput 创建消息输入
type CreateMessageInput struct {
	SessionID string
	MessageID string
	Role      string
	Content   string
}
