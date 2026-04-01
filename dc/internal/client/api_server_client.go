package client

import (
	"context"
	"dc/config"
	"dc/pkg/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type APIServerClient struct {
	conn *grpc.ClientConn
}

func NewAPIServerClient(cfg *config.Config) (*APIServerClient, error) {
	// For now, we don't have the API Server gRPC service defined
	// This is a placeholder for future integration
	logger.Info("API Server client created (no gRPC connection configured)")
	return &APIServerClient{}, nil
}

func (c *APIServerClient) Connect(addr string) error {
	var err error
	c.conn, err = grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("Failed to connect to API Server", zap.Error(err), zap.String("addr", addr))
		return err
	}
	logger.Info("Connected to API Server", zap.String("addr", addr))
	return nil
}

func (c *APIServerClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *APIServerClient) GetConn() *grpc.ClientConn {
	return c.conn
}

// HealthCheck performs a health check on the API server
func (c *APIServerClient) HealthCheck(ctx context.Context) error {
	// Placeholder - would call API Server health endpoint
	return nil
}