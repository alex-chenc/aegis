package assistant

import (
	"context"
	"fmt"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// --- fakes for ToolDispatcher tests ---

// fakeToolCallRepo implements repository.AssistantToolCallRepository
type fakeToolCallRepo struct {
	createdCall     *model.AssistantToolCall
	markSuccessCall *markSuccessRecord
	markFailedCall  *markFailedRecord
}

type markSuccessRecord struct {
	CallID   string
	Duration int64
}

type markFailedRecord struct {
	CallID   string
	ErrorMsg string
	Duration int64
}

func (f *fakeToolCallRepo) Create(_ context.Context, call *model.AssistantToolCall) error {
	f.createdCall = call
	return nil
}
func (f *fakeToolCallRepo) FindByCallID(_ context.Context, _ string) (*model.AssistantToolCall, error) {
	return nil, nil
}
func (f *fakeToolCallRepo) ListBySession(_ context.Context, _ string, _, _ int) ([]model.AssistantToolCall, int64, error) {
	return nil, 0, nil
}
func (f *fakeToolCallRepo) MarkSuccess(_ context.Context, callID string, _ interface{}, duration int64) error {
	f.markSuccessCall = &markSuccessRecord{CallID: callID, Duration: duration}
	return nil
}
func (f *fakeToolCallRepo) MarkFailed(_ context.Context, callID, errMsg string, duration int64) error {
	f.markFailedCall = &markFailedRecord{CallID: callID, ErrorMsg: errMsg, Duration: duration}
	return nil
}
func (f *fakeToolCallRepo) MarkApprovalRequired(_ context.Context, _, _ string) error {
	return nil
}
func (f *fakeToolCallRepo) MarkRejected(_ context.Context, _, _ string) error { return nil }
func (f *fakeToolCallRepo) UpdateStatus(_ context.Context, _, _ string) error { return nil }

// fakeSessionRepo implements repository.AssistantSessionRepository
type fakeSessionRepo struct{}

func (f *fakeSessionRepo) Create(_ context.Context, _ *model.AssistantSession) error { return nil }
func (f *fakeSessionRepo) FindBySessionID(_ context.Context, _ string) (*model.AssistantSession, error) {
	return nil, nil
}
func (f *fakeSessionRepo) FindByID(_ context.Context, _ uuid.UUID) (*model.AssistantSession, error) {
	return nil, nil
}
func (f *fakeSessionRepo) List(_ context.Context, _ repository.SessionQuery) ([]model.AssistantSession, int64, error) {
	return nil, 0, nil
}
func (f *fakeSessionRepo) Update(_ context.Context, _ *model.AssistantSession) error { return nil }
func (f *fakeSessionRepo) UpdateStatus(_ context.Context, _, _ string) error         { return nil }
func (f *fakeSessionRepo) IncrementMessageCount(_ context.Context, _ string) error   { return nil }
func (f *fakeSessionRepo) IncrementToolCallCount(_ context.Context, _ string) error  { return nil }
func (f *fakeSessionRepo) IncrementApprovalCount(_ context.Context, _ string) error  { return nil }
func (f *fakeSessionRepo) Delete(_ context.Context, _ string) error                  { return nil }

// --- tests ---

func newTestToolDispatcher(t *testing.T, registry *ToolRegistry) (*ToolDispatcher, *fakeToolCallRepo) {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	approvalGate := NewApprovalGate(ApprovalGateDeps{
		RiskPolicy: NewRiskPolicy(RiskPolicyDeps{}),
		Logger:     logger,
	})
	toolCallRepo := &fakeToolCallRepo{}
	sessionRepo := &fakeSessionRepo{}
	policyService := NewToolPolicyService(ToolPolicyServiceDeps{
		Registry: registry,
		Logger:   logger,
	})
	dispatcher := NewToolDispatcher(registry, approvalGate, toolCallRepo, sessionRepo, policyService, logger)
	return dispatcher, toolCallRepo
}

func TestToolDispatcher_NormalExecution(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:               "Test.Tool",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"result": "ok"}, nil
		},
	})

	dispatcher, _ := newTestToolDispatcher(t, registry)

	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		ToolName:  "Test.Tool",
		Args:      map[string]interface{}{},
		Approved:  true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Error)
	}
	if result.DurationMs < 0 {
		t.Fatalf("expected non-negative duration, got %d", result.DurationMs)
	}
}

func TestToolDispatcher_UsesProvidedCallID(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:               "Test.Tool",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"result": "ok"}, nil
		},
	})

	dispatcher, toolCallRepo := newTestToolDispatcher(t, registry)

	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		MessageID: "msg-1",
		CallID:    "runtime-call-1",
		ToolName:  "Test.Tool",
		Args:      map[string]interface{}{},
		Approved:  true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CallID != "runtime-call-1" {
		t.Fatalf("expected dispatch result call ID to use runtime call ID, got %q", result.CallID)
	}
	if toolCallRepo.createdCall == nil || toolCallRepo.createdCall.CallID != "runtime-call-1" {
		t.Fatalf("expected created tool call to use runtime call ID, got %#v", toolCallRepo.createdCall)
	}
	if toolCallRepo.markSuccessCall == nil || toolCallRepo.markSuccessCall.CallID != "runtime-call-1" {
		t.Fatalf("expected MarkSuccess to use runtime call ID, got %#v", toolCallRepo.markSuccessCall)
	}
}

func TestAssistantToolGatewayAdapterPublishesCompletionForRuntimeCallID(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:               "Test.Tool",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"result": "ok"}, nil
		},
	})

	dispatcher, _ := newTestToolDispatcher(t, registry)
	var startedCallID string
	var completedCallID string
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "test-session",
		MessageID:  "msg-1",
		RunID:      "run-1",
		OnToolCall: func(callID, toolName string, args interface{}) {
			startedCallID = callID
		},
		OnToolResult: func(callID string, result interface{}) {
			completedCallID = callID
		},
	})

	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "runtime-call-2",
		ToolName: "Test.Tool",
		Args:     map[string]interface{}{},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success response, got %s", resp.Status)
	}
	if startedCallID != "runtime-call-2" || completedCallID != "runtime-call-2" {
		t.Fatalf("expected matching runtime call IDs, started=%q completed=%q", startedCallID, completedCallID)
	}
}

func TestToolDispatcher_ExecutionTimeout(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:    "Slow.Tool",
		Enabled: true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			// Simulate a slow operation that respects context
			select {
			case <-time.After(60 * time.Second):
				return "done", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	dispatcher, toolCallRepo := newTestToolDispatcher(t, registry)

	start := time.Now()
	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		ToolName:  "Slow.Tool",
		Args:      map[string]interface{}{},
		Approved:  true,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure due to timeout, got success")
	}
	// Should contain timeout error message
	if result.Error == "" {
		t.Fatalf("expected timeout error message, got empty")
	}
	// Should complete within ~30s (the tool dispatcher timeout) + some margin
	if elapsed > 35*time.Second {
		t.Fatalf("expected completion within 35s, took %v", elapsed)
	}
	// Verify markFailed was called with timeout error
	if toolCallRepo.markFailedCall == nil {
		t.Fatalf("expected MarkFailed to be called")
	}
	if toolCallRepo.markFailedCall.ErrorMsg == "" {
		t.Fatalf("expected non-empty error message in MarkFailed call")
	}
}

func TestToolDispatcher_ToolHandlerError(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:    "Error.Tool",
		Enabled: true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("database connection failed")
		},
	})

	dispatcher, _ := newTestToolDispatcher(t, registry)

	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		ToolName:  "Error.Tool",
		Args:      map[string]interface{}{},
		Approved:  true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure, got success")
	}
	if result.Error != "database connection failed" {
		t.Fatalf("expected 'database connection failed', got '%s'", result.Error)
	}
}

func TestToolDispatcher_ToolNotFound(t *testing.T) {
	registry := NewToolRegistry()
	dispatcher, _ := newTestToolDispatcher(t, registry)

	_, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		ToolName:  "NonExistent.Tool",
		Args:      map[string]interface{}{},
		Approved:  true,
	})

	if err == nil {
		t.Fatalf("expected error for non-existent tool")
	}
}

func TestToolDispatcher_TimeoutWithSlowDB(t *testing.T) {
	// Simulate the exact scenario: tool handler calls DB without respecting context
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:    "Host.List",
		Enabled: true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			// Simulate a DB call that blocks indefinitely (doesn't respect ctx)
			time.Sleep(5 * time.Second)
			return map[string]interface{}{"data": []string{}, "total": 0}, nil
		},
	})

	dispatcher, toolCallRepo := newTestToolDispatcher(t, registry)

	start := time.Now()
	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		ToolName:  "Host.List",
		Args:      map[string]interface{}{"page": 1, "page_size": 20},
		Approved:  true,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The handler sleeps for 5s, which is within the 30s timeout,
	// so it should succeed
	if !result.Success {
		t.Fatalf("expected success (5s < 30s timeout), got failure: %s", result.Error)
	}
	if elapsed < 4*time.Second {
		t.Fatalf("expected at least 4s elapsed, got %v", elapsed)
	}
	if toolCallRepo.markFailedCall != nil {
		t.Fatalf("expected MarkFailed NOT to be called for successful execution")
	}
}
