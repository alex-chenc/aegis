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
	gormlogger "gorm.io/gorm/logger"
)

func NewDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Info),
	})
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

	// Pre-migration: clean invalid JSON data in assistant_messages.thinking
	// column before GORM AutoMigrate attempts TEXT -> JSONB conversion.
	if err := cleanInvalidThinkingData(db); err != nil {
		logger.Error("failed to clean invalid thinking data", zap.Error(err))
		return nil, fmt.Errorf("failed to clean invalid thinking data: %w", err)
	}

	// Auto-migrate newer generic tables. Core legacy tables already exist in
	// database with proper constraints.
	if err := db.AutoMigrate(
		&model.AIConfig{},
		&model.LLMConfig{},
		&model.ImageModelConfig{},
		&model.Notification{},
		&model.AuthUser{},
		&model.AuthSession{},
		&model.CommandAuditRule{},
		&model.ScriptAuditLog{},
		&model.SystemConfig{},
		&model.AgentExecution{},
		&model.AgentStepExecution{},
		&model.AgentReflection{},
		&model.AgentAudit{},
		&model.AgentCorrection{},
		&model.AgentToolCallRecord{},
		&model.AgentModelError{},
		&model.DetectionPackageDraft{},
		&model.DetectionPackage{},
		&model.DetectionPackageBuild{},
		&model.DetectionPackageHostStatus{},
		&model.DetectionPackageOperation{},
		&model.EBPFHookAllowlistConfig{},
		&model.CorrelationRule{},
		&model.RolePermission{},
		// V6.0 Assistant tables
		&model.AssistantSession{},
		&model.AssistantMessage{},
		&model.AssistantContextRef{},
		&model.AssistantToolCall{},
		&model.AssistantApproval{},
		&model.AssistantToolSelection{},
		&model.AssistantToolPolicy{},
		&model.AssistantMemory{},
		&model.AssistantInvestigationReport{},
		&model.AssistantInvestigationEvidence{},
		&model.ExternalMCPSource{},
		&model.ExternalMCPQueryLog{},
		// V6.1 Weak password detection tables
		&model.WeakPasswordScanTask{},
		&model.WeakPasswordAssetAppAnalysis{},
		&model.WeakPasswordCandidateApplication{},
		&model.WeakPasswordCollectionPlan{},
		&model.WeakPasswordScanHost{},
		&model.WeakPasswordScanApplication{},
		&model.WeakPasswordAgentToolCall{},
		&model.WeakPasswordDictionary{},
		&model.WeakPasswordDictionaryEntry{},
		&model.WeakPasswordMatchBatch{},
		&model.WeakPasswordFinding{},
		&model.WeakPasswordCollectionError{},
		&model.WeakPasswordAIReport{},
		&model.WeakPasswordRevealAudit{},
	); err != nil {
		logger.Error("failed to auto migrate models", zap.Error(err))
		return nil, fmt.Errorf("failed to auto migrate models: %w", err)
	}

	if err := ensureDetectionEnhancementSchema(db); err != nil {
		logger.Error("failed to ensure detection enhancement schema", zap.Error(err))
		return nil, fmt.Errorf("failed to ensure detection enhancement schema: %w", err)
	}

	if err := ensureSigmaRuleEnhancementSchema(db); err != nil {
		logger.Error("failed to ensure sigma rule enhancement schema", zap.Error(err))
		return nil, fmt.Errorf("failed to ensure sigma rule enhancement schema: %w", err)
	}

	if err := ensureAIAnalysisTraceSchema(db); err != nil {
		logger.Error("failed to ensure AI analysis trace schema", zap.Error(err))
		return nil, fmt.Errorf("failed to ensure AI analysis trace schema: %w", err)
	}

	if err := ensureAssetCollectionSchema(db); err != nil {
		logger.Error("failed to ensure asset collection schema", zap.Error(err))
		return nil, fmt.Errorf("failed to ensure asset collection schema: %w", err)
	}

	logger.Info("database connected successfully",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("dbname", cfg.DBName),
	)

	return db, nil
}

// cleanInvalidThinkingData cleans up non-JSON data in assistant_messages.thinking
// column before GORM AutoMigrate attempts TEXT -> JSONB conversion. This prevents
// migration failure when existing rows contain plain text mixed with JSON.
func cleanInvalidThinkingData(db *gorm.DB) error {
	// Check if assistant_messages table exists
	if !db.Migrator().HasTable("assistant_messages") {
		return nil
	}

	// Check if thinking column exists
	if !db.Migrator().HasColumn(&model.AssistantMessage{}, "thinking") {
		return nil
	}

	var dataType string
	if err := db.Raw(`
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = CURRENT_SCHEMA()
		  AND table_name = 'assistant_messages'
		  AND column_name = 'thinking'
		LIMIT 1
	`).Scan(&dataType).Error; err != nil {
		return err
	}

	if dataType == "json" || dataType == "jsonb" {
		return nil
	}

	// Clean non-JSON thinking data by setting to NULL
	// This includes: empty strings, plain text, and any non-JSON content
	result := db.Exec(`
		UPDATE assistant_messages
		SET thinking = NULL
		WHERE thinking IS NOT NULL
		  AND (thinking::text = '' OR NOT (thinking::text ~ '^\s*[\[\{]'))
	`)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		logger.Info("cleaned invalid thinking data before migration",
			zap.Int64("rows_affected", result.RowsAffected))
	}

	return nil
}

func detectionEnhancementSchemaStatements() []string {
	return []string{
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS judgment_source VARCHAR(20) DEFAULT 'system'`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS block_status VARCHAR(20) DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS block_message TEXT DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS auto_dispose BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS llm_disposal_strategy TEXT DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_id VARCHAR(128) DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_title VARCHAR(255) DEFAULT NULL`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS ppid INTEGER DEFAULT 0`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS command_line TEXT`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS process_tree JSONB`,
		`ALTER TABLE block_policies ADD COLUMN IF NOT EXISTS auto_dispose BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE block_policies ADD COLUMN IF NOT EXISTS ai_auto_block BOOLEAN DEFAULT FALSE`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'chk_block_policies_auto_exclusive'
			) THEN
				ALTER TABLE block_policies ADD CONSTRAINT chk_block_policies_auto_exclusive
					CHECK (NOT (auto_block = TRUE AND ai_auto_block = TRUE));
			END IF;
		END $$`,
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
	}
}

func ensureDetectionEnhancementSchema(db *gorm.DB) error {
	for _, statement := range detectionEnhancementSchemaStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	return nil
}

func sigmaRuleEnhancementSchemaStatements() []string {
	return []string{
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS source VARCHAR(50) DEFAULT 'upload'`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_name VARCHAR(255)`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_hash VARCHAR(64)`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_size INTEGER`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parsed_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parse_error TEXT`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS ai_generated BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parent_rule_id VARCHAR(100)`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS generation_prompt TEXT`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS generation_context TEXT`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS approved_by VARCHAR(100)`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS approved_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS dispatch_hosts TEXT DEFAULT '[]'`,
		`ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS dispatch_status VARCHAR(20) DEFAULT 'pending'`,
		`CREATE INDEX IF NOT EXISTS idx_sigma_rules_file_hash ON sigma_rules(file_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_sigma_rules_ai_generated ON sigma_rules(ai_generated)`,
	}
}

func ensureSigmaRuleEnhancementSchema(db *gorm.DB) error {
	for _, statement := range sigmaRuleEnhancementSchemaStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	return nil
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

// ensureAssetCollectionSchema creates V5.8 intelligent asset collection tables
// if they do not already exist. This ensures the schema is present regardless of
// whether the SQL migration file (015_v5.8_intelligent_asset_collection.sql) was
// manually applied.
func ensureAssetCollectionSchema(db *gorm.DB) error {
	for _, statement := range assetCollectionSchemaStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func assetCollectionSchemaStatements() []string {
	return []string{
		// 1. Asset Collection Configs
		`CREATE TABLE IF NOT EXISTS asset_collection_configs (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			enabled            BOOLEAN NOT NULL DEFAULT true,
			interval_hours     INT NOT NULL DEFAULT 12,
			collect_types      JSONB NOT NULL DEFAULT '["process","application_analysis"]',
			scope              VARCHAR(32) NOT NULL DEFAULT 'all_hosts',
			next_run_at        TIMESTAMPTZ,
			last_run_at        TIMESTAMPTZ,
			updated_by         VARCHAR(100),
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_asset_collection_config_interval') THEN
				ALTER TABLE asset_collection_configs ADD CONSTRAINT chk_asset_collection_config_interval
					CHECK (interval_hours >= 1 AND interval_hours <= 168);
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_asset_collection_config_scope') THEN
				ALTER TABLE asset_collection_configs ADD CONSTRAINT chk_asset_collection_config_scope
					CHECK (scope IN ('all_hosts','host_group','hosts'));
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_asset_collection_configs_next_run
			ON asset_collection_configs(enabled, next_run_at)`,
		`INSERT INTO asset_collection_configs (enabled, interval_hours, collect_types, scope)
			SELECT true, 12, '["process","application_analysis"]'::jsonb, 'all_hosts'
			WHERE NOT EXISTS (SELECT 1 FROM asset_collection_configs)`,

		// 2. Asset Collection Tasks
		`CREATE TABLE IF NOT EXISTS asset_collection_tasks (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			task_type          VARCHAR(32) NOT NULL DEFAULT 'full',
			trigger_source     VARCHAR(32) NOT NULL DEFAULT 'manual',
			scope              VARCHAR(32) NOT NULL DEFAULT 'hosts',
			host_filter        JSONB NOT NULL DEFAULT '[]',
			collect_types      JSONB NOT NULL DEFAULT '["process","application_analysis"]',
			status             VARCHAR(32) NOT NULL DEFAULT 'collecting',
			total_hosts        INT NOT NULL DEFAULT 0,
			success_hosts      INT NOT NULL DEFAULT 0,
			failed_hosts       INT NOT NULL DEFAULT 0,
			current_stage      VARCHAR(64),
			error_message      TEXT,
			requested_by       VARCHAR(100),
			started_at         TIMESTAMPTZ,
			finished_at        TIMESTAMPTZ,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_asset_collection_task_status') THEN
				ALTER TABLE asset_collection_tasks ADD CONSTRAINT chk_asset_collection_task_status
					CHECK (status IN ('collecting','analyzing','completed','failed','cancelled'));
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_status
			ON asset_collection_tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_created_at
			ON asset_collection_tasks(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_host_filter
			ON asset_collection_tasks USING GIN(host_filter)`,

		// 3. Asset Collection Task Hosts
		`CREATE TABLE IF NOT EXISTS asset_collection_task_hosts (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			task_id            UUID NOT NULL,
			host_id            UUID NOT NULL,
			hostname           VARCHAR(255),
			ip_address         VARCHAR(45),
			status             VARCHAR(32) NOT NULL DEFAULT 'collecting',
			collect_started_at TIMESTAMPTZ,
			collect_finished_at TIMESTAMPTZ,
			software_count     INT NOT NULL DEFAULT 0,
			process_count      INT NOT NULL DEFAULT 0,
			application_count  INT NOT NULL DEFAULT 0,
			error_message      TEXT,
			raw_snapshot_id    UUID,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// Add foreign keys only if they don't exist
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'asset_collection_task_hosts_task_id_fkey') THEN
				ALTER TABLE asset_collection_task_hosts ADD CONSTRAINT asset_collection_task_hosts_task_id_fkey
					FOREIGN KEY (task_id) REFERENCES asset_collection_tasks(id) ON DELETE CASCADE;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'asset_collection_task_hosts_host_id_fkey') THEN
				ALTER TABLE asset_collection_task_hosts ADD CONSTRAINT asset_collection_task_hosts_host_id_fkey
					FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'asset_collection_task_hosts_task_id_host_id_key') THEN
				ALTER TABLE asset_collection_task_hosts ADD CONSTRAINT asset_collection_task_hosts_task_id_host_id_key
					UNIQUE(task_id, host_id);
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_task
			ON asset_collection_task_hosts(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_host
			ON asset_collection_task_hosts(host_id)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_status
			ON asset_collection_task_hosts(status)`,

		// 4. Host Software Assets
		`CREATE TABLE IF NOT EXISTS host_software_assets (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			host_id            UUID NOT NULL,
			hostname           VARCHAR(255),
			ip_address         VARCHAR(45),
			group_name         VARCHAR(255) NOT NULL DEFAULT '默认分组',
			os_type            VARCHAR(50) NOT NULL,
			package_manager    VARCHAR(32) NOT NULL,
			name               VARCHAR(255) NOT NULL,
			version            VARCHAR(255),
			release            VARCHAR(255),
			epoch              VARCHAR(64),
			architecture       VARCHAR(64),
			source_name        VARCHAR(255),
			vendor             VARCHAR(255),
			license            VARCHAR(255),
			install_paths      JSONB NOT NULL DEFAULT '[]',
			file_count         INT NOT NULL DEFAULT 0,
			package_metadata   JSONB NOT NULL DEFAULT '{}',
			fingerprint        VARCHAR(128) NOT NULL,
			status             VARCHAR(32) NOT NULL DEFAULT 'active',
			last_modified_at   TIMESTAMPTZ,
			first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_software_assets_host_id_fkey') THEN
				ALTER TABLE host_software_assets ADD CONSTRAINT host_software_assets_host_id_fkey
					FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_host_software_package_manager') THEN
				ALTER TABLE host_software_assets ADD CONSTRAINT chk_host_software_package_manager
					CHECK (package_manager IN ('rpm','dpkg','apk','unknown'));
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_host_software_status') THEN
				ALTER TABLE host_software_assets ADD CONSTRAINT chk_host_software_status
					CHECK (status IN ('active','inactive','deleted'));
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_software_assets_host_id_package_manager_fingerprint_key') THEN
				ALTER TABLE host_software_assets ADD CONSTRAINT host_software_assets_host_id_package_manager_fingerprint_key
					UNIQUE(host_id, package_manager, fingerprint);
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_host_software_assets_host ON host_software_assets(host_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_software_assets_name ON host_software_assets(name)`,
		`CREATE INDEX IF NOT EXISTS idx_host_software_assets_version ON host_software_assets(version)`,
		`CREATE INDEX IF NOT EXISTS idx_host_software_assets_manager ON host_software_assets(package_manager)`,
		`CREATE INDEX IF NOT EXISTS idx_host_software_assets_seen ON host_software_assets(last_seen_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_host_software_assets_paths ON host_software_assets USING GIN(install_paths)`,

		// 5. Host Process Snapshots
		`CREATE TABLE IF NOT EXISTS host_process_snapshots (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			task_id            UUID,
			host_id            UUID NOT NULL,
			hostname           VARCHAR(255),
			ip_address         VARCHAR(45),
			process_count      INT NOT NULL DEFAULT 0,
			listen_port_count  INT NOT NULL DEFAULT 0,
			snapshot_hash      VARCHAR(64) NOT NULL,
			snapshot_json      JSONB NOT NULL DEFAULT '{}',
			redaction_summary  JSONB NOT NULL DEFAULT '{}',
			collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_process_snapshots_task_id_fkey') THEN
				ALTER TABLE host_process_snapshots ADD CONSTRAINT host_process_snapshots_task_id_fkey
					FOREIGN KEY (task_id) REFERENCES asset_collection_tasks(id) ON DELETE SET NULL;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_process_snapshots_host_id_fkey') THEN
				ALTER TABLE host_process_snapshots ADD CONSTRAINT host_process_snapshots_host_id_fkey
					FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_host
			ON host_process_snapshots(host_id, collected_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_task
			ON host_process_snapshots(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_hash
			ON host_process_snapshots(snapshot_hash)`,

		// 6. Host Application Assets
		`CREATE TABLE IF NOT EXISTS host_application_assets (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			host_id            UUID NOT NULL,
			hostname           VARCHAR(255),
			ip_address         VARCHAR(45),
			group_name         VARCHAR(255) NOT NULL DEFAULT '默认分组',
			os_type            VARCHAR(50) NOT NULL,
			category           VARCHAR(32) NOT NULL DEFAULT 'unknown',
			name               VARCHAR(255) NOT NULL,
			display_name       VARCHAR(255),
			version            VARCHAR(255),
			version_source     VARCHAR(64),
			install_path       TEXT,
			start_path         TEXT,
			config_paths       JSONB NOT NULL DEFAULT '[]',
			site_paths         JSONB NOT NULL DEFAULT '[]',
			domains            JSONB NOT NULL DEFAULT '[]',
			listen_ports       JSONB NOT NULL DEFAULT '[]',
			run_user           VARCHAR(255),
			runtime_name       VARCHAR(100),
			runtime_version    VARCHAR(100),
			framework_name     VARCHAR(100),
			framework_version  VARCHAR(100),
			related_pids       JSONB NOT NULL DEFAULT '[]',
			related_packages   JSONB NOT NULL DEFAULT '[]',
			ai_confidence      NUMERIC(4,3) NOT NULL DEFAULT 0,
			ai_evidence        JSONB NOT NULL DEFAULT '[]',
			ai_raw_output      JSONB NOT NULL DEFAULT '{}',
			manual_overrides   JSONB NOT NULL DEFAULT '{}',
			review_status      VARCHAR(32) NOT NULL DEFAULT 'auto',
			status             VARCHAR(32) NOT NULL DEFAULT 'active',
			fingerprint        VARCHAR(128) NOT NULL,
			first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_application_assets_host_id_fkey') THEN
				ALTER TABLE host_application_assets ADD CONSTRAINT host_application_assets_host_id_fkey
					FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_host_application_category') THEN
				ALTER TABLE host_application_assets DROP CONSTRAINT chk_host_application_category;
			END IF;
			ALTER TABLE host_application_assets ADD CONSTRAINT chk_host_application_category
				CHECK (category IN ('database','web_service','web_framework','web_site','llm_service','ai_agent','mcp_server','other','unknown'));
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_host_application_review') THEN
				ALTER TABLE host_application_assets ADD CONSTRAINT chk_host_application_review
					CHECK (review_status IN ('pending','confirmed','rejected','auto'));
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_host_application_status') THEN
				ALTER TABLE host_application_assets ADD CONSTRAINT chk_host_application_status
					CHECK (status IN ('active','inactive','deleted','needs_review'));
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_application_assets_host_id_fingerprint_key') THEN
				ALTER TABLE host_application_assets ADD CONSTRAINT host_application_assets_host_id_fingerprint_key
					UNIQUE(host_id, fingerprint);
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_assets_host ON host_application_assets(host_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_assets_category ON host_application_assets(category)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_assets_name ON host_application_assets(name)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_assets_version ON host_application_assets(version)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_assets_ports ON host_application_assets USING GIN(listen_ports)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_assets_review ON host_application_assets(review_status)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_assets_seen ON host_application_assets(last_seen_at DESC)`,

		// 7. Host Application Tool Calls
		`CREATE TABLE IF NOT EXISTS host_application_tool_calls (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			task_id            UUID,
			application_id     UUID,
			host_id            UUID NOT NULL,
			call_id            VARCHAR(128) NOT NULL,
			tool_name          VARCHAR(128) NOT NULL,
			arguments_json     JSONB NOT NULL DEFAULT '{}',
			result_json        JSONB NOT NULL DEFAULT '{}',
			success            BOOLEAN NOT NULL DEFAULT false,
			error_message      TEXT,
			execution_time_ms  BIGINT NOT NULL DEFAULT 0,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_application_tool_calls_task_id_fkey') THEN
				ALTER TABLE host_application_tool_calls ADD CONSTRAINT host_application_tool_calls_task_id_fkey
					FOREIGN KEY (task_id) REFERENCES asset_collection_tasks(id) ON DELETE SET NULL;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_application_tool_calls_application_id_fkey') THEN
				ALTER TABLE host_application_tool_calls ADD CONSTRAINT host_application_tool_calls_application_id_fkey
					FOREIGN KEY (application_id) REFERENCES host_application_assets(id) ON DELETE SET NULL;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_application_tool_calls_host_id_fkey') THEN
				ALTER TABLE host_application_tool_calls ADD CONSTRAINT host_application_tool_calls_host_id_fkey
					FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_application_tool_calls_call_id_key') THEN
				ALTER TABLE host_application_tool_calls ADD CONSTRAINT host_application_tool_calls_call_id_key
					UNIQUE(call_id);
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_app
			ON host_application_tool_calls(application_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_host
			ON host_application_tool_calls(host_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_tool
			ON host_application_tool_calls(tool_name)`,

		// 8. Extend host_vulnerabilities with asset references
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS software_asset_id UUID`,
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS application_asset_id UUID`,
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS asset_name VARCHAR(255)`,
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS asset_version VARCHAR(255)`,
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS asset_collected_at TIMESTAMPTZ`,
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS vulnerability_source JSONB NOT NULL DEFAULT '{}'`,
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS match_evidence JSONB NOT NULL DEFAULT '[]'`,
		`ALTER TABLE host_vulnerabilities ADD COLUMN IF NOT EXISTS verification_status VARCHAR(32) NOT NULL DEFAULT 'verified'`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_vulnerabilities_software_asset_id_fkey') THEN
				ALTER TABLE host_vulnerabilities ADD CONSTRAINT host_vulnerabilities_software_asset_id_fkey
					FOREIGN KEY (software_asset_id) REFERENCES host_software_assets(id) ON DELETE SET NULL;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'host_vulnerabilities_application_asset_id_fkey') THEN
				ALTER TABLE host_vulnerabilities ADD CONSTRAINT host_vulnerabilities_application_asset_id_fkey
					FOREIGN KEY (application_asset_id) REFERENCES host_application_assets(id) ON DELETE SET NULL;
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_software_asset
			ON host_vulnerabilities(software_asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_application_asset
			ON host_vulnerabilities(application_asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_verification_status
			ON host_vulnerabilities(verification_status)`,
	}
}
