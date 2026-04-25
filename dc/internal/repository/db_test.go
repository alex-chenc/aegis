package repository

import (
	"strings"
	"testing"
)

func TestRuntimeEventSchemaStatementsIncludeProcessName(t *testing.T) {
	statements := strings.Join(runtimeEventSchemaStatements(), "\n")

	requiredFragments := []string{
		"CREATE TABLE IF NOT EXISTS runtime_events",
		"ALTER TABLE runtime_events ADD COLUMN IF NOT EXISTS process_name",
		"CREATE INDEX IF NOT EXISTS idx_runtime_events_host_time",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("expected runtime event schema to include %q", fragment)
		}
	}
}
