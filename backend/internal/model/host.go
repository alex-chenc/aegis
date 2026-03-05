package model

import "time"

type Host struct {
	ID              string    `json:"id"`
	IPAddress       string    `json:"ip_address"`
	Hostname        string    `json:"hostname"`
	OSType          string    `json:"os_type"`
	AgentVersion    string    `json:"agent_version"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
