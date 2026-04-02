package grpc_server

import (
	"context"
	"encoding/json"
	"fmt"

	"server/internal/repository"
	"server/internal/storage"
	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// APIServerToServerImpl implements the APIServerToServer gRPC service
// This service is used by the API Server to query agent status and forward commands
type APIServerToServerImpl struct {
	pb.UnimplementedAPIServerToServerServer
	grpcServer  *GRPCServer
	hostRepo    *repository.HostRepository
	redisClient *storage.RedisClient
}

// NewAPIServerToServerImpl creates a new APIServerToServer implementation
func NewAPIServerToServerImpl(grpcServer *GRPCServer, hostRepo *repository.HostRepository, redisClient *storage.RedisClient) *APIServerToServerImpl {
	return &APIServerToServerImpl{
		grpcServer:  grpcServer,
		hostRepo:    hostRepo,
		redisClient: redisClient,
	}
}

// GetAgentStatus gets the status of a specific agent
func (s *APIServerToServerImpl) GetAgentStatus(ctx context.Context, req *pb.GetAgentStatusRequest) (*pb.GetAgentStatusResponse, error) {
	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		return nil, fmt.Errorf("invalid host_id: %w", err)
	}

	// Get host info from database
	host, err := s.hostRepo.FindByID(hostID)
	if err != nil {
		logger.Error("failed to get host", zap.Error(err), zap.String("host_id", req.HostId))
		return &pb.GetAgentStatusResponse{
			Connected: false,
		}, nil
	}

	// Check if agent is connected (has active gRPC stream)
	connected := s.grpcServer.IsAgentConnected(hostID)

	// Get heartbeat time from Redis
	heartbeatTime, _ := s.redisClient.GetHeartbeatTime(req.HostId)

	response := &pb.GetAgentStatusResponse{
		Connected:       connected,
		HostId:          req.HostId,
		Hostname:        host.Hostname,
		IpAddress:       host.IPAddress,
		OsType:          host.OSType,
		AgentVersion:    host.AgentVersion,
		LastHeartbeat:   heartbeatTime,
		PendingCommands: 0, // TODO: implement pending command count
	}

	logger.Debug("GetAgentStatus",
		zap.String("host_id", req.HostId),
		zap.Bool("connected", connected),
		zap.Int64("heartbeat", heartbeatTime),
	)

	return response, nil
}

// ListConnectedAgents lists all connected agents
func (s *APIServerToServerImpl) ListConnectedAgents(ctx context.Context, req *pb.ListConnectedAgentsRequest) (*pb.ListConnectedAgentsResponse, error) {
	// Get all hosts from database (page 1, large page size to get all)
	hosts, err := s.hostRepo.FindAll(1, 1000, "")
	if err != nil {
		logger.Error("failed to list hosts", zap.Error(err))
		return nil, err
	}

	// Get online status from Redis
	hostIDs := make([]string, len(hosts))
	for i, h := range hosts {
		hostIDs[i] = h.ID.String()
	}

	onlineMap, err := s.redisClient.BatchCheckOnline(hostIDs)
	if err != nil {
		logger.Error("failed to batch check online status", zap.Error(err))
		onlineMap = make(map[string]bool)
	}

	agents := make([]*pb.AgentInfo, 0)
	onlineCount := 0
	offlineCount := 0

	for _, host := range hosts {
		isOnline := onlineMap[host.ID.String()]
		connected := s.grpcServer.IsAgentConnected(host.ID)

		if isOnline {
			onlineCount++
		} else {
			offlineCount++
		}

		// Apply status filter if specified
		if req.StatusFilter == "online" && !isOnline {
			continue
		}
		if req.StatusFilter == "offline" && isOnline {
			continue
		}

		heartbeatTime, _ := s.redisClient.GetHeartbeatTime(host.ID.String())

		agent := &pb.AgentInfo{
			HostId:        host.ID.String(),
			Hostname:      host.Hostname,
			IpAddress:     host.IPAddress,
			OsType:        host.OSType,
			AgentVersion:  host.AgentVersion,
			Connected:     connected,
			LastHeartbeat: heartbeatTime,
		}
		agents = append(agents, agent)
	}

	// Apply pagination
	total := int32(len(agents))
	page := int(req.Page)
	pageSize := int(req.PageSize)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > int(total) {
		start = int(total)
	}
	if end > int(total) {
		end = int(total)
	}
	agents = agents[start:end]

	return &pb.ListConnectedAgentsResponse{
		Agents:      agents,
		Total:       total,
		OnlineCount:  int32(onlineCount),
		OfflineCount: int32(offlineCount),
	}, nil
}

// HealthCheck performs a health check
func (s *APIServerToServerImpl) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "running",
		Message: "server is healthy",
		Details: map[string]string{
			"grpc_port":    fmt.Sprintf("%d", s.grpcServer.GetPort()),
			"agents_count": fmt.Sprintf("%d", s.grpcServer.GetConnectedAgents()),
		},
	}, nil
}

// ForwardCommand forwards a command to an agent via the Server service
func (s *APIServerToServerImpl) ForwardCommand(ctx context.Context, req *pb.ForwardCommandRequest) (*pb.ForwardCommandResponse, error) {
	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		return &pb.ForwardCommandResponse{
			Success: false,
			Message: fmt.Sprintf("invalid host_id: %v", err),
		}, nil
	}

	// Create command to send to agent
	cmd := &pb.CommandExecute{
		TaskId:         req.TaskId,
		ScriptContent:  req.ScriptContent,
		TimeoutSeconds: req.TimeoutSeconds,
	}

	// Send command through the server's command pipeline
	if err := s.grpcServer.SendCommand(hostID, cmd); err != nil {
		logger.Error("failed to forward command",
			zap.Error(err),
			zap.String("host_id", req.HostId),
			zap.String("task_id", req.TaskId),
		)
		return &pb.ForwardCommandResponse{
			Success: false,
			Message: fmt.Sprintf("failed to send command: %v", err),
			TaskId:  req.TaskId,
		}, nil
	}

	logger.Info("command forwarded",
		zap.String("host_id", req.HostId),
		zap.String("task_id", req.TaskId),
	)

	return &pb.ForwardCommandResponse{
		Success: true,
		Message: "command forwarded successfully",
		TaskId:  req.TaskId,
	}, nil
}

// UpdateAgentRules updates the Sigma rules on agents
func (s *APIServerToServerImpl) UpdateAgentRules(ctx context.Context, req *pb.UpdateAgentRulesRequest) (*pb.UpdateAgentRulesResponse, error) {
	if len(req.Rules) == 0 {
		return &pb.UpdateAgentRulesResponse{
			Success:        true,
			AffectedAgents: 0,
			Message:        "no rules to update",
		}, nil
	}

	logger.Info("UpdateAgentRules called",
		zap.String("host_id", req.HostId),
		zap.String("action", req.Action),
		zap.Int("rule_count", len(req.Rules)),
	)

	// Convert AgentRuleUpdate to RuleUpdate
	ruleUpdates := make([]*pb.RuleUpdate, len(req.Rules))
	for i, r := range req.Rules {
		ruleUpdates[i] = &pb.RuleUpdate{
			RuleId:  r.RuleId,
			Action:  r.Action,
			Content: r.Content,
		}
	}

	// Create RuleUpdateRequest
	ruleUpdateReq := &pb.RuleUpdateRequest{
		Action: req.Action,
		Rules:  ruleUpdates,
	}

	var affectedAgents int32

	if req.HostId == "" {
		// Broadcast to all connected agents
		s.grpcServer.agentConnections.Range(func(key, value interface{}) bool {
			conn, ok := value.(*AgentConnection)
			if !ok || conn.Stream == nil {
				return true
			}

			err := conn.Stream.Send(&pb.CommandRequest{
				Request: &pb.CommandRequest_RuleUpdate{
					RuleUpdate: ruleUpdateReq,
				},
			})
			if err != nil {
				logger.Warn("failed to send rule update to agent",
					zap.String("host_id", key.(uuid.UUID).String()),
					zap.Error(err))
			} else {
				logger.Info("rule update sent to agent",
					zap.String("host_id", key.(uuid.UUID).String()),
					zap.Int("rule_count", len(ruleUpdates)))
				affectedAgents++
			}
			return true
		})
	} else {
		// Send to specific agent
		hostID, err := uuid.Parse(req.HostId)
		if err != nil {
			return &pb.UpdateAgentRulesResponse{
				Success: false,
				Message: fmt.Sprintf("invalid host_id: %v", err),
			}, nil
		}

		conn, ok := s.grpcServer.agentConnections.Load(hostID)
		if !ok {
			return &pb.UpdateAgentRulesResponse{
				Success: false,
				Message: "agent not connected",
			}, nil
		}

		agentConn := conn.(*AgentConnection)
		err = agentConn.Stream.Send(&pb.CommandRequest{
			Request: &pb.CommandRequest_RuleUpdate{
				RuleUpdate: ruleUpdateReq,
			},
		})
		if err != nil {
			logger.Error("failed to send rule update to agent",
				zap.String("host_id", req.HostId),
				zap.Error(err))
			return &pb.UpdateAgentRulesResponse{
				Success: false,
				Message: fmt.Sprintf("failed to send: %v", err),
			}, nil
		}
		affectedAgents = 1
		logger.Info("rule update sent to specific agent",
			zap.String("host_id", req.HostId),
			zap.Int("rule_count", len(ruleUpdates)))
	}

	return &pb.UpdateAgentRulesResponse{
		Success:        true,
		AffectedAgents: affectedAgents,
		Message:        fmt.Sprintf("updated %d agents with %d rules", affectedAgents, len(ruleUpdates)),
	}, nil
}

// ExecuteBlockCommand executes a block command on an agent
func (s *APIServerToServerImpl) ExecuteBlockCommand(ctx context.Context, req *pb.ExecuteBlockCommandRequest) (*pb.ExecuteBlockCommandResponse, error) {
	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		return &pb.ExecuteBlockCommandResponse{
			CommandId: req.CommandId,
			Success:   false,
			Error:     fmt.Sprintf("invalid host_id: %v", err),
		}, nil
	}

	// Create block command
	blockCmd := &pb.BlockCommand{
		CommandId: req.CommandId,
		HostId:    req.HostId,
		Action:    req.Action,
		Target:    req.Target,
		Reason:    req.Reason,
	}

	// Send block command to agent
	if err := s.grpcServer.SendBlockCommand(hostID, blockCmd); err != nil {
		logger.Error("failed to execute block command",
			zap.Error(err),
			zap.String("host_id", req.HostId),
			zap.String("command_id", req.CommandId),
		)
		return &pb.ExecuteBlockCommandResponse{
			CommandId: req.CommandId,
			Success:   false,
			Error:     fmt.Sprintf("failed to send block command: %v", err),
		}, nil
	}

	logger.Info("block command executed",
		zap.String("host_id", req.HostId),
		zap.String("command_id", req.CommandId),
		zap.String("action", req.Action),
	)

	return &pb.ExecuteBlockCommandResponse{
		CommandId:  req.CommandId,
		Success:    true,
		ExecutedAt: 0, // TODO: implement proper timestamp
	}, nil
}

// CollectSoftware collects software list from an agent via the Server service
func (s *APIServerToServerImpl) CollectSoftware(ctx context.Context, req *pb.CollectSoftwareRequest) (*pb.CollectSoftwareResponse, error) {
	if req.HostId == "" {
		return &pb.CollectSoftwareResponse{
			Success: false,
			Error:   "host_id is required",
		}, nil
	}

	// Call the internal method to collect software from the agent
	softwareList, err := s.grpcServer.CollectSoftwareListForHost(ctx, req.HostId)
	if err != nil {
		logger.Error("failed to collect software",
			zap.String("host_id", req.HostId),
			zap.Error(err),
		)
		return &pb.CollectSoftwareResponse{
			Success:      false,
			SoftwareJson: "[]",
			Error:       err.Error(),
		}, nil
	}

	// Convert to JSON
	softwareJSON, err := json.Marshal(softwareList)
	if err != nil {
		logger.Error("failed to marshal software list",
			zap.String("host_id", req.HostId),
			zap.Error(err),
		)
		return &pb.CollectSoftwareResponse{
			Success:      false,
			SoftwareJson: "[]",
			Error:       "failed to marshal software list",
		}, nil
	}

	logger.Info("software collected successfully",
		zap.String("host_id", req.HostId),
		zap.Int("count", len(softwareList)),
	)

	return &pb.CollectSoftwareResponse{
		Success:      true,
		SoftwareJson: string(softwareJSON),
		Error:       "",
	}, nil
}