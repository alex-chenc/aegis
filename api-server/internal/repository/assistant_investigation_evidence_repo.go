package repository

import (
	"context"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantInvestigationEvidenceRepository 攻击研判证据仓库接口
type AssistantInvestigationEvidenceRepository interface {
	Save(ctx context.Context, evidence *model.AssistantInvestigationEvidence) error
	BatchSave(ctx context.Context, evidences []model.AssistantInvestigationEvidence) error
	ListByInvestigation(ctx context.Context, investigationID string, query EvidenceQuery) ([]model.AssistantInvestigationEvidence, int64, error)
	DeleteByInvestigation(ctx context.Context, investigationID string) error
}

// EvidenceQuery 证据查询参数
type EvidenceQuery struct {
	SourceType string `json:"source_type"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

type assistantInvestigationEvidenceRepo struct {
	db *gorm.DB
}

// NewAssistantInvestigationEvidenceRepository 创建攻击研判证据仓库
func NewAssistantInvestigationEvidenceRepository(db *gorm.DB) AssistantInvestigationEvidenceRepository {
	return &assistantInvestigationEvidenceRepo{db: db}
}

func (r *assistantInvestigationEvidenceRepo) Save(ctx context.Context, evidence *model.AssistantInvestigationEvidence) error {
	if evidence.ID == uuid.Nil {
		evidence.ID = uuid.New()
	}

	// Upsert based on investigation_id + evidence_id
	existing := &model.AssistantInvestigationEvidence{}
	err := r.db.WithContext(ctx).
		Where("investigation_id = ? AND evidence_id = ?", evidence.InvestigationID, evidence.EvidenceID).
		First(existing).Error

	if err == nil {
		evidence.ID = existing.ID
		return r.db.WithContext(ctx).Save(evidence).Error
	}

	return r.db.WithContext(ctx).Create(evidence).Error
}

func (r *assistantInvestigationEvidenceRepo) BatchSave(ctx context.Context, evidences []model.AssistantInvestigationEvidence) error {
	if len(evidences) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, evidence := range evidences {
			if evidence.ID == uuid.Nil {
				evidence.ID = uuid.New()
			}

			// Upsert
			existing := &model.AssistantInvestigationEvidence{}
			err := tx.Where("investigation_id = ? AND evidence_id = ?", evidence.InvestigationID, evidence.EvidenceID).
				First(existing).Error

			if err == nil {
				evidence.ID = existing.ID
				if err := tx.Save(&evidence).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Create(&evidence).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *assistantInvestigationEvidenceRepo) ListByInvestigation(ctx context.Context, investigationID string, query EvidenceQuery) ([]model.AssistantInvestigationEvidence, int64, error) {
	var evidences []model.AssistantInvestigationEvidence
	var total int64

	tx := r.db.WithContext(ctx).
		Model(&model.AssistantInvestigationEvidence{}).
		Where("investigation_id = ?", investigationID)

	if query.SourceType != "" {
		tx = tx.Where("source_type = ?", query.SourceType)
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	err := tx.
		Order("event_time DESC NULLS LAST, created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&evidences).Error

	return evidences, total, err
}

func (r *assistantInvestigationEvidenceRepo) DeleteByInvestigation(ctx context.Context, investigationID string) error {
	return r.db.WithContext(ctx).
		Where("investigation_id = ?", investigationID).
		Delete(&model.AssistantInvestigationEvidence{}).Error
}
