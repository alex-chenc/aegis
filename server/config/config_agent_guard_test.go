package config

import "testing"

func TestAgentGuardActionConsumerDefaultsOffAndRequiresValidExplicitEnv(t *testing.T) {
	t.Setenv("AGENT_GUARD_ACTION_CONSUMER_ENABLED", "")
	cfg := Config{}
	overrideFromEnv(&cfg)
	if cfg.AgentGuard.ActionConsumerEnabled {
		t.Fatal("Agent Guard action consumer must default off")
	}

	t.Setenv("AGENT_GUARD_ACTION_CONSUMER_ENABLED", "true")
	overrideFromEnv(&cfg)
	if !cfg.AgentGuard.ActionConsumerEnabled {
		t.Fatal("valid explicit action consumer flag did not enable")
	}

	t.Setenv("AGENT_GUARD_ACTION_CONSUMER_ENABLED", "not-a-bool")
	overrideFromEnv(&cfg)
	if !cfg.AgentGuard.ActionConsumerEnabled {
		t.Fatal("invalid action consumer flag changed the existing value")
	}
}
