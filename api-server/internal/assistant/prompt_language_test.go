package assistant

import (
	"context"
	"regexp"
	"strings"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

var hanPromptPattern = regexp.MustCompile(`[\p{Han}]`)

func TestAssistantStaticModelInstructionsAreEnglish(t *testing.T) {
	staticPrompts := map[string]string{
		"intent_router":     intentRouterSystemPrompt,
		"intent_decomposer": intentDecomposerSystemPrompt,
		"json_retry":        jsonOnlyRetryReminder,
	}
	for name, prompt := range staticPrompts {
		t.Run(name, func(t *testing.T) {
			assertNoHanPromptText(t, prompt)
		})
	}

	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{{
		Name:        "Vulnerability.Script.Generate",
		Description: "Capability: generate_vulnerability_script.",
		ArgsSchema: map[string]interface{}{
			"properties": map[string]interface{}{
				"cve_id": map[string]interface{}{"type": "string"},
			},
		},
	}}, nil, "operations", "生成漏洞脚本")

	for _, purpose := range []agentruntime.LLMPurpose{
		agentruntime.PurposePlan,
		agentruntime.PurposeReact,
		agentruntime.PurposeSummarize,
	} {
		bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: purpose})
		if err != nil {
			t.Fatalf("Build(%s) error = %v", purpose, err)
		}
		assertNoHanPromptText(t, bundle.SystemPrompt)
	}
}

func TestModelFacingToolMetadataUsesEnglishContract(t *testing.T) {
	spec := &ToolSpec{
		Name:        "Vulnerability.Script.Generate",
		Domain:      DomainVulnerability,
		Operation:   OpGenerate,
		Capability:  "generate_vulnerability_script",
		Description: "生成漏洞验证或修复脚本",
		Aliases:     []string{"生成漏洞脚本"},
		ObjectTypes: []string{"vulnerability", "script"},
		Risk:        ToolRiskMedium,
		Enabled:     true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cve_id": map[string]interface{}{
					"type":        "string",
					"description": "漏洞编号",
				},
				"script_type": map[string]interface{}{
					"type":        "string",
					"description": "脚本类型",
					"enum":        []interface{}{"poc", "fix"},
				},
			},
			"required": []interface{}{"cve_id", "script_type"},
		},
	}

	for name, text := range map[string]string{
		"model_description": modelFacingToolDescription(spec),
		"descriptor":        spec.Descriptor().Description,
	} {
		t.Run(name, func(t *testing.T) {
			assertNoHanPromptText(t, text)
			if !strings.Contains(text, "generate_vulnerability_script") {
				t.Fatalf("model metadata must expose the capability, got %q", text)
			}
		})
	}
}

func TestToolRegistryRejectsNonEnglishCapability(t *testing.T) {
	registry := NewToolRegistry()
	err := registry.Register(&ToolSpec{
		Name:       "Vulnerability.Script.Generate",
		Capability: "生成漏洞脚本",
		Handler:    noopToolHandler,
	})
	if err == nil || !strings.Contains(err.Error(), "lowercase English capability identifier") {
		t.Fatalf("expected non-English tool capability to be rejected, got %v", err)
	}
}

func assertNoHanPromptText(t *testing.T, value string) {
	t.Helper()
	if hanPromptPattern.MatchString(value) {
		t.Fatalf("model-facing static prompt contains Han characters:\n%s", value)
	}
}
