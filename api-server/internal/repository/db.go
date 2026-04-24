package repository

import (
	"fmt"
	"time"

	"api-server/config"
	"api-server/internal/model"
	"api-server/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func NewDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Error("failed to connect database", zap.Error(err))
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		logger.Error("failed to ping database", zap.Error(err))
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Auto-migrate newer generic tables. Core legacy tables already exist in
	// database with proper constraints.
	if err := db.AutoMigrate(
		&model.AIConfig{},
		&model.LLMConfig{},
		&model.Notification{},
	); err != nil {
		logger.Error("failed to auto migrate models", zap.Error(err))
		return nil, fmt.Errorf("failed to auto migrate models: %w", err)
	}

	if err := ensureAIAnalysisTraceSchema(db); err != nil {
		logger.Error("failed to ensure AI analysis trace schema", zap.Error(err))
		return nil, fmt.Errorf("failed to ensure AI analysis trace schema: %w", err)
	}

	logger.Info("database connected successfully",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("dbname", cfg.DBName),
	)

	return db, nil
}

func ensureAIAnalysisTraceSchema(db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ai_analysis_session (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			session_id VARCHAR(100) UNIQUE NOT NULL,
			alert_ids JSONB DEFAULT '[]',
			host_ids JSONB DEFAULT '[]',
			host_filter JSONB DEFAULT '[]',
			time_range JSONB,
			status VARCHAR(20) DEFAULT 'active',
			max_iterations INTEGER DEFAULT 15,
			message_count INTEGER DEFAULT 0,
			tool_call_count INTEGER DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			concluded_at TIMESTAMP WITH TIME ZONE,
			conclusion JSONB
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_analysis_session_session_id ON ai_analysis_session(session_id)`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS alert_ids JSONB DEFAULT '[]'`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS host_ids JSONB DEFAULT '[]'`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS host_filter JSONB DEFAULT '[]'`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS time_range JSONB`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active'`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS max_iterations INTEGER DEFAULT 15`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS message_count INTEGER DEFAULT 0`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS tool_call_count INTEGER DEFAULT 0`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS concluded_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE ai_analysis_session ADD COLUMN IF NOT EXISTS conclusion JSONB`,
		`CREATE TABLE IF NOT EXISTS ai_analysis_message (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			session_id VARCHAR(100) NOT NULL,
			message_id VARCHAR(100) UNIQUE NOT NULL DEFAULT gen_random_uuid()::text,
			role VARCHAR(20) NOT NULL,
			content TEXT,
			thinking TEXT,
			tool_calls JSONB,
			tool_results JSONB,
			steps JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`ALTER TABLE ai_analysis_message ADD COLUMN IF NOT EXISTS message_id VARCHAR(100)`,
		`ALTER TABLE ai_analysis_message ADD COLUMN IF NOT EXISTS thinking TEXT`,
		`ALTER TABLE ai_analysis_message ADD COLUMN IF NOT EXISTS tool_calls JSONB`,
		`ALTER TABLE ai_analysis_message ADD COLUMN IF NOT EXISTS tool_results JSONB`,
		`UPDATE ai_analysis_message SET message_id = gen_random_uuid()::text WHERE message_id IS NULL OR message_id = ''`,
		`ALTER TABLE ai_analysis_message ALTER COLUMN message_id SET NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_analysis_message_message_id ON ai_analysis_message(message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_message_session ON ai_analysis_message(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_message_created ON ai_analysis_message(created_at)`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	return nil
}
