package repository

import "testing"

func TestOperationLedgerProjectionExtractsCountsReferencesAndViolations(t *testing.T) {
	counts, references, violations := operationLedgerProjection(map[string]interface{}{
		"expected_count": 12,
		"created_count":  12,
		"task_group_id":  "group-1",
		"error_code":     "coverage_gap",
		"error_message":  "one target is missing",
	})
	if counts["expected_count"] != 12 || counts["created_count"] != 12 {
		t.Fatalf("counts were not projected: %#v", counts)
	}
	if references["task_group_id"] != "group-1" {
		t.Fatalf("references were not projected: %#v", references)
	}
	if len(violations) != 1 || violations[0]["code"] != "coverage_gap" {
		t.Fatalf("violations were not projected: %#v", violations)
	}
}
