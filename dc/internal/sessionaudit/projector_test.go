package sessionaudit

import "testing"

func TestDecodeBytesAcceptsKafkaJSONBytes(t *testing.T) {
	got, err := decodeBytes([]byte(`"e30="`))
	if err != nil || string(got) != "{}" {
		t.Fatalf("decode failed: %s %v", got, err)
	}
}

func TestVisibilityAndRedactionDefaults(t *testing.T) {
	if visibility(map[string]any{}) != "normal" {
		t.Fatal("unexpected visibility default")
	}
	if !redactionApplied(map[string]any{"redaction_state": "partial"}) {
		t.Fatal("expected redaction marker")
	}
}
