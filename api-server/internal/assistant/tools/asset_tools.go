package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/repository"
)

type AssetCollectionServiceForTools interface {
	TriggerAssetCollection(ctx context.Context, req model.TriggerAssetCollectionRequest, requestedBy string) (*model.AssetCollectionTask, error)
}

type AssetQueryServiceForTools interface {
	GetSummary() (*model.AssetSummary, error)
	ListSoftwareAssets(query model.SoftwareAssetQuery) ([]model.HostSoftwareAsset, int64, error)
	ListApplicationAssets(query model.ApplicationAssetQuery) ([]model.HostApplicationAsset, int64, error)
}

// AssetToolDeps 资产工具依赖。
type AssetToolDeps struct {
	CollectionService AssetCollectionServiceForTools
	QueryService      AssetQueryServiceForTools
	AssetRepo         *repository.AssetCollectionRepository
}

// RegisterAssetTools 注册资产域工具。
func RegisterAssetTools(registry *assistant.ToolRegistry, deps AssetToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Asset.Summary.Get",
		Domain:             assistant.DomainAsset,
		Operation:          assistant.OpGet,
		Capability:         "asset_summary",
		Description:        "Get asset inventory totals for software, applications, databases, web services, AI agents, LLM services, MCP servers, and pending reviews.",
		Aliases:            []string{"资产概览", "资产统计", "资源统计", "资产态势", "AI资产统计"},
		Tags:               []string{"asset", "summary", "software", "application", "ai_agent", "llm_service", "mcp_server"},
		ObjectTypes:        []string{"asset"},
		PageRoutes:         []string{"/hosts/assets", "/assets", "/hosts"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: makeAssetSummaryHandler(deps.QueryService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Asset.Software.List",
		Domain:             assistant.DomainAsset,
		Operation:          assistant.OpList,
		Capability:         "list_software_assets",
		Description:        "List software assets with keyword, host, operating-system, package-manager, and status filters.",
		Aliases:            []string{"软件资产", "软件清单", "已安装软件", "主机软件"},
		Tags:               []string{"asset", "software", "host"},
		ObjectTypes:        []string{"asset", "software", "host"},
		PageRoutes:         []string{"/hosts/assets", "/assets", "/hosts"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":            map[string]interface{}{"type": "integer", "description": "One-based page number; defaults to 1."},
				"page_size":       map[string]interface{}{"type": "integer", "description": "Items per page; defaults to 20."},
				"keyword":         map[string]interface{}{"type": "string", "description": "Software name or version keyword."},
				"host_id":         map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact host UUID."},
				"os_type":         map[string]interface{}{"type": "string", "description": "Operating-system type filter."},
				"package_manager": map[string]interface{}{"type": "string", "description": "Package-manager filter."},
				"status":          map[string]interface{}{"type": "string", "description": "Asset status filter."},
			},
		},
		Handler: makeAssetSoftwareListHandler(deps.QueryService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Asset.Application.List",
		Domain:             assistant.DomainAsset,
		Operation:          assistant.OpList,
		Capability:         "list_application_assets",
		Description:        "List application and AI assets with keyword, host, category, confidence, review-status, and asset-status filters.",
		Aliases:            []string{"应用资产", "应用清单", "服务资产", "数据库资产", "Web服务资产", "AI资产", "AI Agent资产", "LLM服务", "MCP Server"},
		Tags:               []string{"asset", "application", "database", "web", "ai_agent", "llm_service", "mcp_server"},
		ObjectTypes:        []string{"asset", "application", "host"},
		PageRoutes:         []string{"/hosts/assets", "/assets", "/hosts"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":           map[string]interface{}{"type": "integer", "description": "One-based page number; defaults to 1."},
				"page_size":      map[string]interface{}{"type": "integer", "description": "Items per page; defaults to 20."},
				"keyword":        map[string]interface{}{"type": "string", "description": "Application name or path keyword."},
				"host_id":        map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact host UUID."},
				"category":       map[string]interface{}{"type": "string", "description": "Application category such as database, web_service, web_framework, web_site, llm_service, ai_agent, or mcp_server."},
				"min_confidence": map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1, "description": "Minimum AI classification confidence."},
				"review_status":  map[string]interface{}{"type": "string", "description": "Review status filter."},
				"status":         map[string]interface{}{"type": "string", "description": "Asset status filter."},
			},
		},
		Handler: makeAssetApplicationListHandler(deps.QueryService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Asset.Collection.Trigger",
		Domain:             assistant.DomainAsset,
		Operation:          assistant.OpExecute,
		Capability:         "trigger_asset_collection",
		Description:        "Trigger process, application, and AI asset collection through the operations service and return a progress reference.",
		Aliases:            []string{"资产采集", "资源采集", "资产清点", "实时采集", "重新采集资产", "AI资产采集", "采集AI Agent", "采集MCP"},
		Tags:               []string{"asset", "collection", "task", "operation", "ai_agent", "llm_service", "mcp_server"},
		ObjectTypes:        []string{"asset", "host"},
		PageRoutes:         []string{"/hosts/assets", "/assets", "/hosts"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ExecutionContract: assistant.ToolExecutionContract{
			Mode:                 assistant.ToolExecutionAsynchronous,
			CompletionCapability: "get_asset_collection_task",
		},
		ResultContract: assistant.ToolResultContract{
			// Trigger creates a background collection task and returns task_id.
			// Declaring AcceptedOnSuccess keeps the step non-terminal so the
			// mapped completion tool (Asset.Collection.Get) polls the real
			// task_id until a terminal status is reached.
			AcceptedOnSuccess:  true,
			OperationRefFields: []string{"task_id"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"scope": map[string]interface{}{
					"type":        "string",
					"description": "Collection scope: all_hosts or hosts; inferred from host_ids when omitted.",
				},
				"host_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Exact host UUIDs when scope is hosts.",
				},
				"types": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Collection types; defaults to process and application_analysis.",
				},
				"force": map[string]interface{}{"type": "boolean", "description": "Force a new collection even when recent inventory exists."},
			},
		},
		Handler: makeAssetCollectionTriggerHandler(deps.CollectionService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Asset.Collection.List",
		Domain:             assistant.DomainAsset,
		Operation:          assistant.OpList,
		Capability:         "list_asset_collection_tasks",
		Description:        "List asset collection tasks with an optional status filter.",
		Aliases:            []string{"资产采集任务", "采集进度列表", "资源采集任务"},
		Tags:               []string{"asset", "collection", "task"},
		ObjectTypes:        []string{"asset_collection"},
		PageRoutes:         []string{"/hosts/assets", "/assets", "/hosts"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "One-based page number; defaults to 1."},
				"page_size": map[string]interface{}{"type": "integer", "description": "Items per page; defaults to 20."},
				"status":    map[string]interface{}{"type": "string", "description": "Collection task status filter."},
			},
		},
		Handler: makeAssetCollectionListHandler(deps.AssetRepo),
	}); err != nil {
		return err
	}

	return registry.Register(&assistant.ToolSpec{
		Name:               "Asset.Collection.Get",
		Domain:             assistant.DomainAsset,
		Operation:          assistant.OpGet,
		Capability:         "get_asset_collection_task",
		Description:        "Get one asset collection task and its per-host progress.",
		Aliases:            []string{"资产采集详情", "采集任务详情", "采集进度"},
		Tags:               []string{"asset", "collection", "task", "progress"},
		ObjectTypes:        []string{"asset_collection"},
		PageRoutes:         []string{"/hosts/assets", "/assets", "/hosts"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ResultContract: assistant.ToolResultContract{
			OperationStatusField:  "status",
			PendingValues:         []string{"pending", "collecting", "analyzing"},
			SuccessValues:         []string{"completed"},
			FailureValues:         []string{"failed", "cancelled"},
			OperationRefFields:    []string{"task_id"},
			SatisfiesCapabilities: []string{"trigger_asset_collection"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact asset collection task UUID."},
			},
			"required": []string{"task_id"},
		},
		Handler: makeAssetCollectionGetHandler(deps.AssetRepo),
	})
}

func makeAssetSummaryHandler(svc AssetQueryServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if svc == nil {
			return nil, fmt.Errorf("asset query service not configured")
		}
		summary, err := svc.GetSummary()
		if err != nil {
			return nil, fmt.Errorf("failed to get asset summary: %w", err)
		}
		return map[string]interface{}{
			"summary":    summary,
			"route_path": "/hosts/assets",
		}, nil
	}
}

func makeAssetSoftwareListHandler(svc AssetQueryServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if svc == nil {
			return nil, fmt.Errorf("asset query service not configured")
		}
		query := model.SoftwareAssetQuery{
			Page:           getIntArg(args, "page", 1),
			PageSize:       getIntArg(args, "page_size", 20),
			Keyword:        getStringArg(args, "keyword", ""),
			HostID:         getStringArg(args, "host_id", ""),
			OSType:         getStringArg(args, "os_type", ""),
			PackageManager: getStringArg(args, "package_manager", ""),
			Status:         getStringArg(args, "status", ""),
		}
		items, total, err := svc.ListSoftwareAssets(query)
		if err != nil {
			return nil, fmt.Errorf("failed to list software assets: %w", err)
		}
		return map[string]interface{}{
			"data":       items,
			"total":      total,
			"route_path": "/hosts/assets",
		}, nil
	}
}

func makeAssetApplicationListHandler(svc AssetQueryServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if svc == nil {
			return nil, fmt.Errorf("asset query service not configured")
		}
		query := model.ApplicationAssetQuery{
			Page:          getIntArg(args, "page", 1),
			PageSize:      getIntArg(args, "page_size", 20),
			Keyword:       getStringArg(args, "keyword", ""),
			HostID:        getStringArg(args, "host_id", ""),
			Category:      getStringArg(args, "category", ""),
			MinConfidence: getFloatArg(args, "min_confidence", 0),
			ReviewStatus:  getStringArg(args, "review_status", ""),
			Status:        getStringArg(args, "status", ""),
		}
		items, total, err := svc.ListApplicationAssets(query)
		if err != nil {
			return nil, fmt.Errorf("failed to list application assets: %w", err)
		}
		return map[string]interface{}{
			"data":       items,
			"total":      total,
			"route_path": "/hosts/assets",
		}, nil
	}
}

func makeAssetCollectionTriggerHandler(svc AssetCollectionServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if svc == nil {
			return nil, fmt.Errorf("asset collection service not configured")
		}
		hostIDs, _ := getStringSliceArg(args, "host_ids")
		types, _ := getStringSliceArg(args, "types")
		if len(types) == 0 {
			types = []string{"process", "application_analysis"}
		}
		scope := getStringArg(args, "scope", "")
		if scope == "" {
			if len(hostIDs) > 0 {
				scope = "hosts"
			} else {
				scope = "all_hosts"
			}
		}

		task, err := svc.TriggerAssetCollection(ctx, model.TriggerAssetCollectionRequest{
			Scope:   scope,
			HostIDs: hostIDs,
			Types:   types,
			Force:   getBoolArg(args, "force", false),
		}, "assistant")
		if err != nil {
			return nil, fmt.Errorf("failed to trigger asset collection: %w", err)
		}

		taskID := task.ID.String()
		return map[string]interface{}{
			"task_kind": "asset_collection",
			"task_id":   taskID,
			"status":    task.Status,
			"progress": map[string]interface{}{
				"total_hosts":   task.TotalHosts,
				"success_hosts": task.SuccessHosts,
				"failed_hosts":  task.FailedHosts,
				"current_stage": task.CurrentStage,
			},
			"task_ref": buildTaskRef(
				"asset_collection",
				taskID,
				"",
				"/api/v1/host-assets/collections/"+taskID,
				"/hosts/assets",
			),
		}, nil
	}
}

func makeAssetCollectionListHandler(repo *repository.AssetCollectionRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if repo == nil {
			return nil, fmt.Errorf("asset collection repository not configured")
		}
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)
		status := getStringArg(args, "status", "")
		tasks, total, err := repo.ListTasks(page, pageSize, status)
		if err != nil {
			return nil, fmt.Errorf("failed to list asset collection tasks: %w", err)
		}
		return map[string]interface{}{
			"data":       tasks,
			"total":      total,
			"route_path": "/hosts/assets",
		}, nil
	}
}

func makeAssetCollectionGetHandler(repo *repository.AssetCollectionRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		_ = ctx
		if repo == nil {
			return nil, fmt.Errorf("asset collection repository not configured")
		}
		taskID, err := parseUUID(args, "task_id")
		if err != nil {
			return nil, err
		}
		task, err := repo.GetTask(taskID)
		if err != nil {
			return nil, fmt.Errorf("failed to get asset collection task: %w", err)
		}
		hosts, err := repo.GetTaskHosts(taskID)
		if err != nil {
			return nil, fmt.Errorf("failed to get asset collection task hosts: %w", err)
		}
		id := task.ID.String()
		// Stable top-level fields let the declarative ResultContract resolve
		// terminal status and task_id without coupling to nested model shapes.
		return map[string]interface{}{
			"task_id": id,
			"status":  task.Status,
			"progress": map[string]interface{}{
				"total_hosts":   task.TotalHosts,
				"success_hosts": task.SuccessHosts,
				"failed_hosts":  task.FailedHosts,
				"current_stage": task.CurrentStage,
			},
			"task":  task,
			"hosts": hosts,
			"task_ref": buildTaskRef(
				"asset_collection",
				id,
				"",
				"/api/v1/host-assets/collections/"+id,
				"/hosts/assets",
			),
		}, nil
	}
}
