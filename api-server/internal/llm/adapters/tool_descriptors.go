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
		Description: "获取指定主机上指定进程的完整进程树",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "主机ID"},
				"pid":     map[string]any{"type": "number", "description": "进程PID"},
			},
			"required": []string{"host_id", "pid"},
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
		Description: "获取指定主机的网络连接信息",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "主机ID"},
				"pid":     map[string]any{"type": "number", "description": "进程PID（可选，按进程过滤）"},
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
		Description: "获取指定进程打开的文件列表",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "主机ID"},
				"pid":     map[string]any{"type": "number", "description": "进程PID"},
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
		Description: "获取指定主机上正在运行的进程列表",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "主机ID"},
				"filter":  map[string]any{"type": "string", "description": "进程名过滤条件（可选）"},
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
		Description: "获取指定主机上的用户会话信息",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]any{"type": "string", "description": "主机ID"},
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
		Description: "查询指定主机的历史日志",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id":    map[string]any{"type": "string", "description": "主机ID"},
				"start_time": map[string]any{"type": "string", "description": "查询起始时间"},
				"end_time":   map[string]any{"type": "string", "description": "查询结束时间"},
				"filter":     map[string]any{"type": "string", "description": "日志过滤条件（可选）"},
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
