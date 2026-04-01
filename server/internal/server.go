package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"server/config"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type Server struct {
	cfg          *config.Config
	logger       *zap.Logger
	db           *gorm.DB
	redisClient  *redis.Client
	minioClient  interface{}
	grpcServer   *grpc.Server
	httpEngine   *gin.Engine
	grpcListener net.Listener
	httpListener net.Listener
}

func NewServer(cfg *config.Config, logger *zap.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
	}
}

func (s *Server) Start() error {
	s.logger.Info("Initializing Server...")

	var err error
	s.db, err = s.initDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	s.logger.Info("Database connected")

	s.redisClient, err = s.initRedis()
	if err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}
	s.logger.Info("Redis connected")

	s.minioClient, err = s.initMinIO()
	if err != nil {
		return fmt.Errorf("failed to initialize MinIO: %w", err)
	}
	s.logger.Info("MinIO connected")

	s.grpcServer = grpc.NewServer()

	grpcAddr := fmt.Sprintf(":%d", s.cfg.Server.GRPCPort)
	s.grpcListener, err = net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port %d: %w", s.cfg.Server.GRPCPort, err)
	}
	s.logger.Info("gRPC listener started", zap.String("address", grpcAddr))

	go func() {
		s.logger.Info("Starting gRPC server", zap.String("address", grpcAddr))
		if err := s.grpcServer.Serve(s.grpcListener); err != nil {
			s.logger.Error("gRPC server failed", zap.Error(err))
		}
	}()

	s.httpEngine = gin.New()
	s.httpEngine.Use(gin.Recovery())

	s.setupMiddleware()

	s.setupRoutes()

	httpAddr := fmt.Sprintf(":%d", s.cfg.Server.HTTPPort)
	s.httpListener, err = net.Listen("tcp", httpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on HTTP port %d: %w", s.cfg.Server.HTTPPort, err)
	}
	s.logger.Info("HTTP listener started", zap.String("address", httpAddr))

	go func() {
		s.logger.Info("Starting HTTP server", zap.String("address", httpAddr))
		if err := s.httpEngine.Run(httpAddr); err != nil {
			s.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()

	s.waitForShutdown()

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Shutting down Server...")

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.logger.Info("gRPC server stopped")
	}

	if s.httpEngine != nil {
		if err := s.httpListener.Close(); err != nil {
			s.logger.Error("Error closing HTTP listener", zap.Error(err))
		}
		s.logger.Info("HTTP server stopped")
	}

	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err != nil {
			return fmt.Errorf("failed to get database instance: %w", err)
		}
		if err := sqlDB.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
		s.logger.Info("Database connection closed")
	}

	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			return fmt.Errorf("failed to close Redis connection: %w", err)
		}
		s.logger.Info("Redis connection closed")
	}

	if s.minioClient != nil {
		s.logger.Info("MinIO connection closed")
	}

	s.logger.Info("Server shutdown complete")
	return nil
}

func (s *Server) initDB() (*gorm.DB, error) {
	return nil, fmt.Errorf("database initialization not implemented")
}

func (s *Server) initRedis() (*redis.Client, error) {
	return nil, fmt.Errorf("Redis initialization not implemented")
}

func (s *Server) initMinIO() (interface{}, error) {
	return nil, fmt.Errorf("MinIO initialization not implemented")
}

func (s *Server) setupMiddleware() {
}

func (s *Server) setupRoutes() {
}

func (s *Server) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	s.logger.Info("Shutdown signal received")
}
