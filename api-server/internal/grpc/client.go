package grpc

import (
	"context"
	"encoding/json"
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
	Error        string
}

// NewServerClient creates a new gRPC client connection to the Server service
func NewServerClient(address string) (*ServerClient, error) {
	kaParams := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             3 * time.Second,
		PermitWithoutStream: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kaParams),
		grpc.WithBlock(),
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

// UpdateAgentRules updates the Sigma rules on agents
func (c *ServerClient) UpdateAgentRules(ctx context.Context, req *pb.UpdateAgentRulesRequest) (*pb.UpdateAgentRulesResponse, error) {
	return c.client.UpdateAgentRules(ctx, req)
}

// ExecuteBlockCommand executes a block command on an agent
func (c *ServerClient) ExecuteBlockCommand(ctx context.Context, req *pb.ExecuteBlockCommandRequest) (*pb.ExecuteBlockCommandResponse, error) {
	return c.client.ExecuteBlockCommand(ctx, req)
}

// CollectSoftware collects software list from an agent via the Server service
func (c *ServerClient) CollectSoftware(ctx context.Context, hostID string) (*SoftwareCollectionResponse, error) {
	resp, err := c.client.CollectSoftware(ctx, &pb.CollectSoftwareRequest{
		HostId: hostID,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return &SoftwareCollectionResponse{
			SoftwareJson: resp.SoftwareJson,
			Error:        resp.Error,
		}, fmt.Errorf("software collection failed: %s", resp.Error)
	}
	return &SoftwareCollectionResponse{
		SoftwareJson: resp.SoftwareJson,
		Error:        "",
	}, nil
}

// ExecuteTool synchronously executes a tool on an agent and waits for the result
func (c *ServerClient) ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error) {
	return c.client.ExecuteTool(ctx, &pb.ToolExecuteRequest{
		CallId:         callID,
		HostId:         hostID,
		Tool:           tool,
		Arguments:      arguments,
		TimeoutSeconds: timeoutSeconds,
	})
}

// InstallDetectionPackage installs a detection package on agents
func (c *ServerClient) InstallDetectionPackage(ctx context.Context, hostID string, command *pb.DetectionPackageCommandRequest) (int32, error) {
	resp, err := c.client.InstallDetectionPackage(ctx, &pb.InstallDetectionPackageRequest{
		HostId:  hostID,
		Command: command,
	})
	if err != nil {
		return 0, err
	}
	return resp.AffectedAgents, nil
}

// InstallDetectionPackageFromService installs a detection package using service types
func (c *ServerClient) InstallDetectionPackageFromService(ctx context.Context, hostID string, command interface{}) (int32, error) {
	// Convert service.DetectionPackageCommand to pb.DetectionPackageCommandRequest via JSON
	if cmd, ok := command.(*pb.DetectionPackageCommandRequest); ok {
		return c.InstallDetectionPackage(ctx, hostID, cmd)
	}

	// Use JSON roundtrip to convert any struct to pb type
	jsonData, err := json.Marshal(command)
	if err != nil {
		return 0, fmt.Errorf("marshal command: %w", err)
	}

	cmd := &pb.DetectionPackageCommandRequest{}
	if err := json.Unmarshal(jsonData, cmd); err != nil {
		return 0, fmt.Errorf("unmarshal command: %w", err)
	}

	return c.InstallDetectionPackage(ctx, hostID, cmd)
}

// SyncAgentConfig syncs config to agents
func (c *ServerClient) SyncAgentConfig(ctx context.Context, hostID string, configs []*pb.AgentConfig) (int32, error) {
	resp, err := c.client.SyncAgentConfig(ctx, &pb.SyncAgentConfigRequest{
		HostId:  hostID,
		Configs: configs,
	})
	if err != nil {
		return 0, err
	}
	return resp.AffectedAgents, nil
}

// UninstallDetectionPackage uninstalls a detection package from agents
func (c *ServerClient) UninstallDetectionPackage(ctx context.Context, hostID, packageID, version string) (int32, error) {
	resp, err := c.client.UninstallDetectionPackage(ctx, &pb.UninstallDetectionPackageRequest{
		HostId:    hostID,
		PackageId: packageID,
		Version:   version,
	})
	if err != nil {
		return 0, err
	}
	return resp.AffectedAgents, nil
}
