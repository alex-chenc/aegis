package service

import (
	"context"
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

func TestNormalizeBPFTargetArch(t *testing.T) {
	tests := map[string]string{
		"":        "x86",
		"amd64":   "x86",
		"x86_64":  "x86",
		"x86":     "x86",
		"arm64":   "arm",
		"aarch64": "arm",
		"arm":     "arm",
	}

	for input, want := range tests {
		got, err := normalizeBPFTargetArch(input)
		if err != nil {
			t.Fatalf("normalizeBPFTargetArch(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeBPFTargetArch(%q) = %q, want %q", input, got, want)
		}
	}

	if _, err := normalizeBPFTargetArch("mips"); err == nil {
		t.Fatal("expected unsupported arch error")
	}
}

func TestBPFTransportMacro(t *testing.T) {
	tests := map[string]string{
		"perf":    "AEGIS_EVENT_PERF",
		"ringbuf": "AEGIS_EVENT_RINGBUF",
	}

	for input, want := range tests {
		got, err := bpfTransportMacro(input)
		if err != nil {
			t.Fatalf("bpfTransportMacro(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("bpfTransportMacro(%q) = %q, want %q", input, got, want)
		}
	}

	if _, err := bpfTransportMacro("raw"); err == nil {
		t.Fatal("expected unsupported transport error")
	}
}

func TestStartBuildValidationFailureIncludesBuildLogTail(t *testing.T) {
	svc := NewBuilderService(nil, t.TempDir(), nil)

	result, err := svc.StartBuild(context.Background(), BuildRequest{
		BuildID:    "validation-build",
		PackageID:  "pkg-validation",
		Version:    "1.0.0",
		EBPFSource: "int x(void) { return (long)bpf_get_current_task(); }",
	})
	if err != nil {
		t.Fatalf("StartBuild returned error: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("Status = %q, want failed", result.Status)
	}
	if result.ErrorMessage != "validation: forbidden BPF helper call: bpf_get_current_task" {
		t.Fatalf("unexpected ErrorMessage: %q", result.ErrorMessage)
	}
	if result.BuildLogTail == "" {
		t.Fatal("expected BuildLogTail for validation failure")
	}
	if result.BuildLogObjectKey != "" {
		t.Fatalf("BuildLogObjectKey = %q, want empty when MinIO is unavailable", result.BuildLogObjectKey)
	}
}
