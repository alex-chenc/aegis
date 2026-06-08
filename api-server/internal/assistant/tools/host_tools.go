package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
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
		Name:               "Host.List",
		Domain:             assistant.DomainHost,
		Operation:          assistant.OpList,
		Capability:         "list_hosts",
		Description:        "列出所有主机，支持分页和关键字搜索",
		Aliases:            []string{"主机列表", "资产列表", "list hosts"},
		Tags:               []string{"v5.5", "host", "asset"},
		ObjectTypes:        []string{"host"},
		PageRoutes:         []string{"/hosts", "/assets"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":         map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size":    map[string]interface{}{"type": "integer", "description": "每页数量，默认20，最大100"},
				"limit":        map[string]interface{}{"type": "integer", "description": "兼容参数，等同于 page_size，最大100"},
				"query":        map[string]interface{}{"type": "string", "description": "搜索关键字（IP或主机名）"},
				"status":       map[string]interface{}{"type": "string", "description": "Agent 在线状态筛选：online/offline/all"},
				"agent_status": map[string]interface{}{"type": "string", "description": "Agent 在线状态筛选：online/offline/all"},
				"filters":      map[string]interface{}{"type": "array", "description": "兼容过滤条件，可包含 field=status/agent_status 与 value=online/offline"},
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
		Name:               "Host.Get",
		Domain:             assistant.DomainHost,
		Operation:          assistant.OpGet,
		Capability:         "get_host_detail",
		Description:        "根据主机ID获取主机详细信息",
		Aliases:            []string{"主机详情", "主机信息", "get host"},
		Tags:               []string{"v5.5", "host", "asset"},
		ObjectTypes:        []string{"host"},
		PageRoutes:         []string{"/hosts/:id"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
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
		Name:               "Host.AgentStatus.Get",
		Domain:             assistant.DomainHost,
		Operation:          assistant.OpGet,
		Capability:         "get_agent_status",
		Description:        "查询 Agent 在线状态，返回在线和离线主机统计",
		Aliases:            []string{"Agent状态", "在线状态", "离线主机"},
		Tags:               []string{"v5.5", "host", "agent", "status"},
		ObjectTypes:        []string{"host"},
		PageRoutes:         []string{"/hosts"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
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
		pageSize := normalizeHostPageSize(args)
		query := getStringArg(args, "query", "")
		status := normalizeHostStatusFilter(args)

		// Apply timeout to database queries to prevent indefinite blocking
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		queryPage := page
		queryPageSize := pageSize
		if status != "" {
			queryPage = 1
			queryPageSize = 1000
		}

		hosts, err := repo.FindAllWithContext(queryCtx, queryPage, queryPageSize, query)
		if err != nil {
			if queryCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("host list query timeout: %w", err)
			}
			return nil, fmt.Errorf("failed to list hosts: %w", err)
		}

		var total int64
		if status != "" {
			hosts = filterHostsByAgentStatus(hosts, status)
			total = int64(len(hosts))
			hosts = paginateHosts(hosts, page, pageSize)
		} else {
			var err error
			total, err = repo.CountWithContext(queryCtx, query)
			if err != nil {
				if queryCtx.Err() == context.DeadlineExceeded {
					return nil, fmt.Errorf("host count query timeout: %w", err)
				}
				return nil, fmt.Errorf("failed to count hosts: %w", err)
			}
		}

		return map[string]interface{}{
			"data":     hosts,
			"total":    total,
			"page":     page,
			"status":   statusOrAll(status),
			"has_more": int64(page*pageSize) < total,
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
		pageSize := normalizeHostPageSize(args)

		// Apply timeout to database query
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		hosts, err := repo.FindAllWithContext(queryCtx, 1, 1000, "")
		if err != nil {
			if queryCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("agent status query timeout: %w", err)
			}
			return nil, fmt.Errorf("failed to list hosts: %w", err)
		}

		onlineHosts := filterHostsByAgentStatus(hosts, "online")
		offlineHosts := filterHostsByAgentStatus(hosts, "offline")
		pagedHosts := paginateHosts(hosts, page, pageSize)

		return map[string]interface{}{
			"data":          pagedHosts,
			"total":         len(hosts),
			"online_hosts":  onlineHosts,
			"offline_hosts": offlineHosts,
			"online_total":  len(onlineHosts),
			"offline_total": len(offlineHosts),
		}, nil
	}
}

func normalizeHostPageSize(args map[string]interface{}) int {
	pageSize := getIntArg(args, "page_size", 0)
	if pageSize <= 0 {
		pageSize = getIntArg(args, "limit", 20)
	}
	if pageSize <= 0 {
		return 20
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}

func normalizeHostStatusFilter(args map[string]interface{}) string {
	for _, key := range []string{"status", "agent_status"} {
		if status := normalizeHostStatusValue(getStringArg(args, key, "")); status != "" {
			return status
		}
	}

	filters, _ := args["filters"].([]interface{})
	for _, item := range filters {
		filter, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		field := strings.ToLower(strings.TrimSpace(fmt.Sprint(filter["field"])))
		if field != "status" && field != "agent_status" {
			continue
		}
		if status := normalizeHostStatusValue(fmt.Sprint(filter["value"])); status != "" {
			return status
		}
	}

	return ""
}

func normalizeHostStatusValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "online", "在线":
		return "online"
	case "offline", "离线":
		return "offline"
	case "all", "全部", "所有":
		return ""
	default:
		return ""
	}
}

func filterHostsByAgentStatus(hosts []model.Host, status string) []model.Host {
	if status == "" {
		return hosts
	}
	filtered := make([]model.Host, 0, len(hosts))
	for _, host := range hosts {
		online := isHostAgentOnline(host)
		if (status == "online" && online) || (status == "offline" && !online) {
			filtered = append(filtered, host)
		}
	}
	return filtered
}

func isHostAgentOnline(host model.Host) bool {
	return !host.LastHeartbeatAt.IsZero() && host.LastHeartbeatAt.After(time.Now().Add(-2*time.Minute))
}

func paginateHosts(hosts []model.Host, page, pageSize int) []model.Host {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(hosts) {
		return []model.Host{}
	}
	end := start + pageSize
	if end > len(hosts) {
		end = len(hosts)
	}
	return hosts[start:end]
}

func statusOrAll(status string) string {
	if status == "" {
		return "all"
	}
	return status
}
