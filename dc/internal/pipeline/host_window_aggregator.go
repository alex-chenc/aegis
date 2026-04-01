package pipeline

import (
	"dc/internal/model"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type HostWindowAggregator struct {
	windowSize time.Duration
	maxEvents  int
}

func NewHostWindowAggregator(windowSize time.Duration, maxEvents int) *HostWindowAggregator {
	return &HostWindowAggregator{
		windowSize: windowSize,
		maxEvents:  maxEvents,
	}
}

// Aggregate aggregates multiple events into a window
func (a *HostWindowAggregator) Aggregate(events []*model.RuntimeEvent) (*model.EventWindow, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to aggregate")
	}

	// Group events by host
	hostEvents := make(map[uuid.UUID][]*model.RuntimeEvent)
	for _, event := range events {
		hostEvents[event.HostID] = append(hostEvents[event.HostID], event)
	}

	// Create windows for each host
	var windows []*model.EventWindow
	now := time.Now()

	for hostID, hostEventList := range hostEvents {
		window := &model.EventWindow{
			HostID:     hostID,
			Events:     hostEventList,
			StartTime:  now,
			EndTime:    now.Add(a.windowSize),
			EventCount: len(hostEventList),
		}
		windows = append(windows, window)
	}

	return &model.EventWindow{
		Events:     events,
		StartTime:  now,
		EndTime:    now.Add(a.windowSize),
		EventCount: len(events),
		HostWindows: windows,
	}, nil
}

func (a *HostWindowAggregator) GetWindowSize() time.Duration {
	return a.windowSize
}

func (a *HostWindowAggregator) GetMaxEvents() int {
	return a.maxEvents
}