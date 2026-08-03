package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	ServerAddr                       string `toml:"ServerAddr"`
	APIServerAddr                    string `toml:"APIServerAddr"`
	AuthToken                        string `toml:"AuthToken"`
	HostID                           string `toml:"HostID"`
	EventBufferSize                  int    `toml:"EventBufferSize"`
	RuleDir                          string `toml:"RuleDir"`
	QuarantineDir                    string `toml:"QuarantineDir"`
	LogLevel                         string `toml:"LogLevel"`
	AgentGuardEnabled                bool   `toml:"AgentGuardEnabled"`
	AgentGuardBehaviorMonitorEnabled bool   `toml:"AgentGuardBehaviorMonitorEnabled"`
	AgentGuardToolAdapterEnabled     bool   `toml:"AgentGuardToolAdapterEnabled"`
	AgentGuardToolSourceManifest     string `toml:"AgentGuardToolSourceManifest"`
	AgentGuardToolHookSocket         string `toml:"AgentGuardToolHookSocket"`
	AgentGuardEnforcementEnabled     bool   `toml:"AgentGuardEnforcementEnabled"`
	AgentGuardFreezeEnabled          bool   `toml:"AgentGuardFreezeEnabled"`
	AgentGuardStateDir               string `toml:"AgentGuardStateDir"`
	AgentGuardSpoolCapacity          int    `toml:"AgentGuardSpoolCapacity"`
	AgentGuardReconcileSeconds       int    `toml:"AgentGuardReconcileSeconds"`
}

const configPath = "/etc/aegis-agent/config.toml"

func LoadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	updated := false

	if cfg.HostID == "" {
		cfg.HostID = uuid.New().String()
		updated = true
	}

	if cfg.EventBufferSize == 0 {
		cfg.EventBufferSize = 10000
		updated = true
	}

	if cfg.APIServerAddr == "" {
		cfg.APIServerAddr = DeriveAPIServerAddr(cfg.ServerAddr)
		updated = true
	}

	if cfg.RuleDir == "" {
		cfg.RuleDir = "/etc/aegis-agent/rules"
		updated = true
	}

	if cfg.QuarantineDir == "" {
		cfg.QuarantineDir = "/var/quarantine"
		updated = true
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
		updated = true
	}

	if cfg.AgentGuardStateDir == "" {
		cfg.AgentGuardStateDir = "/var/lib/aegis/agent-guard"
		updated = true
	}

	if cfg.AgentGuardSpoolCapacity <= 0 {
		cfg.AgentGuardSpoolCapacity = 4096
		updated = true
	}

	if cfg.AgentGuardReconcileSeconds <= 0 {
		cfg.AgentGuardReconcileSeconds = 30
		updated = true
	}

	if updated {
		if err := saveConfig(&cfg); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

func UpdateHostID(cfg *Config, hostID string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	hostID = strings.TrimSpace(hostID)
	if _, err := uuid.Parse(hostID); err != nil {
		return fmt.Errorf("invalid canonical host ID: %w", err)
	}
	if cfg.HostID == hostID {
		return nil
	}
	cfg.HostID = hostID
	return saveConfig(cfg)
}

func DeriveAPIServerAddr(serverAddr string) string {
	host, _, err := net.SplitHostPort(serverAddr)
	if err != nil || host == "" {
		host = serverAddr
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, "8082")
}

func saveConfig(cfg *Config) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
