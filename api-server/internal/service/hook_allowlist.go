package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
)

// HookPlan represents the parsed hook_plan_yaml structure.
// Only the fields needed for allowlist validation are extracted.
type HookPlan struct {
	Hooks []HookEntry `yaml:"hooks"`
}

// HookEntry represents a single hook in the hook plan.
type HookEntry struct {
	Name       string `yaml:"name"`
	AttachType string `yaml:"attach_type"`
	Attach     string `yaml:"attach"`
}

// AllowlistConfig represents the deserialized eBPF hook allowlist JSON.
// This mirrors the agent-side HookAllowlist struct in agent/internal/dynpkg/allowlist.go.
type AllowlistConfig struct {
	Tracepoints []string `json:"tracepoints"`
	Kprobes     []string `json:"kprobes"`
	LSM         []string `json:"lsm"`
	XDP         []string `json:"xdp"`
	TC          []string `json:"tc"`
}

// parseHookPlan parses a hook_plan_yaml string into structured hook entries.
func parseHookPlan(yamlStr string) ([]HookEntry, error) {
	if strings.TrimSpace(yamlStr) == "" {
		return nil, nil
	}
	var plan HookPlan
	if err := yaml.Unmarshal([]byte(yamlStr), &plan); err != nil {
		return nil, fmt.Errorf("parse hook_plan_yaml: %w", err)
	}
	return plan.Hooks, nil
}

// parseAllowlistConfig deserializes the allowlist JSONB into a typed struct.
func parseAllowlistConfig(configJSON datatypes.JSON) (*AllowlistConfig, error) {
	var config AllowlistConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("parse allowlist config: %w", err)
	}
	return &config, nil
}

// ValidateHooksAgainstAllowlist checks each hook against the allowlist.
// Returns nil if all hooks are allowed; returns an error listing disallowed hooks.
// This replicates the agent-side logic in agent/internal/dynpkg/allowlist.go:checkHooksAgainstAllowlist.
func ValidateHooksAgainstAllowlist(hooks []HookEntry, allowlist *AllowlistConfig) error {
	var disallowed []string
	for _, hook := range hooks {
		if !isHookAllowed(hook.AttachType, hook.Attach, allowlist) {
			disallowed = append(disallowed, fmt.Sprintf("%s(%s)", hook.Name, hook.Attach))
		}
	}
	if len(disallowed) > 0 {
		return fmt.Errorf("hooks not in allowlist: %s", strings.Join(disallowed, ", "))
	}
	return nil
}

// isHookAllowed checks if a single hook is present in the allowlist.
func isHookAllowed(attachType, attach string, allowlist *AllowlistConfig) bool {
	switch attachType {
	case "tracepoint":
		return containsString(allowlist.Tracepoints, attach)
	case "kprobe":
		return containsString(allowlist.Kprobes, attach)
	case "lsm":
		return containsString(allowlist.LSM, attach)
	case "xdp":
		return containsString(allowlist.XDP, attach)
	case "tc":
		return containsString(allowlist.TC, attach)
	}
	return false
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
