package repository

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"api-server/internal/model"
)

func TestAutoMigrateModelsIncludeLegacyRolePermissionSchema(t *testing.T) {
	for _, item := range autoMigrateModels() {
		if _, ok := item.(*model.RolePermission); ok {
			return
		}
	}
	t.Fatal("startup auto migration must keep the legacy role_permissions table")
}

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

func TestAssistantRecoveryMigrationContainsDurableDecisionState(t *testing.T) {
	repositoryDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository test directory: %v", err)
	}
	migrationPath := filepath.Clean(filepath.Join(
		repositoryDir,
		"..", "..", "..",
		"migrations",
		"027_v6.1_assistant_recovery_requests.sql",
	))
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read assistant recovery migration: %v", err)
	}
	requiredFragments := []string{
		"CREATE TABLE IF NOT EXISTS assistant_recovery_requests",
		"recovery_id VARCHAR(100) NOT NULL UNIQUE",
		"actions JSONB NOT NULL",
		"decision_input JSONB NOT NULL",
		"resolution_result JSONB NOT NULL",
		"resume_run_id VARCHAR(100)",
		"uq_assistant_recovery_active_tool_code",
		"WHERE status IN ('pending', 'executing', 'paused')",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(string(content), fragment) {
			t.Fatalf("assistant recovery migration missing %q", fragment)
		}
	}
}

func TestAssistantRecoveryStartupSchemaIncludesActiveRequestUniqueness(t *testing.T) {
	statements := strings.Join(assistantRecoverySchemaStatements(), "\n")
	for _, fragment := range []string{
		"uq_assistant_recovery_active_tool_code",
		"assistant_recovery_requests(tool_call_id, code)",
		"ranked_active_recoveries",
		"duplicate_rank > 1",
		"uq_assistant_recovery_active_run_step_tool_code",
		"COALESCE(NULLIF(step_id, ''), tool_name)",
		"WHERE status IN ('pending', 'executing', 'paused')",
	} {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("assistant recovery startup schema missing %q", fragment)
		}
	}
}

func TestAssistantRecoveryBusinessIdentityMigrationExpiresHistoricalDuplicates(t *testing.T) {
	repositoryDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository test directory: %v", err)
	}
	migrationPath := filepath.Clean(filepath.Join(
		repositoryDir,
		"..", "..", "..",
		"migrations",
		"028_v6.1_assistant_recovery_business_identity.sql",
	))
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read assistant recovery business identity migration: %v", err)
	}
	for _, fragment := range []string{
		"ranked_active_recoveries",
		"status = 'expired'",
		"duplicate_rank > 1",
		"WHEN 'executing' THEN 0",
		"WHEN 'paused' THEN 1",
		"uq_assistant_recovery_active_run_step_tool_code",
		"COALESCE(NULLIF(step_id, ''), tool_name)",
		"WHERE status IN ('pending', 'executing', 'paused')",
	} {
		if !strings.Contains(string(content), fragment) {
			t.Fatalf("assistant recovery business identity migration missing %q", fragment)
		}
	}
}

func TestMCPAuthorizationMigrationSeedsOnlyBootstrapAdmin(t *testing.T) {
	repositoryDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository test directory: %v", err)
	}
	migrationPath := filepath.Clean(filepath.Join(repositoryDir, "..", "..", "..", "migrations", "039_v6.3_mcp_opa_authorization.sql"))
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read MCP authorization migration: %v", err)
	}
	sql := string(content)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS role_permissions",
		"CHECK (role IN ('security_analyst', 'security_developer', 'admin'))",
		"SELECT username, 'admin' FROM auth_users",
		"WHERE username = 'admin'",
		"ON CONFLICT (user_id) DO NOTHING",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("MCP authorization migration missing %q", fragment)
		}
	}
	if strings.Contains(sql, "SELECT username, 'admin' FROM auth_users\nWHERE username NOT IN") {
		t.Fatal("MCP authorization migration must not grant admin to every auth user")
	}
}

func TestMCPAuthorizationMigrationOrdersRolePermissionBootstrapAfterSchema(t *testing.T) {
	repositoryDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository test directory: %v", err)
	}
	migrationsDir := filepath.Clean(filepath.Join(repositoryDir, "..", "..", "..", "migrations"))
	content, err := os.ReadFile(filepath.Join(migrationsDir, "039_v6.3_mcp_opa_authorization.sql"))
	if err != nil {
		t.Fatalf("read MCP authorization migration: %v", err)
	}
	sql := string(content)
	createRole := strings.Index(sql, "CREATE TABLE IF NOT EXISTS role_permissions")
	seedRole := strings.Index(sql, "INSERT INTO role_permissions")
	if createRole < 0 || seedRole < 0 || createRole > seedRole {
		t.Fatalf("039 must create role_permissions before seeding it: create=%d seed=%d", createRole, seedRole)
	}
	authContent, err := os.ReadFile(filepath.Join(migrationsDir, "009_v5.6_auth.sql"))
	if err != nil {
		t.Fatalf("read auth migration: %v", err)
	}
	if !strings.Contains(string(authContent), "CREATE TABLE IF NOT EXISTS auth_users") {
		t.Fatal("039 bootstrap seed depends on auth_users from the root auth migration")
	}
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		t.Fatalf("list root migrations: %v", err)
	}
	sort.Strings(files)
	position := make(map[string]int, len(files))
	for index, file := range files {
		position[filepath.Base(file)] = index
	}
	for _, prerequisite := range []string{
		"009_v5.6_auth.sql",
		"033_v6.3_mcp_platform_control_plane.sql",
		"034_v6.3_mcp_platform_audit_analysis.sql",
		"035_v6.3_mcp_client_endpoints.sql",
	} {
		if position[prerequisite] >= position["039_v6.3_mcp_opa_authorization.sql"] {
			t.Fatalf("039 must run after prerequisite migration %s", prerequisite)
		}
	}
}
