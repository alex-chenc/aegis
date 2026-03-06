package config

import (
	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
)

type Config struct {
	ServerAddr string `toml:"server_addr"`
	GRPCAddr   string `toml:"grpc_addr"`
	HostID     string `toml:"host_id"`
	LogLevel   string `toml:"log_level"`
	LogFile    string `toml:"log_file"`
}

const configPath = "/etc/baseline-agent/config.toml"

func Load() (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Generate HostID if empty
	if cfg.HostID == "" {
		cfg.HostID = uuid.New().String()
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
