package ebpf

import (
	"sync"
	"testing"
	"time"

	"aegis-agent/internal/monitor"
	"aegis-agent/internal/sigma"
	pb "aegis-agent/pkg/api/v1"
)

type testReporter struct {
	mu    sync.Mutex
	batch [][]*pb.RuntimeEvent
}

func (r *testReporter) ReportEvents(events []*pb.RuntimeEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make([]*pb.RuntimeEvent, len(events))
	copy(cp, events)
	r.batch = append(r.batch, cp)
	return nil
}

func (r *testReporter) totalEvents() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := 0
	for _, b := range r.batch {
		total += len(b)
	}
	return total
}

func (r *testReporter) firstEvent() *pb.RuntimeEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.batch) == 0 || len(r.batch[0]) == 0 {
		return nil
	}
	return r.batch[0][0]
}

func TestPipelineReportsOnlySigmaMatchedEvents(t *testing.T) {
	loader := sigma.NewLoader(t.TempDir())
	err := loader.LoadAll([]sigma.Rule{
		{
			ID:    "rule-1",
			Title: "match malware commandline",
			Level: "high",
			Tags:  []string{"attack.T1059"},
			Logsource: sigma.Logsource{
				Category: "process_creation",
				Product:  "linux",
			},
			Detection: sigma.Detection{
				Selections: map[string]any{
					"selection": map[string]any{
						"CommandLine": "malware",
					},
				},
				Condition: "selection",
			},
		},
	})
	if err != nil {
		t.Fatalf("load sigma rules: %v", err)
	}

	reporter := &testReporter{}
	collector := NewCollector("host-1", 10)
	pipeline := NewPipeline(collector, loader, reporter, "host-1", monitor.NewMetrics())

	done := make(chan struct{})
	go pipeline.Run(done)

	collector.events <- Event{HostID: "host-1", Hostname: "test-host", EventType: "process_exec", ProcessName: "bash", CommandLine: "/bin/bash -lc whoami", Timestamp: time.Now().UnixMilli(), PID: 100, PPID: 1, UID: 0}
	collector.events <- Event{HostID: "host-1", Hostname: "test-host", EventType: "process_exec", ProcessName: "bash", CommandLine: "/bin/bash -lc malware --run", Timestamp: time.Now().UnixMilli(), PID: 101, PPID: 1, UID: 0}

	close(done)
	time.Sleep(100 * time.Millisecond)

	if reporter.totalEvents() != 1 {
		t.Fatalf("expected exactly 1 reported event, got %d", reporter.totalEvents())
	}

	reported := reporter.firstEvent()
	if reported == nil {
		t.Fatal("expected a reported event")
	}

	if reported.GetMatchedRuleId() != "rule-1" {
		t.Fatalf("expected matched rule id rule-1, got %q", reported.GetMatchedRuleId())
	}
	if reported.GetSeverity() != "high" {
		t.Fatalf("expected severity high, got %q", reported.GetSeverity())
	}
	if reported.GetMitreId() != "1059" {
		t.Fatalf("expected mitre id 1059, got %q", reported.GetMitreId())
	}
}
