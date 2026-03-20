package service

import (
	"testing"

	"aegis-system/internal/pipeline"
)

func TestRuntimePipelineService_GetStats(t *testing.T) {
	// Create service with nil dependencies (for stats testing)
	s := &RuntimePipelineService{
		aggregator: pipeline.NewHostWindowAggregator(0, nil),
	}

	stats := s.GetStats()
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}

	if _, ok := stats["active_windows"]; !ok {
		t.Error("expected active_windows in stats")
	}
	if _, ok := stats["total_events"]; !ok {
		t.Error("expected total_events in stats")
	}
}
