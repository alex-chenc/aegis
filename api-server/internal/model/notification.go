package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Notification 系统全局通知消息
type Notification struct {
	// 通知唯一ID
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	// 通知标题（最大100字符）
	Title string `gorm:"not null;size:100" json:"title"`
	// 通知正文
	Content string `gorm:"not null;type:text" json:"content"`
	// 是否已读
	IsRead bool `gorm:"not null;default:false;index" json:"is_read"`
	// 告警级别：critical/high/medium/low/info
	Severity string `gorm:"not null;default:'info';size:20" json:"severity"`
	// 通知类型：rule_generated/alert_triggered/approval_required/system
	Type string `gorm:"not null;size:50;index" json:"type"`
	// 可选跳转路径
	Link string `gorm:"size:500" json:"link,omitempty"`
	// 扩展业务字段（JSON）：rule_id/mitre_id/trigger_count/alert_ids 等
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	// 创建时间
	CreatedAt time.Time `gorm:"not null;index:,sort:desc" json:"timestamp"`
	// 更新时间
	UpdatedAt time.Time `gorm:"not null" json:"-"`
}

// TableName 返回表名
func (Notification) TableName() string {
	return "notifications"
}