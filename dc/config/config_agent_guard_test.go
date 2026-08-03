package config

import "testing"

func TestAgentGuardProjectionFlagDefaultsOffAndLoadsFromEnv(t *testing.T) {
	t.Setenv("AGENT_GUARD_PROJECTION_ENABLED", "true")
	cfg, err := Load("dc.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AgentGuard.ProjectionEnabled {
		t.Fatal("projection flag was not loaded")
	}
	if cfg.AgentGuard.RulesEnabled || cfg.AgentGuard.FindingsEnabled ||
		cfg.AgentGuard.AnalysisRequestEnabled || cfg.AgentGuard.AlertEnabled ||
		cfg.AgentGuard.ActionEnabled || cfg.AgentGuard.DenyEnabled ||
		cfg.AgentGuard.FreezeEnabled || cfg.AgentGuard.ActionPublishEnabled {
		t.Fatal("P2 flags must remain disabled")
	}
}

func TestAgentGuardActionFlagsDefaultOffAndRequireFullParentChain(t *testing.T) {
	t.Setenv("AGENT_GUARD_PROJECTION_ENABLED", "true")
	t.Setenv("AGENT_BEHAVIOR_RULES_ENABLED", "true")
	t.Setenv("AGENT_BEHAVIOR_FINDINGS_ENABLED", "true")
	t.Setenv("AGENT_GUARD_ACTION_ENABLED", "false")
	t.Setenv("AGENT_GUARD_DENY_ENABLED", "true")
	t.Setenv("AGENT_GUARD_FREEZE_ENABLED", "true")
	t.Setenv("AGENT_GUARD_ACTION_PUBLISH_ENABLED", "true")
	cfg, err := Load("dc.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AgentGuard.ActionEnabled || cfg.AgentGuard.DenyEnabled ||
		cfg.AgentGuard.FreezeEnabled || cfg.AgentGuard.ActionPublishEnabled {
		t.Fatalf("child action gates escaped disabled parent: %#v", cfg.AgentGuard)
	}

	t.Setenv("AGENT_GUARD_ACTION_ENABLED", "true")
	cfg, err = Load("dc.yaml")
	if err != nil {
		t.Fatalf("Load enabled: %v", err)
	}
	if !cfg.AgentGuard.ActionEnabled || !cfg.AgentGuard.DenyEnabled ||
		!cfg.AgentGuard.FreezeEnabled || !cfg.AgentGuard.ActionPublishEnabled {
		t.Fatalf("action gates did not enable: %#v", cfg.AgentGuard)
	}
}

func TestAgentGuardRuleFeatureFlagsRequireTheirParentGate(t *testing.T) {
	t.Setenv("AGENT_BEHAVIOR_RULES_ENABLED", "false")
	t.Setenv("AGENT_BEHAVIOR_FINDINGS_ENABLED", "true")
	t.Setenv("AGENT_BEHAVIOR_ANALYSIS_REQUEST_ENABLED", "true")
	t.Setenv("AGENT_GUARD_ALERT_ENABLED", "true")
	cfg, err := Load("dc.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AgentGuard.FindingsEnabled || cfg.AgentGuard.AnalysisRequestEnabled ||
		cfg.AgentGuard.AlertEnabled {
		t.Fatalf("child gates escaped disabled rules: %#v", cfg.AgentGuard)
	}
}
