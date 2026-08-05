package pipeline

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"dc/internal/model"

	"github.com/google/uuid"
)

type BuiltinRuleDefinition struct {
	ID                uuid.UUID
	Key               string
	Version           int64
	Digest            string
	Name              string
	DefaultSeverity   string
	DefaultAction     string
	DefaultParameters map[string]any
	RequiredEvidence  []string
	AllowConditions   []string
}

type AgentRuleHit struct {
	RuleID                 uuid.UUID `json:"rule_id"`
	RuleKey                string    `json:"rule_key"`
	RuleVersion            int64     `json:"rule_version"`
	RuleDigest             string    `json:"rule_digest"`
	RuleName               string    `json:"rule_name"`
	EventID                string    `json:"event_id"`
	Severity               string    `json:"severity"`
	Decision               string    `json:"decision"`
	Confidence             float64   `json:"confidence"`
	MatchKind              string    `json:"match_kind"`
	AttackStage            string    `json:"attack_stage"`
	Outcome                string    `json:"outcome"`
	ResourceClassification string    `json:"resource_classification,omitempty"`
}

var builtinRuleManifest = []BuiltinRuleDefinition{
	{
		ID: uuid.MustParse("62000000-0000-4000-8000-000000000001"), Key: "AGB-BUILTIN-001",
		Version: 1, Digest: "sha256:e9a7f8b0dda7c742557bbc1a0551ea4caeb0329973ec1c24f7751b4cd2902a82",
		Name: "操作敏感目录", DefaultSeverity: "medium", DefaultAction: "alert",
		DefaultParameters: map[string]any{
			"resource_groups": []string{"credential", "privilege_policy", "cloud_or_cluster_credential", "persistence", "security_control", "container_control"},
			"operations":      []string{"open_intent", "read_observed", "write", "create", "truncate", "delete", "rename", "chmod", "chown", "execute"},
		},
		RequiredEvidence: []string{"actor.pid", "actor.ppid", "actor.start_ticks", "operation", "resource.resolved_path", "resource.classification", "outcome"},
		AllowConditions:  []string{"trusted_process_digest", "policy_exception", "approved_change_window"},
	},
	{
		ID: uuid.MustParse("62000000-0000-4000-8000-000000000002"), Key: "AGB-BUILTIN-002",
		Version: 1, Digest: "sha256:5852cf43c0be2ddc21e83c8c12fb898ac2aae47bc0d7bff2a5246d4d2436e613",
		Name: "外部网络连接", DefaultSeverity: "medium", DefaultAction: "alert",
		DefaultParameters: map[string]any{"trusted_cidrs": []string{}, "trusted_domains": []string{}, "trusted_ports": []int{}},
		RequiredEvidence:  []string{"actor.pid", "actor.ppid", "actor.start_ticks", "network.direction", "network.destination_ip", "network.destination_port", "network.protocol", "outcome"},
		AllowConditions:   []string{"loopback_or_link_local", "private_or_cluster_network", "trusted_destination", "policy_exception"},
	},
	{
		ID: uuid.MustParse("62000000-0000-4000-8000-000000000003"), Key: "AGB-BUILTIN-003",
		Version: 1, Digest: "sha256:b066e0b452fb7749f9afb49e8a6b918de1285ccef912f50680795a4bc110e03e",
		Name: "文件生成", DefaultSeverity: "low", DefaultAction: "audit",
		DefaultParameters: map[string]any{"alert_on_executable": true, "alert_on_hidden": true, "hash_max_bytes": 10485760},
		RequiredEvidence:  []string{"actor.pid", "actor.ppid", "actor.start_ticks", "operation", "resource.inode_created", "resource.resolved_path", "outcome"},
		AllowConditions:   []string{"workspace_low_risk_file", "policy_exception"},
	},
	{
		ID: uuid.MustParse("62000000-0000-4000-8000-000000000004"), Key: "AGB-BUILTIN-004",
		Version: 1, Digest: "sha256:43e4e365124e4d895a27f8267e8ab424f0482f121794a812aea946231520e130",
		Name: "敏感命令执行", DefaultSeverity: "medium", DefaultAction: "alert",
		DefaultParameters: map[string]any{
			"command_categories":          []string{"network_transfer", "privilege", "permission_change", "namespace_mount", "account_persistence", "destructive", "security_control"},
			"require_resolved_executable": false,
		},
		RequiredEvidence: []string{"actor.pid", "actor.ppid", "actor.start_ticks", "process.executable", "process.argv", "process.cwd", "outcome"},
		AllowConditions:  []string{"trusted_process_digest", "policy_exception", "approved_change_window"},
	},
	{
		ID: uuid.MustParse("62000000-0000-4000-8000-000000000005"), Key: "AGB-BUILTIN-005",
		Version: 1, Digest: "sha256:63ce19628fc8285ded19f9609ec93770341c14e24e47680e95aff3cec4d775f1",
		Name: "提权行为", DefaultSeverity: "high", DefaultAction: "alert",
		DefaultParameters: map[string]any{"alert_on_failed_attempt": true, "host_root_severity": "critical", "unexpected_capability_severity": "high"},
		RequiredEvidence:  []string{"actor.pid", "actor.ppid", "actor.start_ticks", "identity.before", "identity.after", "identity.user_namespace", "outcome"},
		AllowConditions:   []string{"profile_expected_identity_transition", "container_user_namespace_root", "policy_exception"},
	},
}

func BuiltinRuleManifest() []BuiltinRuleDefinition {
	result := make([]BuiltinRuleDefinition, 0, len(builtinRuleManifest))
	for _, rule := range builtinRuleManifest {
		cloned := rule
		cloned.DefaultParameters = make(map[string]any, len(rule.DefaultParameters))
		for key, value := range rule.DefaultParameters {
			switch typed := value.(type) {
			case []string:
				cloned.DefaultParameters[key] = append([]string(nil), typed...)
			case []int:
				cloned.DefaultParameters[key] = append([]int(nil), typed...)
			default:
				cloned.DefaultParameters[key] = typed
			}
		}
		cloned.RequiredEvidence = append([]string(nil), rule.RequiredEvidence...)
		cloned.AllowConditions = append([]string(nil), rule.AllowConditions...)
		result = append(result, cloned)
	}
	return result
}

func EvaluateBuiltinRules(event *model.AgentBehaviorEvent, options RuleEvaluationOptions) []AgentRuleHit {
	if event == nil || event.InstanceID == nil || event.SessionID == nil || event.ExecutionUnitID == nil {
		return nil
	}
	resource := decodeJSONObject(event.Resource)
	attributes := objectField(resource, "attributes")
	if hasPolicyException(event, attributes, options) {
		return nil
	}
	hits := make([]AgentRuleHit, 0, 2)
	if hit := evaluateSensitiveResource(event, options); hit != nil {
		hits = append(hits, *hit)
	}
	if hit := evaluateExternalNetwork(event); hit != nil {
		hits = append(hits, *hit)
	}
	if hit := evaluateFileCreate(event, attributes); hit != nil {
		hits = append(hits, *hit)
	}
	if hit := evaluateSensitiveCommand(event); hit != nil {
		hits = append(hits, *hit)
	}
	if hit := evaluatePrivilege(event, attributes, options); hit != nil {
		hits = append(hits, *hit)
	}
	filtered := hits[:0]
	for _, hit := range hits {
		if !containsStringValue(options.ExcludedRuleKeys, hit.RuleKey) {
			filtered = append(filtered, hit)
		}
	}
	return filtered
}

func evaluateSensitiveResource(event *model.AgentBehaviorEvent, options RuleEvaluationOptions) *AgentRuleHit {
	if event.Category != "file" ||
		!containsStringValue([]string{
			"open_intent", "read_observed", "write", "create", "truncate",
			"delete", "rename", "chmod", "chown", "execute",
		}, event.Operation) ||
		!containsStringValue([]string{
			"credential", "privilege_policy", "cloud_or_cluster_credential",
			"persistence", "security_control", "container_control",
		}, event.ResourceClassification) ||
		containsStringValue(options.ExcludedResourceGroups, event.ResourceClassification) ||
		containsStringValue(options.ExcludedOperations, event.Operation) {
		return nil
	}
	severity := "medium"
	decision := "audit"
	modifying := containsStringValue([]string{"write", "create", "truncate", "delete", "rename", "chmod", "chown"}, event.Operation)
	switch event.ResourceClassification {
	case "credential", "privilege_policy", "cloud_or_cluster_credential", "security_control", "container_control":
		severity, decision = "high", "alert"
		if modifying {
			severity = "critical"
		}
	case "persistence":
		if modifying {
			severity, decision = "high", "alert"
		}
	}
	return newRuleHit(0, event, severity, decision, 0.82, "sensitive_resource", "credential_access")
}

func evaluateExternalNetwork(event *model.AgentBehaviorEvent) *AgentRuleHit {
	if event.Category != "network" || event.Operation != "connect" || event.ResourceClassification != "external" {
		return nil
	}
	severity, decision, confidence := "medium", "alert", 0.78
	if event.Outcome != "success" {
		severity, decision, confidence = "low", "audit", 0.62
	}
	return newRuleHit(1, event, severity, decision, confidence, "external_outbound", "command_and_control")
}

func evaluateFileCreate(event *model.AgentBehaviorEvent, attributes map[string]any) *AgentRuleHit {
	if event.Category != "file" || event.Operation != "create" || event.Outcome != "success" ||
		!boolValue(attributes["inode_created"]) {
		return nil
	}
	severity, decision, confidence := "low", "audit", 0.76
	switch event.ResourceClassification {
	case "credential", "privilege_policy", "cloud_or_cluster_credential", "security_control", "container_control":
		severity, decision = "critical", "alert"
	case "persistence":
		severity, decision = "high", "alert"
	default:
		if boolValue(attributes["hidden"]) || boolValue(attributes["executable"]) {
			severity, decision = "medium", "alert"
		}
	}
	return newRuleHit(2, event, severity, decision, confidence, "file_created", "resource_development")
}

func evaluateSensitiveCommand(event *model.AgentBehaviorEvent) *AgentRuleHit {
	if event.ResourceClassification == "" ||
		event.Category == "process" && event.Operation != "exec" ||
		event.Category == "tool" && event.Operation != "tool_call_completed" && event.Operation != "tool_call_failed" ||
		event.Category != "process" && event.Category != "tool" {
		return nil
	}
	if !containsStringValue([]string{
		"network_transfer", "privilege", "permission_change", "namespace_mount",
		"account_persistence", "destructive", "security_control",
	}, event.ResourceClassification) {
		return nil
	}
	severity, decision, confidence := "medium", "alert", 0.75
	if event.Outcome != "success" {
		severity, decision, confidence = "low", "audit", 0.60
	}
	return newRuleHit(3, event, severity, decision, confidence, "sensitive_command", commandAttackStage(event.ResourceClassification))
}

func evaluatePrivilege(event *model.AgentBehaviorEvent, attributes map[string]any, options RuleEvaluationOptions) *AgentRuleHit {
	if event.Category != "identity" || !containsStringValue([]string{
		"setuid", "setgid", "setresuid", "setresgid", "capset", "credential_change", "capability_change",
	}, event.Operation) {
		return nil
	}
	if event.Outcome == "success" && event.ResourceClassification == "container_root_transition" {
		return nil
	}
	severity, decision, confidence, match := "medium", "alert", 0.65, "privilege_attempted"
	if event.Outcome == "success" {
		switch event.ResourceClassification {
		case "host_root_gain":
			severity, confidence, match = "critical", 0.95, "host_root_gained"
		case "capability_gain":
			severity, confidence, match = "high", 0.88, "capability_gained"
		default:
			severity, decision, confidence, match = "medium", "audit", 0.50, "privilege_inconclusive"
		}
	}
	if containsStringValue(options.ExpectedIdentityEventIDs, event.RawEventID) {
		return nil
	}
	return newRuleHit(4, event, severity, decision, confidence, match, "privilege_escalation")
}

func newRuleHit(index int, event *model.AgentBehaviorEvent, severity, decision string, confidence float64, match, stage string) *AgentRuleHit {
	rule := builtinRuleManifest[index]
	return &AgentRuleHit{
		RuleID: rule.ID, RuleKey: rule.Key, RuleVersion: rule.Version, RuleDigest: rule.Digest,
		RuleName: rule.Name, EventID: event.RawEventID, Severity: severity, Decision: decision,
		Confidence: confidence, MatchKind: match, AttackStage: stage, Outcome: event.Outcome,
		ResourceClassification: event.ResourceClassification,
	}
}

func hasPolicyException(event *model.AgentBehaviorEvent, attributes map[string]any, options RuleEvaluationOptions) bool {
	if containsStringValue(options.PolicyExceptionEventIDs, event.RawEventID) ||
		containsStringValue(options.ApprovedChangeEventIDs, event.RawEventID) {
		return true
	}
	digest := strings.ToLower(stringValueAny(attributes["process_digest"]))
	for _, trusted := range options.TrustedProcessDigests {
		if digest != "" && digest == strings.ToLower(strings.TrimSpace(trusted)) {
			return true
		}
	}
	return false
}

func commandAttackStage(category string) string {
	switch category {
	case "network_transfer":
		return "command_and_control"
	case "privilege":
		return "privilege_escalation"
	case "account_persistence":
		return "persistence"
	case "security_control":
		return "defense_evasion"
	default:
		return "execution"
	}
}

func commandBasename(event *model.AgentBehaviorEvent) string {
	if value := filepath.Base(event.ProcessExe); value != "." && value != "" {
		return value
	}
	var argv []string
	if json.Unmarshal(event.CommandArgv, &argv) == nil && len(argv) > 0 {
		return filepath.Base(argv[0])
	}
	return ""
}
