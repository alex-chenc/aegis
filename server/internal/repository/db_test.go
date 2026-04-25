package repository

import (
	"strings"
	"testing"
)

func TestDetectionRuntimeSchemaStatementsIncludeRuntimeEventStorage(t *testing.T) {
	statements := strings.Join(detectionRuntimeSchemaStatements(), "\n")

	requiredFragments := []string{
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS ppid",
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS command_line",
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS process_tree",
		"CREATE TABLE IF NOT EXISTS runtime_events",
		"process_name VARCHAR(255)",
		"CREATE INDEX IF NOT EXISTS idx_runtime_events_host_time",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected detection runtime schema to include %q", fragment)
		}
	}
}
