package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"api-server/internal/assistant"
)

// AgentToolDeps Agent 工具依赖
type AgentToolDeps struct {
	ServerClient agentToolClient
}

// RegisterAgentTools 注册 Agent 域工具（对齐设计文档 14.1 节）
// 这些工具通过 server gRPC ExecuteTool 转发到目标主机的 Agent
func RegisterAgentTools(registry *assistant.ToolRegistry, deps AgentToolDeps) error {
	// Agent.Process.List — 获取运行进程列表
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Agent.Process.List",
		Domain:             assistant.DomainAgent,
		Operation:          assistant.OpList,
		Capability:         "list_running_processes",
		Description:        "Get running processes on a target host, including PID, user, command line, CPU utilization, and memory utilization.",
		Aliases:            []string{"进程列表", "运行进程", "list processes"},
		Tags:               []string{"v5.5", "agent", "process", "forensics"},
		ObjectTypes:        []string{"host"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact target host UUID."},
			},
			"required": []string{"host_id"},
		},
		DefaultTimeout: 30 * time.Second,
		Handler:        makeAgentToolHandler(deps.ServerClient, "GetRunningProcesses", 30),
		ServiceBinding: assistant.ServiceBinding{
			Component: "agent",
			File:      "agent/internal/tools/process.go",
			Function:  "GetRunningProcesses",
		},
	}); err != nil {
		return err
	}

	// Agent.Process.Tree — 获取进程树
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Agent.Process.Tree",
		Domain:             assistant.DomainAgent,
		Operation:          assistant.OpGet,
		Capability:         "get_process_tree",
		Description:        "Get the process tree for a target host and show parent-child relationships for a PID; defaults to PID 1.",
		Aliases:            []string{"进程树", "process tree"},
		Tags:               []string{"v5.5", "agent", "process", "forensics"},
		ObjectTypes:        []string{"host"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact target host UUID."},
				"pid":     map[string]interface{}{"type": "number", "description": "Optional process PID; defaults to 1."},
			},
			"required": []string{"host_id"},
		},
		DefaultTimeout: 30 * time.Second,
		Handler:        makeAgentToolHandler(deps.ServerClient, "GetProcessTree", 30),
		ServiceBinding: assistant.ServiceBinding{
			Component: "agent",
			File:      "agent/internal/tools/process.go",
			Function:  "GetProcessTree",
		},
	}); err != nil {
		return err
	}

	// Agent.Network.List — 获取网络连接列表
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Agent.Network.List",
		Domain:             assistant.DomainAgent,
		Operation:          assistant.OpList,
		Capability:         "list_network_connections",
		Description:        "Get network connections on a target host, including source and destination addresses, ports, and state, with an optional PID filter.",
		Aliases:            []string{"网络连接", "网络状态", "list connections"},
		Tags:               []string{"v5.5", "agent", "network", "forensics"},
		ObjectTypes:        []string{"host"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact target host UUID."},
				"pid":     map[string]interface{}{"type": "number", "description": "Optional process PID; omit it to return all connections."},
			},
			"required": []string{"host_id"},
		},
		DefaultTimeout: 30 * time.Second,
		Handler:        makeAgentToolHandler(deps.ServerClient, "GetNetworkConnections", 30),
		ServiceBinding: assistant.ServiceBinding{
			Component: "agent",
			File:      "agent/internal/tools/network.go",
			Function:  "GetNetworkConnections",
		},
	}); err != nil {
		return err
	}

	// Agent.File.OpenList — 获取打开文件列表
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Agent.File.OpenList",
		Domain:             assistant.DomainAgent,
		Operation:          assistant.OpList,
		Capability:         "list_open_files",
		Description:        "Get the list of open files on a target host.",
		Aliases:            []string{"打开文件", "文件句柄", "open files"},
		Tags:               []string{"v5.5", "agent", "file", "forensics"},
		ObjectTypes:        []string{"host"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact target host UUID."},
			},
			"required": []string{"host_id"},
		},
		DefaultTimeout: 30 * time.Second,
		Handler:        makeAgentToolHandler(deps.ServerClient, "GetOpenFiles", 30),
		ServiceBinding: assistant.ServiceBinding{
			Component: "agent",
			File:      "agent/internal/tools/file.go",
			Function:  "GetOpenFiles",
		},
	}); err != nil {
		return err
	}

	// Agent.Log.Query — 查询历史日志
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Agent.Log.Query",
		Domain:             assistant.DomainAgent,
		Operation:          assistant.OpSearch,
		Capability:         "query_historical_logs",
		Description:        "Query historical logs from a target host with optional time-range and keyword filters.",
		Aliases:            []string{"日志查询", "历史日志", "query logs"},
		Tags:               []string{"v5.5", "agent", "log", "forensics"},
		ObjectTypes:        []string{"host"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id":    map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact target host UUID."},
				"start_time": map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive RFC3339 start time."},
				"end_time":   map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive RFC3339 end time."},
				"keyword":    map[string]interface{}{"type": "string", "description": "Log search keyword."},
			},
			"required": []string{"host_id"},
		},
		DefaultTimeout: 60 * time.Second,
		Handler:        makeAgentToolHandler(deps.ServerClient, "QueryHistoricalLogs", 60),
		ServiceBinding: assistant.ServiceBinding{
			Component: "agent",
			File:      "agent/internal/tools/log.go",
			Function:  "QueryHistoricalLogs",
		},
	}); err != nil {
		return err
	}

	return nil
}

// makeAgentToolHandler 创建 Agent 工具的 gRPC handler
// timeoutSeconds 为 gRPC 调用超时时间，应与 ToolSpec.DefaultTimeout 对应
func makeAgentToolHandler(serverClient agentToolClient, toolName string, timeoutSeconds int) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		hostID, _ := args["host_id"].(string)
		if hostID == "" {
			return nil, fmt.Errorf("host_id is required")
		}
		if serverClient == nil {
			return nil, fmt.Errorf("agent server client is not initialized")
		}

		status, err := serverClient.GetAgentStatus(ctx, hostID)
		if err != nil {
			return nil, fmt.Errorf("failed to get agent status for host %s: %w", hostID, err)
		}
		if status == nil || !status.Connected {
			return map[string]interface{}{
				"host_id":           hostID,
				"tool":              toolName,
				"agent_status":      "offline",
				"agent_connected":   false,
				"runtime_available": false,
				"skipped":           true,
				"reason":            "target agent is not connected; runtime evidence is unavailable",
			}, nil
		}

		// 设置工具默认参数，兼容已部署的旧 Agent 对 pid 的要求。
		switch toolName {
		case "GetProcessTree":
			if _, ok := args["pid"]; !ok {
				args["pid"] = 1
			}
		case "GetNetworkConnections":
			if _, ok := args["pid"]; !ok {
				args["pid"] = 0
			}
		case "QueryHistoricalLogs":
			now := time.Now()
			if _, ok := args["end_time"]; !ok {
				args["end_time"] = now.Format(time.RFC3339)
			}
			if _, ok := args["start_time"]; !ok {
				args["start_time"] = now.Add(-24 * time.Hour).Format(time.RFC3339)
			}
		}

		argsJSON, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal args: %w", err)
		}

		// 通过 gRPC ExecuteTool 转发到 Agent
		resp, err := serverClient.ExecuteTool(ctx, "", hostID, toolName, string(argsJSON), int32(timeoutSeconds))
		if err != nil {
			return nil, fmt.Errorf("gRPC ExecuteTool failed: %w", err)
		}

		if !resp.Success {
			return nil, fmt.Errorf("agent tool execution failed: %s", resp.Error)
		}

		// 尝试解析 JSON 结果
		var result interface{}
		if err := json.Unmarshal([]byte(resp.Result), &result); err != nil {
			return resp.Result, nil
		}
		return result, nil
	}
}
