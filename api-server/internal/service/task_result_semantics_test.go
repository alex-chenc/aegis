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

func TestNormalizeTaskResultStatusKeepsPocVulnerableAsSuccess(t *testing.T) {
	got := NormalizeTaskResultStatus("POC_VERIFY", "FAILED", 1, "")
	if got != "SUCCESS" {
		t.Fatalf("expected POC exit 1 to normalize to SUCCESS, got %s", got)
	}
}

func TestNormalizeTaskResultStatusMarksPocExecutionErrorFailed(t *testing.T) {
	got := NormalizeTaskResultStatus("POC_VERIFY", "SUCCESS", 2, "verification script error")
	if got != "FAILED" {
		t.Fatalf("expected POC exit 2 to normalize to FAILED, got %s", got)
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

func TestIsLLMRepairableTaskExcludesPocVulnerableResult(t *testing.T) {
	exitCode := 1
	if IsLLMRepairableTask("POC_VERIFY", "FAILED", &exitCode, "") {
		t.Fatal("expected POC exit 1 without execution error to be non-repairable")
	}
}

func TestIsTaskExecutionSuccessfulIncludesPocVulnerableResult(t *testing.T) {
	exitCode := 1
	if !IsTaskExecutionSuccessful("POC_VERIFY", "SUCCESS", &exitCode, "") {
		t.Fatal("expected POC exit 1 to be a successful script execution")
	}
}
