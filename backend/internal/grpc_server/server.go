package grpc_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/internal/storage"
	pb "aegis-system/pkg/api/v1"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type TaskResultCallback func(taskID uuid.UUID, stdout, stderr string, exitCode int, status string)

type GRPCServer struct {
	pb.UnimplementedAgentServiceServer
	server             *grpc.Server
	hostRepo           *repository.HostRepository
	taskLogRepo        *repository.TaskLogRepository
	redisClient        *storage.RedisClient
	agentConnections   sync.Map
	port               int
	taskResultCallback TaskResultCallback
}

type AgentConnection struct {
	HostID uuid.UUID
	Stream pb.AgentService_ExecuteCommandServer
	Ctx    context.Context
	Cancel context.CancelFunc
	Inbox  chan *pb.CommandExecute
}

func NewGRPCServer(hostRepo *repository.HostRepository, redisClient *storage.RedisClient, port int) *GRPCServer {
	return &GRPCServer{
		hostRepo:    hostRepo,
		redisClient: redisClient,
		port:        port,
	}
}

func (s *GRPCServer) SetTaskLogRepo(taskLogRepo *repository.TaskLogRepository) {
	s.taskLogRepo = taskLogRepo
}

func (s *GRPCServer) SetTaskResultCallback(callback TaskResultCallback) {
	s.taskResultCallback = callback
}

// Start 启动 gRPC 服务器
func (s *GRPCServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		logger.Error("failed to listen",
			zap.Error(err),
			zap.Int("port", s.port),
		)
		return err
	}

	s.server = grpc.NewServer()
	pb.RegisterAgentServiceServer(s.server, s)

	logger.Info("gRPC server starting",
		zap.Int("port", s.port),
	)

	go func() {
		if err := s.server.Serve(lis); err != nil {
			logger.Error("failed to serve gRPC",
				zap.Error(err),
			)
		}
	}()

	return nil
}

// Stop 停止 gRPC 服务器
func (s *GRPCServer) Stop() {
	logger.Info("stopping gRPC server")
	if s.server != nil {
		s.server.GracefulStop()
	}

	// 关闭所有 Agent 连接
	s.agentConnections.Range(func(key, value interface{}) bool {
		conn := value.(*AgentConnection)
		if conn.Cancel != nil {
			conn.Cancel()
		}
		return true
	})
}

// Register 处理 Agent 注册
func (s *GRPCServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	logger.Info("agent register request received",
		zap.String("host_id", req.HostId),
		zap.String("hostname", req.AssetInfo.Hostname),
		zap.String("ip", req.AssetInfo.IpAddress),
	)

	// 验证 Auth Token
	// TODO: 从配置读取并验证

	// 如果 host_id 为空，生成新的
	var hostID uuid.UUID
	var err error

	if req.HostId == "" {
		hostID = uuid.New()
	} else {
		hostID, err = uuid.Parse(req.HostId)
		if err != nil {
			logger.Error("invalid host_id format",
				zap.Error(err),
				zap.String("host_id", req.HostId),
			)
			return &pb.RegisterResponse{
				Success: false,
				Message: "invalid host_id format",
			}, nil
		}
	}

	// 创建或更新主机记录
	host := &model.Host{
		ID:              hostID,
		IPAddress:       req.AssetInfo.IpAddress,
		Hostname:        req.AssetInfo.Hostname,
		OSType:          req.AssetInfo.OsType,
		AgentVersion:    req.AssetInfo.AgentVersion,
		LastHeartbeatAt: time.Now(),
	}

	if err := s.hostRepo.Upsert(host); err != nil {
		logger.Error("failed to upsert host",
			zap.Error(err),
			zap.String("host_id", hostID.String()),
		)
		return &pb.RegisterResponse{
			Success: false,
			Message: fmt.Sprintf("failed to register host: %v", err),
		}, nil
	}

	// 在 Redis 中设置 session
	sessionKey := fmt.Sprintf("agent:session:%s", hostID.String())
	if err := s.redisClient.Client().Set(ctx, sessionKey, "active", 0).Err(); err != nil {
		logger.Error("failed to set agent session",
			zap.Error(err),
			zap.String("host_id", hostID.String()),
		)
	}

	logger.Info("agent registered successfully",
		zap.String("host_id", hostID.String()),
		zap.String("ip", req.AssetInfo.IpAddress),
		zap.String("hostname", req.AssetInfo.Hostname),
	)

	return &pb.RegisterResponse{
		Success: true,
		HostId:  hostID.String(),
		Message: "registration successful",
	}, nil
}

// Heartbeat 处理 Agent 心跳
func (s *GRPCServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		logger.Error("invalid host_id in heartbeat",
			zap.Error(err),
			zap.String("host_id", req.HostId),
		)
		return &pb.HeartbeatResponse{
			Success: false,
			Message: "invalid host_id",
		}, nil
	}

	// 更新 Redis 心跳
	if err := s.redisClient.SetHeartbeat(req.HostId); err != nil {
		logger.Error("failed to set heartbeat in Redis",
			zap.Error(err),
			zap.String("host_id", req.HostId),
		)
		return &pb.HeartbeatResponse{
			Success: false,
			Message: fmt.Sprintf("failed to update heartbeat: %v", err),
		}, nil
	}

	// 异步更新数据库心跳
	go func() {
		if err := s.hostRepo.UpdateHeartbeat(hostID); err != nil {
			logger.Error("failed to update heartbeat in database",
				zap.Error(err),
				zap.String("host_id", req.HostId),
			)
		}
	}()

	logger.Debug("heartbeat received",
		zap.String("host_id", req.HostId),
		zap.Int64("timestamp", req.Timestamp),
	)

	return &pb.HeartbeatResponse{
		Success: true,
		Message: "heartbeat received",
	}, nil
}

// ExecuteCommand 双向流命令执行
func (s *GRPCServer) ExecuteCommand(stream pb.AgentService_ExecuteCommandServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var hostID uuid.UUID
	var connection *AgentConnection

	inbox := make(chan *pb.CommandExecute, 100)

	// 接收消息循环
	for {
		req, err := stream.Recv()
		if err != nil {
			logger.Error("ExecuteCommand stream error",
				zap.Error(err),
				zap.Stringer("host_id", hostID),
			)

			// 清理连接
			if connection != nil {
				s.agentConnections.Delete(hostID)
				if connection.Cancel != nil {
					connection.Cancel()
				}
			}
			return err
		}

		switch r := req.Request.(type) {
		case *pb.CommandRequest_Execute:
			// Agent 发送命令执行请求
			if r.Execute != nil {
				hostID, _ = uuid.Parse(r.Execute.HostId)
				connection = &AgentConnection{
					HostID: hostID,
					Stream: stream,
					Ctx:    ctx,
					Cancel: cancel,
					Inbox:  inbox,
				}

				s.agentConnections.Store(hostID, connection)

				logger.Info("agent connection established",
					zap.Stringer("host_id", hostID),
				)
			}

		case *pb.CommandRequest_Result:
			result := r.Result
			logger.Info("command result received",
				zap.String("task_id", result.TaskId),
				zap.Stringer("host_id", hostID),
				zap.Int32("exit_code", result.ExitCode),
				zap.String("stdout", result.Stdout),
				zap.String("stderr", result.Stderr),
				zap.Bool("is_final", result.IsFinal),
			)

			if callback, ok := collectCallbacks.Load(result.TaskId); ok {
				callback.(func(*pb.CommandResult))(result)
				collectCallbacks.Delete(result.TaskId)
				continue
			}

			if result.IsFinal && s.taskResultCallback != nil {
				taskID, err := uuid.Parse(result.TaskId)
				if err != nil {
					logger.Error("invalid task_id in result",
						zap.String("task_id", result.TaskId),
						zap.Error(err),
					)
					continue
				}

				status := "success"
				// 对于检查任务：脚本正常执行完成时，无论 exit code 是什么，status 都应为 success。
				// exit code 会单独存储，由前端基于 exit code 判断"通过/未通过"。
				// 对于修复任务：exit code = 0 表示修复成功，exit code != 0 表示修复失败（需要触发自愈）。
				// status=failed 仅用于需要触发自愈的场景（检查任务失败、修复任务失败）。
				if s.taskLogRepo != nil {
					taskLog, findErr := s.taskLogRepo.FindByID(taskID)
					if findErr == nil && taskLog.TaskType == "fix" && result.ExitCode != 0 {
						status = "failed"
						logger.Info("fix task failed with non-zero exit code, marking as failed for self-healing",
							zap.String("task_id", result.TaskId),
							zap.Int32("exit_code", result.ExitCode),
						)
					}
				}

				s.taskResultCallback(
					taskID,
					result.Stdout,
					result.Stderr,
					int(result.ExitCode),
					status,
				)
			}
		}
	}
}

// SendCommand 向指定 Agent 发送命令，支持重试等待连接建立
func (s *GRPCServer) SendCommand(hostID uuid.UUID, execute *pb.CommandExecute) error {
	maxRetries := 3
	retryInterval := 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		value, ok := s.agentConnections.Load(hostID)
		if !ok {
			if attempt < maxRetries-1 {
				logger.Debug("agent connection not found, retrying",
					zap.Stringer("host_id", hostID),
					zap.Int("attempt", attempt+1),
					zap.Int("max_retries", maxRetries),
				)
				time.Sleep(retryInterval)
				continue
			}
			logger.Warn("agent connection not found after retries",
				zap.Stringer("host_id", hostID),
				zap.Int("attempts", maxRetries),
			)
			return fmt.Errorf("agent not connected")
		}

		conn := value.(*AgentConnection)

		select {
		case conn.Inbox <- execute:
			err := conn.Stream.Send(&pb.CommandRequest{
				Request: &pb.CommandRequest_Execute{
					Execute: execute,
				},
			})
			if err != nil {
				logger.Error("failed to send command to agent",
					zap.Error(err),
					zap.Stringer("host_id", hostID),
					zap.String("task_id", execute.TaskId),
				)
				return err
			}

			logger.Debug("command sent to agent",
				zap.Stringer("host_id", hostID),
				zap.String("task_id", execute.TaskId),
			)
			return nil

		case <-conn.Ctx.Done():
			return fmt.Errorf("connection closed")
		}
	}

	return fmt.Errorf("agent not connected")
}

// GetConnectedAgents 获取已连接的 Agent 数量
func (s *GRPCServer) GetConnectedAgents() int {
	count := 0
	s.agentConnections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// IsAgentConnected 检查指定 Agent 是否建立了双向流连接
func (s *GRPCServer) IsAgentConnected(hostID uuid.UUID) bool {
	_, ok := s.agentConnections.Load(hostID)
	return ok
}

// GetConnectedHostIDs 获取所有已建立双向流连接的主机ID列表
func (s *GRPCServer) GetConnectedHostIDs() []uuid.UUID {
	var hostIDs []uuid.UUID
	s.agentConnections.Range(func(key, value interface{}) bool {
		if hostID, ok := key.(uuid.UUID); ok {
			hostIDs = append(hostIDs, hostID)
		}
		return true
	})
	return hostIDs
}

// CollectSoftwareList 向 Agent 发送软件清单采集请求并返回结果
func (s *GRPCServer) CollectSoftwareList(ctx context.Context, hostIDStr string) ([]model.SoftwareInfo, error) {
	hostID, err := uuid.Parse(hostIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid host_id: %w", err)
	}

	logger.Info("collecting software list", zap.String("host_id", hostIDStr))

	value, ok := s.agentConnections.Load(hostID)
	if !ok {
		logger.Error("agent not connected", zap.String("host_id", hostIDStr))
		return nil, fmt.Errorf("agent not connected")
	}

	conn := value.(*AgentConnection)
	logger.Info("agent connection found", zap.String("host_id", hostIDStr))

	collectReq := &pb.CommandExecute{
		TaskId:         uuid.New().String(),
		HostId:         hostIDStr,
		ScriptContent:  "#SOFTWARE_COLLECT#",
		TimeoutSeconds: 120,
	}

	logger.Info("sending collect request", zap.String("task_id", collectReq.TaskId))

	responseChan := make(chan *pb.CommandResult, 1)

	s.storeCollectCallback(collectReq.TaskId, func(result *pb.CommandResult) {
		logger.Info("collect callback triggered",
			zap.String("task_id", result.TaskId),
			zap.String("stdout", result.Stdout),
			zap.Int("lines", strings.Count(result.Stdout, "\n")))
		responseChan <- result
	})
	defer s.removeCollectCallback(collectReq.TaskId)

	err = conn.Stream.Send(&pb.CommandRequest{
		Request: &pb.CommandRequest_Execute{
			Execute: collectReq,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send collect request: %w", err)
	}

	select {
	case result := <-responseChan:
		if result.ExitCode != 0 {
			return nil, fmt.Errorf("collect failed: %s", result.Stderr)
		}
		return s.parseSoftwareList(result.Stdout)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(120 * time.Second):
		return nil, fmt.Errorf("collect timeout")
	}
}

var collectCallbacks sync.Map

func (s *GRPCServer) storeCollectCallback(key string, callback func(*pb.CommandResult)) {
	collectCallbacks.Store(key, callback)
}

func (s *GRPCServer) removeCollectCallback(key string) {
	collectCallbacks.Delete(key)
}

func (s *GRPCServer) parseSoftwareList(output string) ([]model.SoftwareInfo, error) {
	var software []model.SoftwareInfo

	lines := strings.Split(output, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var sw model.SoftwareInfo
		if err := json.Unmarshal([]byte(line), &sw); err != nil {
			logger.Warn("failed to parse software JSON",
				zap.Int("line", i+1),
				zap.String("content", line),
				zap.Error(err))
			continue
		}

		if sw.Name != "" {
			software = append(software, sw)
		}
	}

	logger.Info("parsed software list",
		zap.Int("total_lines", len(lines)),
		zap.Int("parsed_count", len(software)))

	return software, nil
}
