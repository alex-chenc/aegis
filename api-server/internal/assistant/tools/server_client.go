package tools

import (
	"context"

	pb "api-server/pkg/api/v1"
)

type agentStatusClient interface {
	GetAgentStatus(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error)
	ListConnectedAgents(ctx context.Context) (*pb.ListConnectedAgentsResponse, error)
}

type agentToolClient interface {
	GetAgentStatus(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error)
	ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error)
}
