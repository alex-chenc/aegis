package kernel

import (
	"testing"
)

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		release     string
		expectMajor int
		expectMinor int
		expectPatch int
	}{
		{"5.10.0-1160.el8.x86_64", 5, 10, 0},
		{"5.8.0", 5, 8, 0},
		{"4.18.0-305.el8.x86_64", 4, 18, 0},
		{"4.18.0", 4, 18, 0},
		{"3.10.0-1160.el7.x86_64", 3, 10, 0},
		{"6.1.0", 6, 1, 0},
		{"5.15.123", 5, 15, 123},
		{"invalid", 0, 0, 0},
		{"", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.release, func(t *testing.T) {
			major, minor, patch := parseKernelVersion(tt.release)
			if major != tt.expectMajor {
				t.Errorf("major: got %d, want %d", major, tt.expectMajor)
			}
			if minor != tt.expectMinor {
				t.Errorf("minor: got %d, want %d", minor, tt.expectMinor)
			}
			if patch != tt.expectPatch {
				t.Errorf("patch: got %d, want %d", patch, tt.expectPatch)
			}
		})
	}
}

func TestSelectTransport(t *testing.T) {
	tests := []struct {
		name           string
		major          int
		minor          int
		btf            bool
		ringbuf        bool
		expectTrans    EventTransport
		expectDisabled bool
	}{
		{"kernel 5.8+ with BTF", 5, 8, true, true, TransportRingbuf, false},
		{"kernel 5.10 with BTF", 5, 10, true, true, TransportRingbuf, false},
		{"kernel 6.1 with BTF", 6, 1, true, true, TransportRingbuf, false},
		{"kernel 5.8 without ringbuf probe", 5, 8, true, false, TransportPerf, false},
		{"kernel 4.18 with BTF", 4, 18, true, false, TransportPerf, false},
		{"kernel 5.7 with BTF", 5, 7, true, false, TransportPerf, false},
		{"kernel 4.18 without BTF", 4, 18, false, false, TransportDisabled, true},
		{"kernel 3.10", 3, 10, false, false, TransportDisabled, true},
		{"kernel 3.10 with BTF", 3, 10, true, true, TransportDisabled, true},
		{"kernel 4.17 with BTF", 4, 17, true, false, TransportDisabled, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := &Capabilities{
				Major:            tt.major,
				Minor:            tt.minor,
				BTFAvailable:     tt.btf,
				RingbufAvailable: tt.ringbuf,
			}
			trans, reason := selectTransport(caps)
			if trans != tt.expectTrans {
				t.Errorf("transport: got %s, want %s", trans, tt.expectTrans)
			}
			if tt.expectDisabled && reason == "" {
				t.Error("expected disabled reason, got empty")
			}
			if !tt.expectDisabled && reason != "" {
				t.Errorf("unexpected disabled reason: %s", reason)
			}
		})
	}
}

func TestDetectReturnsValidCaps(t *testing.T) {
	caps := Detect()
	if caps == nil {
		t.Fatal("Detect returned nil")
	}
	if caps.KernelRelease == "" {
		t.Error("kernel release is empty")
	}
	if caps.Transport == TransportRingbuf || caps.Transport == TransportPerf {
		if !caps.BTFAvailable {
			t.Error("transport enabled but BTF not available")
		}
	}
	if caps.Transport == TransportDisabled && caps.DisabledReason == "" {
		t.Error("disabled but no reason given")
	}
}
