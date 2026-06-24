package tools

import (
	"context"
	"fmt"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"

	"github.com/google/uuid"
)

type WeakPasswordServiceForTools interface {
	AnalyzeAssetApplications(ctx context.Context, req model.AnalyzeAssetApplicationsRequest, createdBy *uuid.UUID) (*service.AnalyzeAssetApplicationsResponse, error)
	CreateTaskByApplication(ctx context.Context, req model.CreateTaskByApplicationRequest, createdBy *uuid.UUID) (*service.CreateTaskByApplicationResponse, error)
	GenerateAIDictionary(ctx context.Context, req model.AIGenerateDictionaryRequest, createdBy *uuid.UUID) (*service.DictionarySummary, error)
	GetTaskProgress(taskID uuid.UUID) (*model.TaskProgressResponse, error)
	ListTaskCollectionProgress(taskID uuid.UUID, page, pageSize int) ([]service.WeakPasswordCollectionProgressDTO, int64, error)
	ListTaskFindings(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordFinding, int64, error)
}

type WeakPasswordToolDeps struct {
	Service WeakPasswordServiceForTools
}

func RegisterWeakPasswordTools(registry *assistant.ToolRegistry, deps WeakPasswordToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.GenerateDictionary",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGenerate,
		Capability:         "weak_password_dictionary_generation",
		Description:        "根据自然语言调用 AI 生成弱密码字典并保存，复用弱密码模块的 AI 字典生成接口",
		Aliases:            []string{"AI生成弱密码", "生成弱密码字典", "弱密码字典生成", "AI 一键生成字典"},
		Tags:               []string{"weak_password", "credential", "dictionary", "ai", "v6.1"},
		ObjectTypes:        []string{"dictionary"},
		PageRoutes:         []string{"/risk/weak-password/dictionaries"},
		Risk:               assistant.ToolRiskLow,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         false,
		DefaultWhitelisted: true,
		Enabled:            true,
		DefaultTimeout:     120 * time.Second,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "GenerateAIDictionary",
			Notes:     "只调用 LLM 生成并保存字典，不使用随机硬编码兜底。",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"natural_language": map[string]interface{}{"type": "string", "description": "生成弱密码字典的自然语言要求"},
				"count":            map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20},
				"application_type": map[string]interface{}{"type": "string"},
				"organization_keywords": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"account_keywords": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"deduplicate_with_default": map[string]interface{}{"type": "boolean", "default": true},
			},
			"required": []string{"natural_language"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			req := model.AIGenerateDictionaryRequest{
				NaturalLanguage:        getStringArg(args, "natural_language", ""),
				ApplicationType:        getStringArg(args, "application_type", ""),
				Count:                  getIntArg(args, "count", 20),
				DeduplicateWithDefault: getBoolArg(args, "deduplicate_with_default", true),
			}
			if req.NaturalLanguage == "" {
				return nil, fmt.Errorf("natural_language is required")
			}
			req.OrganizationKeywords = getOptionalStringSliceArg(args, "organization_keywords")
			req.AccountKeywords = getOptionalStringSliceArg(args, "account_keywords")
			summary, err := deps.Service.GenerateAIDictionary(ctx, req, nil)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"dictionary":  summary,
				"next_action": "字典已保存；可在弱密码字典管理页面查看，也可继续调用 Credential.WeakPassword.AnalyzeApplications 分析应用后发起检测。",
				"route_path":  "/risk/weak-password/dictionaries",
			}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.AnalyzeApplications",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGenerate,
		Capability:         "weak_password_asset_analysis",
		Description:        "分析在线主机应用资产，筛选市面可见且需要密码认证的应用，并在同一主机上按应用类型去重",
		Aliases:            []string{"弱密码应用分析", "分析弱密码资产", "应用资产分析", "弱口令应用识别"},
		Tags:               []string{"weak_password", "asset", "application", "ai", "v6.1"},
		ObjectTypes:        []string{"candidate_application", "host_application_asset"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskLow,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         false,
		DefaultWhitelisted: true,
		Enabled:            true,
		DefaultTimeout:     60 * time.Second,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "AnalyzeAssetApplications",
			Notes:     "服务端强制 online_agents_only=true，并按 host/application_type 去重。",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"application_types": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"online_agents_only": map[string]interface{}{"type": "boolean", "default": true},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			req := model.AnalyzeAssetApplicationsRequest{}
			req.Scope.HostIDs = getOptionalStringSliceArg(args, "host_ids")
			req.Scope.ApplicationTypes = getOptionalStringSliceArg(args, "application_types")
			req.Scope.OnlineAgentsOnly = getBoolArg(args, "online_agents_only", true)
			resp, err := deps.Service.AnalyzeAssetApplications(ctx, req, nil)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"analysis":    resp,
				"next_action": "选择候选应用 candidate_application_id 后，可调用 Credential.WeakPassword.Scan 创建检测任务；任务进度用 Credential.WeakPassword.QueryProgress 查询。",
				"route_path":  "/risk/weak-password",
			}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.Scan",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpExecute,
		Capability:         "weak_password_scan",
		Description:        "针对候选应用创建弱密码检查任务，复用 V6.1 弱密码检测编排链路",
		Aliases:            []string{"弱密码检查", "弱口令扫描", "检查弱密码"},
		Tags:               []string{"weak_password", "credential", "v6.1"},
		ObjectTypes:        []string{"candidate_application", "task"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		RequiresApproval:   true,
		Idempotent:         false,
		DefaultWhitelisted: false,
		Enabled:            true,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "CreateTaskByApplication",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"candidate_application_id": map[string]interface{}{"type": "string"},
			},
			"required": []string{"candidate_application_id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			req := model.CreateTaskByApplicationRequest{CandidateApplicationID: getStringArg(args, "candidate_application_id", "")}
			req.DictionaryPolicy.UseDefault1000 = true
			req.AIPolicy.RepairCollectionErrors = true
			req.AIPolicy.DetectionRounds = 10
			req.AIPolicy.MaxAgentToolCallsPerApp = 10
			return deps.Service.CreateTaskByApplication(ctx, req, nil)
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.QueryFindings",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpList,
		Capability:         "weak_password_findings",
		Description:        "查询弱密码检测任务的命中结果，默认只返回脱敏密码",
		Aliases:            []string{"弱密码结果", "弱口令命中"},
		Tags:               []string{"weak_password", "finding", "v6.1"},
		ObjectTypes:        []string{"finding", "task"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "ListTaskFindings",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{"type": "string"},
			},
			"required": []string{"task_id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			_ = ctx
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			taskID, err := uuid.Parse(getStringArg(args, "task_id", ""))
			if err != nil {
				return nil, fmt.Errorf("invalid task_id: %w", err)
			}
			findings, total, err := deps.Service.ListTaskFindings(taskID, 1, 20)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"items": findings, "total": total}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.QueryProgress",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGet,
		Capability:         "weak_password_progress",
		Description:        "查询弱密码检测任务总体进度和采集进度，包含 Agent 工具轮次、状态、错误码和错误说明",
		Aliases:            []string{"弱密码进度", "弱口令进度", "采集进度", "弱密码任务进度"},
		Tags:               []string{"weak_password", "progress", "task", "v6.1"},
		ObjectTypes:        []string{"task", "collection_progress"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "GetTaskProgress/ListTaskCollectionProgress",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id":   map[string]interface{}{"type": "string"},
				"page_size": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20},
			},
			"required": []string{"task_id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			_ = ctx
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			taskID, err := uuid.Parse(getStringArg(args, "task_id", ""))
			if err != nil {
				return nil, fmt.Errorf("invalid task_id: %w", err)
			}
			pageSize := getIntArg(args, "page_size", 20)
			if pageSize <= 0 || pageSize > 50 {
				pageSize = 20
			}
			progress, err := deps.Service.GetTaskProgress(taskID)
			if err != nil {
				return nil, err
			}
			items, total, err := deps.Service.ListTaskCollectionProgress(taskID, 1, pageSize)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"task_progress":             progress,
				"collection_progress":       items,
				"collection_progress_total": total,
				"next_action":               "如状态仍在采集或匹配中，可稍后再次查询；失败时优先查看 collection_progress 中的 error_code 和 error_message。",
				"route_path":                "/risk/weak-password/tasks/" + taskID.String(),
			}, nil
		},
	}); err != nil {
		return err
	}

	return registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.Explain",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGet,
		Capability:         "weak_password_explain",
		Description:        "解释弱密码命中依据和整改建议，不展示完整明文密码",
		Aliases:            []string{"解释弱密码", "弱密码建议"},
		Tags:               []string{"weak_password", "explain", "v6.1"},
		ObjectTypes:        []string{"finding"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"finding_id": map[string]interface{}{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			_ = ctx
			return map[string]interface{}{
				"summary":        "弱密码命中由服务端字典或 verifier 校验确认，页面默认只展示脱敏密码。",
				"recommendation": "修改命中账号密码，禁用默认口令，限制配置文件权限，并在修复后发起复测。",
			}, nil
		},
	})
}

func getOptionalStringSliceArg(args map[string]interface{}, key string) []string {
	values, err := getStringSliceArg(args, key)
	if err != nil {
		return nil
	}
	return values
}
