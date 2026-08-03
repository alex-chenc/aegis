package service

import (
	"context"
	"errors"
	"testing"

	"api-server/internal/model"

	"github.com/google/uuid"
)

type agentGuardCatalogStub struct {
	rules map[string]*model.AgentBehaviorRuleDefinition
}

func (s agentGuardCatalogStub) GetRule(
	_ context.Context,
	key string,
	version int64,
) (*model.AgentBehaviorRuleDefinition, error) {
	rule, ok := s.rules[key]
	if !ok || rule.RuleVersion != version {
		return nil, errors.New("not found")
	}
	return rule, nil
}

type agentGuardPolicyStoreStub struct {
	created *model.AgentGuardPolicy
}

func (s *agentGuardPolicyStoreStub) CreateDraft(_ context.Context, policy *model.AgentGuardPolicy) error {
	s.created = policy
	return nil
}

func (s *agentGuardPolicyStoreStub) GetByID(_ context.Context, id uuid.UUID) (*model.AgentGuardPolicy, error) {
	if s.created == nil || s.created.ID != id {
		return nil, errors.New("not found")
	}
	return s.created, nil
}

func (s *agentGuardPolicyStoreStub) UpdateDraft(
	_ context.Context,
	_ uuid.UUID,
	update model.AgentGuardPolicyDraftUpdate,
) (*model.AgentGuardPolicy, error) {
	s.created.Name = update.Name
	s.created.Digest = update.Digest
	return s.created, nil
}

func TestAgentGuardPolicyValidationCompilesStablePreview(t *testing.T) {
	catalog := agentGuardCatalogStub{rules: map[string]*model.AgentBehaviorRuleDefinition{
		model.AgentGuardRuleKeySensitiveDirectory: {
			RuleKey:          model.AgentGuardRuleKeySensitiveDirectory,
			RuleVersion:      1,
			Digest:           "sha256:definition",
			ParametersSchema: []byte(`{"type":"object","additionalProperties":false,"properties":{"resource_groups":{"type":"array","items":{"enum":["credential"]}}}}`),
		},
	}}
	service := NewAgentGuardPolicyService(catalog, &agentGuardPolicyStoreStub{}, true)

	first := service.Validate(context.Background(), validAgentGuardPolicyRequest())
	second := service.Validate(context.Background(), validAgentGuardPolicyRequest())
	if !first.Valid || len(first.Errors) != 0 {
		t.Fatalf("valid policy rejected: %#v", first.Errors)
	}
	if first.Digest == "" || first.Digest != second.Digest {
		t.Fatalf("digest is not stable: first=%q second=%q", first.Digest, second.Digest)
	}
	if first.DefinitionDigests[model.AgentGuardRuleKeySensitiveDirectory+"@1"] != "sha256:definition" {
		t.Fatalf("missing definition digest: %#v", first.DefinitionDigests)
	}
}

func TestAgentGuardPolicyValidationRejectsUnsafeInputs(t *testing.T) {
	service := NewAgentGuardPolicyService(agentGuardCatalogStub{}, &agentGuardPolicyStoreStub{}, true)
	request := validAgentGuardPolicyRequest()
	request.BuiltinRuleOverrides = nil
	request.Targets.AgentTypes = nil
	request.Collection.FileContent = "full"
	request.Analysis.AIOnlyActionCeiling = "deny"
	request.FreezeTimeoutSeconds = 10
	request.AtomicRules[0].Resource["path"] = "/etc/../root;touch"
	request.AtomicRules[0].Action = "deny_and_freeze"
	request.AtomicRules[0].Severity = "medium"

	preview := service.Validate(context.Background(), request)
	if preview.Valid || len(preview.Errors) < 6 {
		t.Fatalf("unsafe policy was not comprehensively rejected: %#v", preview.Errors)
	}
}

func TestAgentGuardCollectionRequiresToolCategoryForAdapterRollout(t *testing.T) {
	collection := validAgentGuardPolicyRequest().Collection
	collection.ToolAdapterEnabled = true
	var fields []string
	validateAgentGuardCollection(collection, func(field string, _, _ string) {
		fields = append(fields, field)
	})
	if !containsStringValue(fields, "collection.tool_adapter_enabled") {
		t.Fatalf("missing tool category dependency error: %#v", fields)
	}

	collection.Categories = append(collection.Categories, "tool")
	fields = nil
	validateAgentGuardCollection(collection, func(field string, _, _ string) {
		fields = append(fields, field)
	})
	if len(fields) != 0 {
		t.Fatalf("valid explicit tool adapter collection was rejected: %#v", fields)
	}
}

func TestAgentGuardTargetsAcceptAllBuiltinP4AgentTypes(t *testing.T) {
	targets := model.AgentGuardPolicyTargets{
		AgentTypes: []string{"claude-code", "opencode", "gemini-cli"},
	}
	var fields []string
	validateAgentGuardTargets(targets, func(field string, _, _ string) {
		fields = append(fields, field)
	})
	if len(fields) != 0 {
		t.Fatalf("P4 builtin Agent types were rejected: %#v", fields)
	}
}

func TestAgentGuardPolicyWriteFlagBlocksMutationAfterValidation(t *testing.T) {
	service := NewAgentGuardPolicyService(
		agentGuardCatalogStub{rules: map[string]*model.AgentBehaviorRuleDefinition{
			model.AgentGuardRuleKeySensitiveDirectory: {
				RuleKey:          model.AgentGuardRuleKeySensitiveDirectory,
				RuleVersion:      1,
				Digest:           "sha256:definition",
				ParametersSchema: []byte(`{"type":"object","additionalProperties":false,"properties":{"resource_groups":{"type":"array","items":{"enum":["credential"]}}}}`),
			},
		}},
		&agentGuardPolicyStoreStub{},
		false,
	)
	_, preview, err := service.CreateDraft(context.Background(), validAgentGuardPolicyRequest(), "admin")
	if !preview.Valid {
		t.Fatalf("expected request itself to be valid: %#v", preview.Errors)
	}
	if !errors.Is(err, ErrAgentGuardPolicyWriteDisabled) {
		t.Fatalf("CreateDraft error = %v, want write-disabled error", err)
	}
}

func TestAgentGuardPolicyValidationAppliesBuiltinParameterSchema(t *testing.T) {
	catalog := agentGuardCatalogStub{rules: map[string]*model.AgentBehaviorRuleDefinition{
		model.AgentGuardRuleKeySensitiveDirectory: {
			RuleKey:     model.AgentGuardRuleKeySensitiveDirectory,
			RuleVersion: 1,
			Digest:      "sha256:definition",
			ParametersSchema: []byte(`{
				"type":"object",
				"additionalProperties":false,
				"properties":{
					"resource_groups":{"type":"array","uniqueItems":true,"items":{"enum":["credential"]}}
				}
			}`),
		},
	}}
	service := NewAgentGuardPolicyService(catalog, &agentGuardPolicyStoreStub{}, true)
	request := validAgentGuardPolicyRequest()
	request.BuiltinRuleOverrides[0].Parameters["unknown"] = true
	request.BuiltinRuleOverrides[0].Parameters["resource_groups"] = []any{"credential", "credential"}

	preview := service.Validate(context.Background(), request)
	if preview.Valid {
		t.Fatal("invalid builtin rule parameters were accepted")
	}
	foundUnknown := false
	foundDuplicate := false
	for _, issue := range preview.Errors {
		foundUnknown = foundUnknown || issue.Code == "unknown_parameter"
		foundDuplicate = foundDuplicate || issue.Code == "duplicate"
	}
	if !foundUnknown || !foundDuplicate {
		t.Fatalf("expected schema errors, got %#v", preview.Errors)
	}
}

func validAgentGuardPolicyRequest() model.AgentGuardPolicyDraftRequest {
	return model.AgentGuardPolicyDraftRequest{
		PolicyKey: "prod-agent-guard",
		Name:      "Production Agent Guard",
		Priority:  100,
		Targets: model.AgentGuardPolicyTargets{
			AgentTypes: []string{"codex"},
		},
		Collection: model.AgentGuardCollectionPolicy{
			Categories:     []string{"process", "file", "network"},
			CommandArgv:    "redacted",
			FileContent:    "disabled",
			NetworkContent: "disabled",
			Aggregation:    map[string]int{"file_read_write_seconds": 2},
		},
		BuiltinRuleOverrides: []model.AgentGuardBuiltinRuleOverride{{
			RuleKey:          model.AgentGuardRuleKeySensitiveDirectory,
			RuleVersion:      1,
			Enabled:          true,
			SeverityOverride: "high",
			ActionOverride:   "alert",
			Parameters:       map[string]any{"resource_groups": []any{"credential"}},
		}},
		AtomicRules: []model.AgentGuardAtomicRule{{
			RuleID:     "PROTECTED-RESOURCE-001",
			Rule:       "protected_resource_access",
			Resource:   map[string]any{"type": "file", "path": "/etc/shadow", "match": "exact"},
			Operations: []string{"read", "write"},
			Action:     "deny",
			Severity:   "critical",
		}},
		CorrelationRules: []model.AgentGuardCorrelationRule{{
			RuleID:        "DOWNLOAD-EXEC-001",
			WindowSeconds: 120,
			Action:        "alert",
			Severity:      "high",
			GroupKeys:     []string{"instance_id", "session_id"},
		}},
		Analysis: model.AgentGuardAnalysisPolicy{
			Enabled:               true,
			TriggerSeverities:     []string{"high", "critical"},
			AIOnlyActionCeiling:   "alert",
			EvidenceWindowSeconds: 300,
		},
		EscapeRules: []model.AgentGuardEscapeRule{{
			RuleID:     "ESCAPE-001",
			Rule:       "join_external_namespace",
			Action:     "deny_and_freeze",
			Severity:   "critical",
			Enabled:    true,
			Parameters: map[string]any{},
		}},
		FreezeTimeoutSeconds: 300,
	}
}
