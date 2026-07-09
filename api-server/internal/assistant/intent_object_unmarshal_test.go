package assistant

import "testing"

// TestIntentBreakdownUnmarshalTolerantOfStringObjects reproduces the LLM output
// that previously aborted intent decomposition with
// "cannot unmarshal string into Go struct field IntentBreakdown.objects".
func TestIntentBreakdownUnmarshalTolerantOfStringObjects(t *testing.T) {
	raw := `{"goal":"g","objects":["cve","CVE-2024-1234","这个告警"],"missing_info":["object"],"scope":{"kind":"all"}}`
	var breakdown IntentBreakdown
	if err := unmarshalFirstJSONObject(raw, &breakdown); err != nil {
		t.Fatalf("unmarshal string-form objects: %v", err)
	}
	if len(breakdown.Objects) != 3 {
		t.Fatalf("expected 3 objects, got %d (%#v)", len(breakdown.Objects), breakdown.Objects)
	}
	if breakdown.Objects[0].Type != "cve" {
		t.Fatalf("expected first object type cve, got %#v", breakdown.Objects[0])
	}
	if breakdown.Objects[1].Type != "CVE-2024-1234" || breakdown.Objects[1].ID != "" {
		t.Fatalf("expected bare string preserved without domain parsing, got %#v", breakdown.Objects[1])
	}
	if breakdown.Objects[2].Type != "这个告警" {
		t.Fatalf("expected reference string preserved as type, got %#v", breakdown.Objects[2])
	}
	if len(breakdown.MissingInfo) != 1 || breakdown.MissingInfo[0].Field != "object" {
		t.Fatalf("expected missing_info string form mapped to field, got %#v", breakdown.MissingInfo)
	}
}

// TestIntentBreakdownUnmarshalStillSupportsObjectForm ensures the structured
// form keeps working after adding string tolerance.
func TestIntentBreakdownUnmarshalStillSupportsObjectForm(t *testing.T) {
	raw := `{"objects":[{"type":"host","id":"h-1","selector":"online"}],"missing_info":[{"field":"alert_id","reason":"missing"}]}`
	var breakdown IntentBreakdown
	if err := unmarshalFirstJSONObject(raw, &breakdown); err != nil {
		t.Fatalf("unmarshal object-form: %v", err)
	}
	if len(breakdown.Objects) != 1 || breakdown.Objects[0].ID != "h-1" || breakdown.Objects[0].Selector != "online" {
		t.Fatalf("expected structured object preserved, got %#v", breakdown.Objects)
	}
	if len(breakdown.MissingInfo) != 1 || breakdown.MissingInfo[0].Reason != "missing" {
		t.Fatalf("expected structured missing_info preserved, got %#v", breakdown.MissingInfo)
	}
}
