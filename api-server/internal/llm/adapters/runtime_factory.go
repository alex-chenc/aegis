package adapters

import (
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"

	"api-server/internal/grpc"
	"api-server/internal/llm"
)

// NewAegisRuntime assembles all adapters and creates an agent-runtime Runtime
// instance configured for Aegis host security analysis tasks.
//
// Parameters:
//   - llmClient: the api-server LLM client for model calls
//   - serverClient: the gRPC client for agent communication
//   - sseWriter: the SSE writer for streaming events to the frontend
//   - collector: event collector for hook-based event forwarding
//   - defaultHostIDs: fallback host IDs when tools do not specify one
//   - alertCtx: alert context injected into LLM prompts (may be nil)
//   - maxIterations: max total turns; <= 0 uses default of 500
//   - experienceProvider: optional experience provider (may be nil)
func NewAegisRuntime(
	llmClient *llm.LLMClient,
	serverClient *grpc.ServerClient,
	sseWriter *llm.SSEWriter,
	collector EventCollector,
	defaultHostIDs []string,
	alertCtx map[string]interface{},
	maxIterations int,
	experienceProvider agentruntime.ExperienceProvider,
) (*agentruntime.Runtime, error) {
	// 1. Create LLM adapter
	llmAdapter := NewLLMClientAdapter(llmClient, alertCtx)

	// 2. Create tool gateway adapter
	toolAdapter := NewToolGatewayAdapter(serverClient, defaultHostIDs)

	// 3. Create SSE hook sink
	hookSink := NewSSEHookSink(sseWriter, collector)

	// 4. Create prompt provider
	promptProvider := NewAegisPromptProvider(alertCtx, experienceProvider)

	// 5. Build runtime config
	if maxIterations <= 0 {
		maxIterations = 500
	}

	config := agentruntime.RuntimeConfig{
		MaxTotalTurns:         maxIterations,
		MaxPlanSteps:          8,
		MaxStepReactTurns:     8,
		MaxToolCalls:          100,
		MaxToolCallsPerStep:   10,
		MaxToolFailures:       15,
		MaxModelFailures:      5,
		MaxParseFailures:      3,
		MaxNoProgressTurns:    3,
		TaskTimeout:           2 * time.Hour,
		ModelTimeout:          1200 * time.Second,
		ToolTimeout:           60 * time.Second,
		HookTimeout:           10 * time.Second,
		EnableReflection:      true,
		EnableAudit:           true,
		EnableCorrection:      true,
		EnableExperience:      experienceProvider != nil,
		AuditEveryNSteps:      3,
		MaxAudits:             2,
		MaxReflections:        3,
		MaxStepRetries:        2,
		MaxCorrections:        2,
		AllowDynamicNewSteps:  true,
		AllowSkipFailedStep:   true,
		AllowBestEffortAnswer: true,
		AllowHighRiskTools:    false,
		AllowDangerousTools:   false,
		// Context budget and progressive compression
		MaxContextTokens:      256000,
		ReservedOutputTokens:  8192,
		EnableContextCompress: true,
		ToolCompressRatio:     0.70,
		StepCompressRatio:     0.80,
		LLMCompressRatio:      0.95,
		CompressTargetRatio:   0.60,
		RecentTurnsToKeep:     6,
	}

	// 6. Build options and create runtime
	opts := []agentruntime.Option{
		agentruntime.WithLLMClient(llmAdapter),
		agentruntime.WithToolGateway(toolAdapter),
		agentruntime.WithTools(AegisTools),
		agentruntime.WithHooks(hookSink),
		agentruntime.WithPromptProvider(promptProvider),
		agentruntime.WithConfig(config),
	}

	if experienceProvider != nil {
		opts = append(opts, agentruntime.WithExperienceProvider(experienceProvider))
	}

	return agentruntime.New(opts...)
}
