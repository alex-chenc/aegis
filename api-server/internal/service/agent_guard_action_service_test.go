package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type fakeAgentGuardActionStore struct {
	unit        *model.AgentExecutionUnit
	instance    *model.AgentRuntimeInstance
	delivery    *model.AgentGuardPolicyDelivery
	active      *model.AgentGuardAction
	created     *model.AgentGuardAction
	resolveCall int
}

func (f *fakeAgentGuardActionStore) ResolveExecutionUnit(context.Context, uuid.UUID) (*model.AgentExecutionUnit, *model.AgentRuntimeInstance, *model.AgentGuardPolicyDelivery, error) {
	f.resolveCall++
	return f.unit, f.instance, f.delivery, nil
}
func (f *fakeAgentGuardActionStore) ResolveInstance(context.Context, uuid.UUID) (*model.AgentRuntimeInstance, *model.AgentGuardPolicyDelivery, error) {
	f.resolveCall++
	return f.instance, f.delivery, nil
}
func (f *fakeAgentGuardActionStore) FindActiveFreeze(context.Context, uuid.UUID) (*model.AgentGuardAction, error) {
	if f.active == nil {
		return nil, repository.ErrAgentGuardActionNotFound
	}
	return f.active, nil
}
func (f *fakeAgentGuardActionStore) CreateOrGetActiveFreeze(_ context.Context, action *model.AgentGuardAction) (*model.AgentGuardAction, bool, error) {
	if f.active != nil {
		return f.active, false, nil
	}
	copy := *action
	f.created = &copy
	f.active = &copy
	return &copy, true, nil
}
func (f *fakeAgentGuardActionStore) Create(_ context.Context, action *model.AgentGuardAction) error {
	copy := *action
	f.created = &copy
	return nil
}
func (f *fakeAgentGuardActionStore) GetByID(context.Context, uuid.UUID) (*model.AgentGuardAction, error) {
	if f.created != nil {
		return f.created, nil
	}
	return f.active, nil
}
func (f *fakeAgentGuardActionStore) GetByCommandID(context.Context, string) (*model.AgentGuardAction, error) {
	if f.created != nil {
		return f.created, nil
	}
	return f.active, nil
}
func (f *fakeAgentGuardActionStore) Transition(
	_ context.Context,
	_ uuid.UUID,
	status string,
	result datatypes.JSON,
	errorCode string,
	errorMessage string,
	at time.Time,
) (*model.AgentGuardAction, error) {
	target := f.created
	if target == nil {
		target = f.active
	}
	copy := *target
	copy.Status = status
	copy.Result = result
	copy.ErrorCode = errorCode
	copy.ErrorMessage = errorMessage
	if status == model.AgentGuardActionStatusDispatching {
		copy.DispatchedAt = &at
	}
	if status == model.AgentGuardActionStatusFailed || status == model.AgentGuardActionStatusSuccess {
		copy.CompletedAt = &at
	}
	f.created = &copy
	return &copy, nil
}

type fakeAgentGuardActionClient struct {
	connected bool
	response  *pb.ExecuteBlockCommandResponse
	err       error
	request   *pb.ExecuteBlockCommandRequest
}

func (f *fakeAgentGuardActionClient) GetAgentStatus(context.Context, string) (*pb.GetAgentStatusResponse, error) {
	return &pb.GetAgentStatusResponse{Connected: f.connected}, nil
}
func (f *fakeAgentGuardActionClient) ExecuteBlockCommand(_ context.Context, request *pb.ExecuteBlockCommandRequest) (*pb.ExecuteBlockCommandResponse, error) {
	f.request = request
	return f.response, f.err
}

func TestAgentGuardActionServiceDispatchesExactUnitTargetWithoutClaimingSuccess(t *testing.T) {
	hostID, instanceID, unitID := uuid.New(), uuid.New(), uuid.New()
	store := &fakeAgentGuardActionStore{
		unit: &model.AgentExecutionUnit{
			ID: unitID, HostID: hostID, InstanceID: instanceID, UnitType: "linux_cgroup_v2",
			CoverageLevel: model.AgentGuardCoverageFullEnforcement, Status: "observed",
		},
		instance: &model.AgentRuntimeInstance{
			ID: instanceID, HostID: hostID, DetectionConfidence: "confirmed", Status: "running",
		},
		delivery: &model.AgentGuardPolicyDelivery{
			HostID: hostID, Status: "applied",
			CapabilitySnapshot: datatypes.JSON(`{"cgroup_freeze":true,"pidfd":true}`),
		},
	}
	client := &fakeAgentGuardActionClient{
		connected: true,
		response:  &pb.ExecuteBlockCommandResponse{Success: true},
	}
	service := NewAgentGuardActionService(store, client, true, nil)
	action, err := service.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionFreezeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "confirmed namespace escape"},
		"security-admin",
	)
	if err != nil {
		t.Fatalf("request freeze: %v", err)
	}
	if action.Status != model.AgentGuardActionStatusDispatching {
		t.Fatalf("status=%q, want dispatching (Server acceptance is not Agent success)", action.Status)
	}
	if action.CommandID != "AG-GUARD-"+action.ID.String() {
		t.Fatalf("command_id=%q must encode action id %s", action.CommandID, action.ID)
	}
	if client.request == nil || client.request.Target != unitID.String() ||
		client.request.HostId != hostID.String() ||
		client.request.Action != model.AgentGuardActionFreezeExecutionUnit {
		t.Fatalf("unsafe dispatch request: %#v", client.request)
	}
	if store.created == nil || store.created.Source != model.AgentGuardActionSourceManual ||
		store.created.RequestedBy != "security-admin" || store.created.FreezeTimeoutSeconds != nil {
		t.Fatalf("unexpected persisted audit action: %#v", store.created)
	}
}

func TestAgentGuardActionServiceMapsManualHoldToAtomicHoldAction(t *testing.T) {
	hostID, instanceID, unitID := uuid.New(), uuid.New(), uuid.New()
	store := &fakeAgentGuardActionStore{
		unit: &model.AgentExecutionUnit{
			ID: unitID, HostID: hostID, InstanceID: instanceID,
			CoverageLevel: model.AgentGuardCoverageFullEnforcement, Status: "observed",
		},
		instance: &model.AgentRuntimeInstance{
			ID: instanceID, HostID: hostID, DetectionConfidence: "confirmed", Status: "running",
		},
		delivery: &model.AgentGuardPolicyDelivery{
			Status: "applied", CapabilitySnapshot: datatypes.JSON(`{"cgroup_freeze":true}`),
		},
	}
	client := &fakeAgentGuardActionClient{
		connected: true, response: &pb.ExecuteBlockCommandResponse{Success: true},
	}
	service := NewAgentGuardActionService(store, client, true, nil)
	action, err := service.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionFreezeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "manual investigation hold", Hold: true}, "security-admin",
	)
	if err != nil {
		t.Fatalf("request hold: %v", err)
	}
	if action.Action != model.AgentGuardActionHoldExecutionUnit || !action.HoldRequested ||
		action.ExpiresAt != nil || client.request == nil || client.request.Action != model.AgentGuardActionHoldExecutionUnit {
		t.Fatalf("hold was not atomic and explicit: action=%#v request=%#v", action, client.request)
	}
}

func TestAgentGuardActionServiceRejectsDisabledRemoteAndResumeConflictBeforeDispatch(t *testing.T) {
	unitID := uuid.New()
	store := &fakeAgentGuardActionStore{}
	client := &fakeAgentGuardActionClient{}
	disabled := NewAgentGuardActionService(store, client, false, nil)
	_, err := disabled.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionFreezeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "manual containment"}, "admin",
	)
	if !errors.Is(err, ErrAgentGuardActionsDisabled) || store.resolveCall != 0 {
		t.Fatalf("disabled err=%v resolve_calls=%d", err, store.resolveCall)
	}

	hostID, instanceID := uuid.New(), uuid.New()
	store.unit = &model.AgentExecutionUnit{
		ID: unitID, HostID: hostID, InstanceID: instanceID, RemoteBackend: "ssh",
		CoverageLevel: model.AgentGuardCoverageRemoteUnobservable, Status: "observed",
	}
	store.instance = &model.AgentRuntimeInstance{
		ID: instanceID, HostID: hostID, DetectionConfidence: "confirmed", Status: "running",
	}
	store.delivery = &model.AgentGuardPolicyDelivery{CapabilitySnapshot: datatypes.JSON(`{"pidfd":true}`)}
	enabled := NewAgentGuardActionService(store, client, true, nil)
	_, err = enabled.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionFreezeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "manual containment"}, "admin",
	)
	if !errors.Is(err, ErrAgentGuardRemoteUnobservable) || client.request != nil || store.created != nil {
		t.Fatalf("remote action err=%v request=%#v created=%#v", err, client.request, store.created)
	}

	store.unit.RemoteBackend = ""
	store.unit.CoverageLevel = model.AgentGuardCoverageFullEnforcement
	_, err = enabled.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionResumeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "resume after review"}, "admin",
	)
	if !errors.Is(err, ErrAgentGuardUnitStateConflict) || client.request != nil || store.created != nil {
		t.Fatalf("resume conflict err=%v request=%#v created=%#v", err, client.request, store.created)
	}

	store.unit.Status = "observed"
	store.unit.CoverageLevel = model.AgentGuardCoverageMonitorOnly
	_, err = enabled.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionFreezeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "manual containment"}, "admin",
	)
	if !errors.Is(err, ErrAgentGuardActionNotSupported) || client.request != nil || store.created != nil {
		t.Fatalf("monitor-only action err=%v request=%#v created=%#v", err, client.request, store.created)
	}

	store.unit.CoverageLevel = model.AgentGuardCoverageFullEnforcement
	store.delivery.Status = "received"
	_, err = enabled.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionFreezeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "manual containment"}, "admin",
	)
	if !errors.Is(err, ErrAgentGuardActionNotSupported) || client.request != nil || store.created != nil {
		t.Fatalf("unapplied capability action err=%v request=%#v created=%#v", err, client.request, store.created)
	}
}

func TestAgentGuardActionServicePersistsDispatchFailure(t *testing.T) {
	hostID, instanceID, unitID := uuid.New(), uuid.New(), uuid.New()
	store := &fakeAgentGuardActionStore{
		unit: &model.AgentExecutionUnit{
			ID: unitID, HostID: hostID, InstanceID: instanceID, UnitType: "linux_cgroup_v2",
			CoverageLevel: model.AgentGuardCoverageFullEnforcement, Status: "observed",
		},
		instance: &model.AgentRuntimeInstance{
			ID: instanceID, HostID: hostID, DetectionConfidence: "confirmed", Status: "running",
		},
		delivery: &model.AgentGuardPolicyDelivery{Status: "applied", CapabilitySnapshot: datatypes.JSON(`{"cgroup_freeze":true}`)},
	}
	client := &fakeAgentGuardActionClient{
		connected: true,
		response:  &pb.ExecuteBlockCommandResponse{Success: false, Error: "protected target"},
	}
	service := NewAgentGuardActionService(store, client, true, nil)
	action, err := service.RequestExecutionUnit(
		context.Background(), unitID, model.AgentGuardActionFreezeExecutionUnit,
		AgentGuardManualActionRequest{Reason: "manual containment"}, "admin",
	)
	if !errors.Is(err, ErrAgentGuardActionDispatchFailed) || action == nil ||
		action.Status != model.AgentGuardActionStatusFailed || action.ErrorCode == "" {
		t.Fatalf("dispatch failure action=%#v err=%v", action, err)
	}
}

func TestAgentGuardActionServiceAppliesOnlyOwnedAgentStatusWithExecutionEvidence(t *testing.T) {
	hostID, instanceID, unitID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC()
	action := newManualActionForStatus(hostID, instanceID, unitID, now)
	store := &fakeAgentGuardActionStore{created: action}
	service := NewAgentGuardActionService(store, &fakeAgentGuardActionClient{}, false, nil)

	report := AgentGuardActionStatusReport{
		Schema: agentGuardActionStatusSchema, ActionID: action.ID.String(), CommandID: action.CommandID,
		HostID: hostID.String(), InstanceID: instanceID.String(), ExecutionUnitID: unitID.String(),
		Action: action.Action, Status: model.AgentGuardActionStatusSuccess,
		Method: "cgroup_v2", Executed: true, StateChanged: true,
	}
	updated, err := service.ApplyReportedStatus(context.Background(), report)
	if err != nil || updated.Status != model.AgentGuardActionStatusSuccess || updated.CompletedAt == nil {
		t.Fatalf("status update=%#v err=%v", updated, err)
	}
	if !strings.Contains(string(updated.Result), `"method":"cgroup_v2"`) ||
		!strings.Contains(string(updated.Result), `"state_changed":true`) {
		t.Fatalf("bounded action result=%s", updated.Result)
	}

	store.created = action
	report.HostID = uuid.NewString()
	if _, err := service.ApplyReportedStatus(context.Background(), report); !errors.Is(err, ErrAgentGuardActionOwnershipInvalid) {
		t.Fatalf("foreign host report error=%v", err)
	}
	report.HostID = hostID.String()
	report.Executed, report.StateChanged = false, false
	if _, err := service.ApplyReportedStatus(context.Background(), report); !errors.Is(err, ErrAgentGuardActionRequestInvalid) {
		t.Fatalf("success without state evidence error=%v", err)
	}
}

func newManualActionForStatus(hostID, instanceID, unitID uuid.UUID, now time.Time) *model.AgentGuardAction {
	actionID := uuid.New()
	return &model.AgentGuardAction{
		ID: actionID, CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID,
		InstanceID: &instanceID, ExecutionUnitID: &unitID,
		Action: model.AgentGuardActionFreezeExecutionUnit, Source: model.AgentGuardActionSourceManual,
		Status: model.AgentGuardActionStatusDispatching, Reason: "manual containment",
		RequestedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}
