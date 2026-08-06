package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	AgentGuardRuleIDSensitiveDirectory  = "62000000-0000-4000-8000-000000000001"
	AgentGuardRuleIDExternalNetwork     = "62000000-0000-4000-8000-000000000002"
	AgentGuardRuleIDFileCreation        = "62000000-0000-4000-8000-000000000003"
	AgentGuardRuleIDSensitiveCommand    = "62000000-0000-4000-8000-000000000004"
	AgentGuardRuleIDPrivilegeEscalation = "62000000-0000-4000-8000-000000000005"

	AgentGuardProfileIDCodexLinux      = "62000000-0000-4000-8000-000000000101"
	AgentGuardProfileIDOpenClawLinux   = "62000000-0000-4000-8000-000000000102"
	AgentGuardProfileIDHermesLinux     = "62000000-0000-4000-8000-000000000103"
	AgentGuardProfileIDClaudeCodeLinux = "62000000-0000-4000-8000-000000000104"
	AgentGuardProfileIDOpenCodeLinux   = "62000000-0000-4000-8000-000000000105"
	AgentGuardProfileIDGeminiCLILinux  = "62000000-0000-4000-8000-000000000106"
	AgentGuardProfileIDZcodeLinux      = "62000000-0000-4000-8000-000000000107"
)

var builtinAgentBehaviorRules = []AgentBehaviorRuleDefinition{
	{
		ID:                uuid.MustParse(AgentGuardRuleIDSensitiveDirectory),
		RuleKey:           AgentGuardRuleKeySensitiveDirectory,
		RuleVersion:       1,
		Name:              "操作敏感目录",
		Description:       "检测已归属智能体进程对凭据、权限策略、持久化、安全控制和容器控制资源的访问或修改。",
		Source:            "builtin",
		Engine:            "agent_and_dc",
		Categories:        jsonValue(`["file"]`),
		DefaultEnabled:    true,
		DefaultSeverity:   "medium",
		DefaultAction:     "alert",
		RecommendedAction: "alert",
		ParametersSchema: jsonValue(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"resource_groups":{"type":"array","uniqueItems":true,"items":{"enum":["credential","privilege_policy","cloud_or_cluster_credential","persistence","security_control","container_control"]}},
				"operations":{"type":"array","uniqueItems":true,"items":{"enum":["open_intent","read_observed","write","create","truncate","delete","rename","chmod","chown","execute"]}}
			}
		}`),
		DefaultParameters: jsonValue(`{
			"resource_groups":["credential","privilege_policy","cloud_or_cluster_credential","persistence","security_control","container_control"],
			"operations":["open_intent","read_observed","write","create","truncate","delete","rename","chmod","chown","execute"]
		}`),
		RequiredEvidence: jsonValue(`["actor.pid","actor.ppid","actor.start_ticks","operation","resource.resolved_path","resource.classification","outcome"]`),
		AllowConditions:  jsonValue(`["trusted_process_digest","policy_exception","approved_change_window"]`),
		MITRE:            jsonValue(`["T1005","T1543","T1552"]`),
		Immutable:        true,
		Digest:           "sha256:e9a7f8b0dda7c742557bbc1a0551ea4caeb0329973ec1c24f7751b4cd2902a82",
	},
	{
		ID:                uuid.MustParse(AgentGuardRuleIDExternalNetwork),
		RuleKey:           AgentGuardRuleKeyExternalNetwork,
		RuleVersion:       1,
		Name:              "外部网络连接",
		Description:       "检测已归属智能体进程主动连接非本机、非内网且不在管理员信任范围内的目标。",
		Source:            "builtin",
		Engine:            "agent_and_dc",
		Categories:        jsonValue(`["network"]`),
		DefaultEnabled:    true,
		DefaultSeverity:   "medium",
		DefaultAction:     "alert",
		RecommendedAction: "alert",
		ParametersSchema: jsonValue(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"trusted_cidrs":{"type":"array","uniqueItems":true,"items":{"type":"string","maxLength":64}},
				"trusted_domains":{"type":"array","uniqueItems":true,"items":{"type":"string","maxLength":253}},
				"trusted_ports":{"type":"array","uniqueItems":true,"items":{"type":"integer","minimum":1,"maximum":65535}}
			}
		}`),
		DefaultParameters: jsonValue(`{"trusted_cidrs":[],"trusted_domains":[],"trusted_ports":[]}`),
		RequiredEvidence:  jsonValue(`["actor.pid","actor.ppid","actor.start_ticks","network.direction","network.destination_ip","network.destination_port","network.protocol","outcome"]`),
		AllowConditions:   jsonValue(`["loopback_or_link_local","private_or_cluster_network","trusted_destination","policy_exception"]`),
		MITRE:             jsonValue(`["T1041","T1071"]`),
		Immutable:         true,
		Digest:            "sha256:5852cf43c0be2ddc21e83c8c12fb898ac2aae47bc0d7bff2a5246d4d2436e613",
	},
	{
		ID:                uuid.MustParse(AgentGuardRuleIDFileCreation),
		RuleKey:           AgentGuardRuleKeyFileCreation,
		RuleVersion:       1,
		Name:              "文件生成",
		Description:       "记录已归属智能体进程成功创建此前不存在的文件，并区分失败的创建意图。",
		Source:            "builtin",
		Engine:            "agent_and_dc",
		Categories:        jsonValue(`["file"]`),
		DefaultEnabled:    true,
		DefaultSeverity:   "low",
		DefaultAction:     "audit",
		RecommendedAction: "alert",
		ParametersSchema: jsonValue(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"alert_on_executable":{"type":"boolean"},
				"alert_on_hidden":{"type":"boolean"},
				"hash_max_bytes":{"type":"integer","minimum":0,"maximum":104857600}
			}
		}`),
		DefaultParameters: jsonValue(`{"alert_on_executable":true,"alert_on_hidden":true,"hash_max_bytes":10485760}`),
		RequiredEvidence:  jsonValue(`["actor.pid","actor.ppid","actor.start_ticks","operation","resource.inode_created","resource.resolved_path","outcome"]`),
		AllowConditions:   jsonValue(`["workspace_low_risk_file","policy_exception"]`),
		MITRE:             jsonValue(`["T1105","T1204"]`),
		Immutable:         true,
		Digest:            "sha256:b066e0b452fb7749f9afb49e8a6b918de1285ccef912f50680795a4bc110e03e",
	},
	{
		ID:                uuid.MustParse(AgentGuardRuleIDSensitiveCommand),
		RuleKey:           AgentGuardRuleKeySensitiveCommand,
		RuleVersion:       1,
		Name:              "敏感命令执行",
		Description:       "检测具有网络传输、提权、权限变更、隔离控制、持久化、破坏或防御规避能力的命令执行。",
		Source:            "builtin",
		Engine:            "agent_and_dc",
		Categories:        jsonValue(`["process"]`),
		DefaultEnabled:    true,
		DefaultSeverity:   "medium",
		DefaultAction:     "alert",
		RecommendedAction: "alert",
		ParametersSchema: jsonValue(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"command_categories":{"type":"array","uniqueItems":true,"items":{"enum":["network_transfer","privilege","permission_change","namespace_mount","account_persistence","destructive","security_control"]}},
				"require_resolved_executable":{"type":"boolean"}
			}
		}`),
		DefaultParameters: jsonValue(`{
			"command_categories":["network_transfer","privilege","permission_change","namespace_mount","account_persistence","destructive","security_control"],
			"require_resolved_executable":false
		}`),
		RequiredEvidence: jsonValue(`["actor.pid","actor.ppid","actor.start_ticks","process.executable","process.argv","process.cwd","outcome"]`),
		AllowConditions:  jsonValue(`["trusted_process_digest","policy_exception","approved_change_window"]`),
		MITRE:            jsonValue(`["T1059","T1105","T1548","T1562"]`),
		Immutable:        true,
		Digest:           "sha256:43e4e365124e4d895a27f8267e8ab424f0482f121794a812aea946231520e130",
	},
	{
		ID:                uuid.MustParse(AgentGuardRuleIDPrivilegeEscalation),
		RuleKey:           AgentGuardRuleKeyPrivilegeEscalation,
		RuleVersion:       1,
		Name:              "提权行为",
		Description:       "检测智能体进程尝试或成功获得高于基线的 UID、GID 或 capability，并区分 attempted、succeeded 与 inconclusive。",
		Source:            "builtin",
		Engine:            "agent_and_dc",
		Categories:        jsonValue(`["identity","process"]`),
		DefaultEnabled:    true,
		DefaultSeverity:   "high",
		DefaultAction:     "alert",
		RecommendedAction: "alert",
		ParametersSchema: jsonValue(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"alert_on_failed_attempt":{"type":"boolean"},
				"host_root_severity":{"enum":["high","critical"]},
				"unexpected_capability_severity":{"enum":["high","critical"]}
			}
		}`),
		DefaultParameters: jsonValue(`{"alert_on_failed_attempt":true,"host_root_severity":"critical","unexpected_capability_severity":"high"}`),
		RequiredEvidence:  jsonValue(`["actor.pid","actor.ppid","actor.start_ticks","identity.before","identity.after","identity.user_namespace","outcome"]`),
		AllowConditions:   jsonValue(`["profile_expected_identity_transition","container_user_namespace_root","policy_exception"]`),
		MITRE:             jsonValue(`["T1068","T1548"]`),
		Immutable:         true,
		Digest:            "sha256:63ce19628fc8285ded19f9609ec93770341c14e24e47680e95aff3cec4d775f1",
	},
}

var builtinAgentGuardProfiles = []AgentGuardAdapterProfile{
	{
		ID:             uuid.MustParse(AgentGuardProfileIDCodexLinux),
		ProfileKey:     AgentGuardProfileKeyCodexLinux,
		ProfileVersion: 1,
		AgentType:      "codex",
		DisplayName:    "Codex",
		Source:         "builtin",
		SandboxFamily:  "linux_namespace",
		ControllerMatch: jsonValue(`[
			{"exe_basenames":["codex"],"cmdline_tokens":["codex"],"evidence_weight":60},
			{"config_paths":[".codex/config.toml"],"evidence_weight":40}
		]`),
		WorkerMatch: jsonValue(`[
			{"exe_basenames":["codex-linux-sandbox","bwrap"],"ancestor_basenames":["codex"]},
			{"namespace_helper":true,"required_namespace_changes":["mnt","pid"]}
		]`),
		BackendDetectors: jsonValue(`[
			{"backend":"linux_namespace","signals":["codex-linux-sandbox","bubblewrap","namespace_tuple"]}
		]`),
		IsolationExpectation: jsonValue(`{
			"namespaces":["mnt","pid","user","net"],
			"require_no_new_privs":true,
			"seccomp":"profile_or_filter",
			"controller_outside_worker_namespace":true
		}`),
		DefaultEscapeRules: jsonValue(`["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]`),
		Digest:             "sha256:5e2058f4656ea4d7540ac1a662b4806edcc46aa9a860ac38ca65c1bb27deb629",
		Enabled:            true,
	},
	{
		ID:             uuid.MustParse(AgentGuardProfileIDOpenClawLinux),
		ProfileKey:     AgentGuardProfileKeyOpenClawLinux,
		ProfileVersion: 1,
		AgentType:      "openclaw",
		DisplayName:    "OpenClaw",
		Source:         "builtin",
		SandboxFamily:  "local_process_tree",
		ControllerMatch: jsonValue(`[
			{"exe_basenames":["openclaw"],"cmdline_tokens":["openclaw"],"evidence_weight":60},
			{"config_paths":[".openclaw/config.json"],"evidence_weight":40}
		]`),
		WorkerMatch: jsonValue(`[
			{"ancestor_basenames":["openclaw"],"fork_descendant":true},
			{"container_labels":["openclaw"],"backend_required":"docker"}
		]`),
		BackendDetectors: jsonValue(`[
			{"backend":"local","signals":["sandbox_off","local_backend"]},
			{"backend":"docker","signals":["docker_request","container_id","container_label","cgroup"]},
			{"backend":"ssh","signals":["ssh_backend","remote_execution_id"]},
			{"backend":"openshell","signals":["openshell_backend","remote_execution_id"]}
		]`),
		IsolationExpectation: jsonValue(`{
			"local":{"coverage":"no_isolation"},
			"docker":{"family":"oci_container","require_container_cgroup":true},
			"ssh":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"},
			"openshell":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"}
		}`),
		DefaultEscapeRules: jsonValue(`["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]`),
		Digest:             "sha256:e6f916a2eb9b4fab6efd72f539efa7d7c9d51ab2f2d14255a55689249b9cfb79",
		Enabled:            true,
	},
	{
		ID:             uuid.MustParse(AgentGuardProfileIDHermesLinux),
		ProfileKey:     AgentGuardProfileKeyHermesLinux,
		ProfileVersion: 1,
		AgentType:      "hermes",
		DisplayName:    "Hermes",
		Source:         "builtin",
		SandboxFamily:  "local_process_tree",
		ControllerMatch: jsonValue(`[
			{"exe_basenames":["hermes"],"cmdline_tokens":["hermes"],"evidence_weight":60},
			{"exe_basenames":["python","python3"],"cmdline_tokens":["hermes"],"config_paths":[".hermes/config.yaml"],"evidence_weight":40}
		]`),
		WorkerMatch: jsonValue(`[
			{"ancestor_cmdline_tokens":["hermes"],"fork_descendant":true},
			{"container_labels":["hermes"],"backend_required":"docker"}
		]`),
		BackendDetectors: jsonValue(`[
			{"backend":"local","signals":["terminal_local"]},
			{"backend":"docker","signals":["docker_request","container_id","cgroup"]},
			{"backend":"singularity","signals":["singularity_process","namespace_tuple"]},
			{"backend":"ssh","signals":["ssh_backend","remote_execution_id"]},
			{"backend":"modal","signals":["modal_backend","remote_execution_id"]},
			{"backend":"daytona","signals":["daytona_backend","remote_execution_id"]},
			{"backend":"openshell","signals":["whole_process_wrapper","remote_execution_id"]}
		]`),
		IsolationExpectation: jsonValue(`{
			"local":{"coverage":"no_isolation"},
			"docker":{"family":"oci_container","require_container_cgroup":true},
			"singularity":{"family":"oci_container","require_namespace_baseline":true},
			"remote":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"},
			"whole_process_wrapper":{"family":"whole_process_container"}
		}`),
		DefaultEscapeRules: jsonValue(`["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]`),
		Digest:             "sha256:eccaf4fdc6287ff8cfb74e03c3c15aa86304d32d2f2401794d7f31b6fbfb9166",
		Enabled:            true,
	},
	{
		ID:             uuid.MustParse(AgentGuardProfileIDClaudeCodeLinux),
		ProfileKey:     AgentGuardProfileKeyClaudeCodeLinux,
		ProfileVersion: 1,
		AgentType:      "claude-code",
		DisplayName:    "Claude Code",
		Source:         "builtin",
		SandboxFamily:  "local_process_tree",
		ControllerMatch: jsonValue(`[
			{"exe_basenames":["claude"],"cmdline_tokens":["claude"],"evidence_weight":60},
			{"config_paths":[".claude/settings.json"],"evidence_weight":40}
		]`),
		WorkerMatch: jsonValue(`[
			{"ancestor_basenames":["claude"],"fork_descendant":true}
		]`),
		BackendDetectors: jsonValue(`[
			{"backend":"local","signals":["terminal_local"]},
			{"backend":"ssh","signals":["ssh_backend","remote_execution_id"]}
		]`),
		IsolationExpectation: jsonValue(`{
			"local":{"coverage":"no_isolation"},
			"ssh":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"}
		}`),
		DefaultEscapeRules: jsonValue(`["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]`),
		Digest:             "sha256:94eb603baadec817c6e03857064fbe809aa5da42d612d9dd7e8b486f66cb63a7",
		Enabled:            true,
	},
	{
		ID:             uuid.MustParse(AgentGuardProfileIDOpenCodeLinux),
		ProfileKey:     AgentGuardProfileKeyOpenCodeLinux,
		ProfileVersion: 1,
		AgentType:      "opencode",
		DisplayName:    "OpenCode",
		Source:         "builtin",
		SandboxFamily:  "local_process_tree",
		ControllerMatch: jsonValue(`[
			{"exe_basenames":["opencode"],"cmdline_tokens":["opencode"],"evidence_weight":60},
			{"config_paths":[".config/opencode/opencode.json"],"evidence_weight":40}
		]`),
		WorkerMatch: jsonValue(`[
			{"ancestor_basenames":["opencode"],"fork_descendant":true}
		]`),
		BackendDetectors: jsonValue(`[
			{"backend":"local","signals":["terminal_local"]},
			{"backend":"ssh","signals":["ssh_backend","remote_execution_id"]}
		]`),
		IsolationExpectation: jsonValue(`{
			"local":{"coverage":"no_isolation"},
			"ssh":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"}
		}`),
		DefaultEscapeRules: jsonValue(`["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]`),
		Digest:             "sha256:b0fff61d935a97de75d5e90c658248201117608d256cd9be3f9a30d1ee3a34c2",
		Enabled:            true,
	},
	{
		ID:             uuid.MustParse(AgentGuardProfileIDGeminiCLILinux),
		ProfileKey:     AgentGuardProfileKeyGeminiCLILinux,
		ProfileVersion: 1,
		AgentType:      "gemini-cli",
		DisplayName:    "Gemini CLI",
		Source:         "builtin",
		SandboxFamily:  "local_process_tree",
		ControllerMatch: jsonValue(`[
			{"exe_basenames":["gemini"],"cmdline_tokens":["gemini"],"evidence_weight":60},
			{"config_paths":[".gemini/settings.json"],"evidence_weight":40}
		]`),
		WorkerMatch: jsonValue(`[
			{"ancestor_basenames":["gemini"],"fork_descendant":true}
		]`),
		BackendDetectors: jsonValue(`[
			{"backend":"local","signals":["terminal_local"]},
			{"backend":"ssh","signals":["ssh_backend","remote_execution_id"]}
		]`),
		IsolationExpectation: jsonValue(`{
			"local":{"coverage":"no_isolation"},
			"ssh":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"}
		}`),
		DefaultEscapeRules: jsonValue(`["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]`),
		Digest:             "sha256:300f72f233925ac36203a8a6d6ad4d8aa3247b93cf03474e8cede761315f5f66",
		Enabled:            true,
	},
	{
		ID:             uuid.MustParse(AgentGuardProfileIDZcodeLinux),
		ProfileKey:     AgentGuardProfileKeyZcodeLinux,
		ProfileVersion: 1,
		AgentType:      "zcode",
		DisplayName:    "Zcode",
		Source:         "builtin",
		SandboxFamily:  "local_process_tree",
		ControllerMatch: jsonValue(`[
			{"exe_basenames":["zcode","zcode-cli"],"cmdline_tokens":["zcode"],"evidence_weight":60},
			{"config_paths":[".zcode/cli/config.json"],"evidence_weight":40}
		]`),
		WorkerMatch: jsonValue(`[
			{"ancestor_basenames":["zcode","zcode-cli"],"fork_descendant":true}
		]`),
		BackendDetectors: jsonValue(`[
			{"backend":"local","signals":["terminal_local"]},
			{"backend":"ssh","signals":["ssh_backend","remote_execution_id"]}
		]`),
		IsolationExpectation: jsonValue(`{
			"local":{"coverage":"no_isolation"},
			"ssh":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"}
		}`),
		DefaultEscapeRules: jsonValue(`["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]`),
		Digest:             "sha256:dd1e0a0d89bf1fdb6152ce92c57ef7cf460c9f49f1402b503348bba407ff8c2f",
		Enabled:            true,
	},
}

// BuiltinAgentBehaviorRuleManifest returns a deep copy so callers cannot
// mutate the process-wide immutable manifest.
func BuiltinAgentBehaviorRuleManifest() []AgentBehaviorRuleDefinition {
	var cloned []AgentBehaviorRuleDefinition
	cloneManifest(builtinAgentBehaviorRules, &cloned)
	return cloned
}

// BuiltinAgentGuardProfileManifest returns a deep copy so callers cannot
// mutate the process-wide immutable manifest.
func BuiltinAgentGuardProfileManifest() []AgentGuardAdapterProfile {
	var cloned []AgentGuardAdapterProfile
	cloneManifest(builtinAgentGuardProfiles, &cloned)
	return cloned
}

func CalculateAgentBehaviorRuleDigest(rule AgentBehaviorRuleDefinition) (string, error) {
	categories, err := canonicalJSONField(rule.Categories)
	if err != nil {
		return "", fmt.Errorf("categories: %w", err)
	}
	parametersSchema, err := canonicalJSONField(rule.ParametersSchema)
	if err != nil {
		return "", fmt.Errorf("parameters_schema: %w", err)
	}
	defaultParameters, err := canonicalJSONField(rule.DefaultParameters)
	if err != nil {
		return "", fmt.Errorf("default_parameters: %w", err)
	}
	requiredEvidence, err := canonicalJSONField(rule.RequiredEvidence)
	if err != nil {
		return "", fmt.Errorf("required_evidence: %w", err)
	}
	allowConditions, err := canonicalJSONField(rule.AllowConditions)
	if err != nil {
		return "", fmt.Errorf("allow_conditions: %w", err)
	}
	mitre, err := canonicalJSONField(rule.MITRE)
	if err != nil {
		return "", fmt.Errorf("mitre: %w", err)
	}
	return sha256Digest(struct {
		RuleKey           string `json:"rule_key"`
		RuleVersion       int64  `json:"rule_version"`
		Name              string `json:"name"`
		Description       string `json:"description"`
		Source            string `json:"source"`
		Engine            string `json:"engine"`
		Categories        any    `json:"categories"`
		DefaultEnabled    bool   `json:"default_enabled"`
		DefaultSeverity   string `json:"default_severity"`
		DefaultAction     string `json:"default_action"`
		RecommendedAction string `json:"recommended_action"`
		ParametersSchema  any    `json:"parameters_schema"`
		DefaultParameters any    `json:"default_parameters"`
		RequiredEvidence  any    `json:"required_evidence"`
		AllowConditions   any    `json:"allow_conditions"`
		MITRE             any    `json:"mitre"`
		Immutable         bool   `json:"immutable"`
	}{
		RuleKey: rule.RuleKey, RuleVersion: rule.RuleVersion, Name: rule.Name,
		Description: rule.Description, Source: rule.Source, Engine: rule.Engine,
		Categories: categories, DefaultEnabled: rule.DefaultEnabled,
		DefaultSeverity: rule.DefaultSeverity, DefaultAction: rule.DefaultAction,
		RecommendedAction: rule.RecommendedAction, ParametersSchema: parametersSchema,
		DefaultParameters: defaultParameters, RequiredEvidence: requiredEvidence,
		AllowConditions: allowConditions, MITRE: mitre, Immutable: rule.Immutable,
	})
}

func CalculateAgentGuardProfileDigest(profile AgentGuardAdapterProfile) (string, error) {
	controllerMatch, err := canonicalJSONField(profile.ControllerMatch)
	if err != nil {
		return "", fmt.Errorf("controller_match: %w", err)
	}
	workerMatch, err := canonicalJSONField(profile.WorkerMatch)
	if err != nil {
		return "", fmt.Errorf("worker_match: %w", err)
	}
	backendDetectors, err := canonicalJSONField(profile.BackendDetectors)
	if err != nil {
		return "", fmt.Errorf("backend_detectors: %w", err)
	}
	isolationExpectation, err := canonicalJSONField(profile.IsolationExpectation)
	if err != nil {
		return "", fmt.Errorf("isolation_expectation: %w", err)
	}
	defaultEscapeRules, err := canonicalJSONField(profile.DefaultEscapeRules)
	if err != nil {
		return "", fmt.Errorf("default_escape_rules: %w", err)
	}
	return sha256Digest(struct {
		ProfileKey           string `json:"profile_key"`
		ProfileVersion       int64  `json:"profile_version"`
		AgentType            string `json:"agent_type"`
		DisplayName          string `json:"display_name"`
		Source               string `json:"source"`
		SandboxFamily        string `json:"sandbox_family"`
		ControllerMatch      any    `json:"controller_match"`
		WorkerMatch          any    `json:"worker_match"`
		BackendDetectors     any    `json:"backend_detectors"`
		IsolationExpectation any    `json:"isolation_expectation"`
		DefaultEscapeRules   any    `json:"default_escape_rules"`
		Enabled              bool   `json:"enabled"`
	}{
		ProfileKey: profile.ProfileKey, ProfileVersion: profile.ProfileVersion,
		AgentType: profile.AgentType, DisplayName: profile.DisplayName, Source: profile.Source,
		SandboxFamily: profile.SandboxFamily, ControllerMatch: controllerMatch,
		WorkerMatch: workerMatch, BackendDetectors: backendDetectors,
		IsolationExpectation: isolationExpectation, DefaultEscapeRules: defaultEscapeRules,
		Enabled: profile.Enabled,
	})
}

func VerifyBuiltinAgentGuardManifest() error {
	if len(builtinAgentBehaviorRules) != 5 {
		return fmt.Errorf("built-in rule count = %d, want 5", len(builtinAgentBehaviorRules))
	}
	if len(builtinAgentGuardProfiles) != 7 {
		return fmt.Errorf("built-in profile count = %d, want 7", len(builtinAgentGuardProfiles))
	}
	identities := make(map[string]struct{}, 8)
	for _, rule := range builtinAgentBehaviorRules {
		identity := fmt.Sprintf("rule:%s:%d", rule.RuleKey, rule.RuleVersion)
		if _, duplicate := identities[identity]; duplicate {
			return fmt.Errorf("duplicate manifest identity %s", identity)
		}
		identities[identity] = struct{}{}
		if rule.ID == uuid.Nil || !rule.Immutable || rule.Source != "builtin" {
			return fmt.Errorf("invalid immutable rule identity %s", identity)
		}
		digest, err := CalculateAgentBehaviorRuleDigest(rule)
		if err != nil {
			return fmt.Errorf("%s: %w", identity, err)
		}
		if digest != rule.Digest {
			return fmt.Errorf("%s digest mismatch: declared %s calculated %s", identity, rule.Digest, digest)
		}
	}
	for _, profile := range builtinAgentGuardProfiles {
		identity := fmt.Sprintf("profile:%s:%d", profile.ProfileKey, profile.ProfileVersion)
		if _, duplicate := identities[identity]; duplicate {
			return fmt.Errorf("duplicate manifest identity %s", identity)
		}
		identities[identity] = struct{}{}
		if profile.ID == uuid.Nil || profile.Source != "builtin" {
			return fmt.Errorf("invalid built-in profile identity %s", identity)
		}
		digest, err := CalculateAgentGuardProfileDigest(profile)
		if err != nil {
			return fmt.Errorf("%s: %w", identity, err)
		}
		if digest != profile.Digest {
			return fmt.Errorf("%s digest mismatch: declared %s calculated %s", identity, profile.Digest, digest)
		}
	}
	return nil
}

func jsonValue(value string) datatypes.JSON {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		panic(fmt.Sprintf("invalid built-in Agent Guard JSON: %v", err))
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		panic(fmt.Sprintf("normalize built-in Agent Guard JSON: %v", err))
	}
	return datatypes.JSON(normalized)
}

func canonicalJSONField(value datatypes.JSON) (any, error) {
	if len(value) == 0 {
		return nil, nil
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func sha256Digest(value any) (string, error) {
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func cloneManifest(source, destination any) {
	encoded, err := json.Marshal(source)
	if err != nil {
		panic(fmt.Sprintf("encode built-in Agent Guard manifest: %v", err))
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		panic(fmt.Sprintf("clone built-in Agent Guard manifest: %v", err))
	}
}
