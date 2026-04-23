package repository

import (
	"time"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AIMessageRepository struct {
	db *gorm.DB
}

func NewAIMessageRepository(db *gorm.DB) *AIMessageRepository {
	return &AIMessageRepository{db: db}
}

// Create creates a new message
func (r *AIMessageRepository) Create(msg *model.AIMessage) error {
	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	return r.db.Create(msg).Error
}

// FindBySessionID finds all messages for a session
func (r *AIMessageRepository) FindBySessionID(sessionID string) ([]*model.AIMessage, error) {
	var messages []*model.AIMessage
	err := r.db.Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&messages).Error
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// UpdateToolResult updates tool execution result
func (r *AIMessageRepository) UpdateToolResult(messageID string, result model.JSONB) error {
	return r.db.Model(&model.AIMessage{}).Where("message_id = ?", messageID).
		Updates(map[string]interface{}{
			"tool_results": result,
		}).Error
}

// DeleteBySessionID deletes all messages for a session
func (r *AIMessageRepository) DeleteBySessionID(sessionID string) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&model.AIMessage{}).Error
}