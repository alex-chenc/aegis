package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dc/internal/model"
	"dc/internal/pipeline"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAgentGuardActionDependencyMissing = errors.New("agent guard action dependency missing")

func (r *AgentSecurityFindingRepository) ResolveActionEligibility(
	ctx context.Context,
	finding *model.AgentSecurityFinding,
	flags pipeline.AgentActionFeatureFlags,
) (pipeline.AgentActionEligibilityInput, error) {
	input := pipeline.AgentActionEligibilityInput{
		ExecutionUnitID: finding.ExecutionUnitID,
		FindingVerdict:  finding.Verdict,
		Flags:           flags,
	}
	_ = json.Unmarshal(finding.DecisionSources, &input.DecisionSources)
	ruleKeys := findingRuleKeys(finding.RuleHits)
	input.RuleEvidence = len(ruleKeys) > 0
	evidenceIDs, evidenceErr := findingEvidenceIDs(finding.EvidenceEventIDs)
	if evidenceErr == nil {
		input.EvidenceResolved = validateFindingEvidence(r.db.WithContext(ctx), evidenceIDs, finding.EvidenceSourceTable) == nil
	}
	input.EvidenceVisibility = findingEvidenceVisibility(finding.EvidenceGraph)
	if finding.SessionID != nil {
		var session model.AgentBehaviorSession
		if err := r.db.WithContext(ctx).Select("confidence").First(&session, "id = ?", *finding.SessionID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return input, err
			}
		} else {
			input.AttributionConfidence = session.Confidence
		}
	}
	if finding.ExecutionUnitID != nil {
		var unit model.AgentExecutionUnit
		if err := r.db.WithContext(ctx).Select("coverage_level").First(&unit, "id = ?", *finding.ExecutionUnitID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return input, err
			}
		} else {
			input.CoverageLevel = unit.CoverageLevel
		}
	}
	if len(evidenceIDs) > 0 {
		if finding.EvidenceSourceTable != "agent_behavior_events" {
			input.NonToolEvidence = true
		} else {
			var categories []string
			if err := r.db.WithContext(ctx).Model(&model.AgentBehaviorEvent{}).
				Where("raw_event_id IN ?", evidenceIDs).
				Distinct("category").
				Pluck("category", &categories).Error; err != nil {
				return input, err
			}
			for _, category := range categories {
				if category != "tool" {
					input.NonToolEvidence = true
					break
				}
			}
		}
		if err := r.db.WithContext(ctx).Model(&model.AgentBehaviorEvent{}).
			Where("raw_event_id IN ?", evidenceIDs).
			Distinct("decision").
			Pluck("decision", &input.BehaviorDecisions).Error; err != nil {
			return input, err
		}
	}
	if finding.PolicyID == nil || finding.PolicyVersion == nil {
		return input, nil
	}
	var policy model.AgentGuardPolicy
	err := r.db.WithContext(ctx).
		Where("id = ? AND version = ?", *finding.PolicyID, *finding.PolicyVersion).
		First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return input, nil
	}
	if err != nil {
		return input, err
	}
	input.PublishedPolicy = policy.Status == "published" && policy.PublishedAt != nil
	input.FreezeTimeoutSeconds = policy.FreezeTimeoutSeconds
	input.RequestedAction, input.PolicyAuthorized = policyActionAuthorization(policy, ruleKeys)
	return input, nil
}

func (r *AgentSecurityFindingRepository) UpsertAgentGuardAction(
	ctx context.Context,
	action *model.AgentGuardAction,
) (string, error) {
	if action == nil || action.ID == uuid.Nil || action.TriggerFindingID == nil ||
		action.InstanceID == nil || action.ExecutionUnitID == nil {
		return "", ErrAgentGuardActionDependencyMissing
	}
	if action.Status == "failed" && action.CompletedAt == nil {
		now := time.Now().UTC()
		action.CompletedAt = &now
	}
	status := ""
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var dependencyCount int64
		if err := tx.Model(&model.AgentExecutionUnit{}).
			Where("id = ? AND host_id = ? AND instance_id = ?", *action.ExecutionUnitID, action.HostID, *action.InstanceID).
			Count(&dependencyCount).Error; err != nil {
			return err
		}
		if dependencyCount != 1 {
			return ErrAgentGuardActionDependencyMissing
		}
		if err := tx.Model(&model.AgentSecurityFinding{}).
			Where("id = ? AND host_id = ?", *action.TriggerFindingID, action.HostID).
			Count(&dependencyCount).Error; err != nil {
			return err
		}
		if dependencyCount != 1 {
			return ErrAgentGuardActionDependencyMissing
		}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(action)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 1 {
			status = action.Status
			return updateFindingActionAlert(tx, action, status, action.ErrorMessage)
		}
		return tx.Model(&model.AgentGuardAction{}).
			Where("id = ?", action.ID).
			Pluck("status", &status).Error
	})
	return status, err
}

func (r *AgentSecurityFindingRepository) UpdateAgentGuardActionDispatch(
	ctx context.Context,
	actionID uuid.UUID,
	status, errorCode, errorMessage string,
) error {
	if !containsActionDispatchStatus(status) {
		return fmt.Errorf("invalid Agent Guard dispatch status %q", status)
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"status": status, "error_code": errorCode,
		"error_message": errorMessage, "updated_at": now,
	}
	if status == "dispatching" {
		updates["dispatched_at"] = now
	}
	if status == "failed" {
		updates["completed_at"] = now
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentGuardAction{}).
			Where("id = ? AND status = 'pending'", actionID).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAgentGuardActionStateMissing
		}
		var action model.AgentGuardAction
		if err := tx.First(&action, "id = ?", actionID).Error; err != nil {
			return err
		}
		return updateFindingActionAlert(tx, &action, status, errorMessage)
	})
}

func policyActionAuthorization(policy model.AgentGuardPolicy, ruleKeys []string) (string, bool) {
	type policyRule struct {
		RuleID string `json:"rule_id"`
		Rule   string `json:"rule"`
		Action string `json:"action"`
	}
	for _, raw := range []json.RawMessage{policy.CorrelationRules, policy.AtomicRules, policy.EscapeRules} {
		var rules []policyRule
		if json.Unmarshal(raw, &rules) != nil {
			continue
		}
		for _, rule := range rules {
			key := rule.RuleID
			if key == "" {
				key = rule.Rule
			}
			if !stringInSlice(ruleKeys, key) {
				continue
			}
			switch rule.Action {
			case "freeze_execution_unit", "deny_and_freeze":
				return "freeze_execution_unit", true
			case "deny":
				return "deny", true
			}
		}
	}
	return "", false
}

func findingRuleKeys(raw json.RawMessage) []string {
	var hits []map[string]any
	if json.Unmarshal(raw, &hits) != nil {
		return nil
	}
	result := make([]string, 0, len(hits))
	for _, hit := range hits {
		if key, ok := hit["rule_key"].(string); ok && key != "" {
			result = append(result, key)
		}
	}
	return result
}

func findingEvidenceVisibility(raw json.RawMessage) string {
	var graph struct {
		Completeness struct {
			Visibility string `json:"visibility"`
		} `json:"completeness"`
	}
	if json.Unmarshal(raw, &graph) != nil {
		return ""
	}
	return graph.Completeness.Visibility
}

func stringInSlice(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsActionDispatchStatus(status string) bool {
	return status == "dispatching" || status == "failed"
}

func updateFindingActionAlert(tx *gorm.DB, action *model.AgentGuardAction, status, message string) error {
	if action.TriggerFindingID == nil {
		return nil
	}
	alertID := "AGF-" + action.TriggerFindingID.String()
	updates := map[string]any{
		"block_status":  status,
		"block_message": strings.TrimSpace(message),
		"updated_at":    time.Now().UTC(),
	}
	if status == "success" {
		updates["auto_blocked"] = true
	}
	return tx.Table("alerts").Where("alert_id = ?", alertID).Updates(updates).Error
}
