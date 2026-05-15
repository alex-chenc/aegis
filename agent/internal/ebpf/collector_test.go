package ebpf

import (
	"os"
	"testing"
)

func TestSnapshotExistingProcesses(t *testing.T) {
	c := NewCollector("test-host", 10000)
	knownPIDs := make(map[int]struct{})
	c.snapshotExistingProcesses(knownPIDs)

	// Current process should be in the snapshot
	myPID := os.Getpid()
	if _, ok := knownPIDs[myPID]; !ok {
		t.Errorf("current PID %d not found in snapshot", myPID)
	}

	// Drain events and verify our process is among them
	foundMyPID := false
	for {
		select {
		case event := <-c.events:
			if event.PID == myPID {
				foundMyPID = true
			}
			if event.EventType != "process_exec" {
				t.Errorf("expected event type process_exec, got %q", event.EventType)
			}
		default:
			if !foundMyPID {
				t.Errorf("event for current PID %d not found in snapshot events", myPID)
			}
			return
		}
	}
}

func TestSnapshotExistingProcessesPopulatesKnownPIDs(t *testing.T) {
	c := NewCollector("test-host", 10000)
	knownPIDs := make(map[int]struct{})
	c.snapshotExistingProcesses(knownPIDs)

	// Should have multiple PIDs (at least our process and system processes)
	if len(knownPIDs) < 2 {
		t.Errorf("expected at least 2 known PIDs, got %d", len(knownPIDs))
	}

	// PID 1 (init) should be present on Linux
	if _, ok := knownPIDs[1]; !ok {
		t.Log("PID 1 not found (may be expected in containers)")
	}
}
