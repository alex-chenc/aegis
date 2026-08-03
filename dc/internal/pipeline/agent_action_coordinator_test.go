package pipeline

import (
	"context"
	"errors"
	"testing"

	"dc/internal/model"

	"github.com/google/uuid"
)

type fakeActionContextResolver struct {
	input AgentActionEligibilityInput
}

func (f fakeActionContextResolver) ResolveActionEligibility(
	context.Context,
	*model.AgentSecurityFinding,
	AgentActionFeatureFlags,
) (AgentActionEligibilityInput, error) {
	return f.input, nil
}

type fakeActionStore struct {
	action   *model.AgentGuardAction
	statuses []string
	errors   []string
}

func (f *fakeActionStore) UpsertAgentGuardAction(
	_ context.Context,
	action *model.AgentGuardAction,
) (string, error) {
	f.action = action
	return action.Status, nil
}

func (f *fakeActionStore) UpdateAgentGuardActionDispatch(
	_ context.Context,
	_ uuid.UUID,
	status, errorCode, _ string,
) error {
	f.statuses = append(f.statuses, status)
	f.errors = append(f.errors, errorCode)
	return nil
}

type fakeActionPublisher struct {
	commands []AgentGuardBlockCommand
	err      error
}

func (f *fakeActionPublisher) PublishAgentGuardAction(
	_ context.Context,
	command AgentGuardBlockCommand,
) error {
	f.commands = append(f.commands, command)
	return f.err
}

func TestAgentActionCoordinatorPublishesStableUnitCommandAndNeverClaimsSuccess(t *testing.T) {
	unitID, instanceID := uuid.New(), uuid.New()
	finding := &model.AgentSecurityFinding{
		ID: uuid.New(), FindingKey: "correlation:v1:AGB-DOWNLOAD-EXEC-001:anchor",
		HostID: uuid.New(), InstanceID: &instanceID, ExecutionUnitID: &unitID,
	}
	input := AgentActionEligibilityInput{
		RequestedAction: "freeze_execution_unit", AttributionConfidence: "confirmed",
		ExecutionUnitID: &unitID, CoverageLevel: "full_enforcement", FindingVerdict: "malicious",
		RuleEvidence: true, NonToolEvidence: true, EvidenceResolved: true, EvidenceVisibility: "complete",
		PublishedPolicy: true, PolicyAuthorized: true,
		DecisionSources:   []string{"agent_guard_rule", "event_correlation"},
		BehaviorDecisions: []string{"audit"}, FreezeTimeoutSeconds: 420,
	}
	store := &fakeActionStore{}
	publisher := &fakeActionPublisher{}
	coordinator := NewAgentActionCoordinator(fakeActionContextResolver{input: input}, store, publisher)
	update, err := coordinator.ConsiderFinding(context.Background(), finding, AgentActionFeatureFlags{
		ActionEnabled: true, FreezeEnabled: true, PublishEnabled: true,
	})
	if err != nil {
		t.Fatalf("ConsiderFinding: %v", err)
	}
	if update == nil || update.Status != "dispatching" || len(publisher.commands) != 1 ||
		publisher.commands[0].Target != unitID.String() || publisher.commands[0].HostID != finding.HostID.String() ||
		store.action == nil || store.action.FreezeTimeoutSeconds == nil ||
		*store.action.FreezeTimeoutSeconds != 420 || len(store.statuses) != 1 || store.statuses[0] != "dispatching" {
		t.Fatalf("update=%#v commands=%#v statuses=%v", update, publisher.commands, store.statuses)
	}
	if update.Status == "success" {
		t.Fatal("DC forged action execution success")
	}
}

func TestAgentActionCoordinatorRecordsTransportFailureAndAsyncDenyDegrade(t *testing.T) {
	unitID, instanceID := uuid.New(), uuid.New()
	finding := &model.AgentSecurityFinding{
		ID: uuid.New(), HostID: uuid.New(), InstanceID: &instanceID, ExecutionUnitID: &unitID,
	}
	base := AgentActionEligibilityInput{
		AttributionConfidence: "confirmed", ExecutionUnitID: &unitID,
		CoverageLevel: "full_enforcement", FindingVerdict: "malicious", RuleEvidence: true, NonToolEvidence: true,
		EvidenceResolved: true, EvidenceVisibility: "complete", PublishedPolicy: true,
		PolicyAuthorized: true, DecisionSources: []string{"agent_guard_rule"},
		BehaviorDecisions: []string{"audit"},
	}

	t.Run("publisher failure", func(t *testing.T) {
		input := base
		input.RequestedAction = "freeze_execution_unit"
		store := &fakeActionStore{}
		publisher := &fakeActionPublisher{err: errors.New("kafka unavailable")}
		coordinator := NewAgentActionCoordinator(fakeActionContextResolver{input: input}, store, publisher)
		update, err := coordinator.ConsiderFinding(context.Background(), finding, AgentActionFeatureFlags{
			ActionEnabled: true, FreezeEnabled: true, PublishEnabled: true,
		})
		if err == nil || update == nil || update.Status != "failed" ||
			store.errors[len(store.errors)-1] != "action_publish_failed" {
			t.Fatalf("update=%#v err=%v store=%#v", update, err, store)
		}
	})

	t.Run("async deny", func(t *testing.T) {
		input := base
		input.RequestedAction = "deny"
		store := &fakeActionStore{}
		publisher := &fakeActionPublisher{}
		coordinator := NewAgentActionCoordinator(fakeActionContextResolver{input: input}, store, publisher)
		update, err := coordinator.ConsiderFinding(context.Background(), finding, AgentActionFeatureFlags{
			ActionEnabled: true, DenyEnabled: true, PublishEnabled: true,
		})
		if err != nil || update == nil || update.Status != "failed" ||
			update.ErrorCode != "dc_async_deny_not_dispatchable" || len(publisher.commands) != 0 {
			t.Fatalf("update=%#v err=%v commands=%v", update, err, publisher.commands)
		}
	})
}
