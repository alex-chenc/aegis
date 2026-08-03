package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

const (
	agentGuardActionStatusSchema    = "aegis.agent_guard.v1"
	agentGuardActionDispatchTimeout = 30 * time.Second
	agentGuardActionMaxReasonBytes  = 1000
)

var (
	ErrAgentGuardActionsDisabled        = errors.New("agent guard actions are disabled")
	ErrAgentGuardActionRequestInvalid   = errors.New("agent guard action request is invalid")
	ErrAgentGuardAgentOffline           = errors.New("agent guard agent is offline")
	ErrAgentGuardActionNotSupported     = errors.New("agent guard action is not supported")
	ErrAgentGuardUnitStateConflict      = errors.New("agent guard unit state conflict")
	ErrAgentGuardRemoteUnobservable     = errors.New("agent guard remote execution is unobservable")
	ErrAgentGuardActionDispatchFailed   = errors.New("agent guard action dispatch failed")
	ErrAgentGuardActionOwnershipInvalid = errors.New("agent guard action target ownership is invalid")
)

type AgentGuardManualActionRequest struct {
	Reason string `json:"reason"`
	Hold   bool   `json:"hold"`
}

type AgentGuardActionStatusReport struct {
	Schema          string          `json:"schema"`
	ActionID        string          `json:"action_id"`
	CommandID       string          `json:"command_id"`
	HostID          string          `json:"host_id"`
	Action          string          `json:"action"`
	Status          string          `json:"status"`
	InstanceID      string          `json:"instance_id"`
	ExecutionUnitID string          `json:"execution_unit_id"`
	Result          json.RawMessage `json:"result"`
	Method          string          `json:"method"`
	Degraded        bool            `json:"degraded"`
	AutoResume      bool            `json:"auto_resume"`
	Executed        bool            `json:"executed"`
	StateChanged    bool            `json:"state_changed"`
	ErrorCode       string          `json:"error_code"`
	ErrorMessage    string          `json:"error_message"`
}

type AgentGuardActionStore interface {
	ResolveExecutionUnit(context.Context, uuid.UUID) (*model.AgentExecutionUnit, *model.AgentRuntimeInstance, *model.AgentGuardPolicyDelivery, error)
	ResolveInstance(context.Context, uuid.UUID) (*model.AgentRuntimeInstance, *model.AgentGuardPolicyDelivery, error)
	FindActiveFreeze(context.Context, uuid.UUID) (*model.AgentGuardAction, error)
	CreateOrGetActiveFreeze(context.Context, *model.AgentGuardAction) (*model.AgentGuardAction, bool, error)
	Create(context.Context, *model.AgentGuardAction) error
	GetByID(context.Context, uuid.UUID) (*model.AgentGuardAction, error)
	GetByCommandID(context.Context, string) (*model.AgentGuardAction, error)
	Transition(context.Context, uuid.UUID, string, datatypes.JSON, string, string, time.Time) (*model.AgentGuardAction, error)
}

type AgentGuardActionClient interface {
	GetAgentStatus(context.Context, string) (*pb.GetAgentStatusResponse, error)
	ExecuteBlockCommand(context.Context, *pb.ExecuteBlockCommandRequest) (*pb.ExecuteBlockCommandResponse, error)
}

type AgentGuardActionService struct {
	store   AgentGuardActionStore
	client  AgentGuardActionClient
	enabled bool
	logger  *zap.Logger
	now     func() time.Time
}

func NewAgentGuardActionService(
	store AgentGuardActionStore,
	client AgentGuardActionClient,
	enabled bool,
	logger *zap.Logger,
) *AgentGuardActionService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentGuardActionService{
		store: store, client: client, enabled: enabled, logger: logger, now: time.Now,
	}
}

func (s *AgentGuardActionService) RequestExecutionUnit(
	ctx context.Context,
	unitID uuid.UUID,
	actionName string,
	request AgentGuardManualActionRequest,
	requestedBy string,
) (*model.AgentGuardAction, error) {
	if !s.enabled || s.store == nil || s.client == nil {
		return nil, ErrAgentGuardActionsDisabled
	}
	if actionName != model.AgentGuardActionFreezeExecutionUnit &&
		actionName != model.AgentGuardActionResumeExecutionUnit &&
		actionName != model.AgentGuardActionKillExecutionUnit {
		return nil, ErrAgentGuardActionRequestInvalid
	}
	if err := validateAgentGuardManualActionRequest(actionName, request); err != nil {
		return nil, err
	}
	resolvedAction := actionName
	if actionName == model.AgentGuardActionFreezeExecutionUnit && request.Hold {
		resolvedAction = model.AgentGuardActionHoldExecutionUnit
	}
	unit, instance, delivery, err := s.store.ResolveExecutionUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	if unit == nil || instance == nil || unit.ID != unitID || unit.InstanceID != instance.ID || unit.HostID != instance.HostID {
		return nil, ErrAgentGuardActionOwnershipInvalid
	}
	if resolvedAction == model.AgentGuardActionFreezeExecutionUnit ||
		resolvedAction == model.AgentGuardActionHoldExecutionUnit {
		if active, activeErr := s.store.FindActiveFreeze(ctx, unitID); activeErr == nil {
			return active, nil
		} else if !errors.Is(activeErr, repository.ErrAgentGuardActionNotFound) {
			return nil, activeErr
		}
	}
	if err := validateAgentGuardExecutionUnitAction(unit, instance, delivery, resolvedAction); err != nil {
		return nil, err
	}
	if err := s.ensureHostConnected(ctx, unit.HostID); err != nil {
		return nil, err
	}
	return s.createAndDispatch(
		ctx,
		unit.HostID,
		&instance.ID,
		&unit.ID,
		resolvedAction,
		unit.ID.String(),
		request,
		requestedBy,
	)
}

func (s *AgentGuardActionService) RequestInstanceKill(
	ctx context.Context,
	instanceID uuid.UUID,
	request AgentGuardManualActionRequest,
	requestedBy string,
) (*model.AgentGuardAction, error) {
	if !s.enabled || s.store == nil || s.client == nil {
		return nil, ErrAgentGuardActionsDisabled
	}
	if err := validateAgentGuardManualActionRequest(model.AgentGuardActionKillAgentInstance, request); err != nil {
		return nil, err
	}
	instance, delivery, err := s.store.ResolveInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if instance == nil || instance.ID != instanceID {
		return nil, ErrAgentGuardActionOwnershipInvalid
	}
	if err := validateAgentGuardInstanceAction(instance, delivery); err != nil {
		return nil, err
	}
	if err := s.ensureHostConnected(ctx, instance.HostID); err != nil {
		return nil, err
	}
	return s.createAndDispatch(
		ctx,
		instance.HostID,
		&instance.ID,
		nil,
		model.AgentGuardActionKillAgentInstance,
		instance.ID.String(),
		request,
		requestedBy,
	)
}

func (s *AgentGuardActionService) createAndDispatch(
	ctx context.Context,
	hostID uuid.UUID,
	instanceID *uuid.UUID,
	unitID *uuid.UUID,
	actionName string,
	target string,
	request AgentGuardManualActionRequest,
	requestedBy string,
) (*model.AgentGuardAction, error) {
	requestedBy = strings.TrimSpace(requestedBy)
	if requestedBy == "" {
		return nil, ErrAgentGuardActionRequestInvalid
	}
	now := s.now().UTC()
	actionID := uuid.New()
	action := &model.AgentGuardAction{
		ID: actionID, CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID,
		InstanceID: instanceID, ExecutionUnitID: unitID, Action: actionName,
		Source: model.AgentGuardActionSourceManual, Status: model.AgentGuardActionStatusPending,
		Reason: strings.TrimSpace(request.Reason), RequestedBy: truncateAgentGuardActionText(requestedBy, 100),
		HoldRequested: request.Hold, Result: datatypes.JSON(`{}`),
		RequestedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if actionName == model.AgentGuardActionFreezeExecutionUnit ||
		actionName == model.AgentGuardActionHoldExecutionUnit {
		stored, created, err := s.store.CreateOrGetActiveFreeze(ctx, action)
		if err != nil {
			return nil, err
		}
		if !created {
			return stored, nil
		}
		action = stored
	} else if err := s.store.Create(ctx, action); err != nil {
		return nil, err
	}

	s.logger.Info("agent_guard_manual_action_requested",
		zap.String("action_id", action.ID.String()),
		zap.String("command_id", action.CommandID),
		zap.String("host_id", hostID.String()),
		zap.String("instance_id", agentGuardOptionalUUID(instanceID)),
		zap.String("execution_unit_id", agentGuardOptionalUUID(unitID)),
		zap.String("action", actionName),
		zap.String("requested_by", action.RequestedBy),
		zap.Bool("hold_requested", request.Hold),
	)

	dispatchCtx, cancel := context.WithTimeout(ctx, agentGuardActionDispatchTimeout)
	response, dispatchErr := s.client.ExecuteBlockCommand(dispatchCtx, &pb.ExecuteBlockCommandRequest{
		CommandId: action.CommandID,
		HostId:    hostID.String(),
		Action:    actionName,
		Target:    target,
		Reason:    action.Reason,
	})
	cancel()
	if dispatchErr != nil || response == nil || !response.Success {
		safeMessage := "action dispatch failed"
		if response != nil && strings.TrimSpace(response.Error) != "" {
			safeMessage = truncateAgentGuardActionText(response.Error, 1000)
		}
		failed, transitionErr := s.store.Transition(
			ctx, action.ID, model.AgentGuardActionStatusFailed, nil,
			"agent_guard_action_dispatch_failed", safeMessage, s.now().UTC(),
		)
		if transitionErr != nil {
			return action, fmt.Errorf("persist agent guard dispatch failure: %w", transitionErr)
		}
		s.logger.Warn("agent_guard_manual_action_dispatch_failed",
			zap.String("action_id", action.ID.String()),
			zap.String("command_id", action.CommandID),
			zap.String("host_id", hostID.String()),
			zap.String("action", actionName),
			zap.String("error_code", "agent_guard_action_dispatch_failed"),
			zap.Bool("transport_error", dispatchErr != nil),
		)
		return failed, ErrAgentGuardActionDispatchFailed
	}
	dispatching, err := s.store.Transition(
		ctx, action.ID, model.AgentGuardActionStatusDispatching, nil, "", "", s.now().UTC(),
	)
	if errors.Is(err, repository.ErrAgentGuardActionStateConflict) {
		current, currentErr := s.store.GetByID(ctx, action.ID)
		if currentErr == nil {
			return current, nil
		}
	}
	if err != nil {
		return action, err
	}
	s.logger.Info("agent_guard_manual_action_dispatched",
		zap.String("action_id", action.ID.String()),
		zap.String("command_id", action.CommandID),
		zap.String("host_id", hostID.String()),
		zap.String("action", actionName),
		zap.String("status", dispatching.Status),
	)
	return dispatching, nil
}

func (s *AgentGuardActionService) ensureHostConnected(ctx context.Context, hostID uuid.UUID) error {
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	status, err := s.client.GetAgentStatus(statusCtx, hostID.String())
	cancel()
	if err != nil {
		return fmt.Errorf("%w: agent status unavailable", ErrAgentGuardAgentOffline)
	}
	if status == nil || !status.Connected {
		return ErrAgentGuardAgentOffline
	}
	return nil
}

func (s *AgentGuardActionService) ApplyReportedStatus(
	ctx context.Context,
	report AgentGuardActionStatusReport,
) (*model.AgentGuardAction, error) {
	if s.store == nil || report.Schema != agentGuardActionStatusSchema ||
		strings.TrimSpace(report.CommandID) == "" {
		return nil, ErrAgentGuardActionRequestInvalid
	}
	action, err := s.store.GetByCommandID(ctx, strings.TrimSpace(report.CommandID))
	if err != nil {
		return nil, err
	}
	reportedActionID, parseErr := uuid.Parse(strings.TrimSpace(report.ActionID))
	reportedHostID, hostParseErr := uuid.Parse(strings.TrimSpace(report.HostID))
	if parseErr != nil || reportedActionID != action.ID ||
		hostParseErr != nil || reportedHostID != action.HostID || report.Action != action.Action {
		return action, ErrAgentGuardActionOwnershipInvalid
	}
	if !agentGuardActionTargetMatches(action.InstanceID, report.InstanceID) {
		return action, ErrAgentGuardActionOwnershipInvalid
	}
	if !agentGuardActionTargetMatches(action.ExecutionUnitID, report.ExecutionUnitID) {
		return action, ErrAgentGuardActionOwnershipInvalid
	}
	if !agentGuardReportedActionStatus(report.Status) {
		return action, ErrAgentGuardActionRequestInvalid
	}
	if report.Status == model.AgentGuardActionStatusSuccess && !agentGuardActionReportHasStateEvidence(report) {
		return action, ErrAgentGuardActionRequestInvalid
	}
	result, err := agentGuardActionReportResult(report)
	if err != nil {
		return action, err
	}
	if len(result) > 16*1024 {
		return action, ErrAgentGuardActionRequestInvalid
	}
	updated, err := s.store.Transition(
		ctx,
		action.ID,
		report.Status,
		result,
		truncateAgentGuardActionText(report.ErrorCode, 100),
		truncateAgentGuardActionText(report.ErrorMessage, 1000),
		s.now().UTC(),
	)
	if errors.Is(err, repository.ErrAgentGuardActionStateConflict) {
		current, currentErr := s.store.GetByID(ctx, action.ID)
		if currentErr == nil {
			return current, err
		}
	}
	if err == nil {
		s.logger.Info("agent_guard_action_status_applied",
			zap.String("action_id", action.ID.String()),
			zap.String("command_id", action.CommandID),
			zap.String("host_id", action.HostID.String()),
			zap.String("action", action.Action),
			zap.String("status", report.Status),
			zap.String("error_code", truncateAgentGuardActionText(report.ErrorCode, 100)),
		)
	}
	return updated, err
}

func agentGuardActionTargetMatches(expected *uuid.UUID, reported string) bool {
	reported = strings.TrimSpace(reported)
	if expected == nil {
		return reported == ""
	}
	parsed, err := uuid.Parse(reported)
	return err == nil && parsed == *expected
}

func agentGuardActionReportHasStateEvidence(report AgentGuardActionStatusReport) bool {
	if report.Executed || report.StateChanged {
		return true
	}
	if len(report.Result) == 0 || string(report.Result) == "null" {
		return false
	}
	var nested struct {
		Executed     bool `json:"executed"`
		StateChanged bool `json:"state_changed"`
	}
	return json.Unmarshal(report.Result, &nested) == nil && (nested.Executed || nested.StateChanged)
}

func agentGuardActionReportResult(report AgentGuardActionStatusReport) (datatypes.JSON, error) {
	result := map[string]any{
		"method":        truncateAgentGuardActionText(report.Method, 100),
		"degraded":      report.Degraded,
		"auto_resume":   report.AutoResume,
		"executed":      report.Executed,
		"state_changed": report.StateChanged,
	}
	if len(report.Result) > 0 && string(report.Result) != "null" {
		var nested struct {
			Method       *string `json:"method"`
			Degraded     *bool   `json:"degraded"`
			AutoResume   *bool   `json:"auto_resume"`
			Executed     *bool   `json:"executed"`
			StateChanged *bool   `json:"state_changed"`
		}
		if err := json.Unmarshal(report.Result, &nested); err != nil {
			return nil, ErrAgentGuardActionRequestInvalid
		}
		if nested.Method != nil {
			result["method"] = truncateAgentGuardActionText(*nested.Method, 100)
		}
		for key, value := range map[string]*bool{
			"degraded": nested.Degraded, "auto_resume": nested.AutoResume,
			"executed": nested.Executed, "state_changed": nested.StateChanged,
		} {
			if value != nil {
				result[key] = *value
			}
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 16*1024 {
		return nil, ErrAgentGuardActionRequestInvalid
	}
	return datatypes.JSON(encoded), nil
}

func validateAgentGuardManualActionRequest(actionName string, request AgentGuardManualActionRequest) error {
	reason := strings.TrimSpace(request.Reason)
	if reason == "" || len(reason) > agentGuardActionMaxReasonBytes {
		return ErrAgentGuardActionRequestInvalid
	}
	if actionName != model.AgentGuardActionFreezeExecutionUnit {
		if request.Hold {
			return ErrAgentGuardActionRequestInvalid
		}
	}
	return nil
}

func validateAgentGuardExecutionUnitAction(
	unit *model.AgentExecutionUnit,
	instance *model.AgentRuntimeInstance,
	delivery *model.AgentGuardPolicyDelivery,
	actionName string,
) error {
	if unit.RemoteBackend != "" || unit.CoverageLevel == model.AgentGuardCoverageRemoteUnobservable {
		return ErrAgentGuardRemoteUnobservable
	}
	if instance.DetectionConfidence != "confirmed" {
		return ErrAgentGuardActionOwnershipInvalid
	}
	if unit.CoverageLevel != model.AgentGuardCoverageFullEnforcement &&
		unit.CoverageLevel != model.AgentGuardCoverageBehaviorMonitorEscapeEnforce {
		return ErrAgentGuardActionNotSupported
	}
	if agentGuardTargetTerminal(unit.Status) || unit.StoppedAt != nil || agentGuardTargetTerminal(instance.Status) {
		return ErrAgentGuardUnitStateConflict
	}
	if actionName == model.AgentGuardActionResumeExecutionUnit &&
		unit.FrozenAt == nil && unit.Status != "frozen" && unit.Status != "freezing" {
		return ErrAgentGuardUnitStateConflict
	}
	if (actionName == model.AgentGuardActionFreezeExecutionUnit ||
		actionName == model.AgentGuardActionHoldExecutionUnit) &&
		(unit.FrozenAt != nil || unit.Status == "frozen" || unit.Status == "freezing") {
		return ErrAgentGuardUnitStateConflict
	}
	capabilities, ok := decodeAgentGuardActionCapabilities(delivery)
	if !ok {
		return ErrAgentGuardActionNotSupported
	}
	switch actionName {
	case model.AgentGuardActionFreezeExecutionUnit, model.AgentGuardActionHoldExecutionUnit,
		model.AgentGuardActionResumeExecutionUnit:
		if !capabilities.CgroupFreeze && !capabilities.Pidfd {
			return ErrAgentGuardActionNotSupported
		}
	case model.AgentGuardActionKillExecutionUnit:
		if !capabilities.CgroupFreeze && !capabilities.Pidfd {
			return ErrAgentGuardActionNotSupported
		}
	}
	return nil
}

func validateAgentGuardInstanceAction(
	instance *model.AgentRuntimeInstance,
	delivery *model.AgentGuardPolicyDelivery,
) error {
	if instance.DetectionConfidence != "confirmed" {
		return ErrAgentGuardActionOwnershipInvalid
	}
	if instance.CoverageLevel == model.AgentGuardCoverageRemoteUnobservable {
		return ErrAgentGuardRemoteUnobservable
	}
	if agentGuardTargetTerminal(instance.Status) || instance.StoppedAt != nil {
		return ErrAgentGuardUnitStateConflict
	}
	if instance.CoverageLevel != model.AgentGuardCoverageFullEnforcement &&
		instance.CoverageLevel != model.AgentGuardCoverageBehaviorMonitorEscapeEnforce {
		return ErrAgentGuardActionNotSupported
	}
	capabilities, ok := decodeAgentGuardActionCapabilities(delivery)
	if !ok || !capabilities.Pidfd {
		return ErrAgentGuardActionNotSupported
	}
	return nil
}

type agentGuardActionCapabilities struct {
	CgroupFreeze bool `json:"cgroup_freeze"`
	Pidfd        bool `json:"pidfd"`
}

func decodeAgentGuardActionCapabilities(delivery *model.AgentGuardPolicyDelivery) (agentGuardActionCapabilities, bool) {
	var direct agentGuardActionCapabilities
	if delivery == nil || delivery.Status != "applied" || len(delivery.CapabilitySnapshot) == 0 {
		return direct, false
	}
	if err := json.Unmarshal(delivery.CapabilitySnapshot, &direct); err != nil {
		return direct, false
	}
	if direct.CgroupFreeze || direct.Pidfd {
		return direct, true
	}
	var wrapped struct {
		Capabilities agentGuardActionCapabilities `json:"capabilities"`
	}
	if err := json.Unmarshal(delivery.CapabilitySnapshot, &wrapped); err != nil {
		return direct, false
	}
	if wrapped.Capabilities.CgroupFreeze || wrapped.Capabilities.Pidfd {
		return wrapped.Capabilities, true
	}
	return direct, false
}

func agentGuardTargetTerminal(status string) bool {
	switch status {
	case "stopped", "terminated", "exited", "killed":
		return true
	default:
		return false
	}
}

func agentGuardReportedActionStatus(status string) bool {
	switch status {
	case model.AgentGuardActionStatusDispatching,
		model.AgentGuardActionStatusRunning,
		model.AgentGuardActionStatusSuccess,
		model.AgentGuardActionStatusFailed,
		model.AgentGuardActionStatusExpired,
		model.AgentGuardActionStatusCancelled:
		return true
	default:
		return false
	}
}

func truncateAgentGuardActionText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func agentGuardOptionalUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
