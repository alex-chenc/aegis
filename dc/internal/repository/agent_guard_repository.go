package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dc/internal/model"
	"dc/internal/pipeline"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAgentGuardStateDependencyMissing = errors.New("agent guard state dependency missing")
	ErrAgentGuardDeliveryStateMissing   = errors.New("agent guard delivery state missing")
	ErrAgentGuardActionStateMissing     = errors.New("agent guard action state missing")
	ErrAgentGuardToolProofReplay        = errors.New("agent guard tool proof replay")
)

type AgentBehaviorEventRepository struct {
	db *gorm.DB
}

func NewAgentBehaviorEventRepository(db *gorm.DB) *AgentBehaviorEventRepository {
	return &AgentBehaviorEventRepository{db: db}
}

// CreateWithContext returns true only when a new immutable projection was
// inserted. Both the raw event ID and host/boot/sequence constraints make
// Kafka replay safe.
func (r *AgentBehaviorEventRepository) CreateWithContext(ctx context.Context, event *model.AgentBehaviorEvent) (bool, error) {
	inserted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if event.InstanceID != nil && event.SessionID != nil && event.ExecutionUnitID != nil {
			var dependencyCount int64
			if err := tx.Model(&model.AgentBehaviorSession{}).
				Where(
					"id = ? AND host_id = ? AND instance_id = ? AND execution_unit_id = ?",
					*event.SessionID, event.HostID, *event.InstanceID, *event.ExecutionUnitID,
				).
				Count(&dependencyCount).Error; err != nil {
				return err
			}
			if dependencyCount != 1 {
				return ErrAgentGuardStateDependencyMissing
			}
		}
		if err := rejectReplayedToolProof(tx, event); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(event)
		if result.Error != nil {
			return result.Error
		}
		inserted = result.RowsAffected == 1
		if !inserted || event.SessionID == nil {
			return nil
		}
		result = tx.Model(&model.AgentBehaviorSession{}).
			Where("id = ?", *event.SessionID).
			Updates(map[string]interface{}{
				"behavior_count": gorm.Expr("behavior_count + 1"),
				"last_seen_at":   gorm.Expr("GREATEST(last_seen_at, ?)", event.OccurredAt),
				"completeness": gorm.Expr(
					`jsonb_build_object(
						'visibility', CASE
							WHEN completeness->>'visibility' = 'unobservable' OR ? = 'unobservable' THEN 'unobservable'
							WHEN completeness->>'visibility' = 'partial' OR ? = 'partial' OR ? > 0 OR ? THEN 'partial'
							ELSE 'complete'
						END,
						'lost_events_total', COALESCE((completeness->>'lost_events_total')::bigint, 0) + ?,
						'aggregated_operations_total', COALESCE((completeness->>'aggregated_operations_total')::bigint, 0) + ?,
						'has_truncated_fields', COALESCE((completeness->>'has_truncated_fields')::boolean, false) OR ?,
						'last_collection', ?::jsonb
					)`,
					event.CommandVisibility,
					event.CommandVisibility,
					event.LostEventsSinceLast,
					event.HasTruncatedFields,
					event.LostEventsSinceLast,
					event.AggregatedCount,
					event.HasTruncatedFields,
					string(event.Completeness),
				),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAgentGuardStateDependencyMissing
		}
		return nil
	})
	return inserted, err
}

func rejectReplayedToolProof(tx *gorm.DB, event *model.AgentBehaviorEvent) error {
	digest := pipeline.ToolProofDigest(event)
	if digest == "" {
		return nil
	}
	lockKey := event.HostID.String() + ":" + digest
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockKey).Error; err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&model.AgentBehaviorEvent{}).
		Where("host_id = ? AND category = 'tool' AND raw_event_id <> ?", event.HostID, event.RawEventID).
		Where("evidence -> 'tool_semantics' ->> 'proof_digest' = ?", digest).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrAgentGuardToolProofReplay
	}
	return nil
}

type AgentGuardStateRepository struct {
	db *gorm.DB
}

func NewAgentGuardStateRepository(db *gorm.DB) *AgentGuardStateRepository {
	return &AgentGuardStateRepository{db: db}
}

func (r *AgentGuardStateRepository) UpsertWithContext(
	ctx context.Context,
	projection *model.AgentGuardStateProjection,
) (bool, error) {
	if projection == nil {
		return false, fmt.Errorf("agent guard state projection is nil")
	}
	switch {
	case projection.Delivery != nil:
		delivery := projection.Delivery
		query := r.db.WithContext(ctx).
			Table("agent_guard_policy_deliveries").
			Where("host_id = ? AND bundle_version = ?", delivery.HostID, delivery.BundleVersion)
		if delivery.BundleDigest != "" {
			query = query.Where("bundle_digest = ?", delivery.BundleDigest)
		}
		updates := map[string]interface{}{
			"status": gorm.Expr(
				`CASE
					WHEN status = 'unsupported_agent_version' THEN status
					WHEN ? = 'applied' THEN 'applied'
					WHEN status = 'applied' THEN status
					WHEN status = 'failed' AND ? = 'received' THEN status
					ELSE ?
				END`,
				delivery.Status, delivery.Status, delivery.Status,
			),
			"error_code": gorm.Expr(
				"CASE WHEN status = 'unsupported_agent_version' THEN error_code ELSE ? END",
				delivery.ErrorCode,
			),
			"last_reported_at": gorm.Expr("GREATEST(COALESCE(last_reported_at, ?), ?)", delivery.LastReportedAt, delivery.LastReportedAt),
			"updated_at":       gorm.Expr("GREATEST(updated_at, ?)", delivery.LastReportedAt),
		}
		switch delivery.Status {
		case "received":
			updates["received_at"] = delivery.LastReportedAt
		case "applied":
			updates["applied_at"] = gorm.Expr(
				"CASE WHEN status = 'unsupported_agent_version' THEN applied_at ELSE ? END",
				delivery.LastReportedAt,
			)
			updates["coverage_level"] = gorm.Expr(
				"CASE WHEN status = 'unsupported_agent_version' THEN coverage_level ELSE 'monitor_only' END",
			)
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return false, result.Error
		}
		if result.RowsAffected != 1 {
			return false, ErrAgentGuardDeliveryStateMissing
		}
		return true, nil
	case projection.Action != nil:
		action := projection.Action
		changed := false
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ? AND host_id = ? AND action = ?", action.ID, action.HostID, action.Action)
			if action.CommandID != "" {
				query = query.Where("command_id = ?", action.CommandID)
			}
			if action.ExecutionUnitID != nil {
				query = query.Where("execution_unit_id = ?", *action.ExecutionUnitID)
			}
			if action.InstanceID != nil {
				query = query.Where("instance_id = ?", *action.InstanceID)
			}
			var current model.AgentGuardAction
			if err := query.First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					inserted, insertErr := insertTimeoutAutoResume(tx, action)
					changed = inserted
					return insertErr
				}
				return err
			}
			if !shouldApplyActionStatus(&current, action) {
				return nil
			}
			result := tx.Model(&model.AgentGuardAction{}).
				Where("id = ?", current.ID).
				Updates(map[string]interface{}{
					"status":        action.Status,
					"result":        action.Result,
					"error_code":    action.ErrorCode,
					"error_message": action.ErrorMessage,
					"completed_at":  action.CompletedAt,
					"updated_at":    action.UpdatedAt,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAgentGuardActionStateMissing
			}
			current.Status = action.Status
			current.Result = action.Result
			current.ErrorCode = action.ErrorCode
			current.ErrorMessage = action.ErrorMessage
			current.CompletedAt = action.CompletedAt
			current.UpdatedAt = action.UpdatedAt
			if err := updateFindingActionAlert(tx, &current, action.Status, action.ErrorMessage); err != nil {
				return err
			}
			changed = true
			return nil
		})
		return changed, err
	case projection.Instance != nil:
		if projection.Instance.AssetID == nil {
			assetID, err := r.resolveAgentAssetID(ctx, projection.Instance.HostID, projection.Instance.AgentType)
			if err != nil {
				return false, err
			}
			projection.Instance.AssetID = assetID
		}
		result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"asset_id":             gorm.Expr("COALESCE(agent_runtime_instances.asset_id, EXCLUDED.asset_id)"),
				"profile_key":          gorm.Expr("EXCLUDED.profile_key"),
				"profile_version":      gorm.Expr("GREATEST(agent_runtime_instances.profile_version, EXCLUDED.profile_version)"),
				"agent_type":           gorm.Expr("EXCLUDED.agent_type"),
				"display_name":         gorm.Expr("EXCLUDED.display_name"),
				"controller_exe":       gorm.Expr("EXCLUDED.controller_exe"),
				"run_uid":              gorm.Expr("EXCLUDED.run_uid"),
				"detection_confidence": gorm.Expr("EXCLUDED.detection_confidence"),
				"status": gorm.Expr(
					"CASE WHEN agent_runtime_instances.status = 'stopped' THEN agent_runtime_instances.status ELSE EXCLUDED.status END",
				),
				"coverage_level":   gorm.Expr("EXCLUDED.coverage_level"),
				"coverage_reasons": gorm.Expr("EXCLUDED.coverage_reasons"),
				"last_seen_at":     gorm.Expr("GREATEST(agent_runtime_instances.last_seen_at, EXCLUDED.last_seen_at)"),
				"stopped_at":       gorm.Expr("COALESCE(agent_runtime_instances.stopped_at, EXCLUDED.stopped_at)"),
				"updated_at":       gorm.Expr("GREATEST(agent_runtime_instances.updated_at, EXCLUDED.updated_at)"),
			}),
		}).Create(projection.Instance)
		return result.RowsAffected == 1, result.Error
	case projection.Unit != nil:
		var dependencyCount int64
		if err := r.db.WithContext(ctx).Model(&model.AgentRuntimeInstance{}).
			Where("id = ? AND host_id = ?", projection.Unit.InstanceID, projection.Unit.HostID).
			Count(&dependencyCount).Error; err != nil {
			return false, err
		}
		if dependencyCount != 1 {
			return false, ErrAgentGuardStateDependencyMissing
		}
		result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"unit_type":           gorm.Expr("EXCLUDED.unit_type"),
				"fingerprint":         gorm.Expr("EXCLUDED.fingerprint"),
				"root_pid":            gorm.Expr("EXCLUDED.root_pid"),
				"root_start_ticks":    gorm.Expr("EXCLUDED.root_start_ticks"),
				"cgroup_path":         gorm.Expr("EXCLUDED.cgroup_path"),
				"container_id":        gorm.Expr("EXCLUDED.container_id"),
				"container_runtime":   gorm.Expr("EXCLUDED.container_runtime"),
				"remote_backend":      gorm.Expr("EXCLUDED.remote_backend"),
				"remote_execution_id": gorm.Expr("EXCLUDED.remote_execution_id"),
				"remote_host_ref":     gorm.Expr("EXCLUDED.remote_host_ref"),
				"coverage_level":      gorm.Expr("EXCLUDED.coverage_level"),
				"coverage_reasons":    gorm.Expr("EXCLUDED.coverage_reasons"),
				"isolation_baseline":  gorm.Expr("EXCLUDED.isolation_baseline"),
				"isolation_actual":    gorm.Expr("EXCLUDED.isolation_actual"),
				"isolation_diff":      gorm.Expr("EXCLUDED.isolation_diff"),
				"status": gorm.Expr(
					"CASE WHEN agent_execution_units.status = 'stopped' THEN agent_execution_units.status ELSE EXCLUDED.status END",
				),
				"last_seen_at": gorm.Expr("GREATEST(agent_execution_units.last_seen_at, EXCLUDED.last_seen_at)"),
				"stopped_at":   gorm.Expr("COALESCE(agent_execution_units.stopped_at, EXCLUDED.stopped_at)"),
				"updated_at":   gorm.Expr("GREATEST(agent_execution_units.updated_at, EXCLUDED.updated_at)"),
			}),
		}).Create(projection.Unit)
		return result.RowsAffected == 1, result.Error
	case projection.Session != nil:
		var dependencyCount int64
		if err := r.db.WithContext(ctx).Model(&model.AgentExecutionUnit{}).
			Where(
				"id = ? AND host_id = ? AND instance_id = ?",
				projection.Session.ExecutionUnitID, projection.Session.HostID, projection.Session.InstanceID,
			).
			Count(&dependencyCount).Error; err != nil {
			return false, err
		}
		if dependencyCount != 1 {
			return false, ErrAgentGuardStateDependencyMissing
		}
		result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"execution_unit_id":      gorm.Expr("EXCLUDED.execution_unit_id"),
				"external_session_id":    gorm.Expr("EXCLUDED.external_session_id"),
				"source":                 gorm.Expr("EXCLUDED.source"),
				"confidence":             gorm.Expr("EXCLUDED.confidence"),
				"correlation_token_hash": gorm.Expr("EXCLUDED.correlation_token_hash"),
				"completeness":           gorm.Expr("EXCLUDED.completeness"),
				"status": gorm.Expr(
					"CASE WHEN agent_behavior_sessions.status = 'ended' THEN agent_behavior_sessions.status ELSE EXCLUDED.status END",
				),
				"last_seen_at": gorm.Expr("GREATEST(agent_behavior_sessions.last_seen_at, EXCLUDED.last_seen_at)"),
				"ended_at":     gorm.Expr("COALESCE(agent_behavior_sessions.ended_at, EXCLUDED.ended_at)"),
				"updated_at":   gorm.Expr("GREATEST(agent_behavior_sessions.updated_at, EXCLUDED.updated_at)"),
			}),
		}).Create(projection.Session)
		return result.RowsAffected == 1, result.Error
	default:
		return false, fmt.Errorf("agent guard state projection has no entity")
	}
}

func (r *AgentGuardStateRepository) resolveAgentAssetID(
	ctx context.Context,
	hostID uuid.UUID,
	agentType string,
) (*uuid.UUID, error) {
	aliases := normalizedAgentAssetAliases(agentType)
	if len(aliases) == 0 {
		return nil, nil
	}
	var row struct {
		ID uuid.UUID
	}
	normalized := `LOWER(REPLACE(REPLACE(REPLACE(
		COALESCE(NULLIF(runtime_name, ''), NULLIF(name, ''), ''), '_', ''), '-', ''), ' ', ''))`
	err := r.db.WithContext(ctx).
		Table("host_application_assets").
		Select("id").
		Where("host_id = ? AND category = ? AND status <> ?", hostID, "ai_agent", "deleted").
		Where(normalized+" IN ?", aliases).
		Order("CASE status WHEN 'active' THEN 0 WHEN 'needs_review' THEN 1 ELSE 2 END ASC").
		Order("last_seen_at DESC, id ASC").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve agent runtime asset: %w", err)
	}
	return &row.ID, nil
}

func normalizedAgentAssetAliases(agentType string) []string {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "codex", "openai-codex", "openai_codex":
		return []string{"codex", "openaicodex"}
	case "claude", "claude-code", "claude_code":
		return []string{"claude", "claudecode"}
	case "openclaw", "open-claw", "open_claw":
		return []string{"openclaw"}
	case "hermes":
		return []string{"hermes"}
	case "opencode", "open-code", "open_code":
		return []string{"opencode"}
	case "gemini", "gemini-cli", "gemini_cli":
		return []string{"gemini", "geminicli"}
	default:
		normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(agentType)))
		if normalized == "" {
			return nil
		}
		return []string{normalized}
	}
}

func shouldApplyActionStatus(current, incoming *model.AgentGuardAction) bool {
	if current == nil || incoming == nil || isTerminalActionStatus(current.Status) {
		return false
	}
	currentRank, incomingRank := actionStatusRank(current.Status), actionStatusRank(incoming.Status)
	if currentRank == 0 || incomingRank == 0 || incomingRank < currentRank {
		return false
	}
	if incomingRank == currentRank && !incoming.UpdatedAt.After(current.UpdatedAt) {
		return false
	}
	return true
}

func actionStatusRank(status string) int {
	switch status {
	case "pending":
		return 1
	case "dispatching":
		return 2
	case "running":
		return 3
	case "success", "failed", "expired", "cancelled":
		return 4
	default:
		return 0
	}
}

func isTerminalActionStatus(status string) bool {
	return status == "success" || status == "failed" || status == "expired" || status == "cancelled"
}

func insertTimeoutAutoResume(tx *gorm.DB, action *model.AgentGuardAction) (bool, error) {
	if tx == nil || !isInsertableTimeoutAutoResume(action) {
		return false, ErrAgentGuardActionStateMissing
	}
	var dependencyCount int64
	if err := tx.Model(&model.AgentExecutionUnit{}).
		Where(
			"id = ? AND host_id = ? AND instance_id = ?",
			*action.ExecutionUnitID, action.HostID, *action.InstanceID,
		).
		Count(&dependencyCount).Error; err != nil {
		return false, err
	}
	if dependencyCount != 1 {
		return false, ErrAgentGuardStateDependencyMissing
	}
	insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(action)
	if insert.Error != nil {
		return false, insert.Error
	}
	if insert.RowsAffected == 0 {
		var existingCount int64
		if err := tx.Model(&model.AgentGuardAction{}).
			Where(
				"id = ? AND command_id = ? AND host_id = ? AND instance_id = ? AND execution_unit_id = ? AND action = 'auto_resume' AND source = 'timeout' AND status = 'success'",
				action.ID, action.CommandID, action.HostID, *action.InstanceID, *action.ExecutionUnitID,
			).
			Count(&existingCount).Error; err != nil {
			return false, err
		}
		if existingCount != 1 {
			return false, ErrAgentGuardActionStateMissing
		}
		return false, nil
	}
	unitUpdate := tx.Model(&model.AgentExecutionUnit{}).
		Where("id = ? AND host_id = ? AND instance_id = ?", *action.ExecutionUnitID, action.HostID, *action.InstanceID).
		Updates(map[string]interface{}{
			"status": gorm.Expr(
				"CASE WHEN status = 'stopped' OR updated_at > ? THEN status ELSE 'observed' END",
				action.UpdatedAt,
			),
			"last_seen_at": gorm.Expr("GREATEST(last_seen_at, ?)", action.UpdatedAt),
			"updated_at":   gorm.Expr("GREATEST(updated_at, ?)", action.UpdatedAt),
		})
	if unitUpdate.Error != nil {
		return false, unitUpdate.Error
	}
	if unitUpdate.RowsAffected != 1 {
		return false, ErrAgentGuardStateDependencyMissing
	}
	return true, nil
}

func isInsertableTimeoutAutoResume(action *model.AgentGuardAction) bool {
	if action == nil || action.ID == uuid.Nil || action.HostID == uuid.Nil ||
		action.InstanceID == nil || *action.InstanceID == uuid.Nil ||
		action.ExecutionUnitID == nil || *action.ExecutionUnitID == uuid.Nil ||
		action.Action != "auto_resume" || action.Source != "timeout" ||
		action.Reason != "automatic freeze timeout elapsed" || action.Status != "success" ||
		action.RequestedAt.IsZero() || action.UpdatedAt.IsZero() {
		return false
	}
	commandActionID, err := uuid.Parse(strings.TrimPrefix(action.CommandID, "AG-GUARD-"))
	if err != nil || !strings.HasPrefix(action.CommandID, "AG-GUARD-") || commandActionID != action.ID {
		return false
	}
	var result map[string]any
	if json.Unmarshal(action.Result, &result) != nil ||
		(!boolJSONValue(result["executed"]) && !boolJSONValue(result["state_changed"])) {
		return false
	}
	return true
}

func boolJSONValue(value any) bool {
	result, _ := value.(bool)
	return result
}

// mergeDeliveryStatus mirrors the SQL transition used above. A reconnect can
// legitimately turn a prior dispatch failure into applied, while stale or
// delayed received reports must never regress an applied bundle.
func mergeDeliveryStatus(current, incoming string) string {
	switch {
	case current == "unsupported_agent_version":
		return current
	case incoming == "applied":
		return incoming
	case current == "applied":
		return current
	case current == "failed" && incoming == "received":
		return current
	default:
		return incoming
	}
}
