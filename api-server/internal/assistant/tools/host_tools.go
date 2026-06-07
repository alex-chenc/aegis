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

// RegisterHostTools 注册主机域工具
func RegisterHostTools(registry *assistant.ToolRegistry, deps HostToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Host.List",
		Domain:      "host",
		Operation:   "list",
		Description: "列出所有主机，支持分页和关键字搜索",
		RiskLevel:   "low",
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量"},
				"query":     map[string]interface{}{"type": "string", "description": "搜索关键字（IP或主机名）"},
			},
		},
		Handler: makeHostListHandler(deps.HostRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Host.GetDetail",
		Domain:      "host",
		Operation:   "get_detail",
		Description: "根据主机ID获取主机详细信息",
		RiskLevel:   "low",
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id": map[string]interface{}{"type": "string", "description": "主机ID（UUID）"},
			},
			"required": []string{"host_id"},
		},
		Handler: makeHostGetDetailHandler(deps.HostRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Host.FindOffline",
		Domain:      "host",
		Operation:   "find_offline",
		Description: "查找离线主机，通过查询心跳超时的主机",
		RiskLevel:   "low",
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量"},
			},
		},
		Handler: makeHostFindOfflineHandler(deps.HostRepo),
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

		hosts, err := repo.FindAll(page, pageSize, query)
		if err != nil {
			return nil, fmt.Errorf("failed to list hosts: %w", err)
		}

		total, err := repo.Count(query)
		if err != nil {
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

		host, err := repo.FindByID(hostID)
		if err != nil {
			return nil, fmt.Errorf("failed to find host: %w", err)
		}

		return host, nil
	}
}

func makeHostFindOfflineHandler(repo *repository.HostRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)

		// FindAll with empty query returns all hosts; we filter offline ones client-side.
		// Offline = last_heartbeat_at is older than 90 seconds or nil.
		hosts, err := repo.FindAll(page, pageSize, "")
		if err != nil {
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
