package grpc_server

import "testing"

func TestNormalizeJSONBTextReturnsValidJSONForEmptyProcessTree(t *testing.T) {
	if got := normalizeJSONBText(""); got != "{}" {
		t.Fatalf("expected empty JSON object for empty input, got %q", got)
	}
}

func TestNormalizeJSONBTextPreservesValidJSON(t *testing.T) {
	input := `{"pid":123,"children":[]}`
	if got := normalizeJSONBText(input); got != input {
		t.Fatalf("expected valid JSON to be preserved, got %q", got)
	}
}

func TestNormalizeJSONBTextReplacesInvalidJSON(t *testing.T) {
	if got := normalizeJSONBText("not-json"); got != "{}" {
		t.Fatalf("expected invalid JSON to be replaced, got %q", got)
	}
}
