package pipeline

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

const remoteUnobservable = "remote_unobservable"

const maxRemoteEvidenceSelectors = 128

var trustedToolSources = map[string]struct{}{
	"agent_official": {},
	"adapter_hook":   {},
	"aegis_wrapper":  {},
}

var trustedRemoteOSSources = map[string]struct{}{
	"ebpf":   {},
	"procfs": {},
}

type RemoteEvidenceSelector struct {
	EventID         string
	HostID          uuid.UUID
	ExecutionUnitID uuid.UUID
	CorrelationHash string
	ToolOccurredAt  time.Time
}

type ToolSemanticNode struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	HostID string `json:"host_id,omitempty"`
}

type ToolSemanticEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

// ToolSemanticGraph contains evidence-backed links only. A correlation hash is
// deliberately omitted: it is a join key, never an authentication assertion.
type ToolSemanticGraph struct {
	Nodes          []ToolSemanticNode `json:"nodes"`
	Edges          []ToolSemanticEdge `json:"edges"`
	EventIDs       []string           `json:"event_ids"`
	Limitations    []string           `json:"limitations"`
	RemoteCoverage string             `json:"remote_coverage,omitempty"`
}

func normalizeAgentToolSemantics(envelope *behaviorEnvelope, occurredAt time.Time) error {
	if envelope == nil {
		return fmt.Errorf("%w: tool envelope", ErrAgentBehaviorInvalidContract)
	}
	if containsRawCorrelationToken(envelope.Evidence) || containsRawCorrelationToken(envelope.Resource.Attributes) {
		return fmt.Errorf("%w: raw correlation token", ErrAgentBehaviorInvalidContract)
	}
	if value, ok := envelope.Evidence["correlation_token_hash"]; ok && !isSHA256Reference(stringValueAny(value)) {
		return fmt.Errorf("%w: correlation token hash", ErrAgentBehaviorInvalidContract)
	}
	if envelope.Category != "tool" {
		return nil
	}
	if _, ok := trustedToolSources[envelope.Collection.Source]; !ok {
		return fmt.Errorf("%w: untrusted tool source", ErrAgentBehaviorInvalidContract)
	}
	if !allowedValue(envelope.Operation, "tool_call_started", "tool_call_completed", "tool_call_failed") ||
		envelope.Decision != "" && envelope.Decision != "audit" ||
		envelope.RuleID != "" || envelope.AttributionConfidence != "confirmed" ||
		!allowedValue(defaultString(envelope.Severity, "info"), "info", "low") {
		return fmt.Errorf("%w: tool semantic assertion", ErrAgentBehaviorInvalidContract)
	}
	if envelope.Resource.Type != "tool" || strings.TrimSpace(envelope.Resource.Identity) == "" {
		return fmt.Errorf("%w: tool identity", ErrAgentBehaviorInvalidContract)
	}

	proof := objectField(envelope.Evidence, "trusted_proof")
	verified, _ := proof["verified"].(bool)
	verifier := stringValueAny(proof["verifier"])
	proofDigest := stringValueAny(proof["proof_digest"])
	issuedAt, err := time.Parse(time.RFC3339Nano, stringValueAny(proof["issued_at"]))
	if !verified || verifier != "ed25519" ||
		!isSHA256Reference(proofDigest) || err != nil || absDuration(occurredAt.Sub(issuedAt)) > 5*time.Minute {
		return fmt.Errorf("%w: trusted tool proof", ErrAgentBehaviorInvalidContract)
	}

	attributes := envelope.Resource.Attributes
	toolCallID := stringValueAny(attributes["tool_call_id"])
	if !isNonNilUUID(toolCallID) {
		return fmt.Errorf("%w: tool_call_id", ErrAgentBehaviorInvalidContract)
	}
	processEventID := stringValueAny(attributes["process_event_id"])
	if processEventID != "" && !isNonNilUUID(processEventID) {
		return fmt.Errorf("%w: process_event_id", ErrAgentBehaviorInvalidContract)
	}
	resourceEventIDs, ok := uuidStringList(attributes["resource_event_ids"], 64)
	if !ok {
		return fmt.Errorf("%w: resource_event_ids", ErrAgentBehaviorInvalidContract)
	}

	correlationHash := stringValueAny(envelope.Evidence["correlation_token_hash"])
	if !isSHA256Reference(correlationHash) || envelope.CorrelationID != correlationHash {
		return fmt.Errorf("%w: tool correlation hash", ErrAgentBehaviorInvalidContract)
	}
	remoteHostID := stringValueAny(attributes["remote_host_id"])
	remoteUnitID := stringValueAny(attributes["remote_execution_unit_id"])
	remoteSensorIDs, sensorIDsOK := uuidStringList(firstValue(attributes, "remote_sensor_event_ids", "remote_sensor_event_id"), 64)
	if !sensorIDsOK || (remoteHostID == "") != (remoteUnitID == "") ||
		(remoteHostID != "" && (!isNonNilUUID(remoteHostID) || !isNonNilUUID(remoteUnitID))) {
		return fmt.Errorf("%w: remote tool evidence", ErrAgentBehaviorInvalidContract)
	}

	limitations := []string{}
	remoteCoverage := ""
	if remoteHostID != "" {
		remoteCoverage = remoteUnobservable
		limitations = append(limitations, "remote_sensor_evidence_not_yet_correlated")
		envelope.Evidence["remote_coverage"] = remoteUnobservable
	}
	envelope.Evidence["tool_semantics"] = map[string]any{
		"trusted":                     true,
		"source":                      envelope.Collection.Source,
		"tool_call_id":                toolCallID,
		"process_event_id":            processEventID,
		"resource_event_ids":          resourceEventIDs,
		"remote_host_id":              remoteHostID,
		"remote_execution_unit_id":    remoteUnitID,
		"remote_sensor_event_ids":     remoteSensorIDs,
		"remote_coverage":             remoteCoverage,
		"limitations":                 limitations,
		"proof_digest":                proofDigest,
		"correlation_token_hash_only": true,
	}
	return nil
}

func isSHA256Reference(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && value == strings.ToLower(value)
}

func isNonNilUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil
}

func containsRawCorrelationToken(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			if strings.Contains(normalized, "correlation_token") && normalized != "correlation_token_hash" {
				return true
			}
			if containsRawCorrelationToken(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsRawCorrelationToken(child) {
				return true
			}
		}
	}
	return false
}

func uuidStringList(value any, limit int) ([]string, bool) {
	if value == nil || value == "" {
		return []string{}, true
	}
	var raw []any
	switch typed := value.(type) {
	case []any:
		raw = typed
	case string:
		raw = []any{typed}
	default:
		return nil, false
	}
	if len(raw) > limit {
		return nil, false
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		id := stringValueAny(item)
		if !isNonNilUUID(id) {
			return nil, false
		}
		result = append(result, id)
	}
	return uniqueSortedStrings(result), true
}

func firstValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := values[key]; exists {
			return value
		}
	}
	return nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// CorrelateTrustedToolSemantics is deterministic for duplicate and out-of-order
// input. It verifies every requested edge against an immutable OS event.
func CorrelateTrustedToolSemantics(events []*model.AgentBehaviorEvent) ToolSemanticGraph {
	graph := ToolSemanticGraph{Nodes: []ToolSemanticNode{}, Edges: []ToolSemanticEdge{}, EventIDs: []string{}, Limitations: []string{}}
	byID := make(map[string]*model.AgentBehaviorEvent)
	for _, event := range events {
		if event != nil && event.RawEventID != "" {
			byID[event.RawEventID] = event
		}
	}
	toolIDs := make([]string, 0)
	for id, event := range byID {
		if isTrustedToolProjection(event) {
			toolIDs = append(toolIDs, id)
		}
	}
	sort.Strings(toolIDs)
	nodeSeen, edgeSeen := map[string]struct{}{}, map[string]struct{}{}
	addNode := func(node ToolSemanticNode) {
		key := node.Kind + ":" + node.ID
		if _, exists := nodeSeen[key]; !exists {
			nodeSeen[key] = struct{}{}
			graph.Nodes = append(graph.Nodes, node)
			graph.EventIDs = append(graph.EventIDs, node.ID)
		}
	}
	addEdge := func(edge ToolSemanticEdge) {
		key := edge.From + "\x00" + edge.To + "\x00" + edge.Relation
		if _, exists := edgeSeen[key]; !exists {
			edgeSeen[key] = struct{}{}
			graph.Edges = append(graph.Edges, edge)
		}
	}
	for _, toolID := range toolIDs {
		tool := byID[toolID]
		semantic := objectField(decodeJSONObject(tool.Evidence), "tool_semantics")
		addNode(ToolSemanticNode{ID: toolID, Kind: "tool_call", HostID: tool.HostID.String()})
		processID := stringValueAny(semantic["process_event_id"])
		if process := byID[processID]; processID != "" && isLocalOSEvidence(tool, process) && process.Category == "process" {
			addNode(ToolSemanticNode{ID: processID, Kind: "process", HostID: process.HostID.String()})
			addEdge(ToolSemanticEdge{From: toolID, To: processID, Relation: "invoked_process"})
			for _, resourceID := range toolStringSlice(semantic["resource_event_ids"]) {
				resource := byID[resourceID]
				if resource != nil && resource.Category != "tool" && isLocalOSEvidence(process, resource) {
					addNode(ToolSemanticNode{ID: resourceID, Kind: "resource", HostID: resource.HostID.String()})
					addEdge(ToolSemanticEdge{From: processID, To: resourceID, Relation: "accessed_resource"})
				}
			}
		}

		remoteHostID := stringValueAny(semantic["remote_host_id"])
		remoteUnitID := stringValueAny(semantic["remote_execution_unit_id"])
		if remoteHostID == "" {
			continue
		}
		correlationHash := stringValueAny(decodeJSONObject(tool.Evidence)["correlation_token_hash"])
		linked := false
		for _, remoteID := range toolStringSlice(semantic["remote_sensor_event_ids"]) {
			remote := byID[remoteID]
			selector, ok := remoteSelector(remoteID, remoteHostID, remoteUnitID, correlationHash, tool.OccurredAt)
			if !ok || !MatchesTrustedRemoteEvidence(selector, remote, agentCorrelationWindow) {
				continue
			}
			addNode(ToolSemanticNode{ID: remoteID, Kind: "remote_sensor", HostID: remoteHostID})
			addEdge(ToolSemanticEdge{From: toolID, To: remoteID, Relation: "observed_remote_activity"})
			linked = true
		}
		if linked {
			if graph.RemoteCoverage != remoteUnobservable {
				graph.RemoteCoverage = "sensor_verified"
			}
		} else {
			graph.Limitations = append(graph.Limitations, remoteUnobservable)
			graph.RemoteCoverage = remoteUnobservable
		}
	}
	graph.EventIDs = uniqueSortedStrings(graph.EventIDs)
	graph.Limitations = uniqueSortedStrings(graph.Limitations)
	sort.Slice(graph.Nodes, func(i, j int) bool {
		return graph.Nodes[i].Kind+graph.Nodes[i].ID < graph.Nodes[j].Kind+graph.Nodes[j].ID
	})
	sort.Slice(graph.Edges, func(i, j int) bool {
		return graph.Edges[i].From+graph.Edges[i].To+graph.Edges[i].Relation < graph.Edges[j].From+graph.Edges[j].To+graph.Edges[j].Relation
	})
	return graph
}

// TrustedRemoteEvidenceSelectors extracts only signed, normalized tool claims.
// The result is bounded and deterministic before it reaches repository SQL.
func TrustedRemoteEvidenceSelectors(events []*model.AgentBehaviorEvent, relevantEvidenceIDs []string) []RemoteEvidenceSelector {
	byKey := map[string]RemoteEvidenceSelector{}
	relevant := make(map[string]struct{}, len(relevantEvidenceIDs))
	for _, eventID := range relevantEvidenceIDs {
		relevant[eventID] = struct{}{}
	}
	toolEvents := append([]*model.AgentBehaviorEvent(nil), events...)
	sort.Slice(toolEvents, func(i, j int) bool {
		if toolEvents[i] == nil {
			return false
		}
		if toolEvents[j] == nil {
			return true
		}
		return toolEvents[i].RawEventID < toolEvents[j].RawEventID
	})
	for _, tool := range toolEvents {
		if !isTrustedToolProjection(tool) {
			continue
		}
		evidence := decodeJSONObject(tool.Evidence)
		semantic := objectField(evidence, "tool_semantics")
		if _, linked := relevant[stringValueAny(semantic["process_event_id"])]; !linked {
			continue
		}
		remoteHostID := stringValueAny(semantic["remote_host_id"])
		remoteUnitID := stringValueAny(semantic["remote_execution_unit_id"])
		correlationHash := stringValueAny(evidence["correlation_token_hash"])
		for _, remoteID := range toolStringSlice(semantic["remote_sensor_event_ids"]) {
			selector, ok := remoteSelector(remoteID, remoteHostID, remoteUnitID, correlationHash, tool.OccurredAt)
			if !ok {
				continue
			}
			key := selector.EventID + ":" + selector.HostID.String() + ":" + selector.ExecutionUnitID.String()
			byKey[key] = selector
			if len(byKey) >= maxRemoteEvidenceSelectors {
				break
			}
		}
		if len(byKey) >= maxRemoteEvidenceSelectors {
			break
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]RemoteEvidenceSelector, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func remoteSelector(eventID, hostID, unitID, correlationHash string, occurredAt time.Time) (RemoteEvidenceSelector, bool) {
	eventUUID, eventErr := uuid.Parse(eventID)
	hostUUID, hostErr := uuid.Parse(hostID)
	unitUUID, unitErr := uuid.Parse(unitID)
	if eventErr != nil || eventUUID == uuid.Nil || hostErr != nil || hostUUID == uuid.Nil ||
		unitErr != nil || unitUUID == uuid.Nil || !isSHA256Reference(correlationHash) || occurredAt.IsZero() {
		return RemoteEvidenceSelector{}, false
	}
	return RemoteEvidenceSelector{
		EventID: eventID, HostID: hostUUID, ExecutionUnitID: unitUUID,
		CorrelationHash: correlationHash, ToolOccurredAt: occurredAt,
	}, true
}

func ValidRemoteEvidenceSelector(selector RemoteEvidenceSelector) bool {
	if selector.HostID == uuid.Nil || selector.ExecutionUnitID == uuid.Nil {
		return false
	}
	canonical, ok := remoteSelector(
		selector.EventID, selector.HostID.String(), selector.ExecutionUnitID.String(),
		selector.CorrelationHash, selector.ToolOccurredAt,
	)
	return ok && canonical == selector
}

func MatchesTrustedRemoteEvidence(selector RemoteEvidenceSelector, event *model.AgentBehaviorEvent, window time.Duration) bool {
	if event == nil || window <= 0 || event.ExecutionUnitID == nil || event.RawEventID != selector.EventID ||
		event.HostID != selector.HostID || *event.ExecutionUnitID != selector.ExecutionUnitID ||
		absDuration(event.OccurredAt.Sub(selector.ToolOccurredAt)) > window ||
		!allowedValue(event.Category, "process", "file", "network", "identity", "persistence", "isolation", "kernel", "ipc") ||
		event.CommandVisibility != "complete" {
		return false
	}
	collection := decodeJSONObject(event.Collection)
	if _, trusted := trustedRemoteOSSources[stringValueAny(collection["source"])]; !trusted ||
		strings.TrimSpace(stringValueAny(collection["sensor"])) == "" ||
		stringValueAny(collection["attribution_confidence"]) != "confirmed" ||
		stringValueAny(collection["visibility"]) != "complete" {
		return false
	}
	lost, ok := int64Value(collection["lost_events_since_last"])
	truncated, truncatedOK := collection["truncated_fields"].([]any)
	aggregated, aggregatedOK := int64Value(collection["aggregated_count"])
	if !ok || lost != 0 || !truncatedOK || len(truncated) != 0 || !aggregatedOK || aggregated != 1 {
		return false
	}
	coverage := stringValueAny(collection["coverage_level"])
	if !allowedValue(coverage, "full_enforcement", "behavior_monitor_escape_enforce", "monitor_only", "no_isolation") {
		return false
	}
	return stringValueAny(decodeJSONObject(event.Evidence)["correlation_token_hash"]) == selector.CorrelationHash
}

func isTrustedToolProjection(event *model.AgentBehaviorEvent) bool {
	if event == nil || event.Category != "tool" || event.Decision != "audit" || event.RuleID != "" {
		return false
	}
	collection := decodeJSONObject(event.Collection)
	if _, ok := trustedToolSources[stringValueAny(collection["source"])]; !ok {
		return false
	}
	semantic := objectField(decodeJSONObject(event.Evidence), "tool_semantics")
	trusted, _ := semantic["trusted"].(bool)
	return trusted && isSHA256Reference(stringValueAny(semantic["proof_digest"]))
}

func isLocalOSEvidence(scope, event *model.AgentBehaviorEvent) bool {
	return scope != nil && event != nil && event.Category != "tool" && scope.InstanceID != nil && event.InstanceID != nil &&
		scope.SessionID != nil && event.SessionID != nil && scope.ExecutionUnitID != nil && event.ExecutionUnitID != nil &&
		scope.HostID == event.HostID && *scope.InstanceID == *event.InstanceID && *scope.SessionID == *event.SessionID &&
		*scope.ExecutionUnitID == *event.ExecutionUnitID
}

func toolStringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValueAny(item); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func ToolProofDigest(event *model.AgentBehaviorEvent) string {
	if !isTrustedToolProjection(event) {
		return ""
	}
	return stringValueAny(objectField(decodeJSONObject(event.Evidence), "tool_semantics")["proof_digest"])
}

func encodeToolSemanticGraph(graph ToolSemanticGraph) json.RawMessage {
	return mustJSON(graph, ToolSemanticGraph{})
}
