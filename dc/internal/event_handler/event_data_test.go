package event_handler

import "testing"

func TestNormalizeEventDataJSONReturnsObjectForEmptyInput(t *testing.T) {
	if got := normalizeEventDataJSON(""); got != "{}" {
		t.Fatalf("expected empty object for empty event_data, got %q", got)
	}
}

func TestNormalizeEventDataJSONPreservesValidJSON(t *testing.T) {
	input := `{"process_name":"bash"}`
	if got := normalizeEventDataJSON(input); got != input {
		t.Fatalf("expected valid JSON to be preserved, got %q", got)
	}
}
