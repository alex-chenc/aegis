package pipeline

import (
	"context"
	"sync"
	"time"
)

// RuntimeEvent represents a security event from Agent
type RuntimeEvent struct {
	EventType     string `json:"event_type"`
	PID           int    `json:"pid"`
	CommandLine   string `json:"command_line"`
	MatchedRuleID string `json:"matched_rule_id"`
	MitreID       string `json:"mitre_id"`
	Severity      string `json:"severity"`
	Timestamp     int64  `json:"timestamp"` // Unix millisecond timestamp from agent
}

// HostWindow holds events for a single host within a time window
type HostWindow struct {
	HostID      string
	Events      []RuntimeEvent
	WindowStart time.Time
	WindowEnd   time.Time
	mu          sync.Mutex
}

// FlushCallback is called when a window is ready for processing
type FlushCallback func(window *HostWindow)

// HostWindowAggregator manages per-host time windows
type HostWindowAggregator struct {
	windows    map[string]*HostWindow
	windowSize time.Duration
	mu         sync.RWMutex
	onFlush    FlushCallback
}

// NewHostWindowAggregator creates a new aggregator with the specified window size
func NewHostWindowAggregator(windowSize time.Duration, onFlush FlushCallback) *HostWindowAggregator {
	return &HostWindowAggregator{
		windows:    make(map[string]*HostWindow),
		windowSize: windowSize,
		onFlush:    onFlush,
	}
}

// AddEvent adds an event to the host's current window
func (a *HostWindowAggregator) AddEvent(hostID string, event RuntimeEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	window, ok := a.windows[hostID]
	if !ok {
		now := time.Now()
		window = &HostWindow{
			HostID:      hostID,
			Events:      []RuntimeEvent{},
			WindowStart: now,
			WindowEnd:   now.Add(a.windowSize),
		}
		a.windows[hostID] = window
	}

	window.mu.Lock()
	window.Events = append(window.Events, event)
	window.mu.Unlock()
}

// FlushReady returns and removes all windows that have expired
func (a *HostWindowAggregator) FlushReady() map[string][]RuntimeEvent {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	ready := make(map[string][]RuntimeEvent)

	for hostID, window := range a.windows {
		if now.After(window.WindowEnd) {
			window.mu.Lock()
			ready[hostID] = window.Events
			window.mu.Unlock()
			delete(a.windows, hostID)

			if a.onFlush != nil {
				go a.onFlush(window)
			}
		}
	}

	return ready
}

// StartTicker starts a background goroutine that periodically flushes expired windows
func (a *HostWindowAggregator) StartTicker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.FlushReady()
		}
	}
}

// GetWindowCount returns the number of active windows (for monitoring)
func (a *HostWindowAggregator) GetWindowCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.windows)
}

// GetEventCount returns the total number of events across all windows (for monitoring)
func (a *HostWindowAggregator) GetEventCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	total := 0
	for _, window := range a.windows {
		window.mu.Lock()
		total += len(window.Events)
		window.mu.Unlock()
	}
	return total
}
