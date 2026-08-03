package pipeline

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"dc/internal/model"
)

const (
	downloadExecuteRuleKey     = "AGB-DOWNLOAD-EXEC-001"
	downloadExecuteRuleVersion = int64(1)
	downloadExecuteRuleDigest  = "sha256:8fd0ecae6e28684a87dc85a9a8ec79c9a6e86e17874af3f504b431bd970b79ab"
)

type EvidenceCompleteness struct {
	Visibility      string   `json:"visibility"`
	LostEvents      int64    `json:"lost_events"`
	SequenceGaps    int64    `json:"sequence_gaps"`
	TruncatedFields []string `json:"truncated_fields"`
	Limitations     []string `json:"limitations"`
}

type CorrelatedFinding struct {
	RuleKey                 string
	RuleVersion             int64
	RuleDigest              string
	Title                   string
	Severity                string
	Verdict                 string
	Confidence              float64
	RecommendedAction       string
	EvidenceEventIDs        []string
	CounterEvidenceEventIDs []string
	RuleHits                []AgentRuleHit
	AttackStages            []string
	Completeness            EvidenceCompleteness
	FirstObservedAt         time.Time
	LastObservedAt          time.Time
	CorrelationAnchor       string
	AnchorEventID           string
}

func CorrelateDownloadExecute(events []*model.AgentBehaviorEvent, window time.Duration) *CorrelatedFinding {
	ordered := normalizeCorrelationEvents(events, window)
	if len(ordered) < 3 {
		return nil
	}
	var download, create, chmod, execute, callback *model.AgentBehaviorEvent
	for _, event := range ordered {
		classified := ClassifyAgentBehavior(event, RuleEvaluationOptions{})
		switch {
		case download == nil && classified.Category == "network" && classified.Operation == "connect" &&
			classified.Outcome == "success" && classified.ResourceClassification == "external":
			download = classified
		case create == nil && download != nil && classified.Category == "file" &&
			containsStringValue([]string{"create", "write"}, classified.Operation) &&
			classified.Outcome == "success" && correlationRelated(download, classified):
			if classified.Operation == "create" && !boolValue(resourceAttributes(classified)["inode_created"]) {
				continue
			}
			create = classified
		case chmod == nil && create != nil && classified.Category == "file" &&
			containsStringValue([]string{"chmod", "chown"}, classified.Operation) &&
			classified.Outcome == "success" && sameResource(create, classified):
			chmod = classified
		case execute == nil && create != nil && classified.Category == "process" &&
			classified.Operation == "exec" && sameExecutedResource(create, classified):
			execute = classified
		case callback == nil && execute != nil && classified.Category == "network" &&
			classified.Operation == "connect" && classified.OccurredAt.After(execute.OccurredAt) &&
			correlationRelated(execute, classified):
			callback = classified
		}
	}
	if download == nil || create == nil || execute == nil {
		return nil
	}

	evidence := []*model.AgentBehaviorEvent{download, create}
	stages := []string{"ingress_tool_transfer", "resource_development"}
	if chmod != nil {
		evidence = append(evidence, chmod)
		stages = append(stages, "permission_change")
	}
	evidence = append(evidence, execute)
	stages = append(stages, "execution")
	if callback != nil {
		evidence = append(evidence, callback)
		stages = append(stages, "command_and_control")
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].OccurredAt.Equal(evidence[j].OccurredAt) {
			return evidence[i].AgentSequence < evidence[j].AgentSequence
		}
		return evidence[i].OccurredAt.Before(evidence[j].OccurredAt)
	})
	completeness := correlationCompleteness(evidence)
	result := &CorrelatedFinding{
		RuleKey: downloadExecuteRuleKey, RuleVersion: downloadExecuteRuleVersion,
		RuleDigest: downloadExecuteRuleDigest, Title: "Agent download and execute chain",
		Severity: "high", Verdict: "suspicious", Confidence: 0.86,
		RecommendedAction: "alert", AttackStages: stages, Completeness: completeness,
		FirstObservedAt:   evidence[0].OccurredAt,
		LastObservedAt:    evidence[len(evidence)-1].OccurredAt,
		CorrelationAnchor: create.ResourceIdentity,
		AnchorEventID:     execute.RawEventID,
	}
	for _, event := range evidence {
		result.EvidenceEventIDs = append(result.EvidenceEventIDs, event.RawEventID)
		result.RuleHits = append(result.RuleHits, EvaluateBuiltinRules(event, RuleEvaluationOptions{})...)
	}
	if execute.Outcome != "success" {
		result.Title = "Agent download with failed execution attempt"
		result.Severity, result.Verdict, result.Confidence = "medium", "inconclusive", 0.62
		result.CounterEvidenceEventIDs = []string{execute.RawEventID}
	} else if chmod != nil && callback != nil {
		result.Severity, result.Verdict, result.Confidence = "critical", "malicious", 0.94
	}
	if completeness.Visibility != "complete" {
		result.Verdict = "inconclusive"
		if result.Confidence > 0.69 {
			result.Confidence = 0.69
		}
	}
	return result
}

func normalizeCorrelationEvents(events []*model.AgentBehaviorEvent, window time.Duration) []*model.AgentBehaviorEvent {
	seen := make(map[string]struct{}, len(events))
	result := make([]*model.AgentBehaviorEvent, 0, len(events))
	var host, instance, session, unit string
	for _, event := range events {
		if event == nil || event.InstanceID == nil || event.SessionID == nil || event.ExecutionUnitID == nil ||
			event.RawEventID == "" {
			continue
		}
		if _, exists := seen[event.RawEventID]; exists {
			continue
		}
		scopeHost, scopeInstance := event.HostID.String(), event.InstanceID.String()
		scopeSession, scopeUnit := event.SessionID.String(), event.ExecutionUnitID.String()
		if len(result) == 0 {
			host, instance, session, unit = scopeHost, scopeInstance, scopeSession, scopeUnit
		}
		if scopeHost != host || scopeInstance != instance || scopeSession != session || scopeUnit != unit {
			continue
		}
		seen[event.RawEventID] = struct{}{}
		result = append(result, event)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].OccurredAt.Equal(result[j].OccurredAt) {
			return result[i].AgentSequence < result[j].AgentSequence
		}
		return result[i].OccurredAt.Before(result[j].OccurredAt)
	})
	if len(result) > 1 && window > 0 {
		latest := result[len(result)-1].OccurredAt
		start := 0
		for start < len(result) && latest.Sub(result[start].OccurredAt) > window {
			start++
		}
		result = result[start:]
	}
	return result
}

func correlationRelated(left, right *model.AgentBehaviorEvent) bool {
	if left.PID == right.PID {
		return true
	}
	return left.CorrelationID != "" && left.CorrelationID == right.CorrelationID
}

func sameResource(left, right *model.AgentBehaviorEvent) bool {
	return normalizedResourceIdentity(left) != "" &&
		normalizedResourceIdentity(left) == normalizedResourceIdentity(right)
}

func sameExecutedResource(fileEvent, executeEvent *model.AgentBehaviorEvent) bool {
	wanted := normalizedResourceIdentity(fileEvent)
	if wanted == "" {
		return false
	}
	for _, candidate := range []string{
		executeEvent.ProcessExe,
		executeEvent.ResourceIdentity,
		stringValueAny(resourceAttributes(executeEvent)["resolved_path"]),
	} {
		if normalizedPath(candidate) == wanted {
			return true
		}
	}
	return false
}

func normalizedResourceIdentity(event *model.AgentBehaviorEvent) string {
	for _, value := range []string{
		stringValueAny(resourceAttributes(event)["resolved_path"]),
		stringValueAny(resourceAttributes(event)["host_path"]),
		event.ResourceIdentity,
	} {
		if normalized := normalizedPath(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizedPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimSuffix(value, "/")
}

func resourceAttributes(event *model.AgentBehaviorEvent) map[string]any {
	return objectField(decodeJSONObject(event.Resource), "attributes")
}

func correlationCompleteness(events []*model.AgentBehaviorEvent) EvidenceCompleteness {
	result := EvidenceCompleteness{Visibility: "complete", TruncatedFields: []string{}, Limitations: []string{}}
	sequences := make([]int64, 0, len(events))
	seenTruncated := make(map[string]struct{})
	for _, event := range events {
		sequences = append(sequences, event.AgentSequence)
		collection := decodeJSONObject(event.Collection)
		visibility := stringValueAny(collection["visibility"])
		if visibility == "unobservable" {
			result.Visibility = "unobservable"
		} else if visibility == "partial" && result.Visibility == "complete" {
			result.Visibility = "partial"
		}
		lost, _ := int64Value(collection["lost_events_since_last"])
		result.LostEvents += lost
		if fields, ok := collection["truncated_fields"].([]any); ok {
			for _, field := range fields {
				value := stringValueAny(field)
				if value != "" {
					seenTruncated[value] = struct{}{}
				}
			}
		}
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	for index := 1; index < len(sequences); index++ {
		if delta := sequences[index] - sequences[index-1] - 1; delta > 0 {
			result.SequenceGaps += delta
		}
	}
	for field := range seenTruncated {
		result.TruncatedFields = append(result.TruncatedFields, field)
	}
	sort.Strings(result.TruncatedFields)
	if result.LostEvents > 0 {
		result.Limitations = append(result.Limitations, "collector_event_loss")
	}
	if result.SequenceGaps > 0 {
		result.Limitations = append(result.Limitations, "agent_sequence_gap")
	}
	if len(result.TruncatedFields) > 0 {
		result.Limitations = append(result.Limitations, "evidence_fields_truncated")
	}
	if len(result.Limitations) > 0 && result.Visibility == "complete" {
		result.Visibility = "partial"
	}
	return result
}

func marshalCorrelationValue(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
