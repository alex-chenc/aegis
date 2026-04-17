package repository

import (
	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NotificationRepository 通知数据访问层
type NotificationRepository struct {
	db *gorm.DB
}

// NewNotificationRepository 构造函数
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// ListFilter 列表查询过滤条件
type ListFilter struct {
	Page     int
	PageSize int
	IsRead   *bool
	Type     string
}

// ListResult 列表查询结果
type ListResult struct {
	Items      []model.Notification
	Total      int64
	UnreadCount int64
}

// List 获取通知列表
func (r *NotificationRepository) List(filter ListFilter) (*ListResult, error) {
	query := r.db.Model(&model.Notification{})

	// 应用过滤条件
	if filter.IsRead != nil {
		query = query.Where("is_read = ?", *filter.IsRead)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}

	// 计算总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 计算未读数
	var unreadCount int64
	if err := r.db.Model(&model.Notification{}).Where("is_read = ?", false).Count(&unreadCount).Error; err != nil {
		return nil, err
	}

	// 分页查询
	offset := (filter.Page - 1) * filter.PageSize
	var items []model.Notification
	if err := query.Order("created_at DESC").Offset(offset).Limit(filter.PageSize).Find(&items).Error; err != nil {
		return nil, err
	}

	return &ListResult{
		Items:       items,
		Total:       total,
		UnreadCount: unreadCount,
	}, nil
}

// GetByID 根据ID获取通知
func (r *NotificationRepository) GetByID(id uuid.UUID) (*model.Notification, error) {
	var notification model.Notification
	if err := r.db.Where("id = ?", id).First(&notification).Error; err != nil {
		return nil, err
	}
	return &notification, nil
}

// MarkRead 将通知标为已读
func (r *NotificationRepository) MarkRead(id uuid.UUID) error {
	return r.db.Model(&model.Notification{}).Where("id = ?", id).Update("is_read", true).Error
}

// MarkAllRead 将所有通知标为已读
func (r *NotificationRepository) MarkAllRead() (int64, error) {
	result := r.db.Model(&model.Notification{}).Where("is_read = ?", false).Update("is_read", true)
	return result.RowsAffected, result.Error
}

// Create 创建通知
func (r *NotificationRepository) Create(notification *model.Notification) error {
	if notification.ID == uuid.Nil {
		notification.ID = uuid.New()
	}
	return r.db.Create(notification).Error
}