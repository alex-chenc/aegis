package repository

import (
	"context"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantMemoryRepository 智能体记忆仓库接口
type AssistantMemoryRepository interface {
	Create(ctx context.Context, memory *model.AssistantMemory) error
	ListBySession(ctx context.Context, sessionID string, memoryType string) ([]model.AssistantMemory, error)
	DeleteBySession(ctx context.Context, sessionID string) error
}

type assistantMemoryRepo struct {
	db *gorm.DB
}

// NewAssistantMemoryRepository 创建智能体记忆仓库
func NewAssistantMemoryRepository(db *gorm.DB) AssistantMemoryRepository {
	return &assistantMemoryRepo{db: db}
}

func (r *assistantMemoryRepo) Create(ctx context.Context, memory *model.AssistantMemory) error {
	if memory.ID == uuid.Nil {
		memory.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(memory).Error
}

func (r *assistantMemoryRepo) ListBySession(ctx context.Context, sessionID string, memoryType string) ([]model.AssistantMemory, error) {
	var memories []model.AssistantMemory

	tx := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID)

	if memoryType != "" {
		tx = tx.Where("memory_type = ?", memoryType)
	}

	err := tx.Order("created_at DESC").Find(&memories).Error
	return memories, err
}

func (r *assistantMemoryRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.AssistantMemory{}).Error
}
