package pipeline

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

type fakeAgentFindingStore struct {
	events          []*model.AgentBehaviorEvent
	remoteEvents    []*model.AgentBehaviorEvent
	remoteSelectors []RemoteEvidenceSelector
	findings        map[string]*model.AgentSecurityFinding
	alerts          []bool
}

type countingFindingActionCoordinator struct{ calls int }

func (c *countingFindingActionCoordinator) ConsiderFinding(
	_ context.Context,
	_ *model.AgentSecurityFinding,
	_ AgentActionFeatureFlags,
) (*AgentGuardActionUpdate, error) {
	c.calls++
	return &AgentGuardActionUpdate{Action: "freeze_execution_unit", Status: "dispatching"}, nil
}

func TestRuleEngineProductionPathFetchesAndPersistsBoundedRemoteEvidence(t *testing.T) {
	localHost := uuid.New()
	toolID, downloadID, createID, executeID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	tool, err := normalizeToolTestInput(localHost, validToolBehaviorInput(localHost, toolID, executeID))
	if err != nil {
		t.Fatal(err)
	}
	semantic := objectField(decodeJSONObject(tool.Evidence), "tool_semantics")
	remoteID := toolStringSlice(semantic["remote_sensor_event_ids"])[0]
	remoteHost := uuid.MustParse(stringValueAny(semantic["remote_host_id"]))
	remoteUnit := uuid.MustParse(stringValueAny(semantic["remote_execution_unit_id"]))

	download := sameScopeEvidence(tool, downloadID, "network")
	download.Operation, download.Outcome, download.AgentSequence = "connect", "success", 1
	download.OccurredAt = tool.OccurredAt.Add(-3 * time.Second)
	setResource(download, "8.8.8.8:443", map[string]any{"destination_ip": "8.8.8.8", "direction": "outbound"})
	create := sameScopeEvidence(tool, createID, "file")
	create.Operation, create.Outcome, create.AgentSequence = "create", "success", 2
	create.OccurredAt = tool.OccurredAt.Add(-2 * time.Second)
	setResource(create, "/tmp/remote-payload", map[string]any{"resolved_path": "/tmp/remote-payload", "inode_created": true})
	execute := sameScopeEvidence(tool, executeID, "process")
	execute.Operation, execute.Outcome, execute.ProcessExe, execute.AgentSequence = "exec", "success", "/tmp/remote-payload", 3
	execute.OccurredAt = tool.OccurredAt.Add(-time.Second)
	remote := trustedRemoteOSEvent(remoteID, remoteHost, remoteUnit, tool.OccurredAt.Add(time.Second))

	store := &fakeAgentFindingStore{
		events:       []*model.AgentBehaviorEvent{tool, execute, download, create},
		remoteEvents: []*model.AgentBehaviorEvent{remote},
	}
	actions := &countingFindingActionCoordinator{}
	engine := NewAgentRuleEngineWithActions(store, actions)
	result, err := engine.ProcessBehavior(context.Background(), tool, AgentRuleProcessingOptions{
		RulesEnabled: true, FindingsEnabled: true,
		ActionFlags: AgentActionFeatureFlags{ActionEnabled: true, FreezeEnabled: true, PublishEnabled: true},
	})
	if err != nil {
		t.Fatalf("ProcessBehavior: %v", err)
	}
	if len(store.remoteSelectors) != 1 || store.remoteSelectors[0].EventID != remoteID ||
		store.remoteSelectors[0].HostID != remoteHost || store.remoteSelectors[0].ExecutionUnitID != remoteUnit {
		t.Fatalf("unbounded or incorrect remote selectors: %#v", store.remoteSelectors)
	}
	if result.HitCount != 0 || len(result.ActionUpdates) != 0 || actions.calls != 0 {
		t.Fatalf("tool enrichment evaluated rules or actions: %#v", result)
	}
	var finding *model.AgentSecurityFinding
	for key, candidate := range store.findings {
		if len(key) >= len("correlation:v1:") && key[:len("correlation:v1:")] == "correlation:v1:" {
			finding = candidate
			break
		}
	}
	if finding == nil || !containsJSONValue(finding.EvidenceEventIDs, remoteID) ||
		stringValueAny(objectField(decodeJSONObject(finding.EvidenceGraph), "tool_semantics")["remote_coverage"]) != "sensor_verified" {
		t.Fatalf("remote evidence graph was not persisted: finding=%#v", finding)
	}

	for _, test := range []struct {
		name   string
		mutate func(*model.AgentBehaviorEvent)
	}{
		{name: "spoofed host", mutate: func(event *model.AgentBehaviorEvent) { event.HostID = uuid.New() }},
		{name: "stale sensor", mutate: func(event *model.AgentBehaviorEvent) {
			event.OccurredAt = tool.OccurredAt.Add(-agentCorrelationWindow - time.Second)
		}},
		{name: "untrusted source", mutate: func(event *model.AgentBehaviorEvent) {
			collection := decodeJSONObject(event.Collection)
			collection["source"] = "adapter_hook"
			event.Collection = mustJSON(collection, map[string]any{})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalidRemote := *remote
			test.mutate(&invalidRemote)
			invalidStore := &fakeAgentFindingStore{
				events:       []*model.AgentBehaviorEvent{tool, execute, download, create},
				remoteEvents: []*model.AgentBehaviorEvent{&invalidRemote},
			}
			_, err := NewAgentRuleEngine(invalidStore).ProcessBehavior(
				context.Background(), tool,
				AgentRuleProcessingOptions{RulesEnabled: true, FindingsEnabled: true},
			)
			if err != nil {
				t.Fatal(err)
			}
			checked := false
			for key, invalidFinding := range invalidStore.findings {
				if len(key) < len("correlation:v1:") || key[:len("correlation:v1:")] != "correlation:v1:" {
					continue
				}
				checked = true
				semanticGraph := objectField(decodeJSONObject(invalidFinding.EvidenceGraph), "tool_semantics")
				if containsJSONValue(invalidFinding.EvidenceEventIDs, remoteID) ||
					stringValueAny(semanticGraph["remote_coverage"]) != remoteUnobservable {
					t.Fatalf("invalid remote evidence entered finding: ids=%s graph=%s", invalidFinding.EvidenceEventIDs, invalidFinding.EvidenceGraph)
				}
			}
			if !checked {
				t.Fatal("correlated finding was not produced")
			}
		})
	}
}

func (f *fakeAgentFindingStore) ListRemoteBehaviorEvidence(
	_ context.Context,
	selectors []RemoteEvidenceSelector,
	_ time.Duration,
) ([]*model.AgentBehaviorEvent, error) {
	f.remoteSelectors = append([]RemoteEvidenceSelector(nil), selectors...)
	return f.remoteEvents, nil
}

func (f *fakeAgentFindingStore) ListBehaviorWindow(
	_ context.Context,
	_ *model.AgentBehaviorEvent,
	_ time.Duration,
) ([]*model.AgentBehaviorEvent, error) {
	return f.events, nil
}

func (f *fakeAgentFindingStore) UpsertAgentFinding(
	_ context.Context,
	finding *model.AgentSecurityFinding,
	alertEnabled bool,
) (AgentFindingWriteResult, error) {
	if f.findings == nil {
		f.findings = make(map[string]*model.AgentSecurityFinding)
	}
	_, exists := f.findings[finding.FindingKey]
	f.findings[finding.FindingKey] = finding
	f.alerts = append(f.alerts, alertEnabled)
	return AgentFindingWriteResult{
		FindingID: finding.ID,
		Created:   !exists,
		Changed:   true,
	}, nil
}

func TestRuleEngineLayersRulesFindingsAndAlerts(t *testing.T) {
	event := testBehaviorEvent()
	event.RawEventID = "00000000-0000-4000-8000-000000000001"
	event.Category, event.Operation, event.Outcome = "file", "read_observed", "success"
	setResource(event, "/etc/shadow", map[string]any{"resolved_path": "/etc/shadow"})
	store := &fakeAgentFindingStore{events: []*model.AgentBehaviorEvent{event}}
	engine := NewAgentRuleEngine(store)

	result, err := engine.ProcessBehavior(context.Background(), event, AgentRuleProcessingOptions{
		RulesEnabled: true,
	})
	if err != nil {
		t.Fatalf("rules only: %v", err)
	}
	if result.HitCount != 1 || len(store.findings) != 0 {
		t.Fatalf("rules-only result=%#v findings=%d", result, len(store.findings))
	}

	result, err = engine.ProcessBehavior(context.Background(), event, AgentRuleProcessingOptions{
		RulesEnabled: true, FindingsEnabled: true,
	})
	if err != nil {
		t.Fatalf("findings: %v", err)
	}
	if result.HitCount != 1 || len(result.FindingUpdates) != 1 || len(store.findings) != 1 || store.alerts[0] {
		t.Fatalf("findings result=%#v store=%#v alerts=%v", result, store.findings, store.alerts)
	}

	_, err = engine.ProcessBehavior(context.Background(), event, AgentRuleProcessingOptions{
		RulesEnabled: true, FindingsEnabled: true, AlertsEnabled: true,
	})
	if err != nil {
		t.Fatalf("alerts: %v", err)
	}
	if !store.alerts[len(store.alerts)-1] {
		t.Fatal("alert projection was not enabled")
	}
}

func TestFindingBuildersUseOnlyRealEvidenceIDsAndStableKeys(t *testing.T) {
	event := testBehaviorEvent()
	event.RawEventID = "00000000-0000-4000-8000-000000000001"
	event.Category, event.Operation = "network", "connect"
	setResource(event, "8.8.8.8:443", map[string]any{"destination_ip": "8.8.8.8"})
	event = ClassifyAgentBehavior(event, RuleEvaluationOptions{})
	hit := EvaluateBuiltinRules(event, RuleEvaluationOptions{})[0]

	first := BuildSingleEventFinding(event, hit)
	second := BuildSingleEventFinding(event, hit)
	if first.ID != second.ID || first.FindingKey != second.FindingKey {
		t.Fatalf("single event key is not deterministic: %#v %#v", first, second)
	}
	var evidence []string
	if err := json.Unmarshal(first.EvidenceEventIDs, &evidence); err != nil ||
		len(evidence) != 1 || evidence[0] != event.RawEventID {
		t.Fatalf("evidence = %s err=%v", first.EvidenceEventIDs, err)
	}
	if string(first.EvidenceGraph) == "" ||
		containsJSONValue(first.EvidenceGraph, event.ResourceIdentity) {
		t.Fatalf("evidence graph leaked content: %s", first.EvidenceGraph)
	}

	correlated := &CorrelatedFinding{
		RuleKey: downloadExecuteRuleKey, RuleVersion: 1, RuleDigest: downloadExecuteRuleDigest,
		Title: "Agent download and execute chain", Severity: "high", Verdict: "suspicious",
		Confidence: 0.86, RecommendedAction: "alert",
		EvidenceEventIDs: []string{
			"00000000-0000-4000-8000-000000000001",
			"00000000-0000-4000-8000-000000000002",
			"00000000-0000-4000-8000-000000000003",
		},
		CounterEvidenceEventIDs: []string{"00000000-0000-4000-8000-000000000003"},
		AttackStages:            []string{"ingress_tool_transfer", "resource_development", "execution"},
		Completeness:            EvidenceCompleteness{Visibility: "partial", SequenceGaps: 1},
		FirstObservedAt:         event.OccurredAt,
		LastObservedAt:          event.OccurredAt.Add(time.Second),
	}
	correlationFinding := BuildCorrelatedSecurityFinding(event, correlated)
	if correlationFinding == nil ||
		correlationFinding.FindingKey != "correlation:v1:AGB-DOWNLOAD-EXEC-001:00000000-0000-4000-8000-000000000003" {
		t.Fatalf("correlation finding = %#v", correlationFinding)
	}
}

func containsJSONValue(raw json.RawMessage, value string) bool {
	return value != "" && string(raw) != "" && json.Valid(raw) &&
		containsRawString(string(raw), value)
}

func containsRawString(raw, wanted string) bool {
	for index := 0; index+len(wanted) <= len(raw); index++ {
		if raw[index:index+len(wanted)] == wanted {
			return true
		}
	}
	return false
}
