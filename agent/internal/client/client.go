package client

import (
	"context"
	"fmt"
	"time"

	"baseline-agent/internal/asset"
	"baseline-agent/internal/config"
	"baseline-agent/internal/executor"
	pb "baseline-agent/pkg/api/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client gRPC 客户端
type Client struct {
	serverAddr     string
	authToken      string
	hostID         string
	executor       *executor.Executor
	conn           *grpc.ClientConn
	client         pb.AgentServiceClient
	stream         pb.AgentService_ExecuteCommandClient
	ctx            context.Context
	cancel         context.CancelFunc
	heartbeatDone  chan struct{}
}

// NewClient 创建客户端
func NewClient(cfg *config.Config, exec *executor.Executor) *Client {
	return &Client{
		serverAddr:    cfg.ServerAddr,
		authToken:     cfg.AuthToken,
		hostID:        cfg.HostID,
		executor:      exec,
		heartbeatDone: make(chan struct{}),
	}
}

// Run 运行客户端主循环
func (c *Client) Run() error {
	reconnectInterval := 5 * time.Second
	maxReconnectInterval := 5 * time.Minute

	for {
		if err := c.connect(); err != nil {
			fmt.Printf("Connection failed: %v, retrying in %v\n", err, reconnectInterval)
			time.Sleep(reconnectInterval)
			reconnectInterval *= 2
			if reconnectInterval > maxReconnectInterval {
				reconnectInterval = maxReconnectInterval
			}
			continue
		}

		// 连接成功，重置退避间隔
		reconnectInterval = 5 * time.Second

		// 运行主循环
		c.run()
		
		// 清理连接
		c.cleanup()
		fmt.Println("Disconnected, reconnecting...")
	}
}

// connect 建立 gRPC 连接并注册
func (c *Client) connect() error {
	var err error
	c.conn, err = grpc.NewClient(c.serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to create connection: %w", err)
	}

	c.client = pb.NewAgentServiceClient(c.conn)
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// 发送注册消息
	if err := c.register(); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// 启动心跳 goroutine
	go c.sendHeartbeats()

	return nil
}

// register 发送注册消息
func (c *Client) register() error {
	assetInfo, err := asset.Collect()
	if err != nil {
		return fmt.Errorf("failed to collect asset info: %w", err)
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
		return fmt.Errorf("registration rejected: %s", resp.Message)
	}

	c.hostID = resp.HostId
	fmt.Printf("✓ Registered successfully with host_id: %s\n", c.hostID)
	fmt.Printf("  Asset Info: IP=%s, Hostname=%s, OS=%s\n", 
		assetInfo.IPAddress, assetInfo.Hostname, assetInfo.OSType)
	return nil
}

// sendHeartbeats 定时发送心跳 (30 秒间隔)
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
				fmt.Printf("✗ Heartbeat failed: %v\n", err)
				return
			}
			fmt.Printf("♥ Heartbeat sent\n")
		}
	}
}

// run 运行双向流主循环
func (c *Client) run() {
	var err error
	c.stream, err = c.client.ExecuteCommand(c.ctx)
	if err != nil {
		fmt.Printf("Failed to create command stream: %v\n", err)
		return
	}

	fmt.Println("✓ Command stream established, waiting for commands...")

	// 接收命令循环
	for {
		select {
		case <-c.ctx.Done():
			fmt.Println("Context cancelled, stopping command receiver")
			return
		default:
			req, err := c.stream.Recv()
			if err != nil {
				fmt.Printf("Stream receive error: %v\n", err)
				return
			}

			// 处理接收到的命令
			if execute := req.GetExecute(); execute != nil {
				fmt.Printf("\n⚡ Received command:\n")
				fmt.Printf("  Task ID: %s\n", execute.TaskId)
				fmt.Printf("  Rule ID: %s\n", execute.RuleId)
				fmt.Printf("  Timeout: %ds\n", execute.TimeoutSeconds)
				go c.handleCommand(execute)
			}
		}
	}
}

// handleCommand 处理单个命令
func (c *Client) handleCommand(execute *pb.CommandExecute) {
	fmt.Printf("\n▶ Executing command for task_id: %s...\n", execute.TaskId)

	// 执行命令
	result := c.executor.ExecuteCommand(c.ctx, execute.TaskId, execute.ScriptContent, execute.TimeoutSeconds)

	fmt.Printf("◀ Command completed:\n")
	fmt.Printf("  Exit Code: %d\n", result.ExitCode)
	fmt.Printf("  Timed Out: %v\n", result.TimedOut)
	if result.Stdout != "" {
		fmt.Printf("  Stdout: %s\n", result.Stdout[:min(100, len(result.Stdout))])
	}
	if result.Stderr != "" {
		fmt.Printf("  Stderr: %s\n", result.Stderr[:min(100, len(result.Stderr))])
	}

	// 发送结果回服务器
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
		fmt.Printf("✗ Failed to send result: %v\n", err)
		return
	}

	fmt.Printf("✓ Result sent back to server\n")
}

// cleanup 清理资源
func (c *Client) cleanup() {
	if c.cancel != nil {
		c.cancel()
	}
	// 等待心跳 goroutine 退出
	select {
	case <-c.heartbeatDone:
	case <-time.After(2 * time.Second):
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// Close 关闭客户端
func (c *Client) Close() {
	fmt.Println("Closing client...")
	c.cleanup()
	fmt.Println("Client closed")
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
