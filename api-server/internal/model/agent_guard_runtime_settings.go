package model

import "time"

const AgentGuardRuntimeSettingsSchema = "aegis.agent_guard.runtime_settings.v1"

var AgentGuardHookAgentTypes = []string{
	"codex",
	"claude-code",
	"openclaw",
	"hermes",
	"zcode",
}

// AgentGuardHookInjection records the desired and last known dispatch state for
// one native smart-agent Hook integration. Status is control-plane state; the
// Agent reports the actual Hook processing result asynchronously.
type AgentGuardHookInjection struct {
	AgentType string `json:"agent_type"`
	// Enabled is retained as a wire-compatible alias for the legacy behavior
	// Hook. New clients must use the scope-specific switches below.
	Enabled         bool      `json:"enabled"`
	BehaviorEnabled bool      `json:"behavior_enabled"`
	EscapeEnabled   bool      `json:"escape_enabled"`
	Status          string    `json:"status"`
	ErrorCode       string    `json:"error_code,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type AgentGuardRuntimeSettings struct {
	Schema                string                    `json:"schema"`
	Version               int64                     `json:"version"`
	HostID                string                    `json:"host_id"`
	ToolAdapterEnabled    bool                      `json:"tool_adapter_enabled"`
	SessionHookEnabled    bool                      `json:"session_hook_enabled"`
	BehaviorPolicyEnabled bool                      `json:"behavior_policy_enabled"`
	EscapePolicyEnabled   bool                      `json:"escape_policy_enabled"`
	Injections            []AgentGuardHookInjection `json:"injections"`
	DispatchStatus        string                    `json:"dispatch_status"`
	DispatchErrorCode     string                    `json:"dispatch_error_code,omitempty"`
	UpdatedAt             time.Time                 `json:"updated_at"`
}

func DefaultAgentGuardRuntimeSettings(hostID string) AgentGuardRuntimeSettings {
	injections := make([]AgentGuardHookInjection, 0, len(AgentGuardHookAgentTypes))
	for _, agentType := range AgentGuardHookAgentTypes {
		injections = append(injections, AgentGuardHookInjection{
			AgentType:       agentType,
			Enabled:         agentType == "codex",
			BehaviorEnabled: agentType == "codex",
			EscapeEnabled:   agentType == "codex",
			Status:          map[bool]string{true: "pending", false: "disabled"}[agentType == "codex"],
		})
	}
	return AgentGuardRuntimeSettings{
		Schema:                AgentGuardRuntimeSettingsSchema,
		Version:               0,
		HostID:                hostID,
		ToolAdapterEnabled:    true,
		SessionHookEnabled:    true,
		BehaviorPolicyEnabled: true,
		EscapePolicyEnabled:   true,
		Injections:            injections,
		DispatchStatus:        "not_dispatched",
	}
}
