package repository

import (
	"context"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantContextRefRepository 上下文引用仓库接口
type AssistantContextRefRepository interface {
	Create(ctx context.Context, ref *model.AssistantContextRef) error
	UpsertMany(ctx context.Context, refs []model.AssistantContextRef) error
	ListBySession(ctx context.Context, sessionID string) ([]model.AssistantContextRef, error)
	DeleteBySession(ctx context.Context, sessionID string) error
}

type assistantContextRefRepo struct {
	db *gorm.DB
}

// NewAssistantContextRefRepository 创建上下文引用仓库
func NewAssistantContextRefRepository(db *gorm.DB) AssistantContextRefRepository {
	return &assistantContextRefRepo{db: db}
}

func (r *assistantContextRefRepo) Create(ctx context.Context, ref *model.AssistantContextRef) error {
	if ref.ID == uuid.Nil {
		ref.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(ref).Error
}

func (r *assistantContextRefRepo) UpsertMany(ctx context.Context, refs []model.AssistantContextRef) error {
	if len(refs) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, ref := range refs {
			if ref.ID == uuid.Nil {
				ref.ID = uuid.New()
			}
			err := tx.Where("session_id = ? AND object_type = ? AND object_id = ?",
				ref.SessionID, ref.ObjectType, ref.ObjectID).
				Assign(ref).
				FirstOrCreate(&ref).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *assistantContextRefRepo) ListBySession(ctx context.Context, sessionID string) ([]model.AssistantContextRef, error) {
	var refs []model.AssistantContextRef
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&refs).Error
	return refs, err
}

func (r *assistantContextRefRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.AssistantContextRef{}).Error
}
