package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"server/config"
	"server/internal/grpc_server"
	"server/internal/queue"
	"server/internal/repository"
	"server/internal/storage"
	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	serviceCtx, cancelService := context.WithCancel(context.Background())
	defer cancelService()

	// Initialize logger with default config
	if err := logger.Init(&logger.Config{
		Level:      "info",
		FileName:   "server.log",
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   true,
	}); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	cfg, err := config.Load("config/server.yaml")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Construct DSN for PostgreSQL
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}

	redisClient, err := storage.NewRedisClient(&cfg.Redis)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer redisClient.Close()

	kafkaProducerInstance := queue.NewKafkaProducer(cfg.Kafka.Brokers, logger.Get())
	defer kafkaProducerInstance.Close()

	// Create repositories
	hostRepo := repository.NewHostRepository(db)
	taskLogRepo := repository.NewTaskLogRepository(db)
	sigmaRuleRepo := repository.NewSigmaRuleRepository(db)
	alertRepo := repository.NewAlertRepository(db)
	runtimeEventRepo := repository.NewRuntimeEventRepository(db)
	blockPolicyRepo := repository.NewBlockPolicyRepository(db)
	commandAuditRuleRepo := repository.NewCommandAuditRuleRepo(db)
	systemConfigRepo := repository.NewSystemConfigRepo(db)
	detectionPackageRepo := repository.NewDetectionPackageRepository(db)

	grpcServer := grpc_server.NewGRPCServer(
		hostRepo,
		redisClient,
		kafkaProducerInstance,
		cfg.Server.GRPCPort,
	)

	// Set additional repositories for event handling and rule pushing
	grpcServer.SetTaskLogRepo(taskLogRepo)
	grpcServer.SetSigmaRuleRepo(sigmaRuleRepo)
	grpcServer.SetAlertRepo(alertRepo, nil) // wsBroadcaster is nil for server - DC handles WS broadcasting
	grpcServer.SetRuntimeEventRepo(runtimeEventRepo)
	grpcServer.SetBlockPolicyRepo(blockPolicyRepo)
	grpcServer.SetCommandAuditRuleRepo(commandAuditRuleRepo)
	grpcServer.SetSystemConfigRepo(systemConfigRepo)
	grpcServer.SetDetectionPackageRepo(detectionPackageRepo)

	var agentGuardActionConsumer *queue.KafkaConsumer
	if cfg.AgentGuard.ActionConsumerEnabled {
		groupID := cfg.Kafka.GroupID
		if groupID == "" {
			groupID = "aegis-server"
		}
		agentGuardActionConsumer = queue.NewKafkaConsumer(
			cfg.Kafka.Brokers,
			"aegis.block.commands",
			groupID+"-agent-guard-actions",
			grpcServer.HandleAgentGuardBlockMessage,
			logger.Get(),
		)
		defer func() {
			if err := agentGuardActionConsumer.Close(); err != nil {
				logger.Warn("agent_guard_action_consumer_close_failed",
					zap.String("error_code", "agent_guard_action_consumer_close_failed"),
					zap.Error(err))
			}
		}()
		go func() {
			if err := agentGuardActionConsumer.Start(serviceCtx); err != nil && serviceCtx.Err() == nil {
				logger.Error("agent_guard_action_consumer_failed",
					zap.String("error_code", "agent_guard_action_consumer_failed"),
					zap.Error(err))
			}
		}()
	}
	logger.Info("agent_guard_action_transport_configured",
		zap.Bool("consumer_enabled", cfg.AgentGuard.ActionConsumerEnabled))

	// Create APIServerToServer gRPC server on different port (19094)
	apiServerGRPCPort := 19094
	apiServerLis, err := net.Listen("tcp", fmt.Sprintf(":%d", apiServerGRPCPort))
	if err != nil {
		logger.Fatal("Failed to listen for APIServerToServer gRPC", zap.Error(err), zap.Int("port", apiServerGRPCPort))
	}

	apiServerGRPCServer := grpc.NewServer()

	apiServerClientAddr := os.Getenv("API_SERVER_GRPC_ADDR")
	if apiServerClientAddr == "" {
		apiServerClientAddr = "api-server:19093"
	}
	apiServerClientConn, err := grpc.NewClient(apiServerClientAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Fatal("Failed to create API Server gRPC client", zap.Error(err))
	}
	apiServerClient := pb.NewAPIServerToServerClient(apiServerClientConn)

	apiServerImpl := grpc_server.NewAPIServerToServerImpl(grpcServer, hostRepo, redisClient, apiServerClient)
	pb.RegisterAPIServerToServerServer(apiServerGRPCServer, apiServerImpl)

	// Real-time task result push to the API Server. When an agent reports a
	// final result, the Server immediately notifies the API Server so that
	// auto-verify / large-model repair can trigger without waiting for the
	// API Server's 5s poll. The poll remains as a fallback.
	apiServerHTTPAddr := os.Getenv("API_SERVER_HTTP_ADDR")
	if apiServerHTTPAddr == "" {
		apiServerHTTPAddr = "http://api-server:8082"
	}
	grpcServer.SetTaskResultCallback(grpc_server.NewAPIServerTaskResultPusher(apiServerHTTPAddr))

	go func() {
		logger.Info("Starting APIServerToServer gRPC server", zap.Int("port", apiServerGRPCPort))
		if err := apiServerGRPCServer.Serve(apiServerLis); err != nil {
			logger.Error("Failed to serve APIServerToServer gRPC", zap.Error(err))
		}
	}()

	go func() {
		logger.Info("Starting AgentService gRPC server", zap.Int("port", cfg.Server.GRPCPort))
		if err := grpcServer.Start(); err != nil {
			logger.Fatal("Failed to start gRPC server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")
	cancelService()

	grpcServer.Stop()
	apiServerGRPCServer.GracefulStop()

	logger.Info("Server stopped")
}
