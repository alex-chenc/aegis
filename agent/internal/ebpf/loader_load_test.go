package ebpf

import (
	"testing"

	"aegis-agent/internal/ebpf/kernel"
)



func TestDefaultBPFPrograms(t *testing.T) {
	progs := defaultBPFPrograms()
	if len(progs) < 2 {
		t.Fatalf("expected at least 2 default programs, got %d", len(progs))
	}

	found := false
	for _, p := range progs {
		if p.name == "execve" {
			found = true
			if !p.required {
				t.Error("execve should be required")
			}
			if p.mapName != "exec_events" {
				t.Errorf("execve mapName: got %q, want %q", p.mapName, "exec_events")
			}
		}
	}
	if !found {
		t.Error("execve not found in default programs")
	}
}

func TestBPFObjectSuffix(t *testing.T) {
	tests := []struct {
		transport kernel.EventTransport
		expected  string
	}{
		{kernel.TransportRingbuf, ".ringbuf.bpf.o"},
		{kernel.TransportPerf, ".perf.bpf.o"},
		{kernel.TransportDisabled, ".bpf.o"},
	}
	for _, tt := range tests {
		caps := &kernel.Capabilities{Transport: tt.transport}
		result := BPFObjectSuffix(caps)
		if result != tt.expected {
			t.Errorf("BPFObjectSuffix(%s) = %q, want %q", tt.transport, result, tt.expected)
		}
	}
}

func TestNewLoaderDisabled(t *testing.T) {
	ch := make(chan Event, 10)
	loader, err := NewLoader("test-host", ch)
	if err != nil {
		t.Logf("NewLoader returned error (expected in test env): %v", err)
		return
	}
	if loader != nil {
		loader.Close()
	}
}
