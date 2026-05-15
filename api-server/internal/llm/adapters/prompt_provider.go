package adapters

import (
	"context"
	"fmt"
	"strings"

	agentruntime "github.com/chenchen511/agent-runtime"
)

// AegisPromptProvider implements agent-runtime's PromptProvider interface,
// generating purpose-specific prompts for the Aegis security analysis workflow.
type AegisPromptProvider struct {
	alertCtx           map[string]interface{}
	experienceProvider agentruntime.ExperienceProvider
}

// NewAegisPromptProvider creates a new prompt provider that generates prompts
// tailored to the Aegis security analysis use case. The alertCtx provides
// contextual information about the current alert being analysed. The
// experienceProvider is optional; when non-nil it is used to inject historical
// experience into plan prompts.
func NewAegisPromptProvider(alertCtx map[string]interface{}, experienceProvider agentruntime.ExperienceProvider) *AegisPromptProvider {
	return &AegisPromptProvider{
		alertCtx:           alertCtx,
		experienceProvider: experienceProvider,
	}
}

// Build implements agentruntime.PromptProvider. It dispatches to the appropriate
// prompt template based on req.Purpose.
func (p *AegisPromptProvider) Build(ctx context.Context, req agentruntime.PromptRequest) (agentruntime.PromptBundle, error) {
	switch req.Purpose {
	case agentruntime.PurposePlan:
		return p.buildPlanPrompt(ctx, req)
	case agentruntime.PurposeReact:
		return p.buildReactPrompt(ctx, req)
	case agentruntime.PurposeSummarize:
		return p.buildSummarizePrompt(), nil
	case agentruntime.PurposeAudit, agentruntime.PurposeReflect, agentruntime.PurposeCorrect:
		// Return empty PromptBundle; agent-runtime provides built-in defaults
		// for these purposes.
		return agentruntime.PromptBundle{}, nil
	default:
		return agentruntime.PromptBundle{}, fmt.Errorf("unsupported purpose: %s", req.Purpose)
	}
}

// buildPlanPrompt constructs the plan-phase prompt. When an experience provider
// is configured it fetches relevant historical experience and appends it to the
// system prompt.
func (p *AegisPromptProvider) buildPlanPrompt(ctx context.Context, req agentruntime.PromptRequest) (agentruntime.PromptBundle, error) {
	systemPrompt := planPromptTemplate

	if p.experienceProvider != nil {
		expResp, err := p.experienceProvider.Fetch(ctx, agentruntime.ExperienceRequest{
			TaskID:   req.TaskID,
			Query:    getAlertQuery(p.alertCtx),
			MaxItems: 3,
		})
		if err == nil && len(expResp.Items) > 0 {
			systemPrompt += "\n\n## 历史经验参考\n" + formatExperienceForPrompt(expResp.Items)
		}
	}

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
		Messages: []agentruntime.LLMMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: "请根据以上指令和告警上下文，制定详细的安全分析计划。"},
		},
	}, nil
}

// buildReactPrompt returns the ReAct JSON-action prompt. When an experience
// provider is configured it fetches relevant historical experience and appends
// it to the system prompt.
func (p *AegisPromptProvider) buildReactPrompt(ctx context.Context, req agentruntime.PromptRequest) (agentruntime.PromptBundle, error) {
	systemPrompt := reactJSONPromptTemplate

	if p.experienceProvider != nil {
		expResp, err := p.experienceProvider.Fetch(ctx, agentruntime.ExperienceRequest{
			TaskID:   req.TaskID,
			Query:    getAlertQuery(p.alertCtx),
			MaxItems: 3,
		})
		if err == nil && len(expResp.Items) > 0 {
			systemPrompt += "\n\n## 历史经验参考\n" + formatExperienceForPrompt(expResp.Items)
		}
	}

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}, nil
}

// buildSummarizePrompt returns the attack-graph summarisation prompt.
func (p *AegisPromptProvider) buildSummarizePrompt() agentruntime.PromptBundle {
	return agentruntime.PromptBundle{
		SystemPrompt: summarizePromptTemplate,
	}
}

// ---------------------------------------------------------------------------
// Prompt templates
// ---------------------------------------------------------------------------

const planPromptTemplate = `你是一个专业的安全分析AI助手，负责分析主机安全告警并制定分析计划。

## 可用工具
你可以使用以下工具来收集信息：
- GetProcessTree: 获取指定主机上指定进程的完整进程树
- GetNetworkConnections: 获取指定主机的网络连接信息
- GetOpenFiles: 获取指定进程打开的文件列表
- GetRunningProcesses: 获取指定主机上正在运行的进程列表
- GetUserSessions: 获取指定主机上的用户会话信息
- QueryHistoricalLogs: 查询指定主机的历史日志

## 输出要求
请以JSON格式输出分析计划，包含以下字段：
{
  "goal": "分析目标描述",
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
}`

const reactJSONPromptTemplate = `你是一个安全分析AI助手，正在执行分析计划的某个步骤。

## 可用工具（必须严格使用以下工具名，不得发明新工具名）
- GetProcessTree: 获取指定主机上指定进程的完整进程树。参数：{"host_id":"主机ID","pid":进程PID}
- GetNetworkConnections: 获取指定主机的网络连接信息。参数：{"host_id":"主机ID","pid":进程PID（可选）}
- GetOpenFiles: 获取指定进程打开的文件列表。参数：{"host_id":"主机ID","pid":进程PID}
- GetRunningProcesses: 获取指定主机上正在运行的进程列表。参数：{"host_id":"主机ID","filter":"过滤条件（可选）"}
- GetUserSessions: 获取指定主机上的用户会话信息。参数：{"host_id":"主机ID"}
- QueryHistoricalLogs: 查询指定主机的历史日志。参数：{"host_id":"主机ID","start_time":"起始时间","end_time":"结束时间","filter":"过滤条件（可选）"}

## 行动格式
你必须以JSON格式输出行动指令：

调用工具时输出：
{"action":"tool_call","summary":"调用目的简述","tool_call":{"tool_name":"上面列出的工具名之一","reason":"调用原因","args":{...}}}

当收集到足够信息完成当前步骤时，输出：
{"action":"step_result","summary":"完成总结","step_result":{"result":"步骤结果","evidence":["证据1","证据2"],"confidence":"high/medium/low"}}

当无法继续时，输出：
{"action":"fail_step","summary":"失败总结","failure":{"reason":"失败原因","recoverable":true/false}}`

const summarizePromptTemplate = `你是一个安全分析AI助手，需要根据所有收集到的信息生成最终分析报告。

## 输出格式
请以严格的JSON格式输出，包含以下结构：
{
  "attack_graph": {
    "nodes": [{"id":"n1","label":"节点标签","type":"process|file|network|user","details":{}}],
    "edges": [{"from":"n1","to":"n2","label":"关系描述","type":"creates|connects|accesses|executes"}],
    "timeline": [{"time":"时间","event":"事件描述","severity":"low|medium|high|critical"}],
    "threat_level": "low|medium|high|critical",
    "recommendations": ["建议1", "建议2"]
  },
  "conclusions": [
    {"alert_id":"告警ID","action":"mark_false_positive|confirm_threat|generate_rule","summary":"该告警的中文分析结论"}
  ]
}

要求：
- conclusions[].alert_id 必须使用告警上下文中的 alert_id 或原始告警 ID。
- conclusions[].action 只能取 mark_false_positive、confirm_threat、generate_rule。
- 不要声称未执行的计划步骤已经完成。`

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// getAlertQuery extracts a query string from the alert context map for use in
// experience retrieval. It tries common fields and falls back to joining all
// string values in the map.
func getAlertQuery(alertCtx map[string]interface{}) string {
	if alertCtx == nil {
		return ""
	}

	// Try common descriptive fields in priority order.
	candidates := []string{"title", "description", "rule_name", "rule_id", "alert_type"}
	for _, key := range candidates {
		if v, ok := alertCtx[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}

	// Fallback: join all string values.
	var parts []string
	for _, v := range alertCtx {
		if s, ok := v.(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// formatExperienceForPrompt renders a slice of ExperienceItem into a
// human-readable text block suitable for injection into a system prompt.
func formatExperienceForPrompt(items []agentruntime.ExperienceItem) string {
	var buf strings.Builder
	for i, item := range items {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		fmt.Fprintf(&buf, "### 经验 %d: %s\n%s", i+1, item.Summary, item.Content)
	}
	return buf.String()
}
