package model

import (
	"time"

	"github.com/google/uuid"
)

type ToolCall struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CallID     string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"call_id"`
	HostID     uuid.UUID `gorm:"type:uuid;not null;index" json:"host_id"`
	Tool       string    `gorm:"type:varchar(50);not null" json:"tool"`
	ParamsJSON string    `gorm:"type:text;column:params_json" json:"params_json"`
	ResultJSON string    `gorm:"type:text;column:result_json" json:"result_json"`
	Status     string    `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Error      string    `gorm:"type:text" json:"error"`
	CreatedAt  time.Time `gorm:"default:now()" json:"created_at"`
}

func (ToolCall) TableName() string { return "tool_calls" }
