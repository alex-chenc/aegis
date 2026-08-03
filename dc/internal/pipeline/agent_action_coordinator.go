package pipeline

import (
	"context"
	"fmt"

	"dc/internal/model"

	"github.com/google/uuid"
)

type AgentGuardBlockCommand struct {
	CommandID string `json:"command_id"`
	HostID    string `json:"host_id"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Reason    string `json:"reason"`
}

type AgentGuardActionUpdate struct {
	ActionID  uuid.UUID
	CommandID string
	Action    string
	Status    string
	ErrorCode string
}

type AgentActionContextResolver interface {
	ResolveActionEligibility(
		context.Context,
		*model.AgentSecurityFinding,
		AgentActionFeatureFlags,
	) (AgentActionEligibilityInput, error)
}

type AgentActionStore interface {
	UpsertAgentGuardAction(context.Context, *model.AgentGuardAction) (string, error)
	UpdateAgentGuardActionDispatch(context.Context, uuid.UUID, string, string, string) error
}

type AgentActionPublisher interface {
	PublishAgentGuardAction(context.Context, AgentGuardBlockCommand) error
}

type AgentActionCoordinator struct {
	resolver  AgentActionContextResolver
	store     AgentActionStore
	publisher AgentActionPublisher
}

func NewAgentActionCoordinator(
	resolver AgentActionContextResolver,
	store AgentActionStore,
	publisher AgentActionPublisher,
) *AgentActionCoordinator {
	return &AgentActionCoordinator{resolver: resolver, store: store, publisher: publisher}
}

func (c *AgentActionCoordinator) ConsiderFinding(
	ctx context.Context,
	finding *model.AgentSecurityFinding,
	flags AgentActionFeatureFlags,
) (*AgentGuardActionUpdate, error) {
	if c == nil || c.resolver == nil || c.store == nil || finding == nil || !flags.ActionEnabled {
		return nil, nil
	}
	input, err := c.resolver.ResolveActionEligibility(ctx, finding, flags)
	if err != nil {
		return nil, err
	}
	input.Flags = flags
	eligibility := EvaluateAgentGuardActionEligibility(input)
	if !eligibility.Eligible {
		return &AgentGuardActionUpdate{
			Action: input.RequestedAction, Status: "not_eligible", ErrorCode: eligibility.ReasonCode,
		}, nil
	}
	freezeTimeoutSeconds := input.FreezeTimeoutSeconds
	if freezeTimeoutSeconds < 30 || freezeTimeoutSeconds > 900 {
		freezeTimeoutSeconds = 300
	}
	action := BuildAgentGuardActionCandidate(finding, input.RequestedAction, freezeTimeoutSeconds)
	if action == nil {
		return nil, fmt.Errorf("eligible Agent Guard action has incomplete target")
	}
	if !eligibility.Dispatchable {
		action.Status = "failed"
		action.ErrorCode = eligibility.ReasonCode
		action.ErrorMessage = "DC cannot retroactively execute an Agent-local deny decision"
	}
	status, err := c.store.UpsertAgentGuardAction(ctx, action)
	if err != nil {
		return nil, err
	}
	update := &AgentGuardActionUpdate{
		ActionID: action.ID, CommandID: action.CommandID, Action: action.Action,
		Status: status, ErrorCode: action.ErrorCode,
	}
	if !eligibility.Dispatchable || status != "pending" {
		return update, nil
	}
	if c.publisher == nil {
		update.Status, update.ErrorCode = "failed", "action_transport_unavailable"
		err := c.store.UpdateAgentGuardActionDispatch(
			ctx, action.ID, update.Status, update.ErrorCode, "Agent Guard action publisher is unavailable",
		)
		return update, err
	}
	command := AgentGuardBlockCommand{
		CommandID: action.CommandID,
		HostID:    action.HostID.String(),
		Action:    action.Action,
		Target:    action.ExecutionUnitID.String(),
		Reason:    action.Reason,
	}
	if err := c.publisher.PublishAgentGuardAction(ctx, command); err != nil {
		update.Status, update.ErrorCode = "failed", "action_publish_failed"
		if updateErr := c.store.UpdateAgentGuardActionDispatch(
			ctx, action.ID, update.Status, update.ErrorCode, err.Error(),
		); updateErr != nil {
			return update, updateErr
		}
		return update, err
	}
	update.Status = "dispatching"
	if err := c.store.UpdateAgentGuardActionDispatch(ctx, action.ID, update.Status, "", ""); err != nil {
		return update, err
	}
	return update, nil
}
