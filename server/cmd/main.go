package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"server/config"
	"server/internal/grpc_server"
	"server/internal/queue"
	"server/internal/repository"
	"server/internal/storage"
	"server/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
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

	grpcServer := grpc_server.NewGRPCServer(
		hostRepo,
		redisClient,
		kafkaProducerInstance,
		cfg.Server.GRPCPort,
	)

	go func() {
		logger.Info("Starting gRPC server", zap.Int("port", cfg.Server.GRPCPort))
		if err := grpcServer.Start(); err != nil {
			logger.Fatal("Failed to start gRPC server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	grpcServer.Stop()

	logger.Info("Server stopped")
}