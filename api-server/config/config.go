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
	AgentGuard  AgentGuardConfig  `mapstructure:"agent_guard"`
	SelfHealing SelfHealingConfig `mapstructure:"self_healing"`
	Kafka       KafkaConfig       `mapstructure:"kafka"`
	GRPC        GRPCConfig        `mapstructure:"grpc"`
}

type ServerConfig struct {
	HTTPPort         int    `mapstructure:"http_port"`
	GRPCPort         int    `mapstructure:"grpc_port"`
	AgentHubPort     int    `mapstructure:"agent_hub_port"` // Port for Agent Hub (Server service agent gRPC port)
	ExternalIP       string `mapstructure:"external_ip"`
	ExternalGRPCPort int    `mapstructure:"external_grpc_port"`
}

type KafkaConfig struct {
	Brokers []string `mapstructure:"brokers"`
	GroupID string   `mapstructure:"group_id"`
}

type GRPCConfig struct {
	ServerAddress string `mapstructure:"server_address"` // Server service gRPC address (e.g., server:19090)
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
	// ArtifactBaseURL is the HTTP base URL agents use to download objects
	// (e.g. "http://localhost:9000/agent-artifacts"). When empty it is
	// derived from Endpoint at startup.
	ArtifactBaseURL string `mapstructure:"artifact_base_url"`
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

// AgentGuardConfig keeps every V6.2 control-plane write capability behind an
// explicit flag. Read routes remain registered so operators can inspect
// unsupported, monitor-only, and disabled states without enabling protection.
type AgentGuardConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	PolicyWriteEnabled bool   `mapstructure:"policy_write_enabled"`
	AnalysisEnabled    bool   `mapstructure:"analysis_enabled"`
	ActionEnabled      bool   `mapstructure:"action_enabled"`
	ToolAdapterEnabled bool   `mapstructure:"tool_adapter_enabled"`
	ScopeSigningKey    string `mapstructure:"scope_signing_key"`
}

type SelfHealingConfig struct {
	MaxRetries int  `mapstructure:"max_retries"`
	Enabled    bool `mapstructure:"enabled"`
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
	if artifactBaseURL := getEnv("MINIO_ARTIFACT_BASE_URL"); artifactBaseURL != "" {
		cfg.MinIO.ArtifactBaseURL = artifactBaseURL
	}
	if serverHTTPPort := getEnvInt("SERVER_HTTP_PORT"); serverHTTPPort != 0 {
		cfg.Server.HTTPPort = serverHTTPPort
	}
	if serverGRPCPort := getEnvInt("SERVER_GRPC_PORT"); serverGRPCPort != 0 {
		cfg.Server.GRPCPort = serverGRPCPort
	}
	if agentHubPort := getEnvInt("AGENT_HUB_PORT"); agentHubPort != 0 {
		cfg.Server.AgentHubPort = agentHubPort
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
	if enabled, ok := getEnvBool("AGENT_GUARD_ENABLED"); ok {
		cfg.AgentGuard.Enabled = enabled
	}
	if enabled, ok := getEnvBool("AGENT_GUARD_POLICY_WRITE_ENABLED"); ok {
		cfg.AgentGuard.PolicyWriteEnabled = enabled
	}
	if enabled, ok := getEnvBool("AGENT_GUARD_ANALYSIS_ENABLED"); ok {
		cfg.AgentGuard.AnalysisEnabled = enabled
	}
	if enabled, ok := getEnvBool("AGENT_GUARD_ACTION_ENABLED"); ok {
		cfg.AgentGuard.ActionEnabled = enabled
	}
	if enabled, ok := getEnvBool("AGENT_GUARD_TOOL_ADAPTER_ENABLED"); ok {
		cfg.AgentGuard.ToolAdapterEnabled = enabled
	}
	if signingKey := getEnv("AGENT_GUARD_SCOPE_SIGNING_KEY"); signingKey != "" {
		cfg.AgentGuard.ScopeSigningKey = signingKey
	}
	if grpcServerAddr := getEnv("GRPC_SERVER_ADDRESS"); grpcServerAddr != "" {
		cfg.GRPC.ServerAddress = grpcServerAddr
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

func getEnvBool(key string) (bool, bool) {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return false, false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return parsed, true
}

func Get() *Config {
	return globalConfig
}
