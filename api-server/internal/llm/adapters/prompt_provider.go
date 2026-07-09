package adapters

import (
	"context"
	"fmt"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
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
			systemPrompt += "\n\n## Relevant historical experience\n" + formatExperienceForPrompt(expResp.Items)
		}
	}

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
		Messages: []agentruntime.LLMMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: "Create a detailed security-analysis plan from the instructions and alert context."},
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
			systemPrompt += "\n\n## Relevant historical experience\n" + formatExperienceForPrompt(expResp.Items)
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

const planPromptTemplate = `You are a security-analysis assistant planning an investigation of a host security alert.

## Available tools
- GetProcessTree: retrieve a process tree for a host; pid is optional and defaults to 1.
- GetNetworkConnections: retrieve network connections for a host.
- GetOpenFiles: retrieve files opened by a process.
- GetRunningProcesses: retrieve running processes for a host.
- GetUserSessions: retrieve active user sessions for a host.
- QueryHistoricalLogs: query historical logs for a host.

## Output
Return JSON only:
{
  "goal": "analysis goal",
  "assumptions": ["assumption"],
  "steps": [
    {
      "step_id": "step_1",
      "title": "step title",
      "objective": "step objective",
      "expected_output": "expected output",
      "suggested_tools": ["ToolName1", "ToolName2"]
    }
  ]
}

Use only listed tools. Base the plan on actual alert context and keep user-facing natural language in the user's language.`

const reactJSONPromptTemplate = `You are a security-analysis assistant executing one plan step.

## Available tools
Use only these exact names:
- GetProcessTree: {"host_id":"host ID","pid":"optional process ID, default 1"}
- GetNetworkConnections: {"host_id":"host ID","pid":"optional process ID"}
- GetOpenFiles: {"host_id":"host ID","pid":"process ID"}
- GetRunningProcesses: {"host_id":"host ID","filter":"optional filter"}
- GetUserSessions: {"host_id":"host ID"}
- QueryHistoricalLogs: {"host_id":"host ID","start_time":"start time","end_time":"end time","filter":"optional filter"}

## Output protocol
Return one JSON object only.

Tool call:
{"action":"tool_call","summary":"concise purpose","tool_call":{"tool_name":"one exact available tool","reason":"reason","args":{}}}

Completed step:
{"action":"step_result","summary":"completion summary","step_result":{"result":"step result","evidence":["evidence"],"confidence":"high|medium|low"}}

Cannot continue:
{"action":"fail_step","summary":"failure summary","failure":{"reason":"failure reason","recoverable":true}}`

const summarizePromptTemplate = `You are a security-analysis assistant generating the final report from collected evidence.

Return strict JSON:
{
  "attack_graph": {
    "nodes": [{"id":"n1","label":"node label","type":"process|file|network|user","details":{}}],
    "edges": [{"from":"n1","to":"n2","label":"relationship","type":"creates|connects|accesses|executes"}],
    "timeline": [{"time":"timestamp","event":"event description","severity":"low|medium|high|critical"}],
    "threat_level": "low|medium|high|critical",
    "recommendations": ["recommendation"]
  },
  "conclusions": [
    {"alert_id":"alert ID","action":"mark_false_positive|confirm_threat|generate_rule","summary":"analysis conclusion in the user's language"}
  ]
}

Rules:
- conclusions[].alert_id must use the alert_id or original alert ID from context.
- conclusions[].action must be mark_false_positive, confirm_threat, or generate_rule.
- Never claim that an unexecuted plan step completed.`

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
		fmt.Fprintf(&buf, "### Experience %d: %s\n%s", i+1, item.Summary, item.Content)
	}
	return buf.String()
}
