package aggregator

import (
	"sync"
	"time"

	"dc/internal/model"
	"dc/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EventWindow struct {
	HostID     uuid.UUID
	Events     []*model.RuntimeEvent
	StartTime  time.Time
	EndTime    time.Time
}

type Aggregator struct {
	windowSize time.Duration
	maxEvents  int
	windows    sync.Map
	logger     *zap.Logger
}

func NewAggregator(windowSize time.Duration, maxEvents int) *Aggregator {
	return &Aggregator{
		windowSize: windowSize,
		maxEvents:  maxEvents,
		logger:     logger.Get(),
	}
}

func (a *Aggregator) AddEvent(event *model.RuntimeEvent) *EventWindow {
	key := event.HostID.String()
	now := time.Now()

	windowInterface, _ := a.windows.LoadOrStore(key, &EventWindow{
		HostID:    event.HostID,
		Events:    make([]*model.RuntimeEvent, 0),
		StartTime: now,
	})

	window := windowInterface.(*EventWindow)

	// Check if window is expired
	if now.Sub(window.StartTime) > a.windowSize {
		// Return old window and create new one
		newWindow := &EventWindow{
			HostID:    event.HostID,
			Events:    make([]*model.RuntimeEvent, 0),
			StartTime: now,
		}
		a.windows.Store(key, newWindow)
		window.EndTime = now
		a.logger.Info("Window expired, returning events",
			zap.String("host_id", key),
			zap.Int("event_count", len(window.Events)),
		)
		return window
	}

	// Add event if under limit
	if len(window.Events) < a.maxEvents {
		window.Events = append(window.Events, event)
	}

	return nil // No window ready yet
}

func (a *Aggregator) GetAndClearWindow(hostID string) *EventWindow {
	windowInterface, ok := a.windows.LoadAndDelete(hostID)
	if !ok {
		return nil
	}
	return windowInterface.(*EventWindow)
}

func (a *Aggregator) GetWindowSize() time.Duration {
	return a.windowSize
}

func (a *Aggregator) GetMaxEvents() int {
	return a.maxEvents
}