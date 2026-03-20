package config

import (
	"os"
	"strconv"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Redis       RedisConfig       `mapstructure:"redis"`
	MinIO       MinIOConfig       `mapstructure:"minio"`
	LLM         LLMConfig         `mapstructure:"llm"`
	Agent       AgentConfig       `mapstructure:"agent"`
	SelfHealing SelfHealingConfig `mapstructure:"self_healing"`
	Kafka       KafkaConfig       `mapstructure:"kafka"`
}

type ServerConfig struct {
	HTTPPort         int    `mapstructure:"http_port"`
	GRPCPort         int    `mapstructure:"grpc_port"`
	ExternalIP       string `mapstructure:"external_ip"`
	ExternalGRPCPort int    `mapstructure:"external_grpc_port"` // 对外暴露的 gRPC 端口，用于 Agent 连接
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

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type MinIOConfig struct {
	Endpoint  string   `mapstructure:"endpoint"`
	AccessKey string   `mapstructure:"access_key"`
	SecretKey string   `mapstructure:"secret_key"`
	UseSSL    bool     `mapstructure:"use_ssl"`
	Buckets   []string `mapstructure:"buckets"`
}

type LLMConfig struct {
	APIKey         string `mapstructure:"api_key"`
	BaseURL        string `mapstructure:"base_url"`
	ModelName      string `mapstructure:"model_name"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	MaxRetries     int    `mapstructure:"max_retries"`
}

type AgentConfig struct {
	AuthToken               string `mapstructure:"auth_token"`
	HeartbeatTimeoutSeconds int    `mapstructure:"heartbeat_timeout_seconds"`
	ScriptTimeoutSeconds    int    `mapstructure:"script_timeout_seconds"`
}

type SelfHealingConfig struct {
	MaxRetries int  `mapstructure:"max_retries"`
	Enabled    bool `mapstructure:"enabled"`
}

type KafkaConfig struct {
	Brokers []string `mapstructure:"brokers"`
	GroupID string   `mapstructure:"group_id"`
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
	if redisHost := getEnv("REDIS_HOST"); redisHost != "" {
		cfg.Redis.Host = redisHost
	}
	if redisPort := getEnvInt("REDIS_PORT"); redisPort != 0 {
		cfg.Redis.Port = redisPort
	}
	if redisPassword := getEnv("REDIS_PASSWORD"); redisPassword != "" {
		cfg.Redis.Password = redisPassword
	}
	if minioEndpoint := getEnv("MINIO_ENDPOINT"); minioEndpoint != "" {
		cfg.MinIO.Endpoint = minioEndpoint
	}
	if minioAccessKey := getEnv("MINIO_ACCESS_KEY"); minioAccessKey != "" {
		cfg.MinIO.AccessKey = minioAccessKey
	}
	if minioSecretKey := getEnv("MINIO_SECRET_KEY"); minioSecretKey != "" {
		cfg.MinIO.SecretKey = minioSecretKey
	}
	if serverHTTPPort := getEnvInt("SERVER_HTTP_PORT"); serverHTTPPort != 0 {
		cfg.Server.HTTPPort = serverHTTPPort
	}
	if serverGRPCPort := getEnvInt("SERVER_GRPC_PORT"); serverGRPCPort != 0 {
		cfg.Server.GRPCPort = serverGRPCPort
	}
	if externalIP := getEnv("SERVER_EXTERNAL_IP"); externalIP != "" {
		cfg.Server.ExternalIP = externalIP
	}
	if externalGRPCPort := getEnvInt("SERVER_EXTERNAL_GRPC_PORT"); externalGRPCPort != 0 {
		cfg.Server.ExternalGRPCPort = externalGRPCPort
	}
	if llmAPIKey := getEnv("LLM_API_KEY"); llmAPIKey != "" {
		cfg.LLM.APIKey = llmAPIKey
	}
	if authToken := getEnv("AGENT_AUTH_TOKEN"); authToken != "" {
		cfg.Agent.AuthToken = authToken
	}
	if kafkaBrokers := getEnv("KAFKA_BROKERS"); kafkaBrokers != "" {
		cfg.Kafka.Brokers = []string{kafkaBrokers}
	}
	if kafkaGroupID := getEnv("KAFKA_GROUP_ID"); kafkaGroupID != "" {
		cfg.Kafka.GroupID = kafkaGroupID
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
