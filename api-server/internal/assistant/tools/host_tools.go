package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/repository"
	"github.com/google/uuid"
)

// HostToolDeps 主机工具依赖
type HostToolDeps struct {
	HostRepo     *repository.HostRepository
	ServerClient agentStatusClient
}

const hostTargetScopeAllOnline = "all_online_hosts"

// RegisterHostTools 注册主机域工具（对齐设计文档命名规范）
func RegisterHostTools(registry *assistant.ToolRegistry, deps HostToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Host.Resolve",
		Domain:             assistant.DomainHost,
		Operation:          assistant.OpGet,
		Capability:         "resolve_hosts",
		Description:        "Resolve user-provided host selectors to exact host UUIDs and report ambiguity, missing targets, and online status.",
		Aliases:            []string{"解析主机", "主机选择器", "resolve hosts"},
		Tags:               []string{"v6.1", "host", "resolver"},
		ObjectTypes:        []string{"host"},
		PageRoutes:         []string{"/hosts", "/baseline"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ExposurePolicy: assistant.ToolExposurePolicy{
			Exposure:        assistant.ToolExposurePrimary,
			Discoverable:    true,
			DirectCallable:  true,
			CatalogPriority: 100,
		},
		ResultContract: assistant.ToolResultContract{
			OperationStatusField: "operation_status",
			SuccessValues:        []string{"succeeded"},
			FailureValues:        []string{"failed"},
			FactBindings: []assistant.ToolFactBinding{{
				Kind:       "host_resolved",
				ItemsField: "resolved",
				IDField:    "host_id",
				StateField: "agent_status",
			}},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_selectors": map[string]interface{}{
					"type":        "array",
					"minItems":    1,
					"items":       map[string]interface{}{"type": "string"},
					"description": "Explicit host UUIDs, IP addresses, hostnames, or short labels supplied by the user. Do not use natural-language groups here.",
					"examples":    []interface{}{[]interface{}{"159IP"}},
				},
				"target_scope": map[string]interface{}{
					"type":        "string",
					"enum":        []interface{}{hostTargetScopeAllOnline},
					"description": "Deterministic server-side host scope. Use all_online_hosts for every currently connected or recently heartbeating host. Mutually exclusive with host_selectors.",
				},
				"require_online": map[string]interface{}{
					"type":        "boolean",
					"description": "When true, report resolved offline hosts as unavailable for runtime operations.",
				},
			},
			"additionalProperties": false,
		},
		Preflight: validateHostResolveArgs,
		Handler:   makeHostResolveHandler(deps.HostRepo, deps.ServerClient),
	}); err != nil {
		return err
	}

	// Host.List — 查询主机列表
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Host.List",
		Domain:             assistant.DomainHost,
		Operation:          assistant.OpList,
		Capability:         "list_hosts",
		Description:        "List hosts with pagination, keyword search, and agent-status filters.",
		ModelDescription:   "List hosts with pagination, query, and online or offline agent-status filters.",
		Aliases:            []string{"主机列表", "资产列表", "list hosts"},
		Tags:               []string{"v5.5", "host", "asset"},
		ObjectTypes:        []string{"host"},
		PageRoutes:         []string{"/hosts", "/assets"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ResultContract: assistant.ToolResultContract{
			FactBindings: []assistant.ToolFactBinding{{
				Kind:       "host_online",
				ItemsField: "data",
				IDField:    "id",
				StateField: "agent_status",
				StateValue: "online",
			}},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":         map[string]interface{}{"type": "integer", "description": "One-based page number."},
				"page_size":    map[string]interface{}{"type": "integer", "description": "Items per page; defaults to 20 and is capped at 100."},
				"limit":        map[string]interface{}{"type": "integer", "description": "Compatibility alias for page_size; capped at 100."},
				"query":        map[string]interface{}{"type": "string", "description": "Keyword matched against IP address or hostname."},
				"status":       map[string]interface{}{"type": "string", "description": "Agent status filter: online, offline, or all."},
				"agent_status": map[string]interface{}{"type": "string", "description": "Compatibility agent status filter: online, offline, or all."},
				"filters":      map[string]interface{}{"type": "array", "description": "Compatibility filters containing field=status or agent_status and value=online or offline."},
			},
		},
		Handler: makeHostListHandler(deps.HostRepo, deps.ServerClient),
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
		Description:        "Get detailed host information by exact host UUID.",
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
				"host_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact host UUID."},
			},
			"required": []string{"host_id"},
		},
		Handler: makeHostGetDetailHandler(deps.HostRepo, deps.ServerClient),
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
		Description:        "Get agent connectivity status and counts for online and offline hosts.",
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
				"page":      map[string]interface{}{"type": "integer", "description": "One-based page number."},
				"page_size": map[string]interface{}{"type": "integer", "description": "Items per page."},
			},
		},
		Handler: makeHostAgentStatusHandler(deps.HostRepo, deps.ServerClient),
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

type hostResolution struct {
	TargetScope     string                   `json:"target_scope,omitempty"`
	Requested       []string                 `json:"requested"`
	Resolved        []map[string]interface{} `json:"resolved"`
	Ambiguous       []map[string]interface{} `json:"ambiguous"`
	Unresolved      []string                 `json:"unresolved"`
	Offline         []map[string]interface{} `json:"offline,omitempty"`
	Coverage        map[string]int           `json:"coverage"`
	OperationStatus string                   `json:"operation_status"`
}

func makeHostResolveHandler(repo *repository.HostRepository, statusClient agentStatusClient) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		return resolveHostTargetInput(ctx, repo, statusClient, args, getBoolArg(args, "require_online", false))
	}
}

func validateHostResolveArgs(_ context.Context, args map[string]interface{}) error {
	targetScope := strings.ToLower(strings.TrimSpace(getStringArg(args, "target_scope", "")))
	selectorsRaw, selectorsProvided := args["host_selectors"]
	if targetScope != "" {
		if targetScope != hostTargetScopeAllOnline {
			return fmt.Errorf("unsupported target_scope %q", targetScope)
		}
		if selectorsProvided && selectorsRaw != nil {
			selectors, err := getStringSliceArg(args, "host_selectors")
			if err != nil {
				return fmt.Errorf("host_selectors: %w", err)
			}
			if len(selectors) > 0 {
				return fmt.Errorf("target_scope and host_selectors are mutually exclusive")
			}
		}
		return nil
	}
	if !selectorsProvided {
		return fmt.Errorf("provide host_selectors or target_scope=all_online_hosts")
	}
	selectors, err := getStringSliceArg(args, "host_selectors")
	if err != nil {
		return fmt.Errorf("host_selectors: %w", err)
	}
	if len(selectors) == 0 {
		return fmt.Errorf("at least one host selector is required")
	}
	return nil
}

// resolveHostTargetInput owns the boundary between model arguments and actual
// host identities. Semantic scopes are enumerated server-side; literal
// selectors remain available for precise user-provided targets.
func resolveHostTargetInput(ctx context.Context, repo *repository.HostRepository, statusClient agentStatusClient, args map[string]interface{}, requireOnline bool) (*hostResolution, error) {
	targetScope := strings.ToLower(strings.TrimSpace(getStringArg(args, "target_scope", "")))
	selectorsRaw, selectorsProvided := args["host_selectors"]
	if targetScope != "" {
		if targetScope != hostTargetScopeAllOnline {
			return nil, fmt.Errorf("unsupported target_scope %q", targetScope)
		}
		if selectorsProvided && selectorsRaw != nil {
			selectors, err := getStringSliceArg(args, "host_selectors")
			if err != nil {
				return nil, fmt.Errorf("host_selectors: %w", err)
			}
			if len(selectors) > 0 {
				return nil, fmt.Errorf("target_scope and host_selectors are mutually exclusive")
			}
		}
		return resolveAllOnlineHosts(ctx, repo, statusClient)
	}
	if !selectorsProvided {
		return nil, fmt.Errorf("provide host_selectors or target_scope=all_online_hosts")
	}
	selectors, err := getStringSliceArg(args, "host_selectors")
	if err != nil {
		return nil, fmt.Errorf("host_selectors: %w", err)
	}
	return resolveHostSelectors(ctx, repo, statusClient, selectors, requireOnline)
}

var compactIPSelectorPattern = regexp.MustCompile(`(?i)^\s*(\d{1,3})\s*(?:ip|号主机|号)?\s*$`)

func resolveHostSelectors(ctx context.Context, repo *repository.HostRepository, statusClient agentStatusClient, selectors []string, requireOnline bool) (*hostResolution, error) {
	if repo == nil {
		return nil, fmt.Errorf("host repository not configured")
	}
	if len(selectors) == 0 {
		return nil, fmt.Errorf("at least one host selector is required")
	}
	statuses := loadAgentRuntimeStatuses(ctx, statusClient)
	result := &hostResolution{Requested: append([]string(nil), selectors...)}
	seen := make(map[uuid.UUID]struct{})

	for _, rawSelector := range selectors {
		selector := strings.TrimSpace(rawSelector)
		if selector == "" {
			result.Unresolved = append(result.Unresolved, rawSelector)
			continue
		}

		var candidates []model.Host
		if hostID, err := uuid.Parse(selector); err == nil {
			host, findErr := repo.FindByIDWithContext(ctx, hostID)
			if findErr == nil && host != nil {
				candidates = []model.Host{*host}
			}
		} else {
			query := normalizeHostSelectorQuery(selector)
			var findErr error
			candidates, findErr = repo.FindAllWithContext(ctx, 1, 100, query)
			if findErr != nil {
				return nil, fmt.Errorf("resolve host selector %q: %w", selector, findErr)
			}
			candidates = preferExactHostCandidates(selector, query, candidates)
		}

		switch len(candidates) {
		case 0:
			result.Unresolved = append(result.Unresolved, selector)
		case 1:
			host := candidates[0]
			if _, exists := seen[host.ID]; exists {
				continue
			}
			seen[host.ID] = struct{}{}
			item := decorateHostWithAgentStatus(host, statuses)
			item["host_id"] = host.ID.String()
			item["selector"] = selector
			item["matched_by"] = hostSelectorMatchReason(selector, host)
			if requireOnline && !isHostAgentOnlineWithRuntime(host, statuses) {
				result.Offline = append(result.Offline, item)
				continue
			}
			result.Resolved = append(result.Resolved, item)
		default:
			items := decorateHostsWithAgentStatus(candidates, statuses)
			result.Ambiguous = append(result.Ambiguous, map[string]interface{}{
				"selector":   selector,
				"candidates": items,
			})
		}
	}

	finalizeHostResolution(result)
	return result, nil
}

func resolveAllOnlineHosts(ctx context.Context, repo *repository.HostRepository, statusClient agentStatusClient) (*hostResolution, error) {
	if repo == nil {
		return nil, fmt.Errorf("host repository not configured")
	}
	hosts, err := repo.FindAllWithContext(ctx, 1, 1000, "")
	if err != nil {
		return nil, fmt.Errorf("list all online hosts: %w", err)
	}
	statuses := loadAgentRuntimeStatuses(ctx, statusClient)
	result := &hostResolution{
		TargetScope: hostTargetScopeAllOnline,
		Requested:   []string{hostTargetScopeAllOnline},
	}
	for _, host := range filterHostsByAgentStatusWithRuntime(hosts, "online", statuses) {
		item := decorateHostWithAgentStatus(host, statuses)
		item["host_id"] = host.ID.String()
		item["selector"] = hostTargetScopeAllOnline
		item["matched_by"] = "target_scope"
		result.Resolved = append(result.Resolved, item)
	}
	finalizeHostResolution(result)
	return result, nil
}

func finalizeHostResolution(result *hostResolution) {
	if result == nil {
		return
	}
	result.Coverage = map[string]int{
		"requested":  len(result.Requested),
		"resolved":   len(result.Resolved),
		"ambiguous":  len(result.Ambiguous),
		"unresolved": len(result.Unresolved),
		"offline":    len(result.Offline),
	}
	if len(result.Resolved) > 0 && len(result.Ambiguous) == 0 && len(result.Unresolved) == 0 && len(result.Offline) == 0 {
		result.OperationStatus = "succeeded"
		return
	}
	result.OperationStatus = "failed"
}

func hostSelectorMatchReason(selector string, host model.Host) string {
	if _, err := uuid.Parse(strings.TrimSpace(selector)); err == nil {
		return "host_uuid"
	}
	if strings.EqualFold(strings.TrimSpace(selector), host.IPAddress) {
		return "ip_exact"
	}
	if strings.EqualFold(strings.TrimSpace(selector), host.Hostname) {
		return "hostname_exact"
	}
	if normalizeHostSelectorQuery(selector) != strings.TrimSpace(selector) {
		return "ip_token_unique"
	}
	return "query_unique"
}

func normalizeHostSelectorQuery(selector string) string {
	if matches := compactIPSelectorPattern.FindStringSubmatch(selector); len(matches) == 2 {
		return matches[1]
	}
	return selector
}

func preferExactHostCandidates(selector, normalized string, candidates []model.Host) []model.Host {
	exact := make([]model.Host, 0, len(candidates))
	for _, host := range candidates {
		if strings.EqualFold(host.IPAddress, selector) || strings.EqualFold(host.Hostname, selector) {
			exact = append(exact, host)
			continue
		}
		if normalized != selector && strings.HasSuffix(host.IPAddress, "."+normalized) {
			exact = append(exact, host)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return candidates
}

func makeHostListHandler(repo *repository.HostRepository, statusClient agentStatusClient) assistant.ToolHandler {
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

		statuses := loadAgentRuntimeStatuses(ctx, statusClient)
		var total int64
		if status != "" {
			hosts = filterHostsByAgentStatusWithRuntime(hosts, status, statuses)
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
			"data":     decorateHostsWithAgentStatus(hosts, statuses),
			"total":    total,
			"page":     page,
			"status":   statusOrAll(status),
			"has_more": int64(page*pageSize) < total,
		}, nil
	}
}

func makeHostGetDetailHandler(repo *repository.HostRepository, statusClient agentStatusClient) assistant.ToolHandler {
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

		statuses := loadAgentRuntimeStatuses(ctx, statusClient)
		return decorateHostWithAgentStatus(*host, statuses), nil
	}
}

func makeHostAgentStatusHandler(repo *repository.HostRepository, statusClient agentStatusClient) assistant.ToolHandler {
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

		statuses := loadAgentRuntimeStatuses(ctx, statusClient)
		onlineHosts := filterHostsByAgentStatusWithRuntime(hosts, "online", statuses)
		offlineHosts := filterHostsByAgentStatusWithRuntime(hosts, "offline", statuses)
		pagedHosts := paginateHosts(hosts, page, pageSize)

		return map[string]interface{}{
			"data":          decorateHostsWithAgentStatus(pagedHosts, statuses),
			"total":         len(hosts),
			"online_hosts":  decorateHostsWithAgentStatus(onlineHosts, statuses),
			"offline_hosts": decorateHostsWithAgentStatus(offlineHosts, statuses),
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
	return filterHostsByAgentStatusWithRuntime(hosts, status, nil)
}

func filterHostsByAgentStatusWithRuntime(hosts []model.Host, status string, statuses map[string]agentRuntimeStatus) []model.Host {
	if status == "" {
		return hosts
	}
	filtered := make([]model.Host, 0, len(hosts))
	for _, host := range hosts {
		online := isHostAgentOnlineWithRuntime(host, statuses)
		if (status == "online" && online) || (status == "offline" && !online) {
			filtered = append(filtered, host)
		}
	}
	return filtered
}

func isHostAgentOnline(host model.Host) bool {
	return !host.LastHeartbeatAt.IsZero() && host.LastHeartbeatAt.After(time.Now().Add(-2*time.Minute))
}

func isHostAgentOnlineWithRuntime(host model.Host, statuses map[string]agentRuntimeStatus) bool {
	if status, ok := statuses[host.ID.String()]; ok {
		return status.Connected
	}
	return isHostAgentOnline(host)
}

type agentRuntimeStatus struct {
	Connected     bool
	LastHeartbeat int64
}

func loadAgentRuntimeStatuses(ctx context.Context, statusClient agentStatusClient) map[string]agentRuntimeStatus {
	if statusClient == nil {
		return nil
	}

	statusCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := statusClient.ListConnectedAgents(statusCtx)
	if err != nil || resp == nil {
		return nil
	}

	statuses := make(map[string]agentRuntimeStatus, len(resp.Agents))
	for _, agent := range resp.Agents {
		if agent == nil || agent.HostId == "" {
			continue
		}
		statuses[agent.HostId] = agentRuntimeStatus{
			Connected:     agent.Connected,
			LastHeartbeat: agent.LastHeartbeat,
		}
	}
	return statuses
}

func decorateHostsWithAgentStatus(hosts []model.Host, statuses map[string]agentRuntimeStatus) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(hosts))
	for _, host := range hosts {
		result = append(result, decorateHostWithAgentStatus(host, statuses))
	}
	return result
}

func decorateHostWithAgentStatus(host model.Host, statuses map[string]agentRuntimeStatus) map[string]interface{} {
	online := isHostAgentOnlineWithRuntime(host, statuses)
	agentStatus := "offline"
	if online {
		agentStatus = "online"
	}

	item := map[string]interface{}{
		"id":                 host.ID.String(),
		"ip_address":         host.IPAddress,
		"hostname":           host.Hostname,
		"os_type":            host.OSType,
		"agent_version":      host.AgentVersion,
		"last_heartbeat_at":  host.LastHeartbeatAt,
		"created_at":         host.CreatedAt,
		"updated_at":         host.UpdatedAt,
		"agent_status":       agentStatus,
		"agent_connected":    online,
		"runtime_available":  online,
		"status_source":      "database_heartbeat",
		"status_freshness_s": 120,
	}

	if runtimeStatus, ok := statuses[host.ID.String()]; ok {
		item["agent_connected"] = runtimeStatus.Connected
		item["runtime_available"] = runtimeStatus.Connected
		item["status_source"] = "server_connection"
		if runtimeStatus.LastHeartbeat > 0 {
			item["agent_last_heartbeat_unix"] = runtimeStatus.LastHeartbeat
		}
	}

	return item
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
