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
		Description:        "查询资产概览，包括软件、应用、数据库、Web服务和待复核资产数量",
		Aliases:            []string{"资产概览", "资产统计", "资源统计", "资产态势"},
		Tags:               []string{"asset", "summary", "software", "application"},
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
		Description:        "查询软件资产列表，支持关键字、主机、操作系统、包管理器和状态筛选",
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
				"page":            map[string]interface{}{"type": "integer", "description": "页码，默认1"},
				"page_size":       map[string]interface{}{"type": "integer", "description": "每页数量，默认20"},
				"keyword":         map[string]interface{}{"type": "string", "description": "软件名称或版本关键字"},
				"host_id":         map[string]interface{}{"type": "string", "description": "主机ID"},
				"os_type":         map[string]interface{}{"type": "string", "description": "操作系统类型"},
				"package_manager": map[string]interface{}{"type": "string", "description": "包管理器"},
				"status":          map[string]interface{}{"type": "string", "description": "资产状态"},
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
		Description:        "查询应用资产列表，支持关键字、主机、分类、置信度、复核状态和状态筛选",
		Aliases:            []string{"应用资产", "应用清单", "服务资产", "数据库资产", "Web服务资产"},
		Tags:               []string{"asset", "application", "database", "web"},
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
				"page":           map[string]interface{}{"type": "integer", "description": "页码，默认1"},
				"page_size":      map[string]interface{}{"type": "integer", "description": "每页数量，默认20"},
				"keyword":        map[string]interface{}{"type": "string", "description": "应用名称或路径关键字"},
				"host_id":        map[string]interface{}{"type": "string", "description": "主机ID"},
				"category":       map[string]interface{}{"type": "string", "description": "应用分类"},
				"min_confidence": map[string]interface{}{"type": "number", "description": "最小AI置信度"},
				"review_status":  map[string]interface{}{"type": "string", "description": "复核状态"},
				"status":         map[string]interface{}{"type": "string", "description": "资产状态"},
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
		Description:        "触发资产采集任务，复用运维模式的资产采集服务并返回采集进度引用",
		Aliases:            []string{"资产采集", "资源采集", "资产清点", "实时采集", "重新采集资产"},
		Tags:               []string{"asset", "collection", "task", "operation"},
		ObjectTypes:        []string{"asset", "host"},
		PageRoutes:         []string{"/hosts/assets", "/assets", "/hosts"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"scope": map[string]interface{}{
					"type":        "string",
					"description": "采集范围：all_hosts 或 hosts；未提供时根据 host_ids 自动判断",
				},
				"host_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "指定主机ID列表",
				},
				"types": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "采集类型，默认 process 和 application_analysis",
				},
				"force": map[string]interface{}{"type": "boolean", "description": "是否强制采集"},
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
		Description:        "查询资产采集任务列表，支持按状态筛选",
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
				"page":      map[string]interface{}{"type": "integer", "description": "页码，默认1"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量，默认20"},
				"status":    map[string]interface{}{"type": "string", "description": "采集任务状态"},
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
		Description:        "查询单个资产采集任务详情和主机级采集进度",
		Aliases:            []string{"资产采集详情", "采集任务详情", "采集进度"},
		Tags:               []string{"asset", "collection", "task", "progress"},
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
				"task_id": map[string]interface{}{"type": "string", "description": "资产采集任务ID"},
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
		return map[string]interface{}{
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
