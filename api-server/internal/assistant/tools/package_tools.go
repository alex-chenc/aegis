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
		Name:        "Package.List",
		Domain:      "package",
		Operation:   "list",
		Description: "列出检测包，支持按状态和关键字筛选",
		RiskLevel:   "low",
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量"},
				"status":    map[string]interface{}{"type": "string", "description": "状态筛选（enabled/disabled/built/build_success/draft）"},
				"search":    map[string]interface{}{"type": "string", "description": "搜索关键字（包ID或标题）"},
			},
		},
		Handler: makePackageListHandler(deps.PackageRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:        "Package.Get",
		Domain:      "package",
		Operation:   "get",
		Description: "根据包ID获取检测包详情（最新版本）",
		RiskLevel:   "low",
		Enabled:     true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{"type": "string", "description": "检测包ID"},
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
