package handler

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"api-server/internal/model"
	"api-server/internal/service"
)

const agentGuardProcessFactLimit = 5000

type agentGuardProcessSnapshot struct {
	Key               string
	PID               int
	PPID              int
	StartTicks        string
	Name              string
	Exe               string
	Cmdline           string
	CommandCwd        string
	CommandVisibility string
	Status            string
	ParentKey         string
	FirstSeenAt       time.Time
	LastSeenAt        time.Time
	EventCount        int64
	Matched           bool
	MatchedEventIDs   []string
	Children          []*agentGuardProcessSnapshot
}

type agentGuardProcessTree struct {
	Nodes map[string]*agentGuardProcessSnapshot
	Roots []*agentGuardProcessSnapshot
}

func buildAgentGuardProcessTree(events []model.AgentBehaviorEvent) agentGuardProcessTree {
	tree := agentGuardProcessTree{Nodes: make(map[string]*agentGuardProcessSnapshot)}
	latest := make(map[string]model.AgentBehaviorEvent)
	for _, event := range events {
		if event.Category != "process" || event.PID == nil || *event.PID <= 0 ||
			strings.TrimSpace(event.ProcessStartTicks) == "" || event.ProcessStartTicks == "0" {
			continue
		}
		key := agentGuardProcessKey(*event.PID, event.ProcessStartTicks)
		node := tree.Nodes[key]
		if node == nil {
			node = &agentGuardProcessSnapshot{
				Key: key, PID: *event.PID, StartTicks: event.ProcessStartTicks,
				Status: "running", FirstSeenAt: event.OccurredAt, LastSeenAt: event.OccurredAt,
			}
			tree.Nodes[key] = node
		}
		node.EventCount++
		if node.FirstSeenAt.IsZero() || event.OccurredAt.Before(node.FirstSeenAt) {
			node.FirstSeenAt = event.OccurredAt
		}
		current, exists := latest[key]
		if !exists || event.OccurredAt.After(current.OccurredAt) ||
			(event.OccurredAt.Equal(current.OccurredAt) && event.AgentSequence > current.AgentSequence) {
			latest[key] = event
			node.LastSeenAt = event.OccurredAt
			if event.PPID != nil && *event.PPID >= 0 {
				node.PPID = *event.PPID
			}
			if event.ProcessName != "" {
				node.Name = event.ProcessName
			}
			if event.ProcessExe != "" {
				node.Exe = event.ProcessExe
			}
			if cmdline := agentGuardCommandLine(event.CommandArgv); cmdline != "" {
				node.Cmdline = cmdline
			}
			if event.CommandCwd != "" {
				node.CommandCwd = event.CommandCwd
			}
			if event.CommandVisibility != "" {
				node.CommandVisibility = event.CommandVisibility
			}
			if event.Operation == "exit" {
				node.Status = "stopped"
			} else {
				node.Status = "running"
			}
		}
	}

	byPID := make(map[int][]*agentGuardProcessSnapshot)
	for _, node := range tree.Nodes {
		byPID[node.PID] = append(byPID[node.PID], node)
	}
	for _, candidates := range byPID {
		sort.Slice(candidates, func(i, j int) bool {
			return processTicks(candidates[i].StartTicks) < processTicks(candidates[j].StartTicks)
		})
	}
	for _, node := range tree.Nodes {
		parent := selectAgentGuardProcessParent(node, byPID[node.PPID])
		if parent == nil {
			tree.Roots = append(tree.Roots, node)
			continue
		}
		node.ParentKey = parent.Key
		parent.Children = append(parent.Children, node)
	}
	for _, node := range tree.Nodes {
		sortAgentGuardProcessNodes(node.Children)
	}
	sortAgentGuardProcessNodes(tree.Roots)
	return tree
}

func markAgentGuardProcessTreeMatches(tree *agentGuardProcessTree, events []model.AgentBehaviorEvent) {
	if tree == nil {
		return
	}
	byPID := make(map[int][]*agentGuardProcessSnapshot)
	for _, node := range tree.Nodes {
		byPID[node.PID] = append(byPID[node.PID], node)
	}
	for _, event := range events {
		if event.PID == nil || *event.PID <= 0 || strings.TrimSpace(event.ProcessStartTicks) == "" ||
			event.ProcessStartTicks == "0" {
			continue
		}
		key := agentGuardProcessKey(*event.PID, event.ProcessStartTicks)
		node := tree.Nodes[key]
		if node == nil {
			// A process fact can be absent when the rule event arrived before the
			// process snapshot. Keep the matched process visible instead of
			// dropping the only process identity available for the rule hit.
			node = &agentGuardProcessSnapshot{
				Key: key, PID: *event.PID, StartTicks: event.ProcessStartTicks,
				Status: "unknown", FirstSeenAt: event.OccurredAt, LastSeenAt: event.OccurredAt,
			}
			if event.PPID != nil {
				node.PPID = *event.PPID
			}
			node.Name = event.ProcessName
			node.Exe = event.ProcessExe
			node.Cmdline = agentGuardCommandLine(event.CommandArgv)
			node.CommandCwd = event.CommandCwd
			node.CommandVisibility = event.CommandVisibility
			tree.Nodes[key] = node
			byPID[node.PID] = append(byPID[node.PID], node)
			parent := selectAgentGuardProcessParent(node, byPID[node.PPID])
			if parent == nil {
				tree.Roots = append(tree.Roots, node)
			} else {
				node.ParentKey = parent.Key
				parent.Children = append(parent.Children, node)
				sortAgentGuardProcessNodes(parent.Children)
			}
		}
		node.Matched = true
		if event.RawEventID != "" && !containsString(node.MatchedEventIDs, event.RawEventID) {
			node.MatchedEventIDs = append(node.MatchedEventIDs, event.RawEventID)
		}
	}
	sortAgentGuardProcessNodes(tree.Roots)
}

func selectAgentGuardProcessParent(child *agentGuardProcessSnapshot, candidates []*agentGuardProcessSnapshot) *agentGuardProcessSnapshot {
	if child == nil || child.PPID <= 0 {
		return nil
	}
	childTicks := processTicks(child.StartTicks)
	var selected *agentGuardProcessSnapshot
	for _, candidate := range candidates {
		candidateTicks := processTicks(candidate.StartTicks)
		if candidate.Key == child.Key || candidateTicks == 0 || childTicks == 0 || candidateTicks >= childTicks {
			continue
		}
		if selected == nil || candidateTicks > processTicks(selected.StartTicks) {
			selected = candidate
		}
	}
	return selected
}

func sortAgentGuardProcessNodes(nodes []*agentGuardProcessSnapshot) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].FirstSeenAt.Equal(nodes[j].FirstSeenAt) {
			if nodes[i].PID == nodes[j].PID {
				return processTicks(nodes[i].StartTicks) < processTicks(nodes[j].StartTicks)
			}
			return nodes[i].PID < nodes[j].PID
		}
		return nodes[i].FirstSeenAt.Before(nodes[j].FirstSeenAt)
	})
}

func processTicks(value string) uint64 {
	parsed, _ := strconv.ParseUint(value, 10, 64)
	return parsed
}

func agentGuardProcessKey(pid int, startTicks string) string {
	return strconv.Itoa(pid) + ":" + strings.TrimSpace(startTicks)
}

func agentGuardSessionLabel(session model.AgentBehaviorSession) string {
	if value := strings.TrimSpace(session.ExternalSessionID); value != "" {
		return value
	}
	if session.Source == "activity_window" {
		return "inferred activity window"
	}
	return session.Source + " session"
}

func agentGuardCommandLine(raw []byte) string {
	var argv []string
	if len(raw) == 0 || json.Unmarshal(raw, &argv) != nil {
		return ""
	}
	// Linux /proc/<pid>/cmdline stores argv entries separated by NUL bytes.
	// The API carries those entries as JSON, so join them in the same order.
	// A malformed or padded historical argv must not put control characters in
	// the UI; normalize it to one readable line while keeping redaction intact.
	parts := make([]string, 0, len(argv))
	for _, argument := range argv {
		argument = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, argument)
		argument = strings.Join(strings.Fields(argument), " ")
		if argument != "" {
			parts = append(parts, argument)
		}
	}
	return strings.Join(parts, " ")
}

func (h *AgentGuardHandler) panoramaProcessNode(
	ref service.AgentGuardPanoramaNodeRef,
	process *agentGuardProcessSnapshot,
) (agentGuardPanoramaNode, error) {
	token, err := h.scopeSigner.SignPanoramaNode(service.AgentGuardPanoramaNodeRef{
		NodeType: "process", ObjectID: process.Key, HostID: ref.HostID, AssetID: ref.AssetID,
		InstanceID: ref.InstanceID, SessionID: ref.SessionID,
		ExecutionUnitID: ref.ExecutionUnitID, ProcessPID: process.PID,
		ProcessStartTicks: process.StartTicks,
	}, agentGuardPanoramaNodeTTL)
	if err != nil {
		return agentGuardPanoramaNode{}, err
	}
	label := "PID " + strconv.Itoa(process.PID)
	if process.Cmdline != "" {
		label += " · " + process.Cmdline
	} else if process.Name != "" {
		label += " · " + process.Name
	}
	return agentGuardPanoramaNode{
		ID: token, NodeType: "process", Label: label, HasChildren: len(process.Children) > 0,
		ChildCount: int64(len(process.Children)),
		OccurredAt: process.LastSeenAt.UTC().Format(time.RFC3339Nano),
		PID:        process.PID, PPID: process.PPID, StartTicks: process.StartTicks,
		ProcessStatus: process.Status, Cmdline: process.Cmdline,
	}, nil
}

func (h *AgentGuardHandler) panoramaBehaviorNode(
	ref service.AgentGuardPanoramaNodeRef,
	behavior model.AgentBehaviorEvent,
	trustedSession *model.AgentBehaviorSession,
) (agentGuardPanoramaNode, error) {
	nodeType := "behavior"
	if behavior.Category == "tool" {
		nodeType = "tool_call"
	}
	token, err := h.scopeSigner.SignPanoramaNode(service.AgentGuardPanoramaNodeRef{
		NodeType: nodeType, ObjectID: behavior.RawEventID, HostID: ref.HostID, AssetID: ref.AssetID,
		InstanceID: ref.InstanceID, SessionID: ref.SessionID, ExecutionUnitID: ref.ExecutionUnitID,
	}, agentGuardPanoramaNodeTTL)
	if err != nil {
		return agentGuardPanoramaNode{}, err
	}
	label := strings.TrimSpace(behavior.ProcessName)
	projection := service.ProjectAgentGuardToolEvidence(behavior, trustedSession)
	if behavior.Category == "tool" {
		label = strings.ReplaceAll(behavior.Operation, "_", " ")
		if projection.Trust != nil && projection.Trust.ToolSemantics == service.AgentGuardToolSemanticsTrusted {
			label = strings.TrimSpace(behavior.ResourceIdentity)
		}
	}
	if label == "" {
		label = behavior.Category + ":" + behavior.Operation
	}
	node := agentGuardPanoramaNode{
		ID: token, NodeType: nodeType, Label: label, Severity: behavior.Severity,
		HasChildren: behavior.Category != "tool" && (behavior.ResourceType != "" || behavior.RuleID != ""),
		OccurredAt:  behavior.OccurredAt.UTC().Format(time.RFC3339Nano),
		Trust:       projection.Trust, Collection: projection.Collection,
		StartTicks: behavior.ProcessStartTicks,
	}
	if behavior.PID != nil {
		node.PID = *behavior.PID
	}
	if behavior.PPID != nil {
		node.PPID = *behavior.PPID
	}
	if behavior.Category == "tool" {
		attributes := panoramaBehaviorResourceAttributes(behavior.Resource)
		node.ToolName = strings.TrimSpace(behavior.ResourceIdentity)
		node.ToolCallID = panoramaStringAttribute(attributes, "tool_call_id")
		node.TurnID = panoramaStringAttribute(attributes, "turn_id")
		node.Command = panoramaStringAttribute(attributes, "command")
		node.ToolInput = attributes["tool_input"]
		node.ToolResponse = attributes["tool_response"]
		node.CorrelationStatus = panoramaStringAttribute(attributes, "correlation_status")
		node.CorrelationMethod = panoramaStringAttribute(attributes, "correlation_method")
		if node.CorrelationStatus == "unmatched" {
			// The tool hook PID identifies the controller, not the shell/worker
			// that executed the command. Do not expose that anchor as the tool PID.
			node.PID = 0
			node.PPID = 0
			node.StartTicks = ""
		}
		if node.Command == "" && node.ToolInput != nil {
			if input, ok := node.ToolInput.(map[string]any); ok {
				node.Command = panoramaStringAttribute(input, "command")
			}
		}
	}
	return node, nil
}

func panoramaBehaviorResourceAttributes(raw []byte) map[string]any {
	var resource map[string]any
	if json.Unmarshal(raw, &resource) != nil {
		return nil
	}
	attributes, _ := resource["attributes"].(map[string]any)
	return attributes
}

func panoramaStringAttribute(attributes map[string]any, key string) string {
	if attributes == nil {
		return ""
	}
	value, _ := attributes[key].(string)
	return strings.TrimSpace(value)
}
