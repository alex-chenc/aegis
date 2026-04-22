package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

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
	Content    string       `json:"content"`
	Steps      []AgentStep  `json:"steps"`
	ToolCalls  []*ToolCall `json:"tool_calls"`
	SessionID  string      `json:"session_id"`
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

// NewReActAgent creates a new ReAct agent
func NewReActAgent(llmClient *LLMClient, toolExecutor ToolExecutor, sessionID string) *ReActAgent {
	return &ReActAgent{
		llmClient:     llmClient,
		toolExecutor:  toolExecutor,
		maxIterations: 10,
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

// Stream executes with SSE streaming output
// This implements the full ReAct loop: think -> action -> observe -> think -> ... -> final answer
func (a *ReActAgent) Stream(ctx context.Context, userMessage string, history []*AIMessage, writer *SSEWriter, context map[string]interface{}) error {
	// Build initial prompt with full context
	prompt := BuildReActPrompt(userMessage, history, context)

	iteration := 0
	maxIterations := a.maxIterations

	// ReAct loop: continue until we get a Final Answer or hit max iterations
	for iteration < maxIterations {
		iteration++
		_zapLogger.Info("ReAct iteration started", zap.Int("iteration", iteration), zap.Int("max", maxIterations))

		// Use streaming LLM call for this iteration
		stream, err := a.llmClient.ChatCompletionStreamWithMessages(ctx, prompt, 0.7)
		if err != nil {
			writer.WriteError(fmt.Sprintf("LLM stream failed: %v", err))
			return err
		}

		currentStep := &AgentStep{}
		buffer := ""
		pendingThinking := ""
		hasAction := false
		actionExecuted := false

		// Process streaming response
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				// Stream ended - send accumulated thinking
				if pendingThinking != "" && !hasAction {
					writer.WriteThinking(pendingThinking)
				}
				break
			}
			if err != nil {
				writer.WriteError(fmt.Sprintf("stream error: %v", err))
				stream.Close()
				return err
			}

			buffer += chunk.Content
			pendingThinking += chunk.Content

			// Try to parse a complete step
			if step, done := a.tryParseStep(buffer); done {
				hasAction = step.Action != "" || step.ActionInput != nil

				// Check if step contains Final Answer - if so, skip action execution
				if strings.Contains(buffer, "Final Answer:") {
					// Flush any pending thinking
					if pendingThinking != "" {
						writer.WriteThinking(pendingThinking)
					}
					// Parse and return the final answer
					_, finalAnswer := a.parseFinalAnswer(buffer)
					if finalAnswer != "" {
						writer.WriteContent(finalAnswer)
					}
					writer.WriteDone()
					stream.Close()
					return nil
				}

				if hasAction && !actionExecuted {
					// Action found - flush thinking and execute
					if pendingThinking != "" {
						writer.WriteThinking(pendingThinking)
						pendingThinking = ""
					}

					currentStep = step
					actionName := strings.TrimSpace(step.Action)
					if actionName == "" {
						actionName = "InferredAction"
					}

					// Write tool call event
					callID := generateCallID()
					writer.WriteToolCall(actionName, callID, step.ActionInput)

					// Execute tool
					start := time.Now()
					result, err := a.toolExecutor.Execute(ctx, actionName, step.ActionInput)
					elapsed := time.Since(start).Milliseconds()

					if err != nil {
						writer.WriteToolError(callID, err.Error())
						currentStep.Observation = fmt.Sprintf("Error: %v", err)
					} else {
						writer.WriteToolResult(callID, result, elapsed)
						currentStep.Observation = a.formatObservation(result, nil)
					}

					a.steps = append(a.steps, *currentStep)
					actionExecuted = true

					// Add tool result to prompt for next iteration
					prompt = append(prompt, Message{
						Role:    "user",
						Content: fmt.Sprintf("Observation: %s", currentStep.Observation),
					})

					currentStep = &AgentStep{}
				} else if step.Thought != "" && !hasAction {
					// Only thought, no action yet - accumulate
					pendingThinking = step.Thought
				}

				buffer = ""
			} else if chunk.Done || err != nil {
				// Stream ending with incomplete buffer
				if pendingThinking != "" && !hasAction {
					writer.WriteThinking(pendingThinking)
					pendingThinking = ""
				}
				break
			} else if len(pendingThinking) >= 100 && !hasAction {
				// Send periodic thinking updates to avoid choppy display
				writer.WriteThinking(pendingThinking)
				pendingThinking = ""
			}
		}

		stream.Close()

		// Check if we have a final answer (no action was executed in this iteration)
		if !actionExecuted {
			_, finalAnswer := a.parseFinalAnswer(buffer)
			if finalAnswer != "" {
				writer.WriteContent(finalAnswer)
				writer.WriteDone()
				return nil
			}
		}

		// If we executed an action, continue to next iteration
		_zapLogger.Info("ReAct iteration completed, continuing loop",
			zap.Int("iteration", iteration),
			zap.Bool("action_executed", actionExecuted))
	}

	// Max iterations reached
	writer.WriteError("Maximum iterations reached without final answer")
	writer.WriteDone()
	return nil
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

	// No match found - return original
	return name
}

// tryParseStep attempts to parse a complete step from the buffer
func (a *ReActAgent) tryParseStep(buffer string) (*AgentStep, bool) {
	step := &AgentStep{}
	lines := strings.Split(buffer, "\n")
	foundAction := false
	foundThought := false
	foundActionInput := false

	// First, check if buffer contains a complete JSON object (no Thought:/Action: prefixes)
	// This handles LLM output like: {"query": "...", "time_range": {...}}
	trimmed := strings.TrimSpace(buffer)
	if strings.HasPrefix(trimmed, "{") {
		// Check if it's a complete JSON object (ends with })
		if idx := strings.LastIndex(trimmed, "}"); idx >= 0 {
			jsonStr := trimmed[:idx+1]
			if json.Unmarshal([]byte(jsonStr), &step.ActionInput) == nil {
				// JSON parsed successfully - infer tool name from keys
				if _, hasQuery := step.ActionInput["query"]; hasQuery {
					step.Action = "QueryHistoricalLogs"
					return step, true
				}
				if _, hasPid := step.ActionInput["pid"]; hasPid {
					step.Action = "GetProcessTree"
					return step, true
				}
				if _, hasHostID := step.ActionInput["host_id"]; hasHostID {
					step.Action = "GetRunningProcesses"
					return step, true
				}
				if _, hasProcessName := step.ActionInput["process_name"]; hasProcessName {
					step.Action = "GetOpenFiles"
					return step, true
				}
				// Unknown JSON structure, treat as action input
				return step, true
			}
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Thought:") {
			step.Thought = strings.TrimPrefix(line, "Thought:")
			foundThought = true
		} else if strings.HasPrefix(line, "Action:") {
			step.Action = normalizeToolName(strings.TrimPrefix(line, "Action:"))
			foundAction = true
		} else if strings.HasPrefix(line, "Action Input:") {
			inputStr := strings.TrimPrefix(line, "Action Input:")
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(inputStr), &input); err == nil {
				step.ActionInput = input
				foundActionInput = true
			}
			// Only return complete step if we have both Action and ActionInput
			if foundAction && step.Action != "" && foundActionInput {
				return step, true
			}
		} else if strings.HasPrefix(line, "Observation:") {
			step.Observation = strings.TrimPrefix(line, "Observation:")
			return step, true
		} else if strings.HasPrefix(line, "Final Answer:") {
			return step, true
		}
	}

	// Only return true for Action if we have BOTH Action and ActionInput
	if foundAction && step.Action != "" && foundActionInput {
		return step, true
	}

	// If we have a thought and the buffer ends with "}", it might be JSON Action Input
	if foundThought && foundAction && step.Action != "" {
		// Try to parse remaining as JSON
		remaining := strings.TrimSpace(buffer)
		if idx := strings.LastIndex(remaining, "}"); idx >= 0 {
			jsonStr := remaining[strings.Index(remaining, "{"):idx+1]
			if jsonStr != "" {
				var input map[string]interface{}
				if err := json.Unmarshal([]byte(jsonStr), &input); err == nil {
					step.ActionInput = input
					return step, true
				}
			}
		}
	}

	return nil, false
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

// parseFinalAnswer extracts the final answer from content and parses attack_graph if present
func (a *ReActAgent) parseFinalAnswer(content string) (*AgentStep, string) {
	lines := strings.Split(content, "\n")
	var finalAnswer string
	var jsonPart string
	inJsonBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Final Answer:") {
			finalAnswer = strings.TrimPrefix(line, "Final Answer:")
			// Check if Final Answer is immediately followed by JSON
			if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(content, "Final Answer:")), "{") {
				jsonPart = strings.TrimSpace(strings.TrimPrefix(content, "Final Answer:"))
				inJsonBlock = true
			}
		} else if inJsonBlock {
			jsonPart += "\n" + line
		}
	}

	// If we have JSON, try to parse and pretty-print it
	if jsonPart != "" {
		// Try to find the JSON boundaries
		jsonStr := strings.TrimSpace(jsonPart)
		startIdx := strings.Index(jsonStr, "{")
		endIdx := strings.LastIndex(jsonStr, "}")
		if startIdx >= 0 && endIdx >= startIdx {
			jsonStr = jsonStr[startIdx : endIdx+1]
			// Validate JSON by trying to parse it
			var jsonData map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &jsonData); err == nil {
				// Pretty print the JSON
				prettyJSON, _ := json.MarshalIndent(jsonData, "", "  ")
				return nil, string(prettyJSON)
			}
		}
	}

	if finalAnswer == "" {
		finalAnswer = content
	}
	return nil, finalAnswer
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
	return string(data)
}

// GetSteps returns the steps taken so far
func (a *ReActAgent) GetSteps() []AgentStep {
	return a.steps
}

// ClearSteps clears the steps
func (a *ReActAgent) ClearSteps() {
	a.steps = make([]AgentStep, 0)
}

var _zapLogger *zap.Logger

func init() {
	_zapLogger, _ = zap.NewDevelopment()
}
