package service

import (
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NotificationService 通知业务逻辑层
type NotificationService struct {
	repo *repository.NotificationRepository
}

// NewNotificationService 构造函数
func NewNotificationService(repo *repository.NotificationRepository) *NotificationService {
	return &NotificationService{repo: repo}
}

// ListResult 列表查询结果
type ListResult struct {
	List        interface{} `json:"list"`
	Total       int64       `json:"total"`
	UnreadCount int64       `json:"unread_count"`
	Page        int         `json:"page"`
	PageSize    int         `json:"page_size"`
}

// List 获取通知列表
func (s *NotificationService) List(filter repository.ListFilter) (*ListResult, error) {
	result, err := s.repo.List(filter)
	if err != nil {
		logger.Error("Failed to get notification list", zap.Error(err))
		return nil, err
	}

	return &ListResult{
		List:        result.Items,
		Total:       result.Total,
		UnreadCount: result.UnreadCount,
		Page:        filter.Page,
		PageSize:    filter.PageSize,
	}, nil
}

// MarkRead 将通知标为已读
func (s *NotificationService) MarkRead(id uuid.UUID) error {
	err := s.repo.MarkRead(id)
	if err != nil {
		logger.Error("Failed to mark notification as read", zap.String("id", id.String()), zap.Error(err))
		return err
	}
	return nil
}

// MarkAllRead 将所有通知标为已读
func (s *NotificationService) MarkAllRead() (int64, error) {
	count, err := s.repo.MarkAllRead()
	if err != nil {
		logger.Error("Failed to mark all notifications as read", zap.Error(err))
		return 0, err
	}
	logger.Info("Marked all notifications as read", zap.Int64("count", count))
	return count, nil
}