package pipeline

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

const agentCorrelationWindow = 5 * time.Minute

type AgentFindingWriteResult struct {
	FindingID uuid.UUID
	Created   bool
	Changed   bool
}

type AgentFindingStore interface {
	ListBehaviorWindow(context.Context, *model.AgentBehaviorEvent, time.Duration) ([]*model.AgentBehaviorEvent, error)
	ListRemoteBehaviorEvidence(context.Context, []RemoteEvidenceSelector, time.Duration) ([]*model.AgentBehaviorEvent, error)
	UpsertAgentFinding(context.Context, *model.AgentSecurityFinding, bool) (AgentFindingWriteResult, error)
}

type AgentRuleProcessingOptions struct {
	RulesEnabled    bool
	FindingsEnabled bool
	AlertsEnabled   bool
	Evaluation      RuleEvaluationOptions
	ActionFlags     AgentActionFeatureFlags
}

type AgentFindingUpdate struct {
	FindingID uuid.UUID
	Created   bool
	Changed   bool
	Severity  string
}

type AgentRuleProcessingResult struct {
	HitCount       int
	FindingUpdates []AgentFindingUpdate
	ActionUpdates  []AgentGuardActionUpdate
}

type AgentFindingActionCoordinator interface {
	ConsiderFinding(context.Context, *model.AgentSecurityFinding, AgentActionFeatureFlags) (*AgentGuardActionUpdate, error)
}

type AgentRuleEngine struct {
	store             AgentFindingStore
	actionCoordinator AgentFindingActionCoordinator
}

func NewAgentRuleEngine(store AgentFindingStore) *AgentRuleEngine {
	return &AgentRuleEngine{store: store}
}

func NewAgentRuleEngineWithActions(
	store AgentFindingStore,
	actionCoordinator AgentFindingActionCoordinator,
) *AgentRuleEngine {
	return &AgentRuleEngine{store: store, actionCoordinator: actionCoordinator}
}

func (e *AgentRuleEngine) ProcessBehavior(
	ctx context.Context,
	event *model.AgentBehaviorEvent,
	options AgentRuleProcessingOptions,
) (AgentRuleProcessingResult, error) {
	result := AgentRuleProcessingResult{}
	if event == nil || !options.RulesEnabled {
		return result, nil
	}
	// Trusted tool semantics never evaluate rules or reach the action
	// coordinator. A late tool event may only materialize/enrich a finding when
	// an independently sufficient OS-event correlation already exists.
	if event.Category == "tool" {
		if !options.FindingsEnabled || e == nil || e.store == nil {
			return result, nil
		}
		events, err := e.store.ListBehaviorWindow(ctx, event, agentCorrelationWindow)
		if err != nil {
			return result, err
		}
		correlated := CorrelateDownloadExecute(events, agentCorrelationWindow)
		if correlated == nil {
			return result, nil
		}
		selectors := TrustedRemoteEvidenceSelectors(events, correlated.EvidenceEventIDs)
		if len(selectors) > 0 {
			remoteEvents, err := e.store.ListRemoteBehaviorEvidence(ctx, selectors, agentCorrelationWindow)
			if err != nil {
				return result, err
			}
			events = append(events, remoteEvents...)
		}
		scope := correlationScopeEvent(events, correlated.AnchorEventID)
		if scope == nil {
			return result, nil
		}
		finding := BuildCorrelatedSecurityFinding(scope, correlated)
		attachTrustedToolSemantics(finding, events)
		write, err := e.store.UpsertAgentFinding(ctx, finding, options.AlertsEnabled)
		if err != nil {
			return result, err
		}
		result.FindingUpdates = appendFindingUpdate(result.FindingUpdates, write, finding.Severity)
		return result, nil
	}
	classified := ClassifyAgentBehavior(event, options.Evaluation)
	hits := EvaluateBuiltinRules(classified, options.Evaluation)
	result.HitCount = len(hits)
	if !options.FindingsEnabled || e == nil || e.store == nil {
		return result, nil
	}
	for _, hit := range hits {
		finding := BuildSingleEventFinding(classified, hit)
		write, err := e.store.UpsertAgentFinding(ctx, finding, options.AlertsEnabled)
		if err != nil {
			return result, err
		}
		result.FindingUpdates = appendFindingUpdate(result.FindingUpdates, write, finding.Severity)
		if err := e.considerAction(ctx, finding, options.ActionFlags, &result); err != nil {
			return result, err
		}
	}
	events, err := e.store.ListBehaviorWindow(ctx, classified, agentCorrelationWindow)
	if err != nil {
		return result, err
	}
	correlated := CorrelateDownloadExecute(events, agentCorrelationWindow)
	if correlated == nil {
		return result, nil
	}
	selectors := TrustedRemoteEvidenceSelectors(events, correlated.EvidenceEventIDs)
	if len(selectors) > 0 {
		remoteEvents, err := e.store.ListRemoteBehaviorEvidence(ctx, selectors, agentCorrelationWindow)
		if err != nil {
			return result, err
		}
		events = append(events, remoteEvents...)
	}
	finding := BuildCorrelatedSecurityFinding(classified, correlated)
	attachTrustedToolSemantics(finding, events)
	write, err := e.store.UpsertAgentFinding(ctx, finding, options.AlertsEnabled)
	if err != nil {
		return result, err
	}
	result.FindingUpdates = appendFindingUpdate(result.FindingUpdates, write, finding.Severity)
	if err := e.considerAction(ctx, finding, options.ActionFlags, &result); err != nil {
		return result, err
	}
	return result, nil
}

func correlationScopeEvent(events []*model.AgentBehaviorEvent, eventID string) *model.AgentBehaviorEvent {
	for _, event := range events {
		if event != nil && event.RawEventID == eventID && event.Category != "tool" {
			return event
		}
	}
	return nil
}

func attachTrustedToolSemantics(finding *model.AgentSecurityFinding, events []*model.AgentBehaviorEvent) {
	if finding == nil {
		return
	}
	var findingIDs []string
	if json.Unmarshal(finding.EvidenceEventIDs, &findingIDs) != nil {
		return
	}
	evidenceSet := make(map[string]struct{}, len(findingIDs))
	for _, id := range findingIDs {
		evidenceSet[id] = struct{}{}
	}
	graph := CorrelateTrustedToolSemantics(events)
	qualifiedTools := map[string]struct{}{}
	for _, edge := range graph.Edges {
		if edge.Relation != "invoked_process" {
			continue
		}
		if _, relevant := evidenceSet[edge.To]; relevant {
			qualifiedTools[edge.From] = struct{}{}
		}
	}
	if len(qualifiedTools) == 0 {
		return
	}
	selectedNodes := map[string]ToolSemanticNode{}
	selectedEdges := make([]ToolSemanticEdge, 0)
	linkedProcesses := map[string]struct{}{}
	for _, edge := range graph.Edges {
		if _, selected := qualifiedTools[edge.From]; selected {
			selectedEdges = append(selectedEdges, edge)
			if edge.Relation == "invoked_process" {
				linkedProcesses[edge.To] = struct{}{}
			}
		}
	}
	for _, edge := range graph.Edges {
		if _, selected := linkedProcesses[edge.From]; selected && edge.Relation == "accessed_resource" {
			selectedEdges = append(selectedEdges, edge)
		}
	}
	for _, edge := range selectedEdges {
		for _, node := range graph.Nodes {
			if node.ID == edge.From || node.ID == edge.To {
				selectedNodes[node.Kind+":"+node.ID] = node
			}
		}
	}
	selected := ToolSemanticGraph{
		Nodes:       make([]ToolSemanticNode, 0, len(selectedNodes)),
		Edges:       selectedEdges,
		Limitations: []string{},
	}
	remoteLinked := map[string]bool{}
	for _, edge := range selectedEdges {
		if edge.Relation == "observed_remote_activity" {
			remoteLinked[edge.From] = true
		}
	}
	for _, event := range events {
		if event == nil {
			continue
		}
		if _, selectedTool := qualifiedTools[event.RawEventID]; !selectedTool {
			continue
		}
		semantic := objectField(decodeJSONObject(event.Evidence), "tool_semantics")
		if stringValueAny(semantic["remote_host_id"]) == "" {
			continue
		}
		if remoteLinked[event.RawEventID] {
			if selected.RemoteCoverage != remoteUnobservable {
				selected.RemoteCoverage = "sensor_verified"
			}
		} else {
			selected.RemoteCoverage = remoteUnobservable
			selected.Limitations = append(selected.Limitations, remoteUnobservable)
		}
	}
	selected.Limitations = uniqueSortedStrings(selected.Limitations)
	for _, node := range selectedNodes {
		selected.Nodes = append(selected.Nodes, node)
		selected.EventIDs = append(selected.EventIDs, node.ID)
	}
	sort.Slice(selected.Nodes, func(i, j int) bool {
		return selected.Nodes[i].Kind+selected.Nodes[i].ID < selected.Nodes[j].Kind+selected.Nodes[j].ID
	})
	selected.EventIDs = uniqueSortedStrings(selected.EventIDs)
	findingIDs = uniqueSortedStrings(append(findingIDs, selected.EventIDs...))
	finding.EvidenceEventIDs = mustJSON(findingIDs, []string{})
	root := decodeJSONObject(finding.EvidenceGraph)
	root["tool_semantics"] = selected
	finding.EvidenceGraph = mustJSON(root, map[string]any{})
}

func (e *AgentRuleEngine) ProcessGuardEvent(
	ctx context.Context,
	event *model.RuntimeEvent,
	options AgentRuleProcessingOptions,
) (AgentRuleProcessingResult, error) {
	result := AgentRuleProcessingResult{}
	if event == nil || !options.RulesEnabled {
		return result, nil
	}
	finding, err := NormalizeAgentGuardEscapeFinding(event)
	if err != nil {
		return result, err
	}
	result.HitCount = 1
	if !options.FindingsEnabled || e == nil || e.store == nil {
		return result, nil
	}
	write, err := e.store.UpsertAgentFinding(ctx, finding, options.AlertsEnabled)
	if err != nil {
		return result, err
	}
	result.FindingUpdates = appendFindingUpdate(result.FindingUpdates, write, finding.Severity)
	if err := e.considerAction(ctx, finding, options.ActionFlags, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (e *AgentRuleEngine) considerAction(
	ctx context.Context,
	finding *model.AgentSecurityFinding,
	flags AgentActionFeatureFlags,
	result *AgentRuleProcessingResult,
) error {
	if e == nil || e.actionCoordinator == nil || !flags.ActionEnabled {
		return nil
	}
	update, err := e.actionCoordinator.ConsiderFinding(ctx, finding, flags)
	if err != nil {
		return err
	}
	if update != nil && update.ActionID != uuid.Nil {
		result.ActionUpdates = append(result.ActionUpdates, *update)
	}
	return nil
}

func appendFindingUpdate(
	updates []AgentFindingUpdate,
	write AgentFindingWriteResult,
	severity string,
) []AgentFindingUpdate {
	if !write.Changed {
		return updates
	}
	return append(updates, AgentFindingUpdate{
		FindingID: write.FindingID,
		Created:   write.Created,
		Changed:   write.Changed,
		Severity:  severity,
	})
}

func BuildSingleEventFinding(event *model.AgentBehaviorEvent, hit AgentRuleHit) *model.AgentSecurityFinding {
	if event == nil || event.RawEventID == "" {
		return nil
	}
	findingKey := "single:v1:" + hit.RuleKey + ":" + event.RawEventID
	evidenceIDs := []string{event.RawEventID}
	graph := evidenceGraph(evidenceIDs, nil, hit.RuleKey)
	return &model.AgentSecurityFinding{
		ID:                  stableFindingID(findingKey),
		FindingKey:          findingKey,
		HostID:              event.HostID,
		InstanceID:          event.InstanceID,
		SessionID:           event.SessionID,
		ExecutionUnitID:     event.ExecutionUnitID,
		PolicyID:            event.PolicyID,
		PolicyVersion:       event.PolicyVersion,
		Title:               hit.RuleName,
		Severity:            hit.Severity,
		Verdict:             "suspicious",
		Confidence:          hit.Confidence,
		Status:              "open",
		DecisionSources:     mustJSON([]string{"agent_guard_rule"}, []string{}),
		RuleHits:            mustJSON([]AgentRuleHit{hit}, []AgentRuleHit{}),
		EvidenceEventIDs:    mustJSON(evidenceIDs, []string{}),
		EvidenceGraph:       graph,
		AttackStages:        mustJSON(nonEmptyStrings([]string{hit.AttackStage}), []string{}),
		Summary:             "Agent Guard built-in rule matched an immutable behavior event.",
		RecommendedAction:   auditOnlyRecommendation(hit.Decision),
		FirstObservedAt:     event.OccurredAt,
		LastObservedAt:      event.OccurredAt,
		EvidenceSourceTable: "agent_behavior_events",
	}
}

func BuildCorrelatedSecurityFinding(
	scope *model.AgentBehaviorEvent,
	correlation *CorrelatedFinding,
) *model.AgentSecurityFinding {
	if scope == nil || correlation == nil || len(correlation.EvidenceEventIDs) == 0 {
		return nil
	}
	anchorEventID := correlation.AnchorEventID
	if anchorEventID == "" {
		anchorEventID = correlation.EvidenceEventIDs[len(correlation.EvidenceEventIDs)-1]
	}
	findingKey := "correlation:v1:" + correlation.RuleKey + ":" + anchorEventID
	ruleHits := append([]AgentRuleHit{{
		RuleID:  uuid.NewSHA1(uuid.NameSpaceOID, []byte(correlation.RuleKey)),
		RuleKey: correlation.RuleKey, RuleVersion: correlation.RuleVersion,
		RuleDigest: correlation.RuleDigest, RuleName: correlation.Title,
		EventID: anchorEventID, Severity: correlation.Severity,
		Decision: "alert", Confidence: correlation.Confidence,
		MatchKind: "event_sequence", AttackStage: "execution", Outcome: "success",
	}}, correlation.RuleHits...)
	return &model.AgentSecurityFinding{
		ID:                  stableFindingID(findingKey),
		FindingKey:          findingKey,
		HostID:              scope.HostID,
		InstanceID:          scope.InstanceID,
		SessionID:           scope.SessionID,
		ExecutionUnitID:     scope.ExecutionUnitID,
		PolicyID:            scope.PolicyID,
		PolicyVersion:       scope.PolicyVersion,
		Title:               correlation.Title,
		Severity:            correlation.Severity,
		Verdict:             correlation.Verdict,
		Confidence:          correlation.Confidence,
		Status:              "open",
		DecisionSources:     mustJSON([]string{"agent_guard_rule", "event_correlation"}, []string{}),
		RuleHits:            mustJSON(ruleHits, []AgentRuleHit{}),
		EvidenceEventIDs:    mustJSON(uniqueSortedStrings(correlation.EvidenceEventIDs), []string{}),
		EvidenceGraph:       correlatedEvidenceGraph(correlation),
		AttackStages:        mustJSON(nonEmptyStrings(correlation.AttackStages), []string{}),
		Summary:             "Agent Guard correlated a download, file, and execution sequence from immutable event IDs.",
		RecommendedAction:   auditOnlyRecommendation(correlation.RecommendedAction),
		FirstObservedAt:     correlation.FirstObservedAt,
		LastObservedAt:      correlation.LastObservedAt,
		EvidenceSourceTable: "agent_behavior_events",
	}
}

func correlatedEvidenceGraph(correlation *CorrelatedFinding) json.RawMessage {
	graph := map[string]any{}
	_ = json.Unmarshal(
		evidenceGraph(correlation.EvidenceEventIDs, correlation.CounterEvidenceEventIDs, correlation.RuleKey),
		&graph,
	)
	graph["completeness"] = correlation.Completeness
	graph["correlation_rule"] = map[string]any{
		"rule_key": correlation.RuleKey, "rule_version": correlation.RuleVersion,
		"rule_digest": correlation.RuleDigest,
	}
	return mustJSON(graph, map[string]any{})
}

func stableFindingID(findingKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(findingKey))
}

func evidenceGraph(eventIDs, counterEventIDs []string, ruleKey string) json.RawMessage {
	counter := make(map[string]struct{}, len(counterEventIDs))
	for _, eventID := range counterEventIDs {
		counter[eventID] = struct{}{}
	}
	ids := uniqueStrings(eventIDs)
	nodes := make([]map[string]any, 0, len(ids)+1)
	nodes = append(nodes, map[string]any{"id": ruleKey, "kind": "rule"})
	edges := make([]map[string]string, 0, len(ids))
	previous := ""
	for _, eventID := range ids {
		node := map[string]any{"id": eventID, "kind": "event"}
		if _, exists := counter[eventID]; exists {
			node["counter_evidence"] = true
		}
		nodes = append(nodes, node)
		if previous == "" {
			edges = append(edges, map[string]string{"from": ruleKey, "to": eventID, "relation": "matched"})
		} else {
			edges = append(edges, map[string]string{"from": previous, "to": eventID, "relation": "preceded"})
		}
		previous = eventID
	}
	return mustJSON(map[string]any{
		"nodes":                nodes,
		"edges":                edges,
		"counter_evidence_ids": uniqueSortedStrings(counterEventIDs),
	}, map[string]any{})
}

func auditOnlyRecommendation(value string) string {
	if value == "alert" {
		return "alert"
	}
	return "audit"
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueSortedStrings(values []string) []string {
	result := uniqueStrings(values)
	sort.Strings(result)
	return result
}

func nonEmptyStrings(values []string) []string {
	return uniqueStrings(values)
}
