package assistant

import (
	"strings"
	"testing"

	"api-server/internal/model"
	"gorm.io/datatypes"
)

func TestBuildRecoveryResumeQueryIncludesDecisionAndResolutionWithoutReplacingOriginalGoal(t *testing.T) {
	request := &model.AssistantRecoveryRequest{
		RecoveryID:       "recovery-1",
		Code:             "detection_package_hook_coverage_blocked",
		Summary:          "Hook coverage needs confirmation.",
		Detail:           "socket and splice are not observable",
		SelectedActionID: "extend_hook_allowlist",
		DecisionInput:    datatypes.JSON([]byte(`{"comment":"continue with the approved hooks"}`)),
		ResolutionResult: datatypes.JSON([]byte(`{"allowlist_version":108,"added_hooks":["tracepoint:syscalls/sys_enter_socket"]}`)),
		Context:          datatypes.JSON([]byte(`{"required_hooks":[{"attach_type":"tracepoint","attach":"syscalls/sys_enter_socket"}]}`)),
	}

	resume := recoveryResumeContextFromRequest(request)
	query := buildRecoveryResumeQuery("generate and build the detector", resume)

	for _, expected := range []string{
		"generate and build the detector",
		"extend_hook_allowlist",
		"allowlist_version",
		"syscalls/sys_enter_socket",
		"continue with the approved hooks",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("resume query does not contain %q: %s", expected, query)
		}
	}
}

func TestRecoveryResumeContextUsesBoundedBuildDiagnostics(t *testing.T) {
	longLog := strings.Repeat("x", maxRecoveryContextTextLength+200)
	request := &model.AssistantRecoveryRequest{
		RecoveryID:       "recovery-build",
		Code:             "detection_package_build_failed",
		SelectedActionID: "regenerate_detection_package",
		Context:          datatypes.JSON([]byte(`{"build_log_tail":"` + longLog + `"}`)),
	}

	query := buildRecoveryResumeQuery("build package", recoveryResumeContextFromRequest(request))
	if len(query) > maxRecoveryResumeQueryLength {
		t.Fatalf("resume query is not bounded: len=%d", len(query))
	}
}

func TestApplyRecoveryResumeContextInjectsBuilderFailureIntoGenerationParameters(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:       "generate a detector",
		Parameters: map[string]interface{}{"vulnerability_description": "AF_ALG exploit"},
	}
	context := &RecoveryResumeContext{
		Code: "detection_package_build_failed",
		BlockerContext: map[string]interface{}{
			"error_message":  "compile perf: exit status 1",
			"build_log_tail": "plugin.c:179:23: error: expected ')'",
		},
	}

	applyRecoveryResumeContextToBreakdown(breakdown, context)
	description, _ := breakdown.Parameters["vulnerability_description"].(string)
	if !strings.Contains(description, "AF_ALG exploit") ||
		!strings.Contains(description, "plugin.c:179:23") {
		t.Fatalf("builder failure was not injected into generation parameters: %#v", breakdown.Parameters)
	}
}
