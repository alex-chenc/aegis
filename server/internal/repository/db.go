package repository

import (
	"fmt"
	"time"

	"server/config"
	"server/pkg/logger"

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

	if err := ensureDetectionRuntimeSchema(db); err != nil {
		logger.Error("failed to ensure detection runtime schema", zap.Error(err))
		return nil, fmt.Errorf("failed to ensure detection runtime schema: %w", err)
	}

	logger.Info("database connected successfully",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("dbname", cfg.DBName),
	)

	return db, nil
}

func detectionRuntimeSchemaStatements() []string {
	return []string{
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS ppid INTEGER DEFAULT 0`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS command_line TEXT`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS process_tree JSONB`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS judgment_source VARCHAR(20) DEFAULT 'system'`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS block_status VARCHAR(20) DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS block_message TEXT DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS auto_dispose BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS llm_disposal_strategy TEXT DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_id VARCHAR(128) DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_title VARCHAR(255) DEFAULT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_judgment_source ON alerts(judgment_source)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_block_status ON alerts(block_status)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_rule_id ON alerts(rule_id)`,
		`CREATE TABLE IF NOT EXISTS runtime_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id VARCHAR(64) UNIQUE NOT NULL,
			host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
			event_type VARCHAR(32) NOT NULL,
			event_data JSONB NOT NULL,
			matched_rule_id VARCHAR(128),
			rule_title VARCHAR(255),
			mitre_id VARCHAR(20),
			severity VARCHAR(16),
			pid INTEGER,
			command_line TEXT,
			process_name VARCHAR(255),
			timestamp BIGINT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			aggregated BOOLEAN DEFAULT FALSE
		)`,
		`ALTER TABLE runtime_events ADD COLUMN IF NOT EXISTS process_name VARCHAR(255)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_host_time ON runtime_events(host_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_aggregated ON runtime_events(aggregated)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_events_type ON runtime_events(event_type)`,
		`ALTER TABLE block_policies ADD COLUMN IF NOT EXISTS ai_auto_block BOOLEAN DEFAULT FALSE`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'chk_block_policies_auto_exclusive'
			) THEN
				ALTER TABLE block_policies ADD CONSTRAINT chk_block_policies_auto_exclusive
					CHECK (NOT (auto_block = TRUE AND ai_auto_block = TRUE));
			END IF;
		END $$`,
	}
}

func ensureDetectionRuntimeSchema(db *gorm.DB) error {
	for _, statement := range detectionRuntimeSchemaStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	return nil
}
