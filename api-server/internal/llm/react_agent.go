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
	ToolCalls  []*ToolCall  `json:"tool_calls"`
	SessionID  string       `json:"session_id"`
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
		sessionID:    sessionID,
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
func (a *ReActAgent) Stream(ctx context.Context, userMessage string, history []*AIMessage, writer *SSEWriter, context map[string]interface{}) error {
	prompt := BuildReActPrompt(userMessage, history, context)

	// Use streaming LLM call
	stream, err := a.llmClient.ChatCompletionStreamWithMessages(ctx, prompt, 0.7)
	if err != nil {
		writer.WriteError(fmt.Sprintf("LLM stream failed: %v", err))
		return err
	}
	defer stream.Close()

	currentStep := &AgentStep{}
	buffer := ""

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			writer.WriteError(fmt.Sprintf("stream error: %v", err))
			return err
		}

		// DEBUG: Log chunk
		_zapLogger.Info("stream chunk", zap.String("content", chunk.Content), zap.Bool("done", chunk.Done))

		buffer += chunk.Content

		// Try to parse a complete step
		if step, done := a.tryParseStep(buffer); done {
			currentStep = step

			if step.Action != "" {
				// Send tool call event
				callID := generateCallID()
				writer.WriteToolCall(step.Action, callID, step.ActionInput)

				// Execute tool
				start := time.Now()
				result, err := a.toolExecutor.Execute(ctx, step.Action, step.ActionInput)
				elapsed := time.Since(start).Milliseconds()

				if err != nil {
					writer.WriteToolError(callID, err.Error())
					currentStep.Observation = fmt.Sprintf("Error: %v", err)
				} else {
					writer.WriteToolResult(callID, result, elapsed)
					currentStep.Observation = a.formatObservation(result, nil)
				}

				a.steps = append(a.steps, *currentStep)
				currentStep = &AgentStep{}
			} else if step.Thought != "" {
				// Pure thought, send as thinking
				writer.WriteThinking(step.Thought)
			}

			buffer = ""
		} else {
			// Still parsing, send thinking
			if buffer != "" {
				writer.WriteThinking(buffer)
			}
		}
	}

	// Parse final answer
	_, finalAnswer := a.parseFinalAnswer(buffer)
	writer.WriteContent(finalAnswer)
	writer.WriteDone()

	return nil
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
			currentStep.Action = strings.TrimPrefix(line, "Action:")
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

// tryParseStep attempts to parse a complete step from the buffer
func (a *ReActAgent) tryParseStep(buffer string) (*AgentStep, bool) {
	step := &AgentStep{}
	lines := strings.Split(buffer, "\n")
	foundAction := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Thought:") {
			step.Thought = strings.TrimPrefix(line, "Thought:")
		} else if strings.HasPrefix(line, "Action:") {
			step.Action = strings.TrimPrefix(line, "Action:")
			foundAction = true
		} else if strings.HasPrefix(line, "Action Input:") {
			inputStr := strings.TrimPrefix(line, "Action Input:")
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(inputStr), &input); err == nil {
				step.ActionInput = input
			}
		} else if strings.HasPrefix(line, "Observation:") {
			step.Observation = strings.TrimPrefix(line, "Observation:")
			return step, true
		} else if strings.HasPrefix(line, "Final Answer:") {
			return step, true
		}
	}

	if foundAction && step.Action != "" {
		return step, true
	}

	return nil, false
}

// parseFinalAnswer extracts the final answer from content
func (a *ReActAgent) parseFinalAnswer(content string) (*AgentStep, string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Final Answer:") {
			return nil, strings.TrimPrefix(line, "Final Answer:")
		}
	}
	return nil, content
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
