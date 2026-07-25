package assistant

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/recovery"
	"api-server/internal/repository"
	"api-server/internal/service"
	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type testRecoverableError struct{}

func (testRecoverableError) Error() string { return "typed recoverable blocker" }

func (testRecoverableError) RecoveryDescriptor() recovery.Descriptor {
	return recovery.Descriptor{
		Code:      "test_blocker",
		Category:  recovery.CategoryRecoverableBusinessBlocker,
		Summary:   "A test blocker needs a decision.",
		RiskLevel: model.RiskHigh,
		Context:   map[string]interface{}{"safe": true},
		Actions: []recovery.Action{{
			ID:        "pause",
			Label:     "Pause",
			RiskLevel: model.RiskReadonly,
		}},
	}
}

type testProposalError struct{}

func (testProposalError) Error() string { return "proposal blocker" }
func (testProposalError) RecoveryDescriptor() recovery.Descriptor {
	return recovery.Descriptor{
		Code:      "proposal_blocker",
		Category:  recovery.CategoryRecoverableBusinessBlocker,
		Summary:   "proposal is available",
		RiskLevel: model.RiskReadonly,
		Actions: []recovery.Action{{
			ID:        "inspect",
			Label:     "Inspect",
			RiskLevel: model.RiskReadonly,
			Executor:  "test",
			KeepsOpen: true,
		}, {
			ID:        "cancel",
			Label:     "Cancel",
			RiskLevel: model.RiskReadonly,
		}},
	}
}

type testRecoveryExecutor struct{}

func (testRecoveryExecutor) ExecuteRecoveryAction(
	context.Context,
	*model.AssistantRecoveryRequest,
	recovery.Action,
	map[string]interface{},
	string,
) (map[string]interface{}, error) {
	return map[string]interface{}{"proposal": "ready"}, nil
}

type fakeRecoveryRepo struct {
	requests map[string]*model.AssistantRecoveryRequest
}

func newFakeRecoveryRepo() *fakeRecoveryRepo {
	return &fakeRecoveryRepo{requests: make(map[string]*model.AssistantRecoveryRequest)}
}

func (f *fakeRecoveryRepo) Create(_ context.Context, request *model.AssistantRecoveryRequest) error {
	copy := *request
	f.requests[request.RecoveryID] = &copy
	return nil
}

func (f *fakeRecoveryRepo) FindByRecoveryID(_ context.Context, recoveryID string) (*model.AssistantRecoveryRequest, error) {
	request, ok := f.requests[recoveryID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *request
	return &copy, nil
}

func (f *fakeRecoveryRepo) FindPendingByToolCall(_ context.Context, toolCallID, code string) (*model.AssistantRecoveryRequest, error) {
	for _, request := range f.requests {
		if request.ToolCallID == toolCallID && request.Code == code &&
			(request.Status == model.RecoveryStatusPending ||
				request.Status == model.RecoveryStatusExecuting ||
				request.Status == model.RecoveryStatusPaused) {
			copy := *request
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeRecoveryRepo) FindActiveByRunStepCode(
	_ context.Context,
	runID, stepID, toolName, code string,
) (*model.AssistantRecoveryRequest, error) {
	for _, request := range f.requests {
		normalizedRequestStep := strings.TrimSpace(request.StepID)
		if normalizedRequestStep == "" {
			normalizedRequestStep = request.ToolName
		}
		normalizedStep := strings.TrimSpace(stepID)
		if normalizedStep == "" {
			normalizedStep = toolName
		}
		if request.RunID == runID &&
			normalizedRequestStep == normalizedStep &&
			request.ToolName == toolName &&
			request.Code == code &&
			(request.Status == model.RecoveryStatusPending ||
				request.Status == model.RecoveryStatusExecuting ||
				request.Status == model.RecoveryStatusPaused) {
			copy := *request
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeRecoveryRepo) FindPendingByRun(_ context.Context, runID string) (*model.AssistantRecoveryRequest, error) {
	for _, request := range f.requests {
		if request.RunID == runID && request.Status == model.RecoveryStatusPending {
			copy := *request
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeRecoveryRepo) ListBySession(_ context.Context, sessionID string, _, _ int) ([]model.AssistantRecoveryRequest, int64, error) {
	var requests []model.AssistantRecoveryRequest
	for _, request := range f.requests {
		if request.SessionID == sessionID {
			requests = append(requests, *request)
		}
	}
	return requests, int64(len(requests)), nil
}

func (f *fakeRecoveryRepo) BeginDecision(_ context.Context, recoveryID, actionID, operator string, input datatypes.JSON) error {
	request, ok := f.requests[recoveryID]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	if request.Status != model.RecoveryStatusPending && request.Status != model.RecoveryStatusPaused {
		return repository.ErrRecoveryAlreadyDecided
	}
	now := time.Now()
	request.Status = model.RecoveryStatusExecuting
	request.SelectedActionID = actionID
	request.DecidedBy = operator
	request.DecisionInput = input
	request.DecidedAt = &now
	return nil
}

func (f *fakeRecoveryRepo) CompleteDecision(_ context.Context, recoveryID, status string, result datatypes.JSON, resumeRunID string) error {
	request, ok := f.requests[recoveryID]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	request.Status = status
	request.ResolutionResult = result
	request.ResumeRunID = resumeRunID
	if status == model.RecoveryStatusResolved {
		now := time.Now()
		request.ResolvedAt = &now
	}
	return nil
}

func TestToolDispatcherPersistsTypedRecoverableBlocker(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:               "Test.Recoverable",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("wrapped handler failure: %w", testRecoverableError{})
		},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	dispatcher, toolCalls := newTestToolDispatcher(t, registry)
	recoveryRepo := newFakeRecoveryRepo()
	dispatcher.SetRecoveryManager(NewRecoveryManager(recoveryRepo, nil))

	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "session-1",
		RunID:     "run-1",
		MessageID: "message-1",
		CallID:    "call-1",
		ToolName:  "Test.Recoverable",
		UserQuery: "perform the original task",
		Args:      map[string]interface{}{"scope": "safe"},
		Approved:  true,
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if !result.Blocked || result.Recovery == nil {
		t.Fatalf("typed blocker was not converted to recovery: %#v", result)
	}
	if result.Recovery.OriginalQuery != "perform the original task" {
		t.Fatalf("original query was not persisted: %#v", result.Recovery)
	}
	if toolCalls.markFailedCall == nil || len(toolCalls.calls) != 1 ||
		toolCalls.calls[0].Status != model.ToolCallStatusBlocked {
		t.Fatalf("tool call was not marked blocked: %#v", toolCalls.calls)
	}
}

func TestRecoveryManagerReusesActiveBlockerAcrossToolCallRetries(t *testing.T) {
	repo := newFakeRecoveryRepo()
	manager := NewRecoveryManager(repo, nil)
	first, handled, err := manager.CreateFromError(context.Background(), RecoveryCreateRequest{
		SessionID:  "session-1",
		RunID:      "run-1",
		MessageID:  "message-1",
		StepID:     "authorized_01",
		ToolCallID: "call-1",
		ToolName:   "Test.Recoverable",
		Error:      testRecoverableError{},
	})
	if err != nil || !handled {
		t.Fatalf("create first recovery: handled=%v err=%v", handled, err)
	}
	second, handled, err := manager.CreateFromError(context.Background(), RecoveryCreateRequest{
		SessionID:  "session-1",
		RunID:      "run-1",
		MessageID:  "message-1",
		StepID:     "authorized_01",
		ToolCallID: "call-2",
		ToolName:   "Test.Recoverable",
		Error:      testRecoverableError{},
	})
	if err != nil || !handled {
		t.Fatalf("create retried recovery: handled=%v err=%v", handled, err)
	}
	if second.RecoveryID != first.RecoveryID {
		t.Fatalf("tool retry created a second recovery: first=%s second=%s", first.RecoveryID, second.RecoveryID)
	}
	if len(repo.requests) != 1 {
		t.Fatalf("expected one active recovery request, got %d", len(repo.requests))
	}
}

func TestAssistantToolGatewayStopsRunAfterRecoverableBlocker(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:               "Test.Recoverable",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return nil, testRecoverableError{}
		},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	dispatcher, _ := newTestToolDispatcher(t, registry)
	recoveryRepo := newFakeRecoveryRepo()
	dispatcher.SetRecoveryManager(NewRecoveryManager(recoveryRepo, nil))
	runManager := NewRunManager()
	run := runManager.Start("session-1")
	runtimeCtx, stopRuntime := context.WithCancel(run.Context())
	defer stopRuntime()
	recoveryEvents := 0
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:      dispatcher,
		SessionID:       "session-1",
		RunID:           run.RunID,
		MessageID:       "message-1",
		RunManager:      runManager,
		StopForRecovery: stopRuntime,
		OnRecovery: func(interface{}) {
			recoveryEvents++
		},
	})

	response, err := gateway.Call(runtimeCtx, agentruntime.ToolRequest{
		CallID:   "call-1",
		StepID:   "authorized_01",
		ToolName: "Test.Recoverable",
		Args:     map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("gateway call: %v", err)
	}
	if response.Status != agentruntime.ToolCallFailed {
		t.Fatalf("blocked tool response status = %s, want failed evidence", response.Status)
	}
	select {
	case <-runtimeCtx.Done():
	default:
		t.Fatal("recoverable blocker did not stop the agent-runtime execution context")
	}
	select {
	case <-run.Context().Done():
		t.Fatal("recovery pause cancelled the parent run context")
	default:
	}
	waiting := run.GetWaitingRecovery()
	if waiting == nil || waiting.RecoveryID == "" || waiting.ToolCallID != "call-1" {
		t.Fatalf("active run did not retain recovery pause state: %#v", waiting)
	}
	if recoveryEvents != 1 {
		t.Fatalf("recovery event count = %d, want 1", recoveryEvents)
	}
}

func TestToolDispatcherDoesNotInventRecoveryForOrdinaryError(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(&ToolSpec{
		Name:               "Test.Terminal",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("ordinary unclassified error")
		},
	})
	dispatcher, _ := newTestToolDispatcher(t, registry)
	recoveryRepo := newFakeRecoveryRepo()
	dispatcher.SetRecoveryManager(NewRecoveryManager(recoveryRepo, nil))

	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "session-1",
		RunID:     "run-1",
		CallID:    "call-terminal",
		ToolName:  "Test.Terminal",
		Args:      map[string]interface{}{},
		Approved:  true,
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if result.Blocked || result.Recovery != nil || len(recoveryRepo.requests) != 0 {
		t.Fatalf("ordinary error must fail closed without recovery: %#v", result)
	}
}

func TestRecoveryManagerRejectsActionNotInPersistedContract(t *testing.T) {
	repo := newFakeRecoveryRepo()
	manager := NewRecoveryManager(repo, nil)
	request, handled, err := manager.CreateFromError(context.Background(), RecoveryCreateRequest{
		SessionID:  "session-1",
		RunID:      "run-1",
		ToolCallID: "call-1",
		ToolName:   "Test.Recoverable",
		Error:      testRecoverableError{},
		OriginalArgs: map[string]interface{}{
			"scope":      "safe",
			"auth_token": "must-not-persist",
		},
	})
	if err != nil || !handled {
		t.Fatalf("create recovery: handled=%v err=%v", handled, err)
	}
	if _, err := manager.Decide(context.Background(), request.RecoveryID, RecoveryDecisionRequest{
		ActionID: "invented_by_client",
	}, "admin"); err == nil {
		t.Fatal("client-invented recovery action was accepted")
	}
	stored, _ := repo.FindByRecoveryID(context.Background(), request.RecoveryID)
	if stored.Status != model.RecoveryStatusPending {
		t.Fatalf("invalid action changed recovery state: %s", stored.Status)
	}
	if string(stored.OriginalArgs) == "" || string(stored.OriginalArgs) == `{"auth_token":"must-not-persist","scope":"safe"}` ||
		!strings.Contains(string(stored.OriginalArgs), `"auth_token":"***"`) {
		t.Fatalf("sensitive recovery arguments were not redacted: %s", stored.OriginalArgs)
	}
}

func TestRecoveryProposalKeepsRequestOpenForLaterDecision(t *testing.T) {
	repo := newFakeRecoveryRepo()
	manager := NewRecoveryManager(repo, nil)
	manager.RegisterExecutor("test", testRecoveryExecutor{})
	request, _, err := manager.CreateFromError(context.Background(), RecoveryCreateRequest{
		SessionID:  "session-1",
		RunID:      "run-1",
		ToolCallID: "call-proposal",
		ToolName:   "Test.Proposal",
		Error:      testProposalError{},
	})
	if err != nil {
		t.Fatalf("create proposal recovery: %v", err)
	}
	result, err := manager.Decide(context.Background(), request.RecoveryID, RecoveryDecisionRequest{
		ActionID: "inspect",
	}, "admin")
	if err != nil {
		t.Fatalf("inspect proposal: %v", err)
	}
	if result.Recovery.Status != model.RecoveryStatusPaused {
		t.Fatalf("proposal closed the original blocker: %s", result.Recovery.Status)
	}
	if _, err := manager.Decide(context.Background(), request.RecoveryID, RecoveryDecisionRequest{
		ActionID: "cancel",
	}, "admin"); err != nil {
		t.Fatalf("paused recovery did not accept a later declared decision: %v", err)
	}
}

func TestRecoveryManagerResumesWithOperatorGuidance(t *testing.T) {
	repo := newFakeRecoveryRepo()
	manager := NewRecoveryManager(repo, nil)
	request := &model.AssistantRecoveryRequest{
		ID:         uuid.New(),
		RecoveryID: "recovery-guidance",
		Status:     model.RecoveryStatusPending,
		Actions: mustMarshalJSON([]recovery.Action{{
			ID:            "provide_other",
			Label:         "Provide other guidance",
			RiskLevel:     model.RiskReadonly,
			Executor:      recoveryResumeExecutor,
			InputRequired: true,
			ResumesRun:    true,
		}}),
		DecisionInput:    datatypes.JSON([]byte("{}")),
		ResolutionResult: datatypes.JSON([]byte("{}")),
	}
	if err := repo.Create(context.Background(), request); err != nil {
		t.Fatalf("create recovery: %v", err)
	}

	result, err := manager.Decide(context.Background(), request.RecoveryID, RecoveryDecisionRequest{
		ActionID: "provide_other",
		Input:    map[string]interface{}{"comment": "regenerate without collecting file paths"},
	}, "admin")
	if err != nil {
		t.Fatalf("decide guidance recovery: %v", err)
	}
	if !result.ResumeRequest || result.Recovery.Status != model.RecoveryStatusResolved {
		t.Fatalf("operator guidance did not resolve and resume: %#v", result)
	}
	if !strings.Contains(string(result.Recovery.DecisionInput), "regenerate without collecting file paths") {
		t.Fatalf("operator guidance was not persisted: %s", result.Recovery.DecisionInput)
	}
}

func TestMergeRequiredHooksIsIdempotentAndPreservesExistingConfig(t *testing.T) {
	config := &service.AllowlistConfig{
		Tracepoints: []string{"syscalls/sys_enter_socket"},
		Kprobes:     []string{"do_sys_open"},
	}
	added, err := mergeRequiredHooks(config, []recoveryHookRequirement{
		{AttachType: "tracepoint", Attach: "syscalls/sys_enter_socket"},
		{AttachType: "tracepoint", Attach: "syscalls/sys_enter_splice"},
	})
	if err != nil {
		t.Fatalf("merge required hooks: %v", err)
	}
	if len(added) != 1 || added[0] != "tracepoint:syscalls/sys_enter_splice" {
		t.Fatalf("unexpected hook diff: %#v", added)
	}
	if len(config.Kprobes) != 1 || config.Kprobes[0] != "do_sys_open" {
		t.Fatalf("existing allowlist category was lost: %#v", config)
	}
}
