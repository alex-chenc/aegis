package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

// AssistantToolGatewayAdapter 适配 agent-runtime ToolGateway 接口
// 将 agent-runtime 的工具调用桥接到 assistant 的 ToolDispatcher
type AssistantToolGatewayAdapter struct {
	dispatcher *ToolDispatcher
	sessionID  string
	messageID  string
	runID      string
	operator   string
	logger     *zap.Logger

	// 回调函数，用于 SSE 事件推送
	onToolCall   func(callID, toolName string, args interface{})
	onToolResult func(callID string, result interface{})
	onToolError  func(callID, errMsg string)
	onApproval   func(approval interface{})
}

// AssistantToolGatewayConfig 适配器配置
type AssistantToolGatewayConfig struct {
	Dispatcher   *ToolDispatcher
	SessionID    string
	MessageID    string
	RunID        string
	Operator     string
	Logger       *zap.Logger
	OnToolCall   func(callID, toolName string, args interface{})
	OnToolResult func(callID string, result interface{})
	OnToolError  func(callID, errMsg string)
	OnApproval   func(approval interface{})
}

// NewAssistantToolGatewayAdapter 创建适配器
func NewAssistantToolGatewayAdapter(cfg AssistantToolGatewayConfig) *AssistantToolGatewayAdapter {
	return &AssistantToolGatewayAdapter{
		dispatcher:   cfg.Dispatcher,
		sessionID:    cfg.SessionID,
		messageID:    cfg.MessageID,
		runID:        cfg.RunID,
		operator:     cfg.Operator,
		logger:       cfg.Logger,
		onToolCall:   cfg.OnToolCall,
		onToolResult: cfg.OnToolResult,
		onToolError:  cfg.OnToolError,
		onApproval:   cfg.OnApproval,
	}
}

// Call 实现 agentruntime.ToolGateway 接口
func (a *AssistantToolGatewayAdapter) Call(ctx context.Context, req agentruntime.ToolRequest) (agentruntime.ToolResponse, error) {
	startedAt := time.Now()

	// 解析参数
	args := make(map[string]interface{})
	if req.Args != nil {
		for k, v := range req.Args {
			args[k] = v
		}
	}

	// 通知工具调用开始
	if a.onToolCall != nil {
		a.onToolCall(req.CallID, req.ToolName, args)
	}

	// 通过 ToolDispatcher 调度执行
	result, err := a.dispatcher.Dispatch(ctx, DispatchRequest{
		SessionID: a.sessionID,
		MessageID: a.messageID,
		RunID:     a.runID,
		CallID:    req.CallID,
		ToolName:  req.ToolName,
		Args:      args,
		Operator:  a.operator,
	})

	endedAt := time.Now()

	if err != nil {
		errMsg := fmt.Sprintf("tool dispatch error: %v", err)
		if a.onToolError != nil {
			a.onToolError(req.CallID, errMsg)
		}
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			ErrorMessage: errMsg,
			Summary:      fmt.Sprintf("工具 %s 调度失败", req.ToolName),
			StartedAt:    startedAt,
			EndedAt:      endedAt,
		}, nil
	}

	// 处理需要审批的情况
	if result.ApprovalRequired {
		if a.onApproval != nil {
			a.onApproval(map[string]interface{}{
				"approval_id": result.ApprovalID,
				"tool_name":   result.ToolName,
				"call_id":     result.CallID,
			})
		}
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			Summary:      fmt.Sprintf("工具 %s 需要审批", req.ToolName),
			ErrorMessage: fmt.Sprintf("approval_required:%s", result.ApprovalID),
			StartedAt:    startedAt,
			EndedAt:      endedAt,
		}, nil
	}

	// 处理执行结果
	if result.Success {
		resultJSON, _ := json.Marshal(result.Data)
		if a.onToolResult != nil {
			a.onToolResult(result.CallID, result.Data)
		}
		return agentruntime.ToolResponse{
			CallID:    req.CallID,
			ToolName:  req.ToolName,
			Status:    agentruntime.ToolCallSuccess,
			Content:   string(resultJSON),
			Summary:   fmt.Sprintf("工具 %s 执行成功", req.ToolName),
			StartedAt: startedAt,
			EndedAt:   endedAt,
		}, nil
	}

	// 工具执行失败
	if a.onToolError != nil {
		a.onToolError(result.CallID, result.Error)
	}
	return agentruntime.ToolResponse{
		CallID:       req.CallID,
		ToolName:     req.ToolName,
		Status:       agentruntime.ToolCallFailed,
		ErrorMessage: result.Error,
		Summary:      fmt.Sprintf("工具 %s 执行失败", req.ToolName),
		StartedAt:    startedAt,
		EndedAt:      endedAt,
	}, nil
}

// Cancel 实现 agentruntime.ToolGateway 接口（同步执行，无需取消）
func (a *AssistantToolGatewayAdapter) Cancel(_ context.Context, _ string, _ string) error {
	return nil
}
