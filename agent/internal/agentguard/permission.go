package agentguard

import (
	"path/filepath"
	"strings"
)

// PermissionClass is the effective Codex isolation posture. It is deliberately
// separate from approval policy: an approval reviewer can change who reviews a
// command without changing the sandbox boundary.
type PermissionClass string

const (
	PermissionFullAccess PermissionClass = "full_access"
	PermissionRestricted PermissionClass = "restricted"
	PermissionUnknown    PermissionClass = "unknown"
)

const (
	EscapeClassificationNotApplicable               = "not_applicable"
	EscapeClassificationPolicyViolationAttempt      = "policy_violation_attempt"
	EscapeClassificationConfirmedEscape             = "confirmed_escape"
	EscapeClassificationAuthorizedExpansion         = "authorized_boundary_expansion"
	EscapeClassificationAuthorizedBoundaryExpansion = EscapeClassificationAuthorizedExpansion
	EscapeClassificationInsufficientEvidence        = "evidence_insufficient"
)

// EscapePermissionContext is the signed, redacted policy snapshot associated
// with a Hook session. Roots are lexical policy roots; the Agent never follows
// symlinks while deciding whether to emit an event.
type EscapePermissionContext struct {
	AgentType         string          `json:"agent_type,omitempty"`
	Backend           string          `json:"backend,omitempty"`
	Boundary          string          `json:"boundary,omitempty"`
	Class             PermissionClass `json:"class"`
	PermissionMode    string          `json:"permission_mode,omitempty"`
	SandboxMode       string          `json:"sandbox_mode,omitempty"`
	ApprovalPolicy    string          `json:"approval_policy,omitempty"`
	ApprovalStatus    string          `json:"approval_status,omitempty"`
	CWD               string          `json:"cwd,omitempty"`
	WorkspaceRoots    []string        `json:"workspace_roots,omitempty"`
	TempRoots         []string        `json:"temp_roots,omitempty"`
	NetworkAccess     *bool           `json:"network_access,omitempty"`
	SandboxEnabled    *bool           `json:"sandbox_enabled,omitempty"`
	WorkspaceAccess   string          `json:"workspace_access,omitempty"`
	AllowedDomains    []string        `json:"allowed_domains,omitempty"`
	DeniedDomains     []string        `json:"denied_domains,omitempty"`
	Elevated          bool            `json:"elevated,omitempty"`
	ApprovalRequired  bool            `json:"approval_required,omitempty"`
	SafeWriteRoot     string          `json:"safe_write_root,omitempty"`
	RemoteExecutionID string          `json:"remote_execution_id,omitempty"`
	Source            string          `json:"source,omitempty"`
	Complete          bool            `json:"complete"`
}

// AgentPermissionInput is the redacted policy metadata supplied by a native
// adapter. It intentionally contains only effective boundary settings; no
// prompts, transcripts, secrets, or command output are accepted here.
type AgentPermissionInput struct {
	AgentType         string
	Backend           string
	PermissionMode    string
	SandboxMode       string
	ApprovalPolicy    string
	ApprovalStatus    string
	CWD               string
	WorkspaceRoots    []string
	TempRoots         []string
	NetworkAccess     *bool
	SandboxEnabled    *bool
	WorkspaceAccess   string
	AllowedDomains    []string
	DeniedDomains     []string
	Elevated          bool
	ApprovalRequired  bool
	SafeWriteRoot     string
	RemoteExecutionID string
	Source            string
}

func NormalizeEscapePermission(permissionMode, sandboxMode, approvalPolicy, approvalStatus, cwd string, workspaceRoots, tempRoots []string, networkAccess *bool, source string) EscapePermissionContext {
	return NormalizeAgentPermission(AgentPermissionInput{
		AgentType: "codex", PermissionMode: permissionMode, SandboxMode: sandboxMode,
		ApprovalPolicy: approvalPolicy, ApprovalStatus: approvalStatus, CWD: cwd,
		WorkspaceRoots: workspaceRoots, TempRoots: tempRoots, NetworkAccess: networkAccess,
		Source: source,
	})
}

// NormalizeAgentPermission resolves the product-specific effective boundary.
// A permission prompt mode is not itself a sandbox: Claude bypassPermissions
// still remains restricted when its native sandbox is enabled. Conversely,
// products that explicitly report no isolation are not treated as escaped.
func NormalizeAgentPermission(input AgentPermissionInput) EscapePermissionContext {
	p := EscapePermissionContext{
		AgentType: normalizePermissionAgent(input.AgentType), Backend: strings.ToLower(strings.TrimSpace(input.Backend)),
		PermissionMode: strings.TrimSpace(input.PermissionMode), SandboxMode: strings.TrimSpace(input.SandboxMode),
		ApprovalPolicy: strings.TrimSpace(input.ApprovalPolicy), ApprovalStatus: strings.TrimSpace(input.ApprovalStatus),
		CWD: cleanPolicyPath(input.CWD), WorkspaceRoots: cleanPolicyPaths(input.WorkspaceRoots), TempRoots: cleanPolicyPaths(input.TempRoots),
		NetworkAccess: input.NetworkAccess, SandboxEnabled: input.SandboxEnabled,
		WorkspaceAccess: strings.ToLower(strings.TrimSpace(input.WorkspaceAccess)),
		AllowedDomains:  cleanPolicyDomains(input.AllowedDomains), DeniedDomains: cleanPolicyDomains(input.DeniedDomains),
		Elevated: input.Elevated, ApprovalRequired: input.ApprovalRequired,
		SafeWriteRoot: cleanPolicyPath(input.SafeWriteRoot), RemoteExecutionID: strings.TrimSpace(input.RemoteExecutionID),
		Source: strings.TrimSpace(input.Source),
	}
	if p.AgentType == "" {
		p.AgentType = agentTypeFromSource(input.Source)
	}
	if p.AgentType == "" {
		p.AgentType = "codex"
	}
	if p.Backend == "" {
		p.Backend = "local"
	}
	if p.AgentType == "claude-code" && p.SandboxEnabled == nil {
		// Claude's native sandbox is only considered present when the adapter
		// explicitly reports it; permission_mode alone is not a sandbox signal.
		if strings.EqualFold(p.SandboxMode, "disabled") || strings.EqualFold(p.SandboxMode, "off") {
			p.SandboxEnabled = boolPtrValue(false)
		}
	}
	if p.AgentType == "openclaw" && p.WorkspaceAccess == "" {
		p.WorkspaceAccess = "rw"
	}
	if p.AgentType == "hermes" && p.Backend == "" {
		p.Backend = "local"
	}
	if !p.ApprovalRequired && (p.AgentType == "zcode" || p.AgentType == "claude-code") && p.ApprovalStatus != "" && !strings.EqualFold(p.ApprovalStatus, "not_required") {
		p.ApprovalRequired = true
	}

	fullAccess := strings.EqualFold(p.PermissionMode, "full_access") ||
		strings.EqualFold(p.SandboxMode, "danger-full-access") ||
		(strings.EqualFold(p.AgentType, "zcode") && strings.EqualFold(p.PermissionMode, "full access"))
	if p.AgentType == "codex" && strings.EqualFold(p.PermissionMode, "bypassPermissions") {
		fullAccess = true
	}
	if strings.EqualFold(p.AgentType, "claude-code") && strings.EqualFold(p.PermissionMode, "bypassPermissions") {
		fullAccess = p.SandboxEnabled != nil && !*p.SandboxEnabled
	}
	if fullAccess {
		p.Class, p.Complete, p.Boundary = PermissionFullAccess, true, "none"
		return p
	}

	// There is no host boundary to escape when the product is explicitly
	// running without isolation. We retain the mode in the signed snapshot so
	// the UI explains why no finding is generated.
	if agentHasNoIsolation(p) {
		p.Class, p.Complete, p.Boundary = PermissionRestricted, true, "no_isolation"
		return p
	}
	if p.AgentType == "openclaw" && p.Backend == "ssh" && p.RemoteExecutionID == "" ||
		(p.AgentType == "hermes" && isRemoteBackend(p.Backend) && p.RemoteExecutionID == "") ||
		(p.AgentType == "claude-code" && p.Backend == "ssh" && p.RemoteExecutionID == "") ||
		(p.AgentType == "zcode" && p.Backend == "ssh" && p.RemoteExecutionID == "") {
		p.Class, p.Complete, p.Boundary = PermissionUnknown, false, "remote_unobservable"
		return p
	}

	p.Boundary = "enforced"
	switch {
	case p.PermissionMode != "" || p.SandboxMode != "" || p.ApprovalPolicy != "" || p.CWD != "" || len(p.WorkspaceRoots) > 0,
		p.AgentType == "openclaw" && (p.Backend != "" || p.WorkspaceAccess != ""),
		p.AgentType == "hermes" && (p.Backend != "local" || p.SafeWriteRoot != ""):
		p.Class, p.Complete = PermissionRestricted, true
	default:
		p.Class, p.Complete = PermissionUnknown, false
	}
	switch {
	case p.AgentType == "openclaw" && p.NetworkAccess == nil:
		// OpenClaw Docker sandbox defaults to network none; do not apply this
		// default to a local/no-isolation session (returned above).
		p.NetworkAccess = boolPtrValue(p.Backend != "docker" && p.Backend != "podman")
	case p.AgentType == "codex" && p.NetworkAccess == nil:
		p.NetworkAccess = boolPtrValue(false)
	}
	if p.Class == PermissionRestricted && len(p.WorkspaceRoots) == 0 && p.CWD != "" {
		p.WorkspaceRoots = []string{p.CWD}
	}
	if p.Class == PermissionRestricted && len(p.TempRoots) == 0 {
		p.TempRoots = []string{"/tmp", "/var/tmp"}
	}
	return p
}

func normalizePermissionAgent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "claude" || value == "claude_code" {
		return "claude-code"
	}
	return value
}

func agentTypeFromSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.Contains(source, "claude"):
		return "claude-code"
	case strings.Contains(source, "openclaw"):
		return "openclaw"
	case strings.Contains(source, "hermes"):
		return "hermes"
	case strings.Contains(source, "zcode"):
		return "zcode"
	case strings.Contains(source, "codex"):
		return "codex"
	default:
		return ""
	}
}

func isRemoteBackend(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "ssh", "modal", "daytona", "openshell", "vercel_sandbox", "vercel":
		return true
	}
	return false
}

func agentHasNoIsolation(p EscapePermissionContext) bool {
	if p.SandboxEnabled != nil && !*p.SandboxEnabled {
		return true
	}
	switch p.AgentType {
	case "hermes":
		return (p.Backend == "local" || p.Backend == "") && p.SafeWriteRoot == ""
	case "openclaw":
		return (p.Backend == "local" || p.Backend == "") && (p.SandboxMode == "" || strings.EqualFold(p.SandboxMode, "off"))
	case "claude-code":
		return p.Backend == "local" && (p.SandboxMode == "" || strings.EqualFold(p.SandboxMode, "disabled") || strings.EqualFold(p.SandboxMode, "off"))
	}
	return false
}

func cleanPolicyDomains(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(strings.ToLower(value)); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func (p EscapePermissionContext) IsFullAccess() bool { return p.Class == PermissionFullAccess }
func (p EscapePermissionContext) IsRestricted() bool {
	return p.Class == PermissionRestricted && p.Complete
}
func (p EscapePermissionContext) IsKnown() bool { return p.Class != PermissionUnknown && p.Complete }
func (p EscapePermissionContext) IsDetectionApplicable() bool {
	return p.IsKnown() && !p.IsFullAccess() && p.Boundary != "no_isolation" && p.Boundary != "remote_unobservable"
}

func (p EscapePermissionContext) AllowsTarget(category Category, operation, target string) bool {
	if p.IsFullAccess() {
		return true
	}
	if !p.IsRestricted() {
		return false
	}
	if category == CategoryNetwork {
		if p.NetworkAccess != nil && !*p.NetworkAccess {
			return false
		}
		host := networkHost(target)
		for _, denied := range p.DeniedDomains {
			if domainMatches(host, denied) {
				return false
			}
		}
		if len(p.AllowedDomains) > 0 {
			for _, allowed := range p.AllowedDomains {
				if domainMatches(host, allowed) {
					return true
				}
			}
			return false
		}
		return true
	}
	if category != CategoryFile {
		return true
	}
	if p.AgentType == "openclaw" {
		switch p.WorkspaceAccess {
		case "none":
			return false
		case "ro":
			if isWriteOperation(operation) {
				return false
			}
		}
	}
	clean := cleanPolicyPath(target)
	if clean == "" || !filepath.IsAbs(clean) {
		return true
	}
	if isRuntimeSocket(clean) || strings.HasPrefix(clean, "/proc/1/root") || strings.HasPrefix(clean, "/sys/fs/cgroup") {
		return false
	}
	for _, root := range append(append([]string{}, p.WorkspaceRoots...), p.TempRoots...) {
		if pathWithinRoot(clean, root) {
			return true
		}
	}
	return false
}

func (p EscapePermissionContext) BoundaryRule(category Category, operation, target string) string {
	if !p.IsDetectionApplicable() {
		return ""
	}
	if p.AgentType == "zcode" && p.ApprovalRequired && !strings.EqualFold(p.ApprovalStatus, "approved") && (category == CategoryProcess || category == CategoryFile || category == CategoryNetwork) {
		return EscapeRuleApprovalBoundary
	}
	if p.AgentType == "hermes" && p.Backend == "local" && p.SafeWriteRoot != "" {
		if category == CategoryFile && isWriteOperation(operation) && !pathWithinRoot(cleanPolicyPath(target), p.SafeWriteRoot) {
			return EscapeRuleProtectedPathWrite
		}
		return ""
	}
	if category == CategoryNetwork && !p.AllowsTarget(category, operation, target) {
		return EscapeRuleNetworkBoundary
	}
	if category == CategoryFile && !p.AllowsTarget(category, operation, target) && isBoundaryFileOperation(operation) {
		return EscapeRuleAccessOutsideWorkspace
	}
	if p.AgentType == "hermes" && p.SafeWriteRoot != "" && category == CategoryFile && isWriteOperation(operation) && !pathWithinRoot(cleanPolicyPath(target), p.SafeWriteRoot) {
		return EscapeRuleProtectedPathWrite
	}
	return ""
}

func isWriteOperation(operation string) bool {
	switch operation {
	case "open_write", "write", "create", "truncate", "unlink", "rename":
		return true
	}
	return false
}

func networkHost(target string) string {
	host := strings.ToLower(strings.TrimSpace(target))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "tcp://")
	if index := strings.IndexByte(host, '/'); index >= 0 {
		host = host[:index]
	}
	if index := strings.LastIndexByte(host, ':'); index > 0 {
		host = host[:index]
	}
	return host
}

func domainMatches(host, pattern string) bool {
	pattern = strings.TrimSpace(strings.ToLower(pattern))
	pattern = strings.TrimPrefix(pattern, "*.")
	return host == pattern || strings.HasSuffix(host, "."+pattern)
}

func cleanPolicyPaths(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if clean := cleanPolicyPath(value); clean != "" && filepath.IsAbs(clean) {
			result = append(result, clean)
		}
	}
	return result
}

func cleanPolicyPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}

func pathWithinRoot(path, root string) bool {
	root = cleanPolicyPath(root)
	path = cleanPolicyPath(path)
	return root != "" && (path == root || strings.HasPrefix(path, root+string(filepath.Separator)))
}

func isBoundaryFileOperation(operation string) bool {
	switch operation {
	case "read", "open", "open_read", "open_write", "write", "create", "truncate", "unlink", "rename", "mount", "pivot_root", "chroot", "connect_unix":
		return true
	default:
		return false
	}
}

type EscapeEvaluation struct {
	Violation      SandboxViolation        `json:"violation"`
	Classification string                  `json:"classification"`
	Permission     EscapePermissionContext `json:"permission"`
	Reason         string                  `json:"reason,omitempty"`
	EvidenceReady  bool                    `json:"evidence_ready"`
}

// EvaluateEscapeAttempt applies the permission-first contract. The bool is
// true only when a user-visible escape finding should be emitted.
func EvaluateEscapeAttempt(attempt GuardAttempt, permission EscapePermissionContext) (EscapeEvaluation, bool) {
	result := EscapeEvaluation{Classification: EscapeClassificationInsufficientEvidence, Permission: permission}
	if permission.IsFullAccess() {
		result.Classification = EscapeClassificationNotApplicable
		result.Reason = "full_access_no_sandbox_boundary"
		return result, false
	}
	if permission.Boundary == "no_isolation" {
		result.Classification = EscapeClassificationNotApplicable
		result.Reason = "agent_reports_no_isolation_boundary"
		return result, false
	}
	if permission.Boundary == "remote_unobservable" {
		result.Reason = "remote_execution_sensor_unavailable"
		return result, false
	}
	if !permission.IsKnown() {
		result.Reason = "permission_snapshot_unavailable"
		return result, false
	}
	if attempt.ApprovalStatus == "approved" || permission.ApprovalStatus == "approved" {
		result.Classification = EscapeClassificationAuthorizedExpansion
		result.Reason = "explicit_approval"
		return result, false
	}
	if attempt.Outcome == OutcomeUnknown {
		result.Reason = "ebpf_execution_outcome_unavailable"
		return result, false
	}
	var violation SandboxViolation
	var detected bool
	if permission.allowsGenericEscapeRules() {
		violation, detected = DetectEscapeAttempt(attempt)
	}
	if permission.AgentType == "openclaw" && permission.Elevated && (attempt.Category == CategoryProcess || attempt.Category == CategoryFile || attempt.Category == CategoryNetwork) {
		violation = policyBoundaryViolation(attempt, EscapeRuleHostExecutionBypass)
		detected = true
	}
	if !detected {
		if rule := permission.BoundaryRule(attempt.Category, attempt.Operation, attempt.Target); rule != "" {
			violation = policyBoundaryViolation(attempt, rule)
			detected = true
		}
	}
	if !detected {
		result.Classification = EscapeClassificationNotApplicable
		result.Reason = "within_effective_policy"
		return result, false
	}
	if !attempt.HookMatched || !attempt.ProcessMatched {
		result.Reason = "hook_pid_correlation_incomplete"
		return result, false
	}
	if attempt.Outcome == OutcomeFailed || attempt.ReturnCode < 0 {
		violation.Decision = DecisionAlert
		violation.Severity = "medium"
		result.Violation = violation
		result.Classification = EscapeClassificationPolicyViolationAttempt
		result.EvidenceReady = true
		result.Reason = "boundary_request_rejected"
		return result, true
	}
	violation.Decision = DecisionAlert
	result.Violation = violation
	result.Classification = EscapeClassificationConfirmedEscape
	result.EvidenceReady = true
	result.Reason = "boundary_action_succeeded_after_hook_pid_correlation"
	return result, true
}

func (p EscapePermissionContext) allowsGenericEscapeRules() bool {
	switch p.AgentType {
	case "zcode":
		// Zcode documents confirmation semantics, not a kernel/filesystem
		// sandbox. Only an explicit approval-boundary signal is actionable.
		return false
	case "hermes":
		return p.Backend != "local" && p.Backend != ""
	case "claude-code":
		return p.SandboxEnabled != nil && *p.SandboxEnabled
	case "openclaw":
		return p.Backend != "local" && p.Backend != ""
	default:
		return true
	}
}

func boolPtrValue(value bool) *bool { return &value }

func policyBoundaryViolation(attempt GuardAttempt, rule string) SandboxViolation {
	ids := append([]string(nil), attempt.EvidenceEventIDs...)
	if attempt.EventID != "" && len(ids) == 0 {
		ids = append(ids, attempt.EventID)
	}
	return SandboxViolation{Rule: rule, Operation: attempt.Operation, Target: RedactString(attempt.Target), ReturnCode: attempt.ReturnCode, Decision: DecisionAlert, Severity: "medium", EvidenceEventIDs: ids}
}
