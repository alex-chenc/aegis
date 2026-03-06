// Package pb contains generated protobuf code for Agent communication
// This is a stub file - in production, generate from agent_comm.proto
package pb

import (
	"context"

	"google.golang.org/grpc"
)

// AgentServiceServer is the server API for AgentService service
type AgentServiceServer interface {
	Register(context.Context, *RegisterRequest) (*RegisterResponse, error)
	Heartbeat(context.Context, *HeartbeatRequest) (*HeartbeatResponse, error)
	ExecuteCommand(AgentService_ExecuteCommandServer) error
}

// AgentService_ExecuteCommandServer is the stream for ExecuteCommand
type AgentService_ExecuteCommandServer interface {
	Send(*CommandRequest) error
	Recv() (*CommandRequest, error)
	grpc.ServerStream
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	HostId    string     `protobuf:"bytes,1,opt,name=host_id,json=hostId,proto3" json:"host_id,omitempty"`
	AssetInfo *AssetInfo `protobuf:"bytes,2,opt,name=asset_info,json=assetInfo,proto3" json:"asset_info,omitempty"`
	AuthToken string     `protobuf:"bytes,3,opt,name=auth_token,json=authToken,proto3" json:"auth_token,omitempty"`
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	HostId  string `protobuf:"bytes,2,opt,name=host_id,json=hostId,proto3" json:"host_id,omitempty"`
	Message string `protobuf:"bytes,3,opt,name=message,proto3" json:"message,omitempty"`
}

// AssetInfo 资产信息
type AssetInfo struct {
	IpAddress    string `protobuf:"bytes,1,opt,name=ip_address,json=ipAddress,proto3" json:"ip_address,omitempty"`
	Hostname     string `protobuf:"bytes,2,opt,name=hostname,proto3" json:"hostname,omitempty"`
	OsType       string `protobuf:"bytes,3,opt,name=os_type,json=osType,proto3" json:"os_type,omitempty"`
	OsVersion    string `protobuf:"bytes,4,opt,name=os_version,json=osVersion,proto3" json:"os_version,omitempty"`
	Arch         string `protobuf:"bytes,5,opt,name=arch,proto3" json:"arch,omitempty"`
	AgentVersion string `protobuf:"bytes,6,opt,name=agent_version,json=agentVersion,proto3" json:"agent_version,omitempty"`
}

// HeartbeatRequest 心跳请求
type HeartbeatRequest struct {
	HostId    string `protobuf:"bytes,1,opt,name=host_id,json=hostId,proto3" json:"host_id,omitempty"`
	Timestamp int64  `protobuf:"varint,2,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
}

// HeartbeatResponse 心跳响应
type HeartbeatResponse struct {
	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Message string `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
}

// CommandRequest 命令请求
type CommandRequest struct {
	Request isCommandRequest_Request `protobuf:"oneof:request,opt,name=request,proto3" json:"request,omitempty"`
}

type isCommandRequest_Request interface {
	isCommandRequest_Request()
}

type CommandRequest_Execute struct {
	Execute *CommandExecute `protobuf:"bytes,1,opt,name=execute,proto3,oneof"`
}

type CommandRequest_Result struct {
	Result *CommandResult `protobuf:"bytes,2,opt,name=result,proto3,oneof"`
}

func (*CommandRequest_Execute) isCommandRequest_Request() {}
func (*CommandRequest_Result) isCommandRequest_Request()  {}

// CommandExecute 命令执行
type CommandExecute struct {
	TaskId         string `protobuf:"bytes,1,opt,name=task_id,json=taskId,proto3" json:"task_id,omitempty"`
	RuleId         string `protobuf:"bytes,2,opt,name=rule_id,json=ruleId,proto3" json:"rule_id,omitempty"`
	HostId         string `protobuf:"bytes,3,opt,name=host_id,json=hostId,proto3" json:"host_id,omitempty"`
	ScriptContent  string `protobuf:"bytes,4,opt,name=script_content,json=scriptContent,proto3" json:"script_content,omitempty"`
	TimeoutSeconds int32  `protobuf:"varint,5,opt,name=timeout_seconds,json=timeoutSeconds,proto3" json:"timeout_seconds,omitempty"`
}

// CommandResult 命令结果
type CommandResult struct {
	TaskId   string `protobuf:"bytes,1,opt,name=task_id,json=taskId,proto3" json:"task_id,omitempty"`
	HostId   string `protobuf:"bytes,2,opt,name=host_id,json=hostId,proto3" json:"host_id,omitempty"`
	ExitCode int32  `protobuf:"varint,3,opt,name=exit_code,json=exitCode,proto3" json:"exit_code,omitempty"`
	Stdout   string `protobuf:"bytes,4,opt,name=stdout,proto3" json:"stdout,omitempty"`
	Stderr   string `protobuf:"bytes,5,opt,name=stderr,proto3" json:"stderr,omitempty"`
	IsFinal  bool   `protobuf:"varint,6,opt,name=is_final,json=isFinal,proto3" json:"is_final,omitempty"`
}

// UnimplementedAgentServiceServer can be embedded to have forward compatible implementations
type UnimplementedAgentServiceServer struct{}

func (*UnimplementedAgentServiceServer) Register(context.Context, *RegisterRequest) (*RegisterResponse, error) {
	return nil, nil
}

func (*UnimplementedAgentServiceServer) Heartbeat(context.Context, *HeartbeatRequest) (*HeartbeatResponse, error) {
	return nil, nil
}

func (*UnimplementedAgentServiceServer) ExecuteCommand(AgentService_ExecuteCommandServer) error {
	return nil
}

// RegisterAgentServiceServer registers the AgentService server with the gRPC server
func RegisterAgentServiceServer(s *grpc.Server, srv AgentServiceServer) {
	s.RegisterService(&AgentService_ServiceDesc, srv)
}

// AgentService_ServiceDesc is the grpc.ServiceDesc for AgentService
var AgentService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "agent_comm.v1.AgentService",
	HandlerType: (*AgentServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Register",
			Handler:    _AgentService_Register_Handler,
		},
		{
			MethodName: "Heartbeat",
			Handler:    _AgentService_Heartbeat_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ExecuteCommand",
			Handler:       _AgentService_ExecuteCommand_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
}

func _AgentService_Register_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServiceServer).Register(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/agent_comm.v1.AgentService/Register"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServiceServer).Register(ctx, req.(*RegisterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AgentService_Heartbeat_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HeartbeatRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServiceServer).Heartbeat(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/agent_comm.v1.AgentService/Heartbeat"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServiceServer).Heartbeat(ctx, req.(*HeartbeatRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AgentService_ExecuteCommand_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(AgentServiceServer).ExecuteCommand(&executeCommandServer{stream})
}

// executeCommandServer wraps grpc.ServerStream to implement AgentService_ExecuteCommandServer
type executeCommandServer struct {
	grpc.ServerStream
}

func (x *executeCommandServer) Send(m *CommandRequest) error {
	return x.ServerStream.SendMsg(m)
}

func (x *executeCommandServer) Recv() (*CommandRequest, error) {
	m := new(CommandRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetExecute returns the Execute field if present
func (x *CommandRequest) GetExecute() *CommandExecute {
	if x, ok := x.Request.(*CommandRequest_Execute); ok {
		return x.Execute
	}
	return nil
}

// GetResult returns the Result field if present
func (x *CommandRequest) GetResult() *CommandResult {
	if x, ok := x.Request.(*CommandRequest_Result); ok {
		return x.Result
	}
	return nil
}
