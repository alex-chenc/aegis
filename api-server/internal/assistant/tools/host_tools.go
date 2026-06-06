package tools

import (
	"context"
	"fmt"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/repository"
	"github.com/google/uuid"
)

// HostToolDeps 主机工具依赖
type HostToolDeps struct {
	HostRepo *repository.HostRepository
}

// RegisterHostTools 注册主机域工具（对齐设计文档命名规范）
func RegisterHostTools(registry *assistant.ToolRegistry, deps HostToolDeps) error {
	// Host.List — 查询主机列表
	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Host.List",
		Domain:      assistant.DomainHost,
		Operation:   assistant.OpList,
		Capability:  "list_hosts",
		Description: "列出所有主机，支持分页和关键字搜索",
		Aliases:     []string{"主机列表", "资产列表", "list hosts"},
		Tags:        []string{"v5.5", "host", "asset"},
		ObjectTypes: []string{"host"},
		PageRoutes:  []string{"/hosts", "/assets"},
		Risk:        assistant.ToolRiskReadonly,
		AutoCallable: true,
		Idempotent:   true,
		DefaultWhitelisted: true,
		Enabled:      true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量，默认20，最大100"},
				"query":     map[string]interface{}{"type": "string", "description": "搜索关键字（IP或主机名）"},
			},
		},
		Handler: makeHostListHandler(deps.HostRepo),
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "api-server/internal/repository/host_repo.go",
			Function:  "HostRepository.FindAll",
		},
	}); err != nil {
		return err
	}

	// Host.Get — 获取主机详情（对齐设计文档命名：Host.Get 而非 Host.GetDetail）
	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Host.Get",
		Domain:      assistant.DomainHost,
		Operation:   assistant.OpGet,
		Capability:  "get_host_detail",
		Description: "根据主机ID获取主机详细信息",
		Aliases:     []string{"主机详情", "主机信息", "get host"},
		Tags:        []string{"v5.5", "host", "asset"},
		ObjectTypes: []string{"host"},
		PageRoutes:  []string{"/hosts/:id"},
		Risk:        assistant.ToolRiskReadonly,
		AutoCallable: true,
		Idempotent:   true,
		DefaultWhitelisted: true,
		Enabled:      true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id": map[string]interface{}{"type": "string", "description": "主机ID（UUID）"},
			},
			"required": []string{"host_id"},
		},
		Handler: makeHostGetDetailHandler(deps.HostRepo),
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "api-server/internal/repository/host_repo.go",
			Function:  "HostRepository.FindByID",
		},
	}); err != nil {
		return err
	}

	// Host.AgentStatus.Get — 查询 Agent 在线状态（对齐设计文档：Host.AgentStatus.Get 而非 Host.FindOffline）
	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Host.AgentStatus.Get",
		Domain:      assistant.DomainHost,
		Operation:   assistant.OpGet,
		Capability:  "get_agent_status",
		Description: "查询 Agent 在线状态，返回在线和离线主机统计",
		Aliases:     []string{"Agent状态", "在线状态", "离线主机"},
		Tags:        []string{"v5.5", "host", "agent", "status"},
		ObjectTypes: []string{"host"},
		PageRoutes:  []string{"/hosts"},
		Risk:        assistant.ToolRiskReadonly,
		AutoCallable: true,
		Idempotent:   true,
		DefaultWhitelisted: true,
		Enabled:      true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量"},
			},
		},
		Handler: makeHostAgentStatusHandler(deps.HostRepo),
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "api-server/internal/repository/host_repo.go",
			Function:  "HostRepository.FindAll + heartbeat filter",
		},
	}); err != nil {
		return err
	}

	return nil
}

func makeHostListHandler(repo *repository.HostRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)
		query := getStringArg(args, "query", "")

		// Apply timeout to database queries to prevent indefinite blocking
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		hosts, err := repo.FindAllWithContext(queryCtx, page, pageSize, query)
		if err != nil {
			if queryCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("host list query timeout: %w", err)
			}
			return nil, fmt.Errorf("failed to list hosts: %w", err)
		}

		total, err := repo.CountWithContext(queryCtx, query)
		if err != nil {
			if queryCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("host count query timeout: %w", err)
			}
			return nil, fmt.Errorf("failed to count hosts: %w", err)
		}

		return map[string]interface{}{
			"data":  hosts,
			"total": total,
		}, nil
	}
}

func makeHostGetDetailHandler(repo *repository.HostRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		hostIDStr := getStringArg(args, "host_id", "")
		if hostIDStr == "" {
			return nil, fmt.Errorf("host_id is required")
		}

		hostID, err := uuid.Parse(hostIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid host_id: %w", err)
		}

		// Apply timeout to database query
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		host, err := repo.FindByIDWithContext(queryCtx, hostID)
		if err != nil {
			if queryCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("host detail query timeout: %w", err)
			}
			return nil, fmt.Errorf("failed to find host: %w", err)
		}

		return host, nil
	}
}

func makeHostAgentStatusHandler(repo *repository.HostRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)

		// Apply timeout to database query
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// FindAll with empty query returns all hosts; we filter offline ones client-side.
		// Offline = last_heartbeat_at is older than 90 seconds or nil.
		hosts, err := repo.FindAllWithContext(queryCtx, page, pageSize, "")
		if err != nil {
			if queryCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("offline hosts query timeout: %w", err)
			}
			return nil, fmt.Errorf("failed to list hosts: %w", err)
		}

		threshold := time.Now().Add(-90 * time.Second)
		var offlineHosts []interface{}
		for _, h := range hosts {
			if h.LastHeartbeatAt.Before(threshold) || h.LastHeartbeatAt.IsZero() {
				offlineHosts = append(offlineHosts, h)
			}
		}

		return map[string]interface{}{
			"data":  offlineHosts,
			"total": len(offlineHosts),
		}, nil
	}
}
