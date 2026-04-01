package llm_analyzer

import (
	"context"
	"dc/internal/model"
	"dc/internal/pipeline"
	"dc/pkg/logger"

	"fmt"
	"go.uber.org/zap"
)

type LLMAnalyzer struct {
	promptBuilder  *pipeline.LLMPromptBuilder
	responseParser *pipeline.LLMResponseParser
	llmClient      LLMClientInterface
	logger         *zap.Logger
}

type LLMClientInterface interface {
	Analyze(ctx context.Context, prompt string) (string, error)
}

func NewLLMAnalyzer(client LLMClientInterface) *LLMAnalyzer {
	return &LLMAnalyzer{
		promptBuilder:  pipeline.NewLLMPromptBuilder(),
		responseParser: pipeline.NewLLMResponseParser(),
		llmClient:      client,
		logger:         logger.Get(),
	}
}

func (a *LLMAnalyzer) AnalyzeEvents(ctx context.Context, events []*model.RuntimeEvent) (*model.LLMAnalysisResult, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to analyze")
	}

	// Build prompt from events
	prompt := a.promptBuilder.Build(events)

	// If no LLM client is configured, return a default analysis
	if a.llmClient == nil {
		a.logger.Warn("No LLM client configured, returning default analysis")
		return &model.LLMAnalysisResult{
			Summary:      "LLM analysis not configured",
			Severity:     "medium",
			MitreTechnique: "",
			MatchedRule:  "",
			AnalysisDetails: "No LLM client configured",
		}, nil
	}

	// Call LLM
	response, err := a.llmClient.Analyze(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	// Parse response
	result, err := a.responseParser.Parse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return result, nil
}

func (a *LLMAnalyzer) SetLLMClient(client LLMClientInterface) {
	a.llmClient = client
}