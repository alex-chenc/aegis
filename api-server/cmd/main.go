package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"api-server/config"
	"api-server/internal/api"
	"api-server/internal/api/handler"
	"api-server/internal/checker"
	grpcclient "api-server/internal/grpc"
	"api-server/internal/ipdetect"
	"api-server/internal/queue"
	"api-server/internal/repository"
	"api-server/internal/seed"
	"api-server/internal/service"
	"api-server/internal/storage"
	"api-server/pkg/logger"

	"go.uber.org/zap"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/api-server.yaml", "config file path")
	flag.Parse()

	fmt.Println("API Server starting...")

	// Initialize logger FIRST (per design spec)
	if err := logger.Init(&logger.Config{
		Level:      "info",
		FileName:   "api-server.log",
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     7,
		Compress:   true,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("api-server initializing", zap.String("config", *configPath))

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

	// Initialize Kafka producer
	kafkaBrokers := []string{}
	if len(cfg.Kafka.Brokers) > 0 {
		kafkaBrokers = cfg.Kafka.Brokers
	} else {
		kafkaBrokers = []string{"kafka:29092"}
	}
	kafkaProducer := queue.NewKafkaProducer(kafkaBrokers, logger.Get())
	logger.Info("Kafka producer initialized")

	// Initialize gRPC client to Server service
	var serverAddr string
	if cfg.GRPC.ServerAddress != "" {
		serverAddr = cfg.GRPC.ServerAddress
	} else {
		serverAddr = fmt.Sprintf("%s:%d", cfg.Server.ExternalIP, cfg.Server.GRPCPort)
		if cfg.Server.ExternalIP == "" {
			serverAddr = fmt.Sprintf("localhost:%d", cfg.Server.GRPCPort)
		}
	}
	serverClient, err := grpcclient.NewServerClient(serverAddr)
	if err != nil {
		logger.Fatal("failed to connect to Server gRPC", zap.Error(err), zap.String("addr", serverAddr))
	}
	defer serverClient.Close()
	logger.Info("Server gRPC client initialized", zap.String("addr", serverAddr))

	// Initialize repositories
	hostRepo := repository.NewHostRepository(db)
	templateRepo := repository.NewTemplateRepository(db)
	ruleRepo := repository.NewRuleRepository(db)
	taskLogRepo := repository.NewTaskLogRepository(db)
	configRepo := repository.NewConfigRepository(db, "default-encryption-key")
	scriptVersionRepo := repository.NewScriptVersionRepository(db)
	healingLogRepo := repository.NewHealingLogRepository(db)
	vulnRepo := repository.NewVulnerabilityRepo(db)
	customCVEQueryRepo := repository.NewCustomCVEQueryRepository(db)
	hostVulnerabilityScriptRepo := repository.NewHostVulnerabilityScriptRepository(db)
	alertRepo := repository.NewAlertRepository(db)
	blockRepo := repository.NewBlockRepository(db)
	blockPolicyRepo := repository.NewBlockPolicyRepository(db)
	sigmaRuleRepo := repository.NewSigmaRuleRepository(db)
	toolCallRepo := repository.NewToolCallRepository(db)
	llmAggregationRepo := repository.NewLLMAggregationRepository(db)
	runtimeEventRepo := repository.NewRuntimeEventRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	authRepo := repository.NewAuthRepository(db)
	commandAuditRuleRepo := repository.NewCommandAuditRuleRepo(db)
	auditLogRepo := repository.NewAuditLogRepo(db)
	systemConfigRepo := repository.NewSystemConfigRepo(db)

	// V5.7 Script Audit Services (must initialize before services that depend on it)
	blacklistChecker := checker.NewBlacklistChecker()
	scriptAuditService := service.NewScriptAuditService(blacklistChecker, auditLogRepo, configRepo, systemConfigRepo, commandAuditRuleRepo, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries)
	if err := scriptAuditService.ReloadRules(context.Background()); err != nil {
		logger.Warn("failed to load initial audit rules", zap.Error(err))
	}

	// Initialize services
	templateService := service.NewTemplateService(templateRepo, ruleRepo, configRepo, minioClient, redisClient, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, 3)
	scriptGenService := service.NewScriptGenerationService(ruleRepo, scriptVersionRepo, configRepo, minioClient, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, 2, scriptAuditService)
	taskService := service.NewTaskService(taskLogRepo, hostRepo, ruleRepo, healingLogRepo, redisClient, serverClient)
	taskService.SetAuditService(scriptAuditService)
	selfHealingService := service.NewSelfHealingService(healingLogRepo, scriptVersionRepo, configRepo, ruleRepo, taskLogRepo, minioClient, redisClient, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, 3, scriptAuditService)
	vulnService := service.NewVulnerabilityService(vulnRepo, hostRepo, taskLogRepo, redisClient, configRepo, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, serverClient, scriptAuditService)
	customCVEService := service.NewCustomCVEService(vulnRepo, customCVEQueryRepo, configRepo, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries)
	hostVulnerabilityScriptService := service.NewHostVulnerabilityScriptService(hostVulnerabilityScriptRepo, vulnRepo, hostRepo, taskLogRepo, configRepo, scriptAuditService, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries, serverClient)
	alertService := service.NewAlertService(alertRepo, blockPolicyRepo, blockRepo, serverClient)
	sigmaRuleService := service.NewSigmaRuleService(sigmaRuleRepo, serverClient)

	// AI Rule Config Services
	aiRuleConfigRepo := repository.NewAIRuleConfigRepository(db)
	aiRuleConfigService := service.NewAIRuleConfigService(aiRuleConfigRepo)
	ruleGenerationService := service.NewRuleGenerationService(
		aiRuleConfigService,
		configRepo,
		sigmaRuleRepo,
		alertRepo,
		notificationRepo,
		sigmaRuleService,
		serverClient,
		cfg.LLM.TimeoutSeconds,
		cfg.LLM.MaxRetries,
	)
	sigmaRuleUploadService := service.NewSigmaRuleUploadService(sigmaRuleRepo, serverClient)

	// Notification Service
	notificationSvc := service.NewNotificationService(notificationRepo)
	authService := service.NewAuthService(authRepo, redisClient)

	// V5.0 Runtime Detection Services
	wsService := service.NewWebSocketService()
	_ = service.NewLLMAnalysisService(configRepo, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries)
	_ = service.NewBlockService(blockRepo, alertRepo)
	_ = service.NewRuleService(sigmaRuleRepo, kafkaProducer)
	websocketHandler := handler.NewWebSocketHandler(wsService)

	// AI Analysis Services
	// Initialize EmbeddingService for RAG vector search
	embeddingSvc := service.NewEmbeddingService(cfg.LLM.APIKey, cfg.LLM.BaseURL)
	vectorService := service.NewVectorService(db, embeddingSvc)
	aiSessionRepo := repository.NewAISessionRepository(db)
	aiMessageRepo := repository.NewAIMessageRepository(db)
	agentExecRepo := repository.NewAgentExecutionRepository(db)
	aiAutoBlockService := service.NewAIAutoBlockService(alertRepo, blockPolicyRepo, blockRepo, serverClient)
	aiAnalysisHandler := handler.NewAIAnalysisHandler(alertRepo, configRepo, vectorService, serverClient, aiSessionRepo, aiMessageRepo, agentExecRepo, blockPolicyRepo, blockRepo, aiAutoBlockService)

	// Start background workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	templateService.StartWorkers(ctx)
	scriptGenService.StartWorkers(ctx)
	selfHealingService.StartWorkers(ctx)

	logger.Info("background workers started")

	// Detect server IP
	serverIP := ipdetect.DetectServerIP(cfg.Server.ExternalIP)

	logger.Info("server ports configured",
		zap.String("http_port", fmt.Sprintf("%d", cfg.Server.HTTPPort)),
		zap.String("grpc_port", fmt.Sprintf("%d", cfg.Server.GRPCPort)),
	)

	// Start AI rule auto-update service
	ruleGenerationService.Start(ctx)

	// Initialize handlers
	configHandler := handler.NewConfigHandler(configRepo, "default-encryption-key")
	hostHandler := handler.NewHostHandler(hostRepo, redisClient, serverClient)
	templateHandler := handler.NewTemplateHandler(templateRepo, ruleRepo, minioClient, redisClient, templateService, scriptGenService)
	taskHandler := handler.NewTaskHandler(taskService, taskLogRepo, healingLogRepo, scriptGenService, serverClient, ruleRepo, selfHealingService, auditLogRepo)
	taskHandlerWithHealing := handler.NewTaskHandlerWithHealing(taskService, taskLogRepo, healingLogRepo, scriptGenService, serverClient, selfHealingService, ruleRepo, auditLogRepo)
	agentHandler := handler.NewAgentHandler(serverClient, minioClient, serverIP, cfg.Server.HTTPPort, cfg.Server.AgentHubPort)
	ruleHandler := handler.NewRuleHandler(ruleRepo, taskLogRepo, scriptGenService)
	vulnerabilityHandler := handler.NewVulnerabilityHandler(vulnService, customCVEService, hostVulnerabilityScriptService)
	detectionHandler := handler.NewDetectionHandler(alertRepo, blockRepo, blockPolicyRepo, sigmaRuleRepo, toolCallRepo, alertService, sigmaRuleService, sigmaRuleUploadService, llmAggregationRepo, runtimeEventRepo, configRepo, serverClient, wsService, aiRuleConfigService, ruleGenerationService)

	// V5.8 Detection Package
	detectionPkgRepo := repository.NewDetectionPackageRepo(db)

	// Builder gRPC client
	builderAddr := os.Getenv("BUILDER_GRPC_ADDRESS")
	if builderAddr == "" {
		builderAddr = "builder:19096"
	}
	var builderSvcClient service.BuilderClient
	if bc, err := grpcclient.NewBuilderClient(builderAddr); err != nil {
		logger.Warn("failed to connect to builder gRPC, builds will be DB-only", zap.Error(err), zap.String("addr", builderAddr))
	} else {
		builderSvcClient = bc
		defer bc.Close()
		logger.Info("Builder gRPC client initialized", zap.String("addr", builderAddr))
	}

	// Build the base URL agents use to download detection-package artifacts
	// from MinIO.  When MINIO_ARTIFACT_BASE_URL / config value is set, use it
	// directly; otherwise derive from the MinIO endpoint.
	artifactBaseURL := cfg.MinIO.ArtifactBaseURL
	if artifactBaseURL == "" {
		scheme := "http"
		if cfg.MinIO.UseSSL {
			scheme = "https"
		}
		artifactBaseURL = fmt.Sprintf("%s://%s/aegis-releases", scheme, strings.TrimRight(cfg.MinIO.Endpoint, "/"))
	}
	detectionPkgService := service.NewDetectionPackageService(detectionPkgRepo, db, serverClient, builderSvcClient, artifactBaseURL)

	detectionPkgHandler := handler.NewDetectionPackageHandler(detectionPkgService, configRepo, cfg.LLM.TimeoutSeconds, cfg.LLM.MaxRetries)

	notificationHandler := handler.NewNotificationHandler(notificationSvc)
	roleRepo := repository.NewRoleRepo(db)
	authHandler := handler.NewAuthHandler(authService, roleRepo)
	commandAuditHandler := handler.NewCommandAuditHandler(commandAuditRuleRepo, systemConfigRepo, scriptAuditService)
	auditLogHandler := handler.NewAuditLogHandler(auditLogRepo)

	// Initialize HTTP router
	router := api.NewRouter(roleRepo, authService, authHandler, configHandler, hostHandler, templateHandler, taskHandler, taskHandlerWithHealing, agentHandler, ruleHandler, vulnerabilityHandler, detectionHandler, detectionPkgHandler, websocketHandler, notificationHandler, aiAnalysisHandler, commandAuditHandler, auditLogHandler)
	router.Setup()

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

	logger.Info("api-server started successfully",
		zap.String("http_addr", fmt.Sprintf("http://localhost:%d", cfg.Server.HTTPPort)),
		zap.String("server_grpc_addr", serverAddr),
	)

	// Seed default block policies
	seed.SeedBlockPolicies(blockPolicyRepo)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down...")

	// Graceful shutdown
	cancel()
	time.Sleep(2 * time.Second)

	kafkaProducer.Close()
	serverClient.Close()

	logger.Info("api-server stopped")
}
