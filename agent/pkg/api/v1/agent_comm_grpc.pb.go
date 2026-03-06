// Generated gRPC client code for Agent communication
package pb

import (
	"context"
	"google.golang.org/grpc"
)

// AgentServiceClient is the client API for AgentService service
type AgentServiceClient interface {
	Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*RegisterResponse, error)
	Heartbeat(ctx context.Context, in *HeartbeatRequest, opts ...grpc.CallOption) (*HeartbeatResponse, error)
	ExecuteCommand(ctx context.Context, opts ...grpc.CallOption) (AgentService_ExecuteCommandClient, error)
}

type agentServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAgentServiceClient(cc grpc.ClientConnInterface) AgentServiceClient {
	return &agentServiceClient{cc}
}

func (c *agentServiceClient) Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*RegisterResponse, error) {
	out := new(RegisterResponse)
	err := c.cc.Invoke(ctx, "/agent_comm.v1.AgentService/Register", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *agentServiceClient) Heartbeat(ctx context.Context, in *HeartbeatRequest, opts ...grpc.CallOption) (*HeartbeatResponse, error) {
	out := new(HeartbeatResponse)
	err := c.cc.Invoke(ctx, "/agent_comm.v1.AgentService/Heartbeat", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AgentService_ExecuteCommandClient is the client stream for ExecuteCommand
type AgentService_ExecuteCommandClient interface {
	grpc.ClientStream
	Send(*CommandRequest) error
	Recv() (*CommandRequest, error)
}

type agentServiceExecuteCommandClient struct {
	grpc.ClientStream
}

func (c *agentServiceExecuteCommandClient) Send(m *CommandRequest) error {
	return c.ClientStream.SendMsg(m)
}

func (c *agentServiceExecuteCommandClient) Recv() (*CommandRequest, error) {
	m := new(CommandRequest)
	if err := c.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *agentServiceClient) ExecuteCommand(ctx context.Context, opts ...grpc.CallOption) (AgentService_ExecuteCommandClient, error) {
	stream, err := c.cc.NewStream(ctx, &AgentService_ServiceDesc.Streams[0], "/agent_comm.v1.AgentService/ExecuteCommand", opts...)
	if err != nil {
		return nil, err
	}
	x := &agentServiceExecuteCommandClient{stream}
	return x, nil
}
