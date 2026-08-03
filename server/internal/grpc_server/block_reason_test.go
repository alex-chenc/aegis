package grpc_server

import (
	"context"
	"testing"

	pb "server/pkg/api/v1"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type failingAgentClient struct {
	pb.UnimplementedAgentServiceServer
	reason string
}

func (c *failingAgentClient) Register(context.Context, *pb.RegisterRequest, ...grpc.CallOption) (*pb.RegisterResponse, error) {
	return nil, nil
}

func (c *failingAgentClient) Heartbeat(context.Context, *pb.HeartbeatRequest, ...grpc.CallOption) (*pb.HeartbeatResponse, error) {
	return nil, nil
}

func (c *failingAgentClient) ExecuteCommand(context.Context, ...grpc.CallOption) (grpc.BidiStreamingClient[pb.CommandRequest, pb.CommandRequest], error) {
	return nil, nil
}

func (c *failingAgentClient) CollectSoftwareList(context.Context, *pb.SoftwareListRequest, ...grpc.CallOption) (*pb.SoftwareListResponse, error) {
	return nil, nil
}

func (c *failingAgentClient) ReportEvent(context.Context, *pb.ReportEventRequest, ...grpc.CallOption) (*pb.ReportEventResponse, error) {
	return nil, nil
}

func (c *failingAgentClient) ExecuteTool(context.Context, *pb.ToolRequest, ...grpc.CallOption) (*pb.ToolResponse, error) {
	return nil, nil
}

func (c *failingAgentClient) UpdateRules(context.Context, *pb.RuleUpdateRequest, ...grpc.CallOption) (*pb.RuleUpdateResponse, error) {
	return nil, nil
}

func (c *failingAgentClient) ExecuteBlockCommand(context.Context, *pb.BlockCommand, ...grpc.CallOption) (*pb.BlockResponse, error) {
	return &pb.BlockResponse{Success: false, Error: c.reason}, nil
}

func (c *failingAgentClient) SyncConfig(context.Context, *pb.ConfigSyncRequest, ...grpc.CallOption) (*pb.ConfigSyncResponse, error) {
	return &pb.ConfigSyncResponse{Success: true}, nil
}

func TestExecuteBlockCommandPreservesAgentFailureReason(t *testing.T) {
	hostID := uuid.New()
	unitID := uuid.New()
	reason := "freeze_execution_unit failed: cgroup freezer unavailable"
	grpcServer := &GRPCServer{}
	grpcServer.agentConnections.Store(hostID, &AgentConnection{
		HostID:         hostID,
		CallbackClient: &failingAgentClient{reason: reason},
		Ctx:            context.Background(),
	})

	impl := &APIServerToServerImpl{grpcServer: grpcServer}
	resp, err := impl.ExecuteBlockCommand(context.Background(), &pb.ExecuteBlockCommandRequest{
		CommandId: "AG-GUARD-" + uuid.NewString(),
		HostId:    hostID.String(),
		Action:    "freeze_execution_unit",
		Target:    unitID.String(),
	})
	if err != nil {
		t.Fatalf("expected nil grpc error, got %v", err)
	}
	if resp.Success {
		t.Fatal("expected failed response")
	}
	if resp.Error != reason {
		t.Fatalf("expected exact agent reason %q, got %q", reason, resp.Error)
	}
}
