package agentguard

import (
	"path/filepath"
	"strings"
)

const (
	EscapeRuleAccessRuntimeSocket    = "access_container_runtime_socket"
	EscapeRuleProcessBoundary        = "process_boundary_operation"
	EscapeRuleAccessOutsideWorkspace = "access_outside_workspace"
	EscapeRuleNetworkBoundary        = "network_boundary_violation"
	EscapeRuleApprovalBoundary       = "approval_boundary_violation"
	EscapeRuleProtectedPathWrite     = "protected_path_write"
	EscapeRuleUnsandboxedExecution   = "unsandboxed_execution"
	EscapeRuleHostExecutionBypass    = "host_execution_bypass"
)

var runtimeSocketPaths = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/run/containerd/containerd.sock",
	"/var/run/crio/crio.sock",
	"/run/podman/podman.sock",
}

func DetectEscapeAttempt(attempt GuardAttempt) (SandboxViolation, bool) {
	rule := ""
	target := filepath.Clean(attempt.Target)
	switch {
	case attempt.Operation == "setns", attempt.Operation == "unshare":
		rule = EscapeRuleProcessBoundary
	case isRuntimeSocket(target) && (attempt.Operation == "connect_unix" || attempt.Category == CategoryFile):
		rule = EscapeRuleAccessRuntimeSocket
	case isSensitiveMount(attempt):
		rule = EscapeRuleProcessBoundary
	case attempt.Operation == "ptrace" && attempt.TargetPID > 0:
		rule = EscapeRuleProcessBoundary
	case isKernelLoadOperation(attempt.Operation):
		rule = EscapeRuleProcessBoundary
	case attempt.Operation == "setuid" && target == "argument:0" &&
		attempt.BeforeUIDVisible && attempt.BeforeUID > 0:
		rule = EscapeRuleProcessBoundary
	case attempt.Operation == "capset" || attempt.Operation == "setgid" ||
		attempt.Operation == "setresuid" || attempt.Operation == "setresgid":
		rule = EscapeRuleProcessBoundary
	}
	if rule == "" {
		return SandboxViolation{}, false
	}
	evidenceIDs := append([]string(nil), attempt.EvidenceEventIDs...)
	if attempt.EventID != "" && len(evidenceIDs) == 0 {
		evidenceIDs = append(evidenceIDs, attempt.EventID)
	}
	return SandboxViolation{
		Rule: rule, Operation: attempt.Operation, Target: RedactString(attempt.Target),
		ReturnCode: attempt.ReturnCode,
		Decision:   DecisionWouldDeny, Severity: violationSeverity(rule),
		EvidenceEventIDs: evidenceIDs,
	}, true
}

func isRuntimeSocket(target string) bool {
	for _, socket := range runtimeSocketPaths {
		if target == socket || strings.HasSuffix(target, socket) {
			return true
		}
	}
	return false
}

func isSensitiveMount(attempt GuardAttempt) bool {
	if attempt.Operation != "mount" && attempt.Operation != "pivot_root" && attempt.Operation != "chroot" {
		return false
	}
	for _, path := range []string{attempt.Target, attempt.SecondaryTarget} {
		clean := filepath.Clean(path)
		for _, prefix := range []string{"/proc", "/sys", "/dev", "/etc", "/root", "/var/run", "/run"} {
			if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
				return true
			}
		}
	}
	return false
}

func isKernelLoadOperation(operation string) bool {
	switch operation {
	case "bpf", "init_module", "finit_module", "delete_module":
		return true
	default:
		return false
	}
}

func violationSeverity(rule string) string {
	switch rule {
	case EscapeRuleAccessRuntimeSocket, EscapeRuleProcessBoundary:
		return "critical"
	default:
		return "high"
	}
}
