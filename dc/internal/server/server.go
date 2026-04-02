package server

import (
	"context"
	"fmt"
	"net"

	"dc/config"
	"dc/internal/aggregator"
	"dc/internal/alert_generator"
	"dc/internal/block_manager"
	"dc/internal/kafka_consumer"
	"dc/internal/llm_analyzer"
	"dc/internal/model"
	"dc/internal/repository"
	"dc/pkg/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type Server struct {
	cfg         *config.Config
	db          *gorm.DB
	consumer    *kafka_consumer.KafkaConsumer
	grpcServer  *grpc.Server
	alertGen    *alert_generator.AlertGenerator
	blockMgr    *block_manager.BlockManager
	llmAnalyzer *llm_analyzer.LLMAnalyzer
	aggregator  *aggregator.Aggregator
}

func NewServer(
	cfg *config.Config,
	db *gorm.DB,
	consumer *kafka_consumer.KafkaConsumer,
	alertRepo *repository.AlertRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	runtimeEventRepo *repository.RuntimeEventRepository,
	llmAnalyzer *llm_analyzer.LLMAnalyzer,
	alertGen *alert_generator.AlertGenerator,
	aggregator *aggregator.Aggregator,
) *Server {
	blockMgr := block_manager.NewBlockManager()

	// Use provided alertGen if not nil, otherwise create new one
	if alertGen == nil {
		alertGen = alert_generator.NewAlertGenerator(blockMgr)
	}

	return &Server{
		cfg:         cfg,
		db:          db,
		consumer:    consumer,
		blockMgr:    blockMgr,
		alertGen:    alertGen,
		llmAnalyzer: llmAnalyzer,
		aggregator:  aggregator,
	}
}

func (s *Server) Start(ctx context.Context) error {
	// Start Kafka consumer in background
	go func() {
		if err := s.consumer.Start(ctx); err != nil {
			logger.Error("Kafka consumer error", zap.Error(err))
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Server.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.grpcServer = grpc.NewServer()

	logger.Info("DC server starting",
		zap.Int("grpc_port", s.cfg.Server.GRPCPort),
	)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

func (s *Server) Stop() {
	logger.Info("Stopping DC server")
	if s.consumer != nil {
		s.consumer.Close()
	}
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}

// ProcessEvent processes a runtime event from Kafka
func (s *Server) ProcessEvent(ctx context.Context, event *model.RuntimeEvent) error {
	logger.Debug("Processing event",
		zap.String("event_id", event.EventID),
		zap.String("host_id", event.HostID.String()),
		zap.String("event_type", event.EventType),
	)

	// Store the event
	if err := s.db.Create(event).Error; err != nil {
		logger.Error("Failed to store event", zap.Error(err))
	}

	// Generate alert if needed
	alert := s.alertGen.GenerateAlert(event)
	if alert != nil {
		if err := s.db.Create(alert).Error; err != nil {
			logger.Error("Failed to create alert", zap.Error(err))
		}
		logger.Info("Alert generated",
			zap.String("alert_id", alert.AlertID),
			zap.String("severity", alert.Severity),
		)
	}

	return nil
}