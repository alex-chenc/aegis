package tools

import (
	"context"
	"testing"
	"time"

	"api-server/internal/model"
	pb "api-server/pkg/api/v1"
	"github.com/google/uuid"
)

func TestNormalizeHostStatusFilterSupportsCommonModelArgs(t *testing.T) {
	args := map[string]interface{}{
		"filters": []interface{}{
			map[string]interface{}{
				"field":    "status",
				"operator": "eq",
				"value":    "online",
			},
		},
	}

	if got := normalizeHostStatusFilter(args); got != "online" {
		t.Fatalf("status filter = %q, want online", got)
	}

	args = map[string]interface{}{"agent_status": "离线"}
	if got := normalizeHostStatusFilter(args); got != "offline" {
		t.Fatalf("agent status filter = %q, want offline", got)
	}
}

func TestFilterHostsByAgentStatusUsesHeartbeatFreshness(t *testing.T) {
	hosts := []model.Host{{
		Hostname:        "online-host",
		LastHeartbeatAt: time.Now(),
	}, {
		Hostname:        "offline-host",
		LastHeartbeatAt: time.Now().Add(-10 * time.Minute),
	}}

	online := filterHostsByAgentStatus(hosts, "online")
	if len(online) != 1 || online[0].Hostname != "online-host" {
		t.Fatalf("online hosts = %#v", online)
	}

	offline := filterHostsByAgentStatus(hosts, "offline")
	if len(offline) != 1 || offline[0].Hostname != "offline-host" {
		t.Fatalf("offline hosts = %#v", offline)
	}
}

func TestFilterHostsByAgentStatusPrefersServerConnection(t *testing.T) {
	offlineID := uuid.New()
	onlineID := uuid.New()
	hosts := []model.Host{{
		ID:              offlineID,
		Hostname:        "db-heartbeat-fresh-but-disconnected",
		LastHeartbeatAt: time.Now(),
	}, {
		ID:              onlineID,
		Hostname:        "db-heartbeat-stale-but-connected",
		LastHeartbeatAt: time.Now().Add(-10 * time.Minute),
	}}
	statuses := map[string]agentRuntimeStatus{
		offlineID.String(): {Connected: false},
		onlineID.String():  {Connected: true},
	}

	online := filterHostsByAgentStatusWithRuntime(hosts, "online", statuses)
	if len(online) != 1 || online[0].ID != onlineID {
		t.Fatalf("online hosts = %#v", online)
	}

	decorated := decorateHostWithAgentStatus(hosts[0], statuses)
	if decorated["agent_status"] != "offline" || decorated["agent_connected"] != false {
		t.Fatalf("unexpected decorated offline host: %#v", decorated)
	}
	if decorated["status_source"] != "server_connection" {
		t.Fatalf("status source = %#v, want server_connection", decorated["status_source"])
	}
}

func TestAgentToolHandlerSkipsDisconnectedHostWithoutDispatch(t *testing.T) {
	client := &fakeAgentToolClient{
		status: &pb.GetAgentStatusResponse{Connected: false},
	}
	handler := makeAgentToolHandler(client, "GetRunningProcesses", 30)

	result, err := handler(context.Background(), map[string]interface{}{"host_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.executeCalls != 0 {
		t.Fatalf("expected no ExecuteTool call, got %d", client.executeCalls)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if data["skipped"] != true || data["runtime_available"] != false {
		t.Fatalf("unexpected skipped result: %#v", data)
	}
}

type fakeAgentToolClient struct {
	status       *pb.GetAgentStatusResponse
	statusErr    error
	executeCalls int
}

func (f *fakeAgentToolClient) GetAgentStatus(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
	return f.status, f.statusErr
}

func (f *fakeAgentToolClient) ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error) {
	f.executeCalls++
	return &pb.ToolExecuteResponse{Success: true, Result: "{}"}, nil
}
