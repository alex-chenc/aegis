package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/repository"
)

// AuditToolDeps 审计工具依赖
type AuditToolDeps struct {
	AuditLogRepo *repository.AuditLogRepo
}

// RegisterAuditTools 注册审计域工具
func RegisterAuditTools(registry *assistant.ToolRegistry, deps AuditToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Audit.Log.List",
		Domain:             "audit",
		Operation:          "log_list",
		Capability:         "list_audit_logs",
		Description:        "列出审计日志，支持按脚本类型、审计来源和审核结果筛选",
		ModelDescription:   "List audit-log records with pagination and optional script type, source, or review-result filters.",
		Risk:               assistant.ToolRiskLow,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":         map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size":    map[string]interface{}{"type": "integer", "description": "每页数量"},
				"script_type":  map[string]interface{}{"type": "string", "description": "脚本类型"},
				"audit_source": map[string]interface{}{"type": "string", "description": "审计来源"},
				"passed":       map[string]interface{}{"type": "string", "description": "审核结果（true/false）"},
			},
		},
		Handler: makeAuditLogListHandler(deps.AuditLogRepo),
	}); err != nil {
		return err
	}

	return nil
}

func makeAuditLogListHandler(repo *repository.AuditLogRepo) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)
		scriptType := getStringArg(args, "script_type", "")
		auditSource := getStringArg(args, "audit_source", "")
		passed := getStringArg(args, "passed", "")

		logs, total, err := repo.List(scriptType, auditSource, passed, page, pageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to list audit logs: %w", err)
		}

		return map[string]interface{}{
			"data":  logs,
			"total": total,
		}, nil
	}
}
