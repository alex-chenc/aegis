package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"
	"github.com/google/uuid"
)

type fakeAgentConversationService struct {
	result service.AgentSessionListResult
}

func (f fakeAgentConversationService) List(context.Context, *uuid.UUID, string, string, int, int) (service.AgentSessionListResult, error) {
	return f.result, nil
}

func (fakeAgentConversationService) Detail(context.Context, uuid.UUID, bool) (*model.AgentConversationSession, []model.AgentConversationItem, error) {
	return nil, nil, nil
}

func (fakeAgentConversationService) RequestCollection(context.Context, uuid.UUID, string) (service.AgentSessionCollectionResult, error) {
	return service.AgentSessionCollectionResult{}, nil
}

func (fakeAgentConversationService) Analyze(context.Context, uuid.UUID) (service.AgentSessionAIResult, error) {
	return service.AgentSessionAIResult{}, nil
}

func (fakeAgentConversationService) GetAIAnalysis(context.Context, uuid.UUID) (service.AgentSessionAIResult, error) {
	return service.AgentSessionAIResult{}, nil
}

func TestAgentGuardToolsUseHighLevelExposureAndHidePolicyWrites(t *testing.T) {
	registry := assistant.NewToolRegistry()
	if err := RegisterAgentGuardTools(registry, AgentGuardToolDeps{}); err != nil {
		t.Fatalf("register agent guard tools: %v", err)
	}
	if err := registry.ValidateModelFacingEnglish(); err != nil {
		t.Fatalf("model-facing contract: %v", err)
	}
	for _, name := range []string{
		"AgentGuard.Posture.Assess", "AgentGuard.Scope.Investigate", "AgentGuard.Configuration.Assess",
		"AgentGuard.Finding.Analyze", "AgentGuard.ExecutionUnit.Freeze", "AgentGuard.RuntimeSettings.Update",
	} {
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("expected tool %s", name)
		}
		if !tool.ExposurePolicy.DirectCallable {
			t.Fatalf("high-level tool %s must be directly callable through the guarded gateway", name)
		}
	}
	if _, ok := registry.Get("AgentGuard.Policy.Update"); ok {
		t.Fatal("raw Agent Guard policy write must not be model-facing")
	}
	if tool, _ := registry.Get("AgentGuard.Finding.Analyze"); !tool.RequiresApproval || tool.Risk != assistant.ToolRiskMedium {
		t.Fatal("finding analysis must remain approval-gated and medium risk")
	}
	if tool, _ := registry.Get("AgentGuard.ExecutionUnit.Kill"); !tool.RequiresApproval || tool.Risk != assistant.ToolRiskHigh {
		t.Fatal("execution-unit kill must remain approval-gated and high risk")
	}
}

func TestAgentGuardExposureDoesNotLeakInternalPolicyTools(t *testing.T) {
	registry := assistant.NewToolRegistry()
	if err := RegisterAgentGuardTools(registry, AgentGuardToolDeps{}); err != nil {
		t.Fatalf("register agent guard tools: %v", err)
	}
	resolver := assistant.NewToolExposureResolver(registry)
	catalog := resolver.IntentCatalog(assistant.ToolExposureContext{Domains: []string{"agent_guard"}})
	for _, tool := range catalog {
		if tool.Domain != assistant.DomainAgentGuard {
			continue
		}
		if tool.ExposurePolicy.Exposure == assistant.ToolExposureInternal {
			t.Fatalf("internal tool %s leaked into intent catalog", tool.Name)
		}
		if tool.Name == "AgentGuard.RuntimeSettings.Update" && tool.Risk != assistant.ToolRiskHigh {
			t.Fatalf("runtime settings update risk was weakened")
		}
	}
}

func TestAgentConversationQueryPutsPaginationMetadataBeforeCompactItems(t *testing.T) {
	firstID, secondID := uuid.New(), uuid.New()
	serviceResult := service.AgentSessionListResult{
		Items: []model.AgentConversationSession{
			{ID: firstID, HostID: uuid.New(), AgentType: model.AgentSessionSourceCodex, ExternalSessionID: "codex-1", State: model.AgentSessionStateActive, RiskLevel: model.AgentSessionRiskHigh, RuleHitCount: 7, ItemCount: 339},
			{ID: secondID, HostID: uuid.New(), AgentType: model.AgentSessionSourceCodex, ExternalSessionID: "codex-2", State: model.AgentSessionStateDone, RiskLevel: model.AgentSessionRiskLow, RuleHitCount: 0, ItemCount: 14},
		},
		Total:    51,
		Page:     1,
		PageSize: 50,
	}
	handler := makeConversationQueryHandler(fakeAgentConversationService{result: serviceResult})
	data, err := handler(context.Background(), map[string]interface{}{"page": 1, "page_size": 50, "agent_type": model.AgentSessionSourceCodex})
	if err != nil {
		t.Fatalf("query handler returned error: %v", err)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal query result: %v", err)
	}
	encoded := string(payload)
	metadata := `{"page":1,"page_size":50,"returned_count":2,"total":51,"total_pages":2,"has_next_page":true,"has_previous_page":false,"risk_summary":{"high_risk_count":1,"active_high_risk_count":1,"unknown_risk_count":0,"risk_types_available":false,"high_risk_sessions":[{"id":"` + firstID.String() + `","state":"active_inferred","rule_hit_count":7,"item_count":339}]}`
	if !strings.HasPrefix(encoded, metadata) {
		t.Fatalf("pagination metadata must be first, got %s", encoded[:minTestStringLen(len(encoded), 220)])
	}
	if !strings.Contains(encoded, `"id":"`+firstID.String()+`"`) || !strings.Contains(encoded, `"id":"`+secondID.String()+`"`) {
		t.Fatalf("compact session summaries missing from result: %s", encoded)
	}
	if strings.Contains(encoded, "normalized_json") || strings.Contains(encoded, "content_redacted") {
		t.Fatalf("conversation query leaked detail/content fields: %s", encoded)
	}
}

func minTestStringLen(value, max int) int {
	if value < max {
		return value
	}
	return max
}
