package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/repository"
)

// PackageToolDeps 检测包工具依赖
type PackageToolDeps struct {
	PackageRepo *repository.DetectionPackageRepo
}

// RegisterPackageTools 注册检测包域工具
func RegisterPackageTools(registry *assistant.ToolRegistry, deps PackageToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.List",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpList,
		Capability:         "list_detection_packages",
		Description:        "List dynamic detection packages with status and keyword filters.",
		Aliases:            []string{"动态检测包", "检测包列表", "规则包", "eBPF 检测包", "检测包状态"},
		Tags:               []string{"package", "detection_package", "dynamic", "ebpf", "sigma"},
		ObjectTypes:        []string{"detection_package", "package", "sigma_rule"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "One-based page number."},
				"page_size": map[string]interface{}{"type": "integer", "description": "Items per page."},
				"status":    map[string]interface{}{"type": "string", "description": "Status filter: enabled, disabled, built, build_success, or draft."},
				"search":    map[string]interface{}{"type": "string", "description": "Package ID or title keyword."},
			},
		},
		Handler: makePackageListHandler(deps.PackageRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Get",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpGet,
		Capability:         "get_detection_package",
		Description:        "Get the latest version of a dynamic detection package by package ID.",
		Aliases:            []string{"检测包详情", "动态检测包详情", "规则包详情", "包版本"},
		Tags:               []string{"package", "detection_package", "dynamic", "version"},
		ObjectTypes:        []string{"detection_package", "package", "sigma_rule"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{"type": "string", "description": "Exact detection package ID."},
			},
			"required": []string{"package_id"},
		},
		Handler: makePackageGetHandler(deps.PackageRepo),
	}); err != nil {
		return err
	}

	return nil
}

func makePackageListHandler(repo *repository.DetectionPackageRepo) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)
		status := getStringArg(args, "status", "")
		search := getStringArg(args, "search", "")

		packages, total, err := repo.ListPackages(page, pageSize, status, search)
		if err != nil {
			return nil, fmt.Errorf("failed to list packages: %w", err)
		}

		return map[string]interface{}{
			"data":  packages,
			"total": total,
		}, nil
	}
}

func makePackageGetHandler(repo *repository.DetectionPackageRepo) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		packageID := getStringArg(args, "package_id", "")
		if packageID == "" {
			return nil, fmt.Errorf("package_id is required")
		}

		pkg, err := repo.GetLatestPackage(packageID)
		if err != nil {
			return nil, fmt.Errorf("failed to get package: %w", err)
		}

		return pkg, nil
	}
}
