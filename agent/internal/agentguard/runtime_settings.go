package agentguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

const AgentGuardRuntimeSettingsSchema = "aegis.agent_guard.runtime_settings.v1"

var supportedHookAgentTypes = map[string]struct{}{
	"codex": {}, "claude-code": {}, "openclaw": {}, "hermes": {}, "zcode": {},
}

type HookInjection struct {
	AgentType       string `json:"agent_type"`
	Enabled         bool   `json:"enabled"`
	BehaviorEnabled bool   `json:"behavior_enabled"`
	EscapeEnabled   bool   `json:"escape_enabled"`
}

type RuntimeSettings struct {
	Schema                string          `json:"schema"`
	Version               int64           `json:"version"`
	HostID                string          `json:"host_id"`
	ToolAdapterEnabled    bool            `json:"tool_adapter_enabled"`
	SessionHookEnabled    bool            `json:"session_hook_enabled"`
	BehaviorPolicyEnabled bool            `json:"behavior_policy_enabled"`
	EscapePolicyEnabled   bool            `json:"escape_policy_enabled"`
	Injections            []HookInjection `json:"injections"`
}

func decodeRuntimeSettings(payload string) (RuntimeSettings, error) {
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	var settings RuntimeSettings
	if err := decoder.Decode(&settings); err != nil {
		return RuntimeSettings{}, errors.New("agent_guard_runtime_settings_invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return RuntimeSettings{}, errors.New("agent_guard_runtime_settings_invalid")
	}
	if settings.Schema != AgentGuardRuntimeSettingsSchema || settings.Version < 1 || strings.TrimSpace(settings.HostID) == "" {
		return RuntimeSettings{}, errors.New("agent_guard_runtime_settings_invalid")
	}
	seen := make(map[string]struct{}, len(settings.Injections))
	for _, injection := range settings.Injections {
		if _, ok := supportedHookAgentTypes[injection.AgentType]; !ok {
			return RuntimeSettings{}, fmt.Errorf("agent_guard_runtime_settings_agent_type_invalid")
		}
		if _, ok := seen[injection.AgentType]; ok {
			return RuntimeSettings{}, errors.New("agent_guard_runtime_settings_duplicate_agent_type")
		}
		seen[injection.AgentType] = struct{}{}
	}
	return settings, nil
}

type HookProvisioner func(agentType string) error

type ScopedHookProvisioner func(agentType, scope string) error

func DefaultHookProvisioner(binaryPath string) HookProvisioner {
	return func(agentType string) error {
		if strings.TrimSpace(binaryPath) == "" {
			return errors.New("agent_guard_hook_binary_missing")
		}
		command := exec.Command(binaryPath, "provision", "--agent-type", agentType)
		command.Env = hookCommandEnvironment()
		command.Stdout = nil
		command.Stderr = nil
		if err := command.Run(); err != nil {
			// Do not expose command output or user paths in the Agent event.
			return errors.New("agent_guard_hook_provision_failed")
		}
		return nil
	}
}

type HookRemover func(agentType string) error

func DefaultHookRemover(binaryPath string) HookRemover {
	return func(agentType string) error {
		if strings.TrimSpace(binaryPath) == "" {
			return errors.New("agent_guard_hook_binary_missing")
		}
		command := exec.Command(binaryPath, "remove", "--agent-type", agentType)
		command.Env = hookCommandEnvironment()
		command.Stdout = nil
		command.Stderr = nil
		if err := command.Run(); err != nil {
			// Do not expose command output or user paths in the Agent event.
			return errors.New("agent_guard_hook_remove_failed")
		}
		return nil
	}
}

func DefaultScopedHookProvisioner(binaryPath string) ScopedHookProvisioner {
	return func(agentType, scope string) error {
		if strings.TrimSpace(binaryPath) == "" {
			return errors.New("agent_guard_hook_binary_missing")
		}
		command := exec.Command(binaryPath, "provision", "--agent-type", agentType, "--scope", scope)
		command.Env = hookCommandEnvironment()
		command.Stdout = nil
		command.Stderr = nil
		if err := command.Run(); err != nil {
			return errors.New("agent_guard_hook_provision_failed")
		}
		return nil
	}
}

func DefaultScopedHookRemover(binaryPath string) ScopedHookProvisioner {
	return func(agentType, scope string) error {
		if strings.TrimSpace(binaryPath) == "" {
			return errors.New("agent_guard_hook_binary_missing")
		}
		command := exec.Command(binaryPath, "remove", "--agent-type", agentType, "--scope", scope)
		command.Env = hookCommandEnvironment()
		command.Stdout = nil
		command.Stderr = nil
		if err := command.Run(); err != nil {
			return errors.New("agent_guard_hook_remove_failed")
		}
		return nil
	}
}

func hookCommandEnvironment() []string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		if current, userErr := user.Current(); userErr == nil {
			home = current.HomeDir
		}
	}
	environment := os.Environ()
	if strings.TrimSpace(home) == "" {
		return environment
	}
	for index, entry := range environment {
		if strings.HasPrefix(entry, "HOME=") {
			environment[index] = "HOME=" + home
			return environment
		}
	}
	return append(environment, "HOME="+home)
}
