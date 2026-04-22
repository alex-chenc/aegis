package model

import (
	"time"

	"github.com/google/uuid"
)

type Host struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	IPAddress       string    `gorm:"type:varchar(45);uniqueIndex;not null" json:"ip_address"`
	Hostname        string    `gorm:"type:varchar(255);not null" json:"hostname"`
	OSType          string    `gorm:"column:os_version;type:varchar(100)" json:"os_version"`
	AgentVersion    string    `gorm:"type:varchar(50);not null" json:"agent_version"`
	LastHeartbeatAt time.Time `gorm:"not null" json:"last_heartbeat_at"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Host) TableName() string {
	return "hosts"
}
