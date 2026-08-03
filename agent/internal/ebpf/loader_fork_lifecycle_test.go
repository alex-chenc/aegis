package ebpf

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestForkMapCarriesForkAndExitLifecycle(t *testing.T) {
	tests := []struct {
		name     string
		raw      ForkEvent
		wantType string
		wantPID  int
		wantPPID int
		wantName string
	}{
		{
			name: "fork",
			raw: ForkEvent{
				EventType: ForkEventTypeFork, PID: 4110, PPID: 4100, UID: 1000,
				Comm: fixedComm("bash"), ParentComm: fixedComm("codex"),
			},
			wantType: "process_fork", wantPID: 4110, wantPPID: 4100, wantName: "bash",
		},
		{
			name: "exit",
			raw: ForkEvent{
				EventType: ForkEventTypeExit, PID: 4110, PPID: 4100, UID: 1000,
				Comm: fixedComm("bash"), ParentComm: fixedComm("codex"),
			},
			wantType: "process_exit", wantPID: 4110, wantPPID: 4100, wantName: "bash",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var data bytes.Buffer
			if err := binary.Write(&data, binary.LittleEndian, test.raw); err != nil {
				t.Fatal(err)
			}
			loader := &Loader{hostID: "host-1", hostname: "host", eventChan: make(chan Event, 1)}
			loader.processForkEvent(data.Bytes())
			event := <-loader.eventChan
			if event.EventType != test.wantType || event.PID != test.wantPID ||
				event.PPID != test.wantPPID || event.ProcessName != test.wantName {
				t.Fatalf("unexpected lifecycle event: %#v", event)
			}
		})
	}
}

func fixedComm(value string) [16]byte {
	var result [16]byte
	copy(result[:], value)
	return result
}
