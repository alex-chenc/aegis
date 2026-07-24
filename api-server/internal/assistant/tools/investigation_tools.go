package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/model"
)

// InvestigationToolDeps 研判工具依赖
type InvestigationToolDeps struct {
	InvestigationService *assistant.HostAttackInvestigationService
}

// RegisterInvestigationTools 注册 Investigation 域工具（对齐设计文档 14.6 节）
func RegisterInvestigationTools(registry *assistant.ToolRegistry, deps InvestigationToolDeps) error {
	// Investigation.HostAttack.Analyze — 主机攻击研判（高层工具）
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Investigation.HostAttack.Analyze",
		Domain:             assistant.DomainInvestigation,
		Operation:          assistant.OpExecute,
		Capability:         "analyze_host_attack",
		Description:        "Investigate a target host by correlating alert, vulnerability, baseline, block, and live evidence into a compromise assessment, timeline, attack path, and forensic report.",
		Aliases:            []string{"攻击研判", "主机分析", "安全分析", "analyze attack"},
		Tags:               []string{"v6.0", "investigation", "host_attack", "forensics"},
		ObjectTypes:        []string{"host", "alert"},
		PageRoutes:         []string{"/detection/alerts", "/hosts"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact target host UUID."},
			},
			"required": []string{"host_id"},
		},
		Handler: makeInvestigationAnalyzeHandler(deps.InvestigationService),
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "api-server/internal/assistant/investigation_service.go",
			Function:  "HostAttackInvestigationService.CreateInvestigation",
		},
	}); err != nil {
		return err
	}

	// Investigation.HostAttack.Plan — 生成研判计划
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Investigation.HostAttack.Plan",
		Domain:             assistant.DomainInvestigation,
		Operation:          assistant.OpGet,
		Capability:         "plan_investigation",
		Description:        "Create a host-attack investigation plan that lists required evidence and analysis stages.",
		Aliases:            []string{"研判计划", "调查计划"},
		Tags:               []string{"v6.0", "investigation", "planning"},
		ObjectTypes:        []string{"host", "alert"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_id":  map[string]interface{}{"type": "string", "format": "uuid", "description": "Exact target host UUID."},
				"alert_id": map[string]interface{}{"type": "string", "description": "Optional related alert ID."},
			},
			"required": []string{"host_id"},
		},
		Handler: makeInvestigationPlanHandler(),
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "api-server/internal/assistant/investigation_service.go",
			Function:  "HostAttackInvestigationService.CreateInvestigation",
		},
	}); err != nil {
		return err
	}

	return nil
}

// makeInvestigationAnalyzeHandler 创建攻击研判 handler
func makeInvestigationAnalyzeHandler(svc *assistant.HostAttackInvestigationService) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		hostID, _ := args["host_id"].(string)
		if hostID == "" {
			return nil, fmt.Errorf("host_id is required")
		}
		input := model.HostAttackInvestigationInput{
			HostID: hostID,
		}
		return svc.CreateInvestigation(ctx, input, "assistant")
	}
}

// makeInvestigationPlanHandler 创建研判计划 handler
func makeInvestigationPlanHandler() assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		hostID, _ := args["host_id"].(string)
		if hostID == "" {
			return nil, fmt.Errorf("host_id is required")
		}

		// 返回研判计划（静态模板，由 agent-runtime 的 Plan 阶段消费）
		return map[string]interface{}{
			"goal": "Determine whether host " + hostID + " was attacked.",
			"steps": []map[string]interface{}{
				{"step_id": "step_1", "title": "Collect alert evidence", "objective": "Get all alert records for the host.", "suggested_tools": []string{"Detection.Alert.List"}},
				{"step_id": "step_2", "title": "Collect vulnerability evidence", "objective": "Get vulnerability assessment results for the host.", "suggested_tools": []string{"Vulnerability.List"}},
				{"step_id": "step_3", "title": "Collect baseline results", "objective": "Get baseline task results for the host.", "suggested_tools": []string{"Task.List"}},
				{"step_id": "step_4", "title": "Collect block records", "objective": "Get relevant block policies and records for the host.", "suggested_tools": []string{"Block.Policy.List"}},
				{"step_id": "step_5", "title": "Collect live forensic evidence", "objective": "Use the agent to get current process, network, and file evidence.", "suggested_tools": []string{"Agent.Process.List", "Agent.Network.List"}},
				{"step_id": "step_6", "title": "Correlate evidence", "objective": "Correlate all evidence into a compromise assessment and attack path with explicit gaps.", "suggested_tools": []string{"Investigation.HostAttack.Analyze"}},
			},
		}, nil
	}
}
