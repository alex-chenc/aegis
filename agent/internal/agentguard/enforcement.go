package agentguard

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// KernelAction is the compact action stored in Agent Guard BPF maps. Only
// deterministic, pre-execution actions are representable here.
type KernelAction uint32

const (
	KernelActionAudit KernelAction = iota
	KernelActionDeny
	KernelActionDenyAndFreeze
)

const (
	KernelMatchExact  uint32 = 1
	KernelMatchPrefix uint32 = 2
)

type KernelPathRule struct {
	PolicySlot    uint64
	RuleSlot      uint64
	ResourceType  string
	Path          string
	Match         uint32
	OperationMask uint32
	Action        KernelAction
}

type KernelEscapeRule struct {
	PolicySlot uint64
	RuleSlot   uint64
	Rule       string
	Action     KernelAction
}

type CompiledKernelPolicy struct {
	BundleVersion int64
	BundleDigest  string
	Targets       []KernelPolicyTarget
	PathRules     []KernelPathRule
	EscapeRules   []KernelEscapeRule
}

type KernelPolicyTarget struct {
	PolicySlot  uint64
	AgentTypes  []string
	ProfileKeys []string
}

type KernelSubject struct {
	PID          uint32
	InstanceSlot uint64
	UnitSlot     uint64
	PolicySlot   uint64
	ProcessEpoch uint64
}

var kernelOperationBits = map[string]uint32{
	"read":    1 << 0,
	"write":   1 << 1,
	"create":  1 << 2,
	"delete":  1 << 3,
	"rename":  1 << 4,
	"execute": 1 << 5,
	"connect": 1 << 6,
	"setns":   1 << 7,
	"mount":   1 << 8,
	"ptrace":  1 << 9,
	"load":    1 << 10,
}

var kernelEscapeRules = map[string]struct{}{
	"access_container_runtime_socket": {},
	"process_boundary_operation":      {},
}

var kernelAgentTypes = map[string]struct{}{
	"codex":       {},
	"openclaw":    {},
	"hermes":      {},
	"claude-code": {},
	"opencode":    {},
	"gemini-cli":  {},
	"zcode":       {},
}

var kernelProfileKeys = map[string]struct{}{
	"codex-linux":       {},
	"openclaw-linux":    {},
	"hermes-linux":      {},
	"claude-code-linux": {},
	"opencode-linux":    {},
	"gemini-cli-linux":  {},
	"zcode-linux":       {},
}

var builtinRuntimeSocketPaths = []string{
	"/run/containerd/containerd.sock",
	"/run/crio/crio.sock",
	"/run/podman/podman.sock",
	"/var/run/docker.sock",
}

// CompileKernelPolicy deliberately excludes glob, argv, domain and correlation
// semantics. Those remain monitor/DC rules and must never be represented as a
// synchronous kernel deny.
func CompileKernelPolicy(bundle Bundle) (CompiledKernelPolicy, error) {
	compiled := CompiledKernelPolicy{BundleVersion: bundle.BundleVersion, BundleDigest: bundle.Digest}
	policies := append([]BundlePolicy(nil), bundle.Policies...)
	sort.SliceStable(policies, func(i, j int) bool {
		return numberFromAny(policies[i]["priority"]) > numberFromAny(policies[j]["priority"])
	})
	for _, policy := range policies {
		policyKey, _ := policy["policy_key"].(string)
		policyKey = strings.TrimSpace(policyKey)
		version := numberFromAny(policy["version"])
		if policyKey == "" || version <= 0 {
			return CompiledKernelPolicy{}, errors.New("agent_guard_kernel_policy_identity_invalid")
		}
		policySlot := stableKernelSlot("policy", fmt.Sprintf("%s:%d", policyKey, version))
		target, err := compileKernelTarget(policySlot, policy["targets"])
		if err != nil {
			return CompiledKernelPolicy{}, err
		}
		compiled.Targets = append(compiled.Targets, target)
		if err := rejectServerSideDeny(policy["correlation_rules"]); err != nil {
			return CompiledKernelPolicy{}, err
		}
		atomic, err := objectArray(policy["atomic_rules"])
		if err != nil {
			return CompiledKernelPolicy{}, fmt.Errorf("agent_guard_atomic_rules_invalid: %w", err)
		}
		for _, rule := range atomic {
			action, active, err := compileKernelAction(rule["action"])
			if err != nil {
				return CompiledKernelPolicy{}, err
			}
			if !active {
				continue
			}
			ruleID, err := requiredRuleUUID(rule["rule_id"])
			if err != nil {
				return CompiledKernelPolicy{}, err
			}
			resource, ok := rule["resource"].(map[string]any)
			if !ok {
				return CompiledKernelPolicy{}, errors.New("agent_guard_atomic_resource_invalid")
			}
			resourceType := strings.TrimSpace(stringFromAny(resource["type"]))
			// The current LSM object has a reliable pathname at socket_connect.
			// File and executable path deny stays unavailable until a CO-RE
			// file_open/bprm resolver is shipped and verified on a test host.
			if resourceType != "unix_socket" {
				return CompiledKernelPolicy{}, errors.New("agent_guard_atomic_resource_not_kernel_compilable")
			}
			path := strings.TrimSpace(stringFromAny(resource["path"]))
			if !validKernelPath(path) {
				return CompiledKernelPolicy{}, errors.New("agent_guard_atomic_path_invalid")
			}
			match := KernelMatchExact
			switch strings.TrimSpace(stringFromAny(resource["match"])) {
			case "", "exact":
			case "prefix":
				match = KernelMatchPrefix
			default:
				return CompiledKernelPolicy{}, errors.New("agent_guard_atomic_match_not_kernel_compilable")
			}
			operationMask, err := compileOperationMask(rule["operations"])
			if err != nil {
				return CompiledKernelPolicy{}, err
			}
			compiled.PathRules = append(compiled.PathRules, KernelPathRule{
				PolicySlot: policySlot, RuleSlot: stableKernelSlot("rule", ruleID),
				ResourceType: resourceType, Path: filepath.Clean(path), Match: match,
				OperationMask: operationMask, Action: action,
			})
		}
		escape, err := objectArray(policy["escape_rules"])
		if err != nil {
			return CompiledKernelPolicy{}, fmt.Errorf("agent_guard_escape_rules_invalid: %w", err)
		}
		for _, rule := range escape {
			if enabled, exists := rule["enabled"]; exists {
				if value, ok := enabled.(bool); !ok || !value {
					continue
				}
			}
			action, active, err := compileKernelAction(rule["action"])
			if err != nil {
				return CompiledKernelPolicy{}, err
			}
			if !active {
				continue
			}
			ruleID, err := requiredRuleUUID(rule["rule_id"])
			if err != nil {
				return CompiledKernelPolicy{}, err
			}
			name := strings.TrimSpace(stringFromAny(rule["rule"]))
			if _, ok := kernelEscapeRules[name]; !ok {
				return CompiledKernelPolicy{}, errors.New("agent_guard_escape_rule_not_kernel_compilable")
			}
			if name == "access_container_runtime_socket" {
				for _, path := range builtinRuntimeSocketPaths {
					compiled.PathRules = append(compiled.PathRules, KernelPathRule{
						PolicySlot: policySlot, RuleSlot: stableKernelSlot("rule", ruleID),
						ResourceType: "unix_socket", Path: path, Match: KernelMatchExact,
						OperationMask: kernelOperationBits["connect"], Action: action,
					})
				}
				continue
			}
			compiled.EscapeRules = append(compiled.EscapeRules, KernelEscapeRule{
				PolicySlot: policySlot, RuleSlot: stableKernelSlot("rule", ruleID),
				Rule: name, Action: action,
			})
		}
	}
	return compiled, nil
}

func compileKernelTarget(policySlot uint64, value any) (KernelPolicyTarget, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return KernelPolicyTarget{}, errors.New("agent_guard_kernel_policy_targets_invalid")
	}
	agentTypes, err := stringArray(object["agent_types"])
	if err != nil {
		return KernelPolicyTarget{}, errors.New("agent_guard_kernel_policy_agent_types_invalid")
	}
	profileKeys, err := stringArray(object["profile_keys"])
	if err != nil {
		return KernelPolicyTarget{}, errors.New("agent_guard_kernel_policy_profiles_invalid")
	}
	// A host-scoped control-plane policy with no profile selector applies to all
	// locally known immutable profiles. The bundle itself has already enforced
	// the host boundary before kernel compilation.
	if len(agentTypes) == 0 && len(profileKeys) == 0 {
		agentTypes = []string{"*"}
	}
	if !validKernelSelectors(agentTypes, kernelAgentTypes) {
		return KernelPolicyTarget{}, errors.New("agent_guard_kernel_policy_agent_types_invalid")
	}
	if !validKernelSelectors(profileKeys, kernelProfileKeys) {
		return KernelPolicyTarget{}, errors.New("agent_guard_kernel_policy_profiles_invalid")
	}
	return KernelPolicyTarget{PolicySlot: policySlot, AgentTypes: agentTypes, ProfileKeys: profileKeys}, nil
}

func validKernelSelectors(values []string, allowed map[string]struct{}) bool {
	for _, value := range values {
		if value == "*" {
			continue
		}
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func stringArray(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("expected_array")
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(stringFromAny(item))
		if text == "" {
			return nil, errors.New("expected_nonempty_string")
		}
		values = append(values, text)
	}
	return values, nil
}

func (c CompiledKernelPolicy) PolicySlotFor(instance RuntimeInstance) (uint64, bool) {
	for _, target := range c.Targets {
		if !matchesKernelSelector(target.AgentTypes, instance.AgentType) {
			continue
		}
		if len(target.ProfileKeys) > 0 && !matchesKernelSelector(target.ProfileKeys, instance.ProfileKey) {
			continue
		}
		return target.PolicySlot, true
	}
	return 0, false
}

func matchesKernelSelector(values []string, actual string) bool {
	for _, value := range values {
		if value == "*" || value == actual {
			return true
		}
	}
	return false
}

func compileKernelAction(value any) (KernelAction, bool, error) {
	switch strings.TrimSpace(stringFromAny(value)) {
	case "", "audit", "alert", "would_deny":
		return KernelActionAudit, false, nil
	case "deny":
		return KernelActionDeny, true, nil
	case "deny_and_freeze":
		return KernelActionDenyAndFreeze, true, nil
	default:
		return 0, false, errors.New("agent_guard_kernel_action_invalid")
	}
}

func compileOperationMask(value any) (uint32, error) {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return 0, errors.New("agent_guard_atomic_operations_invalid")
	}
	var mask uint32
	for _, item := range items {
		bit, ok := kernelOperationBits[strings.TrimSpace(stringFromAny(item))]
		if !ok {
			return 0, errors.New("agent_guard_atomic_operation_not_kernel_compilable")
		}
		mask |= bit
	}
	return mask, nil
}

func rejectServerSideDeny(value any) error {
	items, err := objectArray(value)
	if err != nil {
		return fmt.Errorf("agent_guard_correlation_rules_invalid: %w", err)
	}
	for _, item := range items {
		switch strings.TrimSpace(stringFromAny(item["action"])) {
		case "deny", "deny_and_freeze":
			return errors.New("agent_guard_correlation_deny_not_kernel_compilable")
		}
	}
	return nil
}

func objectArray(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("expected_array")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("expected_object")
		}
		out = append(out, object)
	}
	return out, nil
}

func requiredRuleUUID(value any) (string, error) {
	raw := strings.ToLower(strings.TrimSpace(stringFromAny(value)))
	parsed, err := uuid.Parse(raw)
	if err != nil || parsed == uuid.Nil || parsed.String() != raw {
		return "", errors.New("agent_guard_kernel_rule_id_invalid")
	}
	return raw, nil
}

func validKernelPath(path string) bool {
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return false
	}
	for _, segment := range strings.Split(strings.ReplaceAll(path, "\\", "/"), "/") {
		if segment == ".." {
			return false
		}
	}
	return len(path) <= 255
}

func stableKernelSlot(namespace, identity string) uint64 {
	sum := sha256.Sum256([]byte(namespace + "\x00" + identity))
	slot := binary.BigEndian.Uint64(sum[:8])
	if slot == 0 {
		return 1
	}
	return slot
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func numberFromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		value, _ := typed.Int64()
		return value
	default:
		return 0
	}
}
