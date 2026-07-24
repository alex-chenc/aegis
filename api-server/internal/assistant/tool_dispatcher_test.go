package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	calls           []model.AssistantToolCall
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
	f.calls = append(f.calls, *call)
	return nil
}
func (f *fakeToolCallRepo) FindByCallID(_ context.Context, _ string) (*model.AssistantToolCall, error) {
	return nil, nil
}
func (f *fakeToolCallRepo) ListBySession(_ context.Context, _ string, _, _ int) ([]model.AssistantToolCall, int64, error) {
	return append([]model.AssistantToolCall{}, f.calls...), int64(len(f.calls)), nil
}
func (f *fakeToolCallRepo) MarkSuccess(_ context.Context, callID string, result interface{}, duration int64) error {
	f.markSuccessCall = &markSuccessRecord{CallID: callID, Duration: duration}
	for i := range f.calls {
		if f.calls[i].CallID == callID {
			f.calls[i].Status = model.ToolCallStatusSuccess
			f.calls[i].Result = mustMarshalJSON(result)
		}
	}
	return nil
}
func (f *fakeToolCallRepo) MarkOutcome(_ context.Context, callID, operationStatus string, terminal bool, outcome interface{}) error {
	for i := range f.calls {
		if f.calls[i].CallID == callID {
			f.calls[i].OperationStatus = operationStatus
			f.calls[i].OperationTerminal = &terminal
			f.calls[i].Outcome = mustMarshalJSON(outcome)
		}
	}
	return nil
}
func (f *fakeToolCallRepo) MarkFailed(_ context.Context, callID, errMsg string, duration int64) error {
	f.markFailedCall = &markFailedRecord{CallID: callID, ErrorMsg: errMsg, Duration: duration}
	for i := range f.calls {
		if f.calls[i].CallID == callID {
			f.calls[i].Status = model.ToolCallStatusFailed
			f.calls[i].ErrorMessage = errMsg
		}
	}
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

// fakeSystemConfigQuerier implements SystemConfigQuerier for tests.
type fakeSystemConfigQuerier struct {
	configs map[string]*model.SystemConfig
}

func (f *fakeSystemConfigQuerier) GetByKey(key string) (*model.SystemConfig, error) {
	if cfg, ok := f.configs[key]; ok {
		return cfg, nil
	}
	return nil, fmt.Errorf("config key %s not found", key)
}

func (f *fakeSystemConfigQuerier) Upsert(key string, value interface{}, description, category string) error {
	b, _ := json.Marshal(value)
	f.configs[key] = &model.SystemConfig{
		ConfigKey:   key,
		ConfigValue: b,
		Description: description,
		Category:    category,
	}
	return nil
}

// fakeApprovalRepo implements repository.AssistantApprovalRepository for tests.
type fakeApprovalRepo struct {
	approvals map[string]*model.AssistantApproval
}

func newFakeApprovalRepo() *fakeApprovalRepo {
	return &fakeApprovalRepo{approvals: make(map[string]*model.AssistantApproval)}
}

func (f *fakeApprovalRepo) Create(_ context.Context, approval *model.AssistantApproval) error {
	f.approvals[approval.ApprovalID] = approval
	return nil
}
func (f *fakeApprovalRepo) FindByApprovalID(_ context.Context, approvalID string) (*model.AssistantApproval, error) {
	if a, ok := f.approvals[approvalID]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("approval %s not found", approvalID)
}
func (f *fakeApprovalRepo) ListBySession(_ context.Context, _ string, _, _ int) ([]model.AssistantApproval, int64, error) {
	return nil, 0, nil
}
func (f *fakeApprovalRepo) ListPending(_ context.Context, _ string) ([]model.AssistantApproval, error) {
	return nil, nil
}
func (f *fakeApprovalRepo) MarkApproved(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeApprovalRepo) MarkRejected(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeApprovalRepo) MarkExecuted(_ context.Context, _ string) error       { return nil }
func (f *fakeApprovalRepo) MarkFailed(_ context.Context, _, _ string) error      { return nil }
func (f *fakeApprovalRepo) MarkExpired(_ context.Context, _ string) error        { return nil }

// --- tests ---

func newTestToolDispatcher(t *testing.T, registry *ToolRegistry) (*ToolDispatcher, *fakeToolCallRepo) {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	approvalGate := NewApprovalGate(ApprovalGateDeps{
		ApprovalRepo: newFakeApprovalRepo(),
		RiskPolicy:   NewRiskPolicy(RiskPolicyDeps{}),
		Logger:       logger,
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

func newTestToolDispatcherWithMode(t *testing.T, registry *ToolRegistry, mode string) (*ToolDispatcher, *fakeToolCallRepo) {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	approvalGate := NewApprovalGate(ApprovalGateDeps{
		ApprovalRepo: newFakeApprovalRepo(),
		RiskPolicy:   NewRiskPolicy(RiskPolicyDeps{}),
		Logger:       logger,
	})
	toolCallRepo := &fakeToolCallRepo{}
	sessionRepo := &fakeSessionRepo{}
	modeJSON, _ := json.Marshal(mode)
	policyService := NewToolPolicyService(ToolPolicyServiceDeps{
		Registry: registry,
		SystemConfig: &fakeSystemConfigQuerier{
			configs: map[string]*model.SystemConfig{
				"assistant.tool_approval_mode": {
					ConfigKey:   "assistant.tool_approval_mode",
					ConfigValue: modeJSON,
				},
			},
		},
		Logger: logger,
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
		OnToolResult: func(callID string, result interface{}, outcome *agentruntime.ToolOutcome) {
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

func TestAssistantToolGatewayAdapterReusesSuccessfulReadonlyToolCall(t *testing.T) {
	registry := NewToolRegistry()
	executeCount := 0
	_ = registry.Register(&ToolSpec{
		Name:               "Test.Readonly",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			executeCount++
			return map[string]interface{}{"result": "ok", "host_id": args["host_id"]}, nil
		},
	})

	dispatcher, repo := newTestToolDispatcher(t, registry)
	startedCount := 0
	resultCount := 0
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "test-session",
		MessageID:  "msg-1",
		RunID:      "run-1",
		Logger:     zap.NewNop(),
		OnToolCall: func(callID, toolName string, args interface{}) {
			startedCount++
		},
		OnToolResult: func(callID string, result interface{}, outcome *agentruntime.ToolOutcome) {
			resultCount++
		},
	})

	args := map[string]interface{}{"host_id": "host-1"}
	first, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "runtime-call-1",
		ToolName: "Test.Readonly",
		Args:     args,
	})
	if err != nil || first.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("first call failed: resp=%#v err=%v", first, err)
	}
	second, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "runtime-call-2",
		ToolName: "Test.Readonly",
		Args:     map[string]interface{}{"host_id": "host-1"},
	})
	if err != nil || second.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("second call failed: resp=%#v err=%v", second, err)
	}

	if executeCount != 1 {
		t.Fatalf("expected one real tool execution, got %d", executeCount)
	}
	if len(repo.calls) != 1 {
		t.Fatalf("expected one persisted tool call, got %d", len(repo.calls))
	}
	if startedCount != 1 || resultCount != 1 {
		t.Fatalf("expected only the real call to publish callbacks, started=%d result=%d", startedCount, resultCount)
	}
	if !strings.Contains(second.Content, "host-1") {
		t.Fatalf("expected cached response content, got %s", second.Content)
	}
}

func TestAssistantToolGatewayAdapterDoesNotReuseVolatileStatusTool(t *testing.T) {
	tool := &ToolSpec{
		Name:       "Vulnerability.Script.Status",
		Operation:  OpGet,
		Risk:       ToolRiskReadonly,
		Idempotent: true,
	}
	if canReuseAssistantToolResult(tool) {
		t.Fatal("volatile status tools must bypass same-message result reuse")
	}
}

func TestAssistantToolGatewayAdapterCollapsesRuntimeAsyncPollsIntoLogicalCall(t *testing.T) {
	registry := NewToolRegistry()
	executions := 0
	_ = registry.Register(&ToolSpec{
		Name:               "Example.Operation.Status",
		Operation:          OpGet,
		Risk:               ToolRiskReadonly,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ResultContract: ToolResultContract{
			OperationStatusField: "status",
			PendingValues:        []string{"running"},
			SuccessValues:        []string{"succeeded"},
			OperationRefFields:   []string{"operation_id"},
		},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			executions++
			status := "running"
			if executions == 3 {
				status = "succeeded"
			}
			return map[string]interface{}{"operation_id": "operation-1", "status": status}, nil
		},
	})

	dispatcher, repo := newTestToolDispatcher(t, registry)
	startedCount := 0
	resultCount := 0
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "session-1",
		MessageID:  "message-1",
		RunID:      "run-1",
		Logger:     zap.NewNop(),
		OnToolCall: func(string, string, interface{}) {
			startedCount++
		},
		OnToolResult: func(string, interface{}, *agentruntime.ToolOutcome) {
			resultCount++
		},
	})

	call := func(polling bool, attempt string) agentruntime.ToolResponse {
		t.Helper()
		requestContext := map[string]string{}
		if polling {
			requestContext["agent_runtime_async_poll"] = "true"
			requestContext["agent_runtime_poll_call_id"] = "logical-call-1"
			requestContext["agent_runtime_poll_attempt"] = attempt
		}
		response, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
			CallID:   "logical-call-1",
			ToolName: "Example.Operation.Status",
			Args:     map[string]interface{}{"operation_id": "operation-1"},
			Context:  requestContext,
		})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	if response := call(false, ""); response.Outcome == nil || response.Outcome.Terminal {
		t.Fatalf("initial response = %#v, want non-terminal", response.Outcome)
	}
	if response := call(true, "1"); response.Outcome == nil || response.Outcome.Terminal {
		t.Fatalf("first poll response = %#v, want non-terminal", response.Outcome)
	}
	if response := call(true, "2"); response.Outcome == nil || !response.Outcome.Terminal {
		t.Fatalf("terminal poll response = %#v, want terminal", response.Outcome)
	}

	if executions != 3 {
		t.Fatalf("executions = %d, want 3", executions)
	}
	if len(repo.calls) != 1 {
		t.Fatalf("persisted tool calls = %d, want one logical call", len(repo.calls))
	}
	if startedCount != 1 || resultCount != 2 {
		t.Fatalf("visible callbacks started=%d results=%d, want 1 start and running+terminal results", startedCount, resultCount)
	}
	if repo.calls[0].OperationTerminal == nil || !*repo.calls[0].OperationTerminal {
		t.Fatalf("persisted outcome did not reach terminal: %#v", repo.calls[0])
	}
}

func TestAssistantToolGatewayAdapterExecutesOnlyRequestedAssetCollectionTool(t *testing.T) {
	registry := NewToolRegistry()
	for _, spec := range []*ToolSpec{
		{
			Name:               "Asset.Collection.Trigger",
			Risk:               ToolRiskMedium,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"task_id": "collection-1", "status": "collecting"}, nil
			},
		},
		{
			Name:               "Asset.Collection.Get",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"task": map[string]interface{}{"id": "collection-1", "status": "completed"}}, nil
			},
		},
		{
			Name:               "Asset.Application.List",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"data": []map[string]interface{}{{"name": "claude-code"}}, "total": 1}, nil
			},
		},
		{
			Name:               "Asset.Summary.Get",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"summary": map[string]interface{}{"ai_agent_count": 1}}, nil
			},
		},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}

	dispatcher, _ := newTestToolDispatcher(t, registry)
	var started []string
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "test-session",
		MessageID:  "msg-asset",
		RunID:      "run-asset",
		UserInput:  "请严格按顺序调用工具：Asset.Collection.Trigger、Asset.Collection.Get、Asset.Application.List、Asset.Summary.Get。",
		OnToolCall: func(callID, toolName string, args interface{}) {
			started = append(started, toolName)
		},
	})

	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "asset-trigger-call",
		ToolName: "Asset.Collection.Trigger",
		Args:     map[string]interface{}{"scope": "hosts", "host_ids": []string{"host-1"}, "types": []string{"process"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success response, got %s: %s", resp.Status, resp.ErrorMessage)
	}
	if strings.Contains(resp.Content, `"asset_collection_sequence_complete":true`) {
		t.Fatalf("gateway must not synthesize asset collection sequence result, got %s", resp.Content)
	}
	if len(started) != 1 || started[0] != "Asset.Collection.Trigger" {
		t.Fatalf("expected only runtime-requested tool call, got %v", started)
	}
}

func TestAssistantToolGatewayAdapterExecutesOnlyRequestedVulnerabilityTool(t *testing.T) {
	registry := NewToolRegistry()
	for _, spec := range []*ToolSpec{
		{
			Name:               "Vulnerability.Script.Status",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"summary": map[string]interface{}{"generated": 1}}, nil
			},
		},
		{
			Name:               "Vulnerability.Script.Execute",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"task_group_id": "task-" + args["script_type"].(string)}, nil
			},
		},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}

	dispatcher, _ := newTestToolDispatcher(t, registry)
	var started []string
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "test-session",
		MessageID:  "msg-vuln",
		RunID:      "run-vuln",
		UserInput: strings.Join([]string{
			`Vulnerability.Script.Status 参数 cve_id="CVE-2023-50495", script_type="poc"。`,
			`Vulnerability.Script.Execute 参数 cve_id="CVE-2023-50495", script_type="poc", host_ids=["cf18f7f7-5b45-46e2-9889-160dddc4ee30"]。`,
			`Vulnerability.Script.Execute 参数 cve_id="CVE-2023-50495", script_type="fix", host_ids=["cf18f7f7-5b45-46e2-9889-160dddc4ee30"]。`,
		}, "\n"),
		OnToolCall: func(callID, toolName string, args interface{}) {
			started = append(started, toolName)
		},
	})

	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "vuln-status-call",
		ToolName: "Vulnerability.Script.Status",
		Args:     map[string]interface{}{"cve_id": "CVE-2023-50495", "script_type": "poc"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success response, got %s: %s", resp.Status, resp.ErrorMessage)
	}
	if strings.Contains(resp.Content, `"vulnerability_script_sequence_complete":true`) {
		t.Fatalf("gateway must not synthesize vulnerability sequence result, got %s", resp.Content)
	}
	if len(started) != 1 || started[0] != "Vulnerability.Script.Status" {
		t.Fatalf("expected only runtime-requested tool call, got %v", started)
	}
}

func TestAssistantToolGatewayAdapterExecutesOnlyRequestedDetectionTool(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{
		"Detection.Alert.List",
		"Detection.Alert.Get",
		"Detection.Statistics.Get",
		"Detection.Trend.Get",
		"SigmaRule.List",
		"Investigation.HostAttack.Analyze",
	} {
		toolName := name
		if err := registry.Register(&ToolSpec{
			Name:               toolName,
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"ok": true, "tool": toolName}, nil
			},
		}); err != nil {
			t.Fatalf("register %s: %v", toolName, err)
		}
	}

	dispatcher, _ := newTestToolDispatcher(t, registry)
	var started []string
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "test-session",
		MessageID:  "msg-detection",
		RunID:      "run-detection",
		UserInput: strings.Join([]string{
			`Detection.Alert.List 参数 page=1,page_size=10。`,
			`Detection.Alert.Get 参数 alert_id="ALT-e69edac6"。`,
			`Detection.Statistics.Get。`,
			`Detection.Trend.Get 参数 hours=24。`,
			`SigmaRule.List 参数 page=1,page_size=10,status="active"。`,
			`Investigation.HostAttack.Analyze 参数 host_id="cf18f7f7-5b45-46e2-9889-160dddc4ee30"。`,
		}, "\n"),
		OnToolCall: func(callID, toolName string, args interface{}) {
			started = append(started, toolName)
		},
	})

	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "detection-start",
		ToolName: "Detection.Alert.Get",
		Args:     map[string]interface{}{"alert_id": "ALT-e69edac6"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success response, got %s: %s", resp.Status, resp.ErrorMessage)
	}
	if strings.Contains(resp.Content, `"detection_sequence_complete":true`) {
		t.Fatalf("gateway must not synthesize detection sequence result, got %s", resp.Content)
	}
	if len(started) != 1 || started[0] != "Detection.Alert.Get" {
		t.Fatalf("expected only runtime-requested tool call, got %v", started)
	}
}

func TestAssistantToolGatewayAdapterExecutesOnlyRequestedPackageTool(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"Package.List", "Package.Get", "Package.Build.Start"} {
		toolName := name
		if err := registry.Register(&ToolSpec{
			Name:               toolName,
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"ok": true, "tool": toolName, "package_id": args["package_id"]}, nil
			},
		}); err != nil {
			t.Fatalf("register %s: %v", toolName, err)
		}
	}

	dispatcher, _ := newTestToolDispatcher(t, registry)
	var started []string
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "test-session",
		MessageID:  "msg-package",
		RunID:      "run-package",
		UserInput: strings.Join([]string{
			`Package.List 参数 page=1,page_size=20。`,
			`Package.Get 参数 package_id="b1c4300a-d050-4b12-8b0f-b41fce167b1e"。`,
			`Package.Build.Start 参数 package_id="codex-e2e-123", operator="playwright"。`,
		}, "\n"),
		OnToolCall: func(callID, toolName string, args interface{}) {
			started = append(started, toolName)
		},
	})

	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "package-start",
		ToolName: "Package.List",
		Args:     map[string]interface{}{"page": 1, "page_size": 20},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success response, got %s: %s", resp.Status, resp.ErrorMessage)
	}
	if strings.Contains(resp.Content, `"package_sequence_complete":true`) {
		t.Fatalf("gateway must not synthesize package sequence result, got %s", resp.Content)
	}
	if len(started) != 1 || started[0] != "Package.List" {
		t.Fatalf("expected only runtime-requested tool call, got %v", started)
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

// --- applyPlanArgs tests ---

func TestGatewayAppliesPlanArgs(t *testing.T) {
	registry := NewToolRegistry()
	var receivedArgs map[string]interface{}
	_ = registry.Register(&ToolSpec{
		Name:               "Asset.Collection.Trigger",
		Domain:             DomainAsset,
		Operation:          OpExecute,
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			receivedArgs = args
			return map[string]interface{}{"task_id": "t-001"}, nil
		},
	})

	dispatcher, _ := newTestToolDispatcher(t, registry)
	plan := &ToolExecutionPlan{
		Steps: []ToolPlanStep{
			{
				ToolName: "Asset.Collection.Trigger",
				Args:     map[string]interface{}{"scope": "online_hosts", "types": []string{"process"}},
			},
		},
	}
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:    dispatcher,
		SessionID:     "test-session",
		MessageID:     "msg-1",
		RunID:         "run-1",
		ExecutionPlan: plan,
	})

	// LLM provides no args; plan args should fill in
	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "call-1",
		ToolName: "Asset.Collection.Trigger",
		Args:     map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success, got %s", resp.Status)
	}
	if receivedArgs["scope"] != "online_hosts" {
		t.Fatalf("expected scope from plan, got %v", receivedArgs["scope"])
	}
}

func TestGatewayPlanArgsOverrideLLMArgs(t *testing.T) {
	registry := NewToolRegistry()
	var receivedArgs map[string]interface{}
	_ = registry.Register(&ToolSpec{
		Name:               "Asset.Collection.Trigger",
		Domain:             DomainAsset,
		Operation:          OpExecute,
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			receivedArgs = args
			return map[string]interface{}{"task_id": "t-001"}, nil
		},
	})

	dispatcher, _ := newTestToolDispatcher(t, registry)
	plan := &ToolExecutionPlan{
		Steps: []ToolPlanStep{
			{
				ToolName: "Asset.Collection.Trigger",
				Args:     map[string]interface{}{"scope": "online_hosts"},
			},
		},
	}
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:    dispatcher,
		SessionID:     "test-session",
		MessageID:     "msg-1",
		RunID:         "run-1",
		ExecutionPlan: plan,
	})

	// Caller-authorized plan scope must override a conflicting model value.
	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "call-2",
		ToolName: "Asset.Collection.Trigger",
		Args:     map[string]interface{}{"scope": "all_hosts"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success, got %s", resp.Status)
	}
	if receivedArgs["scope"] != "online_hosts" {
		t.Fatalf("expected plan arg to override model arg, got scope=%v", receivedArgs["scope"])
	}
}

func TestGatewayFixedPlanArgsDiscardUnknownModelFields(t *testing.T) {
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		ExecutionPlan: &ToolExecutionPlan{Steps: []ToolPlanStep{{
			StepID:   "authorized_02",
			ToolName: "Baseline.Compliance.Run",
			Args: map[string]interface{}{
				"target_scope":      "all_online_hosts",
				"template_selector": "cis-ubuntu",
				"scope":             "all_rules",
			},
		}}},
	})

	prepared, err := gateway.Prepare(context.Background(), agentruntime.ToolRequest{
		StepID:   "authorized_02",
		ToolName: "Baseline.Compliance.Run",
		Args: map[string]interface{}{
			"host_ids":       []string{"host-1"},
			"baseline_name":  "legacy-name",
			"auto_remediate": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"host_ids", "baseline_name", "auto_remediate"} {
		if _, exists := prepared.Args[forbidden]; exists {
			t.Fatalf("fixed plan retained unauthorized model field %q: %#v", forbidden, prepared.Args)
		}
	}
}

func TestGatewayPrepareUsesInvocationFilters(t *testing.T) {
	registry := newInvocationFilterTestRegistry(t)
	dispatcher, _ := newTestToolDispatcher(t, registry)
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "session-1",
		MessageID:  "message-1",
		RunID:      "run-1",
	})

	prepared, err := gateway.Prepare(context.Background(), agentruntime.ToolRequest{
		StepID:   "step-1",
		ToolName: "Host.Resolve",
		Args:     map[string]interface{}{"selector": "live"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Args["target_scope"] != "all_online_hosts" || prepared.Args["require_online"] != true {
		t.Fatalf("gateway did not use the invocation filter result: %#v", prepared.Args)
	}
}

func TestGatewayRejectsToolOutsideMappedStep(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"Example.Allowed", "Example.Invented"} {
		if err := registry.Register(&ToolSpec{
			Name:       name,
			Domain:     DomainSystem,
			Operation:  OpGet,
			Capability: strings.ToLower(strings.ReplaceAll(name, ".", "_")),
			Risk:       ToolRiskReadonly,
			Enabled:    true,
			ExposurePolicy: ToolExposurePolicy{
				Exposure:       ToolExposurePrimary,
				Discoverable:   true,
				DirectCallable: true,
			},
			ArgsSchema: map[string]interface{}{"type": "object"},
			Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"ok": true}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher, _ := newTestToolDispatcher(t, registry)
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		ExecutionPlan: &ToolExecutionPlan{Steps: []ToolPlanStep{{
			StepID:     "authorized_01",
			ToolName:   "Example.Allowed",
			Capability: "example_allowed",
		}}},
	})

	_, err := gateway.Prepare(context.Background(), agentruntime.ToolRequest{
		StepID:   "authorized_01",
		ToolName: "Example.Invented",
		Args:     map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("runtime must not replace the Mapping-bound tool")
	}
}

func TestToolDispatcherRejectsInvalidArgsBeforeDurableCall(t *testing.T) {
	registry := NewToolRegistry()
	handlerCalls := 0
	if err := registry.Register(&ToolSpec{
		Name:      "Example.Strict",
		Domain:    DomainSystem,
		Operation: OpGet,
		Risk:      ToolRiskReadonly,
		Enabled:   true,
		ExposurePolicy: ToolExposurePolicy{
			Exposure:       ToolExposurePrimary,
			Discoverable:   true,
			DirectCallable: true,
		},
		ArgsSchema: map[string]interface{}{
			"type":                 "object",
			"properties":           map[string]interface{}{"query": map[string]interface{}{"type": "string"}},
			"required":             []interface{}{"query"},
			"additionalProperties": false,
		},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			handlerCalls++
			return map[string]interface{}{"ok": true}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	dispatcher, toolCalls := newTestToolDispatcher(t, registry)
	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "session-1",
		RunID:     "run-1",
		MessageID: "message-1",
		ToolName:  "Example.Strict",
		Args:      map[string]interface{}{"expression": "all"},
	})
	if err == nil {
		t.Fatalf("expected pre-invocation rejection, got result %#v", result)
	}
	if handlerCalls != 0 {
		t.Fatalf("invalid arguments invoked handler %d times", handlerCalls)
	}
	if toolCalls.createdCall != nil || len(toolCalls.calls) != 0 {
		t.Fatalf("invalid arguments created a durable tool call: %#v", toolCalls.calls)
	}
}

func TestGatewayUsesStepIDForRepeatedToolArgs(t *testing.T) {
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		ExecutionPlan: &ToolExecutionPlan{Steps: []ToolPlanStep{
			{StepID: "generate_poc", ToolName: "Vulnerability.Script.Generate", Args: map[string]interface{}{"script_type": "poc", "cve_id": "CVE-2021-45340"}},
			{StepID: "generate_fix", ToolName: "Vulnerability.Script.Generate", Args: map[string]interface{}{"script_type": "fix", "cve_id": "CVE-2021-45340"}},
		}},
	})

	prepared, err := gateway.Prepare(context.Background(), agentruntime.ToolRequest{
		StepID:   "generate_fix",
		ToolName: "Vulnerability.Script.Generate",
		Args:     map[string]interface{}{"script_type": "poc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Args["script_type"] != "fix" {
		t.Fatalf("prepared script_type = %#v", prepared.Args["script_type"])
	}
}

func TestGatewayPrepareDoesNotInferScenarioArgsFromPriorCalls(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:      "Example.Apply",
		Domain:    DomainSystem,
		Operation: OpExecute,
		Risk:      ToolRiskLow,
		Enabled:   true,
		ArgsSchema: map[string]interface{}{
			"type":                 "object",
			"properties":           map[string]interface{}{"resource_id": map[string]interface{}{"type": "string"}},
			"required":             []interface{}{"resource_id"},
			"additionalProperties": false,
		},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	dispatcher, toolRepo := newTestToolDispatcher(t, registry)
	toolRepo.calls = []model.AssistantToolCall{{
		SessionID: "session-1", MessageID: "msg-1", ToolName: "Example.Discover",
		Status: model.ToolCallStatusSuccess,
		Result: mustMarshalJSON(map[string]interface{}{"resource_id": "resource-from-history"}),
	}}
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: dispatcher,
		SessionID:  "session-1",
		MessageID:  "msg-1",
	})

	prepared, err := gateway.Prepare(context.Background(), agentruntime.ToolRequest{
		ToolName: "Example.Apply",
		Args:     map[string]interface{}{"resource_id": "resource-from-runtime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Args["resource_id"] != "resource-from-runtime" {
		t.Fatalf("gateway must preserve runtime arguments, got %#v", prepared.Args)
	}
	if prepared.Context["aegis_prepared"] != "true" {
		t.Fatalf("expected generic preparer marker, got %#v", prepared.Context)
	}
}

func TestGatewayNilPlanDoesNotAffectArgs(t *testing.T) {
	registry := NewToolRegistry()
	var receivedArgs map[string]interface{}
	_ = registry.Register(&ToolSpec{
		Name:               "Host.List",
		Domain:             DomainHost,
		Operation:          OpList,
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			receivedArgs = args
			return map[string]interface{}{"hosts": []string{}}, nil
		},
	})

	dispatcher, _ := newTestToolDispatcher(t, registry)
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:    dispatcher,
		SessionID:     "test-session",
		MessageID:     "msg-1",
		RunID:         "run-1",
		ExecutionPlan: nil, // no plan
	})

	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "call-3",
		ToolName: "Host.List",
		Args:     map[string]interface{}{"status": "online"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success, got %s", resp.Status)
	}
	if receivedArgs["status"] != "online" {
		t.Fatalf("expected status=online from LLM, got %v", receivedArgs["status"])
	}
}

func TestGatewayRequiresMappedPlanAndStepInAssistantMode(t *testing.T) {
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		RequireMappedPlan: true,
	})
	if _, err := gateway.Prepare(context.Background(), agentruntime.ToolRequest{
		StepID:   "step_1",
		ToolName: "Host.List",
	}); err == nil {
		t.Fatal("assistant mode must reject a tool request without a Mapping-bound plan")
	}

	gateway = NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		RequireMappedPlan: true,
		ExecutionPlan: &ToolExecutionPlan{Steps: []ToolPlanStep{{
			StepID:   "step_1",
			ToolName: "Host.List",
		}}},
	})
	if _, err := gateway.Prepare(context.Background(), agentruntime.ToolRequest{
		ToolName: "Host.List",
	}); err == nil {
		t.Fatal("assistant mode must reject a mapped tool request without step_id")
	}
}

// TestToolDispatcher_RequiresApprovalTriggersApprovalInWhitelistMode verifies that
// a tool flagged RequiresApproval: true still triggers an approval request when the
// approval mode is the default whitelist (no full_access override). This guards
// against accidentally dropping the tool-level approval override.
func TestToolDispatcher_RequiresApprovalTriggersApprovalInWhitelistMode(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:               "Requires.Approval.Tool",
		Risk:               ToolRiskMedium,
		RequiresApproval:   true,
		DefaultWhitelisted: false,
		Enabled:            true,
		ExposurePolicy: ToolExposurePolicy{
			DirectCallable: true,
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"result": "ok"}, nil
		},
	})

	dispatcher, _ := newTestToolDispatcher(t, registry)

	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		ToolName:  "Requires.Approval.Tool",
		Args:      map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ApprovalRequired {
		t.Fatalf("expected RequiresApproval tool to trigger approval in whitelist mode, got direct execution")
	}
	if result.ApprovalID == "" {
		t.Fatalf("expected non-empty approval ID")
	}
}

// TestToolDispatcher_FullAccessBypassesApprovalForRequiresApprovalTool verifies the
// core regression: when approval mode is full_access, a tool flagged
// RequiresApproval: true executes directly without creating an approval.
func TestToolDispatcher_FullAccessBypassesApprovalForRequiresApprovalTool(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:               "Requires.Approval.Tool",
		Risk:               ToolRiskHigh,
		RequiresApproval:   true,
		DefaultWhitelisted: false,
		Enabled:            true,
		ExposurePolicy: ToolExposurePolicy{
			DirectCallable: true,
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"result": "ok"}, nil
		},
	})

	dispatcher, _ := newTestToolDispatcherWithMode(t, registry, model.ApprovalModeFullAccess)

	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "test-session",
		ToolName:  "Requires.Approval.Tool",
		Args:      map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ApprovalRequired {
		t.Fatalf("full_access must not require approval for RequiresApproval tool, got ApprovalRequired=true (id=%s)", result.ApprovalID)
	}
	if !result.Success {
		t.Fatalf("expected direct execution success in full_access mode, got failure: %s", result.Error)
	}
}

func TestToolDispatcherUsesRunApprovalModeSnapshot(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:               "Snapshot.HighRisk.Tool",
		Risk:               ToolRiskHigh,
		RequiresApproval:   true,
		DefaultWhitelisted: false,
		Enabled:            true,
		ExposurePolicy:     ToolExposurePolicy{DirectCallable: true},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"result": "ok"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	dispatcher, _ := newTestToolDispatcherWithMode(t, registry, model.ApprovalModeWhitelist)
	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID:    "test-session",
		RunID:        "run-full-access",
		ToolName:     "Snapshot.HighRisk.Tool",
		Args:         map[string]interface{}{},
		ApprovalMode: model.ApprovalModeFullAccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ApprovalRequired || !result.Success {
		t.Fatalf("run full-access snapshot must execute directly: %#v", result)
	}

	dispatcher, _ = newTestToolDispatcherWithMode(t, registry, model.ApprovalModeFullAccess)
	result, err = dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID:    "test-session",
		RunID:        "run-whitelist",
		ToolName:     "Snapshot.HighRisk.Tool",
		Args:         map[string]interface{}{},
		ApprovalMode: model.ApprovalModeWhitelist,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ApprovalRequired {
		t.Fatalf("run whitelist snapshot must remain approval-gated: %#v", result)
	}
}
