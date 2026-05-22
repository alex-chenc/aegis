package ebpf

import (
	"testing"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector("test-host", 10000)
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.hostID != "test-host" {
		t.Errorf("hostID: got %q, want %q", c.hostID, "test-host")
	}
	if c.IsRunning() {
		t.Error("new collector should not be running")
	}
}

func TestNewCollectorDefaultBufferSize(t *testing.T) {
	c := NewCollector("test-host", 0)
	if cap(c.events) != 10000 {
		t.Errorf("default buffer size: got %d, want 10000", cap(c.events))
	}
}

func TestCollectorStartWithoutEBPF(t *testing.T) {
	c := NewCollector("test-host", 1000)
	err := c.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if c.IsRunning() {
		t.Log("eBPF loaded successfully in test environment")
	}
}

func TestCollectorStopIdempotent(t *testing.T) {
	c := NewCollector("test-host", 1000)
	c.Stop()
	c.Stop()
}

func TestEventsChannel(t *testing.T) {
	c := NewCollector("test-host", 100)
	ch := c.Events()
	if ch == nil {
		t.Fatal("Events channel is nil")
	}
}

func TestParsePID(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		{"1234", 1234, false},
		{"1", 1, false},
		{"0", 0, false},
		{"abc", 0, true},
		{"12ab", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			pid, err := parsePID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if pid != tt.expected {
				t.Errorf("parsePID(%q) = %d, want %d", tt.input, pid, tt.expected)
			}
		})
	}
}
