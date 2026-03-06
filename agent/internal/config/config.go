package config

import (
	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
)

// Config Agent 配置结构
type Config struct {
	ServerAddr string `toml:"ServerAddr"`
	AuthToken  string `toml:"AuthToken"`
	HostID     string `toml:"HostID"`
}

const configPath = "/etc/baseline-agent/config.toml"

// LoadConfig 加载配置文件，如果 HostID 为空则生成 UUID 并回写
func LoadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 如果 HostID 为空，生成新的 UUID 并回写
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
