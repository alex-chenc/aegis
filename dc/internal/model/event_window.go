package model

import (
	"time"

	"github.com/google/uuid"
)

// EventWindow represents a time window of aggregated events for a host
type EventWindow struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	HostID     uuid.UUID `gorm:"index;not null" json:"host_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	EventCount int       `json:"event_count"`
	Events     []*RuntimeEvent `gorm:"-" json:"events,omitempty"`
	HostWindows []*EventWindow `gorm:"-" json:"host_windows,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// TableName returns the table name for EventWindow
func (EventWindow) TableName() string {
	return "event_windows"
}
