package agentguard

import (
	"path/filepath"
	"strings"
)

type ProfileRegistry struct {
	profiles []AdapterProfile
}

func NewBuiltinProfileRegistry() *ProfileRegistry {
	return &ProfileRegistry{profiles: []AdapterProfile{
		{
			ProfileKey: "codex-linux", ProfileVersion: 1, AgentType: "codex", DisplayName: "Codex",
			SandboxFamily: IsolationLinuxNamespace,
			ControllerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"codex"}, CmdlineTokens: []string{"codex"}, EvidenceWeight: 60},
				{ConfigPaths: []string{".codex/config.toml"}, EvidenceWeight: 40},
			},
			WorkerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"codex-linux-sandbox", "bwrap"}, AncestorBasenames: []string{"codex"}},
				{NamespaceHelper: true, RequiredNamespaceChanges: []string{"mnt", "pid"}},
			},
			BackendDetectors: []BackendDetector{
				{Backend: "linux_namespace", Signals: []string{"codex-linux-sandbox", "bubblewrap", "namespace_tuple"}},
			},
			IsolationExpectation: map[string]any{
				"namespaces":           []string{"mnt", "pid", "user", "net"},
				"require_no_new_privs": true, "seccomp": "profile_or_filter",
				"controller_outside_worker_namespace": true,
			},
			DefaultEscapeRules: []string{"access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"},
			Digest:             "sha256:5e2058f4656ea4d7540ac1a662b4806edcc46aa9a860ac38ca65c1bb27deb629",
		},
		{
			ProfileKey: "openclaw-linux", ProfileVersion: 1, AgentType: "openclaw", DisplayName: "OpenClaw",
			SandboxFamily: IsolationLocalProcessTree,
			ControllerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"openclaw"}, CmdlineTokens: []string{"openclaw"}, EvidenceWeight: 60},
				{ConfigPaths: []string{".openclaw/config.json"}, EvidenceWeight: 40},
			},
			WorkerMatch: []ProcessMatchRule{
				{AncestorBasenames: []string{"openclaw"}, ForkDescendant: true},
				{ContainerLabels: []string{"openclaw"}, BackendRequired: "docker"},
			},
			BackendDetectors: []BackendDetector{
				{Backend: "local", Signals: []string{"sandbox_off", "local_backend"}},
				{Backend: "docker", Signals: []string{"docker_request", "container_id", "container_label", "cgroup"}},
				{Backend: "ssh", Signals: []string{"ssh_backend", "remote_execution_id"}},
				{Backend: "openshell", Signals: []string{"openshell_backend", "remote_execution_id"}},
			},
			IsolationExpectation: map[string]any{
				"local":     map[string]any{"coverage": "no_isolation"},
				"docker":    map[string]any{"family": "oci_container", "require_container_cgroup": true},
				"ssh":       map[string]any{"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
				"openshell": map[string]any{"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
			},
			DefaultEscapeRules: []string{"access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"},
			Digest:             "sha256:e6f916a2eb9b4fab6efd72f539efa7d7c9d51ab2f2d14255a55689249b9cfb79",
		},
		{
			ProfileKey: "hermes-linux", ProfileVersion: 1, AgentType: "hermes", DisplayName: "Hermes",
			SandboxFamily: IsolationLocalProcessTree,
			ControllerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"hermes"}, CmdlineTokens: []string{"hermes"}, EvidenceWeight: 60},
				{ExeBasenames: []string{"python", "python3"}, CmdlineTokens: []string{"hermes"}, ConfigPaths: []string{".hermes/config.yaml"}, EvidenceWeight: 40},
			},
			WorkerMatch: []ProcessMatchRule{
				{AncestorCmdlineTokens: []string{"hermes"}, ForkDescendant: true},
				{ContainerLabels: []string{"hermes"}, BackendRequired: "docker"},
			},
			BackendDetectors: []BackendDetector{
				{Backend: "local", Signals: []string{"terminal_local"}},
				{Backend: "docker", Signals: []string{"docker_request", "container_id", "cgroup"}},
				{Backend: "singularity", Signals: []string{"singularity_process", "namespace_tuple"}},
				{Backend: "ssh", Signals: []string{"ssh_backend", "remote_execution_id"}},
				{Backend: "modal", Signals: []string{"modal_backend", "remote_execution_id"}},
				{Backend: "daytona", Signals: []string{"daytona_backend", "remote_execution_id"}},
				{Backend: "openshell", Signals: []string{"whole_process_wrapper", "remote_execution_id"}},
			},
			IsolationExpectation: map[string]any{
				"local":                 map[string]any{"coverage": "no_isolation"},
				"docker":                map[string]any{"family": "oci_container", "require_container_cgroup": true},
				"singularity":           map[string]any{"family": "oci_container", "require_namespace_baseline": true},
				"remote":                map[string]any{"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
				"whole_process_wrapper": map[string]any{"family": "whole_process_container"},
			},
			DefaultEscapeRules: []string{"access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"},
			Digest:             "sha256:eccaf4fdc6287ff8cfb74e03c3c15aa86304d32d2f2401794d7f31b6fbfb9166",
		},
		{
			ProfileKey: "claude-code-linux", ProfileVersion: 1, AgentType: "claude-code", DisplayName: "Claude Code",
			SandboxFamily: IsolationLocalProcessTree,
			ControllerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"claude"}, CmdlineTokens: []string{"claude"}, EvidenceWeight: 60},
				{ConfigPaths: []string{".claude/settings.json"}, EvidenceWeight: 40},
			},
			WorkerMatch: []ProcessMatchRule{{AncestorBasenames: []string{"claude"}, ForkDescendant: true}},
			BackendDetectors: []BackendDetector{
				{Backend: "local", Signals: []string{"terminal_local"}},
				{Backend: "ssh", Signals: []string{"ssh_backend", "remote_execution_id"}},
			},
			IsolationExpectation: map[string]any{
				"local": map[string]any{"coverage": "no_isolation"},
				"ssh":   map[string]any{"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
			},
			DefaultEscapeRules: []string{"access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"},
			Digest:             "sha256:94eb603baadec817c6e03857064fbe809aa5da42d612d9dd7e8b486f66cb63a7",
		},
		{
			ProfileKey: "opencode-linux", ProfileVersion: 1, AgentType: "opencode", DisplayName: "OpenCode",
			SandboxFamily: IsolationLocalProcessTree,
			ControllerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"opencode"}, CmdlineTokens: []string{"opencode"}, EvidenceWeight: 60},
				{ConfigPaths: []string{".config/opencode/opencode.json"}, EvidenceWeight: 40},
			},
			WorkerMatch: []ProcessMatchRule{{AncestorBasenames: []string{"opencode"}, ForkDescendant: true}},
			BackendDetectors: []BackendDetector{
				{Backend: "local", Signals: []string{"terminal_local"}},
				{Backend: "ssh", Signals: []string{"ssh_backend", "remote_execution_id"}},
			},
			IsolationExpectation: map[string]any{
				"local": map[string]any{"coverage": "no_isolation"},
				"ssh":   map[string]any{"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
			},
			DefaultEscapeRules: []string{"access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"},
			Digest:             "sha256:b0fff61d935a97de75d5e90c658248201117608d256cd9be3f9a30d1ee3a34c2",
		},
		{
			ProfileKey: "gemini-cli-linux", ProfileVersion: 1, AgentType: "gemini-cli", DisplayName: "Gemini CLI",
			SandboxFamily: IsolationLocalProcessTree,
			ControllerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"gemini"}, CmdlineTokens: []string{"gemini"}, EvidenceWeight: 60},
				{ConfigPaths: []string{".gemini/settings.json"}, EvidenceWeight: 40},
			},
			WorkerMatch: []ProcessMatchRule{{AncestorBasenames: []string{"gemini"}, ForkDescendant: true}},
			BackendDetectors: []BackendDetector{
				{Backend: "local", Signals: []string{"terminal_local"}},
				{Backend: "ssh", Signals: []string{"ssh_backend", "remote_execution_id"}},
			},
			IsolationExpectation: map[string]any{
				"local": map[string]any{"coverage": "no_isolation"},
				"ssh":   map[string]any{"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
			},
			DefaultEscapeRules: []string{"access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"},
			Digest:             "sha256:300f72f233925ac36203a8a6d6ad4d8aa3247b93cf03474e8cede761315f5f66",
		},
		{
			ProfileKey: "zcode-linux", ProfileVersion: 1, AgentType: "zcode", DisplayName: "Zcode",
			SandboxFamily: IsolationLocalProcessTree,
			ControllerMatch: []ProcessMatchRule{
				{ExeBasenames: []string{"zcode", "zcode-cli"}, CmdlineTokens: []string{"zcode"}, EvidenceWeight: 60},
				{ConfigPaths: []string{".zcode/cli/config.json"}, EvidenceWeight: 40},
			},
			WorkerMatch: []ProcessMatchRule{{AncestorBasenames: []string{"zcode", "zcode-cli"}, ForkDescendant: true}},
			BackendDetectors: []BackendDetector{
				{Backend: "local", Signals: []string{"terminal_local"}},
				{Backend: "ssh", Signals: []string{"ssh_backend", "remote_execution_id"}},
			},
			IsolationExpectation: map[string]any{
				"local": map[string]any{"coverage": "no_isolation"},
				"ssh":   map[string]any{"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
			},
			DefaultEscapeRules: []string{"access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"},
			Digest:             "sha256:dd1e0a0d89bf1fdb6152ce92c57ef7cf460c9f49f1402b503348bba407ff8c2f",
		},
	}}
}

func (r *ProfileRegistry) Profiles() []AdapterProfile {
	out := make([]AdapterProfile, len(r.profiles))
	copy(out, r.profiles)
	return out
}

func (r *ProfileRegistry) Profile(profileKey string) (AdapterProfile, bool) {
	for _, profile := range r.profiles {
		if profile.ProfileKey == profileKey {
			return profile, true
		}
	}
	return AdapterProfile{}, false
}

func (r *ProfileRegistry) MatchWorker(profileKey string, process ProcessSnapshot) bool {
	profile, ok := r.Profile(profileKey)
	if !ok {
		return false
	}
	for _, rule := range profile.WorkerMatch {
		if len(rule.ExeBasenames) > 0 && matchesProcessRule(rule, process) {
			return true
		}
		if rule.NamespaceHelper && process.KnownHelper {
			return true
		}
	}
	return false
}

func (r *ProfileRegistry) MatchController(process ProcessSnapshot) ProfileMatch {
	var matches []ProfileMatch
	for i := range r.profiles {
		profile := &r.profiles[i]
		processEvidence := false
		configEvidence := false
		for _, rule := range profile.ControllerMatch {
			if matchesProcessRule(rule, process) {
				if len(rule.ExeBasenames) > 0 || len(rule.CmdlineTokens) > 0 {
					processEvidence = true
				}
				if len(rule.ConfigPaths) > 0 && matchesAnyMarker(process.ConfigEvidence, rule.ConfigPaths) {
					configEvidence = true
				}
			} else if len(rule.ExeBasenames) == 0 && len(rule.CmdlineTokens) == 0 &&
				len(rule.ConfigPaths) > 0 && matchesAnyMarker(process.ConfigEvidence, rule.ConfigPaths) {
				configEvidence = true
			}
		}
		if !processEvidence {
			continue
		}
		evidence := []string{"process_signature"}
		independent := configEvidence
		if configEvidence {
			evidence = append(evidence, "config_asset")
		}
		if process.KnownParent {
			evidence = append(evidence, "known_parent")
			independent = true
		}
		if process.KnownHelper {
			evidence = append(evidence, "sandbox_helper")
			independent = true
		}
		if process.ContainerLabel {
			evidence = append(evidence, "container_label")
			independent = true
		}
		confidence := ConfidenceCandidate
		if independent {
			confidence = ConfidenceConfirmed
		}
		matches = append(matches, ProfileMatch{Profile: profile, Confidence: confidence, Evidence: evidence})
	}
	if len(matches) == 0 {
		return ProfileMatch{Confidence: ConfidenceUnattributed}
	}
	if len(matches) > 1 {
		return ProfileMatch{Confidence: ConfidenceAmbiguous, Evidence: []string{"multiple_profiles"}}
	}
	return matches[0]
}

func matchesProcessRule(rule ProcessMatchRule, process ProcessSnapshot) bool {
	base := strings.ToLower(filepath.Base(process.Exe))
	argv := strings.ToLower(strings.Join(process.Argv, " "))
	exeMatch := len(rule.ExeBasenames) == 0
	for _, executable := range rule.ExeBasenames {
		executable = strings.ToLower(executable)
		if base == executable || strings.TrimSuffix(base, filepath.Ext(base)) == executable {
			exeMatch = true
		}
	}
	if !exeMatch {
		return false
	}
	argvMatch := len(rule.CmdlineTokens) == 0
	for _, marker := range rule.CmdlineTokens {
		if tokenContains(argv, marker) || strings.Contains(base, strings.ToLower(marker)) {
			argvMatch = true
		}
	}
	if !argvMatch {
		return false
	}
	if len(rule.ConfigPaths) > 0 && !matchesAnyMarker(process.ConfigEvidence, rule.ConfigPaths) {
		return false
	}
	return true
}

func tokenContains(text, marker string) bool {
	marker = strings.ToLower(marker)
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '/' || r == '.' || r == '_' || r == '-' || r == ':'
	}) {
		if token == marker {
			return true
		}
	}
	return false
}

func matchesAnyMarker(evidence, markers []string) bool {
	for _, value := range evidence {
		value = strings.ToLower(value)
		for _, marker := range markers {
			marker = strings.ToLower(marker)
			if strings.Contains(value, marker) {
				return true
			}
			if strings.HasPrefix(marker, strings.TrimSuffix(value, "/")+"/") {
				return true
			}
			base := strings.SplitN(marker, "/", 2)[0]
			if value == base || strings.HasSuffix(value, "/"+base) {
				return true
			}
		}
	}
	return false
}
