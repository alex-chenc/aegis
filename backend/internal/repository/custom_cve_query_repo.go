package repository

import (
	"errors"
	"time"

	"aegis-system/internal/model"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CustomCVEQueryRepository struct {
	db *gorm.DB
}

func NewCustomCVEQueryRepository(db *gorm.DB) *CustomCVEQueryRepository {
	return &CustomCVEQueryRepository{db: db}
}

func (r *CustomCVEQueryRepository) Create(query *model.CustomCVEQuery) error {
	result := r.db.Create(query)
	if result.Error != nil {
		logger.Error("failed to create custom cve query",
			zap.Error(result.Error),
			zap.String("cve_id", query.CveID),
		)
		return result.Error
	}

	logger.Info("custom cve query created",
		zap.String("id", query.ID.String()),
		zap.String("cve_id", query.CveID),
	)
	return nil
}

func (r *CustomCVEQueryRepository) FindByID(id uuid.UUID) (*model.CustomCVEQuery, error) {
	var query model.CustomCVEQuery
	result := r.db.First(&query, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		logger.Error("failed to find custom cve query by id",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return nil, result.Error
	}
	return &query, nil
}

func (r *CustomCVEQueryRepository) FindQuerying() (*model.CustomCVEQuery, error) {
	var query model.CustomCVEQuery
	result := r.db.Where("status = ?", model.QueryStatusQuerying).
		Order("started_at ASC").
		First(&query)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		logger.Error("failed to find querying custom cve query", zap.Error(result.Error))
		return nil, result.Error
	}

	return &query, nil
}

func (r *CustomCVEQueryRepository) MarkSuccess(id uuid.UUID, vulnerabilityID uuid.UUID) error {
	now := time.Now()
	result := r.db.Model(&model.CustomCVEQuery{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":                  model.QueryStatusSuccess,
			"result_vulnerability_id": vulnerabilityID,
			"error_message":           nil,
			"error_detail":            nil,
			"completed_at":            now,
		})

	if result.Error != nil {
		logger.Error("failed to mark custom cve query success",
			zap.Error(result.Error),
			zap.String("id", id.String()),
			zap.String("vulnerability_id", vulnerabilityID.String()),
		)
		return result.Error
	}
	return nil
}

func (r *CustomCVEQueryRepository) MarkFailed(id uuid.UUID, errMsg string, errDetail string) error {
	now := time.Now()
	updates := map[string]any{
		"status":       model.QueryStatusFailed,
		"completed_at": now,
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if errDetail != "" {
		updates["error_detail"] = errDetail
	}

	result := r.db.Model(&model.CustomCVEQuery{}).
		Where("id = ?", id).
		Updates(updates)
	if result.Error != nil {
		logger.Error("failed to mark custom cve query failed",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return result.Error
	}
	return nil
}

func (r *CustomCVEQueryRepository) MarkExpiredQueryingAsFailed() error {
	now := time.Now()
	expiredAt := now.Add(-5 * time.Minute)

	result := r.db.Model(&model.CustomCVEQuery{}).
		Where("status = ? AND completed_at IS NULL AND started_at < ?", model.QueryStatusQuerying, expiredAt).
		Updates(map[string]any{
			"status":        model.QueryStatusFailed,
			"error_message": "query timeout",
			"error_detail":  "query exceeded 5 minutes without completion",
			"completed_at":  now,
		})

	if result.Error != nil {
		logger.Error("failed to mark expired querying custom cve as failed", zap.Error(result.Error))
		return result.Error
	}

	if result.RowsAffected > 0 {
		logger.Info("expired custom cve queries marked as failed", zap.Int64("count", result.RowsAffected))
	}

	return nil
}
