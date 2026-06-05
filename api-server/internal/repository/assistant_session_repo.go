package repository

import (
	"context"
	"time"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantSessionRepository 智能体会话仓库接口
type AssistantSessionRepository interface {
	Create(ctx context.Context, session *model.AssistantSession) error
	FindBySessionID(ctx context.Context, sessionID string) (*model.AssistantSession, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.AssistantSession, error)
	List(ctx context.Context, query SessionQuery) ([]model.AssistantSession, int64, error)
	Update(ctx context.Context, session *model.AssistantSession) error
	UpdateStatus(ctx context.Context, sessionID string, status string) error
	IncrementMessageCount(ctx context.Context, sessionID string) error
	IncrementToolCallCount(ctx context.Context, sessionID string) error
	IncrementApprovalCount(ctx context.Context, sessionID string) error
	Delete(ctx context.Context, sessionID string) error
}

// SessionQuery 会话查询参数
type SessionQuery struct {
	Status    string `json:"status"`
	CreatedBy string `json:"created_by"`
	Keyword   string `json:"keyword"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
}

type assistantSessionRepo struct {
	db *gorm.DB
}

// NewAssistantSessionRepository 创建智能体会话仓库
func NewAssistantSessionRepository(db *gorm.DB) AssistantSessionRepository {
	return &assistantSessionRepo{db: db}
}

func (r *assistantSessionRepo) Create(ctx context.Context, session *model.AssistantSession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	if session.SessionID == "" {
		session.SessionID = "asst_" + uuid.New().String()[:8]
	}
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *assistantSessionRepo) FindBySessionID(ctx context.Context, sessionID string) (*model.AssistantSession, error) {
	var session model.AssistantSession
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *assistantSessionRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.AssistantSession, error) {
	var session model.AssistantSession
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *assistantSessionRepo) List(ctx context.Context, query SessionQuery) ([]model.AssistantSession, int64, error) {
	var sessions []model.AssistantSession
	var total int64

	tx := r.db.WithContext(ctx).Model(&model.AssistantSession{})

	if query.Status != "" {
		tx = tx.Where("status = ?", query.Status)
	}
	if query.CreatedBy != "" {
		tx = tx.Where("created_by = ?", query.CreatedBy)
	}
	if query.Keyword != "" {
		tx = tx.Where("title ILIKE ?", "%"+query.Keyword+"%")
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
		pageSize = 20
	}

	err := tx.
		Order("updated_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&sessions).Error

	return sessions, total, err
}

func (r *assistantSessionRepo) Update(ctx context.Context, session *model.AssistantSession) error {
	session.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *assistantSessionRepo) UpdateStatus(ctx context.Context, sessionID string, status string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

func (r *assistantSessionRepo) IncrementMessageCount(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"message_count": gorm.Expr("message_count + 1"),
			"updated_at":    time.Now(),
		}).Error
}

func (r *assistantSessionRepo) IncrementToolCallCount(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"tool_call_count": gorm.Expr("tool_call_count + 1"),
			"updated_at":      time.Now(),
		}).Error
}

func (r *assistantSessionRepo) IncrementApprovalCount(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"approval_count": gorm.Expr("approval_count + 1"),
			"updated_at":     time.Now(),
		}).Error
}

func (r *assistantSessionRepo) Delete(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.AssistantSession{}).Error
}
