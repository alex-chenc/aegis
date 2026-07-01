package grpc_server

import "testing"

func TestStatusForCommandResultKeepsCheckExitOneSuccessful(t *testing.T) {
	got := statusForCommandResult("CHECK", 1, "")
	if got != "SUCCESS" {
		t.Fatalf("expected CHECK exit 1 to stay SUCCESS, got %s", got)
	}
}

func TestStatusForCommandResultMarksCheckExecutionErrorFailed(t *testing.T) {
	got := statusForCommandResult("CHECK", 2, "syntax error")
	if got != "FAILED" {
		t.Fatalf("expected CHECK execution error to be FAILED, got %s", got)
	}
}

func TestStatusForCommandResultMarksFixExitOneFailed(t *testing.T) {
	got := statusForCommandResult("FIX", 1, "")
	if got != "FAILED" {
		t.Fatalf("expected FIX exit 1 to be FAILED, got %s", got)
	}
}
