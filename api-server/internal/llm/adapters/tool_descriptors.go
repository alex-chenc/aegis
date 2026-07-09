package adapters

import (
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// AegisTools defines the tool descriptors for Aegis host security operations,
// intended for use with the agent-runtime SDK.
var AegisTools = []agentruntime.ToolDescriptor{
	{
		Name:        "GetProcessTree",
		Description: "Retrieve the complete process tree for a process on a host; pid is optional and defaults to 1.",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "Host ID."},
				"pid":     map[string]any{"type": "number", "description": "Optional process PID; defaults to 1."},
			},
			"required": []string{"host_id"},
		},
		RiskLevel:        agentruntime.RiskReadOnly,
		AutoCallable:     true,
		RequiresApproval: false,
		DefaultTimeout:   60 * time.Second,
		Idempotent:       true,
		Tags:             []string{"process", "inspection"},
	},
	{
		Name:        "GetNetworkConnections",
		Description: "Retrieve network connections for a host, optionally filtered by process.",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "Host ID."},
				"pid":     map[string]any{"type": "number", "description": "Optional process PID filter."},
			},
			"required": []string{"host_id"},
		},
		RiskLevel:        agentruntime.RiskReadOnly,
		AutoCallable:     true,
		RequiresApproval: false,
		DefaultTimeout:   60 * time.Second,
		Idempotent:       true,
		Tags:             []string{"network", "inspection"},
	},
	{
		Name:        "GetOpenFiles",
		Description: "Retrieve files opened by a process on a host.",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "Host ID."},
				"pid":     map[string]any{"type": "number", "description": "Process PID."},
			},
			"required": []string{"host_id", "pid"},
		},
		RiskLevel:        agentruntime.RiskReadOnly,
		AutoCallable:     true,
		RequiresApproval: false,
		DefaultTimeout:   60 * time.Second,
		Idempotent:       true,
		Tags:             []string{"file", "inspection"},
	},
	{
		Name:        "GetRunningProcesses",
		Description: "Retrieve running processes for a host.",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "Host ID."},
				"filter":  map[string]any{"type": "string", "description": "Optional process-name filter."},
			},
			"required": []string{"host_id"},
		},
		RiskLevel:        agentruntime.RiskReadOnly,
		AutoCallable:     true,
		RequiresApproval: false,
		DefaultTimeout:   60 * time.Second,
		Idempotent:       true,
		Tags:             []string{"process", "inspection"},
	},
	{
		Name:        "GetUserSessions",
		Description: "Retrieve active user sessions for a host.",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "Host ID."},
			},
			"required": []string{"host_id"},
		},
		RiskLevel:        agentruntime.RiskReadOnly,
		AutoCallable:     true,
		RequiresApproval: false,
		DefaultTimeout:   60 * time.Second,
		Idempotent:       true,
		Tags:             []string{"user", "session", "inspection"},
	},
	{
		Name:        "QueryHistoricalLogs",
		Description: "Query historical logs for a host within an explicit time range.",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id":    map[string]any{"type": "string", "description": "Host ID."},
				"start_time": map[string]any{"type": "string", "description": "Query start time."},
				"end_time":   map[string]any{"type": "string", "description": "Query end time."},
				"filter":     map[string]any{"type": "string", "description": "Optional log filter."},
			},
			"required": []string{"host_id", "start_time", "end_time"},
		},
		RiskLevel:        agentruntime.RiskReadOnly,
		AutoCallable:     true,
		RequiresApproval: false,
		DefaultTimeout:   60 * time.Second,
		Idempotent:       true,
		Tags:             []string{"log", "history", "inspection"},
	},
}
