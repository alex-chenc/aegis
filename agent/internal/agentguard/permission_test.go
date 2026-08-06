package agentguard

import "testing"

func restrictedPermissionForTest() EscapePermissionContext {
	return NormalizeEscapePermission("default", "workspace-write", "on-request", "", "/workspace/project", []string{"/workspace/project"}, []string{"/tmp"}, boolPtr(false), "hook")
}

func completeEscapeAttemptForTest(operation, target string, returnCode int64) GuardAttempt {
	return GuardAttempt{
		EventID: "event-1", Category: CategoryFile, Operation: operation, Target: target,
		ReturnCode: returnCode, ToolCallID: "tool-1", HookEventID: "hook-1",
		HookMatched: true, ProcessMatched: true, ProcReverified: true,
		Baseline: completeIsolationStateForTest("/agent/unit"),
		Actual:   completeIsolationStateForTest("/agent/unit"), Outcome: outcomeForReturnCode(returnCode),
	}
}

func completeIsolationStateForTest(cgroup string) IsolationState {
	state := newIsolationState()
	for key := range state.Availability {
		state.Availability[key] = EvidenceAvailability{Available: true}
	}
	state.NamespaceInodes = map[string]uint64{"mnt": 1, "pid": 2, "net": 3, "user": 4}
	state.CgroupPath = cgroup
	state.CapturedAt = state.CapturedAt.Add(1)
	return state
}

func boolPtr(value bool) *bool { return &value }

func outcomeForReturnCode(code int64) Outcome {
	if code < 0 {
		return OutcomeFailed
	}
	return OutcomeSuccess
}

func TestEvaluateEscapeAttemptFullAccessIsNotApplicable(t *testing.T) {
	policy := NormalizeEscapePermission("bypassPermissions", "danger-full-access", "never", "", "/workspace/project", nil, nil, nil, "hook")
	attempt := completeEscapeAttemptForTest("connect_unix", "/var/run/docker.sock", 0)
	result, reported := EvaluateEscapeAttempt(attempt, policy)
	if reported || result.Classification != EscapeClassificationNotApplicable {
		t.Fatalf("full access result = %#v, reported=%v", result, reported)
	}
}

func TestEvaluateEscapeAttemptDeniedRestrictedIsSuspicious(t *testing.T) {
	attempt := completeEscapeAttemptForTest("connect_unix", "/var/run/docker.sock", -13)
	result, reported := EvaluateEscapeAttempt(attempt, restrictedPermissionForTest())
	if !reported || result.Classification != EscapeClassificationPolicyViolationAttempt {
		t.Fatalf("denied restricted result = %#v, reported=%v", result, reported)
	}
	if result.Violation.Decision != DecisionAlert {
		t.Fatalf("decision = %s", result.Violation.Decision)
	}
}

func TestEvaluateEscapeAttemptSuccessfulRestrictedIsConfirmed(t *testing.T) {
	attempt := completeEscapeAttemptForTest("connect_unix", "/var/run/docker.sock", 0)
	result, reported := EvaluateEscapeAttempt(attempt, restrictedPermissionForTest())
	if !reported || result.Classification != EscapeClassificationConfirmedEscape {
		t.Fatalf("successful restricted result = %#v, reported=%v", result, reported)
	}
}

func TestEvaluateEscapeAttemptUnknownOrIncompleteIsSuppressed(t *testing.T) {
	attempt := completeEscapeAttemptForTest("connect_unix", "/var/run/docker.sock", 0)
	unknown := NormalizeEscapePermission("", "", "", "", "", nil, nil, nil, "")
	if result, reported := EvaluateEscapeAttempt(attempt, unknown); reported || result.Classification != EscapeClassificationInsufficientEvidence {
		t.Fatalf("unknown result = %#v, reported=%v", result, reported)
	}
	attempt.ProcReverified = false
	if result, reported := EvaluateEscapeAttempt(attempt, restrictedPermissionForTest()); !reported || result.Classification != EscapeClassificationConfirmedEscape {
		t.Fatalf("process execution should not require cgroup re-verification: %#v, reported=%v", result, reported)
	}
}

func TestEvaluateEscapeAttemptAuthorizedExpansionIsSuppressed(t *testing.T) {
	attempt := completeEscapeAttemptForTest("connect_unix", "/var/run/docker.sock", 0)
	attempt.ApprovalStatus = "approved"
	result, reported := EvaluateEscapeAttempt(attempt, restrictedPermissionForTest())
	if reported || result.Classification != EscapeClassificationAuthorizedBoundaryExpansion {
		t.Fatalf("authorized result = %#v, reported=%v", result, reported)
	}
}

func TestEvaluateEscapeAttemptOutsideWorkspaceUsesPolicyRoots(t *testing.T) {
	attempt := completeEscapeAttemptForTest("open_read", "/etc/hosts", -13)
	result, reported := EvaluateEscapeAttempt(attempt, restrictedPermissionForTest())
	if !reported || result.Violation.Rule != EscapeRuleAccessOutsideWorkspace {
		t.Fatalf("outside workspace result = %#v, reported=%v", result, reported)
	}

	attempt.Target = "/workspace/project/config.json"
	if _, reported = EvaluateEscapeAttempt(attempt, restrictedPermissionForTest()); reported {
		t.Fatal("workspace path was reported as escape")
	}
}

func TestEvaluateEscapeAttemptNetworkDisabledFlagsCurlLikeConnect(t *testing.T) {
	attempt := completeEscapeAttemptForTest("connect", "198.51.100.10:443", -13)
	attempt.Category = CategoryNetwork
	result, reported := EvaluateEscapeAttempt(attempt, restrictedPermissionForTest())
	if !reported || result.Violation.Rule != EscapeRuleNetworkBoundary || result.Classification != EscapeClassificationPolicyViolationAttempt {
		t.Fatalf("network denial result = %#v, reported=%v", result, reported)
	}
}

func TestNormalizeAgentPermissionUsesProductBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		input    AgentPermissionInput
		class    PermissionClass
		boundary string
		app      bool
	}{
		{name: "claude sandbox survives bypass permission", input: AgentPermissionInput{AgentType: "claude-code", PermissionMode: "bypassPermissions", SandboxEnabled: boolPtr(true), SandboxMode: "enabled", CWD: "/workspace"}, class: PermissionRestricted, boundary: "enforced", app: true},
		{name: "openclaw docker network default", input: AgentPermissionInput{AgentType: "openclaw", Backend: "docker", SandboxMode: "all", WorkspaceAccess: "rw", CWD: "/workspace"}, class: PermissionRestricted, boundary: "enforced", app: true},
		{name: "hermes local is not a sandbox", input: AgentPermissionInput{AgentType: "hermes", Backend: "local", PermissionMode: "smart", CWD: "/workspace"}, class: PermissionRestricted, boundary: "no_isolation", app: false},
		{name: "hermes safe write root", input: AgentPermissionInput{AgentType: "hermes", Backend: "local", SafeWriteRoot: "/workspace/safe"}, class: PermissionRestricted, boundary: "enforced", app: true},
		{name: "zcode full access", input: AgentPermissionInput{AgentType: "zcode", PermissionMode: "Full Access"}, class: PermissionFullAccess, boundary: "none", app: false},
		{name: "zcode confirmation mode", input: AgentPermissionInput{AgentType: "zcode", PermissionMode: "Confirm Before Changes", CWD: "/workspace"}, class: PermissionRestricted, boundary: "enforced", app: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeAgentPermission(test.input)
			if got.Class != test.class || got.Boundary != test.boundary || got.IsDetectionApplicable() != test.app {
				t.Fatalf("permission = %#v", got)
			}
		})
	}
}

func TestHermesSafeWriteRootOnlyFlagsWrites(t *testing.T) {
	permission := NormalizeAgentPermission(AgentPermissionInput{AgentType: "hermes", Backend: "local", SafeWriteRoot: "/workspace/safe"})
	if got := permission.BoundaryRule(CategoryFile, "open_read", "/etc/hosts"); got != "" {
		t.Fatalf("read outside safe write root should not be reported: %s", got)
	}
	if got := permission.BoundaryRule(CategoryFile, "write", "/etc/hosts"); got != EscapeRuleProtectedPathWrite {
		t.Fatalf("write outside safe write root rule = %s", got)
	}
}

func TestProductsWithoutKernelSandboxDoNotTreatSensitiveSyscallsAsEscape(t *testing.T) {
	attempt := completeEscapeAttemptForTest("setns", "mnt", 0)
	for _, permission := range []EscapePermissionContext{
		NormalizeAgentPermission(AgentPermissionInput{AgentType: "hermes", Backend: "local", SafeWriteRoot: "/workspace/safe"}),
		NormalizeAgentPermission(AgentPermissionInput{AgentType: "zcode", PermissionMode: "Confirm Before Changes", CWD: "/workspace"}),
	} {
		if _, reported := EvaluateEscapeAttempt(attempt, permission); reported {
			t.Fatalf("permission %#v unexpectedly reported a kernel escape", permission)
		}
	}
}

func TestAgentPermissionBoundaryRulesRespectOpenClawWorkspaceAndDomains(t *testing.T) {
	falseValue := false
	permission := NormalizeAgentPermission(AgentPermissionInput{
		AgentType: "openclaw", Backend: "docker", SandboxMode: "all", WorkspaceAccess: "ro", CWD: "/workspace",
		NetworkAccess: &falseValue, AllowedDomains: []string{"api.example.com"},
	})
	if permission.BoundaryRule(CategoryFile, "write", "/workspace/result.txt") != EscapeRuleAccessOutsideWorkspace {
		t.Fatal("read-only workspace write should be outside the effective boundary")
	}
	if permission.BoundaryRule(CategoryNetwork, "connect", "api.example.com:443") != EscapeRuleNetworkBoundary {
		t.Fatal("network disabled should be a boundary violation")
	}
}
