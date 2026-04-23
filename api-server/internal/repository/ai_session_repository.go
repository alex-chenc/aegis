package repository

import (
	"time"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AISessionRepository struct {
	db *gorm.DB
}

func NewAISessionRepository(db *gorm.DB) *AISessionRepository {
	return &AISessionRepository{db: db}
}

// Create creates a new session
func (r *AISessionRepository) Create(session *model.AISession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	session.UpdatedAt = time.Now()
	return r.db.Create(session).Error
}

// FindBySessionID finds a session by session_id
func (r *AISessionRepository) FindBySessionID(sessionID string) (*model.AISession, error) {
	var session model.AISession
	err := r.db.Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// FindList finds sessions with pagination
func (r *AISessionRepository) FindList(page, pageSize int, status string) ([]*model.AISession, int64, error) {
	var sessions []*model.AISession
	var total int64

	query := r.db.Model(&model.AISession{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&sessions).Error; err != nil {
		return nil, 0, err
	}

	return sessions, total, nil
}

// UpdateStatus updates session status
func (r *AISessionRepository) UpdateStatus(sessionID, status string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if status == "completed" {
		updates["concluded_at"] = time.Now()
	}
	return r.db.Model(&model.AISession{}).Where("session_id = ?", sessionID).Updates(updates).Error
}

// UpdateConclusion updates session conclusion
func (r *AISessionRepository) UpdateConclusion(sessionID string, conclusion model.JSONB) error {
	return r.db.Model(&model.AISession{}).Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":      "completed",
			"conclusion":  conclusion,
			"concluded_at": time.Now(),
		}).Error
}

// IncrementMessageCount increments message count
func (r *AISessionRepository) IncrementMessageCount(sessionID string) error {
	return r.db.Model(&model.AISession{}).Where("session_id = ?", sessionID).
		UpdateColumn("message_count", gorm.Expr("message_count + 1")).Error
}

// IncrementToolCallCount increments tool call count
func (r *AISessionRepository) IncrementToolCallCount(sessionID string) error {
	return r.db.Model(&model.AISession{}).Where("session_id = ?", sessionID).
		UpdateColumn("tool_call_count", gorm.Expr("tool_call_count + 1")).Error
}

// Delete deletes a session and its messages
func (r *AISessionRepository) Delete(sessionID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&model.AIMessage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", sessionID).Delete(&model.AISession{}).Error; err != nil {
			return err
		}
		return nil
	})
}