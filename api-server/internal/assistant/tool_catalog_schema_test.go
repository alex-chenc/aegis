package assistant

import (
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// TestNormalizeRuntimeArgsSchemaRelaxesInteger 验证提交给 agent-runtime 的参数
// schema 会把 "integer" 放宽为 "number"。LLM 返回的整数经 JSON 解析后是 float64，
// runtime 校验器对 "integer" 拒绝 float64，会报 `$.page has type number, want integer`。
func TestNormalizeRuntimeArgsSchemaRelaxesInteger(t *testing.T) {
	spec := ToolSpec{
		Name: "Vulnerability.AffectedHosts",
		ArgsSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page":      map[string]any{"type": "integer", "description": "Page number"},
				"page_size": map[string]any{"type": "integer", "description": "Number of items per page"},
				"cve_id":    map[string]any{"type": "string"},
			},
		},
	}

	desc := spec.Descriptor()
	props, ok := desc.ArgsSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", desc.ArgsSchema["properties"])
	}

	for _, field := range []string{"page", "page_size"} {
		prop, _ := props[field].(map[string]any)
		if got := prop["type"]; got != "number" {
			t.Fatalf("field %q type = %v, want number", field, got)
		}
	}

	if got := props["cve_id"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("cve_id type = %v, want string (unchanged)", got)
	}
	if got := desc.ArgsSchema["additionalProperties"]; got != false {
		t.Fatalf("additionalProperties = %v, want false for an exact runtime contract", got)
	}
	if got := props["page"].(map[string]any)["description"]; got != "Page number" {
		t.Fatalf("page description = %v, want the English parameter semantics preserved", got)
	}

	// 源 spec 不应被修改，仍保留 integer 语义。
	srcProps := spec.ArgsSchema["properties"].(map[string]any)
	if got := srcProps["page"].(map[string]any)["type"]; got != "integer" {
		t.Fatalf("source spec mutated: page type = %v, want integer", got)
	}
	if _, exists := spec.ArgsSchema["additionalProperties"]; exists {
		t.Fatal("source spec mutated: additionalProperties was added")
	}
}

func TestNormalizeRuntimeArgsSchemaKeepsExamplesAndFormats(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"host_id": map[string]any{
				"type":        "string",
				"format":      "uuid",
				"description": "Exact host UUID returned by Host.Resolve.",
				"examples":    []any{"cf18f7f7-5b45-46e2-9889-160dddc4ee30"},
			},
		},
	}

	normalized := normalizeRuntimeArgsSchema(schema)
	properties := normalized["properties"].(map[string]any)
	hostID := properties["host_id"].(map[string]any)
	if hostID["description"] == nil || hostID["format"] != "uuid" || hostID["examples"] == nil {
		t.Fatalf("normalized schema lost model-facing semantics: %#v", hostID)
	}
}

func TestToolSpecDescriptorCarriesPrerequisiteEvidenceGate(t *testing.T) {
	spec := ToolSpec{
		Name: "Example.Fallback",
		ExecutionContract: ToolExecutionContract{
			Prerequisites: []ToolPrerequisite{{
				Capability: "list_examples",
				Condition:  agentruntime.PrerequisiteCapabilityEmptyResult,
			}},
		},
	}
	descriptor := spec.Descriptor()
	if len(descriptor.Prerequisites) != 1 {
		t.Fatalf("prerequisites = %#v, want one", descriptor.Prerequisites)
	}
	if got := descriptor.Prerequisites[0]; got.Capability != "list_examples" || got.Condition != agentruntime.PrerequisiteCapabilityEmptyResult {
		t.Fatalf("prerequisite = %#v", got)
	}
}
