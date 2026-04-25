package repository

import (
	"strings"
	"testing"
)

func TestDetectionEnhancementSchemaStatementsIncludeAlertDisplayColumns(t *testing.T) {
	statements := strings.Join(detectionEnhancementSchemaStatements(), "\n")

	requiredFragments := []string{
		"ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_title",
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
