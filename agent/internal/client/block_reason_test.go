package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aegis-agent/internal/agentguard"
	"aegis-agent/internal/blocker"
	pb "aegis-agent/pkg/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestReportEventGRPCCodePreservesSafeTransportClassification(t *testing.T) {
	err := status.Error(codes.ResourceExhausted, "payload exceeds server limit")
	if got := reportEventGRPCCode(err); got != "resourceexhausted" {
		t.Fatalf("reportEventGRPCCode() = %q, want resourceexhausted", got)
	}
}

type fakeAgentGuardActionHandler struct {
	calls  int
	action string
	target string
	err    error
}

func (h *fakeAgentGuardActionHandler) ExecuteAgentGuardAction(
	_ context.Context, _, action, target, _ string,
) (agentguard.ActionResult, error) {
	h.calls++
	h.action, h.target = action, target
	return agentguard.ActionResult{Action: action, ExecutionUnitID: target}, h.err
}

func TestHandleBlockCommandReturnsAgentFailureReason(t *testing.T) {
	c := &Client{blocker: blocker.NewBlocker(t.TempDir())}

	resp, err := c.HandleBlockCommand(context.Background(), &pb.BlockCommand{
		CommandId: "BLK-test",
		Action:    "quarantine_file",
		Target:    "",
	})
	if err != nil {
		t.Fatalf("expected nil grpc error, got %v", err)
	}
	if resp.Success {
		t.Fatal("expected failed block response")
	}
	for _, want := range []string{"quarantine_file", "missing target"} {
		if !strings.Contains(resp.Error, want) {
			t.Fatalf("expected agent error to contain %q, got %q", want, resp.Error)
		}
	}
}

func TestHandleBlockCommandRoutesAgentGuardUUIDToLocalManager(t *testing.T) {
	handler := &fakeAgentGuardActionHandler{}
	c := &Client{blocker: blocker.NewBlocker(t.TempDir()), agentGuardActions: handler}
	target := "66a1dfb2-a483-4b79-af70-2f6cc21f71d0"
	resp, err := c.HandleBlockCommand(context.Background(), &pb.BlockCommand{
		CommandId: "cmd-1", Action: agentguard.ActionFreezeExecutionUnit, Target: target,
	})
	if err != nil || !resp.Success {
		t.Fatalf("agent guard action failed: response=%+v err=%v", resp, err)
	}
	if handler.calls != 1 || handler.action != agentguard.ActionFreezeExecutionUnit || handler.target != target {
		t.Fatalf("action was not routed to local manager: %+v", handler)
	}
}

func TestHandleBlockCommandDoesNotFallBackToLegacyBlockerForAgentGuardAction(t *testing.T) {
	handler := &fakeAgentGuardActionHandler{err: errors.New("agent_guard_action_target_invalid")}
	c := &Client{blocker: blocker.NewBlocker(t.TempDir()), agentGuardActions: handler}
	resp, err := c.HandleBlockCommand(context.Background(), &pb.BlockCommand{
		CommandId: "cmd-2", Action: agentguard.ActionKillExecutionUnit, Target: "1234",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Success || !strings.Contains(resp.Error, "target_invalid") || strings.Contains(resp.Error, "target \"") {
		t.Fatalf("unsafe or misleading response: %+v", resp)
	}
}
