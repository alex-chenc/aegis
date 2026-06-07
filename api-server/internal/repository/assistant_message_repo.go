package repository

import (
	"context"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantMessageRepository 智能体消息仓库接口
type AssistantMessageRepository interface {
	Create(ctx context.Context, message *model.AssistantMessage) error
	FindByMessageID(ctx context.Context, messageID string) (*model.AssistantMessage, error)
	ListBySession(ctx context.Context, sessionID string, limit int) ([]model.AssistantMessage, error)
	CountBySession(ctx context.Context, sessionID string) (int64, error)
	DeleteBySession(ctx context.Context, sessionID string) error
}

type assistantMessageRepo struct {
	db *gorm.DB
}

// NewAssistantMessageRepository 创建智能体消息仓库
func NewAssistantMessageRepository(db *gorm.DB) AssistantMessageRepository {
	return &assistantMessageRepo{db: db}
}

func (r *assistantMessageRepo) Create(ctx context.Context, message *model.AssistantMessage) error {
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}
	if message.MessageID == "" {
		message.MessageID = "msg_" + uuid.New().String()[:8]
	}
	return r.db.WithContext(ctx).Create(message).Error
}

func (r *assistantMessageRepo) FindByMessageID(ctx context.Context, messageID string) (*model.AssistantMessage, error) {
	var message model.AssistantMessage
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		First(&message).Error
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *assistantMessageRepo) ListBySession(ctx context.Context, sessionID string, limit int) ([]model.AssistantMessage, error) {
	var messages []model.AssistantMessage

	tx := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC")

	if limit > 0 {
		tx = tx.Limit(limit)
	}

	err := tx.Find(&messages).Error
	return messages, err
}

func (r *assistantMessageRepo) CountBySession(ctx context.Context, sessionID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.AssistantMessage{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error
	return count, err
}

func (r *assistantMessageRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.AssistantMessage{}).Error
}
