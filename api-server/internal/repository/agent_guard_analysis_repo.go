package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentGuardAnalysisRepository owns only analysis run state. Deterministic
// findings are deliberately not rewritten by an AI result.
type AgentGuardAnalysisRepository struct {
	db *gorm.DB
}

func NewAgentGuardAnalysisRepository(db *gorm.DB) *AgentGuardAnalysisRepository {
	return &AgentGuardAnalysisRepository{db: db}
}

func (r *AgentGuardAnalysisRepository) LoadEvidence(
	ctx context.Context,
	findingID uuid.UUID,
	candidateLimit int,
) (*model.AgentSecurityFinding, []model.AgentBehaviorEvent, error) {
	var finding model.AgentSecurityFinding
	if err := r.db.WithContext(ctx).First(&finding, "id = ?", findingID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrAgentGuardFindingNotFound
		}
		return nil, nil, fmt.Errorf("load agent guard finding for analysis: %w", err)
	}
	if candidateLimit <= 0 {
		candidateLimit = 256
	}

	base := r.db.WithContext(ctx).
		Where("host_id = ?", finding.HostID).
		Where(
			"occurred_at BETWEEN ? AND ?",
			finding.FirstObservedAt.Add(-5*time.Minute),
			finding.LastObservedAt.Add(5*time.Minute),
		)
	switch {
	case finding.SessionID != nil:
		base = base.Where("session_id = ?", *finding.SessionID)
	case finding.ExecutionUnitID != nil:
		base = base.Where("execution_unit_id = ?", *finding.ExecutionUnitID)
	case finding.InstanceID != nil:
		base = base.Where("instance_id = ?", *finding.InstanceID)
	}
	var events []model.AgentBehaviorEvent
	if err := base.
		Order("occurred_at ASC, agent_sequence ASC, id ASC").
		Limit(candidateLimit).
		Find(&events).Error; err != nil {
		return nil, nil, fmt.Errorf("load scoped agent guard analysis evidence: %w", err)
	}

	// Direct finding evidence may sit just outside a correlated time slice. Load
	// it separately, still host-bound, and let the bounded builder deduplicate.
	directIDs := decodeAgentGuardStringArray(finding.EvidenceEventIDs)
	if len(directIDs) > 0 {
		var direct []model.AgentBehaviorEvent
		if err := r.db.WithContext(ctx).
			Where("host_id = ? AND raw_event_id IN ?", finding.HostID, directIDs).
			Find(&direct).Error; err != nil {
			return nil, nil, fmt.Errorf("load direct agent guard analysis evidence: %w", err)
		}
		events = append(events, direct...)
	}
	return &finding, events, nil
}

func (r *AgentGuardAnalysisRepository) CreatePending(
	ctx context.Context,
	run *model.AgentSecurityAnalysisRun,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var finding model.AgentSecurityFinding
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&finding, "id = ?", run.FindingID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAgentGuardFindingNotFound
			}
			return fmt.Errorf("lock agent guard finding for analysis: %w", err)
		}
		var maxAttempt int
		if err := tx.Model(&model.AgentSecurityAnalysisRun{}).
			Where("finding_id = ?", run.FindingID).
			Select("COALESCE(MAX(attempt), 0)").
			Scan(&maxAttempt).Error; err != nil {
			return fmt.Errorf("allocate agent guard analysis attempt: %w", err)
		}
		run.Attempt = maxAttempt + 1
		if err := tx.Create(run).Error; err != nil {
			return fmt.Errorf("create agent guard analysis run: %w", err)
		}
		return nil
	})
}

func (r *AgentGuardAnalysisRepository) MarkRunning(
	ctx context.Context,
	id uuid.UUID,
	startedAt time.Time,
) error {
	result := r.db.WithContext(ctx).Model(&model.AgentSecurityAnalysisRun{}).
		Where("id = ? AND status = ?", id, model.AgentGuardAnalysisStatusPending).
		Updates(map[string]any{
			"status":     model.AgentGuardAnalysisStatusRunning,
			"started_at": startedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("start agent guard analysis run: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrAgentGuardAnalysisNotFound
	}
	return nil
}

func (r *AgentGuardAnalysisRepository) MarkFailed(
	ctx context.Context,
	id uuid.UUID,
	status string,
	provider string,
	llmModel string,
	errorCode string,
	errorMessage string,
	completedAt time.Time,
) error {
	result := r.db.WithContext(ctx).Model(&model.AgentSecurityAnalysisRun{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":        status,
			"provider":      provider,
			"model":         llmModel,
			"error_code":    errorCode,
			"error_message": truncateAgentGuardAnalysisError(errorMessage),
			"completed_at":  completedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("fail agent guard analysis run: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrAgentGuardAnalysisNotFound
	}
	return nil
}

func (r *AgentGuardAnalysisRepository) MarkSucceeded(
	ctx context.Context,
	run *model.AgentSecurityAnalysisRun,
	completedAt time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentSecurityAnalysisRun{}).
			Where("id = ?", run.ID).
			Updates(map[string]any{
				"status":             run.Status,
				"provider":           run.Provider,
				"model":              run.Model,
				"output":             run.Output,
				"verdict":            run.Verdict,
				"attack_probability": run.AttackProbability,
				"confidence":         run.Confidence,
				"error_code":         "",
				"error_message":      "",
				"completed_at":       completedAt,
			})
		if result.Error != nil {
			return fmt.Errorf("complete agent guard analysis run: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrAgentGuardAnalysisNotFound
		}
		// The link is presentation state only. Rule-derived verdict, severity,
		// confidence, summary and recommendation remain authoritative.
		if err := tx.Model(&model.AgentSecurityFinding{}).
			Where("id = ?", run.FindingID).
			Update("latest_analysis_id", run.ID).Error; err != nil {
			return fmt.Errorf("link latest agent guard analysis: %w", err)
		}
		return nil
	})
}

func truncateAgentGuardAnalysisError(value string) string {
	const max = 512
	if len(value) <= max {
		return value
	}
	return value[:max]
}
