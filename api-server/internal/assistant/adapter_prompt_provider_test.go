package assistant

import (
	"context"
	"strings"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestAssistantPromptProviderIncludesGenericAgentReasoningGuide(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{
			Name:        "Host.List",
			Description: "列出主机",
			ArgsSchema: map[string]interface{}{
				"properties": map[string]interface{}{
					"page_size": map[string]interface{}{"type": "integer"},
					"status":    map[string]interface{}{"type": "string"},
				},
			},
		},
		{Name: "Agent.Process.List", Description: "查询进程"},
		{Name: "Detection.Alert.List", Description: "查询告警"},
	}, nil, "analysis", "分析全部在线主机是否存在安全问题")

	cases := []struct {
		name    string
		purpose agentruntime.LLMPurpose
	}{
		{name: "plan", purpose: agentruntime.PurposePlan},
		{name: "react", purpose: agentruntime.PurposeReact},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: tc.purpose})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			prompt := bundle.SystemPrompt
			for _, want := range []string{
				"Generic agent reasoning",
				"dynamic tool catalog is the capability boundary",
				"actual prior results",
				"never apply a fixed workflow",
			} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("prompt for %s missing %q\n%s", tc.name, want, prompt)
				}
			}
		})
	}
}

func TestAssistantSummarizePromptRequiresEvidenceGroundedResult(t *testing.T) {
	provider := NewAssistantPromptProvider(nil, nil, "analysis", "分析主机安全")
	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeSummarize})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"Answer the final goal directly",
		"Never report task creation as task completion",
		"Never generalize a partial result",
		"Never invent IDs, status, counts, impact scope, or execution results",
		"descriptor validation failure",
		"arguments validation failure",
		"must not be described as a missing platform capability",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("summarize prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
}

func TestAssistantReactPromptIncludesExactEnglishArgumentSchema(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{{
		Name:        "Example.Execute",
		Description: "Execute an example operation.",
		ArgsSchema: map[string]interface{}{
			"type":     "object",
			"required": []interface{}{"host_ids", "max_rounds"},
			"properties": map[string]interface{}{
				"host_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "目标主机列表",
				},
				"max_rounds": map[string]interface{}{
					"type":        "integer",
					"minimum":     1,
					"maximum":     10,
					"description": "自动修复轮数",
				},
			},
		},
	}}, nil, "operations", "execute the operation")

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeReact})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, want := range []string{
		`"host_ids"`,
		`"type":"array"`,
		`"max_rounds"`,
		`"type":"integer"`,
		`"required":["host_ids","max_rounds"]`,
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing exact argument schema %q\n%s", want, bundle.SystemPrompt)
		}
	}
	for _, forbidden := range []string{"目标主机列表", "自动修复轮数"} {
		if strings.Contains(bundle.SystemPrompt, forbidden) {
			t.Fatalf("react prompt leaked localized schema description %q\n%s", forbidden, bundle.SystemPrompt)
		}
	}
}

func TestAssistantPromptProviderDoesNotBuildSequenceFromUserText(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{Name: "Baseline.Template.Status.Get", Description: "查询基线模板解析状态"},
		{Name: "Baseline.Template.Rules.List", Description: "查询基线规则"},
		{Name: "Baseline.Script.Generate", Description: "生成基线脚本"},
		{Name: "Task.RunCheck", Description: "下发基线检测任务"},
		{Name: "Task.RunFix", Description: "下发基线修复任务"},
		{Name: "Task.List", Description: "查询任务列表"},
	}, nil, "operations", strings.Join([]string{
		"请按顺序调用：Baseline.Template.Status.Get、Baseline.Template.Rules.List、Baseline.Script.Generate(CHECK)、Baseline.Script.Generate(FIX)、Task.RunCheck、Task.RunFix、Task.List。",
		"模板 template_id=tpl-1，规则 rule_id=rule-1，目标 host_id=host-1。",
	}, "\n"))

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeReact})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"Generic agent reasoning",
		"Re-evaluate the next action after every result",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
	for _, forbidden := range []string{
		"User-specified tool execution sequence",
		"must call in the order listed",
		"1. Baseline.Template.Status.Get",
	} {
		if strings.Contains(bundle.SystemPrompt, forbidden) {
			t.Fatalf("react prompt must not synthesize a fixed sequence %q\n%s", forbidden, bundle.SystemPrompt)
		}
	}
}

func TestAssistantPromptProviderDoesNotDescribeHiddenAssetCollectionSequence(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{Name: "Asset.Collection.Trigger", Description: "触发资产采集"},
		{Name: "Asset.Collection.Get", Description: "查询采集详情"},
		{Name: "Asset.Application.List", Description: "查询应用资产"},
		{Name: "Asset.Summary.Get", Description: "查询资产概览"},
	}, nil, "operations", strings.Join([]string{
		"请严格按顺序调用工具：Asset.Collection.Trigger、Asset.Collection.Get、Asset.Application.List、Asset.Summary.Get。",
		"不要只文字说明。",
	}, "\n"))

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeReact})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, forbidden := range []string{
		"asset_collection_sequence_complete=true",
		"all_requested_tools_success=true",
		"verified_result_summary",
	} {
		if strings.Contains(bundle.SystemPrompt, forbidden) {
			t.Fatalf("react prompt must not contain hidden sequence marker %q\n%s", forbidden, bundle.SystemPrompt)
		}
	}
}

func TestAssistantPromptProviderGuidesNaturalOperationToolReasoning(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{Name: "Asset.Collection.Trigger", Description: "触发资产采集"},
		{Name: "Asset.Collection.Get", Description: "查询采集详情"},
		{Name: "Asset.Application.List", Description: "查询应用资产"},
		{Name: "Asset.Summary.Get", Description: "查询资产概览"},
		{Name: "Software.Installed.Search", Description: "查询已安装软件"},
		{Name: "Vulnerability.List", Description: "查询漏洞"},
		{Name: "Vulnerability.AffectedHosts", Description: "查询受影响主机"},
	}, nil, "operations", "进行资产采集任务，并分析那个主机上有 MySQL 软件，并分析此 MySql 软件是否有漏洞")

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeReact})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"Generic agent reasoning",
		"Understand the final goal",
		"dynamic tool catalog is the capability boundary",
		"actual prior results",
		"If authorized tools cannot complete the goal",
		"Reuse a successful result for the same tool and arguments",
		"only summarizes or organizes existing results",
		"An intermediate step_result contains only that step's output",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
	for _, forbidden := range []string{
		"package_name=\"mysql\"",
		"must execute in this order",
	} {
		if strings.Contains(bundle.SystemPrompt, forbidden) {
			t.Fatalf("react prompt should not contain fixed workflow %q\n%s", forbidden, bundle.SystemPrompt)
		}
	}
}

func TestAssistantPlanAndSummarizePromptsAvoidDuplicateFinalReports(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{Name: "Host.List", Description: "列出主机"},
		{Name: "Software.Installed.Search", Description: "查询已安装软件"},
	}, nil, "operations", "更新存活的 Agent资产，资产中是否存在 MySQL 软件，软件中是否存在漏洞")

	plan, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposePlan})
	if err != nil {
		t.Fatalf("Build(plan) error = %v", err)
	}
	for _, want := range []string{
		"Do not create a separate tool step merely to summarize",
		"must reuse successful evidence",
	} {
		if !strings.Contains(plan.SystemPrompt, want) {
			t.Fatalf("plan prompt missing %q\n%s", want, plan.SystemPrompt)
		}
	}

	summarize, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeSummarize})
	if err != nil {
		t.Fatalf("Build(summarize) error = %v", err)
	}
	if !strings.Contains(summarize.SystemPrompt, "provide the final conclusion only once") {
		t.Fatalf("summarize prompt missing duplicate-final-report guard\n%s", summarize.SystemPrompt)
	}
}

func TestAssistantPromptProviderDoesNotInjectVulnerabilityWorkflow(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{Name: "Vulnerability.Script.Status", Description: "查询漏洞脚本状态"},
		{Name: "Vulnerability.Script.Execute", Description: "执行漏洞脚本"},
	}, nil, "operations", strings.Join([]string{
		"请生成并下发漏洞 POC 与修复脚本，最大自动修复轮数 max_rounds=5。",
		"请严格按顺序调用工具：Vulnerability.Script.Status、Vulnerability.Script.Execute。",
		`Vulnerability.Script.Execute 参数 cve_id="CVE-2023-50495", script_type="fix", host_ids=["host-1"]。`,
	}, "\n"))

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeReact})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"Generic agent reasoning",
		"Re-evaluate the next action after every result",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
	for _, forbidden := range []string{
		"explicitly plan the vulnerability POC and remediation workflow",
		"vulnerability_script_sequence_complete=true",
		"Vulnerability.CustomQuery.Start",
	} {
		if strings.Contains(bundle.SystemPrompt, forbidden) {
			t.Fatalf("react prompt must not inject a vulnerability workflow %q\n%s", forbidden, bundle.SystemPrompt)
		}
	}
}
