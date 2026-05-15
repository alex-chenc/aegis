package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// AgentStep represents a step in the ReAct agent's reasoning
type AgentStep struct {
	Thought     string                 `json:"thought"`
	Action      string                 `json:"action"`
	ActionInput map[string]interface{} `json:"action_input"`
	Observation string                 `json:"observation"`
}

// AgentResponse represents the agent's response
type AgentResponse struct {
	Content   string      `json:"content"`
	Steps     []AgentStep `json:"steps"`
	ToolCalls []*ToolCall `json:"tool_calls"`
	SessionID string      `json:"session_id"`
}

// ToolCall represents a tool call made by the agent
type ToolCall struct {
	CallID    string                 `json:"call_id"`
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolExecutor is an interface for executing tools
type ToolExecutor interface {
	Execute(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error)
}

// ReActAgent implements a ReAct-style reasoning agent
type ReActAgent struct {
	llmClient     *LLMClient
	toolExecutor  ToolExecutor
	maxIterations int
	sessionID     string
	steps         []AgentStep
}

const (
	maxObservationChars             = 12000
	forceFinalAnswerAfterIterations = 50
	maxNoActionIterations           = 2
)

// NewReActAgent creates a new ReAct agent
func NewReActAgent(llmClient *LLMClient, toolExecutor ToolExecutor, sessionID string, maxIterations int) *ReActAgent {
	if maxIterations <= 0 {
		maxIterations = 15 // default value
	}
	return &ReActAgent{
		llmClient:     llmClient,
		toolExecutor:  toolExecutor,
		maxIterations: maxIterations,
		sessionID:     sessionID,
		steps:         make([]AgentStep, 0),
	}
}

// Invoke executes a single conversation turn (blocking version)
func (a *ReActAgent) Invoke(ctx context.Context, userMessage string, history []*AIMessage, context map[string]interface{}) (*AgentResponse, error) {
	prompt := BuildReActPrompt(userMessage, history, context)

	resp, err := a.llmClient.ChatCompletionWithMessages(ctx, prompt, 0.7)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// DEBUG: Log LLM response
	_zapLogger.Info("LLM response", zap.String("content", resp))

	// Parse ReAct output
	steps, finalAnswer := a.parseReActOutput(resp)

	// Execute tool calls
	for i := range steps {
		if steps[i].Action != "" {
			result, err := a.toolExecutor.Execute(ctx, steps[i].Action, steps[i].ActionInput)
			steps[i].Observation = a.formatObservation(result, err)
			a.steps = append(a.steps, steps[i])
		}
	}

	return &AgentResponse{
		Content:   finalAnswer,
		Steps:     steps,
		ToolCalls: a.extractToolCalls(steps),
		SessionID: a.sessionID,
	}, nil
}

// KnownTools lists all available tools that the agent can call
var KnownTools = []string{
	"GetProcessTree",
	"GetNetworkConnections",
	"GetOpenFiles",
	"GetRunningProcesses",
	"GetUserSessions",
	"QueryHistoricalLogs",
}

// normalizeToolName attempts to match a tool name to a known tool
// It handles partial matches, case variations, and common truncation patterns
func normalizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Direct match
	for _, tool := range KnownTools {
		if tool == name {
			return tool
		}
	}

	// Case-insensitive match
	lowerName := strings.ToLower(name)
	for _, tool := range KnownTools {
		if strings.ToLower(tool) == lowerName {
			return tool
		}
	}

	// Prefix match (handles truncated names like "Get" -> "GetProcessTree")
	for _, tool := range KnownTools {
		if strings.HasPrefix(strings.ToLower(tool), lowerName) {
			return tool
		}
	}

	// Substring match (handles cases like "ProcessTree" matching "GetProcessTree")
	for _, tool := range KnownTools {
		if strings.Contains(strings.ToLower(tool), lowerName) {
			return tool
		}
	}

	// Try to infer from common prefixes
	switch {
	case strings.HasPrefix(lowerName, "getprocess"):
		return "GetProcessTree"
	case strings.HasPrefix(lowerName, "getnetwork"):
		return "GetNetworkConnections"
	case strings.HasPrefix(lowerName, "getopen"):
		return "GetOpenFiles"
	case strings.HasPrefix(lowerName, "getrunning"):
		return "GetRunningProcesses"
	case strings.HasPrefix(lowerName, "getuser"):
		return "GetUserSessions"
	case strings.HasPrefix(lowerName, "query"):
		return "QueryHistoricalLogs"
	}

	// No match found.
	return ""
}

// parseReActOutput parses the ReAct format output from LLM
func (a *ReActAgent) parseReActOutput(content string) ([]AgentStep, string) {
	steps := []AgentStep{}
	var finalAnswer string

	lines := strings.Split(content, "\n")
	currentStep := &AgentStep{}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Thought:") {
			if currentStep.Thought != "" {
				steps = append(steps, *currentStep)
			}
			currentStep = &AgentStep{}
			currentStep.Thought = strings.TrimPrefix(line, "Thought:")
		} else if strings.HasPrefix(line, "Action:") {
			currentStep.Action = normalizeToolName(strings.TrimPrefix(line, "Action:"))
		} else if strings.HasPrefix(line, "Action Input:") {
			inputStr := strings.TrimPrefix(line, "Action Input:")
			var input map[string]interface{}
			json.Unmarshal([]byte(inputStr), &input)
			currentStep.ActionInput = input
		} else if strings.HasPrefix(line, "Observation:") {
			currentStep.Observation = strings.TrimPrefix(line, "Observation:")
		} else if strings.HasPrefix(line, "Final Answer:") {
			finalAnswer = strings.TrimPrefix(line, "Final Answer:")
			if currentStep.Thought != "" {
				steps = append(steps, *currentStep)
			}
		}
	}

	// If no Final Answer but we have steps with thoughts, return the last thought
	if finalAnswer == "" && len(steps) > 0 {
		for i := len(steps) - 1; i >= 0; i-- {
			if steps[i].Thought != "" {
				finalAnswer = "[中间推理结果]\n" + steps[i].Thought
				if steps[i].Observation != "" {
					finalAnswer += "\n\n[工具执行结果]\n" + steps[i].Observation
				}
				break
			}
		}
	}

	// If still no answer, return the raw content
	if finalAnswer == "" {
		finalAnswer = content
	}

	return steps, finalAnswer
}

// extractToolCalls extracts tool calls from steps
func (a *ReActAgent) extractToolCalls(steps []AgentStep) []*ToolCall {
	calls := []*ToolCall{}
	for _, step := range steps {
		if step.Action != "" {
			calls = append(calls, &ToolCall{
				CallID:    generateCallID(),
				Tool:      step.Action,
				Arguments: step.ActionInput,
			})
		}
	}
	return calls
}

// formatObservation formats a tool result into a string
func (a *ReActAgent) formatObservation(result interface{}, err error) string {
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	data, _ := json.Marshal(result)
	observation := string(data)
	if len(observation) <= maxObservationChars {
		return observation
	}
	return observation[:maxObservationChars] + fmt.Sprintf("\n... [truncated %d chars; refine the next tool query/filter if more detail is needed]", len(observation)-maxObservationChars)
}

var _zapLogger *zap.Logger

func init() {
	_zapLogger, _ = zap.NewDevelopment()
}
