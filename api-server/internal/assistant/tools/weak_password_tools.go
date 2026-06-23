package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"

	"github.com/google/uuid"
)

type WeakPasswordServiceForTools interface {
	CreateTaskByApplication(ctx context.Context, req model.CreateTaskByApplicationRequest, createdBy *uuid.UUID) (*service.CreateTaskByApplicationResponse, error)
	ListTaskFindings(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordFinding, int64, error)
}

type WeakPasswordToolDeps struct {
	Service WeakPasswordServiceForTools
}

func RegisterWeakPasswordTools(registry *assistant.ToolRegistry, deps WeakPasswordToolDeps) error {
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
