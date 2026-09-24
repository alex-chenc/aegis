package mcpauthz

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeArgumentsRejectsSchemaAndJSONAmbiguity(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":3}},"additionalProperties":false}`)
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"missing-required-by-schema", `{"extra":true}`},
		{"wrong-type", `{"limit":"2"}`},
		{"below-minimum", `{"limit":0}`},
		{"above-maximum", `{"limit":4}`},
		{"additional-property", `{"limit":2,"extra":true}`},
		{"duplicate-key", `{"limit":2,"limit":3}`},
		{"trailing-json", `{"limit":2}{}`},
		{"non-object", `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := DecodeArguments([]byte(tc.raw), schema); err == nil {
				t.Fatalf("expected rejection for %s", tc.raw)
			}
		})
	}
}

func TestDecodeArgumentsPreservesZeroFalseNullAndLargeInteger(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"zero":{"type":"integer"},"flag":{"type":"boolean"},"value":{"type":["string","null"]},"big":{"type":"integer"}},"required":["zero","flag","value","big"],"additionalProperties":false}`)
	object, canonical, err := DecodeArguments([]byte(`{"zero":0,"flag":false,"value":null,"big":9007199254740993}`), schema)
	if err != nil {
		t.Fatal(err)
	}
	if object["zero"].(json.Number) != "0" || object["flag"].(bool) || object["value"] != nil || object["big"].(json.Number) != "9007199254740993" {
		t.Fatalf("argument semantics changed: %#v", object)
	}
	if string(canonical) != `{"big":9007199254740993,"flag":false,"value":null,"zero":0}` {
		t.Fatalf("unexpected canonical bytes: %s", canonical)
	}
}

func TestDecodeArgumentsDoesNotRoundLargeIntegerBounds(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"value":{"type":"integer","maximum":9007199254740992}},"required":["value"],"additionalProperties":false}`)
	if _, _, err := DecodeArguments([]byte(`{"value":9007199254740993}`), schema); err == nil {
		t.Fatal("integer above maximum must not pass through float64 rounding")
	}
}

func TestValidateSchemaRejectsUnsupportedOrAmbiguousSchemas(t *testing.T) {
	for _, raw := range []string{
		`{"type":"object","$ref":"https://attacker.invalid/schema"}`,
		`{"type":"object","oneOf":[]}`,
		`{"type":"object","properties":{"x":{"type":"wat"}}}`,
		`{"type":"object","pattern":"["}`,
		`{"type":"object","required":["x","x"]}`,
		`{"type":"object","minimum":"1"}`,
		`{"type":"object","properties":{"x":null}}`,
	} {
		if err := ValidateSchema(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected unsupported schema to fail: %s", raw)
		}
	}
	if err := ValidateSchema(json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DecodeArguments([]byte(strings.Repeat("{", MaxJSONDepth+2)), json.RawMessage(`{"type":"object"}`)); err == nil {
		t.Fatal("malformed/deep input must fail")
	}
}

func TestValidateSchemaAcceptsMCPToolDescription(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Optional hostname or IP substring."}},"additionalProperties":false}`)
	if err := ValidateSchema(schema); err != nil {
		t.Fatalf("MCP tool schema descriptions must remain valid: %v", err)
	}
	if _, _, err := DecodeArguments([]byte(`{"query":"host"}`), schema); err != nil {
		t.Fatalf("MCP tool arguments must accept a described property: %v", err)
	}
}
