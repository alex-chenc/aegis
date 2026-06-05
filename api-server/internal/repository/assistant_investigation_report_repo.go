package repository

import (
	"context"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantInvestigationReportRepository 攻击研判报告仓库接口
type AssistantInvestigationReportRepository interface {
	Save(ctx context.Context, report *model.AssistantInvestigationReport) error
	FindByInvestigationID(ctx context.Context, investigationID string) (*model.AssistantInvestigationReport, error)
	ListBySession(ctx context.Context, sessionID string) ([]model.AssistantInvestigationReport, error)
	ListByHost(ctx context.Context, hostID string, limit int) ([]model.AssistantInvestigationReport, error)
	Update(ctx context.Context, report *model.AssistantInvestigationReport) error
}

type assistantInvestigationReportRepo struct {
	db *gorm.DB
}

// NewAssistantInvestigationReportRepository 创建攻击研判报告仓库
func NewAssistantInvestigationReportRepository(db *gorm.DB) AssistantInvestigationReportRepository {
	return &assistantInvestigationReportRepo{db: db}
}

func (r *assistantInvestigationReportRepo) Save(ctx context.Context, report *model.AssistantInvestigationReport) error {
	if report.ID == uuid.Nil {
		report.ID = uuid.New()
	}
	if report.InvestigationID == "" {
		report.InvestigationID = "inv_" + uuid.New().String()[:8]
	}

	// Try to find existing report
	existing, err := r.FindByInvestigationID(ctx, report.InvestigationID)
	if err == nil && existing != nil {
		report.ID = existing.ID
		return r.db.WithContext(ctx).Save(report).Error
	}

	return r.db.WithContext(ctx).Create(report).Error
}

func (r *assistantInvestigationReportRepo) FindByInvestigationID(ctx context.Context, investigationID string) (*model.AssistantInvestigationReport, error) {
	var report model.AssistantInvestigationReport
	err := r.db.WithContext(ctx).
		Where("investigation_id = ?", investigationID).
		First(&report).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *assistantInvestigationReportRepo) ListBySession(ctx context.Context, sessionID string) ([]model.AssistantInvestigationReport, error) {
	var reports []model.AssistantInvestigationReport
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Find(&reports).Error
	return reports, err
}

func (r *assistantInvestigationReportRepo) ListByHost(ctx context.Context, hostID string, limit int) ([]model.AssistantInvestigationReport, error) {
	var reports []model.AssistantInvestigationReport

	tx := r.db.WithContext(ctx).
		Where("host_id = ?", hostID).
		Order("created_at DESC")

	if limit > 0 {
		tx = tx.Limit(limit)
	}

	err := tx.Find(&reports).Error
	return reports, err
}

func (r *assistantInvestigationReportRepo) Update(ctx context.Context, report *model.AssistantInvestigationReport) error {
	return r.db.WithContext(ctx).Save(report).Error
}
