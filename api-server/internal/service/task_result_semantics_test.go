package service

import "testing"

func TestNormalizeTaskResultStatusKeepsCheckNonCompliantAsSuccess(t *testing.T) {
	got := NormalizeTaskResultStatus("CHECK", "FAILED", 1, "")
	if got != "SUCCESS" {
		t.Fatalf("expected CHECK exit 1 to normalize to SUCCESS, got %s", got)
	}
}

func TestNormalizeTaskResultStatusMarksCheckExecutionErrorFailed(t *testing.T) {
	got := NormalizeTaskResultStatus("CHECK", "SUCCESS", 2, "syntax error: unexpected end of file")
	if got != "FAILED" {
		t.Fatalf("expected CHECK execution error to normalize to FAILED, got %s", got)
	}
}

func TestIsLLMRepairableTaskExcludesCheckNonCompliant(t *testing.T) {
	exitCode := 1
	if IsLLMRepairableTask("CHECK", "FAILED", &exitCode, "") {
		t.Fatal("expected CHECK exit 1 without execution error to be non-repairable")
	}
}

func TestIsLLMRepairableTaskIncludesCheckExecutionError(t *testing.T) {
	exitCode := 2
	if !IsLLMRepairableTask("CHECK", "FAILED", &exitCode, "syntax error") {
		t.Fatal("expected CHECK execution error to be repairable")
	}
}

func TestIsLLMRepairableTaskIncludesFixFailure(t *testing.T) {
	exitCode := 1
	if !IsLLMRepairableTask("FIX", "FAILED", &exitCode, "sed: unterminated s command") {
		t.Fatal("expected FIX failure to be repairable")
	}
}
