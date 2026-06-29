package service

import (
	"context"
	"testing"

	"api-server/internal/model"
	pb "api-server/pkg/api/v1"

	"go.uber.org/zap"
)

type fakeAssetCollectionServerClient struct {
	resp *pb.ListConnectedAgentsResponse
	err  error
}

func (f *fakeAssetCollectionServerClient) ListConnectedAgents(ctx context.Context) (*pb.ListConnectedAgentsResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeAssetCollectionServerClient) ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error) {
	return &pb.ToolExecuteResponse{CallId: callID, Success: true, Result: `{}`}, nil
}

func TestResolveTargetHostIDsAllHostsUsesConnectedAgents(t *testing.T) {
	svc := NewAssetCollectionService(nil, &fakeAssetCollectionServerClient{
		resp: &pb.ListConnectedAgentsResponse{
			Agents: []*pb.AgentInfo{
				{HostId: "host-online", Connected: true},
				{HostId: "host-offline", Connected: false},
				{HostId: "", Connected: true},
			},
		},
	}, zap.NewNop())

	hostIDs, err := svc.resolveTargetHostIDs(context.Background(), model.TriggerAssetCollectionRequest{
		Scope: "all_hosts",
		Types: []string{"software", "process"},
	})
	if err != nil {
		t.Fatalf("expected connected host resolution to succeed: %v", err)
	}
	if len(hostIDs) != 1 || hostIDs[0] != "host-online" {
		t.Fatalf("expected only connected host IDs, got %#v", hostIDs)
	}
}

func TestResolveTargetHostIDsAllHostsRequiresConnectedAgents(t *testing.T) {
	svc := NewAssetCollectionService(nil, &fakeAssetCollectionServerClient{
		resp: &pb.ListConnectedAgentsResponse{
			Agents: []*pb.AgentInfo{{HostId: "host-offline", Connected: false}},
		},
	}, zap.NewNop())

	_, err := svc.resolveTargetHostIDs(context.Background(), model.TriggerAssetCollectionRequest{
		Scope: "all_hosts",
		Types: []string{"software", "process"},
	})
	if err == nil {
		t.Fatal("expected error when no connected hosts are available")
	}
}

func TestNormalizeCollectTypesPreservesSoftwareAndAnalysis(t *testing.T) {
	got := normalizeCollectTypes([]string{"software", "application_analysis"})
	want := []string{"software", "process", "application_analysis"}
	if len(got) != len(want) {
		t.Fatalf("normalizeCollectTypes = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeCollectTypes = %#v, want %#v", got, want)
		}
	}
}
