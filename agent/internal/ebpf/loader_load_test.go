package ebpf

import (
	"path/filepath"
	"testing"

	"aegis-agent/internal/ebpf/kernel"

	ciliumebpf "github.com/cilium/ebpf"
)

func TestDefaultBPFPrograms(t *testing.T) {
	progs := defaultBPFPrograms()
	if len(progs) < 2 {
		t.Fatalf("expected at least 2 default programs, got %d", len(progs))
	}

	found := false
	forkFound := false
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
		if p.name == "fork" {
			forkFound = true
			if p.attachType != "raw_tracepoint" {
				t.Errorf("fork attachType: got %q, want raw_tracepoint", p.attachType)
			}
		}
	}
	if !found {
		t.Error("execve not found in default programs")
	}
	if !forkFound {
		t.Error("fork not found in default programs")
	}
}

func TestForkObjectsContainRawTracepointLifecyclePrograms(t *testing.T) {
	for _, transport := range []string{"ringbuf", "perf"} {
		t.Run(transport, func(t *testing.T) {
			path := filepath.Join("bpf", "obj", "fork."+transport+".bpf.o")
			spec, err := ciliumebpf.LoadCollectionSpec(path)
			if err != nil {
				t.Fatalf("load %s: %v", path, err)
			}
			for _, name := range []string{"trace_fork", "trace_guarded_process_exit"} {
				program := spec.Programs[name]
				if program == nil {
					t.Fatalf("%s missing from %s", name, path)
				}
				if program.Type != ciliumebpf.RawTracepoint {
					t.Errorf("%s type = %s, want RawTracepoint", name, program.Type)
				}
			}
		})
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
