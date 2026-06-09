package assistant

import (
	"context"
	"strings"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestAssistantPromptProviderIncludesHostSecurityAnalysisGuide(t *testing.T) {
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
		{name: "summarize", purpose: agentruntime.PurposeSummarize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: tc.purpose})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			prompt := bundle.SystemPrompt
			for _, want := range []string{
				"主机安全整体分析准则",
				"每台目标主机给出清晰风险等级",
				"Host.List 返回 N 台目标主机后必须覆盖全部 N 台",
				"不要因为“没有告警”就直接判定安全",
			} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("prompt for %s missing %q\n%s", tc.name, want, prompt)
				}
			}
		})
	}
}

func TestAssistantSummarizePromptRequiresClearHostSecurityResult(t *testing.T) {
	provider := NewAssistantPromptProvider(nil, nil, "analysis", "分析主机安全")
	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeSummarize})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"安全结论：是否存在安全问题、最高风险等级、最需要立即处理的问题",
		"每台主机分析：主机名/IP、在线状态、风险等级",
		"不得把“没有告警”写成“没有风险”",
		"先给最清晰结论，再给证据",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("summarize prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
}

func TestAssistantPromptProviderIncludesMandatoryToolSequenceGuide(t *testing.T) {
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
		"用户指定工具执行约束",
		"1. Baseline.Template.Status.Get",
		"2. Baseline.Template.Rules.List",
		"3. Baseline.Script.Generate",
		"4. Baseline.Script.Generate",
		"5. Task.RunCheck",
		"6. Task.RunFix",
		"7. Task.List",
		"Task.List 只能在 Task.RunCheck/Task.RunFix 下发后用于查询进度或结果",
		"完成检测脚本和修复脚本生成后，下一步必须下发 Task.RunCheck",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
}

func TestAssistantPromptProviderIncludesAssetCollectionSequenceCompletionGuide(t *testing.T) {
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

	for _, want := range []string{
		"资产采集闭环",
		"asset_collection_sequence_complete=true",
		"all_requested_tools_success=true",
		"verified_result_summary",
		"不要再调用 Task.GetDetail、Tool.Search",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing %q\n%s", want, bundle.SystemPrompt)
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
		"自然语言工具使用推理原则",
		"先理解业务目标",
		"先抽取：目标对象",
		"信息不足时先判断能否安全默认",
		"工具选择应覆盖用户最终目标",
		"采集只是证据来源之一",
		"当前工具不足时使用 Tool.Search",
		"同一工具用同一参数已成功时，直接复用已有结果",
		"汇总、总结、输出结论、整理结果",
		"中间步骤的 step_result 只写该步骤产物",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
	for _, forbidden := range []string{
		"package_name=\"mysql\"",
		"必须按以下顺序执行",
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
		"不要把“汇总分析结果/输出最终结论”规划成需要再次调用工具的独立步骤",
		"不得重复查询已成功获取的数据",
	} {
		if !strings.Contains(plan.SystemPrompt, want) {
			t.Fatalf("plan prompt missing %q\n%s", want, plan.SystemPrompt)
		}
	}

	summarize, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeSummarize})
	if err != nil {
		t.Fatalf("Build(summarize) error = %v", err)
	}
	if !strings.Contains(summarize.SystemPrompt, "合并去重后只给一次最终结论") {
		t.Fatalf("summarize prompt missing duplicate-final-report guard\n%s", summarize.SystemPrompt)
	}
}

func TestAssistantPromptProviderIncludesVulnerabilityExecuteSequenceGuide(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{Name: "Vulnerability.Script.Status", Description: "查询漏洞脚本状态"},
		{Name: "Vulnerability.Script.Execute", Description: "执行漏洞脚本"},
	}, nil, "operations", strings.Join([]string{
		"请严格按顺序调用工具：Vulnerability.Script.Status、Vulnerability.Script.Execute。",
		`Vulnerability.Script.Execute 参数 cve_id="CVE-2023-50495", script_type="fix", host_ids=["host-1"]。`,
	}, "\n"))

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeReact})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"漏洞 POC/FIX 闭环",
		"vulnerability_script_sequence_complete=true",
		"executions 中的 task_group_id",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("react prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
}
