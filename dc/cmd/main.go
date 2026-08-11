package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dc/config"
	"dc/internal/aggregator"
	"dc/internal/alert_generator"
	"dc/internal/block_manager"
	"dc/internal/kafka_consumer"
	"dc/internal/llm"
	"dc/internal/llm_analyzer"
	"dc/internal/pipeline"
	mcpPipeline "dc/internal/pipeline/mcp"
	"dc/internal/repository"
	"dc/internal/server"
	"dc/internal/sessionaudit"
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
	agentBehaviorRepo := repository.NewAgentBehaviorEventRepository(db)
	agentGuardStateRepo := repository.NewAgentGuardStateRepository(db)
	agentFindingRepo := repository.NewAgentSecurityFindingRepository(db)
	agentActionPublisher := pipeline.NewKafkaAgentActionPublisher(cfg.Kafka.Brokers)
	defer func() {
		if err := agentActionPublisher.Close(); err != nil {
			logger.Warn("agent_guard_action_publisher_close_failed",
				zap.String("error_code", "agent_guard_action_transport_close_failed"),
				zap.Error(err))
		}
	}()
	agentActionCoordinator := pipeline.NewAgentActionCoordinator(
		agentFindingRepo,
		agentFindingRepo,
		agentActionPublisher,
	)
	agentRuleEngine := pipeline.NewAgentRuleEngineWithActions(agentFindingRepo, agentActionCoordinator)

	// Create shared components
	blockMgr := block_manager.NewBlockManager()
	alertGen := alert_generator.NewAlertGenerator(blockMgr)

	// Create LLM client if configured
	var llmClient *llm.Client
	var llmAnalyzer *llm_analyzer.LLMAnalyzer
	if cfg.LLM.APIKey != "" {
		llmClient, err = llm.NewClient(&cfg.LLM)
		if err != nil {
			logger.Warn("Failed to create LLM client, LLM analysis disabled", zap.Error(err))
		} else {
			llmAnalyzer = llm_analyzer.NewLLMAnalyzer(llmClient)
			logger.Info("LLM analyzer enabled")
		}
	} else {
		llmAnalyzer = llm_analyzer.NewLLMAnalyzer(nil)
		logger.Warn("LLM API key not configured, LLM analysis disabled")
	}

	// Create aggregator with 2 minute window and 1000 max events
	agg := aggregator.NewAggregator(2*time.Minute, 1000)

	// Create Kafka consumer with all components
	consumer, err := kafka_consumer.NewKafkaConsumer(
		&cfg.Kafka,
		runtimeEventRepo,
		llmAnalyzer,
		alertGen,
		agg,
		agentBehaviorRepo,
		agentGuardStateRepo,
		cfg.AgentGuard.ProjectionEnabled,
		agentRuleEngine,
		pipeline.AgentRuleProcessingOptions{
			RulesEnabled:    cfg.AgentGuard.RulesEnabled,
			FindingsEnabled: cfg.AgentGuard.FindingsEnabled,
			AlertsEnabled:   cfg.AgentGuard.AlertEnabled,
			ActionFlags: pipeline.AgentActionFeatureFlags{
				ActionEnabled:  cfg.AgentGuard.ActionEnabled,
				DenyEnabled:    cfg.AgentGuard.DenyEnabled,
				FreezeEnabled:  cfg.AgentGuard.FreezeEnabled,
				PublishEnabled: cfg.AgentGuard.ActionPublishEnabled,
			},
		},
	)
	if err != nil {
		logger.Fatal("Failed to create Kafka consumer", zap.Error(err))
	}
	defer consumer.Close()
	var sessionConsumer *sessionaudit.Consumer
	if cfg.AgentSession.Enabled {
		sessionProjector := sessionaudit.NewProjector(db, logger.Get().Named("agent_session_projection"))
		sessionConsumer = sessionaudit.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID, sessionProjector, logger.Get().Named("agent_session_consumer"))
		go func() {
			if err := sessionConsumer.Start(ctx); err != nil && ctx.Err() == nil {
				logger.Warn("agent_session_consumer_stopped", zap.Error(err))
			}
		}()
		defer sessionConsumer.Close()
		logger.Info("agent_session_projection_configured", zap.String("topic", sessionaudit.Topic))
	}
	var mcpConsumer *mcpPipeline.Consumer
	mcpConsumer = mcpPipeline.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID, db, logger.Get().Named("mcp_invocation_projection"))
	go func() {
		if err := mcpConsumer.Start(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("mcp_invocation_consumer_stopped", zap.Error(err))
		}
	}()
	defer mcpConsumer.Close()
	logger.Info("mcp_invocation_projection_configured", zap.String("topic", mcpPipeline.Topic))

	logger.Info("agent_guard_projection_configured",
		zap.Bool("projection_enabled", cfg.AgentGuard.ProjectionEnabled),
		zap.Bool("rules_enabled", cfg.AgentGuard.RulesEnabled),
		zap.Bool("findings_enabled", cfg.AgentGuard.FindingsEnabled),
		zap.Bool("analysis_request_enabled", cfg.AgentGuard.AnalysisRequestEnabled),
		zap.Bool("alert_enabled", cfg.AgentGuard.AlertEnabled),
		zap.Bool("action_enabled", cfg.AgentGuard.ActionEnabled),
		zap.Bool("deny_enabled", cfg.AgentGuard.DenyEnabled),
		zap.Bool("freeze_enabled", cfg.AgentGuard.FreezeEnabled),
		zap.Bool("action_publish_enabled", cfg.AgentGuard.ActionPublishEnabled),
	)

	// Load block policies from database into block manager
	if err := blockMgr.LoadPolicies(ctx, blockPolicyRepo); err != nil {
		logger.Warn("Failed to load block policies", zap.Error(err))
	}

	// Create and start DC server
	dcServer := server.NewServer(
		cfg,
		db,
		consumer,
		alertRepo,
		blockPolicyRepo,
		runtimeEventRepo,
		llmAnalyzer,
		alertGen,
		agg,
	)

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
