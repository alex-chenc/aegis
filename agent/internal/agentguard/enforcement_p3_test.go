package agentguard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestP3BundleRequiresLocalCapabilityGatesAndCompilesOnlyAtomicRules(t *testing.T) {
	bundle := validBundle(t, 9)
	bundle.Defaults.Mode = "enforce"
	bundle.Defaults.EnforcementEnabled = true
	bundle.Defaults.FreezeEnabled = true
	bundle.Policies = []BundlePolicy{{
		"policy_key": "escape-protection",
		"version":    float64(1),
		"priority":   float64(100),
		"targets":    map[string]any{"agent_types": []any{"codex"}, "profile_keys": []any{"codex-linux"}},
		"atomic_rules": []any{map[string]any{
			"rule_id":    "56a62b88-c9ce-422b-a59a-b814cf5a718f",
			"rule":       "protected_resource_access",
			"resource":   map[string]any{"type": "unix_socket", "path": "/run/containerd/containerd.sock", "match": "exact"},
			"operations": []any{"connect"},
			"action":     "deny",
			"severity":   "critical",
		}},
		"escape_rules": []any{map[string]any{
			"rule_id":  "fb469091-818c-42c3-86d8-01b9312fca09",
			"rule":     "process_boundary_operation",
			"action":   "deny_and_freeze",
			"severity": "critical",
			"enabled":  true,
		}},
	}}
	digest, err := BundleDigest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Digest = digest
	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeBundle(payload)
	if err != nil {
		t.Fatal(err)
	}
	bundle = decoded

	if err := bundle.Validate("host-1"); err == nil {
		t.Fatal("P3 bundle bypassed default monitor-only validation")
	}
	options := BundleValidationOptions{EnforcementAllowed: true, FreezeAllowed: true}
	if err := bundle.ValidateWithOptions("host-1", options); err != nil {
		t.Fatalf("locally authorized P3 bundle rejected: %v", err)
	}
	compiled, err := CompileKernelPolicy(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.BundleVersion != 9 || len(compiled.PathRules) != 1 || len(compiled.EscapeRules) != 1 {
		t.Fatalf("kernel policy missing atomic rules: %+v", compiled)
	}
	if compiled.PathRules[0].Action != KernelActionDeny ||
		compiled.EscapeRules[0].Action != KernelActionDenyAndFreeze {
		t.Fatalf("kernel actions were not preserved: %+v", compiled)
	}
}

func TestKernelPolicyRejectsComplexOrServerSideDenyRules(t *testing.T) {
	tests := []struct {
		name   string
		policy BundlePolicy
	}{
		{
			name: "glob",
			policy: BundlePolicy{"policy_key": "p", "version": float64(1), "targets": map[string]any{"agent_types": []any{"*"}}, "atomic_rules": []any{map[string]any{
				"rule_id": "56a62b88-c9ce-422b-a59a-b814cf5a718f", "rule": "protected_resource_access",
				"resource":   map[string]any{"type": "unix_socket", "path": "/run/*", "match": "glob"},
				"operations": []any{"write"}, "action": "deny", "severity": "critical",
			}}},
		},
		{
			name: "correlation deny",
			policy: BundlePolicy{"policy_key": "p", "version": float64(1), "targets": map[string]any{"agent_types": []any{"*"}}, "correlation_rules": []any{map[string]any{
				"rule_id": "chain", "action": "deny", "severity": "critical",
			}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := validBundle(t, 10)
			bundle.Policies = []BundlePolicy{test.policy}
			if _, err := CompileKernelPolicy(bundle); err == nil {
				t.Fatal("non-atomic deny compiled into kernel policy")
			}
		})
	}
}

func TestKernelPolicySlotsAreStableAndUUIDsNeverEnterMapKeys(t *testing.T) {
	first := stableKernelSlot("unit", "8a47420a-c88b-4c27-b6b8-66ec82fab505")
	second := stableKernelSlot("unit", "8a47420a-c88b-4c27-b6b8-66ec82fab505")
	if first == 0 || first != second {
		t.Fatalf("unstable kernel slot: %d %d", first, second)
	}
	if first == stableKernelSlot("instance", "8a47420a-c88b-4c27-b6b8-66ec82fab505") {
		t.Fatal("kernel slot namespace was ignored")
	}
}

func TestP4ProfilesCompileMonitorAndEnforcementKernelTargets(t *testing.T) {
	profiles := []struct {
		agentType  string
		profileKey string
	}{
		{agentType: "claude-code", profileKey: "claude-code-linux"},
		{agentType: "opencode", profileKey: "opencode-linux"},
		{agentType: "gemini-cli", profileKey: "gemini-cli-linux"},
	}
	for index, profile := range profiles {
		t.Run(profile.agentType, func(t *testing.T) {
			monitor := validBundle(t, int64(20+index))
			monitor.Policies = []BundlePolicy{{
				"policy_key": "p4-monitor", "version": float64(1),
				"targets": map[string]any{
					"agent_types": []any{profile.agentType}, "profile_keys": []any{profile.profileKey},
				},
			}}
			compiled, err := CompileKernelPolicy(monitor)
			if err != nil {
				t.Fatalf("compile monitor target: %v", err)
			}
			instance := RuntimeInstance{AgentType: profile.agentType, ProfileKey: profile.profileKey}
			if _, ok := compiled.PolicySlotFor(instance); !ok {
				t.Fatalf("P4 profile did not match compiled monitor target: %#v", compiled.Targets)
			}

			enforcement := monitor
			enforcement.Defaults.Mode = "enforce"
			enforcement.Defaults.EnforcementEnabled = true
			enforcement.Policies[0]["atomic_rules"] = []any{map[string]any{
				"rule_id": uuid.NewString(), "rule": "protected_resource_access",
				"resource":   map[string]any{"type": "unix_socket", "path": "/run/containerd/containerd.sock", "match": "exact"},
				"operations": []any{"connect"}, "action": "deny", "severity": "critical",
			}}
			compiled, err = CompileKernelPolicy(enforcement)
			if err != nil || len(compiled.PathRules) != 1 {
				t.Fatalf("compile enforcement target: compiled=%#v err=%v", compiled, err)
			}
		})
	}
}

func TestKernelPolicyHostOnlyTargetDefaultsToStableProfileWildcard(t *testing.T) {
	bundle := validBundle(t, 30)
	bundle.Policies = []BundlePolicy{{
		"policy_key": "host-monitor", "version": float64(1),
		"targets": map[string]any{"host_ids": []any{"host-1"}},
	}}
	compiled, err := CompileKernelPolicy(bundle)
	if err != nil {
		t.Fatalf("host-scoped monitor policy rejected: %v", err)
	}
	if len(compiled.Targets) != 1 || len(compiled.Targets[0].AgentTypes) != 1 ||
		compiled.Targets[0].AgentTypes[0] != "*" {
		t.Fatalf("host-only target did not compile to local wildcard: %#v", compiled.Targets)
	}

	bundle.Policies[0]["targets"] = map[string]any{"agent_types": []any{"unknown-agent"}}
	if _, err := CompileKernelPolicy(bundle); err == nil || !strings.Contains(err.Error(), "agent_types_invalid") {
		t.Fatalf("unknown agent type was not rejected: %v", err)
	}
}
