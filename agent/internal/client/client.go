package client

import (
	"context"
	"time"

	"baseline-agent/internal/asset"
	"baseline-agent/internal/config"
	"baseline-agent/internal/executor"
	"baseline-agent/internal/logger"
	pb "baseline-agent/pkg/api/v1"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	serverAddr    string
	authToken     string
	hostID        string
	executor      *executor.Executor
	conn          *grpc.ClientConn
	client        pb.AgentServiceClient
	stream        pb.AgentService_ExecuteCommandClient
	ctx           context.Context
	cancel        context.CancelFunc
	heartbeatDone chan struct{}
}

func NewClient(cfg *config.Config, exec *executor.Executor) *Client {
	return &Client{
		serverAddr:    cfg.ServerAddr,
		authToken:     cfg.AuthToken,
		hostID:        cfg.HostID,
		executor:      exec,
		heartbeatDone: make(chan struct{}),
	}
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

	go c.sendHeartbeats()
	return nil
}

func (c *Client) register() error {
	assetInfo, err := asset.Collect()
	if err != nil {
		return err
	}

	resp, err := c.client.Register(c.ctx, &pb.RegisterRequest{
		HostId:    c.hostID,
		AssetInfo: toProtoAsset(assetInfo),
		AuthToken: c.authToken,
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
	var err error
	c.stream, err = c.client.ExecuteCommand(c.ctx)
	if err != nil {
		logger.Error("Failed to create command stream", zap.Error(err))
		return
	}

	logger.Info("Command stream established, waiting for commands...")

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
		}
	}
}

func (c *Client) handleCommand(execute *pb.CommandExecute) {
	logger.Info("Executing command", zap.String("task_id", execute.TaskId))

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
}

func (c *Client) Close() {
	logger.Info("Closing client...")
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
