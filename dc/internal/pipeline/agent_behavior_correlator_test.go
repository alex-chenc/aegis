package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dc/internal/model"
)

func TestCorrelateDownloadExecuteHandlesOutOfOrderReplayAndCompleteness(t *testing.T) {
	base := testBehaviorEvent()
	base.RawEventID = "download"
	base.Category, base.Operation = "network", "connect"
	base.AgentSequence = 10
	setResource(base, "8.8.8.8:443", map[string]any{
		"destination_ip": "8.8.8.8", "destination_port": 443, "direction": "outbound",
	})

	create := cloneBehavior(base, "create", 12, time.Second)
	create.Category, create.Operation = "file", "create"
	setResource(create, "/tmp/payload.sh", map[string]any{
		"resolved_path": "/tmp/payload.sh", "inode_created": true,
	})
	chmod := cloneBehavior(base, "chmod", 13, 2*time.Second)
	chmod.Category, chmod.Operation = "file", "chmod"
	setResource(chmod, "/tmp/payload.sh", map[string]any{"resolved_path": "/tmp/payload.sh"})
	execute := cloneBehavior(base, "execute", 14, 3*time.Second)
	execute.Category, execute.Operation, execute.ProcessExe = "process", "exec", "/tmp/payload.sh"
	callback := cloneBehavior(base, "callback", 15, 4*time.Second)
	callback.Category, callback.Operation = "network", "connect"
	setResource(callback, "127.0.0.1:9000", map[string]any{
		"destination_ip": "127.0.0.1", "destination_port": 9000, "direction": "outbound",
	})
	create.Collection = json.RawMessage(`{"visibility":"partial","lost_events_since_last":1,"aggregated_count":1}`)

	// Intentionally out of order and duplicated to model Kafka replay.
	events := []*model.AgentBehaviorEvent{execute, callback, create, base, chmod, create}
	finding := CorrelateDownloadExecute(events, 5*time.Minute)
	if finding == nil {
		t.Fatal("expected correlated finding")
	}
	if finding.RuleKey != "AGB-DOWNLOAD-EXEC-001" || finding.Severity != "critical" {
		t.Fatalf("finding = %#v", finding)
	}
	if len(finding.EvidenceEventIDs) != 5 {
		t.Fatalf("evidence IDs = %v", finding.EvidenceEventIDs)
	}
	if finding.Completeness.Visibility != "partial" || finding.Completeness.SequenceGaps == 0 {
		t.Fatalf("completeness = %#v", finding.Completeness)
	}
	for _, id := range []string{"download", "create", "chmod", "execute", "callback"} {
		if !containsString(finding.EvidenceEventIDs, id) {
			t.Fatalf("missing real evidence ID %s: %v", id, finding.EvidenceEventIDs)
		}
	}
}

func TestCorrelateDownloadExecuteKeepsFailedExecuteAsCounterEvidence(t *testing.T) {
	base := testBehaviorEvent()
	base.RawEventID, base.Category, base.Operation = "download", "network", "connect"
	setResource(base, "8.8.8.8:443", map[string]any{"destination_ip": "8.8.8.8", "direction": "outbound"})
	create := cloneBehavior(base, "create", 2, time.Second)
	create.Category, create.Operation = "file", "create"
	setResource(create, "/tmp/payload", map[string]any{"resolved_path": "/tmp/payload", "inode_created": true})
	execute := cloneBehavior(base, "execute-failed", 3, 2*time.Second)
	execute.Category, execute.Operation, execute.Outcome, execute.ProcessExe = "process", "exec", "failure", "/tmp/payload"

	finding := CorrelateDownloadExecute([]*model.AgentBehaviorEvent{execute, base, create}, 5*time.Minute)
	if finding == nil {
		t.Fatal("expected attempted-chain finding")
	}
	if finding.Verdict != "inconclusive" || strings.Contains(strings.ToLower(finding.Title), "successful") {
		t.Fatalf("failed execution was overstated: %#v", finding)
	}
	if !containsString(finding.CounterEvidenceEventIDs, "execute-failed") {
		t.Fatalf("counter evidence = %v", finding.CounterEvidenceEventIDs)
	}
}

func cloneBehavior(base *model.AgentBehaviorEvent, id string, sequence int64, offset time.Duration) *model.AgentBehaviorEvent {
	copy := *base
	copy.RawEventID = id
	copy.AgentSequence = sequence
	copy.OccurredAt = base.OccurredAt.Add(offset)
	return &copy
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
