package config

import "testing"

func TestAgentGuardToolAdapterDefaultsDisabledInYAML(t *testing.T) {
	t.Setenv("AGENT_GUARD_TOOL_ADAPTER_ENABLED", "")
	cfg, err := Load("api-server.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AgentGuard.ToolAdapterEnabled {
		t.Fatal("tool adapter rollout must be disabled by default")
	}
}

func TestOverrideFromEnvAgentGuardFlagsDefaultSafeAndExplicit(t *testing.T) {
	t.Setenv("AGENT_GUARD_ENABLED", "true")
	t.Setenv("AGENT_GUARD_POLICY_WRITE_ENABLED", "false")
	t.Setenv("AGENT_GUARD_ANALYSIS_ENABLED", "true")
	t.Setenv("AGENT_GUARD_ACTION_ENABLED", "false")
	t.Setenv("AGENT_GUARD_TOOL_ADAPTER_ENABLED", "true")
	t.Setenv("AGENT_GUARD_SCOPE_SIGNING_KEY", "test-only-signing-key")

	cfg := &Config{}
	overrideFromEnv(cfg)

	if !cfg.AgentGuard.Enabled {
		t.Fatal("AGENT_GUARD_ENABLED was not applied")
	}
	if cfg.AgentGuard.PolicyWriteEnabled {
		t.Fatal("policy writes must remain disabled")
	}
	if !cfg.AgentGuard.AnalysisEnabled {
		t.Fatal("analysis flag was not applied")
	}
	if cfg.AgentGuard.ActionEnabled {
		t.Fatal("actions must remain disabled")
	}
	if !cfg.AgentGuard.ToolAdapterEnabled {
		t.Fatal("AGENT_GUARD_TOOL_ADAPTER_ENABLED was not applied")
	}
	if cfg.AgentGuard.ScopeSigningKey != "test-only-signing-key" {
		t.Fatal("scope signing key was not applied")
	}
}

func TestOverrideFromEnvIgnoresInvalidAgentGuardBoolean(t *testing.T) {
	t.Setenv("AGENT_GUARD_ACTION_ENABLED", "not-a-boolean")
	t.Setenv("AGENT_GUARD_TOOL_ADAPTER_ENABLED", "not-a-boolean")

	cfg := &Config{
		AgentGuard: AgentGuardConfig{ActionEnabled: false, ToolAdapterEnabled: false},
	}
	overrideFromEnv(cfg)

	if cfg.AgentGuard.ActionEnabled {
		t.Fatal("invalid boolean must not enable Agent Guard actions")
	}
	if cfg.AgentGuard.ToolAdapterEnabled {
		t.Fatal("invalid boolean must not enable Agent Guard tool adapter rollout")
	}
}
