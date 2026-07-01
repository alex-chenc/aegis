package repository

import (
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
