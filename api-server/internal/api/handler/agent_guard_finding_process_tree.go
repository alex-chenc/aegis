package handler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type agentGuardFindingRuleHit struct {
	RuleKey          string   `json:"rule_key"`
	RuleVersion      int64    `json:"rule_version"`
	RuleName         string   `json:"rule_name"`
	Severity         string   `json:"severity"`
	MatchKind        string   `json:"match_kind"`
	EventID          string   `json:"event_id"`
	EventIDs         []string `json:"event_ids"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
}

type agentGuardFindingRuleDetail struct {
	RuleKey     string   `json:"rule_key"`
	RuleVersion int64    `json:"rule_version,omitempty"`
	Name        string   `json:"name"`
	Severity    string   `json:"severity,omitempty"`
	MatchKind   string   `json:"match_kind,omitempty"`
	EventIDs    []string `json:"event_ids"`
	// ProcessTree is retained as an empty compatibility field for older API
	// clients. New security analysis renders the matched tool calls below and
	// never exposes the OS process tree.
	ProcessTree []*agentGuardFindingProcessNode `json:"process_tree,omitempty"`
	ToolCalls   []*agentGuardFindingToolCall    `json:"tool_calls,omitempty"`
}

type agentGuardFindingToolCall struct {
	EventID           string `json:"event_id"`
	ToolName          string `json:"tool_name"`
	ToolCallID        string `json:"tool_call_id,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	Command           string `json:"command,omitempty"`
	ToolInput         any    `json:"tool_input,omitempty"`
	ToolResponse      any    `json:"tool_response,omitempty"`
	Outcome           string `json:"outcome,omitempty"`
	OccurredAt        string `json:"occurred_at,omitempty"`
	PID               int    `json:"pid,omitempty"`
	PPID              int    `json:"ppid,omitempty"`
	ProcessStartTicks string `json:"process_start_ticks,omitempty"`
	CommandLine       string `json:"command_line,omitempty"`
	CorrelationStatus string `json:"correlation_status,omitempty"`
	CorrelationMethod string `json:"correlation_method,omitempty"`
}

type agentGuardFindingProcessNode struct {
	ID                string                          `json:"id"`
	ParentID          string                          `json:"parent_id,omitempty"`
	PID               int                             `json:"pid"`
	PPID              int                             `json:"ppid"`
	ProcessStartTicks string                          `json:"process_start_ticks,omitempty"`
	ProcessName       string                          `json:"process_name,omitempty"`
	ProcessExe        string                          `json:"process_exe,omitempty"`
	Cmdline           string                          `json:"cmdline,omitempty"`
	CommandCwd        string                          `json:"command_cwd,omitempty"`
	CommandVisibility string                          `json:"command_visibility,omitempty"`
	ProcessStatus     string                          `json:"process_status,omitempty"`
	FirstSeenAt       string                          `json:"first_seen_at,omitempty"`
	LastSeenAt        string                          `json:"last_seen_at,omitempty"`
	EventCount        int64                           `json:"event_count"`
	Matched           bool                            `json:"matched"`
	MatchedEventIDs   []string                        `json:"matched_event_ids,omitempty"`
	Children          []*agentGuardFindingProcessNode `json:"children,omitempty"`
}

type agentGuardFindingRuleGroup struct {
	agentGuardFindingRuleHit
	eventIDs      []string
	matchEventIDs []string
}

// agentGuardRuntimeEventData is the stable subset of runtime_events.event_data
// needed to correlate an older runtime event with Agent Guard process facts.
// Keep this projection deliberately narrow: event_data may contain sensitive
// evidence fields that must not be copied into the finding response.
type agentGuardRuntimeEventData struct {
	Category        string `json:"category"`
	Operation       string `json:"operation"`
	Outcome         string `json:"outcome"`
	Decision        string `json:"decision"`
	Severity        string `json:"severity"`
	OccurredAt      string `json:"occurred_at"`
	InstanceID      string `json:"instance_id"`
	SessionID       string `json:"session_id"`
	ExecutionUnitID string `json:"execution_unit_id"`
	CorrelationID   string `json:"correlation_id"`
	ParentEventID   string `json:"parent_event_id"`
	Actor           struct {
		PID        *int            `json:"pid"`
		PPID       *int            `json:"ppid"`
		StartTicks json.RawMessage `json:"start_ticks"`
		Exe        string          `json:"exe"`
		CWD        string          `json:"cwd"`
		Argv       []string        `json:"argv"`
	} `json:"actor"`
	Collection struct {
		Visibility string `json:"visibility"`
	} `json:"collection"`
}

func decodeAgentGuardFindingRuleHits(raw []byte) []agentGuardFindingRuleHit {
	var hits []agentGuardFindingRuleHit
	if len(raw) > 0 && json.Unmarshal(raw, &hits) == nil {
		return hits
	}

	// Older findings stored only a list of rule keys. Keep those findings
	// selectable even though they cannot identify a rule-specific event.
	var keys []string
	if len(raw) == 0 || json.Unmarshal(raw, &keys) != nil {
		return nil
	}
	hits = make([]agentGuardFindingRuleHit, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			hits = append(hits, agentGuardFindingRuleHit{RuleKey: strings.TrimSpace(key)})
		}
	}
	return hits
}

func findingRuleHitEventIDs(hit agentGuardFindingRuleHit) []string {
	ids := make([]string, 0, 1+len(hit.EventIDs)+len(hit.EvidenceEventIDs))
	appendUnique := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || containsString(ids, value) {
				continue
			}
			ids = append(ids, value)
		}
	}
	appendUnique(hit.EventID)
	appendUnique(hit.EventIDs...)
	appendUnique(hit.EvidenceEventIDs...)
	return ids
}

func findingRuleHitMatchEventIDs(hit agentGuardFindingRuleHit) []string {
	ids := make([]string, 0, 1+len(hit.EventIDs))
	appendUnique := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || containsString(ids, value) {
				continue
			}
			ids = append(ids, value)
		}
	}
	appendUnique(hit.EventID)
	appendUnique(hit.EventIDs...)
	return ids
}

func (h *AgentGuardHandler) buildFindingRuleDetails(
	ctx context.Context,
	finding *model.AgentSecurityFinding,
) ([]agentGuardFindingRuleDetail, error) {
	if finding == nil {
		return nil, nil
	}
	hits := decodeAgentGuardFindingRuleHits(finding.RuleHits)
	if len(hits) == 0 {
		return []agentGuardFindingRuleDetail{}, nil
	}

	groups := make([]agentGuardFindingRuleGroup, 0, len(hits))
	groupIndex := make(map[string]int, len(hits))
	for _, hit := range hits {
		key := strings.TrimSpace(hit.RuleKey)
		if key == "" {
			continue
		}
		groupKey := key + "@" + formatInt64(hit.RuleVersion)
		index, exists := groupIndex[groupKey]
		if !exists {
			groupIndex[groupKey] = len(groups)
			groups = append(groups, agentGuardFindingRuleGroup{agentGuardFindingRuleHit: hit})
			index = len(groups) - 1
		}
		group := &groups[index]
		if group.RuleName == "" {
			group.RuleName = hit.RuleName
		}
		if group.Severity == "" {
			group.Severity = hit.Severity
		}
		if group.MatchKind == "" {
			group.MatchKind = hit.MatchKind
		}
		for _, eventID := range findingRuleHitEventIDs(hit) {
			if !containsString(group.eventIDs, eventID) {
				group.eventIDs = append(group.eventIDs, eventID)
			}
		}
		matchEventIDs := findingRuleHitMatchEventIDs(hit)
		if len(matchEventIDs) == 0 {
			// Older findings put the only usable event references in the
			// evidence list. Use those as match references when no primary
			// rule event was persisted.
			matchEventIDs = hit.EvidenceEventIDs
		}
		for _, eventID := range matchEventIDs {
			eventID = strings.TrimSpace(eventID)
			if eventID != "" && !containsString(group.matchEventIDs, eventID) {
				group.matchEventIDs = append(group.matchEventIDs, eventID)
			}
		}
	}
	if len(groups) == 1 && len(groups[0].eventIDs) == 0 {
		groups[0].eventIDs = decodeAgentGuardJSONStrings(finding.EvidenceEventIDs)
		groups[0].matchEventIDs = append([]string(nil), groups[0].eventIDs...)
	}

	details := make([]agentGuardFindingRuleDetail, 0, len(groups))
	var sessionToolEvents []model.AgentBehaviorEvent
	if finding.SessionID != nil {
		var toolErr error
		sessionToolEvents, _, toolErr = h.query.ListBehaviors(ctx, model.AgentBehaviorEventQuery{
			AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: agentGuardMaxPageSize},
			HostID:              finding.HostID.String(), SessionID: finding.SessionID.String(), Category: "tool",
		})
		if toolErr != nil && h.logger != nil {
			h.logger.Debug("agent guard finding tool correlation unavailable",
				zap.String("finding_id", finding.ID.String()), zap.Error(toolErr))
			sessionToolEvents = nil
		}
	}
	for _, group := range groups {
		name := strings.TrimSpace(group.RuleName)
		version := group.RuleVersion
		if h.catalog != nil && (name == "" || version <= 0) {
			lookupVersion := version
			if lookupVersion <= 0 {
				lookupVersion = 1
			}
			if rule, err := h.catalog.GetRule(ctx, group.RuleKey, lookupVersion); err == nil && rule != nil {
				if name == "" {
					name = rule.Name
				}
				if version <= 0 {
					version = rule.RuleVersion
				}
				if group.Severity == "" {
					group.Severity = rule.DefaultSeverity
				}
			}
		}
		if name == "" {
			name = group.RuleKey
		}

		events := make([]model.AgentBehaviorEvent, 0, len(group.eventIDs))
		matchedEvents := make([]model.AgentBehaviorEvent, 0, len(group.matchEventIDs))
		matchEventIDs := make(map[string]struct{}, len(group.matchEventIDs))
		for _, eventID := range group.matchEventIDs {
			matchEventIDs[eventID] = struct{}{}
		}
		runtimeFallbackCount := 0
		unresolvedEventCount := 0
		for _, eventID := range group.eventIDs {
			event, err := h.query.GetBehavior(ctx, eventID)
			if err == nil && event != nil {
				if event.HostID == finding.HostID {
					events = append(events, *event)
					if _, ok := matchEventIDs[eventID]; ok {
						matchedEvents = append(matchedEvents, *event)
					}
				}
				continue
			}

			runtimeEvent, runtimeErr := h.query.GetRuntimeEvent(ctx, eventID)
			if runtimeErr != nil || runtimeEvent == nil || runtimeEvent.HostID != finding.HostID {
				unresolvedEventCount++
				continue
			}
			projected, projectErr := projectAgentGuardRuntimeEvent(*runtimeEvent, *finding)
			if projectErr != nil {
				unresolvedEventCount++
				continue
			}
			events = append(events, projected)
			if _, ok := matchEventIDs[eventID]; ok {
				matchedEvents = append(matchedEvents, projected)
			}
			runtimeFallbackCount++
		}
		if (runtimeFallbackCount > 0 || unresolvedEventCount > 0) && h.logger != nil {
			h.logger.Debug("agent guard finding event source resolution",
				zap.String("finding_id", finding.ID.String()),
				zap.String("rule_key", group.RuleKey),
				zap.Int("runtime_fallback_count", runtimeFallbackCount),
				zap.Int("unresolved_event_count", unresolvedEventCount),
			)
		}

		if len(events) == 0 {
			details = append(details, agentGuardFindingRuleDetail{
				RuleKey: group.RuleKey, RuleVersion: version, Name: name, Severity: group.Severity,
				MatchKind: group.MatchKind, EventIDs: append([]string(nil), group.eventIDs...),
				ToolCalls: []*agentGuardFindingToolCall{},
			})
			continue
		}
		toolEvents := matchedEvents
		if len(toolEvents) == 0 {
			toolEvents = events
		}
		toolEvents = onlyAgentGuardToolEvents(toolEvents)
		if len(toolEvents) == 0 && len(sessionToolEvents) > 0 {
			toolEvents = correlateAgentGuardToolEvents(events, sessionToolEvents)
		}
		toolCalls := make([]*agentGuardFindingToolCall, 0, len(toolEvents))
		for _, event := range toolEvents {
			toolCalls = append(toolCalls, projectAgentGuardFindingToolCall(event))
		}
		details = append(details, agentGuardFindingRuleDetail{
			RuleKey: group.RuleKey, RuleVersion: version, Name: name, Severity: group.Severity,
			MatchKind: group.MatchKind, EventIDs: append([]string(nil), group.eventIDs...),
			ToolCalls: toolCalls,
		})
	}
	return details, nil
}

func projectAgentGuardFindingToolCall(event model.AgentBehaviorEvent) *agentGuardFindingToolCall {
	attributes := panoramaBehaviorResourceAttributes(event.Resource)
	call := &agentGuardFindingToolCall{
		EventID: event.RawEventID, ToolName: event.ResourceIdentity,
		ToolCallID: panoramaStringAttribute(attributes, "tool_call_id"),
		TurnID:     panoramaStringAttribute(attributes, "turn_id"),
		Command:    panoramaStringAttribute(attributes, "command"),
		ToolInput:  attributes["tool_input"], ToolResponse: attributes["tool_response"],
		Outcome: event.Outcome, OccurredAt: event.OccurredAt.UTC().Format(time.RFC3339Nano),
		CommandLine:       agentGuardCommandLine(event.CommandArgv),
		CorrelationStatus: panoramaStringAttribute(attributes, "correlation_status"),
		CorrelationMethod: panoramaStringAttribute(attributes, "correlation_method"),
	}
	if event.PID != nil {
		call.PID = *event.PID
	}
	if event.PPID != nil {
		call.PPID = *event.PPID
	}
	call.ProcessStartTicks = event.ProcessStartTicks
	if call.CorrelationStatus == "unmatched" {
		// The event PID is the Hook/controller anchor when eBPF correlation did
		// not resolve a worker. It must never be presented as the command's PID.
		call.PID = 0
		call.PPID = 0
		call.ProcessStartTicks = ""
		call.CommandLine = ""
	}
	if call.Command == "" {
		if input, ok := call.ToolInput.(map[string]any); ok {
			call.Command = panoramaStringAttribute(input, "command")
		}
	}
	return call
}

func onlyAgentGuardToolEvents(events []model.AgentBehaviorEvent) []model.AgentBehaviorEvent {
	result := make([]model.AgentBehaviorEvent, 0, len(events))
	for _, event := range events {
		if event.Category == "tool" {
			result = append(result, event)
		}
	}
	return result
}

// correlateAgentGuardToolEvents links a rule's process/resource evidence back
// to the trusted tool event that caused it. Direct evidence often points at a
// syscall or process event, while the tool event carries the stable session
// and correlation token. Do not fall back to all session tools: that would
// make every finding appear to match every command in the session.
func correlateAgentGuardToolEvents(
	evidence []model.AgentBehaviorEvent,
	tools []model.AgentBehaviorEvent,
) []model.AgentBehaviorEvent {
	result := make([]model.AgentBehaviorEvent, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	for _, candidate := range tools {
		for _, event := range evidence {
			if !agentGuardToolMatchesEvidence(candidate, event) {
				continue
			}
			key := candidate.RawEventID
			if key == "" {
				key = candidate.ID.String()
			}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				result = append(result, candidate)
			}
			break
		}
	}
	return result
}

func agentGuardToolMatchesEvidence(tool, evidence model.AgentBehaviorEvent) bool {
	if tool.Category != "tool" {
		return false
	}
	if evidence.CorrelationID != "" && tool.CorrelationID == evidence.CorrelationID {
		return true
	}
	if tool.ParentEventID != "" && tool.ParentEventID == evidence.RawEventID {
		return true
	}
	attributes := panoramaBehaviorResourceAttributes(tool.Resource)
	if panoramaStringAttribute(attributes, "process_event_id") == evidence.RawEventID {
		return true
	}
	if values, ok := attributes["resource_event_ids"].([]any); ok {
		for _, value := range values {
			if stringValue, ok := value.(string); ok && stringValue == evidence.RawEventID {
				return true
			}
		}
	}
	return false
}

func projectAgentGuardRuntimeEvent(raw model.RuntimeEvent, finding model.AgentSecurityFinding) (model.AgentBehaviorEvent, error) {
	var data agentGuardRuntimeEventData
	if strings.TrimSpace(raw.EventData) != "" {
		if err := json.Unmarshal([]byte(raw.EventData), &data); err != nil {
			return model.AgentBehaviorEvent{}, err
		}
	}

	occurredAt := raw.CreatedAt
	if parsed, err := time.Parse(time.RFC3339Nano, data.OccurredAt); err == nil {
		occurredAt = parsed
	}
	if occurredAt.IsZero() {
		occurredAt = finding.LastObservedAt
	}

	pid := raw.PID
	if data.Actor.PID != nil {
		pid = *data.Actor.PID
	}
	argv := append([]string(nil), data.Actor.Argv...)
	if len(argv) == 0 && strings.TrimSpace(raw.CommandLine) != "" {
		argv = []string{raw.CommandLine}
	}
	commandArgv, err := json.Marshal(argv)
	if err != nil {
		return model.AgentBehaviorEvent{}, err
	}
	processName := ""
	if len(argv) > 0 {
		processName = filepath.Base(argv[0])
	}
	if processName == "" {
		processName = filepath.Base(data.Actor.Exe)
	}

	event := model.AgentBehaviorEvent{
		RawEventID:        raw.EventID,
		HostID:            raw.HostID,
		SchemaVersion:     "aegis.agent.behavior.v1",
		Category:          firstNonEmpty(data.Category, raw.EventType),
		Operation:         firstNonEmpty(data.Operation, raw.EventType),
		Outcome:           firstNonEmpty(data.Outcome, "unknown"),
		Decision:          firstNonEmpty(data.Decision, "audit"),
		Severity:          firstNonEmpty(data.Severity, raw.Severity, finding.Severity, "info"),
		PID:               positiveIntPointer(pid),
		PPID:              data.Actor.PPID,
		ProcessStartTicks: runtimeJSONScalarString(data.Actor.StartTicks),
		ProcessName:       processName,
		ProcessExe:        data.Actor.Exe,
		CommandArgv:       commandArgv,
		CommandCwd:        data.Actor.CWD,
		CommandVisibility: firstNonEmpty(data.Collection.Visibility, "complete"),
		OccurredAt:        occurredAt,
		CorrelationID:     data.CorrelationID,
		ParentEventID:     data.ParentEventID,
	}
	if data.InstanceID != "" {
		event.InstanceID = parseUUIDPointer(data.InstanceID)
	}
	if data.SessionID != "" {
		event.SessionID = parseUUIDPointer(data.SessionID)
	}
	if data.ExecutionUnitID != "" {
		event.ExecutionUnitID = parseUUIDPointer(data.ExecutionUnitID)
	}
	if event.InstanceID == nil {
		event.InstanceID = cloneUUIDPointer(finding.InstanceID)
	}
	if event.SessionID == nil {
		event.SessionID = cloneUUIDPointer(finding.SessionID)
	}
	if event.ExecutionUnitID == nil {
		event.ExecutionUnitID = cloneUUIDPointer(finding.ExecutionUnitID)
	}
	return event, nil
}

func runtimeJSONScalarString(raw json.RawMessage) string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&number) == nil {
		return number.String()
	}
	return ""
}

func positiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func parseUUIDPointer(value string) *uuid.UUID {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	return &parsed
}

func cloneUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func findingProcessFactsQuery(finding model.AgentSecurityFinding, events []model.AgentBehaviorEvent) model.AgentBehaviorEventQuery {
	query := model.AgentBehaviorEventQuery{HostID: finding.HostID.String()}
	if finding.InstanceID != nil {
		query.InstanceID = finding.InstanceID.String()
	}
	if finding.SessionID != nil {
		query.SessionID = finding.SessionID.String()
	}
	if finding.ExecutionUnitID != nil {
		query.ExecutionUnitID = finding.ExecutionUnitID.String()
	}
	if len(events) == 0 {
		return query
	}
	first := events[0]
	if query.InstanceID == "" && first.InstanceID != nil {
		query.InstanceID = first.InstanceID.String()
	}
	if query.SessionID == "" && first.SessionID != nil {
		query.SessionID = first.SessionID.String()
	}
	if query.ExecutionUnitID == "" && first.ExecutionUnitID != nil {
		query.ExecutionUnitID = first.ExecutionUnitID.String()
	}
	return query
}

// projectAgentGuardFindingMatchedProcessRoots removes unrelated process facts
// from the Finding response. A matched descendant is promoted to a root when
// its real parent did not match; its PPID remains the real kernel parent PID so
// the rule evidence keeps the original process identity.
func projectAgentGuardFindingMatchedProcessRoots(roots []*agentGuardProcessSnapshot) []*agentGuardFindingProcessNode {
	result := make([]*agentGuardFindingProcessNode, 0)
	var visit func(*agentGuardProcessSnapshot, *agentGuardFindingProcessNode)
	visit = func(node *agentGuardProcessSnapshot, parent *agentGuardFindingProcessNode) {
		if node == nil {
			return
		}
		if node.Matched {
			projected := projectAgentGuardFindingProcessNode(node)
			projected.Children = nil
			if parent == nil {
				projected.ParentID = ""
				result = append(result, projected)
			} else {
				parent.Children = append(parent.Children, projected)
			}
			for _, child := range node.Children {
				visit(child, projected)
			}
			return
		}
		for _, child := range node.Children {
			visit(child, parent)
		}
	}
	for _, root := range roots {
		visit(root, nil)
	}
	return result
}

func projectAgentGuardFindingProcessRoots(roots []*agentGuardProcessSnapshot) []*agentGuardFindingProcessNode {
	result := make([]*agentGuardFindingProcessNode, 0, len(roots))
	for _, root := range roots {
		if root != nil {
			result = append(result, projectAgentGuardFindingProcessNode(root))
		}
	}
	return result
}

func projectAgentGuardFindingProcessNode(node *agentGuardProcessSnapshot) *agentGuardFindingProcessNode {
	if node == nil {
		return nil
	}
	result := &agentGuardFindingProcessNode{
		ID: node.Key, ParentID: node.ParentKey, PID: node.PID, PPID: node.PPID,
		ProcessStartTicks: node.StartTicks, ProcessName: node.Name, ProcessExe: node.Exe,
		Cmdline: node.Cmdline, CommandCwd: node.CommandCwd, CommandVisibility: node.CommandVisibility,
		ProcessStatus: node.Status, FirstSeenAt: formatTime(node.FirstSeenAt), LastSeenAt: formatTime(node.LastSeenAt),
		EventCount: node.EventCount, Matched: node.Matched,
		MatchedEventIDs: append([]string(nil), node.MatchedEventIDs...),
	}
	for _, child := range node.Children {
		if projected := projectAgentGuardFindingProcessNode(child); projected != nil {
			result.Children = append(result.Children, projected)
		}
	}
	return result
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
