package pipeline

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

const testCorrelationHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testProofDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestNormalizeTrustedToolSemanticsRequiresProofAndTrustedSource(t *testing.T) {
	hostID := uuid.New()
	eventID, processID := uuid.NewString(), uuid.NewString()
	input := validToolBehaviorInput(hostID, eventID, processID)

	got, err := normalizeToolTestInput(hostID, input)
	if err != nil {
		t.Fatalf("NormalizeAgentBehavior: %v", err)
	}
	if !isTrustedToolProjection(got) || got.Decision != "audit" || got.RuleID != "" {
		t.Fatalf("trusted projection = %#v", got)
	}
	evidence := decodeJSONObject(got.Evidence)
	semantic := objectField(evidence, "tool_semantics")
	if trusted, _ := semantic["trusted"].(bool); !trusted ||
		stringValueAny(semantic["process_event_id"]) != processID ||
		stringValueAny(semantic["remote_coverage"]) != remoteUnobservable {
		t.Fatalf("canonical semantics = %s", got.Evidence)
	}
	if stringValueAny(evidence["remote_coverage"]) != remoteUnobservable {
		t.Fatalf("self-reported remote coverage was trusted: %s", got.Evidence)
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "execution unit inference", mutate: func(value map[string]any) {
			value["collection"].(map[string]any)["source"] = "execution_unit"
		}},
		{name: "activity window inference", mutate: func(value map[string]any) {
			value["collection"].(map[string]any)["source"] = "activity_window"
		}},
		{name: "spoofed proof", mutate: func(value map[string]any) {
			value["evidence"].(map[string]any)["trusted_proof"].(map[string]any)["verified"] = false
		}},
		{name: "artifact-only proof", mutate: func(value map[string]any) {
			value["evidence"].(map[string]any)["trusted_proof"].(map[string]any)["verifier"] = "artifact_sha256"
		}},
		{name: "raw token", mutate: func(value map[string]any) {
			value["evidence"].(map[string]any)["correlation_token"] = "must-never-persist"
		}},
		{name: "process name guess", mutate: func(value map[string]any) {
			value["collection"].(map[string]any)["source"] = "process_name"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := deepCopyMap(input)
			test.mutate(candidate)
			if _, err := normalizeToolTestInput(hostID, candidate); err == nil {
				t.Fatal("expected untrusted semantic rejection")
			}
		})
	}
}

func TestCorrelateTrustedToolSemanticsIsIdempotentOutOfOrderAndEvidenceBacked(t *testing.T) {
	localHost := uuid.New()
	toolID, processID, resourceID, remoteID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	tool, err := normalizeToolTestInput(localHost, validToolBehaviorInput(localHost, toolID, processID))
	if err != nil {
		t.Fatal(err)
	}
	semantic := objectField(decodeJSONObject(tool.Evidence), "tool_semantics")
	semantic["resource_event_ids"] = []any{resourceID}
	semantic["remote_sensor_event_ids"] = []any{remoteID}
	tool.Evidence = mustJSON(map[string]any{
		"correlation_token_hash": testCorrelationHash,
		"tool_semantics":         semantic,
	}, map[string]any{})

	process := sameScopeEvidence(tool, processID, "process")
	resource := sameScopeEvidence(tool, resourceID, "file")

	// Duplicate and out-of-order delivery produces one deterministic graph.
	first := CorrelateTrustedToolSemantics([]*model.AgentBehaviorEvent{resource, tool, process, tool})
	second := CorrelateTrustedToolSemantics([]*model.AgentBehaviorEvent{process, tool, resource})
	if !reflect.DeepEqual(first, second) || len(first.Nodes) != 3 || len(first.Edges) != 2 {
		t.Fatalf("non-idempotent graph:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.RemoteCoverage != remoteUnobservable || !containsString(first.Limitations, remoteUnobservable) {
		t.Fatalf("remote without local sensor evidence = %#v", first)
	}

	remoteHost := uuid.MustParse(stringValueAny(semantic["remote_host_id"]))
	remoteUnit := uuid.MustParse(stringValueAny(semantic["remote_execution_unit_id"]))
	remote := trustedRemoteOSEvent(remoteID, remoteHost, remoteUnit, tool.OccurredAt.Add(time.Second))
	verified := CorrelateTrustedToolSemantics([]*model.AgentBehaviorEvent{remote, tool, resource, process})
	if verified.RemoteCoverage != "sensor_verified" || len(verified.Nodes) != 4 || len(verified.Edges) != 3 {
		t.Fatalf("remote evidence did not correlate = %#v", verified)
	}

	remote.Evidence = mustJSON(map[string]any{"correlation_token_hash": testProofDigest}, map[string]any{})
	mismatch := CorrelateTrustedToolSemantics([]*model.AgentBehaviorEvent{remote, tool, resource, process})
	if mismatch.RemoteCoverage != remoteUnobservable {
		t.Fatalf("hash was treated as authentication = %#v", mismatch)
	}
}

func TestCorrelationHashAloneCannotCreateTrustedToolGraphOrAction(t *testing.T) {
	plain := testBehaviorEvent()
	plain.RawEventID = uuid.NewString()
	plain.Evidence = mustJSON(map[string]any{"correlation_token_hash": testCorrelationHash}, map[string]any{})
	if graph := CorrelateTrustedToolSemantics([]*model.AgentBehaviorEvent{plain}); len(graph.Nodes) != 0 || len(graph.Edges) != 0 {
		t.Fatalf("correlation hash established trust: %#v", graph)
	}

	tool := testBehaviorEvent()
	tool.RawEventID, tool.Category, tool.Operation = uuid.NewString(), "tool", "tool_call_started"
	store := &fakeAgentFindingStore{events: []*model.AgentBehaviorEvent{tool}}
	engine := NewAgentRuleEngine(store)
	result, err := engine.ProcessBehavior(context.Background(), tool, AgentRuleProcessingOptions{
		RulesEnabled: true, FindingsEnabled: true, AlertsEnabled: true,
		ActionFlags: AgentActionFeatureFlags{ActionEnabled: true},
	})
	if err != nil || result.HitCount != 0 || len(result.FindingUpdates) != 0 || len(result.ActionUpdates) != 0 || len(store.findings) != 0 {
		t.Fatalf("tool semantics triggered enforcement: result=%#v err=%v findings=%v", result, err, store.findings)
	}
}

func TestFindingPersistsVerifiedRemoteToolEvidenceWithoutUpgradingUnitCoverage(t *testing.T) {
	localHost := uuid.New()
	toolID, processID := uuid.NewString(), uuid.NewString()
	tool, err := normalizeToolTestInput(localHost, validToolBehaviorInput(localHost, toolID, processID))
	if err != nil {
		t.Fatal(err)
	}
	semantic := objectField(decodeJSONObject(tool.Evidence), "tool_semantics")
	remoteID := toolStringSlice(semantic["remote_sensor_event_ids"])[0]
	remoteHost := uuid.MustParse(stringValueAny(semantic["remote_host_id"]))
	remoteUnit := uuid.MustParse(stringValueAny(semantic["remote_execution_unit_id"]))
	process := sameScopeEvidence(tool, processID, "process")
	remote := trustedRemoteOSEvent(remoteID, remoteHost, remoteUnit, tool.OccurredAt.Add(time.Second))
	finding := &model.AgentSecurityFinding{
		EvidenceEventIDs: mustJSON([]string{processID}, []string{}),
		EvidenceGraph:    mustJSON(map[string]any{"nodes": []any{}}, map[string]any{}),
	}
	attachTrustedToolSemantics(finding, []*model.AgentBehaviorEvent{remote, process, tool})

	root := decodeJSONObject(finding.EvidenceGraph)
	persisted := objectField(root, "tool_semantics")
	if stringValueAny(persisted["remote_coverage"]) != "sensor_verified" ||
		!containsJSONValue(finding.EvidenceEventIDs, remoteID) ||
		!containsJSONValue(finding.EvidenceEventIDs, toolID) {
		t.Fatalf("verified remote graph was not persisted: ids=%s graph=%s", finding.EvidenceEventIDs, finding.EvidenceGraph)
	}
	// Correlation enriches the finding only; it never mutates execution-unit
	// enforcement coverage to full_enforcement.
	if tool.CommandVisibility != "complete" || stringValueAny(semantic["remote_coverage"]) != remoteUnobservable {
		t.Fatalf("raw projection coverage was incorrectly upgraded: %s", tool.Evidence)
	}
}

func TestRemoteSensorEvidenceRejectsSpoofOldAndUntrustedCollection(t *testing.T) {
	hostID, unitID := uuid.New(), uuid.New()
	eventID := uuid.NewString()
	selector := RemoteEvidenceSelector{
		EventID: eventID, HostID: hostID, ExecutionUnitID: unitID,
		CorrelationHash: testCorrelationHash, ToolOccurredAt: time.Now().UTC(),
	}
	valid := trustedRemoteOSEvent(eventID, hostID, unitID, selector.ToolOccurredAt)
	if !MatchesTrustedRemoteEvidence(selector, valid, agentCorrelationWindow) {
		t.Fatal("valid local Aegis OS evidence was rejected")
	}
	tests := []struct {
		name   string
		mutate func(*model.AgentBehaviorEvent)
	}{
		{name: "spoof host", mutate: func(event *model.AgentBehaviorEvent) { event.HostID = uuid.New() }},
		{name: "old event", mutate: func(event *model.AgentBehaviorEvent) {
			event.OccurredAt = selector.ToolOccurredAt.Add(-agentCorrelationWindow - time.Second)
		}},
		{name: "untrusted source", mutate: func(event *model.AgentBehaviorEvent) {
			event.Collection = mustJSON(map[string]any{"source": "adapter_hook", "sensor": "claimed", "visibility": "complete", "attribution_confidence": "confirmed", "lost_events_since_last": 0, "truncated_fields": []string{}}, map[string]any{})
		}},
		{name: "unconfirmed attribution", mutate: func(event *model.AgentBehaviorEvent) {
			collection := decodeJSONObject(event.Collection)
			collection["attribution_confidence"] = "probable"
			event.Collection = mustJSON(collection, map[string]any{})
		}},
		{name: "partial evidence", mutate: func(event *model.AgentBehaviorEvent) { event.CommandVisibility = "partial" }},
		{name: "wrong hash", mutate: func(event *model.AgentBehaviorEvent) {
			event.Evidence = mustJSON(map[string]any{"correlation_token_hash": testProofDigest}, map[string]any{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copy := *valid
			test.mutate(&copy)
			if MatchesTrustedRemoteEvidence(selector, &copy, agentCorrelationWindow) {
				t.Fatal("untrusted remote evidence was accepted")
			}
		})
	}
}

func validToolBehaviorInput(hostID uuid.UUID, eventID, processID string) map[string]any {
	remoteHost, remoteUnit, remoteSensor := uuid.NewString(), uuid.NewString(), uuid.NewString()
	return map[string]any{
		"schema": "aegis.agent_behavior.v1", "event_id": eventID, "host_id": hostID.String(),
		"host_boot_id": "boot-tool", "agent_sequence": float64(7),
		"instance_id": uuid.NewString(), "session_id": uuid.NewString(), "execution_unit_id": uuid.NewString(),
		"correlation_id": testCorrelationHash, "occurred_at": "2026-08-03T10:00:00Z", "occurred_monotonic_ns": float64(42),
		"category": "tool", "operation": "tool_call_started", "outcome": "unknown", "decision": "audit", "severity": "info",
		"attribution_confidence": "confirmed",
		"actor":                  map[string]any{"pid": float64(1234), "ppid": float64(1), "start_ticks": "777", "name": "worker"},
		"resource": map[string]any{
			"type": "tool", "identity": "filesystem.read", "attributes": map[string]any{
				"tool_call_id": uuid.NewString(), "process_event_id": processID,
				"resource_event_ids": []any{}, "remote_host_id": remoteHost,
				"remote_execution_unit_id": remoteUnit, "remote_sensor_event_ids": []any{remoteSensor},
			},
		},
		"collection": map[string]any{"source": "agent_official", "sensor": "official_audit", "visibility": "complete", "lost_events_since_last": float64(0)},
		"evidence": map[string]any{
			"trusted_proof":          map[string]any{"verified": true, "verifier": "ed25519", "proof_digest": testProofDigest, "issued_at": "2026-08-03T10:00:01Z"},
			"correlation_token_hash": testCorrelationHash, "remote_coverage": "sensor_verified",
		},
	}
}

func normalizeToolTestInput(hostID uuid.UUID, input map[string]any) (*model.AgentBehaviorEvent, error) {
	raw, _ := json.Marshal(input)
	return NormalizeAgentBehavior(stringValueAny(input["event_id"]), hostID, string(raw))
}

func deepCopyMap(input map[string]any) map[string]any {
	raw, _ := json.Marshal(input)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}

func sameScopeEvidence(tool *model.AgentBehaviorEvent, eventID, category string) *model.AgentBehaviorEvent {
	copy := *tool
	copy.RawEventID = eventID
	copy.Category = category
	copy.Operation = "observed"
	copy.Decision = "audit"
	copy.RuleID = ""
	copy.Evidence = mustJSON(map[string]any{}, map[string]any{})
	copy.OccurredAt = copy.OccurredAt.Add(time.Second)
	return &copy
}

func trustedRemoteOSEvent(eventID string, hostID, unitID uuid.UUID, occurredAt time.Time) *model.AgentBehaviorEvent {
	return &model.AgentBehaviorEvent{
		RawEventID: eventID, HostID: hostID, ExecutionUnitID: &unitID,
		Category: "process", CommandVisibility: "complete", OccurredAt: occurredAt,
		Collection: mustJSON(map[string]any{
			"source": "ebpf", "sensor": "execve", "visibility": "complete",
			"attribution_confidence": "confirmed", "lost_events_since_last": 0,
			"truncated_fields": []string{}, "aggregated_count": 1, "coverage_level": "monitor_only",
		}, map[string]any{}),
		Evidence: mustJSON(map[string]any{"correlation_token_hash": testCorrelationHash}, map[string]any{}),
	}
}
