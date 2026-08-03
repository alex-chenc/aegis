package grpc_server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	grpcServer      *GRPCServer
	hostRepo        *repository.HostRepository
	redisClient     *storage.RedisClient
	apiServerClient pb.APIServerToServerClient
}

func NewAPIServerToServerImpl(grpcServer *GRPCServer, hostRepo *repository.HostRepository, redisClient *storage.RedisClient, apiServerClient pb.APIServerToServerClient) *APIServerToServerImpl {
	return &APIServerToServerImpl{
		grpcServer:      grpcServer,
		hostRepo:        hostRepo,
		redisClient:     redisClient,
		apiServerClient: apiServerClient,
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
		Agents:       agents,
		Total:        total,
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
		}, err
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
		}, err
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

// ExecuteTool synchronously executes a tool on an agent and waits for the result
func (s *APIServerToServerImpl) ExecuteTool(ctx context.Context, req *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error) {
	logger.Info("ExecuteTool request received",
		zap.String("call_id", req.CallId),
		zap.String("host_id", req.HostId),
		zap.String("tool", req.Tool),
	)

	// Convert ToolExecuteRequest to ToolRequest for the internal call
	toolReq := &pb.ToolRequest{
		CallId:     req.CallId,
		HostId:     req.HostId,
		Tool:       req.Tool,
		ParamsJson: req.Arguments,
	}

	// Call the internal ExecuteTool which forwards to the agent
	toolResp, err := s.grpcServer.ExecuteTool(ctx, toolReq)
	if err != nil {
		logger.Error("ExecuteTool failed",
			zap.Error(err),
			zap.String("call_id", req.CallId),
			zap.String("host_id", req.HostId),
		)
		return &pb.ToolExecuteResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Convert ToolResponse to ToolExecuteResponse
	return &pb.ToolExecuteResponse{
		CallId:          toolResp.CallId,
		Success:         toolResp.Success,
		Result:          toolResp.ResultJson,
		Error:           toolResp.Error,
		ExecutionTimeMs: 0, // Internal tool response doesn't have execution time
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
	if req == nil {
		return &pb.ExecuteBlockCommandResponse{Success: false, Error: ErrAgentGuardBlockCommandInvalid.Error()}, nil
	}
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
	if err := ValidateAgentGuardBlockCommand(hostID, blockCmd); err != nil {
		logger.Warn("agent_guard_block_command_rejected",
			zap.String("command_id", req.CommandId),
			zap.String("host_id", req.HostId),
			zap.String("action", req.Action),
			zap.String("error_code", "invalid_agent_guard_block_command"))
		return &pb.ExecuteBlockCommandResponse{
			CommandId: req.CommandId,
			Success:   false,
			Error:     err.Error(),
		}, nil
	}

	// Send block command to agent
	if err := s.grpcServer.SendBlockCommand(hostID, blockCmd); err != nil {
		logger.Warn("agent_guard_block_command_forward_failed",
			zap.String("host_id", req.HostId),
			zap.String("command_id", req.CommandId),
			zap.String("action", req.Action),
			zap.String("error_code", "agent_command_failed"),
		)
		return &pb.ExecuteBlockCommandResponse{
			CommandId: req.CommandId,
			Success:   false,
			Error:     err.Error(),
		}, nil
	}

	logger.Info("agent_guard_block_command_forwarded",
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
			Error:        err.Error(),
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
			Error:        "failed to marshal software list",
		}, nil
	}

	logger.Info("software collected successfully",
		zap.String("host_id", req.HostId),
		zap.Int("count", len(softwareList)),
	)

	return &pb.CollectSoftwareResponse{
		Success:      true,
		SoftwareJson: string(softwareJSON),
		Error:        "",
	}, nil
}

// TODO: V5.8 - Uncomment after regenerating proto code
// CollectHostAssets collects host assets (V5.8)
// func (s *APIServerToServerImpl) CollectHostAssets(ctx context.Context, req *pb.CollectHostAssetsRequest) (*pb.CollectHostAssetsResponse, error) {
// 	if req.HostId == "" {
// 		return &pb.CollectHostAssetsResponse{
// 			Success: false,
// 			Error:   "host_id is required",
// 		}, nil
// 	}
//
// 	// Call the internal method to collect assets from the agent
// 	snapshotJSON, err := s.grpcServer.CollectHostAssets(ctx, req.HostId, req.CollectTypes, false, true, 2000)
// 	if err != nil {
// 		logger.Error("failed to collect host assets",
// 			zap.String("host_id", req.HostId),
// 			zap.Error(err),
// 		)
// 		return &pb.CollectHostAssetsResponse{
// 			Success: false,
// 			Error:   err.Error(),
// 		}, nil
// 	}
//
// 	logger.Info("host assets collected successfully",
// 		zap.String("host_id", req.HostId),
// 	)
//
// 	return &pb.CollectHostAssetsResponse{
// 		Success:     true,
// 		HostId:      req.HostId,
// 		SnapshotJson: snapshotJSON,
// 		CollectedAt: time.Now().Unix(),
// 	}, nil
// }

// InstallDetectionPackage installs a detection package on agents
func (s *APIServerToServerImpl) InstallDetectionPackage(ctx context.Context, req *pb.InstallDetectionPackageRequest) (*pb.InstallDetectionPackageResponse, error) {
	if req.Command == nil {
		return &pb.InstallDetectionPackageResponse{
			Success: false,
			Message: "command is required",
		}, nil
	}

	affected := int32(0)
	cmd := &pb.CommandRequest{
		Request: &pb.CommandRequest_DetectionPackageCommand{
			DetectionPackageCommand: &pb.DetectionPackageCommand{
				Action:       req.Command.Action,
				PackageId:    req.Command.PackageId,
				Version:      req.Command.Version,
				PackageUrl:   req.Command.PackageUrl,
				SignatureUrl: req.Command.SignatureUrl,
				PackageSize:  req.Command.PackageSize,
			},
		},
	}

	if req.HostId == "" {
		s.grpcServer.agentConnections.Range(func(key, value interface{}) bool {
			hostID := key.(uuid.UUID)
			conn := value.(*AgentConnection)
			if conn.Stream != nil {
				if err := conn.Stream.Send(cmd); err != nil {
					logger.Error("failed to send detection package command",
						zap.Stringer("host_id", hostID),
						zap.Error(err),
					)
				} else {
					affected++
				}
			}
			return true
		})
	} else {
		hostID, err := uuid.Parse(req.HostId)
		if err != nil {
			return &pb.InstallDetectionPackageResponse{
				Success: false,
				Message: "invalid host_id",
			}, nil
		}
		value, ok := s.grpcServer.agentConnections.Load(hostID)
		if !ok {
			return &pb.InstallDetectionPackageResponse{
				Success: false,
				Message: "agent not connected",
			}, nil
		}
		conn := value.(*AgentConnection)
		if conn.Stream != nil {
			if err := conn.Stream.Send(cmd); err != nil {
				return &pb.InstallDetectionPackageResponse{
					Success: false,
					Message: err.Error(),
				}, nil
			}
			affected = 1
		}
	}

	logger.Info("detection package install command sent",
		zap.String("package_id", req.Command.PackageId),
		zap.String("version", req.Command.Version),
		zap.Int32("affected_agents", affected),
	)

	return &pb.InstallDetectionPackageResponse{
		Success:        true,
		AffectedAgents: affected,
		Message:        "install command sent",
	}, nil
}

// UninstallDetectionPackage uninstalls a detection package from agents
func (s *APIServerToServerImpl) UninstallDetectionPackage(ctx context.Context, req *pb.UninstallDetectionPackageRequest) (*pb.UninstallDetectionPackageResponse, error) {
	affected := int32(0)
	cmd := &pb.CommandRequest{
		Request: &pb.CommandRequest_DetectionPackageCommand{
			DetectionPackageCommand: &pb.DetectionPackageCommand{
				Action:    "uninstall",
				PackageId: req.PackageId,
				Version:   req.Version,
			},
		},
	}

	if req.HostId == "" {
		s.grpcServer.agentConnections.Range(func(key, value interface{}) bool {
			conn := value.(*AgentConnection)
			if conn.Stream != nil {
				if err := conn.Stream.Send(cmd); err != nil {
					logger.Error("failed to send uninstall command",
						zap.Stringer("host_id", key.(uuid.UUID)),
						zap.Error(err),
					)
				} else {
					affected++
				}
			}
			return true
		})
	} else {
		hostID, err := uuid.Parse(req.HostId)
		if err != nil {
			return &pb.UninstallDetectionPackageResponse{
				Success: false,
				Message: fmt.Sprintf("invalid host_id: %v", err),
			}, nil
		}
		value, ok := s.grpcServer.agentConnections.Load(hostID)
		if !ok {
			return &pb.UninstallDetectionPackageResponse{
				Success: false,
				Message: "agent not connected",
			}, nil
		}
		conn := value.(*AgentConnection)
		if conn.Stream != nil {
			if err := conn.Stream.Send(cmd); err != nil {
				return &pb.UninstallDetectionPackageResponse{
					Success: false,
					Message: err.Error(),
				}, nil
			}
			affected = 1
		}
	}

	return &pb.UninstallDetectionPackageResponse{
		Success:        true,
		AffectedAgents: affected,
		Message:        "uninstall command sent",
	}, nil
}

// SyncAgentConfig syncs configuration to agents
func (s *APIServerToServerImpl) SyncAgentConfig(ctx context.Context, req *pb.SyncAgentConfigRequest) (*pb.SyncAgentConfigResponse, error) {
	if req == nil {
		return &pb.SyncAgentConfigResponse{
			Success: false,
			Message: "config sync request is required",
		}, nil
	}
	affected := int32(0)
	pendingReconnect := false

	if req.HostId == "" {
		for _, cfg := range req.Configs {
			if cfg != nil && cfg.ConfigType == agentGuardBundleConfigType {
				return &pb.SyncAgentConfigResponse{
					Success: false,
					Message: "agent_guard_bundle requires an explicit host_id",
				}, nil
			}
		}
	}

	var targetHostID uuid.UUID
	if req.HostId != "" {
		var err error
		targetHostID, err = uuid.Parse(req.HostId)
		if err != nil {
			return &pb.SyncAgentConfigResponse{
				Success: false,
				Message: fmt.Sprintf("invalid host_id: %v", err),
			}, nil
		}
	}

	for _, cfg := range req.Configs {
		if cfg == nil {
			continue
		}
		if cfg.ConfigType == agentGuardBundleConfigType {
			config := &pb.ConfigSync{
				ConfigType: cfg.ConfigType,
				Action:     "full_sync",
				Payload:    cfg.ConfigJson,
			}
			snapshot, err := s.grpcServer.cacheAgentGuardBundle(targetHostID, config)
			if err != nil {
				logger.Warn("agent_guard_bundle_cache_rejected",
					zap.String("host_id", targetHostID.String()),
					zap.String("error_code", agentGuardBundleErrorCode(err)),
				)
				return &pb.SyncAgentConfigResponse{
					Success: false,
					Message: err.Error(),
				}, nil
			}

			value, connected := s.grpcServer.agentConnections.Load(targetHostID)
			if !connected {
				pendingReconnect = true
				logger.Info("agent_guard_bundle_cached_for_reconnect",
					zap.String("host_id", targetHostID.String()),
					zap.Int64("bundle_version", snapshot.Version),
					zap.String("bundle_digest", snapshot.Digest),
				)
				continue
			}
			connection, ok := value.(*AgentConnection)
			if !ok {
				return &pb.SyncAgentConfigResponse{
					Success: false,
					Message: "invalid agent connection state",
				}, nil
			}
			if err := s.grpcServer.dispatchAgentConfig(ctx, connection, snapshot.Config); err != nil {
				logger.Warn("agent_guard_bundle_dispatch_failed",
					zap.String("host_id", targetHostID.String()),
					zap.Int64("bundle_version", snapshot.Version),
					zap.String("bundle_digest", snapshot.Digest),
					zap.String("error_code", "agent_config_dispatch_failed"),
				)
				return &pb.SyncAgentConfigResponse{
					Success: false,
					Message: err.Error(),
				}, nil
			}
			affected++
			logger.Info("agent_guard_bundle_dispatched",
				zap.String("host_id", targetHostID.String()),
				zap.Int64("bundle_version", snapshot.Version),
				zap.String("bundle_digest", snapshot.Digest),
				zap.String("channel", agentConfigChannel(connection)),
			)
			continue
		}

		var commandReq *pb.CommandRequest

		if cfg.ConfigType == "dynamic_ebpf_hook_allowlist" {
			commandReq = &pb.CommandRequest{
				Request: &pb.CommandRequest_AllowlistUpdate{
					AllowlistUpdate: &pb.AllowlistUpdate{
						Version:       fmt.Sprintf("%d", time.Now().Unix()),
						AllowlistJson: cfg.ConfigJson,
					},
				},
			}
		} else {
			commandReq = &pb.CommandRequest{
				Request: &pb.CommandRequest_ConfigSync{
					ConfigSync: &pb.ConfigSync{
						ConfigType: cfg.ConfigType,
						Action:     "full_sync",
						Payload:    cfg.ConfigJson,
					},
				},
			}
		}

		if req.HostId == "" {
			s.grpcServer.agentConnections.Range(func(key, value interface{}) bool {
				conn := value.(*AgentConnection)
				if conn.Stream != nil {
					if err := conn.Stream.Send(commandReq); err != nil {
						logger.Error("failed to send config sync",
							zap.Stringer("host_id", key.(uuid.UUID)),
							zap.String("config_type", cfg.ConfigType),
							zap.Error(err),
						)
					} else {
						affected++
					}
				}
				return true
			})
		} else {
			value, ok := s.grpcServer.agentConnections.Load(targetHostID)
			if !ok {
				return &pb.SyncAgentConfigResponse{
					Success: false,
					Message: "agent not connected",
				}, nil
			}
			conn := value.(*AgentConnection)
			if conn.Stream != nil {
				if err := conn.Stream.Send(commandReq); err != nil {
					return &pb.SyncAgentConfigResponse{
						Success: false,
						Message: err.Error(),
					}, nil
				}
				affected = 1
			}
		}

		logger.Info("config sync sent",
			zap.String("config_type", cfg.ConfigType),
			zap.Int32("affected_agents", affected),
		)
	}

	message := "config sync sent"
	if pendingReconnect {
		message = "agent guard bundle cached for reconnect"
	}
	return &pb.SyncAgentConfigResponse{
		Success:        true,
		AffectedAgents: affected,
		Message:        message,
	}, nil
}

// ReportDetectionPackageStatus receives package status from agent
func (s *APIServerToServerImpl) ReportDetectionPackageStatus(ctx context.Context, req *pb.ReportDetectionPackageStatusRequest) (*pb.ReportDetectionPackageStatusResponse, error) {
	for _, status := range req.Statuses {
		logger.Info("detection package status reported",
			zap.String("host_id", status.HostId),
			zap.String("package_id", status.PackageId),
			zap.String("version", status.Version),
			zap.String("status", status.Status),
			zap.String("active_artifact", status.ActiveArtifact),
			zap.Strings("loaded_hooks", status.LoadedHooks),
			zap.String("error_message", status.ErrorMessage),
		)

		key := status.HostId + ":" + status.PackageId + ":" + status.Version
		s.grpcServer.detectionPackageStatuses.Store(key, status)
	}

	go s.forwardStatusToAPIServer(req.Statuses)

	return &pb.ReportDetectionPackageStatusResponse{
		Success: true,
		Message: "statuses received",
	}, nil
}

func (s *APIServerToServerImpl) forwardStatusToAPIServer(statuses []*pb.DetectionPackageHostStatus) {
	if s.apiServerClient == nil {
		logger.Warn("api server client not available, skipping status forward")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.ReportDetectionPackageStatusRequest{
		Statuses: statuses,
	}
	_, err := s.apiServerClient.ReportDetectionPackageStatus(ctx, req)
	if err != nil {
		logger.Warn("failed to forward status to API Server via gRPC", zap.Error(err))
	}
}

func (s *APIServerToServerImpl) ReportCorrelationAlert(ctx context.Context, req *pb.ReportCorrelationAlertRequest) (*pb.ReportCorrelationAlertResponse, error) {
	logger.Info("correlation alert reported",
		zap.String("agent_id", req.AgentId),
		zap.String("package_id", req.PackageId),
		zap.String("correlation_rule_id", req.CorrelationRuleId),
		zap.String("severity", req.Severity),
	)

	go s.forwardCorrelationAlertToAPIServer(req)

	return &pb.ReportCorrelationAlertResponse{
		Accepted: true,
	}, nil
}

func (s *APIServerToServerImpl) forwardCorrelationAlertToAPIServer(req *pb.ReportCorrelationAlertRequest) {
	if s.apiServerClient == nil {
		logger.Warn("api server client not available, skipping correlation alert forward")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.apiServerClient.ReportCorrelationAlert(ctx, req)
	if err != nil {
		logger.Warn("failed to forward correlation alert to API Server via gRPC", zap.Error(err))
	}
}
