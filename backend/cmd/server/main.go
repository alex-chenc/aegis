package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aegis-system/config"
	"aegis-system/internal/api"
	"aegis-system/internal/api/handler"
	"aegis-system/internal/grpc_server"
	"aegis-system/internal/ipdetect"
	"aegis-system/internal/repository"
	"aegis-system/internal/service"
	"aegis-system/internal/storage"
	"aegis-system/pkg/logger"

	_ "aegis-system/pkg/api/v1"

	"go.uber.org/zap"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/config.yaml", "config file path")
	flag.Parse()

	fmt.Println("Baseline System Server starting...")

	// Initialize logger FIRST (per design spec)
	if err := logger.Init(&logger.Config{
		Level:    "info",
		FileName: "server.log",
		MaxSize:  100,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("server initializing", zap.String("config", *configPath))

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}
	logger.Info("config loaded successfully")

	// Initialize database
	db, err := repository.NewDB(&cfg.Database)
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}
	logger.Info("database connected")

	// Initialize Redis
	redisClient, err := storage.NewRedisClient(&cfg.Redis)
	if err != nil {
		logger.Fatal("failed to connect Redis", zap.Error(err))
	}
	logger.Info("Redis connected")

	// Initialize MinIO
	minioClient, err := storage.NewMinIOClient(&cfg.MinIO)
	if err != nil {
		logger.Fatal("failed to connect MinIO", zap.Error(err))
	}
	logger.Info("MinIO connected")

	// Initialize repositories
	hostRepo := repository.NewHostRepository(db)
	templateRepo := repository.NewTemplateRepository(db)
	ruleRepo := repository.NewRuleRepository(db)
	taskLogRepo := repository.NewTaskLogRepository(db)
	configRepo := repository.NewConfigRepository(db, "default-encryption-key")
	scriptVersionRepo := repository.NewScriptVersionRepository(db)
	healingLogRepo := repository.NewHealingLogRepository(db)
	vulnRepo := repository.NewVulnerabilityRepo(db)

	// Initialize services
	templateService := service.NewTemplateService(templateRepo, ruleRepo, configRepo, minioClient, redisClient, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, 3)
	scriptGenService := service.NewScriptGenerationService(ruleRepo, scriptVersionRepo, configRepo, minioClient, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, 2)
	taskService := service.NewTaskService(taskLogRepo, hostRepo, ruleRepo, healingLogRepo, redisClient, nil)
	selfHealingService := service.NewSelfHealingService(healingLogRepo, scriptVersionRepo, configRepo, ruleRepo, taskLogRepo, minioClient, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, 3)
	vulnService := service.NewVulnerabilityService(vulnRepo, hostRepo, taskLogRepo, redisClient, configRepo, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries)

	// Start background workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	templateService.StartWorkers(ctx)
	scriptGenService.StartWorkers(ctx)
	selfHealingService.StartWorkers(ctx)

	logger.Info("background workers started")

	// Detect server IP
	serverIP := ipdetect.DetectServerIP(cfg.Server.ExternalIP)
	logger.Info("server IP detected",
		zap.String("ip", serverIP),
		zap.String("http_port", fmt.Sprintf("%d", cfg.Server.HTTPPort)),
		zap.String("grpc_port", fmt.Sprintf("%d", cfg.Server.GRPCPort)),
	)

	// Initialize gRPC server
	grpcServer := grpc_server.NewGRPCServer(hostRepo, redisClient, cfg.Server.GRPCPort)
	if err := grpcServer.Start(); err != nil {
		logger.Fatal("failed to start gRPC server", zap.Error(err))
	}
	defer grpcServer.Stop()
	logger.Info("gRPC server started", zap.Int("port", cfg.Server.GRPCPort))

	// Set gRPC server on task service for command dispatch
	grpcServer.SetTaskLogRepo(taskLogRepo)
	grpcServer.SetTaskResultCallback(taskService.ProcessTaskResult)

	taskService.SetGRPCServer(grpcServer)
	taskService.SetScriptGenService(scriptGenService)

	vulnService.SetGRPCServer(grpcServer)
	vulnService.SetTaskService(taskService)

	// Initialize handlers
	configHandler := handler.NewConfigHandler(configRepo, "default-encryption-key")
	hostHandler := handler.NewHostHandler(hostRepo, redisClient, grpcServer)
	templateHandler := handler.NewTemplateHandler(templateRepo, ruleRepo, minioClient, redisClient, templateService, scriptGenService)
	taskHandler := handler.NewTaskHandler(taskService, taskLogRepo, healingLogRepo, scriptGenService, grpcServer, ruleRepo)
	taskHandlerWithHealing := handler.NewTaskHandlerWithHealing(taskService, taskLogRepo, healingLogRepo, scriptGenService, grpcServer, selfHealingService, ruleRepo)
	agentHandler := handler.NewAgentHandler(grpcServer, minioClient, serverIP, cfg.Server.HTTPPort, cfg.Server.GRPCPort)
	ruleHandler := handler.NewRuleHandler(ruleRepo, taskLogRepo, scriptGenService)
	vulnerabilityHandler := handler.NewVulnerabilityHandler(vulnService)

	// Initialize HTTP router
	router := api.NewRouter(configHandler, hostHandler, templateHandler, taskHandler, taskHandlerWithHealing, agentHandler, ruleHandler, vulnerabilityHandler)
	router.Setup(grpcServer)

	// Start HTTP server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
		logger.Info("HTTP server starting", zap.String("addr", addr))
		if err := router.Run(addr); err != nil {
			logger.Fatal("failed to start HTTP server", zap.Error(err))
		}
	}()

	// Start task timeout checker
	taskService.StartTimeoutChecker()

	logger.Info("server started successfully",
		zap.String("http_addr", fmt.Sprintf("http://localhost:%d", cfg.Server.HTTPPort)),
		zap.String("grpc_addr", fmt.Sprintf("localhost:%d", cfg.Server.GRPCPort)),
		zap.String("server_ip", serverIP),
	)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down...")

	// Graceful shutdown
	cancel()
	time.Sleep(2 * time.Second)

	logger.Info("server stopped")
}
