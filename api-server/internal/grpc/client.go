package grpc

import (
	"context"
	"fmt"
	"time"

	pb "api-server/pkg/api/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ServerClient is the gRPC client for communicating with the Server service
type ServerClient struct {
	conn   *grpc.ClientConn
	client pb.APIServerToServerClient
}

// SoftwareCollectionResponse represents software collection response
type SoftwareCollectionResponse struct {
	SoftwareJson string
}

// NewServerClient creates a new gRPC client connection to the Server service
func NewServerClient(address string) (*ServerClient, error) {
	kaParams := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             3 * time.Second,
		PermitWithoutStream: true,
	}

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kaParams),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server gRPC: %w", err)
	}

	return &ServerClient{
		conn:   conn,
		client: pb.NewAPIServerToServerClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *ServerClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ForwardCommand forwards a command to an agent via the Server service
func (c *ServerClient) ForwardCommand(ctx context.Context, req *pb.ForwardCommandRequest) (*pb.ForwardCommandResponse, error) {
	return c.client.ForwardCommand(ctx, req)
}

// GetAgentStatus gets the status of a specific agent
func (c *ServerClient) GetAgentStatus(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
	return c.client.GetAgentStatus(ctx, &pb.GetAgentStatusRequest{
		HostId: hostID,
	})
}

// ListConnectedAgents lists all connected agents
func (c *ServerClient) ListConnectedAgents(ctx context.Context) (*pb.ListConnectedAgentsResponse, error) {
	return c.client.ListConnectedAgents(ctx, &pb.ListConnectedAgentsRequest{
		Page:     1,
		PageSize: 1000,
	})
}

// HealthCheck performs a health check on the Server service
func (c *ServerClient) HealthCheck(ctx context.Context) (*pb.HealthCheckResponse, error) {
	return c.client.HealthCheck(ctx, &pb.HealthCheckRequest{})
}

// UpdateAgentRules updates the Sigma rules on agents
func (c *ServerClient) UpdateAgentRules(ctx context.Context, req *pb.UpdateAgentRulesRequest) (*pb.UpdateAgentRulesResponse, error) {
	return c.client.UpdateAgentRules(ctx, req)
}

// ExecuteBlockCommand executes a block command on an agent
func (c *ServerClient) ExecuteBlockCommand(ctx context.Context, req *pb.ExecuteBlockCommandRequest) (*pb.ExecuteBlockCommandResponse, error) {
	return c.client.ExecuteBlockCommand(ctx, req)
}

// CollectSoftware collects software list from an agent via the Server service
// Note: This requires the Server to implement a CollectSoftware RPC method
func (c *ServerClient) CollectSoftware(ctx context.Context, hostID string) (*SoftwareCollectionResponse, error) {
	// This is a placeholder - the actual implementation depends on what RPC
	// method the Server exposes for software collection
	// For now, return empty response
	return &SoftwareCollectionResponse{SoftwareJson: "[]"}, nil
}
