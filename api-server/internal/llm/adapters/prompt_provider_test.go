package adapters

import (
	"context"
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
