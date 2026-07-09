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
		Description:        "对指定主机进行攻击研判，收集告警、漏洞、基线、阻断等证据，生成失陷评估、攻击时间线、攻击路径和取证报告",
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
				"host_id": map[string]interface{}{"type": "string", "description": "目标主机 ID"},
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
		Description:        "根据上下文生成主机攻击研判计划，列出需要收集的证据和分析步骤",
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
				"host_id":  map[string]interface{}{"type": "string", "description": "目标主机 ID"},
				"alert_id": map[string]interface{}{"type": "string", "description": "关联告警 ID（可选）"},
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
			"goal": "分析主机 " + hostID + " 是否遭受攻击",
			"steps": []map[string]interface{}{
				{"step_id": "step_1", "title": "收集告警证据", "objective": "获取该主机的所有告警记录", "suggested_tools": []string{"Detection.Alert.List"}},
				{"step_id": "step_2", "title": "收集漏洞信息", "objective": "获取该主机的漏洞扫描结果", "suggested_tools": []string{"Vulnerability.List"}},
				{"step_id": "step_3", "title": "收集基线检查结果", "objective": "获取该主机的基线检查任务", "suggested_tools": []string{"Task.List"}},
				{"step_id": "step_4", "title": "收集阻断记录", "objective": "获取该主机的阻断策略和记录", "suggested_tools": []string{"Block.Policy.List"}},
				{"step_id": "step_5", "title": "现场取证", "objective": "通过 Agent 获取进程、网络、文件等实时信息", "suggested_tools": []string{"Agent.Process.List", "Agent.Network.List"}},
				{"step_id": "step_6", "title": "综合分析", "objective": "整合所有证据，生成失陷评估和攻击路径", "suggested_tools": []string{"Investigation.HostAttack.Analyze"}},
			},
		}, nil
	}
}
