package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"dc/config"
	"dc/internal/kafka_consumer"
	"dc/internal/repository"
	"dc/internal/server"
	"dc/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize logger with default config
	if err := logger.Init(&logger.Config{
		Level:      "info",
		FileName:   "dc.log",
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   true,
	}); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	cfg, err := config.Load("config/dc.yaml")
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

	// Create repositories
	alertRepo := repository.NewAlertRepository(db)
	blockPolicyRepo := repository.NewBlockPolicyRepository(db)
	runtimeEventRepo := repository.NewRuntimeEventRepository(db)

	// Create Kafka consumer with repository
	consumer, err := kafka_consumer.NewKafkaConsumer(&cfg.Kafka, runtimeEventRepo)
	if err != nil {
		logger.Fatal("Failed to create Kafka consumer", zap.Error(err))
	}
	defer consumer.Close()

	// Create and start DC server
	dcServer := server.NewServer(cfg, db, consumer, alertRepo, blockPolicyRepo, runtimeEventRepo)

	if err := dcServer.Start(ctx); err != nil {
		logger.Fatal("Failed to start DC server", zap.Error(err))
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down DC...")

	cancel()
	dcServer.Stop()

	logger.Info("DC stopped")
}