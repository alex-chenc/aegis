package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/repository"
)

// BlockToolDeps 阻断策略工具依赖
type BlockToolDeps struct {
	BlockPolicyRepo *repository.BlockPolicyRepository
}

// RegisterBlockTools 注册阻断策略域工具
func RegisterBlockTools(registry *assistant.ToolRegistry, deps BlockToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Block.Policy.List",
		Domain:             "block",
		Operation:          "policy_list",
		Capability:         "list_block_policies",
		Description:        "列出阻断策略，支持分页和关键字搜索",
		ModelDescription:   "List blocking policies with pagination and keyword search.",
		Risk:               assistant.ToolRiskReadonly,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量"},
				"query":     map[string]interface{}{"type": "string", "description": "搜索关键字（MITRE ID或名称）"},
			},
		},
		Handler: makeBlockPolicyListHandler(deps.BlockPolicyRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Block.Policy.Update",
		Domain:             "block",
		Operation:          "policy_update",
		Capability:         "update_block_policy",
		Description:        "更新阻断策略配置（高风险操作，需审批）",
		ModelDescription:   "Update one blocking policy after explicit user intent and approval.",
		Risk:               assistant.ToolRiskHigh,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mitre_id": map[string]interface{}{
					"type":        "string",
					"description": "MITRE技术ID",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "是否启用策略",
				},
				"auto_block": map[string]interface{}{
					"type":        "boolean",
					"description": "是否启用自动阻断",
				},
				"auto_dispose": map[string]interface{}{
					"type":        "boolean",
					"description": "是否启用自动处置",
				},
				"action": map[string]interface{}{
					"type":        "string",
					"description": "阻断动作（kill_process/quarantine_file/block_connection）",
				},
			},
			"required": []string{"mitre_id"},
		},
		Handler: makeBlockPolicyUpdateHandler(deps.BlockPolicyRepo),
	}); err != nil {
		return err
	}

	return nil
}

func makeBlockPolicyListHandler(repo *repository.BlockPolicyRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)
		query := getStringArg(args, "query", "")

		policies, total, err := repo.ListPaginated(page, pageSize, query)
		if err != nil {
			return nil, fmt.Errorf("failed to list block policies: %w", err)
		}

		return map[string]interface{}{
			"data":  policies,
			"total": total,
		}, nil
	}
}

func makeBlockPolicyUpdateHandler(repo *repository.BlockPolicyRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		mitreID := getStringArg(args, "mitre_id", "")
		if mitreID == "" {
			return nil, fmt.Errorf("mitre_id is required")
		}

		updates := make(map[string]interface{})

		if v, ok := args["enabled"]; ok {
			if enabled, ok := v.(bool); ok {
				updates["enabled"] = enabled
			}
		}
		if v, ok := args["auto_block"]; ok {
			if autoBlock, ok := v.(bool); ok {
				updates["auto_block"] = autoBlock
			}
		}
		if v, ok := args["auto_dispose"]; ok {
			if autoDispose, ok := v.(bool); ok {
				updates["auto_dispose"] = autoDispose
			}
		}
		if action := getStringArg(args, "action", ""); action != "" {
			updates["action"] = action
		}

		if len(updates) == 0 {
			return nil, fmt.Errorf("no fields to update")
		}

		if err := repo.Update(mitreID, updates); err != nil {
			return nil, fmt.Errorf("failed to update block policy: %w", err)
		}

		return map[string]interface{}{
			"mitre_id": mitreID,
			"updates":  updates,
		}, nil
	}
}
