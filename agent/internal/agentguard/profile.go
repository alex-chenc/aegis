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
			DefaultEscapeRules: []string{"join_external_namespace", "mount_host_path", "credential_or_capability_gain", "isolation_baseline_drift"},
			Digest:             "sha256:ac7f7259e1ea26729377e4535cbdbb2a1e2c17befdeb3965a924388acb0c2384",
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
			DefaultEscapeRules: []string{"access_container_runtime_socket", "join_external_namespace", "write_cgroupfs", "credential_or_capability_gain", "isolation_baseline_drift"},
			Digest:             "sha256:56804a5b02e48827bb944959412ee8f19d46e333257f068e0197d81245e71c4d",
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
			DefaultEscapeRules: []string{"access_container_runtime_socket", "join_external_namespace", "mount_host_path", "write_cgroupfs", "credential_or_capability_gain", "isolation_baseline_drift"},
			Digest:             "sha256:0bf30bb4daff9b86ccf4fd4fad7bc515f3fb3ed760a7b7ce6ca98f5783889524",
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
			DefaultEscapeRules: []string{"access_container_runtime_socket", "join_external_namespace", "write_cgroupfs", "credential_or_capability_gain", "isolation_baseline_drift"},
			Digest:             "sha256:e4158634ff61db23c9fa930507e5d91bb79840e94508e7ec9d4d5cd76f0e01e1",
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
			DefaultEscapeRules: []string{"access_container_runtime_socket", "join_external_namespace", "write_cgroupfs", "credential_or_capability_gain", "isolation_baseline_drift"},
			Digest:             "sha256:c02f7b4117b237dda288bb3eaf5611770f0efa0b42cb5970f916126472ecb7b1",
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
			DefaultEscapeRules: []string{"access_container_runtime_socket", "join_external_namespace", "write_cgroupfs", "credential_or_capability_gain", "isolation_baseline_drift"},
			Digest:             "sha256:7038eb7b2a4799747ebd3ec4b29b37f40c0ec44db72b362277915aa7b92141d7",
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
