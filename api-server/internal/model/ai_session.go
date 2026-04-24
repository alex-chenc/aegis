package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StringArray is a custom type for JSON array of strings
type StringArray []string

func (s StringArray) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}

// AISession AI分析会话
type AISession struct {
	ID            uuid.UUID   `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID     string      `gorm:"type:varchar(100);uniqueIndex;not null" json:"session_id"`
	AlertIDs      StringArray `gorm:"type:jsonb" json:"alert_ids"`
	HostIDs       StringArray `gorm:"type:jsonb" json:"host_ids"`
	HostFilter    StringArray `gorm:"type:jsonb" json:"host_filter"`
	TimeRange     JSONB       `gorm:"type:jsonb" json:"time_range,omitempty"`
	Status        string      `gorm:"type:varchar(20);default:active" json:"status"`
	MaxIterations int         `gorm:"default:15" json:"max_iterations"`
	MessageCount  int         `gorm:"default:0" json:"message_count"`
	ToolCallCount int         `gorm:"default:0" json:"tool_call_count"`
	CreatedAt     time.Time   `gorm:"default:now()" json:"created_at"`
	UpdatedAt     time.Time   `gorm:"default:now()" json:"updated_at"`
	ConcludedAt   *time.Time  `json:"concluded_at,omitempty"`
	Conclusion    JSONB       `gorm:"type:jsonb" json:"conclusion,omitempty"`
}

func (AISession) TableName() string {
	return "ai_analysis_session"
}

// AIMessage AI分析消息
type AIMessage struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID   string    `gorm:"type:varchar(100);index;not null" json:"session_id"`
	MessageID   string    `gorm:"type:varchar(100);uniqueIndex;not null;default:gen_random_uuid()::text" json:"message_id"`
	Role        string    `gorm:"type:varchar(20);not null" json:"role"`
	Content     string    `gorm:"type:text" json:"content"`
	Thinking    string    `gorm:"type:text" json:"thinking,omitempty"`
	ToolCalls   JSONB     `gorm:"type:jsonb" json:"tool_calls,omitempty"`
	ToolResults JSONB     `gorm:"type:jsonb" json:"tool_results,omitempty"`
	Steps       JSONB     `gorm:"type:jsonb" json:"steps,omitempty"`
	CreatedAt   time.Time `gorm:"default:now()" json:"created_at"`
}

func (AIMessage) TableName() string {
	return "ai_analysis_message"
}
