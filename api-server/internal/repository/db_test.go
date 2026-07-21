package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectionEnhancementSchemaStatementsIncludeAlertDisplayColumns(t *testing.T) {
	statements := strings.Join(detectionEnhancementSchemaStatements(), "\n")

	requiredFragments := []string{
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_title",
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS ppid",
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS command_line",
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS process_tree",
		"CREATE TABLE IF NOT EXISTS runtime_events",
		"process_name VARCHAR(255)",
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_id",
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS judgment_source",
		"CREATE INDEX IF NOT EXISTS idx_alerts_rule_id",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected detection enhancement schema to include %q", fragment)
		}
	}
}

func TestSigmaRuleEnhancementSchemaStatementsIncludeUploadColumns(t *testing.T) {
	statements := strings.Join(sigmaRuleEnhancementSchemaStatements(), "\n")

	requiredFragments := []string{
		"ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS source",
		"ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_hash",
		"ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS dispatch_hosts",
		"CREATE INDEX IF NOT EXISTS idx_sigma_rules_file_hash",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected sigma rule enhancement schema to include %q", fragment)
		}
	}
}

func TestAssetCollectionSchemaStatementsIncludeAIAssetCategories(t *testing.T) {
	statements := strings.Join(assetCollectionSchemaStatements(), "\n")

	requiredFragments := []string{
		"DROP CONSTRAINT chk_host_application_category",
		"'llm_service'",
		"'ai_agent'",
		"'mcp_server'",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected asset collection schema to include %q", fragment)
		}
	}
}

func TestAssetCollectionSchemaStatementsDefaultToSoftwareCollection(t *testing.T) {
	statements := strings.Join(assetCollectionSchemaStatements(), "\n")
	defaultValue := `'["process","software","application_analysis"]'::jsonb`

	requiredFragments := []string{
		"ALTER TABLE asset_collection_configs",
		"ALTER TABLE asset_collection_tasks",
		"ALTER COLUMN collect_types SET DEFAULT " + defaultValue,
		"SELECT true, 12, " + defaultValue,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected asset collection schema to include %q", fragment)
		}
	}
}

func TestAssetCollectionSchemaStatementsIncludeContainerMetadataColumns(t *testing.T) {
	statements := strings.Join(assetCollectionSchemaStatements(), "\n")

	requiredFragments := []string{
		"is_container       BOOLEAN NOT NULL DEFAULT FALSE",
		"container_id       VARCHAR(128)",
		"container_runtime  VARCHAR(64)",
		"ALTER TABLE host_application_assets ADD COLUMN IF NOT EXISTS is_container",
		"ALTER TABLE host_application_assets ADD COLUMN IF NOT EXISTS container_id",
		"ALTER TABLE host_application_assets ADD COLUMN IF NOT EXISTS container_runtime",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected asset collection schema to include %q", fragment)
		}
	}
}

func TestHostApplicationAssetContainerMetadataMigrations(t *testing.T) {
	repositoryDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository test directory: %v", err)
	}
	migrationsDir := filepath.Clean(filepath.Join(repositoryDir, "..", "..", "..", "migrations"))

	tests := []struct {
		name string
		file string
	}{
		{name: "fresh schema", file: "015_v5.8_intelligent_asset_collection.sql"},
		{name: "existing schema upgrade", file: "026_v6.1_host_application_container_metadata.sql"},
	}
	requiredColumns := []string{"is_container", "container_id", "container_runtime"}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			migrationPath := filepath.Join(migrationsDir, test.file)
			content, err := os.ReadFile(migrationPath)
			if err != nil {
				t.Fatalf("read migration %s: %v", migrationPath, err)
			}
			for _, column := range requiredColumns {
				if !strings.Contains(string(content), column) {
					t.Fatalf("expected migration %s to include column %q", test.file, column)
				}
			}
		})
	}
}

func TestBaselineTaskExecutionSchemaStatementsIncludeAutoVerifyColumns(t *testing.T) {
	statements := strings.Join(baselineTaskExecutionSchemaStatements(), "\n")

	requiredFragments := []string{
		"ALTER TABLE task_logs ADD COLUMN IF NOT EXISTS attempt_no",
		"ALTER TABLE task_logs ADD COLUMN IF NOT EXISTS max_rounds",
		"ALTER TABLE task_logs ADD COLUMN IF NOT EXISTS auto_verify",
		"ALTER TABLE task_logs ADD COLUMN IF NOT EXISTS verify_round",
		"CREATE INDEX IF NOT EXISTS idx_task_logs_auto_verify",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected baseline task schema to include %q", fragment)
		}
	}
}
