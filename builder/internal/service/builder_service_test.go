package service

import (
	"encoding/json"
	"testing"
)

func TestExtractEventSchemaReturnsJSON(t *testing.T) {
	metadata := `
schema_version: "aegis.ebpf_plugin.v1"
plugin_id: "copyfail_probe"
package_id: "cve-2026-31431-copyfail"
event_schema:
  events:
    1001:
      name: "af_alg_socket"
      fields:
        1: { name: "family", type: "string" }
`

	got := extractEventSchema(metadata)
	if got == "" {
		t.Fatal("expected event schema")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("expected JSON event schema, got %q: %v", got, err)
	}
	if decoded["events"] == nil {
		t.Fatalf("expected events key, got %#v", decoded)
	}
}
