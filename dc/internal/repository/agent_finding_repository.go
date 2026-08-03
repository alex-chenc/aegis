package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"dc/internal/model"
	"dc/internal/pipeline"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAgentGuardFindingEvidenceMissing = errors.New("agent guard finding evidence missing")

const maxFindingEvidenceEvents = 256

type AgentSecurityFindingRepository struct {
	db *gorm.DB
}

func NewAgentSecurityFindingRepository(db *gorm.DB) *AgentSecurityFindingRepository {
	return &AgentSecurityFindingRepository{db: db}
}

func (r *AgentSecurityFindingRepository) ListBehaviorWindow(
	ctx context.Context,
	event *model.AgentBehaviorEvent,
	window time.Duration,
) ([]*model.AgentBehaviorEvent, error) {
	if event == nil || event.InstanceID == nil || event.SessionID == nil ||
		event.ExecutionUnitID == nil || window <= 0 {
		return nil, fmt.Errorf("agent behavior correlation scope is incomplete")
	}
	var events []*model.AgentBehaviorEvent
	err := r.db.WithContext(ctx).
		Where("host_id = ?", event.HostID).
		Where("instance_id = ?", *event.InstanceID).
		Where("session_id = ?", *event.SessionID).
		Where("execution_unit_id = ?", *event.ExecutionUnitID).
		Where("occurred_at BETWEEN ? AND ?", event.OccurredAt.Add(-window), event.OccurredAt.Add(window)).
		Order("occurred_at ASC, agent_sequence ASC").
		Limit(512).
		Find(&events).Error
	return events, err
}

func (r *AgentSecurityFindingRepository) ListRemoteBehaviorEvidence(
	ctx context.Context,
	selectors []pipeline.RemoteEvidenceSelector,
	window time.Duration,
) ([]*model.AgentBehaviorEvent, error) {
	query, validSelectors, err := buildRemoteBehaviorEvidenceQuery(r.db.WithContext(ctx), selectors, window)
	if err != nil || len(validSelectors) == 0 {
		return nil, err
	}
	var candidates []*model.AgentBehaviorEvent
	if err := query.Limit(len(validSelectors)).Find(&candidates).Error; err != nil {
		return nil, err
	}
	result := make([]*model.AgentBehaviorEvent, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		for _, selector := range validSelectors {
			if !pipeline.MatchesTrustedRemoteEvidence(selector, candidate, window) {
				continue
			}
			if _, exists := seen[candidate.RawEventID]; !exists {
				seen[candidate.RawEventID] = struct{}{}
				result = append(result, candidate)
			}
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RawEventID < result[j].RawEventID })
	return result, nil
}

func buildRemoteBehaviorEvidenceQuery(
	db *gorm.DB,
	selectors []pipeline.RemoteEvidenceSelector,
	window time.Duration,
) (*gorm.DB, []pipeline.RemoteEvidenceSelector, error) {
	if db == nil || window <= 0 {
		return nil, nil, fmt.Errorf("remote behavior evidence query is invalid")
	}
	if len(selectors) > 128 {
		return nil, nil, fmt.Errorf("remote behavior evidence selector limit exceeded")
	}
	valid := make([]pipeline.RemoteEvidenceSelector, 0, len(selectors))
	clauses := make([]string, 0, len(selectors))
	args := make([]any, 0, len(selectors)*7)
	for _, selector := range selectors {
		if !pipeline.ValidRemoteEvidenceSelector(selector) {
			return nil, nil, fmt.Errorf("remote behavior evidence selector is invalid")
		}
		valid = append(valid, selector)
		clauses = append(clauses,
			"(raw_event_id = ? AND host_id = ? AND execution_unit_id = ? AND occurred_at BETWEEN ? AND ? AND evidence ->> 'correlation_token_hash' = ?)",
		)
		args = append(args,
			selector.EventID, selector.HostID, selector.ExecutionUnitID,
			selector.ToolOccurredAt.Add(-window), selector.ToolOccurredAt.Add(window), selector.CorrelationHash,
		)
	}
	if len(valid) == 0 {
		return db, valid, nil
	}
	query := db.Model(&model.AgentBehaviorEvent{}).
		Where("category IN ?", []string{"process", "file", "network", "identity", "persistence", "isolation", "kernel", "ipc"}).
		Where("command_visibility = 'complete'").
		Where("collection ->> 'source' IN ?", []string{"ebpf", "procfs"}).
		Where("collection ->> 'attribution_confidence' = 'confirmed'").
		Where("collection ->> 'visibility' = 'complete'").
		Where("COALESCE((collection ->> 'lost_events_since_last')::bigint, -1) = 0").
		Where("COALESCE((collection ->> 'aggregated_count')::bigint, -1) = 1").
		Where("jsonb_typeof(collection -> 'truncated_fields') = 'array'").
		Where("jsonb_array_length(collection -> 'truncated_fields') = 0").
		Where("collection ->> 'coverage_level' IN ?", []string{"full_enforcement", "behavior_monitor_escape_enforce", "monitor_only", "no_isolation"}).
		Where("("+strings.Join(clauses, " OR ")+")", args...)
	return query, valid, nil
}

func (r *AgentSecurityFindingRepository) UpsertAgentFinding(
	ctx context.Context,
	finding *model.AgentSecurityFinding,
	alertEnabled bool,
) (pipeline.AgentFindingWriteResult, error) {
	result := pipeline.AgentFindingWriteResult{}
	if finding == nil || finding.ID == uuid.Nil || finding.FindingKey == "" {
		return result, fmt.Errorf("agent security finding is incomplete")
	}
	evidenceIDs, err := findingEvidenceIDs(finding.EvidenceEventIDs)
	if err != nil {
		return result, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateFindingEvidence(tx, evidenceIDs, finding.EvidenceSourceTable); err != nil {
			return err
		}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(finding)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 1 {
			result = pipeline.AgentFindingWriteResult{
				FindingID: finding.ID,
				Created:   true,
				Changed:   true,
			}
			if alertEnabled {
				return upsertFindingAlert(tx, finding, len(evidenceIDs))
			}
			return nil
		}

		var existing model.AgentSecurityFinding
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("finding_key = ?", finding.FindingKey).
			First(&existing).Error; err != nil {
			return err
		}
		updates, changed, err := mergeAgentFinding(&existing, finding)
		if err != nil {
			return err
		}
		result.FindingID = existing.ID
		if !changed {
			return nil
		}
		if err := tx.Model(&model.AgentSecurityFinding{}).
			Where("id = ?", existing.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		result.Changed = true
		effective := *finding
		effective.ID = existing.ID
		effective.Severity = updates["severity"].(string)
		effective.Verdict = updates["verdict"].(string)
		effective.Confidence = updates["confidence"].(float64)
		effective.FirstObservedAt = updates["first_observed_at"].(time.Time)
		effective.LastObservedAt = updates["last_observed_at"].(time.Time)
		effective.EvidenceEventIDs = updates["evidence_event_ids"].(json.RawMessage)
		mergedEvidence, _ := findingEvidenceIDs(effective.EvidenceEventIDs)
		if alertEnabled {
			return upsertFindingAlert(tx, &effective, len(mergedEvidence))
		}
		return nil
	})
	return result, err
}

func validateFindingEvidence(tx *gorm.DB, evidenceIDs []string, source string) error {
	seen := make(map[string]struct{}, len(evidenceIDs))
	if source == "agent_behavior_events" || source == "agent_guard_events" {
		var IDs []string
		if err := tx.Model(&model.AgentBehaviorEvent{}).
			Where("raw_event_id IN ?", evidenceIDs).
			Pluck("raw_event_id", &IDs).Error; err != nil {
			return err
		}
		for _, eventID := range IDs {
			seen[eventID] = struct{}{}
		}
	}
	if source == "runtime_events" || source == "agent_guard_events" {
		var IDs []string
		if err := tx.Model(&model.RuntimeEvent{}).
			Where("event_id IN ?", evidenceIDs).
			Pluck("event_id", &IDs).Error; err != nil {
			return err
		}
		for _, eventID := range IDs {
			seen[eventID] = struct{}{}
		}
	}
	if source != "agent_behavior_events" && source != "runtime_events" && source != "agent_guard_events" {
		return fmt.Errorf("%w: unsupported evidence source", ErrAgentGuardFindingEvidenceMissing)
	}
	if len(seen) != len(evidenceIDs) {
		return ErrAgentGuardFindingEvidenceMissing
	}
	return nil
}

func findingEvidenceIDs(raw json.RawMessage) ([]string, error) {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil ||
		len(values) == 0 || len(values) > maxFindingEvidenceEvents {
		return nil, ErrAgentGuardFindingEvidenceMissing
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, err := uuid.Parse(value); err != nil {
			return nil, ErrAgentGuardFindingEvidenceMissing
		}
		if _, exists := seen[value]; exists {
			return nil, ErrAgentGuardFindingEvidenceMissing
		}
		seen[value] = struct{}{}
	}
	sort.Strings(values)
	return values, nil
}

func mergeAgentFinding(
	existing *model.AgentSecurityFinding,
	incoming *model.AgentSecurityFinding,
) (map[string]any, bool, error) {
	existingEvidence, err := findingEvidenceIDs(existing.EvidenceEventIDs)
	if err != nil {
		return nil, false, err
	}
	incomingEvidence, err := findingEvidenceIDs(incoming.EvidenceEventIDs)
	if err != nil {
		return nil, false, err
	}
	evidence := append(existingEvidence, incomingEvidence...)
	sort.Strings(evidence)
	evidence = compactStrings(evidence)
	evidenceRaw, _ := json.Marshal(evidence)

	severity := strongestValue(existing.Severity, incoming.Severity, severityRank)
	verdict := strongestValue(existing.Verdict, incoming.Verdict, verdictRank)
	confidence := existing.Confidence
	if incoming.Confidence > confidence {
		confidence = incoming.Confidence
	}
	firstObservedAt := existing.FirstObservedAt
	if incoming.FirstObservedAt.Before(firstObservedAt) {
		firstObservedAt = incoming.FirstObservedAt
	}
	lastObservedAt := existing.LastObservedAt
	if incoming.LastObservedAt.After(lastObservedAt) {
		lastObservedAt = incoming.LastObservedAt
	}
	ruleHits := preferRicherJSON(existing.RuleHits, incoming.RuleHits)
	evidenceGraph := preferRicherJSON(existing.EvidenceGraph, incoming.EvidenceGraph)
	attackStages := preferRicherJSON(existing.AttackStages, incoming.AttackStages)
	decisionSources := preferRicherJSON(existing.DecisionSources, incoming.DecisionSources)
	title := existing.Title
	if incoming.Title != "" {
		title = incoming.Title
	}
	summary := existing.Summary
	if incoming.Summary != "" {
		summary = incoming.Summary
	}
	recommendedAction := strongestValue(existing.RecommendedAction, incoming.RecommendedAction, actionRank)

	updates := map[string]any{
		"title":              title,
		"severity":           severity,
		"verdict":            verdict,
		"confidence":         confidence,
		"decision_sources":   decisionSources,
		"rule_hits":          ruleHits,
		"evidence_event_ids": json.RawMessage(evidenceRaw),
		"evidence_graph":     evidenceGraph,
		"attack_stages":      attackStages,
		"summary":            summary,
		"recommended_action": recommendedAction,
		"first_observed_at":  firstObservedAt,
		"last_observed_at":   lastObservedAt,
		"updated_at":         time.Now().UTC(),
	}
	changed := title != existing.Title ||
		severity != existing.Severity ||
		verdict != existing.Verdict ||
		confidence != existing.Confidence ||
		!jsonEqual(decisionSources, existing.DecisionSources) ||
		!jsonEqual(ruleHits, existing.RuleHits) ||
		!jsonEqual(evidenceRaw, existing.EvidenceEventIDs) ||
		!jsonEqual(evidenceGraph, existing.EvidenceGraph) ||
		!jsonEqual(attackStages, existing.AttackStages) ||
		summary != existing.Summary ||
		recommendedAction != existing.RecommendedAction ||
		!firstObservedAt.Equal(existing.FirstObservedAt) ||
		!lastObservedAt.Equal(existing.LastObservedAt)
	if !changed {
		return nil, false, nil
	}
	return updates, true, nil
}

func upsertFindingAlert(tx *gorm.DB, finding *model.AgentSecurityFinding, hitCount int) error {
	alertID := "AGF-" + finding.ID.String()
	now := time.Now().UTC()
	values := map[string]any{
		"alert_id":        alertID,
		"host_id":         finding.HostID,
		"pid":             0,
		"mitre_id":        "",
		"severity":        finding.Severity,
		"description":     finding.Title,
		"dedupe_key":      alertID,
		"hit_count":       hitCount,
		"status":          "active",
		"auto_blocked":    false,
		"manual_blocked":  false,
		"judgment_source": "agent_guard_rule",
		"rule_id":         firstFindingRuleKey(finding.RuleHits),
		"first_seen_at":   finding.FirstObservedAt,
		"last_seen_at":    finding.LastObservedAt,
		"created_at":      now,
		"updated_at":      now,
	}
	return tx.Table("alerts").Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "alert_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"severity":        gorm.Expr("EXCLUDED.severity"),
			"description":     gorm.Expr("EXCLUDED.description"),
			"hit_count":       gorm.Expr("GREATEST(alerts.hit_count, EXCLUDED.hit_count)"),
			"last_seen_at":    gorm.Expr("GREATEST(alerts.last_seen_at, EXCLUDED.last_seen_at)"),
			"judgment_source": gorm.Expr("EXCLUDED.judgment_source"),
			"rule_id":         gorm.Expr("EXCLUDED.rule_id"),
			"updated_at":      gorm.Expr("EXCLUDED.updated_at"),
		}),
	}).Create(values).Error
}

func firstFindingRuleKey(raw json.RawMessage) string {
	var hits []map[string]any
	if json.Unmarshal(raw, &hits) != nil || len(hits) == 0 {
		return ""
	}
	value, _ := hits[0]["rule_key"].(string)
	return value
}

func strongestValue(current, incoming string, rank func(string) int) string {
	if rank(incoming) > rank(current) {
		return incoming
	}
	return current
}

func severityRank(value string) int {
	return map[string]int{"info": 1, "low": 2, "medium": 3, "high": 4, "critical": 5}[value]
}

func verdictRank(value string) int {
	return map[string]int{"benign": 1, "inconclusive": 2, "suspicious": 3, "malicious": 4}[value]
}

func actionRank(value string) int {
	return map[string]int{"": 0, "audit": 1, "alert": 2}[value]
}

func preferRicherJSON(current, incoming json.RawMessage) json.RawMessage {
	if len(incoming) > len(current) && json.Valid(incoming) {
		return incoming
	}
	return current
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return string(left) == string(right)
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}
