package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"
)

// DetectionPackageServiceForTools 检测包服务接口（写操作）
type DetectionPackageServiceForTools interface {
	CreateDraft(ctx context.Context, req service.CreateDraftRequest, operator string) (*model.DetectionPackageDraft, error)
	StartBuild(ctx context.Context, packageID string, operator string) (*model.DetectionPackageBuild, error)
	SignPackage(ctx context.Context, packageID string, operator string) (*model.DetectionPackage, error)
	EnablePackage(ctx context.Context, packageID string, operator string) error
}

// PackageWriteToolDeps 检测包写操作工具依赖
type PackageWriteToolDeps struct {
	PackageService DetectionPackageServiceForTools
}

// RegisterPackageWriteTools 注册检测包写操作工具
func RegisterPackageWriteTools(registry *assistant.ToolRegistry, deps PackageWriteToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Draft.Generate",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpGenerate,
		Capability:         "generate_detection_package_draft",
		Description:        "根据 Hook 计划和 Sigma 规则生成动态检测包草稿",
		Aliases:            []string{"生成检测包", "生成动态检测包", "检测包草稿", "创建规则包", "动态包草稿"},
		Tags:               []string{"package", "detection_package", "dynamic", "draft", "sigma", "ebpf"},
		ObjectTypes:        []string{"detection_package", "package", "sigma_rule"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type":        "string",
					"description": "检测包标题",
				},
				"target_version": map[string]interface{}{
					"type":        "string",
					"description": "目标版本号",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "检测包描述",
				},
				"cve_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "关联的CVE ID列表",
				},
				"hook_plan_yaml": map[string]interface{}{
					"type":        "string",
					"description": "eBPF Hook计划YAML",
				},
				"sigma_rules_yaml": map[string]interface{}{
					"type":        "string",
					"description": "Sigma规则YAML",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "操作者名称",
				},
			},
			"required": []string{"title", "target_version"},
		},
		Handler: makePackageDraftGenerateHandler(deps.PackageService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Build.Start",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpExecute,
		Capability:         "start_detection_package_build",
		Description:        "启动动态检测包构建任务",
		Aliases:            []string{"构建检测包", "启动构建", "编译动态检测包", "检测包构建"},
		Tags:               []string{"package", "detection_package", "dynamic", "build", "task"},
		ObjectTypes:        []string{"detection_package", "package", "task"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{
					"type":        "string",
					"description": "检测包ID",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "操作者名称",
				},
			},
			"required": []string{"package_id"},
		},
		Handler: makePackageBuildStartHandler(deps.PackageService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Sign",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpApprove,
		Capability:         "sign_detection_package",
		Description:        "签名动态检测包（高风险操作，需审批）",
		Aliases:            []string{"签名检测包", "检测包签名", "批准检测包", "发布签名"},
		Tags:               []string{"package", "detection_package", "dynamic", "sign", "critical"},
		ObjectTypes:        []string{"detection_package", "package"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskCritical,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{
					"type":        "string",
					"description": "检测包ID",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "操作者名称",
				},
			},
			"required": []string{"package_id"},
		},
		Handler: makePackageSignHandler(deps.PackageService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Enable",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpDispatch,
		Capability:         "enable_detection_package",
		Description:        "启用动态检测包并分发到 Agent（高风险操作，需审批）",
		Aliases:            []string{"启用检测包", "分发检测包", "下发动态检测包", "启用规则包", "发布检测包"},
		Tags:               []string{"package", "detection_package", "dynamic", "enable", "dispatch", "critical"},
		ObjectTypes:        []string{"detection_package", "package", "agent"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskCritical,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{
					"type":        "string",
					"description": "检测包ID",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "操作者名称",
				},
			},
			"required": []string{"package_id"},
		},
		Handler: makePackageEnableHandler(deps.PackageService),
	}); err != nil {
		return err
	}

	return nil
}

func makePackageDraftGenerateHandler(svc DetectionPackageServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		title := getStringArg(args, "title", "")
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		targetVersion := getStringArg(args, "target_version", "")
		if targetVersion == "" {
			return nil, fmt.Errorf("target_version is required")
		}
		operator := getStringArg(args, "operator", "assistant")

		req := service.CreateDraftRequest{
			Title:          title,
			TargetVersion:  targetVersion,
			Description:    getStringArg(args, "description", ""),
			HookPlanYAML:   getStringArg(args, "hook_plan_yaml", ""),
			SigmaRulesYAML: getStringArg(args, "sigma_rules_yaml", ""),
		}

		if cveIDs, err := getStringSliceArg(args, "cve_ids"); err == nil {
			req.CVEIDs = cveIDs
		}

		result, err := svc.CreateDraft(ctx, req, operator)
		if err != nil {
			return nil, fmt.Errorf("failed to create draft: %w", err)
		}

		return result, nil
	}
}

func makePackageBuildStartHandler(svc DetectionPackageServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		packageID := getStringArg(args, "package_id", "")
		if packageID == "" {
			return nil, fmt.Errorf("package_id is required")
		}
		operator := getStringArg(args, "operator", "assistant")

		result, err := svc.StartBuild(ctx, packageID, operator)
		if err != nil {
			return nil, fmt.Errorf("failed to start build: %w", err)
		}

		return result, nil
	}
}

func makePackageSignHandler(svc DetectionPackageServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		packageID := getStringArg(args, "package_id", "")
		if packageID == "" {
			return nil, fmt.Errorf("package_id is required")
		}
		operator := getStringArg(args, "operator", "assistant")

		result, err := svc.SignPackage(ctx, packageID, operator)
		if err != nil {
			return nil, fmt.Errorf("failed to sign package: %w", err)
		}

		return result, nil
	}
}

func makePackageEnableHandler(svc DetectionPackageServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		packageID := getStringArg(args, "package_id", "")
		if packageID == "" {
			return nil, fmt.Errorf("package_id is required")
		}
		operator := getStringArg(args, "operator", "assistant")

		if err := svc.EnablePackage(ctx, packageID, operator); err != nil {
			return nil, fmt.Errorf("failed to enable package: %w", err)
		}

		return map[string]interface{}{
			"package_id": packageID,
			"status":     "enabled",
		}, nil
	}
}
