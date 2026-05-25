package correlation

import (
	"time"
)

type AtomicFinding struct {
	PackageID string
	Version   string
	RuleID    string
	EventType string
	Timestamp int64
	HostID    string
	Hostname  string
	PID       int
	PPID      int
	UID       int
	Process   ProcessContext
	EventMap  map[string]interface{}
}

type CorrelationAlert struct {
	SpecID      string
	PackageID   string
	Title       string
	Severity    string
	MitreID     string
	CVEID       string
	Evidence    []AtomicFinding
	TriggeredAt time.Time
}

type EvidenceItem struct {
	RuleID    string
	Timestamp int64
	HostID    string
	PID       int
}
