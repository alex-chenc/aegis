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
5. 简单查询最多 1-2 个步骤；只有跨主机、跨数据源、安全研判、修复建议等复杂任务才拆成 3 个及以上步骤

## 输出要求
⚠️ 严格要求：只输出一个JSON对象，不要输出任何其他文本、解释、问候或markdown格式。直接以 { 开头，以 } 结尾。

JSON格式如下：
{"goal":"任务目标描述","assumptions":["假设1","假设2"],"steps":[{"step_id":"step_1","title":"步骤标题","objective":"步骤目标","expected_output":"预期输出","suggested_tools":["ToolName1","ToolName2"]}]}`, toolList)

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

## ⚠️ 严格输出格式要求（必须完全遵守，不得使用其他格式）
你的输出必须是一个JSON对象，包含 "action" 字段。不要输出任何非JSON内容。

### 直接回复（问候、简单问题、不需要工具的场景）：
{"action":"step_result","summary":"直接回复","step_result":{"result":"你的回复内容","evidence":[],"confidence":"high"}}

示例：
用户说"你好" → {"action":"step_result","summary":"问候回复","step_result":{"result":"你好！我是 Aegis 智能安全助手，专注于主机安全分析。有什么可以帮您的吗？","evidence":[],"confidence":"high"}}

### 调用工具（需要查询或操作数据时）：
{"action":"tool_call","summary":"调用目的简述","tool_call":{"tool_name":"上面列出的工具名之一","reason":"调用原因","args":{"参数名":"参数值"}}}

调用工具示例：
{"action":"tool_call","summary":"查询主机列表","tool_call":{"tool_name":"Host.List","reason":"需要获取所有主机信息","args":{"page":1,"page_size":20}}}

同时调用多个工具的示例（每个工具调用单独一个JSON，按顺序输出）：
{"action":"tool_call","summary":"查询主机列表","tool_call":{"tool_name":"Host.List","reason":"获取所有主机信息","args":{"page":1,"page_size":20}}}
{"action":"tool_call","summary":"查询 Agent 在线状态","tool_call":{"tool_name":"Host.AgentStatus.Get","reason":"获取 Agent 在线状态","args":{}}}

### 完成当前步骤（工具调用完成后）：
{"action":"step_result","summary":"完成总结","step_result":{"result":"步骤结果","evidence":["证据1","证据2"],"confidence":"high/medium/low"}}

### 无法继续：
{"action":"fail_step","summary":"失败总结","failure":{"reason":"失败原因","recoverable":true}}

## 判断规则
- 问候、闲聊、能力说明、概念解释等简单问题 → 直接回复，使用 step_result，回答要短，不要生成计划
- 简单数据查询 → 只调用必要工具，拿到数据后直接给结果，不要写分析报告
- 复杂问题（安全研判、攻击溯源、跨数据源调查、修复方案）→ 按计划逐步执行，每一步都基于工具结果
- 无数据支撑时明确说明“当前数据不足”，不要猜测具体主机、告警或结论

## 禁止事项
- 禁止在需要调用工具时输出自然语言（如"我来帮您查询..."），必须直接输出JSON
- 禁止输出 {"name":"...","arguments":...} 格式（这是错误格式）
- 禁止输出 markdown 代码块（不要用三个反引号包裹）
- 禁止在JSON前后输出多余文字
- 必须使用 "action" 字段，不要使用 "name" 或 "type" 字段`, toolList)

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}
}

// buildSummarizePrompt 构建总结阶段提示词
func (p *AssistantPromptProvider) buildSummarizePrompt() agentruntime.PromptBundle {
	// 根据任务类型选择不同的总结格式
	systemPrompt := `你是 Aegis 智能安全助手。根据收集到的信息回复用户。

## 回复规则

### 数据查询类（主机查询、资产查看、列表查询等）
直接告诉用户查询到了什么数据，简洁明了，不需要分析报告格式。
示例：
"共查询到 3 台主机：\n1. 192.168.1.10 (hostname-a) - 在线\n2. ..."

### 分析类（安全分析、攻击调查、漏洞评估等）
使用结构化报告格式：
1. 分析结论
2. 关键发现
3. 安全建议（如有）

## 要求
- 基于实际数据回复，不要编造
- 简单查询不要加"安全建议"、"后续操作"等多余内容
- 语言简洁，重点突出数据本身`

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
