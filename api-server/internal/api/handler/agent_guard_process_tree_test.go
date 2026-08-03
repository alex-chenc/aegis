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

func processBehavior(rawID string, hostID, instanceID, sessionID, unitID uuid.UUID, pid, ppid int, ticks, operation, name string, occurredAt time.Time) model.AgentBehaviorEvent {
	return model.AgentBehaviorEvent{
		RawEventID: rawID, HostID: hostID, InstanceID: &instanceID, SessionID: &sessionID,
		ExecutionUnitID: &unitID, Category: "process", Operation: operation,
		PID: &pid, PPID: &ppid, ProcessStartTicks: ticks, ProcessName: name,
		Severity: "info", OccurredAt: occurredAt,
	}
}
