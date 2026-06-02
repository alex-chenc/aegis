package grpc_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"server/internal/model"
	"server/internal/queue"
	"server/internal/repository"
	"server/internal/storage"
	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

type TaskResultCallback func(taskID uuid.UUID, stdout, stderr string, exitCode int, status string)

type WebSocketBroadcaster interface {
	BroadcastAlert(alert *model.Alert)
}

type GRPCServer struct {
	pb.UnimplementedAgentServiceServer
	server               *grpc.Server
	hostRepo             *repository.HostRepository
	taskLogRepo          *repository.TaskLogRepository
	sigmaRuleRepo        *repository.SigmaRuleRepository
	alertRepo            *repository.AlertRepository
	runtimeEventRepo     *repository.RuntimeEventRepository
	blockPolicyRepo      *repository.BlockPolicyRepository
	commandAuditRuleRepo *repository.CommandAuditRuleRepo
	systemConfigRepo     *repository.SystemConfigRepo
	detectionPackageRepo *repository.DetectionPackageRepository
	wsBroadcaster        WebSocketBroadcaster
	redisClient          *storage.RedisClient
	kafkaProducer        *queue.KafkaProducer
	agentConnections     sync.Map
	callbackPorts        sync.Map // hostID -> callback port
	port                 int
	taskResultCallback   TaskResultCallback

	// V5.8: Detection package status storage
	detectionPackageStatuses sync.Map // key: "hostID:packageID:version" -> *pb.DetectionPackageHostStatus
}

type AgentConnection struct {
	HostID         uuid.UUID
	Stream         pb.AgentService_ExecuteCommandServer
	Client         pb.AgentServiceClient // nil - not used for callback
	CallbackClient pb.AgentServiceClient // gRPC client to agent's callback server
	CallbackConn   *grpc.ClientConn      // the underlying connection (must close on reconnect)
	Ctx            context.Context
	Cancel         context.CancelFunc
	Inbox          chan *pb.CommandExecute
}

func NewGRPCServer(hostRepo *repository.HostRepository, redisClient *storage.RedisClient, kafkaProducer *queue.KafkaProducer, port int) *GRPCServer {
	return &GRPCServer{
		hostRepo:      hostRepo,
		redisClient:   redisClient,
		kafkaProducer: kafkaProducer,
		port:          port,
	}
}

func (s *GRPCServer) SetTaskLogRepo(taskLogRepo *repository.TaskLogRepository) {
	s.taskLogRepo = taskLogRepo
}

func (s *GRPCServer) SetSigmaRuleRepo(repo *repository.SigmaRuleRepository) {
	s.sigmaRuleRepo = repo
}

func (s *GRPCServer) SetAlertRepo(alertRepo *repository.AlertRepository, wsBroadcaster WebSocketBroadcaster) {
	s.alertRepo = alertRepo
	s.wsBroadcaster = wsBroadcaster
}

func (s *GRPCServer) SetTaskResultCallback(callback TaskResultCallback) {
	s.taskResultCallback = callback
}

func (s *GRPCServer) SetRuntimeEventRepo(repo *repository.RuntimeEventRepository) {
	s.runtimeEventRepo = repo
}

func (s *GRPCServer) SetBlockPolicyRepo(repo *repository.BlockPolicyRepository) {
	s.blockPolicyRepo = repo
}

func (s *GRPCServer) SetCommandAuditRuleRepo(repo *repository.CommandAuditRuleRepo) {
	s.commandAuditRuleRepo = repo
}

func (s *GRPCServer) SetSystemConfigRepo(repo *repository.SystemConfigRepo) {
	s.systemConfigRepo = repo
}

func (s *GRPCServer) SetDetectionPackageRepo(repo *repository.DetectionPackageRepository) {
	s.detectionPackageRepo = repo
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

	var hostID uuid.UUID
	var err error

	existingHost, err := s.hostRepo.FindByIP(req.AssetInfo.IpAddress)
	if err == nil && existingHost != nil {
		hostID = existingHost.ID
		logger.Info("found existing host by IP, using existing ID",
			zap.String("host_id", hostID.String()),
			zap.String("ip", req.AssetInfo.IpAddress),
		)
	} else {
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
		zap.Int32("callback_port", req.CallbackPort),
	)

	// Store callback port for this agent
	if req.CallbackPort > 0 {
		s.callbackPorts.Store(hostID.String(), int(req.CallbackPort))
		logger.Info("agent callback port stored",
			zap.String("host_id", hostID.String()),
			zap.Int32("callback_port", req.CallbackPort),
		)
	}

	go s.pushConfigToAgent(hostID)

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
				// Close callback connection to agent to prevent leaks
				if connection.CallbackConn != nil {
					connection.CallbackConn.Close()
				}
			}
			return err
		}

		switch r := req.Request.(type) {
		case *pb.CommandRequest_Execute:
			// Agent 发送命令执行请求
			if r.Execute != nil {
				hostID, _ = uuid.Parse(r.Execute.HostId)

				// Look up callback port and create callback client to agent
				var callbackClient pb.AgentServiceClient
				var callbackConn *grpc.ClientConn
				if cbPort, ok := s.callbackPorts.Load(hostID.String()); ok {
					agentIP := ""
					if host, err := s.hostRepo.FindByID(hostID); err == nil && host != nil {
						agentIP = host.IPAddress
					}
					if agentIP == "" {
						agentIP = "127.0.0.1" // fallback
					}
					callbackAddr := fmt.Sprintf("%s:%d", agentIP, cbPort.(int))
					var err error
					callbackConn, err = grpc.NewClient(callbackAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						logger.Error("failed to create callback connection to agent",
							zap.Stringer("host_id", hostID),
							zap.String("addr", callbackAddr),
							zap.Error(err),
						)
					} else {
						callbackClient = pb.NewAgentServiceClient(callbackConn)
						logger.Info("agent callback client created",
							zap.Stringer("host_id", hostID),
							zap.String("addr", callbackAddr),
						)
					}
				}

				connection = &AgentConnection{
					HostID:         hostID,
					Stream:         stream,
					Client:         nil,
					CallbackClient: callbackClient,
					CallbackConn:   callbackConn,
					Ctx:            ctx,
					Cancel:         cancel,
					Inbox:          inbox,
				}

				s.agentConnections.Store(hostID, connection)

				logger.Info("agent connection established",
					zap.Stringer("host_id", hostID),
				)

				go s.pushAgentStartupState(hostID, connection)
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

			if result.IsFinal {
				taskID, err := uuid.Parse(result.TaskId)
				if err != nil {
					logger.Error("invalid task_id in result",
						zap.String("task_id", result.TaskId),
						zap.Error(err),
					)
					continue
				}

				status := "SUCCESS"
				// 对于检查任务：脚本正常执行完成时，无论 exit code 是什么，status 都应为 success。
				// exit code 会单独存储，由前端基于 exit code 判断"通过/未通过"。
				// 对于修复任务、POC验证任务：exit code = 0 表示成功，exit code != 0 表示失败（需要触发自愈）。
				// status=failed 仅用于需要触发自愈的场景（修复任务失败、POC验证失败）。
				if s.taskLogRepo != nil {
					taskLog, findErr := s.taskLogRepo.FindByID(taskID)
					if findErr == nil {
						taskType := strings.ToUpper(taskLog.TaskType)
						if (taskType == "FIX" || taskType == "VULNERABILITY_FIX" || taskType == "POC_VERIFY") && result.ExitCode != 0 {
							status = "FAILED"
							logger.Info("task failed with non-zero exit code, marking as failed for self-healing",
								zap.String("task_id", result.TaskId),
								zap.String("task_type", taskType),
								zap.Int32("exit_code", result.ExitCode),
							)
						}

						// Save result to database
						stdout := result.Stdout
						stderr := result.Stderr
						exitCode := int(result.ExitCode)
						finishedAt := time.Now()
						if err := s.taskLogRepo.UpdateResult(taskID, &stdout, &stderr, &exitCode, status, finishedAt); err != nil {
							logger.Error("failed to update task result",
								zap.String("task_id", result.TaskId),
								zap.Error(err),
							)
						} else {
							logger.Info("task result saved",
								zap.String("task_id", result.TaskId),
								zap.String("status", status),
								zap.Int32("exit_code", result.ExitCode),
							)
						}
					}
				}

				// Also call callback if set (for real-time notifications)
				if s.taskResultCallback != nil {
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
}

func (s *GRPCServer) ReportEvent(ctx context.Context, req *pb.ReportEventRequest) (*pb.ReportEventResponse, error) {
	logger.Info("events received",
		zap.String("host_id", req.HostId),
		zap.Int("event_count", len(req.Events)),
	)

	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		logger.Error("invalid host_id in ReportEvent", zap.String("host_id", req.HostId), zap.Error(err))
		return &pb.ReportEventResponse{Success: false, ReceivedCount: 0}, nil
	}
	if err := s.ensureHostRecordForEvent(ctx, hostID); err != nil {
		logger.Error("failed to ensure host record for events",
			zap.String("host_id", req.HostId),
			zap.Error(err))
		return &pb.ReportEventResponse{Success: false, ReceivedCount: 0}, nil
	}

	for _, event := range req.Events {
		if s.kafkaProducer != nil {
			if err := s.kafkaProducer.SendRawEvent(ctx, req.HostId, event); err != nil {
				logger.Error("failed to send event to kafka",
					zap.String("event_id", event.EventId),
					zap.Error(err),
				)
			}
		}

		if s.runtimeEventRepo != nil {
			// Use event_data_json from agent if available, otherwise build legacy event data
			var eventDataStr string
			if event.EventDataJson != "" {
				eventDataStr = event.EventDataJson
			} else {
				eventData := map[string]interface{}{
					"process_name": event.ProcessName,
					"file_path":    event.FilePath,
					"remote_addr":  event.RemoteAddr,
					"process_tree": event.ProcessTree,
				}
				eventDataJSON, err := json.Marshal(eventData)
				if err != nil {
					logger.Error("failed to marshal event data",
						zap.String("event_id", event.EventId),
						zap.Error(err),
					)
					eventDataJSON = []byte("{}")
				}
				eventDataStr = string(eventDataJSON)
			}
			eventID := event.EventId
			if eventID == "" {
				eventID = "EVT-" + uuid.New().String()[:8]
			}
			runtimeEvent := &model.RuntimeEvent{
				EventID:       eventID,
				HostID:        hostID,
				EventType:     event.EventType,
				EventData:     eventDataStr,
				MatchedRuleID: event.MatchedRuleId,
				MitreID:       event.MitreId,
				Severity:      event.Severity,
				PID:           int(event.Pid),
				CommandLine:   event.CommandLine,
				ProcessName:   event.ProcessName,
				Timestamp:     event.Timestamp,
				Aggregated:    false,
			}
			if err := s.runtimeEventRepo.Create(runtimeEvent); err != nil {
				logger.Error("failed to persist runtime event",
					zap.String("event_id", event.EventId),
					zap.Error(err),
				)
			}
		}

		if s.alertRepo != nil && event.MatchedRuleId != "" {
			s.createAlertFromEvent(req.HostId, event)
		}
	}

	return &pb.ReportEventResponse{
		Success:       true,
		ReceivedCount: int32(len(req.Events)),
	}, nil
}

func (s *GRPCServer) createAlertFromEvent(hostIDStr string, event *pb.RuntimeEvent) {
	hostID, err := uuid.Parse(hostIDStr)
	if err != nil {
		logger.Error("invalid host_id", zap.String("host_id", hostIDStr), zap.Error(err))
		return
	}

	// Check if the rule is disabled - skip alert creation if so
	// Also retain rule reference for MitreID/RuleTitle fallback
	var matchedRule *model.SigmaRule
	if s.sigmaRuleRepo != nil && event.MatchedRuleId != "" {
		rule, err := s.sigmaRuleRepo.FindByRuleID(event.MatchedRuleId)
		if err == nil && rule != nil {
			if rule.Status == "disabled" {
				logger.Debug("rule is disabled, skipping alert creation",
					zap.String("rule_id", event.MatchedRuleId),
					zap.String("mitre_id", event.MitreId))
				return
			}
			matchedRule = rule
		}
	}

	dedupeKey := fmt.Sprintf("%s:%d:%s", hostIDStr, event.Pid, event.MatchedRuleId)

	existing, err := s.alertRepo.FindByDedupeKey(dedupeKey)
	if err == nil && existing != nil {
		existing.HitCount++
		existing.LastSeenAt = time.Now()
		if event.ProcessTree != "" {
			existing.ProcessTree = event.ProcessTree
		}
		if err := s.alertRepo.Update(existing); err != nil {
			logger.Error("failed to update alert", zap.Error(err))
		}
		if s.wsBroadcaster != nil {
			s.wsBroadcaster.BroadcastAlert(existing)
		}
		return
	}

	processTree := normalizeJSONBText(event.ProcessTree)

	// Fallback to sigma_rules table when event fields are empty
	mitreID := strings.ToUpper(event.MitreId)
	ruleTitle := event.MatchedRuleTitle
	if matchedRule != nil {
		if mitreID == "" && matchedRule.MitreID != "" {
			mitreID = strings.ToUpper(matchedRule.MitreID)
		}
		if ruleTitle == "" && matchedRule.Title != "" {
			ruleTitle = matchedRule.Title
		}
	}

	alert := &model.Alert{
		AlertID:        "ALT-" + uuid.New().String()[:8],
		HostID:         hostID,
		PID:            int(event.Pid),
		PPID:           int(event.Ppid),
		CommandLine:    event.CommandLine,
		ProcessTree:    processTree,
		MitreID:        mitreID,
		Severity:       event.Severity,
		DedupeKey:      dedupeKey,
		HitCount:       1,
		Status:         "pending",
		JudgmentSource: "system",
		RuleID:         event.MatchedRuleId,
		RuleTitle:      ruleTitle,
	}

	mitreName, mitreDesc := model.GetMITREChineseDescription(mitreID)
	if mitreName != "" {
		alert.MitreName = mitreName
		alert.Description = mitreDesc
	}

	if err := s.alertRepo.Create(alert); err != nil {
		logger.Error("failed to create alert", zap.Error(err))
		return
	}

	logger.Info("alert created from event",
		zap.String("alert_id", alert.AlertID),
		zap.String("mitre_id", event.MitreId),
		zap.Int("pid", int(event.Pid)),
		zap.Bool("has_process_tree", event.ProcessTree != ""))

	s.checkAutoActions(alert)

	if s.wsBroadcaster != nil {
		s.wsBroadcaster.BroadcastAlert(alert)
	}
}

func (s *GRPCServer) ensureHostRecordForEvent(ctx context.Context, hostID uuid.UUID) error {
	if s.hostRepo == nil {
		return nil
	}
	if _, err := s.hostRepo.FindByID(hostID); err == nil {
		return nil
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	host := &model.Host{
		ID:              hostID,
		IPAddress:       "event-only-" + hostID.String(),
		Hostname:        "agent-" + hostID.String()[:8],
		OSType:          "unknown",
		AgentVersion:    "unknown",
		LastHeartbeatAt: time.Now(),
	}
	if err := s.hostRepo.Upsert(host); err != nil {
		return err
	}
	if s.redisClient != nil {
		sessionKey := fmt.Sprintf("agent:session:%s", hostID.String())
		_ = s.redisClient.Client().Set(ctx, sessionKey, "active", 0).Err()
	}
	return nil
}

func normalizeJSONBText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return "{}"
	}
	return value
}

func (s *GRPCServer) checkAutoActions(alert *model.Alert) {
	if s.blockPolicyRepo == nil {
		return
	}

	policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
	if err != nil || !policy.Enabled {
		return
	}

	if policy.AutoBlock {
		logger.Info("auto-blocking alert",
			zap.String("alert_id", alert.AlertID),
			zap.String("mitre_id", alert.MitreID))
		target, targetErr := blockTargetForAlert(alert, policy.Action)
		alert.AutoBlocked = true
		blockStatus := "blocking"
		alert.BlockStatus = &blockStatus
		if err := s.alertRepo.Update(alert); err != nil {
			logger.Error("failed to update alert for auto-block",
				zap.String("alert_id", alert.AlertID), zap.Error(err))
		}
		if targetErr != nil {
			if err := s.alertRepo.UpdateBlockStatus(alert.AlertID, "failed", targetErr.Error()); err != nil {
				logger.Error("failed to update alert block target error",
					zap.String("alert_id", alert.AlertID), zap.Error(err))
			}
		} else {
			cmd := &pb.BlockCommand{
				CommandId: "BLK-" + uuid.New().String()[:8],
				HostId:    alert.HostID.String(),
				Action:    policy.Action,
				Target:    target,
				Reason:    "auto block",
			}
			if err := s.SendBlockCommand(alert.HostID, cmd); err != nil {
				logger.Error("failed to send auto-block command",
					zap.String("alert_id", alert.AlertID),
					zap.String("host_id", alert.HostID.String()),
					zap.String("action", policy.Action),
					zap.Error(err))
				failedStatus := "failed"
				alert.BlockStatus = &failedStatus
				alert.BlockMessage = err.Error()
				if updateErr := s.alertRepo.UpdateBlockStatus(alert.AlertID, "failed", err.Error()); updateErr != nil {
					logger.Error("failed to update alert block send error",
						zap.String("alert_id", alert.AlertID), zap.Error(updateErr))
				}
			} else {
				logger.Info("auto-block command executed successfully",
					zap.String("alert_id", alert.AlertID),
					zap.String("host_id", alert.HostID.String()),
					zap.String("action", policy.Action))
				successStatus := "success"
				alert.BlockStatus = &successStatus
				alert.BlockMessage = "自动阻断执行成功"
				alert.Status = "resolved"
				if updateErr := s.alertRepo.UpdateBlockStatus(alert.AlertID, "success", "自动阻断执行成功"); updateErr != nil {
					logger.Error("failed to update alert block success status",
						zap.String("alert_id", alert.AlertID), zap.Error(updateErr))
				}
			}
		}
		s.broadcastPolicyUpdate(policy)
	}

	if policy.AutoDispose {
		logger.Info("auto-disposing alert",
			zap.String("alert_id", alert.AlertID),
			zap.String("mitre_id", alert.MitreID))
		alert.AutoDispose = true
		alert.Status = "resolved"
		if err := s.alertRepo.Update(alert); err != nil {
			logger.Error("failed to update alert for auto-dispose",
				zap.String("alert_id", alert.AlertID), zap.Error(err))
		}
		s.broadcastPolicyUpdate(policy)
	}
}

func (s *GRPCServer) broadcastPolicyUpdate(policy *model.BlockPolicy) {
	if s.wsBroadcaster != nil {
		if b, ok := s.wsBroadcaster.(interface{ BroadcastPolicyUpdate(*model.BlockPolicy) }); ok {
			b.BroadcastPolicyUpdate(policy)
		}
	}
}

func (s *GRPCServer) ExecuteTool(ctx context.Context, req *pb.ToolRequest) (*pb.ToolResponse, error) {
	logger.Info("tool call received",
		zap.String("call_id", req.CallId),
		zap.String("host_id", req.HostId),
		zap.String("tool", req.Tool),
	)

	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		return &pb.ToolResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   "invalid host id",
		}, nil
	}

	conn, ok := s.agentConnections.Load(hostID)
	if !ok {
		return &pb.ToolResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   "agent not connected",
		}, nil
	}

	agentConn := conn.(*AgentConnection)
	if agentConn.CallbackClient == nil {
		return &pb.ToolResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   "agent callback client not available (not registered with callback port)",
		}, nil
	}

	resp, callErr := agentConn.CallbackClient.ExecuteTool(ctx, req)
	if callErr != nil {
		return &pb.ToolResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   callErr.Error(),
		}, nil
	}

	return resp, nil
}

func (s *GRPCServer) UpdateRules(ctx context.Context, req *pb.RuleUpdateRequest) (*pb.RuleUpdateResponse, error) {
	logger.Info("rules update request",
		zap.String("action", req.Action),
		zap.Int("rule_count", len(req.Rules)))

	if req.Action == "full_sync" && len(req.Rules) == 0 {
		logger.Info("processing full_sync request")
		if s.sigmaRuleRepo == nil {
			logger.Warn("sigmaRuleRepo is nil, cannot return rules")
			return &pb.RuleUpdateResponse{Success: false, LoadedCount: 0}, nil
		}

		rules, err := s.sigmaRuleRepo.GetActiveAndExperimental()
		logger.Info("querying active rules from database")
		if err != nil {
			logger.Error("failed to get rules for sync", zap.Error(err))
			return &pb.RuleUpdateResponse{Success: false, LoadedCount: 0}, nil
		}

		updates := make([]*pb.RuleUpdate, 0, len(rules))
		for _, rule := range rules {
			updates = append(updates, &pb.RuleUpdate{
				RuleId:  rule.RuleID,
				Action:  "add",
				Content: rule.Content,
			})
		}

		logger.Info("returning rules for sync", zap.Int("count", len(updates)))

		return &pb.RuleUpdateResponse{
			Success:     true,
			LoadedCount: int32(len(updates)),
			Rules:       updates,
		}, nil
	}

	if len(req.Rules) > 0 {
		logger.Info("acknowledging incremental rule updates, not processing (use detection API for rule management)",
			zap.Int("count", len(req.Rules)))
	}

	return &pb.RuleUpdateResponse{
		Success:     true,
		LoadedCount: int32(len(req.Rules)),
	}, nil
}

func (s *GRPCServer) ExecuteBlockCommand(ctx context.Context, cmd *pb.BlockCommand) (*pb.BlockResponse, error) {
	logger.Info("block command",
		zap.String("command_id", cmd.CommandId),
		zap.String("host_id", cmd.HostId),
		zap.String("action", cmd.Action),
	)

	hostID, err := uuid.Parse(cmd.HostId)
	if err != nil {
		return &pb.BlockResponse{
			CommandId: cmd.CommandId,
			Success:   false,
			Error:     "invalid host id",
		}, nil
	}

	conn, ok := s.agentConnections.Load(hostID)
	if !ok {
		return &pb.BlockResponse{
			CommandId: cmd.CommandId,
			Success:   false,
			Error:     "agent not connected",
		}, nil
	}

	agentConn := conn.(*AgentConnection)

	if agentConn.Stream != nil {
		err := agentConn.Stream.Send(&pb.CommandRequest{
			Request: &pb.CommandRequest_Block{
				Block: cmd,
			},
		})
		if err != nil {
			logger.Error("failed to send block command via stream", zap.Error(err))
			return &pb.BlockResponse{
				CommandId: cmd.CommandId,
				Success:   false,
				Error:     fmt.Sprintf("failed to send block command: %v", err),
			}, nil
		}
		logger.Info("block command sent via stream", zap.String("command_id", cmd.CommandId))
		return &pb.BlockResponse{
			CommandId: cmd.CommandId,
			Success:   true,
		}, nil
	}

	if agentConn.Client == nil {
		return &pb.BlockResponse{
			CommandId: cmd.CommandId,
			Success:   false,
			Error:     "agent not connected",
		}, nil
	}

	resp, callErr := agentConn.Client.ExecuteBlockCommand(ctx, cmd)
	if callErr != nil {
		return &pb.BlockResponse{
			CommandId: cmd.CommandId,
			Success:   false,
			Error:     callErr.Error(),
		}, nil
	}

	return resp, nil
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

			logger.Info("command sent to agent",
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

func (s *GRPCServer) CollectSoftwareList(ctx context.Context, req *pb.SoftwareListRequest) (*pb.SoftwareListResponse, error) {
	// Note: Software collection is done via bidirectional stream (ExecuteCommand),
	// not via unary RPC. Agents receive #SOFTWARE_COLLECT# command and respond
	// through the stream. This unary RPC is kept for backwards compatibility
	// but is not used in normal operation.
	logger.Warn("CollectSoftwareList unary RPC called, but software collection uses bidirectional stream",
		zap.String("method", "CollectSoftwareList"))
	return nil, fmt.Errorf("collect software list should be done via ExecuteCommand stream, not unary RPC")
}

func (s *GRPCServer) CollectSoftwareListForHost(ctx context.Context, hostIDStr string) ([]model.SoftwareInfo, error) {
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

func (s *GRPCServer) pushActiveRulesToAgent(hostID uuid.UUID, conn *AgentConnection) {
	time.Sleep(1 * time.Second)

	if s.sigmaRuleRepo == nil {
		logger.Warn("sigma rule repo not initialized")
		return
	}

	rules, err := s.sigmaRuleRepo.GetActiveAndExperimental()
	if err != nil {
		logger.Error("failed to get active rules for push", zap.Error(err))
		return
	}

	if len(rules) == 0 {
		logger.Info("no active rules to push, sending clear_all to agent", zap.String("host_id", hostID.String()))
		// Send clear_all command to agent to remove all local rules
		if conn.Stream != nil {
			err = conn.Stream.Send(&pb.CommandRequest{
				Request: &pb.CommandRequest_RuleUpdate{
					RuleUpdate: &pb.RuleUpdateRequest{
						Action: "clear_all",
						Rules:  nil,
					},
				},
			})
			if err != nil {
				logger.Error("failed to send clear_all to agent",
					zap.String("host_id", hostID.String()),
					zap.Error(err))
			}
		}
		return
	}

	updates := make([]*pb.RuleUpdate, 0, len(rules))
	for _, rule := range rules {
		updates = append(updates, &pb.RuleUpdate{
			RuleId:  rule.RuleID,
			Action:  "add",
			Content: rule.Content,
		})
	}

	if conn.Stream == nil {
		logger.Warn("agent stream is nil", zap.String("host_id", hostID.String()))
		return
	}

	err = conn.Stream.Send(&pb.CommandRequest{
		Request: &pb.CommandRequest_RuleUpdate{
			RuleUpdate: &pb.RuleUpdateRequest{
				Action: "full_sync",
				Rules:  updates,
			},
		},
	})

	if err != nil {
		logger.Error("failed to push rules to agent",
			zap.String("host_id", hostID.String()),
			zap.Error(err))
	} else {
		logger.Info("pushed active rules to agent",
			zap.String("host_id", hostID.String()),
			zap.Int("rule_count", len(rules)))
	}
}

func (s *GRPCServer) BroadcastRuleUpdate(update *pb.RuleUpdate) {
	s.agentConnections.Range(func(key, value interface{}) bool {
		conn, ok := value.(*AgentConnection)
		if !ok || conn.Stream == nil {
			logger.Warn("agent connection has no stream",
				zap.String("host_id", key.(uuid.UUID).String()))
			return true
		}

		err := conn.Stream.Send(&pb.CommandRequest{
			Request: &pb.CommandRequest_RuleUpdate{
				RuleUpdate: &pb.RuleUpdateRequest{
					Action: "incremental",
					Rules:  []*pb.RuleUpdate{update},
				},
			},
		})

		if err != nil {
			logger.Warn("failed to send rule update to agent",
				zap.String("host_id", key.(uuid.UUID).String()),
				zap.String("rule_id", update.RuleId),
				zap.Error(err))
		} else {
			logger.Info("rule update sent to agent",
				zap.String("host_id", key.(uuid.UUID).String()),
				zap.String("rule_id", update.RuleId),
				zap.String("action", update.Action))
		}
		return true
	})
}

// pushConfigToAgent pushes all configurations (sigma rules, audit rules, audit settings) to an agent on registration.
func (s *GRPCServer) pushConfigToAgent(hostID uuid.UUID) {
	// Wait for agent connection to be established
	var conn interface{}
	var ok bool
	for i := 0; i < 5; i++ {
		conn, ok = s.agentConnections.Load(hostID)
		if ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !ok {
		logger.Warn("agent connection not ready for config push after retries",
			zap.String("host_id", hostID.String()))
		return
	}

	agentConn, ok := conn.(*AgentConnection)
	if !ok || agentConn.CallbackClient == nil {
		logger.Warn("agent callback client not available for config push",
			zap.String("host_id", hostID.String()))
		return
	}

	configs := s.buildAllConfigs()
	if len(configs) == 0 {
		logger.Info("no configs to push", zap.String("host_id", hostID.String()))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := agentConn.CallbackClient.SyncConfig(ctx, &pb.ConfigSyncRequest{
		Configs: configs,
	})
	if err != nil {
		logger.Error("failed to push config to agent",
			zap.String("host_id", hostID.String()),
			zap.Error(err))
	} else {
		logger.Info("config pushed to agent",
			zap.String("host_id", hostID.String()),
			zap.Int("config_count", len(configs)),
			zap.Bool("success", resp.Success))
	}
}

func (s *GRPCServer) pushAgentStartupState(hostID uuid.UUID, conn *AgentConnection) {
	s.pushActiveConfigToAgent(hostID, conn)
	time.Sleep(2 * time.Second)
	s.pushEnabledDetectionPackagesToAgent(hostID, conn)
}

// pushActiveConfigToAgent pushes all configs to an agent via the bidirectional stream.
func (s *GRPCServer) pushActiveConfigToAgent(hostID uuid.UUID, conn *AgentConnection) {
	time.Sleep(1 * time.Second)

	configs := s.buildAllConfigs()
	if len(configs) == 0 {
		logger.Info("no configs to push via stream", zap.String("host_id", hostID.String()))
		return
	}

	if conn.Stream == nil {
		logger.Warn("agent stream is nil for config push", zap.String("host_id", hostID.String()))
		return
	}

	for _, cfg := range configs {
		err := conn.Stream.Send(&pb.CommandRequest{
			Request: &pb.CommandRequest_ConfigSync{
				ConfigSync: cfg,
			},
		})
		if err != nil {
			logger.Error("failed to push config via stream",
				zap.String("host_id", hostID.String()),
				zap.String("config_type", cfg.ConfigType),
				zap.Error(err))
		} else {
			logger.Info("config pushed via stream",
				zap.String("host_id", hostID.String()),
				zap.String("config_type", cfg.ConfigType))
		}
	}
}

func (s *GRPCServer) pushEnabledDetectionPackagesToAgent(hostID uuid.UUID, conn *AgentConnection) {
	if s.detectionPackageRepo == nil {
		logger.Warn("detection package repo not initialized", zap.String("host_id", hostID.String()))
		return
	}
	if conn == nil || conn.Stream == nil {
		logger.Warn("agent stream is nil for detection package push", zap.String("host_id", hostID.String()))
		return
	}

	packages, err := s.detectionPackageRepo.ListEnabled()
	if err != nil {
		logger.Error("failed to list enabled detection packages", zap.String("host_id", hostID.String()), zap.Error(err))
		return
	}
	if len(packages) == 0 {
		logger.Info("no enabled detection packages to push", zap.String("host_id", hostID.String()))
		return
	}

	hostname := ""
	if s.hostRepo != nil {
		if host, err := s.hostRepo.FindByID(hostID); err == nil && host != nil {
			hostname = host.Hostname
		} else if err != nil {
			logger.Warn("failed to load host before detection package push",
				zap.String("host_id", hostID.String()),
				zap.Error(err),
			)
		}
	}

	pushed := 0
	for _, pkg := range packages {
		cmd := &pb.CommandRequest{
			Request: &pb.CommandRequest_DetectionPackageCommand{
				DetectionPackageCommand: &pb.DetectionPackageCommand{
					Action:       "install",
					PackageId:    pkg.PackageID,
					Version:      pkg.Version,
					PackageUrl:   detectionPackageObjectKeyToURL(pkg.PackageObjectKey),
					SignatureUrl: detectionPackageObjectKeyToURL(pkg.SignatureObjectKey),
					PackageSize:  pkg.PackageSize,
				},
			},
		}
		if err := conn.Stream.Send(cmd); err != nil {
			logger.Error("failed to push enabled detection package",
				zap.String("host_id", hostID.String()),
				zap.String("package_id", pkg.PackageID),
				zap.String("version", pkg.Version),
				zap.Error(err),
			)
			continue
		}
		pushed++
		if s.detectionPackageRepo != nil {
			if err := s.detectionPackageRepo.UpsertHostStatus(pkg.PackageID, pkg.Version, hostID, hostname, "installing"); err != nil {
				logger.Warn("failed to record detection package installing status",
					zap.String("host_id", hostID.String()),
					zap.String("package_id", pkg.PackageID),
					zap.String("version", pkg.Version),
					zap.Error(err),
				)
			}
		}
	}

	logger.Info("enabled detection packages pushed to agent",
		zap.String("host_id", hostID.String()),
		zap.Int("package_count", len(packages)),
		zap.Int("pushed", pushed),
	)
}

func detectionPackageObjectKeyToURL(objectKey string) string {
	if objectKey == "" {
		return ""
	}
	if strings.HasPrefix(objectKey, "http://") || strings.HasPrefix(objectKey, "https://") {
		return objectKey
	}
	base := strings.TrimRight(os.Getenv("MINIO_ARTIFACT_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost:9000/aegis-releases"
	}
	return base + "/" + strings.TrimLeft(objectKey, "/")
}

// buildAllConfigs builds all config sync messages for an agent.
func (s *GRPCServer) buildAllConfigs() []*pb.ConfigSync {
	var configs []*pb.ConfigSync

	// 1. Dynamic packages require the hook allowlist before installation.
	if allowlistConfig := s.buildHookAllowlistConfig(); allowlistConfig != nil {
		configs = append(configs, allowlistConfig)
	}

	// 2. Build sigma rules config
	if sigmaConfig := s.buildSigmaRulesConfig(); sigmaConfig != nil {
		configs = append(configs, sigmaConfig)
	}

	// 3. Build audit rules config
	if auditRulesConfig := s.buildAuditRulesConfig(); auditRulesConfig != nil {
		configs = append(configs, auditRulesConfig)
	}

	// 4. Build audit settings config
	if auditSettingsConfig := s.buildAuditSettingsConfig(); auditSettingsConfig != nil {
		configs = append(configs, auditSettingsConfig)
	}

	return configs
}

func (s *GRPCServer) buildHookAllowlistConfig() *pb.ConfigSync {
	if s.detectionPackageRepo == nil {
		return nil
	}
	payload, err := s.detectionPackageRepo.GetActiveAllowlistPayload()
	if err != nil {
		logger.Warn("failed to get active hook allowlist for config sync", zap.Error(err))
		return nil
	}
	return &pb.ConfigSync{
		ConfigType: "dynamic_ebpf_hook_allowlist",
		Action:     "full_sync",
		Payload:    payload,
	}
}

// buildSigmaRulesConfig builds a ConfigSync message with all active/experimental sigma rules.
func (s *GRPCServer) buildSigmaRulesConfig() *pb.ConfigSync {
	if s.sigmaRuleRepo == nil {
		return nil
	}

	rules, err := s.sigmaRuleRepo.GetActiveAndExperimental()
	if err != nil {
		logger.Error("failed to get active rules for config sync", zap.Error(err))
		return nil
	}

	type rulePayload struct {
		RuleID  string `json:"rule_id"`
		Action  string `json:"action"`
		Content string `json:"content"`
	}

	var payload []rulePayload
	for _, rule := range rules {
		payload = append(payload, rulePayload{
			RuleID:  rule.RuleID,
			Action:  "add",
			Content: rule.Content,
		})
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal sigma rules payload", zap.Error(err))
		return nil
	}

	return &pb.ConfigSync{
		ConfigType: "sigma_rules",
		Action:     "full_sync",
		Payload:    string(payloadBytes),
	}
}

// buildAuditRulesConfig builds a ConfigSync message with all enabled audit rules.
func (s *GRPCServer) buildAuditRulesConfig() *pb.ConfigSync {
	if s.commandAuditRuleRepo == nil {
		return nil
	}

	rules, err := s.commandAuditRuleRepo.FindAllEnabled()
	if err != nil {
		logger.Error("failed to get audit rules for config sync", zap.Error(err))
		return nil
	}

	payloadBytes, err := json.Marshal(rules)
	if err != nil {
		logger.Error("failed to marshal audit rules payload", zap.Error(err))
		return nil
	}

	return &pb.ConfigSync{
		ConfigType: "audit_rules",
		Action:     "full_sync",
		Payload:    string(payloadBytes),
	}
}

// buildAuditSettingsConfig builds a ConfigSync message with audit settings.
func (s *GRPCServer) buildAuditSettingsConfig() *pb.ConfigSync {
	if s.systemConfigRepo == nil {
		return nil
	}

	settings, err := s.systemConfigRepo.GetCommandAuditSettings()
	if err != nil {
		logger.Error("failed to get audit settings for config sync", zap.Error(err))
		return nil
	}

	payloadBytes, err := json.Marshal(settings)
	if err != nil {
		logger.Error("failed to marshal audit settings payload", zap.Error(err))
		return nil
	}

	return &pb.ConfigSync{
		ConfigType: "audit_settings",
		Action:     "full_sync",
		Payload:    string(payloadBytes),
	}
}

// GetPort returns the gRPC server port
func (s *GRPCServer) GetPort() int {
	return s.port
}

// SendBlockCommand sends a block command to an agent
func (s *GRPCServer) SendBlockCommand(hostID uuid.UUID, cmd *pb.BlockCommand) error {
	if cmd == nil {
		return fmt.Errorf("block command is nil")
	}
	conn, ok := s.agentConnections.Load(hostID)
	if !ok {
		return fmt.Errorf("agent not connected: %s", hostID)
	}

	agentConn := conn.(*AgentConnection)
	client := agentConn.CallbackClient
	if client == nil {
		client = agentConn.Client
	}
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := client.ExecuteBlockCommand(ctx, cmd)
		if err != nil {
			return err
		}
		if !resp.Success {
			if resp.Error != "" {
				return fmt.Errorf("%s", resp.Error)
			}
			return fmt.Errorf("agent block command failed")
		}
		return nil
	}

	if agentConn.Stream != nil {
		return agentConn.Stream.Send(&pb.CommandRequest{
			Request: &pb.CommandRequest_Block{
				Block: cmd,
			},
		})
	}

	return fmt.Errorf("agent block command channel not available: %s", hostID)
}

func blockTargetForAlert(alert *model.Alert, action string) (string, error) {
	switch action {
	case "kill_process":
		if alert.PID <= 0 {
			return "", fmt.Errorf("missing process pid for kill_process")
		}
		return fmt.Sprintf("%d", alert.PID), nil
	case "quarantine_file":
		target := strings.TrimSpace(alert.CommandLine)
		if target == "" {
			return "", fmt.Errorf("missing file path for quarantine_file")
		}
		return target, nil
	case "block_connection":
		target := strings.TrimSpace(alert.CommandLine)
		if target == "" {
			return "", fmt.Errorf("missing remote address for block_connection")
		}
		if net.ParseIP(target) == nil {
			return "", fmt.Errorf("invalid remote address for block_connection: %s", target)
		}
		return target, nil
	default:
		if alert.PID <= 0 {
			return "", fmt.Errorf("missing process pid for %s", action)
		}
		return fmt.Sprintf("%d", alert.PID), nil
	}
}
