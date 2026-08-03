package agentguard

import (
	"path/filepath"
	"strings"
)

const (
	EscapeRuleJoinExternalNamespace  = "join_external_namespace"
	EscapeRuleLeaveExpectedCgroup    = "leave_expected_cgroup"
	EscapeRuleAccessRuntimeSocket    = "access_container_runtime_socket"
	EscapeRuleAccessHostProcRoot     = "access_host_proc_root"
	EscapeRuleWriteCgroupFS          = "write_cgroupfs"
	EscapeRuleMountHostSensitivePath = "mount_host_sensitive_path"
	EscapeRulePtraceExternalProcess  = "ptrace_external_process"
	EscapeRuleLoadBPFOrModule        = "load_bpf_or_module"
	EscapeRuleCapabilityEscalation   = "capability_escalation"
	EscapeRuleIsolationDrift         = "isolation_baseline_drift"
)

var runtimeSocketPaths = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/run/containerd/containerd.sock",
	"/var/run/crio/crio.sock",
	"/run/podman/podman.sock",
}

func DetectEscapeAttempt(attempt GuardAttempt) (SandboxViolation, bool) {
	diff := DiffIsolationState(attempt.Baseline, attempt.Actual)
	rule := ""
	target := filepath.Clean(attempt.Target)
	switch {
	case attempt.Operation == "setns" && namespaceTargetOutsideBaseline(attempt.TargetNamespace, attempt.Target, attempt.Baseline):
		rule = EscapeRuleJoinExternalNamespace
	case isRuntimeSocket(target) && (attempt.Operation == "connect_unix" || attempt.Category == CategoryFile):
		rule = EscapeRuleAccessRuntimeSocket
	case strings.HasPrefix(target, "/proc/1/root"):
		rule = EscapeRuleAccessHostProcRoot
	case isCgroupWrite(attempt.Operation, target):
		rule = EscapeRuleWriteCgroupFS
	case isSensitiveMount(attempt):
		rule = EscapeRuleMountHostSensitivePath
	case attempt.Operation == "ptrace" && attempt.TargetPID > 0:
		rule = EscapeRulePtraceExternalProcess
	case isKernelLoadOperation(attempt.Operation):
		rule = EscapeRuleLoadBPFOrModule
	case attempt.Operation == "setuid" && target == "argument:0" &&
		attempt.BeforeUIDVisible && attempt.BeforeUID > 0:
		rule = EscapeRuleCapabilityEscalation
	case diff.Changes["capabilities.effective_added"].Before != nil:
		rule = EscapeRuleCapabilityEscalation
	case diff.Changes["cgroup_path"].Before != nil:
		rule = EscapeRuleLeaveExpectedCgroup
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
		Baseline: attempt.Baseline, Actual: attempt.Actual, Diff: diff,
		StateChanged: diff.StateChanged, ReturnCode: attempt.ReturnCode,
		Decision: DecisionWouldDeny, Severity: violationSeverity(rule),
		EvidenceEventIDs: evidenceIDs,
	}, true
}

func namespaceTargetOutsideBaseline(kind, target string, baseline IsolationState) bool {
	parsedKind, inode, ok := parseNamespaceIdentity(target)
	if !ok {
		return false
	}
	if kind == "" {
		kind = parsedKind
	}
	expected, visible := baseline.NamespaceInodes[kind]
	return visible && expected != 0 && inode != expected
}

func isRuntimeSocket(target string) bool {
	for _, socket := range runtimeSocketPaths {
		if target == socket || strings.HasSuffix(target, socket) {
			return true
		}
	}
	return false
}

func isCgroupWrite(operation, target string) bool {
	if !strings.HasPrefix(target, "/sys/fs/cgroup") {
		return false
	}
	switch operation {
	case "write", "open_write", "create", "truncate":
		return true
	default:
		return false
	}
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
	case EscapeRuleJoinExternalNamespace, EscapeRuleAccessRuntimeSocket,
		EscapeRuleMountHostSensitivePath, EscapeRuleCapabilityEscalation:
		return "critical"
	default:
		return "high"
	}
}
