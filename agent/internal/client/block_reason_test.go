package client

import (
	"context"
	"strings"
	"testing"

	"aegis-agent/internal/blocker"
	pb "aegis-agent/pkg/api/v1"
)

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
