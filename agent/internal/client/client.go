package client

import (
	"context"
	"fmt"
	"time"

	"baseline-agent/internal/asset"
	"baseline-agent/internal/config"
	"baseline-agent/internal/executor"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client gRPC 客户端
type Client struct {
	serverAddr     string
	authToken      string
	hostID         string
	conn           *grpc.ClientConn
	executor       *executor.Executor
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewClient 创建客户端
func NewClient(cfg *config.Config, exec *executor.Executor) *Client {
	return &Client{
		serverAddr: cfg.ServerAddr,
		authToken:  cfg.AuthToken,
		hostID:     cfg.HostID,
		executor:   exec,
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

		reconnectInterval = 5 * time.Second
		c.run()
		c.cleanup()
	}
}

// connect 建立连接
func (c *Client) connect() error {
	var err error
	c.conn, err = grpc.NewClient(c.serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())
	return c.register()
}

// register 注册
func (c *Client) register() error {
	assetInfo, _ := asset.Collect()
	fmt.Printf("Registered with host_id: %s, IP: %s\n", c.hostID, assetInfo.IPAddress)
	return nil
}

// run 运行主循环
func (c *Client) run() {
	fmt.Println("Agent running, waiting for commands...")
	<-c.ctx.Done()
}

// cleanup 清理
func (c *Client) cleanup() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// Close 关闭
func (c *Client) Close() {
	c.cleanup()
}
