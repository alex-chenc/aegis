package assistant

import (
	"context"
	"testing"
)

func TestValidateToolArgsRejectsInvalidNestedArguments(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host_ids": map[string]interface{}{
				"type":     "array",
				"minItems": 1,
				"items":    map[string]interface{}{"type": "string", "format": "uuid"},
			},
			"remediation": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"max_rounds": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 10},
				},
				"additionalProperties": false,
			},
		},
		"required":             []string{"host_ids"},
		"additionalProperties": false,
	}
	for _, args := range []map[string]interface{}{
		{},
		{"host_ids": []interface{}{}},
		{"host_ids": []interface{}{"not-a-uuid"}},
		{"host_ids": []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}, "remediation": map[string]interface{}{"max_rounds": 11.0}},
		{"host_ids": []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}, "unexpected": true},
	} {
		if err := ValidateToolArgs(schema, args); err == nil {
			t.Fatalf("expected arguments to fail validation: %#v", args)
		}
	}
}

func TestValidateToolArgsAcceptsJSONDecodedInteger(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"max_rounds": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 10},
		},
		"required": []interface{}{"max_rounds"},
	}
	if err := ValidateToolArgs(schema, map[string]interface{}{"max_rounds": 3.0}); err != nil {
		t.Fatalf("valid JSON-decoded integer rejected: %v", err)
	}
}

func TestToolRegistryValidatesArgumentsBeforeHandler(t *testing.T) {
	called := false
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:        "Example.Execute",
		Capability:  "execute_example",
		Description: "Execute an example.",
		Enabled:     true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target_id": map[string]interface{}{"type": "string", "format": "uuid"}},
			"required":   []string{"target_id"},
		},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			called = true
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), "Example.Execute", map[string]interface{}{"target_id": "bad"}); err == nil {
		t.Fatal("expected backend argument validation to fail")
	}
	if called {
		t.Fatal("handler ran after backend argument validation failed")
	}
}
