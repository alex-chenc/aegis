package adapters

import (
	"context"
	"strings"
	"testing"

	agentruntime "github.com/chenchen511/agent-runtime"
)

func TestBuildPlanPrompt_ReturnsMessages(t *testing.T) {
	alertCtx := map[string]interface{}{
		"rule_name": "test_rule",
		"host_id":   "host-1",
	}
	provider := NewAegisPromptProvider(alertCtx, nil)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposePlan,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bundle.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}

	if len(bundle.Messages) == 0 {
		t.Fatal("Messages should not be empty, got 0 messages")
	}

	hasSystem := false
	hasUser := false
	for _, m := range bundle.Messages {
		switch m.Role {
		case "system":
			hasSystem = true
		case "user":
			hasUser = true
		}
	}
	if !hasSystem {
		t.Error("Messages must contain a system message")
	}
	if !hasUser {
		t.Error("Messages must contain a user message")
	}
}

func TestBuildPlanPrompt_SystemPromptInMessages(t *testing.T) {
	provider := NewAegisPromptProvider(nil, nil)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposePlan,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, m := range bundle.Messages {
		if m.Role == "system" && m.Content != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Messages must contain a system message with plan prompt content")
	}
}

func TestBuildReactPrompt_ReturnsSystemPrompt(t *testing.T) {
	provider := NewAegisPromptProvider(nil, nil)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposeReact,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bundle.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty for react purpose")
	}
}

func TestBuildSummarizePrompt_ReturnsSystemPrompt(t *testing.T) {
	provider := NewAegisPromptProvider(nil, nil)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposeSummarize,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bundle.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty for summarize purpose")
	}
	if !strings.Contains(bundle.SystemPrompt, `"alert_id"`) ||
		!strings.Contains(bundle.SystemPrompt, `"action"`) ||
		!strings.Contains(bundle.SystemPrompt, `"summary"`) {
		t.Fatalf("summarize prompt must preserve backend conclusion schema, got: %s", bundle.SystemPrompt)
	}
}

func TestBuildReactPrompt_ExplicitlyListsAllToolNames(t *testing.T) {
	provider := NewAegisPromptProvider(nil, nil)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposeReact,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTools := []string{
		"GetProcessTree",
		"GetNetworkConnections",
		"GetOpenFiles",
		"GetRunningProcesses",
		"GetUserSessions",
		"QueryHistoricalLogs",
	}
	for _, tool := range expectedTools {
		if !strings.Contains(bundle.SystemPrompt, tool) {
			t.Errorf("react prompt must explicitly list tool %q, but it was not found", tool)
		}
	}

	// Verify correct action format matches agent-runtime parser
	if !strings.Contains(bundle.SystemPrompt, `"action":"step_result"`) {
		t.Error("react prompt must use step_result (not step_complete)")
	}
	if !strings.Contains(bundle.SystemPrompt, `"action":"tool_call"`) {
		t.Error("react prompt must include tool_call action format")
	}
	if !strings.Contains(bundle.SystemPrompt, `"action":"fail_step"`) {
		t.Error("react prompt must include fail_step action format")
	}
	if !strings.Contains(bundle.SystemPrompt, `"confidence"`) {
		t.Error("react prompt must include confidence field in step_result")
	}
	// Must NOT say "可用工具同规划阶段" which caused LLM to hallucinate tool names
	if strings.Contains(bundle.SystemPrompt, "可用工具同规划阶段") {
		t.Error("react prompt must NOT contain '可用工具同规划阶段' — tools must be explicitly listed")
	}
}

func TestBuildPlanPrompt_MessagesHaveValidRoles(t *testing.T) {
	alertCtx := map[string]interface{}{
		"rule_name": "Linux Base64 Encoded Pipe to Shell",
		"host_id":   "test-host",
	}
	provider := NewAegisPromptProvider(alertCtx, nil)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposePlan,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	validRoles := map[string]bool{"system": true, "user": true, "assistant": true}
	for i, m := range bundle.Messages {
		if !validRoles[m.Role] {
			t.Errorf("message[%d] has invalid role %q", i, m.Role)
		}
		if m.Content == "" {
			t.Errorf("message[%d] has empty content", i)
		}
	}
}

func TestBuildReactPrompt_IncludesExperience(t *testing.T) {
	querier := &mockReflectionQuerier{
		reflections: []AgentReflectionSummary{
			{ReflectionID: "refl-1", RootCause: "timeout", Impact: "step failed", ReusableLesson: "set timeout"},
		},
	}
	expProvider := NewExperienceProviderAdapter(nil, querier, 5)

	provider := NewAegisPromptProvider(
		map[string]interface{}{"rule_name": "test_rule"},
		expProvider,
	)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposeReact,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(bundle.SystemPrompt, "历史经验参考") {
		t.Error("react prompt should contain historical experience section when experience provider is configured")
	}
	if !strings.Contains(bundle.SystemPrompt, "timeout") {
		t.Error("react prompt should contain reflection root cause from experience")
	}
}

func TestBuildReactPrompt_NoExperienceWhenProviderNil(t *testing.T) {
	provider := NewAegisPromptProvider(nil, nil)

	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{
		TaskID:  "test-task",
		Purpose: agentruntime.PurposeReact,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(bundle.SystemPrompt, "历史经验参考") {
		t.Error("react prompt should NOT contain experience section when no provider is configured")
	}
}
