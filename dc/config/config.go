package config

import (
	"os"
	"strconv"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Kafka       KafkaConfig      `mapstructure:"kafka"`
	LLM         LLMConfig        `mapstructure:"llm"`
	Aggregation AggregationConfig `mapstructure:"aggregation"`
	Alert       AlertConfig      `mapstructure:"alert"`
}

type ServerConfig struct {
	GRPCPort int `mapstructure:"grpc_port"`
}

type DatabaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"dbname"`
	SSLMode         string `mapstructure:"sslmode"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

type KafkaConfig struct {
	Brokers []string `mapstructure:"brokers"`
	GroupID string   `mapstructure:"group_id"`
	Topic   string   `mapstructure:"topic"`
}

type LLMConfig struct {
	APIKey         string `mapstructure:"api_key"`
	BaseURL        string `mapstructure:"base_url"`
	ModelName      string `mapstructure:"model_name"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	MaxRetries     int    `mapstructure:"max_retries"`
}

type AggregationConfig struct {
	WindowSizeSeconds int `mapstructure:"window_size_seconds"`
	MaxEventsPerWindow int `mapstructure:"max_events_per_window"`
}

type AlertConfig struct {
	SeverityThreshold string `mapstructure:"severity_threshold"`
	AutoBlock         bool   `mapstructure:"auto_block"`
}

var globalConfig *Config

func Load(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	overrideFromEnv(&cfg)

	globalConfig = &cfg
	return &cfg, nil
}

func overrideFromEnv(cfg *Config) {
	if dbHost := getEnv("DATABASE_HOST"); dbHost != "" {
		cfg.Database.Host = dbHost
	}
	if dbPort := getEnvInt("DATABASE_PORT"); dbPort != 0 {
		cfg.Database.Port = dbPort
	}
	if dbUser := getEnv("DATABASE_USER"); dbUser != "" {
		cfg.Database.User = dbUser
	}
	if dbPassword := getEnv("DATABASE_PASSWORD"); dbPassword != "" {
		cfg.Database.Password = dbPassword
	}
	if dbDBName := getEnv("DATABASE_DBNAME"); dbDBName != "" {
		cfg.Database.DBName = dbDBName
	}
	if kafkaBrokers := getEnv("KAFKA_BROKERS"); kafkaBrokers != "" {
		cfg.Kafka.Brokers = []string{kafkaBrokers}
	}
	if kafkaGroupID := getEnv("KAFKA_GROUP_ID"); kafkaGroupID != "" {
		cfg.Kafka.GroupID = kafkaGroupID
	}
	if grpcPort := getEnvInt("GRPC_SERVER_PORT"); grpcPort != 0 {
		cfg.Server.GRPCPort = grpcPort
	}
	if llmAPIKey := getEnv("LLM_API_KEY"); llmAPIKey != "" {
		cfg.LLM.APIKey = llmAPIKey
	}
	if llmBaseURL := getEnv("LLM_BASE_URL"); llmBaseURL != "" {
		cfg.LLM.BaseURL = llmBaseURL
	}
	if llmModelName := getEnv("LLM_MODEL_NAME"); llmModelName != "" {
		cfg.LLM.ModelName = llmModelName
	}
}

func getEnv(key string) string {
	if val, exists := os.LookupEnv(key); exists {
		return val
	}
	return ""
}

func getEnvInt(key string) int {
	val := getEnv(key)
	if val == "" {
		return 0
	}
	i, _ := strconv.Atoi(val)
	return i
}

func Get() *Config {
	return globalConfig
}