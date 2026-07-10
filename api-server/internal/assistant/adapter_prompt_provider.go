package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// AssistantPromptProvider 适配 agent-runtime PromptProvider 接口
// 为智能助手生成特定的 Plan/React/Summarize 提示词
type AssistantPromptProvider struct {
	toolDescriptors    []agentruntime.ToolDescriptor
	contextRefs        []ContextRefResult
	taskType           string
	userMessage        string
	reflectionMemories []string
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

func (p *AssistantPromptProvider) WithReflectionMemories(memories []string) *AssistantPromptProvider {
	p.reflectionMemories = memories
	return p
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
	reflectionGuide := p.formatReflectionGuide()
	contextBlock := p.formatContextRefs()
	reasoningGuide := genericAgentReasoningGuide()

	systemPrompt := fmt.Sprintf(`You are the Aegis assistant planner. Plan from the user's goal, context, and current dynamic tool set.

## Capabilities
- Understand arbitrary goals, objects, scope, constraints, and completion criteria.
- Decompose a request into the minimum complete set of single-step or multi-step work.
- Use actual tool results to decide branches, status checks, retries, and verification.
- Operate under approval, authorization, and evidence constraints.

## Available tools
%s

%s

%s

%s

## Rules
1. Perform operations only through available tools. Never execute commands directly.
2. High-risk operations require user approval.
3. Base every conclusion on data and evidence.
4. State uncertainty explicitly and never invent information.
5. Generate the minimum complete steps from the final goal, dependencies, conditional branches, asynchronous states, and verification requirements. Never apply a predefined business workflow.
6. suggested_tools may contain only exact names from Available tools. Record unavailable capabilities as evidence gaps; never invent a tool.
7. Arguments may come from the user message, context, or actual prior tool results. Do not ask the user to repeat an ID that a tool can discover.
8. If the same tool must be called with different arguments, give every step a unique title that reflects its purpose or parameter dimension.
9. Keep titles short and distinct. Put detailed goals, conditions, argument sources, and evidence requirements in objective and expected_output.
10. Do not create a separate tool step merely to summarize or output the final conclusion. The summarization stage produces the final answer and must reuse successful evidence.
11. Natural-language plan fields should follow the user's language. Tool names and machine identifiers must remain exact English catalog values.

## Output requirements
Return exactly one JSON object. Do not output explanations, greetings, prose, or Markdown. Start with { and end with }.

JSON schema:
{"goal":"goal description","assumptions":["assumption"],"steps":[{"step_id":"step_1","title":"unique concise title","objective":"goal, conditions, and argument sources","expected_output":"expected output","suggested_tools":["ToolName1","ToolName2"]}]}`, toolList, reflectionGuide, contextBlock, reasoningGuide)

	userPrompt := p.userMessage
	if contextBlock != "" {
		userPrompt += "\n\n" + contextBlock
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
	reflectionGuide := p.formatReflectionGuide()
	contextBlock := p.formatContextRefs()
	reasoningGuide := genericAgentReasoningGuide()

	systemPrompt := fmt.Sprintf(`You are the Aegis assistant executing the current user task.

## Available tools
Use only the exact names listed here. Never invent a tool.
%s

%s

%s

%s

## Strict output format
Return exactly one JSON object containing an "action" field. Do not output non-JSON text.

### Direct response for a greeting, simple question, or task that needs no tool
{"action":"step_result","summary":"direct response","step_result":{"result":"answer in the user's language","evidence":[],"confidence":"high"}}

### Call one tool
{"action":"tool_call","summary":"concise purpose","tool_call":{"tool_name":"one exact available tool name","reason":"reason for the call","args":{"argument_name":"value"}}}

Choose tool names and arguments only from Available tools, user input, context, and actual prior results. Emit one tool call per response. If more calls may be needed, wait for the actual result before deciding the next call.

### Complete the current step
{"action":"step_result","summary":"completion summary","step_result":{"result":"step result","evidence":["exact terminal call_id when tools were used"],"confidence":"high|medium|low"}}

### Cannot continue
{"action":"fail_step","summary":"failure summary","failure":{"reason":"failure reason","recoverable":true}}

## Decision rules
- For greetings, casual conversation, capability questions, or conceptual explanations, return a concise step_result without a plan.
- For a simple data query, call only the necessary tools and return the result without an unnecessary report.
- For a complex goal, follow the current plan and ground every step in actual tool results.
- Cover the complete set or scope requested by the user. If pagination, offline targets, permissions, or tool failures leave partial coverage, list the exact gap and reason.
- When a tool fails, an asynchronous task is incomplete, or a result is empty, use its contract and actual result to decide whether to retry, query status, use an authorized alternative, or record an evidence gap.
- A successful tool transport can still have operation_status=accepted or running. These non-terminal outcomes never satisfy a step.
- When tools were used, step_result.evidence must contain exact call_id values for successful terminal outcomes. Never cite free-form evidence in place of a call ID.
- When evidence is insufficient, state that evidence is insufficient. Never guess host, alert, status, or conclusion details.
- After a tool error, use Internal recovery reflections to correct arguments or choose an alternative and retry at most once. Do not retry indefinitely.
- Reuse a successful result for the same tool and arguments. Do not issue duplicate calls.
- If the current step only summarizes or organizes existing results, return step_result and do not call another tool.
- An intermediate step_result contains only that step's output. The final report is produced once by the summarization stage.
- Never apply a fixed workflow based on a tool name or business keyword. Decide from the user's goal and observed results.
- Natural-language fields must follow the user's language. Tool names, arguments, enum values, and machine identifiers must remain exact catalog values.

## Forbidden output
- Do not output prose when a tool call is required.
- Do not use {"name":"...","arguments":...}.
- Do not output Markdown fences.
- Do not add text before or after the JSON object.
- Use "action", never "name" or "type", as the discriminator field.`, toolList, reflectionGuide, contextBlock, reasoningGuide)

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}
}

// buildSummarizePrompt 构建总结阶段提示词
func (p *AssistantPromptProvider) buildSummarizePrompt() agentruntime.PromptBundle {
	systemPrompt := fmt.Sprintf(`You are the Aegis assistant. Answer from the user's original goal, plan steps, and actual tool evidence.

## Authorized tool catalog for this run
%s

## Response rules
1. Answer the final goal directly. Do not repeat internal plans, control JSON, or irrelevant process details.
2. Keep simple answers and queries concise. For complex analysis, use only the necessary structure: conclusion, key evidence, coverage, evidence gaps, and recommendations.
3. For operations, distinguish precisely among succeeded, created but still running, skipped, failed, rejected, and not executed. Never report task creation as task completion.
4. For a requested set or scope, state the total target count, covered count, and uncovered objects. Never generalize a partial result to the full set.
5. A successful empty result means the source was queried and no record was found. A missing tool call, failure, permission denial, or non-terminal status is an evidence gap.
6. Derive conclusions only from actual tool results and user-provided context. Never invent IDs, status, counts, impact scope, or execution results.
7. If evidence conflicts, state the conflict and use the more conservative conclusion.
8. Deduplicate evidence and provide the final conclusion only once, with the conclusion first.
9. A descriptor validation failure means the model proposed a tool name outside the current catalog. An arguments validation failure means the model request did not satisfy the registered tool schema. If an authorized catalog tool can provide the requested capability, either failure must not be described as a missing platform capability or an undeployed module.
10. Write the user-facing answer in the same language as the user's request.`, p.formatToolListDetail())

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}
}

func genericAgentReasoningGuide() string {
	return `## Generic agent reasoning
1. Understand the final goal, objects, scope, constraints, and completion criteria before deciding whether tools or decomposition are needed.
2. The dynamic tool catalog is the capability boundary. Use only supplied tools and never apply a fixed workflow from names or keywords.
3. Treat IDs, objects, and states from actual prior results as candidate arguments. Re-evaluate the next action after every result.
4. Handle asynchronous state, pagination, conditional fallback, multiple targets, and verification dynamically from tool schemas, contracts, and results.
5. Write operations require explicit user intent and approval. Ask only when the target is unsafe or cannot be discovered from context or read-only tools.
6. The same tool may be called with different arguments, but plan titles must remain unique and successful identical calls must not be repeated.
7. If authorized tools cannot complete the goal, report the missing capability and available evidence. Never invent a tool or result.`
}

// formatToolList 格式化工具列表（简要版）
func (p *AssistantPromptProvider) formatToolList() string {
	var buf strings.Builder
	for _, desc := range p.toolDescriptors {
		buf.WriteString(fmt.Sprintf("- %s: %s\n", desc.Name, modelSafeRuntimeDescription(desc)))
	}
	return buf.String()
}

// formatToolListDetail 格式化工具列表（详细版，含参数）
func (p *AssistantPromptProvider) formatToolListDetail() string {
	var buf strings.Builder
	for _, desc := range p.toolDescriptors {
		buf.WriteString(fmt.Sprintf("- %s: %s", desc.Name, modelSafeRuntimeDescription(desc)))
		if desc.ArgsSchema != nil {
			if schema := modelSafeRuntimeArgsSchema(desc.ArgsSchema); len(schema) > 0 {
				if encoded, err := json.Marshal(schema); err == nil {
					buf.WriteString(" Input schema: ")
					buf.Write(encoded)
				}
			}
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

func modelSafeRuntimeDescription(desc agentruntime.ToolDescriptor) string {
	description := strings.TrimSpace(desc.Description)
	if description != "" && !containsHan(description) {
		return description
	}
	return fmt.Sprintf("Use %s according to its authorized argument and result contract.", desc.Name)
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func modelSafeRuntimeArgsSchema(schema map[string]interface{}) map[string]interface{} {
	sanitized, _ := sanitizeModelSchemaValue(schema).(map[string]interface{})
	return sanitized
}

func sanitizeModelSchemaValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		sanitized := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "description", "title", "$comment", "examples":
				continue
			default:
				sanitized[key] = sanitizeModelSchemaValue(item)
			}
		}
		return sanitized
	case []interface{}:
		sanitized := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			sanitized = append(sanitized, sanitizeModelSchemaValue(item))
		}
		return sanitized
	case []string:
		sanitized := make([]string, len(typed))
		copy(sanitized, typed)
		return sanitized
	default:
		return typed
	}
}

func (p *AssistantPromptProvider) formatReflectionGuide() string {
	if len(p.reflectionMemories) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("## Internal recovery reflections\n")
	buf.WriteString("These reflections come from previous tool or step failures in this session. Use them only to recover execution; do not repeat them to the user:\n")
	for i, memory := range p.reflectionMemories {
		memory = strings.TrimSpace(memory)
		if memory == "" {
			continue
		}
		buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, memory))
	}
	return strings.TrimSpace(buf.String())
}

func (p *AssistantPromptProvider) formatContextRefs() string {
	if len(p.contextRefs) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("## Attached context\n")
	buf.WriteString("These objects are bound to the session. When the task refers to a file, template, rule, or other attached object, use its ID and status first. Do not claim that an attached object is unavailable:\n")
	for _, ref := range p.contextRefs {
		buf.WriteString(fmt.Sprintf("- %s (%s, id=%s)", ref.Title, ref.ObjectType, ref.ObjectID))
		if strings.TrimSpace(ref.Summary) != "" {
			buf.WriteString(": ")
			buf.WriteString(strings.TrimSpace(ref.Summary))
		}
		if len(ref.Data) > 0 {
			encoded, err := json.Marshal(ref.Data)
			if err == nil && len(encoded) > 0 {
				data := string(encoded)
				if len([]rune(data)) > 1600 {
					data = string([]rune(data)[:1600]) + "..."
				}
				buf.WriteString("\n  data: ")
				buf.WriteString(data)
			}
		}
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}
