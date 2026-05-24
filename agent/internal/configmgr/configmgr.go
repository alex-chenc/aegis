package configmgr

import (
	"encoding/json"
	"fmt"
	"sync"

	pb "aegis-agent/pkg/api/v1"
)

type AuditRule struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	RuleType  string   `json:"rule_type"`
	MatchType string   `json:"match_type"`
	Pattern   string   `json:"pattern"`
	Category  string   `json:"category"`
	Severity  string   `json:"severity"`
	AppliesTo []string `json:"applies_to"`
	IsEnabled bool     `json:"is_enabled"`
}

type AuditSettings struct {
	BlacklistEnabled bool `json:"blacklist_enabled"`
	AIEnabled        bool `json:"ai_enabled"`
	MaxRetry         int  `json:"max_retry"`
	DispatchCheck    bool `json:"dispatch_check"`
	AgentCheck       bool `json:"agent_check"`
}

func DefaultAuditSettings() AuditSettings {
	return AuditSettings{
		BlacklistEnabled: true,
		AIEnabled:        true,
		MaxRetry:         3,
		DispatchCheck:    true,
		AgentCheck:       true,
	}
}

type ConfigManager struct {
	auditRules    []AuditRule
	auditSettings AuditSettings
	mu            sync.RWMutex

	// Callbacks for V5.8 detection package handling
	onDetectionPackage func(action, payload string) error
	onAllowlistUpdate  func(payload string) error
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		auditRules:    []AuditRule{},
		auditSettings: DefaultAuditSettings(),
	}
}

// SetDetectionPackageHandler sets the callback for detection package commands
func (m *ConfigManager) SetDetectionPackageHandler(fn func(action, payload string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDetectionPackage = fn
}

// SetAllowlistUpdateHandler sets the callback for allowlist updates
func (m *ConfigManager) SetAllowlistUpdateHandler(fn func(payload string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAllowlistUpdate = fn
}

func (m *ConfigManager) ApplyConfigSync(sync *pb.ConfigSync) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch sync.ConfigType {
	case "audit_rules":
		return m.applyAuditRules(sync)
	case "audit_settings":
		return m.applyAuditSettings(sync)
	case "sigma_rules":
		// Handled by the existing rule loader, not here
		return nil
	case "detection_package":
		if m.onDetectionPackage != nil {
			return m.onDetectionPackage(sync.Action, sync.Payload)
		}
		return nil
	case "dynamic_ebpf_hook_allowlist":
		if m.onAllowlistUpdate != nil {
			return m.onAllowlistUpdate(sync.Payload)
		}
		return nil
	default:
		return fmt.Errorf("unknown config type: %s", sync.ConfigType)
	}
}

func (m *ConfigManager) applyAuditRules(sync *pb.ConfigSync) error {
	var rules []AuditRule
	if err := json.Unmarshal([]byte(sync.Payload), &rules); err != nil {
		return fmt.Errorf("failed to unmarshal audit rules: %w", err)
	}
	m.auditRules = rules
	return nil
}

func (m *ConfigManager) applyAuditSettings(sync *pb.ConfigSync) error {
	var settings AuditSettings
	if err := json.Unmarshal([]byte(sync.Payload), &settings); err != nil {
		return fmt.Errorf("failed to unmarshal audit settings: %w", err)
	}
	m.auditSettings = settings
	return nil
}

func (m *ConfigManager) GetAuditRules() []AuditRule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]AuditRule, len(m.auditRules))
	copy(result, m.auditRules)
	return result
}

func (m *ConfigManager) GetAuditSettings() AuditSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.auditSettings
}

func (m *ConfigManager) IsBlacklistEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.auditSettings.BlacklistEnabled
}
