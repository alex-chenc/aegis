package ebpf

import (
	"encoding/binary"
	"testing"

	"aegis-agent/internal/tools"
)

// TestProcessFileEventDropsAgentSelfPID verifies the defense-in-depth guard
// that prevents the eBPF file-event feedback loop: events whose pid equals the
// loader's own agent pid are dropped before enrichment/logging. The in-kernel
// primary filter lives in file.bpf.c (is_agent_self); this Go guard is the
// fallback when the agent_pid_config map update fails.
func TestProcessFileEventDropsAgentSelfPID(t *testing.T) {
	const selfPID uint32 = 4242

	tests := []struct {
		name     string
		eventPID uint32
		wantSent bool
	}{
		{"self_pid_dropped", selfPID, false},
		{"other_pid_passed_through", 999999, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &Loader{
				hostID:      "test-host",
				hostname:    "test-host",
				agentPID:    selfPID,
				eventChan:   make(chan Event, 1),
				toolManager: tools.NewToolManager(),
			}

			// FileEvent layout: Pid at offset 8, Action at offset 32.
			buf := make([]byte, 564)
			binary.LittleEndian.PutUint32(buf[8:12], tc.eventPID)
			binary.LittleEndian.PutUint32(buf[32:36], FileActionOpenRead)

			l.processFileEvent(buf)

			select {
			case <-l.eventChan:
				if !tc.wantSent {
					t.Fatalf("expected event to be dropped for pid %d, but one was sent", tc.eventPID)
				}
			default:
				if tc.wantSent {
					t.Fatalf("expected event to be sent for pid %d, but none was", tc.eventPID)
				}
			}
		})
	}
}
