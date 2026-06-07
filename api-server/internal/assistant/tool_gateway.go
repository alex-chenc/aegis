package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ToolGateway 工具网关（实现 agent-runtime ToolGateway 接口）
type ToolGateway struct {
	dispatcher     *ToolDispatcher
	selectedTools  map[string]bool
	sessionID      string
	messageID      string
	runID          string
	operator       string
	onToolCall     func(callID, toolName string, args interface{})
	onToolResult   func(callID string, result interface{})
	onToolError    func(callID, errMsg string)
	onApprovalRequired func(approval interface{})
	logger         *zap.Logger
}

// ToolGatewayConfig 工具网关配置
type ToolGatewayConfig struct {
	Dispatcher         *ToolDispatcher
	SelectedTools      []string
	SessionID          string
	MessageID          string
	RunID              string
	Operator           string
	OnToolCall         func(callID, toolName string, args interface{})
	OnToolResult       func(callID string, result interface{})
	OnToolError        func(callID, errMsg string)
	OnApprovalRequired func(approval interface{})
	Logger             *zap.Logger
}

// NewToolGateway 创建工具网关
func NewToolGateway(cfg ToolGatewayConfig) *ToolGateway {
	selectedSet := make(map[string]bool)
	for _, name := range cfg.SelectedTools {
		selectedSet[name] = true
	}

	return &ToolGateway{
		dispatcher:         cfg.Dispatcher,
		selectedTools:      selectedSet,
		sessionID:          cfg.SessionID,
		messageID:          cfg.MessageID,
		runID:              cfg.RunID,
		operator:           cfg.Operator,
		onToolCall:         cfg.OnToolCall,
		onToolResult:       cfg.OnToolResult,
		onToolError:        cfg.OnToolError,
		onApprovalRequired: cfg.OnApprovalRequired,
		logger:             cfg.Logger,
	}
}

// ToolRequest 工具请求
type ToolRequest struct {
	Name      string                 `json:"name"`
	Args      map[string]interface{} `json:"args"`
	CallID    string                 `json:"call_id,omitempty"`
	Approved  bool                   `json:"approved,omitempty"`
}

// ToolResponse 工具响应
type ToolResponse struct {
	Status  string      `json:"status"` // success, failed, approval_required
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	CallID  string      `json:"call_id"`
}

// Call 执行工具调用（实现 agent-runtime ToolGateway 接口）
func (g *ToolGateway) Call(ctx context.Context, req ToolRequest) (ToolResponse, error) {
	// Check if tool is in selected set
	if !g.selectedTools[req.Name] {
		// Handle Tool.Search specially
		if req.Name == "Tool.Search" {
			return g.handleToolSearch(ctx, req)
		}
		return ToolResponse{
			Status: "failed",
			Error:  fmt.Sprintf("tool %s is not in the selected tool set", req.Name),
			CallID: req.CallID,
		}, nil
	}

	// Notify tool call
	if g.onToolCall != nil {
		g.onToolCall(req.CallID, req.Name, req.Args)
	}

	// Dispatch tool
	result, err := g.dispatcher.Dispatch(ctx, DispatchRequest{
		SessionID: g.sessionID,
		MessageID: g.messageID,
		RunID:     g.runID,
		ToolName:  req.Name,
		Args:      req.Args,
		Operator:  g.operator,
		Approved:  req.Approved,
	})

	if err != nil {
		if g.onToolError != nil {
			g.onToolError(req.CallID, err.Error())
		}
		return ToolResponse{
			Status: "failed",
			Error:  err.Error(),
			CallID: req.CallID,
		}, nil
	}

	// Handle approval required
	if result.ApprovalRequired {
		if g.onApprovalRequired != nil {
			g.onApprovalRequired(map[string]interface{}{
				"approval_id": result.ApprovalID,
				"tool_name":   result.ToolName,
				"call_id":     result.CallID,
			})
		}
		return ToolResponse{
			Status: "approval_required",
			CallID: result.CallID,
		}, nil
	}

	// Notify tool result
	if result.Success {
		if g.onToolResult != nil {
			g.onToolResult(result.CallID, result.Data)
		}
		return ToolResponse{
			Status: "success",
			Data:   result.Data,
			CallID: result.CallID,
		}, nil
	}

	if g.onToolError != nil {
		g.onToolError(result.CallID, result.Error)
	}
	return ToolResponse{
		Status: "failed",
		Error:  result.Error,
		CallID: result.CallID,
	}, nil
}

// Cancel 取消工具调用
func (g *ToolGateway) Cancel(ctx context.Context, callID string, hostID string) error {
	// No-op for synchronous execution
	return nil
}

func (g *ToolGateway) handleToolSearch(ctx context.Context, req ToolRequest) (ToolResponse, error) {
	query, _ := req.Args["query"].(string)
	// Record expansion request - this will be handled by the orchestrator
	g.logger.Info("tool search requested", zap.String("query", query))

	return ToolResponse{
		Status: "success",
		Data: map[string]interface{}{
			"expansion_requested": true,
			"query":               query,
		},
		CallID: req.CallID,
	}, nil
}

// ToolGatewayAdapter 适配 agent-runtime 接口
type ToolGatewayAdapter struct {
	gateway *ToolGateway
}

// NewToolGatewayAdapter 创建适配器
func NewToolGatewayAdapter(gateway *ToolGateway) *ToolGatewayAdapter {
	return &ToolGatewayAdapter{gateway: gateway}
}

// CallAgentTool agent-runtime 调用入口
func (a *ToolGatewayAdapter) CallAgentTool(ctx context.Context, name string, argsJSON string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("failed to parse args: %w", err)
	}

	callID := fmt.Sprintf("call_%d", time.Now().UnixNano())
	resp, err := a.gateway.Call(ctx, ToolRequest{
		Name:   name,
		Args:   args,
		CallID: callID,
	})
	if err != nil {
		return "", err
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}

	return string(respBytes), nil
}
