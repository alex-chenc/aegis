package config

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	ServerAddr      string `toml:"ServerAddr"`
	AuthToken       string `toml:"AuthToken"`
	HostID          string `toml:"HostID"`
	EventBufferSize int    `toml:"EventBufferSize"`
	RuleDir         string `toml:"RuleDir"`
	QuarantineDir   string `toml:"QuarantineDir"`
	LogLevel        string `toml:"LogLevel"`
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

	if updated {
		if err := saveConfig(&cfg); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
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
