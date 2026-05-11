package configmgr

import (
	"encoding/json"
	"testing"

	pb "aegis-agent/pkg/api/v1"
)

func TestNewConfigManager(t *testing.T) {
	mgr := NewConfigManager()
	if mgr == nil {
		t.Fatal("expected non-nil ConfigManager")
	}
	if len(mgr.GetAuditRules()) != 0 {
		t.Fatalf("expected 0 audit rules, got %d", len(mgr.GetAuditRules()))
	}
}

func TestApplyConfigSync_AuditRules(t *testing.T) {
	mgr := NewConfigManager()

	rules := []map[string]interface{}{
		{
			"id":         "rule-001",
			"name":       "Block curl pipe bash",
			"rule_type":  "hard_block",
			"match_type": "regex",
			"pattern":    `(curl|wget).*\|\s*(bash|sh)`,
			"category":   "network",
			"severity":   "critical",
			"applies_to": []string{"all"},
			"is_enabled": true,
		},
		{
			"id":         "rule-002",
			"name":       "Block rm -rf /",
			"rule_type":  "hard_block",
			"match_type": "regex",
			"pattern":    `rm\s+-rf\s+/`,
			"category":   "filesystem",
			"severity":   "critical",
			"applies_to": []string{"all"},
			"is_enabled": true,
		},
	}
	payload, _ := json.Marshal(rules)

	sync := &pb.ConfigSync{
		ConfigType: "audit_rules",
		Action:     "full_sync",
		Payload:    string(payload),
	}

	if err := mgr.ApplyConfigSync(sync); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := mgr.GetAuditRules()
	if len(got) != 2 {
		t.Fatalf("expected 2 audit rules, got %d", len(got))
	}
	if got[0].ID != "rule-001" {
		t.Fatalf("expected rule ID 'rule-001', got '%s'", got[0].ID)
	}
	if got[0].RuleType != "hard_block" {
		t.Fatalf("expected rule_type 'hard_block', got '%s'", got[0].RuleType)
	}
	if got[1].Pattern != `rm\s+-rf\s+/` {
		t.Fatalf("expected pattern 'rm\\s+-rf\\s+/', got '%s'", got[1].Pattern)
	}
}

func TestApplyConfigSync_AuditSettings(t *testing.T) {
	mgr := NewConfigManager()

	settings := map[string]interface{}{
		"blacklist_enabled": true,
		"ai_enabled":        false,
		"max_retry":         5,
		"dispatch_check":    true,
		"agent_check":       true,
	}
	payload, _ := json.Marshal(settings)

	sync := &pb.ConfigSync{
		ConfigType: "audit_settings",
		Action:     "full_sync",
		Payload:    string(payload),
	}

	if err := mgr.ApplyConfigSync(sync); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := mgr.GetAuditSettings()
	if !got.BlacklistEnabled {
		t.Fatal("expected blacklist_enabled=true")
	}
	if got.AIEnabled {
		t.Fatal("expected ai_enabled=false")
	}
	if got.MaxRetry != 5 {
		t.Fatalf("expected max_retry=5, got %d", got.MaxRetry)
	}
	if !got.DispatchCheck {
		t.Fatal("expected dispatch_check=true")
	}
	if !got.AgentCheck {
		t.Fatal("expected agent_check=true")
	}
}

func TestApplyConfigSync_SigmaRules_Skipped(t *testing.T) {
	mgr := NewConfigManager()

	sync := &pb.ConfigSync{
		ConfigType: "sigma_rules",
		Action:     "full_sync",
		Payload:    "some yaml content",
	}

	// sigma_rules should be handled by the existing rule loader, not config manager
	err := mgr.ApplyConfigSync(sync)
	if err != nil {
		t.Fatalf("sigma_rules should not error, got: %v", err)
	}
	// Audit rules should remain empty
	if len(mgr.GetAuditRules()) != 0 {
		t.Fatalf("sigma_rules should not affect audit rules, got %d", len(mgr.GetAuditRules()))
	}
}

func TestApplyConfigSync_InvalidJSON(t *testing.T) {
	mgr := NewConfigManager()

	sync := &pb.ConfigSync{
		ConfigType: "audit_rules",
		Action:     "full_sync",
		Payload:    "not valid json",
	}

	err := mgr.ApplyConfigSync(sync)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestApplyConfigSync_EmptyPayload_ClearsRules(t *testing.T) {
	mgr := NewConfigManager()

	// First, add some rules
	rules := []map[string]interface{}{
		{"id": "rule-001", "name": "test", "rule_type": "hard_block", "match_type": "regex", "pattern": "test", "is_enabled": true},
	}
	payload, _ := json.Marshal(rules)
	mgr.ApplyConfigSync(&pb.ConfigSync{ConfigType: "audit_rules", Action: "full_sync", Payload: string(payload)})

	if len(mgr.GetAuditRules()) != 1 {
		t.Fatalf("expected 1 rule before clear, got %d", len(mgr.GetAuditRules()))
	}

	// Now sync with empty array (full_sync should replace)
	emptyRules := []map[string]interface{}{}
	emptyPayload, _ := json.Marshal(emptyRules)
	mgr.ApplyConfigSync(&pb.ConfigSync{ConfigType: "audit_rules", Action: "full_sync", Payload: string(emptyPayload)})

	if len(mgr.GetAuditRules()) != 0 {
		t.Fatalf("expected 0 rules after empty sync, got %d", len(mgr.GetAuditRules()))
	}
}

func TestApplyConfigSync_DefaultSettings(t *testing.T) {
	mgr := NewConfigManager()

	// Default settings should be applied
	settings := mgr.GetAuditSettings()
	if !settings.BlacklistEnabled {
		t.Fatal("expected default blacklist_enabled=true")
	}
	if !settings.AIEnabled {
		t.Fatal("expected default ai_enabled=true")
	}
	if settings.MaxRetry != 3 {
		t.Fatalf("expected default max_retry=3, got %d", settings.MaxRetry)
	}
}

func TestIsBlacklistEnabled_Default(t *testing.T) {
	mgr := NewConfigManager()
	if !mgr.IsBlacklistEnabled() {
		t.Fatal("expected blacklist enabled by default")
	}
}

func TestIsBlacklistEnabled_AfterSync(t *testing.T) {
	mgr := NewConfigManager()

	settings := map[string]interface{}{
		"blacklist_enabled": false,
		"ai_enabled":        true,
		"max_retry":         3,
		"dispatch_check":    true,
		"agent_check":       true,
	}
	payload, _ := json.Marshal(settings)

	mgr.ApplyConfigSync(&pb.ConfigSync{
		ConfigType: "audit_settings",
		Action:     "full_sync",
		Payload:    string(payload),
	})

	if mgr.IsBlacklistEnabled() {
		t.Fatal("expected blacklist disabled after sync")
	}
}

func TestApplyConfigSync_MultipleConfigTypes(t *testing.T) {
	mgr := NewConfigManager()

	// Apply audit rules
	rules := []map[string]interface{}{
		{"id": "rule-001", "name": "test rule", "rule_type": "hard_block", "match_type": "exact", "pattern": "bad_command", "is_enabled": true},
	}
	rulesPayload, _ := json.Marshal(rules)
	mgr.ApplyConfigSync(&pb.ConfigSync{ConfigType: "audit_rules", Action: "full_sync", Payload: string(rulesPayload)})

	// Apply audit settings
	settings := map[string]interface{}{
		"blacklist_enabled": true,
		"ai_enabled":        false,
		"max_retry":         5,
		"dispatch_check":    true,
		"agent_check":       false,
	}
	settingsPayload, _ := json.Marshal(settings)
	mgr.ApplyConfigSync(&pb.ConfigSync{ConfigType: "audit_settings", Action: "full_sync", Payload: string(settingsPayload)})

	// Verify both are applied independently
	if len(mgr.GetAuditRules()) != 1 {
		t.Fatalf("expected 1 audit rule, got %d", len(mgr.GetAuditRules()))
	}
	if mgr.GetAuditSettings().AIEnabled {
		t.Fatal("expected ai_enabled=false")
	}
	if mgr.GetAuditSettings().AgentCheck {
		t.Fatal("expected agent_check=false")
	}
}

func TestApplyConfigSync_PartialRules(t *testing.T) {
	mgr := NewConfigManager()

	// Apply only enabled rules
	rules := []map[string]interface{}{
		{"id": "rule-001", "name": "enabled", "rule_type": "hard_block", "match_type": "exact", "pattern": "bad", "is_enabled": true},
		{"id": "rule-002", "name": "disabled", "rule_type": "soft_warn", "match_type": "exact", "pattern": "warn", "is_enabled": false},
	}
	payload, _ := json.Marshal(rules)
	mgr.ApplyConfigSync(&pb.ConfigSync{ConfigType: "audit_rules", Action: "full_sync", Payload: string(payload)})

	// ConfigManager stores all rules (filtering is done at check time)
	got := mgr.GetAuditRules()
	if len(got) != 2 {
		t.Fatalf("expected 2 rules (both stored), got %d", len(got))
	}
}
