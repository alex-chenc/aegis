package repository

import (
	"context"
	"fmt"

	"api-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentGuardToolFindingRepository persists findings produced by the
// api-server tool-command rule owner. It intentionally does not require an
// eBPF/process row to exist: the signed Hook tool event is the finding's
// primary evidence and process fields are optional enrichment.
type AgentGuardToolFindingRepository struct {
	db *gorm.DB
}

func NewAgentGuardToolFindingRepository(db *gorm.DB) *AgentGuardToolFindingRepository {
	return &AgentGuardToolFindingRepository{db: db}
}

func (r *AgentGuardToolFindingRepository) UpsertToolFinding(
	ctx context.Context,
	finding *model.AgentSecurityFinding,
) error {
	if finding == nil {
		return fmt.Errorf("agent guard tool finding is nil")
	}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "finding_key"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"instance_id", "session_id", "execution_unit_id", "title", "severity",
				"verdict", "confidence", "status", "decision_sources", "rule_hits",
				"evidence_event_ids", "evidence_graph", "attack_stages", "summary",
				"recommended_action", "first_observed_at", "last_observed_at", "updated_at",
			}),
		}).Create(finding).Error; err != nil {
		return fmt.Errorf("upsert agent guard tool finding: %w", err)
	}
	return nil
}
