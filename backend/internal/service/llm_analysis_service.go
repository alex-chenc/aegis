package service

import (
	"context"
	"fmt"

	"aegis-system/internal/llm"
	"aegis-system/internal/pipeline"
	"aegis-system/pkg/logger"

	"go.uber.org/zap"
)

// LLMAnalysisService handles LLM calls for security event analysis
type LLMAnalysisService struct {
	client *llm.LLMClient
	logger *zap.Logger
}

// NewLLMAnalysisService creates a new LLM analysis service
func NewLLMAnalysisService(client *llm.LLMClient) *LLMAnalysisService {
	return &LLMAnalysisService{
		client: client,
		logger: logger.Logger,
	}
}

// Analyze performs LLM analysis on a host window with tool call loop
func (s *LLMAnalysisService) Analyze(ctx context.Context, window *pipeline.HostWindow) (*pipeline.LLMAnalysisOutput, error) {
	prompt, err := pipeline.BuildAnalysisPrompt(window)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	response, err := s.client.ChatCompletion(ctx, "", prompt, 0.7)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	output, err := pipeline.ParseLLMResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	toolCallCount := 0
	for len(output.ToolCalls) > 0 && toolCallCount < 10 {
		call := output.ToolCalls[0]
		output.ToolCalls = output.ToolCalls[1:]

		result, err := s.executeToolCall(ctx, window.HostID, call)
		if err != nil {
			s.logger.Error("tool call failed",
				zap.String("tool", call.Tool),
				zap.Error(err),
			)
			continue
		}

		prompt = pipeline.BuildToolResultPrompt(prompt, call.Tool, result)

		response, err = s.client.ChatCompletion(ctx, "", prompt, 0.7)
		if err != nil {
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		nextOutput, err := pipeline.ParseLLMResponse(response)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		output.Alerts = append(output.Alerts, nextOutput.Alerts...)
		output.ToolCalls = append(output.ToolCalls, nextOutput.ToolCalls...)
		output.RuleAdjustments = append(output.RuleAdjustments, nextOutput.RuleAdjustments...)

		toolCallCount++
	}

	s.logger.Info("analysis complete",
		zap.String("host_id", window.HostID),
		zap.Int("alerts", len(output.Alerts)),
		zap.Int("tool_calls", toolCallCount),
	)

	return output, nil
}

// executeToolCall executes a tool call via gRPC to the agent
func (s *LLMAnalysisService) executeToolCall(ctx context.Context, hostID string, call pipeline.ToolCallPayload) (string, error) {
	s.logger.Info("executing tool call",
		zap.String("host_id", hostID),
		zap.String("tool", call.Tool),
	)
	return fmt.Sprintf("Tool %s executed (placeholder)", call.Tool), nil
}
