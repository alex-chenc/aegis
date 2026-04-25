package repository

import (
	"fmt"

	"dc/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func NewDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Error("Failed to connect to database", zap.Error(err))
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("Failed to get underlying DB", zap.Error(err))
		return nil, err
	}

	// Set connection pool settings
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(300)

	if err := ensureRuntimeEventSchema(db); err != nil {
		logger.Error("Failed to ensure runtime event schema", zap.Error(err))
		return nil, err
	}

	logger.Info("Database connection established")
	return db, nil
}

func runtimeEventSchemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS runtime_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id VARCHAR(64) UNIQUE NOT NULL,
			host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
			event_type VARCHAR(32) NOT NULL,
			event_data JSONB NOT NULL DEFAULT '{}',
			matched_rule_id VARCHAR(128),
			rule_title VARCHAR(255),
			mitre_id VARCHAR(20),
			severity VARCHAR(16),
			pid INTEGER,
			command_line TEXT,
			process_name VARCHAR(255),
			timestamp BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			aggregated BOOLEAN DEFAULT FALSE
		)`,
		`ALTER TABLE runtime_events ADD COLUMN IF NOT EXISTS process_name VARCHAR(255)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_host_time ON runtime_events(host_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_aggregated ON runtime_events(aggregated)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_type ON runtime_events(event_type)`,
	}
}

func ensureRuntimeEventSchema(db *gorm.DB) error {
	for _, statement := range runtimeEventSchemaStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("runtime event schema statement failed: %w", err)
		}
	}
	return nil
}
