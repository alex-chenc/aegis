package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/recovery"
	"api-server/internal/service"
)

// DetectionPackageServiceForTools 检测包服务接口（写操作）
type DetectionPackageServiceForTools interface {
	StartBuild(ctx context.Context, packageID string, operator string) (*model.DetectionPackageBuild, error)
	GetLatestBuild(ctx context.Context, packageID string) (*model.DetectionPackageBuild, error)
	SignPackage(ctx context.Context, packageID string, operator string) (*model.DetectionPackage, error)
	EnablePackage(ctx context.Context, packageID string, operator string) error
}

type DetectionPackageGeneratorForTools interface {
	GenerateDraft(ctx context.Context, req service.GenerateDetectionPackageDraftRequest) (*model.DetectionPackageDraft, error)
}

type detectionPackageBuildFailedError struct {
	build *model.DetectionPackageBuild
}

func (e *detectionPackageBuildFailedError) Error() string {
	if e == nil || e.build == nil {
		return "detection package build failed"
	}
	if reason := strings.TrimSpace(e.build.ErrorMessage); reason != "" {
		return "detection package build failed: " + reason
	}
	return "detection package build failed"
}

func (e *detectionPackageBuildFailedError) RecoveryDescriptor() recovery.Descriptor {
	contextData := map[string]interface{}{}
	detail := "The detection package builder returned a terminal failure."
	if e != nil && e.build != nil {
		contextData = map[string]interface{}{
			"package_id":     e.build.PackageID,
			"build_id":       e.build.ID.String(),
			"build_status":   e.build.Status,
			"error_message":  boundedBuildDiagnostic(e.build.ErrorMessage, 2048),
			"build_log_tail": boundedBuildDiagnostic(e.build.BuildLog, 4096),
			"clang_version":  e.build.ClangVersion,
			"builder_image":  e.build.BuilderImage,
		}
		if reason := strings.TrimSpace(e.build.ErrorMessage); reason != "" {
			detail = reason
		}
	}
	return recovery.Descriptor{
		Code:      "detection_package_build_failed",
		Category:  recovery.CategoryRecoverableBusinessBlocker,
		Summary:   "动态检测包构建失败，需要选择后续处理方式。",
		Detail:    detail,
		RiskLevel: model.RiskMedium,
		Context:   contextData,
		Actions: []recovery.Action{{
			ID:          "regenerate_detection_package",
			Label:       "AI 修正草稿并重新构建",
			Description: "将真实构建错误交给智能体，重新生成草稿并执行构建。",
			RiskLevel:   model.RiskMedium,
			Executor:    assistant.RecoveryExecutorAssistantResume,
			ResumesRun:  true,
			RetrySafe:   true,
		}, {
			ID:        "pause",
			Label:     "暂停当前操作",
			RiskLevel: model.RiskReadonly,
		}, {
			ID:        "cancel",
			Label:     "取消当前操作",
			RiskLevel: model.RiskReadonly,
		}, {
			ID:            "provide_other",
			Label:         "提供其他处理说明",
			Description:   "将您的补充说明交给智能体，并重新执行原任务。",
			RiskLevel:     model.RiskReadonly,
			Executor:      assistant.RecoveryExecutorAssistantResume,
			ResumesRun:    true,
			InputRequired: true,
		}},
	}
}

func boundedBuildDiagnostic(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

// PackageWriteToolDeps 检测包写操作工具依赖
type PackageWriteToolDeps struct {
	PackageService         DetectionPackageServiceForTools
	PackageGenerator       DetectionPackageGeneratorForTools
	DraftGenerationTimeout time.Duration
}

// RegisterPackageWriteTools 注册检测包写操作工具
func RegisterPackageWriteTools(registry *assistant.ToolRegistry, deps PackageWriteToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Draft.Generate",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpGenerate,
		Capability:         "generate_detection_package_draft",
		Description:        "Use AI to generate a dynamic detection package draft, including HookPlan, eBPF, Sigma, and correlation rules, from a CVE and user-supplied vulnerability or exploitation details.",
		Aliases:            []string{"生成检测包", "生成动态检测包", "检测包草稿", "创建规则包", "动态包草稿"},
		Tags:               []string{"package", "detection_package", "dynamic", "draft", "sigma", "ebpf"},
		ObjectTypes:        []string{"detection_package", "package", "sigma_rule"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		DefaultTimeout:     detectionPackageDraftGenerationTimeout(deps.DraftGenerationTimeout),
		ResultContract: assistant.ToolResultContract{
			OperationStatusField: "status",
			SuccessValues:        []string{"draft"},
			OperationRefFields:   []string{"package_id"},
			ArtifactRefFields:    []string{"id", "package_id"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cve_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact CVE ID to detect.",
				},
				"vulnerability_description": map[string]interface{}{
					"type":        "string",
					"description": "Technical vulnerability and exploitation description supplied by the user.",
				},
				"attack_prerequisites": map[string]interface{}{
					"type":        "string",
					"description": "Required local state, privileges, kernel features, or other exploitation prerequisites.",
				},
				"exploitation_chain": map[string]interface{}{
					"type":        "string",
					"description": "Ordered exploitation actions and relevant syscalls or kernel subsystems.",
				},
				"false_positive_constraints": map[string]interface{}{
					"type":        "string",
					"description": "Known benign patterns and constraints used to reduce false positives.",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "Operator name for the audit record.",
				},
			},
			"required":             []string{"cve_id", "vulnerability_description"},
			"additionalProperties": false,
		},
		Handler: makePackageDraftGenerateHandler(deps.PackageGenerator),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Build.Start",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpExecute,
		Capability:         "start_detection_package_build",
		Description:        "Start a dynamic detection package build.",
		Aliases:            []string{"构建检测包", "启动构建", "编译动态检测包", "检测包构建"},
		Tags:               []string{"package", "detection_package", "dynamic", "build", "task"},
		ObjectTypes:        []string{"detection_package", "package", "task"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ExecutionContract: assistant.ToolExecutionContract{
			Mode:                 assistant.ToolExecutionAsynchronous,
			CompletionCapability: "get_detection_package_build_status",
		},
		ResultContract: assistant.ToolResultContract{
			AcceptedOnSuccess:  true,
			OperationRefFields: []string{"id", "package_id"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact detection package ID.",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "Operator name for the audit record.",
				},
			},
			"required": []string{"package_id"},
		},
		Handler: makePackageBuildStartHandler(deps.PackageService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Build.Status",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpGet,
		Capability:         "get_detection_package_build_status",
		Description:        "Get the latest dynamic detection package build status.",
		ModelDescription:   "Poll the current or terminal state of an asynchronous detection package build.",
		Aliases:            []string{"检测包构建状态", "检测包构建进度", "动态检测包构建结果"},
		Tags:               []string{"package", "detection_package", "build", "status", "progress"},
		ObjectTypes:        []string{"detection_package", "package", "build"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ResultContract: assistant.ToolResultContract{
			OperationStatusField:  "status",
			SuccessValues:         []string{"success", "awaiting_review"},
			PendingValues:         []string{"pending", "running"},
			FailureValues:         []string{"failed", "review_rejected"},
			OperationRefFields:    []string{"id", "package_id"},
			SatisfiesCapabilities: []string{"start_detection_package_build"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact detection package ID.",
				},
			},
			"required": []string{"package_id"},
		},
		Handler: makePackageBuildStatusHandler(deps.PackageService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Package.Sign",
		Domain:             assistant.DomainPackage,
		Operation:          assistant.OpApprove,
		Capability:         "sign_detection_package",
		Description:        "Sign a dynamic detection package after approval.",
		Aliases:            []string{"签名检测包", "检测包签名", "批准检测包", "发布签名"},
		Tags:               []string{"package", "detection_package", "dynamic", "sign", "critical"},
		ObjectTypes:        []string{"detection_package", "package"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskCritical,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ResultContract: assistant.ToolResultContract{
			OperationStatusField: "status",
			SuccessValues:        []string{"signed"},
			OperationRefFields:   []string{"id", "package_id"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact detection package ID.",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "Operator name for the audit record.",
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
		Description:        "Enable a dynamic detection package and distribute it to agents after approval.",
		Aliases:            []string{"启用检测包", "分发检测包", "下发动态检测包", "启用规则包", "发布检测包"},
		Tags:               []string{"package", "detection_package", "dynamic", "enable", "dispatch", "critical"},
		ObjectTypes:        []string{"detection_package", "package", "agent"},
		PageRoutes:         []string{"/detection/packages", "/packages", "/detection"},
		Risk:               assistant.ToolRiskCritical,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ResultContract: assistant.ToolResultContract{
			OperationStatusField: "status",
			SuccessValues:        []string{"enabled"},
			OperationRefFields:   []string{"package_id"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact detection package ID.",
				},
				"operator": map[string]interface{}{
					"type":        "string",
					"description": "Operator name for the audit record.",
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

func detectionPackageDraftGenerationTimeout(configured time.Duration) time.Duration {
	const minimum = 120 * time.Second
	if configured < minimum {
		return minimum
	}
	return configured
}

func makePackageDraftGenerateHandler(generator DetectionPackageGeneratorForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if generator == nil {
			return nil, fmt.Errorf("detection package generator is unavailable")
		}
		cveID := getStringArg(args, "cve_id", "")
		if cveID == "" {
			return nil, fmt.Errorf("cve_id is required")
		}
		description := getStringArg(args, "vulnerability_description", "")
		if description == "" {
			return nil, fmt.Errorf("vulnerability_description is required")
		}
		result, err := generator.GenerateDraft(ctx, service.GenerateDetectionPackageDraftRequest{
			CVEID:                    cveID,
			VulnerabilityDescription: description,
			AttackPrerequisites:      getStringArg(args, "attack_prerequisites", ""),
			ExploitationChain:        getStringArg(args, "exploitation_chain", ""),
			FalsePositiveConstraints: getStringArg(args, "false_positive_constraints", ""),
			Operator:                 getStringArg(args, "operator", "assistant"),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate detection package draft: %w", err)
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

func makePackageBuildStatusHandler(svc DetectionPackageServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		packageID := getStringArg(args, "package_id", "")
		if packageID == "" {
			return nil, fmt.Errorf("package_id is required")
		}
		result, err := svc.GetLatestBuild(ctx, packageID)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest build: %w", err)
		}
		if result != nil && strings.EqualFold(strings.TrimSpace(result.Status), "failed") {
			return nil, fmt.Errorf("failed to build detection package: %w", &detectionPackageBuildFailedError{build: result})
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
