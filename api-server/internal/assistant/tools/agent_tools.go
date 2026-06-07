package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/grpc"
)

// AgentToolDeps Agent 工具依赖
type AgentToolDeps struct {
	ServerClient *grpc.ServerClient
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
		Description:        "获取目标主机上正在运行的进程列表，包含 PID、用户、命令行、CPU/内存使用率",
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
				"host_id": map[string]interface{}{"type": "string", "description": "目标主机 ID"},
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
		Description:        "获取目标主机的进程树，展示指定 PID 的父子进程关系；未提供 pid 时默认使用 PID 1",
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
				"host_id": map[string]interface{}{"type": "string", "description": "目标主机 ID"},
				"pid":     map[string]interface{}{"type": "number", "description": "进程 PID；可选，默认 1"},
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
		Description:        "获取目标主机的网络连接列表，包含源/目的 IP、端口、状态；可选传 pid 按进程过滤",
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
				"host_id": map[string]interface{}{"type": "string", "description": "目标主机 ID"},
				"pid":     map[string]interface{}{"type": "number", "description": "进程 PID；可选，不传则返回全量连接"},
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
		Description:        "获取目标主机上打开的文件列表",
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
				"host_id": map[string]interface{}{"type": "string", "description": "目标主机 ID"},
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
		Description:        "查询目标主机的历史日志，支持时间范围和关键字过滤",
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
				"host_id":    map[string]interface{}{"type": "string", "description": "目标主机 ID"},
				"start_time": map[string]interface{}{"type": "string", "description": "开始时间（RFC3339）"},
				"end_time":   map[string]interface{}{"type": "string", "description": "结束时间（RFC3339）"},
				"keyword":    map[string]interface{}{"type": "string", "description": "搜索关键字"},
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
func makeAgentToolHandler(serverClient *grpc.ServerClient, toolName string, timeoutSeconds int) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		hostID, _ := args["host_id"].(string)
		if hostID == "" {
			return nil, fmt.Errorf("host_id is required")
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
