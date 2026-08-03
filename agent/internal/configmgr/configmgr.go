package configmgr

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	onAgentGuardBundle func(payload string) error
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

// SetAgentGuardBundleHandler installs the trusted Agent Guard full-bundle
// validator/applicator. ConfigManager intentionally does not inspect the
// sensitive policy payload or keep a second copy of its state.
func (m *ConfigManager) SetAgentGuardBundleHandler(fn func(payload string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAgentGuardBundle = fn
}

func (m *ConfigManager) ApplyConfigSync(sync *pb.ConfigSync) error {
	if sync == nil {
		return fmt.Errorf("config sync is nil")
	}
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
	case "agent_guard_bundle":
		if sync.Action != "full_sync" {
			return fmt.Errorf("agent_guard_bundle only supports full_sync")
		}
		if m.onAgentGuardBundle == nil {
			return fmt.Errorf("agent_guard_bundle handler is not configured")
		}
		return m.onAgentGuardBundle(sync.Payload)
	default:
		return fmt.Errorf("unknown config type: %s", sync.ConfigType)
	}
}

func (m *ConfigManager) ApplyDetectionPackageCommand(cmd *pb.DetectionPackageCommand) error {
	if cmd == nil {
		return fmt.Errorf("detection package command is nil")
	}
	m.mu.RLock()
	handler := m.onDetectionPackage
	m.mu.RUnlock()
	if handler == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]interface{}{
		"action":        cmd.Action,
		"package_id":    cmd.PackageId,
		"version":       cmd.Version,
		"package_url":   cmd.PackageUrl,
		"signature_url": cmd.SignatureUrl,
		"package_size":  cmd.PackageSize,
	})
	if err != nil {
		return fmt.Errorf("marshal detection package command: %w", err)
	}
	return handler(cmd.Action, string(payload))
}

func (m *ConfigManager) ApplyAllowlistUpdate(update *pb.AllowlistUpdate) error {
	if update == nil {
		return fmt.Errorf("allowlist update is nil")
	}
	m.mu.RLock()
	handler := m.onAllowlistUpdate
	m.mu.RUnlock()
	if handler == nil {
		return nil
	}
	payload := update.AllowlistJson
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(update.AllowlistJson), &config); err == nil {
		if _, exists := config["version"]; !exists && update.Version != "" {
			if version, err := strconv.ParseInt(update.Version, 10, 64); err == nil {
				config["version"] = version
				if payloadBytes, err := json.Marshal(config); err == nil {
					payload = string(payloadBytes)
				}
			}
		}
	}
	return handler(payload)
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
