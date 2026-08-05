package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/service"

	"github.com/google/uuid"
)

func TestBuildAgentGuardProcessTreeUsesPIDStartTicksAndLatestExitState(t *testing.T) {
	base := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	events := []model.AgentBehaviorEvent{
		{RawEventID: "child-exec", Category: "process", Operation: "exec", PID: intPtr(4110), PPID: intPtr(4100), ProcessStartTicks: "110", ProcessName: "bash", OccurredAt: base.Add(time.Second)},
		{RawEventID: "root-exec", Category: "process", Operation: "exec", PID: intPtr(4100), PPID: intPtr(1), ProcessStartTicks: "100", ProcessName: "codex", OccurredAt: base},
		{RawEventID: "child-exit", Category: "process", Operation: "exit", PID: intPtr(4110), PPID: intPtr(4100), ProcessStartTicks: "110", ProcessName: "bash", OccurredAt: base.Add(2 * time.Second)},
		// PID reuse must be a distinct node and must not overwrite start_ticks=110.
		{RawEventID: "reused-exec", Category: "process", Operation: "exec", PID: intPtr(4110), PPID: intPtr(1), ProcessStartTicks: "999", ProcessName: "python", OccurredAt: base.Add(3 * time.Second)},
	}

	tree := buildAgentGuardProcessTree(events)
	if len(tree.Nodes) != 3 {
		t.Fatalf("nodes=%d tree=%#v", len(tree.Nodes), tree)
	}
	root := tree.Nodes["4100:100"]
	child := tree.Nodes["4110:110"]
	reused := tree.Nodes["4110:999"]
	if root == nil || child == nil || reused == nil {
		t.Fatalf("process identities were merged: %#v", tree.Nodes)
	}
	if child.ParentKey != root.Key || child.Status != "stopped" || child.EventCount != 2 {
		t.Fatalf("child projection=%#v", child)
	}
	if reused.ParentKey != "" || reused.Status != "running" {
		t.Fatalf("PID reuse projection=%#v", reused)
	}
	if len(tree.Roots) != 2 {
		t.Fatalf("root count=%d roots=%#v", len(tree.Roots), tree.Roots)
	}
}

func TestAgentGuardSessionAndProcessLabelsExposeRealSessionIDPIDAndRedactedCmdline(t *testing.T) {
	session := model.AgentBehaviorSession{ExternalSessionID: "thr_real_123", Source: "agent_official"}
	if got := agentGuardSessionLabel(session); got != "thr_real_123" {
		t.Fatalf("real session id was not displayed: %q", got)
	}
	event := model.AgentBehaviorEvent{
		Category: "process", Operation: "exec", PID: intPtr(4100), PPID: intPtr(1),
		ProcessStartTicks: "100", ProcessName: "codex", ProcessExe: "/usr/bin/codex",
		CommandArgv: []byte(`["codex","app-server","--token=[REDACTED]"]`),
		OccurredAt:  time.Now().UTC(),
	}
	tree := buildAgentGuardProcessTree([]model.AgentBehaviorEvent{event})
	root := tree.Nodes["4100:100"]
	if root == nil || root.Cmdline != "codex app-server --token=[REDACTED]" {
		t.Fatalf("cmdline projection=%#v", root)
	}
	handler := &AgentGuardHandler{scopeSigner: testAgentGuardSigner(t)}
	node, err := handler.panoramaProcessNode(service.AgentGuardPanoramaNodeRef{
		HostID: uuid.NewString(), InstanceID: uuid.NewString(), SessionID: uuid.NewString(),
		ExecutionUnitID: uuid.NewString(),
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if node.Label != "PID 4100 · codex app-server --token=[REDACTED]" || node.Cmdline != root.Cmdline {
		t.Fatalf("process label=%#v", node)
	}
}

func TestAgentGuardCommandLineNormalizesProcfsPaddingAndControlBytes(t *testing.T) {
	raw := []byte(`["/bin/sh\u0000\u0000","-c","printf\u0003 ok"]`)
	if got := agentGuardCommandLine(raw); got != "/bin/sh -c printf ok" {
		t.Fatalf("normalized cmdline=%q", got)
	}
}

func intPtr(value int) *int { return &value }

func TestAgentGuardPanoramaRebuildsPIDTreeFromFreshProcessFacts(t *testing.T) {
	hostID, instanceID, sessionID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	base := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	query := &fakeAgentGuardQuery{
		units:    []model.AgentExecutionUnit{{ID: unitID, HostID: hostID, InstanceID: instanceID, UnitType: "local_process_tree"}},
		sessions: []model.AgentBehaviorSession{{ID: sessionID, HostID: hostID, InstanceID: instanceID, ExecutionUnitID: &unitID}},
		behaviors: []model.AgentBehaviorEvent{
			processBehavior(uuid.NewString(), hostID, instanceID, sessionID, unitID, 4100, 1, "100", "exec", "codex", base),
			processBehavior(uuid.NewString(), hostID, instanceID, sessionID, unitID, 4110, 4100, "110", "fork", "bash", base.Add(time.Second)),
			processBehavior(uuid.NewString(), hostID, instanceID, sessionID, unitID, 4110, 4100, "110", "exit", "bash", base.Add(2*time.Second)),
		},
	}
	signer := testAgentGuardSigner(t)
	engine := newAgentGuardHandlerTestEngine(t, query, signer)
	unitToken, err := signer.SignPanoramaNode(service.AgentGuardPanoramaNodeRef{
		NodeType: "execution_unit", ObjectID: unitID.String(), HostID: hostID.String(),
		InstanceID: instanceID.String(), SessionID: sessionID.String(), ExecutionUnitID: unitID.String(),
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	response := serveAgentGuardRequest(engine, http.MethodGet,
		"/api/v1/agent-guard/panorama/nodes/"+unitToken+"/children?page=1&page_size=100", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("unit children status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Items []struct {
				ID            string `json:"id"`
				NodeType      string `json:"node_type"`
				PID           int    `json:"pid"`
				PPID          int    `json:"ppid"`
				StartTicks    string `json:"process_start_ticks"`
				ProcessStatus string `json:"process_status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Items) != 1 || body.Data.Items[0].NodeType != "process" ||
		body.Data.Items[0].PID != 4100 || body.Data.Items[0].PPID != 1 || body.Data.Items[0].StartTicks != "100" {
		t.Fatalf("root tree=%s", response.Body.String())
	}
	rootChildren := serveAgentGuardRequest(engine, http.MethodGet,
		"/api/v1/agent-guard/panorama/nodes/"+body.Data.Items[0].ID+"/children?page=1&page_size=100", nil)
	if rootChildren.Code != http.StatusOK || !strings.Contains(rootChildren.Body.String(), `"pid":4110`) ||
		!strings.Contains(rootChildren.Body.String(), `"process_status":"stopped"`) {
		t.Fatalf("child tree status=%d body=%s", rootChildren.Code, rootChildren.Body.String())
	}
}

func TestAgentGuardPanoramaReturnsToolCallsForSelectedSession(t *testing.T) {
	hostID, assetID, instanceID, sessionID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	query := &fakeAgentGuardQuery{
		instances: []model.AgentRuntimeInstance{{ID: instanceID, HostID: hostID, AssetID: &assetID}},
		sessions: []model.AgentBehaviorSession{{
			ID: sessionID, HostID: hostID, InstanceID: instanceID, ExecutionUnitID: &unitID,
		}},
		units: []model.AgentExecutionUnit{{ID: unitID, HostID: hostID, InstanceID: instanceID}},
		behaviors: []model.AgentBehaviorEvent{
			toolBehavior("tool-1", hostID, instanceID, sessionID, unitID, "Bash", "call-1", "echo hello", time.Now().UTC()),
		},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(engine, http.MethodGet,
		"/api/v1/agent-guard/panorama?asset_id="+assetID.String()+"&session_id="+sessionID.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"node_type":"tool_call"`) ||
		strings.Contains(response.Body.String(), `"node_type":"process"`) ||
		!strings.Contains(response.Body.String(), `"command":"echo hello"`) {
		t.Fatalf("session tool call response=%s", response.Body.String())
	}
}

func TestAgentGuardPanoramaPaginatesSelectedSessionToolCalls(t *testing.T) {
	hostID, assetID, instanceID, sessionID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	base := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	query := &fakeAgentGuardQuery{
		instances: []model.AgentRuntimeInstance{{ID: instanceID, HostID: hostID, AssetID: &assetID}},
		sessions: []model.AgentBehaviorSession{{
			ID: sessionID, HostID: hostID, InstanceID: instanceID, ExecutionUnitID: &unitID,
		}},
		units: []model.AgentExecutionUnit{{ID: unitID, HostID: hostID, InstanceID: instanceID}},
		behaviors: []model.AgentBehaviorEvent{
			toolBehavior("tool-1", hostID, instanceID, sessionID, unitID, "Bash", "call-1", "echo one", base),
			toolBehavior("tool-2", hostID, instanceID, sessionID, unitID, "Bash", "call-2", "echo two", base.Add(time.Second)),
		},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(engine, http.MethodGet,
		"/api/v1/agent-guard/panorama?asset_id="+assetID.String()+"&session_id="+sessionID.String()+"&page=2&page_size=1", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Items []struct {
				PID int `json:"pid"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Total != 2 || len(body.Data.Items) != 1 || !strings.Contains(response.Body.String(), `"command":"echo two"`) {
		t.Fatalf("unexpected paginated tool calls: %s", response.Body.String())
	}
}

func processBehavior(rawID string, hostID, instanceID, sessionID, unitID uuid.UUID, pid, ppid int, ticks, operation, name string, occurredAt time.Time) model.AgentBehaviorEvent {
	return model.AgentBehaviorEvent{
		RawEventID: rawID, HostID: hostID, InstanceID: &instanceID, SessionID: &sessionID,
		ExecutionUnitID: &unitID, Category: "process", Operation: operation,
		PID: &pid, PPID: &ppid, ProcessStartTicks: ticks, ProcessName: name,
		Severity: "info", OccurredAt: occurredAt,
	}
}

func toolBehavior(rawID string, hostID, instanceID, sessionID, unitID uuid.UUID, name, callID, command string, occurredAt time.Time) model.AgentBehaviorEvent {
	data, _ := json.Marshal(map[string]any{
		"type": "tool", "identity": name, "attributes": map[string]any{
			"tool_call_id": callID, "command": command, "correlation_status": "matched",
		},
	})
	pid, ppid := 4100, 1
	return model.AgentBehaviorEvent{
		RawEventID: rawID, HostID: hostID, InstanceID: &instanceID, SessionID: &sessionID,
		ExecutionUnitID: &unitID, Category: "tool", Operation: "tool_call_completed", Outcome: "success",
		PID: &pid, PPID: &ppid, ProcessStartTicks: "100", ProcessName: "codex", ResourceIdentity: name,
		ResourceType: "tool", Resource: data, CommandArgv: []byte(`[]`), Severity: "info", OccurredAt: occurredAt,
	}
}
