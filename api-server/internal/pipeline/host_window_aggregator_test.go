package pipeline

import (
	"sync"
	"testing"
	"time"
)

func TestHostWindowAggregator_AddEvent(t *testing.T) {
	var flushedWindows []*HostWindow
	var mu sync.Mutex

	onFlush := func(window *HostWindow) {
		mu.Lock()
		flushedWindows = append(flushedWindows, window)
		mu.Unlock()
	}

	aggregator := NewHostWindowAggregator(100*time.Millisecond, onFlush)

	// Add events for two hosts
	event1 := RuntimeEvent{EventType: "process_exec", PID: 123, MitreID: "T1059.004"}
	event2 := RuntimeEvent{EventType: "process_exec", PID: 456, MitreID: "T1059.004"}

	aggregator.AddEvent("host-1", event1)
	aggregator.AddEvent("host-2", event2)

	// Verify events are in windows
	if aggregator.GetWindowCount() != 2 {
		t.Errorf("expected 2 windows, got %d", aggregator.GetWindowCount())
	}
	if aggregator.GetEventCount() != 2 {
		t.Errorf("expected 2 events, got %d", aggregator.GetEventCount())
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Flush ready windows
	ready := aggregator.FlushReady()
	if len(ready) != 2 {
		t.Errorf("expected 2 ready windows, got %d", len(ready))
	}

	// Wait for async flush callbacks to complete
	time.Sleep(50 * time.Millisecond)

	// Verify flush callback was called
	mu.Lock()
	if len(flushedWindows) != 2 {
		t.Errorf("expected 2 flushed windows, got %d", len(flushedWindows))
	}
	mu.Unlock()
}

func TestHostWindowAggregator_MultipleEventsSameHost(t *testing.T) {
	aggregator := NewHostWindowAggregator(1*time.Second, nil)

	// Add multiple events for same host
	for i := 0; i < 5; i++ {
		event := RuntimeEvent{EventType: "process_exec", PID: i}
		aggregator.AddEvent("host-1", event)
	}

	// Should have 1 window with 5 events
	if aggregator.GetWindowCount() != 1 {
		t.Errorf("expected 1 window, got %d", aggregator.GetWindowCount())
	}
	if aggregator.GetEventCount() != 5 {
		t.Errorf("expected 5 events, got %d", aggregator.GetEventCount())
	}
}
