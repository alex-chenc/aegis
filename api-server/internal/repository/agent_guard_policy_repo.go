package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAgentGuardPolicyNotFound        = errors.New("agent guard policy not found")
	ErrAgentGuardPolicyNotDraft        = errors.New("agent guard policy is not a draft")
	ErrAgentGuardTargetHostNotFound    = errors.New("agent guard target host not found")
	ErrAgentGuardHostGroupsUnsupported = errors.New("agent guard host groups are not available in this deployment")
)

type AgentGuardPolicyRepository struct {
	db *gorm.DB
}

func NewAgentGuardPolicyRepository(db *gorm.DB) *AgentGuardPolicyRepository {
	return &AgentGuardPolicyRepository{db: db}
}

func (r *AgentGuardPolicyRepository) CreateDraft(
	ctx context.Context,
	policy *model.AgentGuardPolicy,
) error {
	if policy == nil {
		return errors.New("agent guard policy is required")
	}
	if policy.Status == "" {
		policy.Status = "draft"
	}
	if policy.Status != "draft" {
		return ErrAgentGuardPolicyNotDraft
	}
	if policy.ID == uuid.Nil {
		policy.ID = uuid.New()
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if policy.Version < 1 {
			var latest int64
			if err := tx.Model(&model.AgentGuardPolicy{}).
				Where("policy_key = ?", policy.PolicyKey).
				Select("COALESCE(MAX(version), 0)").
				Scan(&latest).Error; err != nil {
				return err
			}
			policy.Version = latest + 1
		}
		return tx.Create(policy).Error
	}); err != nil {
		return fmt.Errorf("create agent guard policy draft: %w", err)
	}
	return nil
}

// PublishDraftWithDeliveries freezes a validated draft and creates the host
// delivery facts in one transaction. Dispatch happens after commit; a pending
// or failed delivery must never be reported as applied.
func (r *AgentGuardPolicyRepository) PublishDraftWithDeliveries(
	ctx context.Context,
	id uuid.UUID,
	publishedBy string,
	deliveries []model.AgentGuardPolicyDelivery,
) (*model.AgentGuardPolicy, []model.AgentGuardPolicyDelivery, error) {
	var published model.AgentGuardPolicy
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var draft model.AgentGuardPolicy
		if err := query.First(&draft, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAgentGuardPolicyNotFound
			}
			return err
		}
		if draft.Status != "draft" {
			return ErrAgentGuardPolicyNotDraft
		}

		now := time.Now().UTC()
		if err := tx.Model(&model.AgentGuardPolicy{}).
			Where("policy_key = ? AND status = ? AND id <> ?", draft.PolicyKey, "published", draft.ID).
			Updates(map[string]any{"status": "superseded", "updated_at": now}).Error; err != nil {
			return err
		}
		result := tx.Model(&model.AgentGuardPolicy{}).
			Where("id = ? AND status = ?", draft.ID, "draft").
			Updates(map[string]any{
				"status":       "published",
				"published_by": strings.TrimSpace(publishedBy),
				"published_at": now,
				"updated_at":   now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAgentGuardPolicyNotDraft
		}

		for index := range deliveries {
			if deliveries[index].ID == uuid.Nil {
				deliveries[index].ID = uuid.New()
			}
			if deliveries[index].Status == "" {
				deliveries[index].Status = "pending"
			}
			if len(deliveries[index].CapabilitySnapshot) == 0 {
				deliveries[index].CapabilitySnapshot = datatypes.JSON([]byte(`{}`))
			}
			if deliveries[index].GeneratedAt.IsZero() {
				deliveries[index].GeneratedAt = now
			}
		}
		if len(deliveries) > 0 {
			if err := tx.Create(&deliveries).Error; err != nil {
				return err
			}
		}
		return tx.First(&published, "id = ?", draft.ID).Error
	})
	if err != nil {
		if errors.Is(err, ErrAgentGuardPolicyNotFound) || errors.Is(err, ErrAgentGuardPolicyNotDraft) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("publish agent guard policy: %w", err)
	}
	return &published, deliveries, nil
}

func (r *AgentGuardPolicyRepository) ResolveTargetHostIDs(
	ctx context.Context,
	targets model.AgentGuardPolicyTargets,
) ([]uuid.UUID, error) {
	if len(targets.HostGroupIDs) > 0 {
		return nil, ErrAgentGuardHostGroupsUnsupported
	}

	query := r.db.WithContext(ctx).Model(&model.Host{}).Order("id ASC")
	requested := make([]uuid.UUID, 0, len(targets.HostIDs))
	for _, rawID := range targets.HostIDs {
		id, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			return nil, fmt.Errorf("parse target host id: %w", err)
		}
		requested = append(requested, id)
	}
	if len(requested) > 0 {
		query = query.Where("id IN ?", requested)
	}

	var hostIDs []uuid.UUID
	if err := query.Pluck("id", &hostIDs).Error; err != nil {
		return nil, fmt.Errorf("resolve agent guard target hosts: %w", err)
	}
	if len(requested) > 0 && len(hostIDs) != len(requested) {
		return nil, ErrAgentGuardTargetHostNotFound
	}
	return hostIDs, nil
}

func (r *AgentGuardPolicyRepository) MaxBundleVersion(
	ctx context.Context,
	hostIDs []uuid.UUID,
) (int64, error) {
	if len(hostIDs) == 0 {
		return 0, nil
	}
	var maxVersion int64
	if err := r.db.WithContext(ctx).
		Model(&model.AgentGuardPolicyDelivery{}).
		Where("host_id IN ?", hostIDs).
		Select("COALESCE(MAX(bundle_version), 0)").
		Scan(&maxVersion).Error; err != nil {
		return 0, fmt.Errorf("get latest agent guard bundle version: %w", err)
	}
	return maxVersion, nil
}

func (r *AgentGuardPolicyRepository) UpdateDeliveryDispatchStatus(
	ctx context.Context,
	id uuid.UUID,
	status string,
	errorCode string,
	errorMessage string,
) error {
	if status != "dispatching" && status != "failed" {
		return fmt.Errorf("unsupported delivery dispatch status: %s", status)
	}
	now := time.Now().UTC()
	values := map[string]any{
		"status":        status,
		"error_code":    errorCode,
		"error_message": errorMessage,
		"updated_at":    now,
	}
	if status == "dispatching" {
		values["dispatched_at"] = now
	}
	result := r.db.WithContext(ctx).
		Model(&model.AgentGuardPolicyDelivery{}).
		Where("id = ? AND status IN ?", id, []string{"pending", "dispatching"}).
		Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update agent guard delivery dispatch status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent guard delivery is no longer dispatchable")
	}
	return nil
}

func (r *AgentGuardPolicyRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentGuardPolicy, error) {
	var policy model.AgentGuardPolicy
	err := r.db.WithContext(ctx).First(&policy, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentGuardPolicyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent guard policy: %w", err)
	}
	return &policy, nil
}

func (r *AgentGuardPolicyRepository) List(
	ctx context.Context,
	query model.AgentGuardPolicyQuery,
) ([]model.AgentGuardPolicy, int64, error) {
	var (
		policies []model.AgentGuardPolicy
		total    int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentGuardPolicy{})
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		db = db.Where(
			"LOWER(policy_key) LIKE ? OR LOWER(name) LIKE ? OR LOWER(description) LIKE ?",
			pattern,
			pattern,
			pattern,
		)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent guard policies: %w", err)
	}
	page, pageSize := normalizeAgentGuardPage(query.Page, query.PageSize)
	if err := db.Order("updated_at DESC, policy_key ASC, version DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&policies).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent guard policies: %w", err)
	}
	return policies, total, nil
}

func (r *AgentGuardPolicyRepository) UpdateDraft(
	ctx context.Context,
	id uuid.UUID,
	update model.AgentGuardPolicyDraftUpdate,
) (*model.AgentGuardPolicy, error) {
	values := map[string]any{
		"name":                   update.Name,
		"description":            update.Description,
		"priority":               update.Priority,
		"targets":                update.Targets,
		"collection_policy":      update.CollectionPolicy,
		"builtin_rule_overrides": update.BuiltinRuleOverrides,
		"atomic_rules":           update.AtomicRules,
		"correlation_rules":      update.CorrelationRules,
		"analysis_policy":        update.AnalysisPolicy,
		"escape_rules":           update.EscapeRules,
		"freeze_timeout_seconds": update.FreezeTimeoutSeconds,
		"compiled_preview":       update.CompiledPreview,
		"digest":                 update.Digest,
	}
	var policy model.AgentGuardPolicy
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentGuardPolicy{}).
			Where("id = ? AND status = ?", id, "draft").
			Updates(values)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var status string
			err := tx.Model(&model.AgentGuardPolicy{}).
				Select("status").
				Where("id = ?", id).
				Scan(&status).Error
			if err != nil {
				return err
			}
			if status == "" {
				return ErrAgentGuardPolicyNotFound
			}
			return ErrAgentGuardPolicyNotDraft
		}
		return tx.First(&policy, "id = ?", id).Error
	})
	if err != nil {
		if errors.Is(err, ErrAgentGuardPolicyNotFound) || errors.Is(err, ErrAgentGuardPolicyNotDraft) {
			return nil, err
		}
		return nil, fmt.Errorf("update agent guard policy draft: %w", err)
	}
	return &policy, nil
}

func (r *AgentGuardPolicyRepository) ListDeliveries(
	ctx context.Context,
	policyKey string,
	policyVersion int64,
	query model.AgentGuardDeliveryQuery,
) ([]model.AgentGuardPolicyDelivery, int64, error) {
	var (
		deliveries []model.AgentGuardPolicyDelivery
		total      int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentGuardPolicyDelivery{})
	if query.HostID != "" {
		db = db.Where("host_id = ?", query.HostID)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if policyKey != "" {
		if r.db.Dialector.Name() == "postgres" {
			manifest := fmt.Sprintf(`[{"policy_key":%q,"version":%d}]`, policyKey, policyVersion)
			db = db.Where("policy_versions @> ?::jsonb", manifest)
		} else {
			db = db.Where(
				`EXISTS (
					SELECT 1 FROM json_each(agent_guard_policy_deliveries.policy_versions)
					WHERE json_extract(json_each.value, '$.policy_key') = ?
					  AND (? < 1 OR json_extract(json_each.value, '$.version') = ?)
				)`,
				policyKey,
				policyVersion,
				policyVersion,
			)
		}
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent guard policy deliveries: %w", err)
	}
	page, pageSize := normalizeAgentGuardPage(query.Page, query.PageSize)
	if err := db.Order("generated_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&deliveries).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent guard policy deliveries: %w", err)
	}
	return deliveries, total, nil
}
