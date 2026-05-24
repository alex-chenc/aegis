package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"aegis-agent/internal/asset"
	"aegis-agent/internal/blocker"
	"aegis-agent/internal/config"
	"aegis-agent/internal/configmgr"
	"aegis-agent/internal/executor"
	"aegis-agent/internal/logger"
	"aegis-agent/internal/sigma"
	"aegis-agent/internal/tools"
	pb "aegis-agent/pkg/api/v1"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const CallbackPort = 19095 // Port for Server to call back to Agent

type Client struct {
	pb.UnimplementedAgentServiceServer // Must embed for forward compatibility
	serverAddr                         string
	authToken                          string
	hostID                             string
	executor                           *executor.Executor
	toolManager                        *tools.ToolManager
	ruleLoader                         *sigma.Loader
	blocker                            *blocker.Blocker
	configManager                      *configmgr.ConfigManager
	conn                               *grpc.ClientConn
	client                             pb.AgentServiceClient
	stream                             pb.AgentService_ExecuteCommandClient
	ctx                                context.Context
	cancel                             context.CancelFunc
	heartbeatDone                      chan struct{}
	callbackServer                     *grpc.Server
	callbackLis                        net.Listener
	callbackPort                       int32
	mu                                 sync.RWMutex
}

func NewClient(cfg *config.Config, exec *executor.Executor, toolManager *tools.ToolManager, ruleLoader *sigma.Loader, blockerInst *blocker.Blocker) *Client {
	return &Client{
		serverAddr:    cfg.ServerAddr,
		authToken:     cfg.AuthToken,
		hostID:        cfg.HostID,
		executor:      exec,
		toolManager:   toolManager,
		ruleLoader:    ruleLoader,
		blocker:       blockerInst,
		configManager: configmgr.NewConfigManager(),
		heartbeatDone: make(chan struct{}),
		callbackPort:  CallbackPort,
	}
}

func (c *Client) ConfigManager() *configmgr.ConfigManager {
	return c.configManager
}

func (c *Client) Run() error {
	reconnectInterval := 5 * time.Second
	maxReconnectInterval := 5 * time.Minute

	for {
		if err := c.connect(); err != nil {
			logger.Error("Connection failed, retrying",
				zap.Error(err),
				zap.Duration("interval", reconnectInterval))
			time.Sleep(reconnectInterval)
			reconnectInterval *= 2
			if reconnectInterval > maxReconnectInterval {
				reconnectInterval = maxReconnectInterval
			}
			continue
		}

		reconnectInterval = 5 * time.Second
		c.run()
		c.cleanup()
		logger.Info("Disconnected, reconnecting...")
	}
}

func (c *Client) connect() error {
	// Start callback gRPC server first (Server will call back to this)
	if err := c.startCallbackServer(); err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}

	var err error
	c.conn, err = grpc.NewClient(c.serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	c.client = pb.NewAgentServiceClient(c.conn)
	c.ctx, c.cancel = context.WithCancel(context.Background())

	if err := c.register(); err != nil {
		return err
	}

	// 心跳延迟到 run() 中双向流建立后启动
	// 这样确保 Agent 显示在线时，双向流已经准备好接收命令
	return nil
}

// startCallbackServer starts a gRPC server for Server to call back with tool execution results
func (c *Client) startCallbackServer() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find an available port
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", CallbackPort))
	if err != nil {
		return fmt.Errorf("failed to listen on callback port %d: %w", CallbackPort, err)
	}

	c.callbackLis = lis
	c.callbackServer = grpc.NewServer()
	pb.RegisterAgentServiceServer(c.callbackServer, c)

	go func() {
		logger.Info("Agent callback server starting", zap.Int("port", CallbackPort))
		if err := c.callbackServer.Serve(lis); err != nil {
			logger.Error("Callback server error", zap.Error(err))
		}
	}()

	logger.Info("Agent callback server started", zap.String("addr", lis.Addr().String()))
	return nil
}

// StopCallbackServer stops the callback server
func (c *Client) StopCallbackServer() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.callbackServer != nil {
		c.callbackServer.GracefulStop()
		c.callbackServer = nil
	}
	if c.callbackLis != nil {
		c.callbackLis.Close()
		c.callbackLis = nil
	}
}

func (c *Client) register() error {
	assetInfo, err := asset.Collect()
	if err != nil {
		return err
	}

	c.mu.RLock()
	callbackPort := c.callbackPort
	c.mu.RUnlock()

	resp, err := c.client.Register(c.ctx, &pb.RegisterRequest{
		HostId:       c.hostID,
		AssetInfo:    toProtoAsset(assetInfo),
		AuthToken:    c.authToken,
		CallbackPort: callbackPort,
	})
	if err != nil {
		return err
	}

	if !resp.Success {
		return err
	}

	c.hostID = resp.HostId
	logger.Info("Registered successfully",
		zap.String("host_id", c.hostID),
		zap.String("ip", assetInfo.IPAddress),
		zap.String("hostname", assetInfo.Hostname),
		zap.String("os", assetInfo.OSType))
	return nil
}

func (c *Client) sendHeartbeats() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer close(c.heartbeatDone)

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			_, err := c.client.Heartbeat(c.ctx, &pb.HeartbeatRequest{
				HostId:    c.hostID,
				Timestamp: time.Now().Unix(),
			})
			if err != nil {
				logger.Error("Heartbeat failed", zap.Error(err))
				return
			}
			logger.Debug("Heartbeat sent")
		}
	}
}

func (c *Client) run() {
	c.heartbeatDone = make(chan struct{})

	var err error
	c.stream, err = c.client.ExecuteCommand(c.ctx)
	if err != nil {
		logger.Error("Failed to create command stream", zap.Error(err))
		return
	}

	if err := c.stream.Send(&pb.CommandRequest{
		Request: &pb.CommandRequest_Execute{
			Execute: &pb.CommandExecute{
				HostId:        c.hostID,
				ScriptContent: "",
			},
		},
	}); err != nil {
		logger.Error("Failed to send ready signal", zap.Error(err))
		return
	}

	logger.Info("Command stream established, starting heartbeat...")

	go c.sendHeartbeats()

	logger.Info("Waiting for commands...")

	for {
		select {
		case <-c.ctx.Done():
			logger.Info("Context cancelled, stopping command receiver")
			return
		default:
			req, err := c.stream.Recv()
			if err != nil {
				logger.Error("Stream receive error", zap.Error(err))
				return
			}

			if execute := req.GetExecute(); execute != nil {
				logger.Info("Received command",
					zap.String("task_id", execute.TaskId),
					zap.String("rule_id", execute.RuleId),
					zap.Int32("timeout", execute.TimeoutSeconds))
				go c.handleCommand(execute)
			}

			if ruleUpdate := req.GetRuleUpdate(); ruleUpdate != nil {
				logger.Info("Received rule update via stream",
					zap.String("action", ruleUpdate.Action),
					zap.Int("rule_count", len(ruleUpdate.Rules)))
				c.applyRuleUpdate(ruleUpdate)
			}

			if block := req.GetBlock(); block != nil {
				logger.Info("Received block command via stream",
					zap.String("command_id", block.CommandId),
					zap.String("action", block.Action),
					zap.String("target", block.Target))
				c.handleBlockCommand(block)
			}

			if configSync := req.GetConfigSync(); configSync != nil {
				logger.Info("Received config sync via stream",
					zap.String("config_type", configSync.ConfigType),
					zap.String("action", configSync.Action))
				if err := c.configManager.ApplyConfigSync(configSync); err != nil {
					logger.Error("Failed to apply config sync",
						zap.String("config_type", configSync.ConfigType),
						zap.Error(err))
				} else {
					logger.Info("Config sync applied",
						zap.String("config_type", configSync.ConfigType))
				}
			}
		}
	}
}

func (c *Client) handleBlockCommand(cmd *pb.BlockCommand) {
	err := c.blocker.Execute(cmd.Action, cmd.Target)
	if err != nil {
		logger.Error("Block command failed",
			zap.String("command_id", cmd.CommandId),
			zap.Error(err))
	} else {
		logger.Info("Block command executed",
			zap.String("command_id", cmd.CommandId),
			zap.String("action", cmd.Action))
	}
}

// SyncConfig handles config sync requests from the Server (called via callback gRPC)
func (c *Client) SyncConfig(ctx context.Context, req *pb.ConfigSyncRequest) (*pb.ConfigSyncResponse, error) {
	logger.Info("Received SyncConfig request",
		zap.Int("config_count", len(req.Configs)))

	applied := make(map[string]bool)
	for _, cfg := range req.Configs {
		if err := c.configManager.ApplyConfigSync(cfg); err != nil {
			logger.Error("Failed to apply config",
				zap.String("config_type", cfg.ConfigType),
				zap.Error(err))
			applied[cfg.ConfigType] = false
		} else {
			logger.Info("Config applied",
				zap.String("config_type", cfg.ConfigType))
			applied[cfg.ConfigType] = true
		}
	}

	return &pb.ConfigSyncResponse{
		Success: true,
		Message: "config sync completed",
		Applied: applied,
	}, nil
}

func (c *Client) applyRuleUpdate(req *pb.RuleUpdateRequest) {
	// Handle clear_all action - removes all rules from memory and disk
	if req.Action == "clear_all" {
		logger.Info("Received clear_all, clearing all rules")
		if err := c.ruleLoader.ClearAll(); err != nil {
			logger.Error("Failed to clear all rules", zap.Error(err))
		}
		return
	}

	for _, rule := range req.Rules {
		if err := c.ruleLoader.ApplyUpdate(rule.Action, rule.RuleId, []byte(rule.Content)); err != nil {
			logger.Error("Failed to apply rule update", zap.String("rule_id", rule.RuleId), zap.Error(err))
			continue
		}
		if rule.Action == "delete" {
			// Delete rule file from disk when rule is deleted
			if err := c.ruleLoader.DeleteRuleFromDisk(rule.RuleId); err != nil {
				logger.Error("Failed to delete rule from disk", zap.String("rule_id", rule.RuleId), zap.Error(err))
			}
		} else if rule.Content != "" {
			if err := c.ruleLoader.SaveRuleToDisk(rule.RuleId, []byte(rule.Content)); err != nil {
				logger.Error("Failed to save rule to disk", zap.String("rule_id", rule.RuleId), zap.Error(err))
			}
		}
		logger.Info("Rule updated", zap.String("rule_id", rule.RuleId), zap.String("action", rule.Action))
	}
}

func (c *Client) handleCommand(execute *pb.CommandExecute) {
	logger.Info("Executing command", zap.String("task_id", execute.TaskId))

	// Check if this is a tool call in #TOOL:ToolName#JSON format
	if strings.HasPrefix(execute.ScriptContent, "#TOOL:") {
		c.handleToolCallCommand(execute)
		return
	}

	result := c.executor.ExecuteCommand(c.ctx, execute.TaskId, execute.ScriptContent, execute.TimeoutSeconds)

	logger.Info("Command completed",
		zap.String("task_id", execute.TaskId),
		zap.Int("exit_code", result.ExitCode),
		zap.Bool("timed_out", result.TimedOut))

	if err := c.stream.Send(&pb.CommandRequest{
		Request: &pb.CommandRequest_Result{
			Result: &pb.CommandResult{
				TaskId:   execute.TaskId,
				HostId:   c.hostID,
				ExitCode: int32(result.ExitCode),
				Stdout:   result.Stdout,
				Stderr:   result.Stderr,
				IsFinal:  true,
			},
		},
	}); err != nil {
		logger.Error("Failed to send result", zap.Error(err))
		return
	}

	logger.Info("Result sent", zap.String("task_id", execute.TaskId))
}

func (c *Client) handleToolCallCommand(execute *pb.CommandExecute) {
	logger.Info("Tool call command received", zap.String("task_id", execute.TaskId))

	// Parse #TOOL:ToolName#JSON format
	scriptContent := execute.ScriptContent
	remaining := strings.TrimPrefix(scriptContent, "#TOOL:")
	parts := strings.SplitN(remaining, "#", 2)
	if len(parts) < 2 {
		c.sendToolError(execute.TaskId, "invalid tool format, expected #TOOL:ToolName#JSON")
		return
	}

	toolName := parts[0]
	paramsJSON := parts[1]

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		c.sendToolError(execute.TaskId, fmt.Sprintf("failed to parse params: %v", err))
		return
	}

	result, err := c.toolManager.Execute(toolName, params)
	if err != nil {
		c.sendToolError(execute.TaskId, err.Error())
		return
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		c.sendToolError(execute.TaskId, fmt.Sprintf("failed to marshal result: %v", err))
		return
	}

	logger.Info("Tool call completed",
		zap.String("task_id", execute.TaskId),
		zap.String("tool", toolName))

	c.sendToolResult(execute.TaskId, string(resultJSON))
}

func (c *Client) sendToolError(taskID, errMsg string) {
	logger.Error("Tool call failed", zap.String("task_id", taskID), zap.String("error", errMsg))
	c.sendToolResult(taskID, fmt.Sprintf(`{"success": false, "error": %q}`, errMsg))
}

func (c *Client) sendToolResult(taskID, resultJSON string) {
	if err := c.stream.Send(&pb.CommandRequest{
		Request: &pb.CommandRequest_Result{
			Result: &pb.CommandResult{
				TaskId:   taskID,
				HostId:   c.hostID,
				ExitCode: 0,
				Stdout:   resultJSON,
				Stderr:   "",
				IsFinal:  true,
			},
		},
	}); err != nil {
		logger.Error("Failed to send tool result", zap.Error(err))
	}
}

func (c *Client) cleanup() {
	if c.cancel != nil {
		c.cancel()
	}
	select {
	case <-c.heartbeatDone:
	case <-time.After(2 * time.Second):
	}
	if c.conn != nil {
		c.conn.Close()
	}
	// Stop callback server on cleanup/reconnect - prevents port binding conflicts
	c.StopCallbackServer()
}

func (c *Client) requestRuleSync() {
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	logger.Info("requesting rule sync from server...")

	resp, err := c.client.UpdateRules(ctx, &pb.RuleUpdateRequest{Action: "full_sync"})
	if err != nil {
		logger.Warn("failed to request rule sync", zap.Error(err))
		return
	}

	if len(resp.Rules) == 0 {
		return
	}

	for _, rule := range resp.Rules {
		if err := c.ruleLoader.ApplyUpdate(rule.Action, rule.RuleId, []byte(rule.Content)); err != nil {
			logger.Error("failed to apply rule", zap.String("rule_id", rule.RuleId), zap.Error(err))
			continue
		}
		if err := c.ruleLoader.SaveRuleToDisk(rule.RuleId, []byte(rule.Content)); err != nil {
			logger.Error("failed to save rule", zap.String("rule_id", rule.RuleId), zap.Error(err))
		}
	}

	logger.Info("rules synced from server", zap.Int("count", len(resp.Rules)))

	// Reload rules from disk
	if err := c.ruleLoader.LoadFromDisk(); err != nil {
		logger.Warn("Failed to reload rules after sync", zap.Error(err))
	} else {
		logger.Info("Rules reloaded after sync", zap.Int("count", c.ruleLoader.RuleCount()))
	}
}

func (c *Client) Close() {
	logger.Info("Closing client...")
	c.StopCallbackServer()
	c.cleanup()
	logger.Info("Client closed")
}

func toProtoAsset(a *asset.AssetInfo) *pb.AssetInfo {
	return &pb.AssetInfo{
		IpAddress:    a.IPAddress,
		Hostname:     a.Hostname,
		OsType:       a.OSType,
		OsVersion:    a.OSVersion,
		Arch:         a.Arch,
		AgentVersion: a.AgentVersion,
	}
}

func (c *Client) ReportEvents(events []*pb.RuntimeEvent) error {
	if len(events) == 0 {
		return nil
	}

	resp, err := c.client.ReportEvent(c.ctx, &pb.ReportEventRequest{
		HostId: c.hostID,
		Events: events,
	})
	if err != nil {
		return fmt.Errorf("failed to report events: %w", err)
	}

	if resp.Success {
		logger.Debug("Events reported", zap.Int("sent", len(events)), zap.Int32("received", resp.ReceivedCount))
	}

	return nil
}

func (c *Client) HandleToolCall(ctx context.Context, req *pb.ToolRequest) (*pb.ToolResponse, error) {
	_ = ctx
	logger.Info("Tool call received", zap.String("call_id", req.CallId), zap.String("tool", req.Tool))

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(req.ParamsJson), &params); err != nil {
		return &pb.ToolResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   fmt.Sprintf("failed to parse params: %v", err),
		}, nil
	}

	result, err := c.toolManager.Execute(req.Tool, params)
	if err != nil {
		return &pb.ToolResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &pb.ToolResponse{
			CallId:  req.CallId,
			Success: false,
			Error:   fmt.Sprintf("failed to marshal tool result: %v", err),
		}, nil
	}

	return &pb.ToolResponse{
		CallId:     req.CallId,
		Success:    true,
		ResultJson: string(resultJSON),
	}, nil
}

func (c *Client) HandleRuleUpdate(ctx context.Context, req *pb.RuleUpdateRequest) (*pb.RuleUpdateResponse, error) {
	_ = ctx
	logger.Info("Rule update received", zap.String("action", req.Action), zap.Int("rule_count", len(req.Rules)))

	// Handle clear_all action - removes all rules from memory and disk
	if req.Action == "clear_all" {
		logger.Info("Received clear_all, clearing all rules")
		if err := c.ruleLoader.ClearAll(); err != nil {
			logger.Error("Failed to clear all rules", zap.Error(err))
			return &pb.RuleUpdateResponse{
				Success:     false,
				LoadedCount: 0,
			}, nil
		}
		return &pb.RuleUpdateResponse{
			Success:     true,
			LoadedCount: 0,
		}, nil
	}

	var loaded int32
	for _, rule := range req.Rules {
		if err := c.ruleLoader.ApplyUpdate(rule.Action, rule.RuleId, []byte(rule.Content)); err != nil {
			logger.Error("Failed to apply rule update", zap.String("rule_id", rule.RuleId), zap.Error(err))
			continue
		}
		loaded++

		if err := c.ruleLoader.SaveRuleToDisk(rule.RuleId, []byte(rule.Content)); err != nil {
			logger.Error("Failed to save rule to disk", zap.Error(err))
		}
	}

	return &pb.RuleUpdateResponse{
		Success:     true,
		LoadedCount: loaded,
	}, nil
}

func (c *Client) HandleBlockCommand(ctx context.Context, cmd *pb.BlockCommand) (*pb.BlockResponse, error) {
	_ = ctx
	if cmd == nil {
		return &pb.BlockResponse{
			Success: false,
			Error:   "block command is nil",
		}, nil
	}
	logger.Info("Block command received", zap.String("command_id", cmd.CommandId), zap.String("action", cmd.Action), zap.String("target", cmd.Target))

	err := c.blocker.Execute(cmd.Action, cmd.Target)
	if err != nil {
		return &pb.BlockResponse{
			CommandId: cmd.CommandId,
			Success:   false,
			Error:     fmt.Sprintf("%s failed for target %q: %v", cmd.Action, cmd.Target, err),
		}, nil
	}

	return &pb.BlockResponse{
		CommandId: cmd.CommandId,
		Success:   true,
	}, nil
}

// ExecuteBlockCommand is an alias for HandleBlockCommand to implement AgentServiceServer interface
func (c *Client) ExecuteBlockCommand(ctx context.Context, cmd *pb.BlockCommand) (*pb.BlockResponse, error) {
	return c.HandleBlockCommand(ctx, cmd)
}

// ExecuteTool is an alias for HandleToolCall to implement AgentServiceServer interface
func (c *Client) ExecuteTool(ctx context.Context, req *pb.ToolRequest) (*pb.ToolResponse, error) {
	return c.HandleToolCall(ctx, req)
}

// gRPC Server methods implementing AgentServiceServer interface

func (c *Client) CollectSoftwareList(ctx context.Context, req *pb.SoftwareListRequest) (*pb.SoftwareListResponse, error) {
	_ = ctx
	logger.Info("CollectSoftwareList request received")

	result, err := c.toolManager.Execute("ListInstalledSoftware", nil)
	if err != nil {
		return &pb.SoftwareListResponse{
			SoftwareList: []*pb.SoftwareInfo{},
		}, nil
	}

	// Convert result to SoftwareInfo array
	softwareList := []*pb.SoftwareInfo{}
	if results, ok := result.([]interface{}); ok {
		for _, r := range results {
			if m, ok := r.(map[string]interface{}); ok {
				info := &pb.SoftwareInfo{}
				if name, ok := m["name"].(string); ok {
					info.Name = name
				}
				if version, ok := m["version"].(string); ok {
					info.Version = version
				}
				if pkgMgr, ok := m["package_manager"].(string); ok {
					info.PackageManager = pkgMgr
				}
				if arch, ok := m["architecture"].(string); ok {
					info.Architecture = arch
				}
				softwareList = append(softwareList, info)
			}
		}
	}

	return &pb.SoftwareListResponse{
		SoftwareList: softwareList,
	}, nil
}

func (c *Client) ReportEvent(ctx context.Context, req *pb.ReportEventRequest) (*pb.ReportEventResponse, error) {
	_ = ctx
	if err := c.ReportEvents(req.Events); err != nil {
		return &pb.ReportEventResponse{
			Success:       false,
			ReceivedCount: 0,
		}, err
	}
	return &pb.ReportEventResponse{
		Success:       true,
		ReceivedCount: int32(len(req.Events)),
	}, nil
}

// Stub implementations for AgentServiceServer interface (used only for callback server)
// The Server will only call ExecuteTool on the callback connection

func (c *Client) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	return &pb.RegisterResponse{
		Success: false,
		Message: "Agent does not accept Register calls on callback server",
	}, nil
}

func (c *Client) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	return &pb.HeartbeatResponse{
		Success: false,
		Message: "Agent does not accept Heartbeat calls on callback server",
	}, nil
}

func (c *Client) ExecuteCommand(stream pb.AgentService_ExecuteCommandServer) error {
	// Agent doesn't accept commands via callback server - commands come through the main connection
	return nil
}

func (c *Client) UpdateRules(ctx context.Context, req *pb.RuleUpdateRequest) (*pb.RuleUpdateResponse, error) {
	return &pb.RuleUpdateResponse{
		Success:     false,
		LoadedCount: 0,
	}, nil
}
