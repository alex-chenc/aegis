package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// AssistantPromptProvider 适配 agent-runtime PromptProvider 接口
// 为智能助手生成特定的 Plan/React/Summarize 提示词
type AssistantPromptProvider struct {
	toolDescriptors []agentruntime.ToolDescriptor
	contextRefs     []ContextRefResult
	taskType        string
	userMessage     string
}

// NewAssistantPromptProvider 创建提示词提供者
func NewAssistantPromptProvider(
	toolDescriptors []agentruntime.ToolDescriptor,
	contextRefs []ContextRefResult,
	taskType string,
	userMessage string,
) *AssistantPromptProvider {
	return &AssistantPromptProvider{
		toolDescriptors: toolDescriptors,
		contextRefs:     contextRefs,
		taskType:        taskType,
		userMessage:     userMessage,
	}
}

// Build 实现 agentruntime.PromptProvider 接口
func (p *AssistantPromptProvider) Build(ctx context.Context, req agentruntime.PromptRequest) (agentruntime.PromptBundle, error) {
	switch req.Purpose {
	case agentruntime.PurposePlan:
		return p.buildPlanPrompt(), nil
	case agentruntime.PurposeReact:
		return p.buildReactPrompt(), nil
	case agentruntime.PurposeSummarize:
		return p.buildSummarizePrompt(), nil
	case agentruntime.PurposeAudit, agentruntime.PurposeReflect, agentruntime.PurposeCorrect:
		// 返回空 PromptBundle，使用 agent-runtime 内置默认
		return agentruntime.PromptBundle{}, nil
	default:
		return agentruntime.PromptBundle{}, nil
	}
}

// buildPlanPrompt 构建计划阶段提示词
func (p *AssistantPromptProvider) buildPlanPrompt() agentruntime.PromptBundle {
	toolList := p.formatToolList()

	systemPrompt := fmt.Sprintf(`你是 Aegis 智能安全助手，专注于主机安全分析和运维操作。

## 你的能力
- 查询和分析主机资产、安全态势
- 分析告警、追溯攻击路径
- 管理基线检查、漏洞扫描
- 管理检测包、Sigma 规则
- 执行阻断策略
- 主机攻击研判

## 可用工具
%s

## 规则
1. 所有操作必须通过工具调用完成，不能直接执行命令
2. 高风险操作需要用户审批
3. 所有结论必须基于数据和证据
4. 不确定时明确说明，不编造信息

## 输出要求
请以JSON格式输出执行计划：
{
  "goal": "任务目标描述",
  "assumptions": ["假设1", "假设2"],
  "steps": [
    {
      "step_id": "step_1",
      "title": "步骤标题",
      "objective": "步骤目标",
      "expected_output": "预期输出",
      "suggested_tools": ["ToolName1", "ToolName2"]
    }
  ]
}`, toolList)

	userPrompt := p.userMessage
	if len(p.contextRefs) > 0 {
		userPrompt += "\n\n## 上下文信息\n"
		for _, ref := range p.contextRefs {
			userPrompt += fmt.Sprintf("- %s (%s): %s\n", ref.Title, ref.ObjectType, ref.Summary)
		}
	}

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
		Messages: []agentruntime.LLMMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
}

// buildReactPrompt 构建 ReAct 阶段提示词
func (p *AssistantPromptProvider) buildReactPrompt() agentruntime.PromptBundle {
	toolList := p.formatToolListDetail()

	systemPrompt := fmt.Sprintf(`你是 Aegis 智能安全助手，正在执行安全分析任务。

## 可用工具（必须严格使用以下工具名，不得发明新工具名）
%s

## 行动格式
你必须以JSON格式输出行动指令：

调用工具时输出：
{"action":"tool_call","summary":"调用目的简述","tool_call":{"tool_name":"上面列出的工具名之一","reason":"调用原因","args":{...}}}

当收集到足够信息完成当前步骤时，输出：
{"action":"step_result","summary":"完成总结","step_result":{"result":"步骤结果","evidence":["证据1","证据2"],"confidence":"high/medium/low"}}

当无法继续时，输出：
{"action":"fail_step","summary":"失败总结","failure":{"reason":"失败原因","recoverable":true/false}}`, toolList)

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}
}

// buildSummarizePrompt 构建总结阶段提示词
func (p *AssistantPromptProvider) buildSummarizePrompt() agentruntime.PromptBundle {
	systemPrompt := `你是 Aegis 智能安全助手，需要根据所有收集到的信息生成最终分析报告。

## 输出格式
请以清晰的中文格式输出分析结果，包含：
1. 任务完成情况总结
2. 关键发现和数据
3. 安全建议（如有）
4. 后续操作建议（如有）

要求：
- 所有结论必须基于实际收集到的数据
- 不要声称未执行的操作已经完成
- 如果信息不足，明确说明`

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}
}

// formatToolList 格式化工具列表（简要版）
func (p *AssistantPromptProvider) formatToolList() string {
	var buf strings.Builder
	for _, desc := range p.toolDescriptors {
		buf.WriteString(fmt.Sprintf("- %s: %s\n", desc.Name, desc.Description))
	}
	return buf.String()
}

// formatToolListDetail 格式化工具列表（详细版，含参数）
func (p *AssistantPromptProvider) formatToolListDetail() string {
	var buf strings.Builder
	for _, desc := range p.toolDescriptors {
		buf.WriteString(fmt.Sprintf("- %s: %s", desc.Name, desc.Description))
		if desc.ArgsSchema != nil {
			if props, ok := desc.ArgsSchema["properties"].(map[string]interface{}); ok {
				var params []string
				for k := range props {
					params = append(params, k)
				}
				if len(params) > 0 {
					buf.WriteString(fmt.Sprintf("。参数：%s", formatParamList(props, desc.ArgsSchema)))
				}
			}
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

// formatParamList 格式化参数列表
func formatParamList(props map[string]interface{}, schema map[string]interface{}) string {
	required, _ := schema["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		if s, ok := r.(string); ok {
			requiredSet[s] = true
		}
	}

	var parts []string
	for name, prop := range props {
		propMap, ok := prop.(map[string]interface{})
		if !ok {
			continue
		}
		desc, _ := propMap["description"].(string)
		if requiredSet[name] {
			parts = append(parts, fmt.Sprintf("%s（必填）", name))
		} else {
			parts = append(parts, fmt.Sprintf("%s（可选）", name))
		}
		_ = desc // 描述暂不输出，保持简洁
	}
	b, _ := json.Marshal(parts)
	return string(b)
}
