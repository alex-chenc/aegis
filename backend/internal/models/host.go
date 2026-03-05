package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Host struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	IPAddress         string         `gorm:"type:varchar(45);not null;uniqueIndex" json:"ip_address"`
	Hostname          string         `gorm:"type:varchar(255);not null;index" json:"hostname"`
	OsType            string         `gorm:"type:varchar(50);not null" json:"os_type"`
	OsVersion         string         `gorm:"type:varchar(100);not null" json:"os_version"`
	KernelVersion     string         `gorm:"type:varchar(100)" json:"kernel_version"`
	AgentVersion      string         `gorm:"type:varchar(50);not null" json:"agent_version"`
	Architecture      string         `gorm:"type:varchar(20);not null" json:"architecture"`
	CpuInfo           datatypes.JSON `gorm:"type:jsonb" json:"cpu_info"`
	TotalMemoryMB     int64          `gorm:"type:bigint" json:"total_memory_mb"`
	TotalDiskGB       int64          `gorm:"type:bigint" json:"total_disk_gb"`
	NetworkInterfaces datatypes.JSON `gorm:"type:jsonb" json:"network_interfaces"`
	CpuLoad1Min       float32        `gorm:"type:real" json:"cpu_load_1min"`
	MemUsagePercent   float32        `gorm:"type:real" json:"mem_usage_percent"`
	LastHeartbeatAt   time.Time      `gorm:"not null;index" json:"last_heartbeat_at"`
	IsOnline          bool           `gorm:"default:false" json:"is_online"`
	CreatedAt         time.Time      `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (Host) TableName() string {
	return "hosts"
}

func (h *Host) CheckOnline() bool {
	return time.Since(h.LastHeartbeatAt) < 70*time.Second
}

type HostListParams struct {
	Page     int    `form:"page" json:"page"`
	PageSize int    `form:"page_size" json:"page_size"`
	Search   string `form:"search" json:"search"`
}

type HostResponse struct {
	Total int    `json:"total"`
	Items []Host `json:"items"`
}
