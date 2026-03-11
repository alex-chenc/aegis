package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"baseline-system/config"
	"baseline-system/pkg/logger"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisClient(cfg *config.RedisConfig) (*RedisClient, error) {
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// 连通性验证
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Error("failed to connect to Redis",
			zap.Error(err),
			zap.String("addr", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		)
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis connected successfully",
		zap.String("addr", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		zap.Int("db", cfg.DB),
		zap.Int("pool_size", cfg.PoolSize),
	)

	return &RedisClient{
		client: rdb,
		ctx:    ctx,
	}, nil
}

func (r *RedisClient) Close() error {
	logger.Info("closing Redis connection")
	return r.client.Close()
}

func (r *RedisClient) Client() *redis.Client {
	return r.client
}

func (r *RedisClient) Context() context.Context {
	return r.ctx
}

// ============================================================================
// Agent Heartbeat Management
// Key pattern: agent:heartbeat:{host_id}
// TTL: 90 seconds
// ============================================================================

const HeartbeatTTL = 90 * time.Second
const HeartbeatKeyPrefix = "agent:heartbeat:"

func heartbeatKey(hostID string) string {
	return fmt.Sprintf("%s%s", HeartbeatKeyPrefix, hostID)
}

// SetHeartbeat sets the heartbeat timestamp for a host
func (r *RedisClient) SetHeartbeat(hostID string) error {
	key := heartbeatKey(hostID)
	err := r.client.Set(r.ctx, key, time.Now().Unix(), HeartbeatTTL).Err()
	if err != nil {
		logger.Error("failed to set heartbeat",
			zap.Error(err),
			zap.String("host_id", hostID),
			zap.String("key", key),
		)
		return err
	}
	logger.Debug("heartbeat set",
		zap.String("host_id", hostID),
		zap.String("key", key),
	)
	return nil
}

// IsOnline checks if a host is online by checking if heartbeat key exists
func (r *RedisClient) IsOnline(hostID string) (bool, error) {
	key := heartbeatKey(hostID)
	exists, err := r.client.Exists(r.ctx, key).Result()
	if err != nil {
		logger.Error("failed to check heartbeat",
			zap.Error(err),
			zap.String("host_id", hostID),
		)
		return false, err
	}
	return exists > 0, nil
}

// BatchCheckOnline checks online status for multiple hosts
func (r *RedisClient) BatchCheckOnline(hostIDs []string) (map[string]bool, error) {
	if len(hostIDs) == 0 {
		return make(map[string]bool), nil
	}
	keys := make([]string, len(hostIDs))
	for i, hostID := range hostIDs {
		keys[i] = heartbeatKey(hostID)
	}

	results, err := r.client.MGet(r.ctx, keys...).Result()
	if err != nil {
		logger.Error("failed to batch check heartbeat",
			zap.Error(err),
			zap.Int("count", len(hostIDs)),
		)
		return nil, err
	}

	onlineMap := make(map[string]bool)
	for i, result := range results {
		onlineMap[hostIDs[i]] = (result != nil)
	}

	logger.Debug("batch heartbeat check completed",
		zap.Int("total", len(hostIDs)),
		zap.Int("online", countTrue(onlineMap)),
	)
	return onlineMap, nil
}

func countTrue(m map[string]bool) int {
	count := 0
	for _, v := range m {
		if v {
			count++
		}
	}
	return count
}

// ============================================================================
// Template Parse Status Management
// Key pattern: template:parse:status:{template_id}
// Type: HASH
// TTL: 1 hour
// Fields: status, progress, message
// ============================================================================

const TemplateStatusTTL = 1 * time.Hour
const TemplateStatusKeyPrefix = "template:parse:status:"

func templateStatusKey(templateID string) string {
	return fmt.Sprintf("%s%s", TemplateStatusKeyPrefix, templateID)
}

// SetParseStatus sets the parsing status for a template
func (r *RedisClient) SetParseStatus(templateID, status string, progress int, message string) error {
	key := templateStatusKey(templateID)
	err := r.client.HSet(r.ctx, key,
		"status", status,
		"progress", strconv.Itoa(progress),
		"message", message,
	).Err()
	if err != nil {
		logger.Error("failed to set template parse status",
			zap.Error(err),
			zap.String("template_id", templateID),
		)
		return err
	}

	// Set TTL
	r.client.Expire(r.ctx, key, TemplateStatusTTL)

	logger.Debug("template parse status set",
		zap.String("template_id", templateID),
		zap.String("status", status),
		zap.Int("progress", progress),
	)
	return nil
}

// GetParseStatus gets the parsing status for a template
func (r *RedisClient) GetParseStatus(templateID string) (status string, progress int, message string, err error) {
	key := templateStatusKey(templateID)
	result, err := r.client.HGetAll(r.ctx, key).Result()
	if err != nil {
		logger.Error("failed to get template parse status",
			zap.Error(err),
			zap.String("template_id", templateID),
		)
		return "", 0, "", err
	}

	status = result["status"]
	progress, _ = strconv.Atoi(result["progress"])
	message = result["message"]

	return status, progress, message, nil
}

// DeleteParseStatus deletes the parsing status for a template
func (r *RedisClient) DeleteParseStatus(templateID string) error {
	key := templateStatusKey(templateID)
	err := r.client.Del(r.ctx, key).Err()
	if err != nil {
		logger.Warn("failed to delete template parse status",
			zap.Error(err),
			zap.String("template_id", templateID),
		)
		return err
	}
	logger.Debug("template parse status deleted", zap.String("template_id", templateID))
	return nil
}

// ============================================================================
// Task Status Management
// Key pattern: task:status:{task_id}
// Type: HASH
// TTL: 2 hours
// Fields: status, stdout, stderr, exit_code
// ============================================================================

const TaskStatusTTL = 2 * time.Hour
const TaskStatusKeyPrefix = "task:status:"

func taskStatusKey(taskID string) string {
	return fmt.Sprintf("%s%s", TaskStatusKeyPrefix, taskID)
}

// SetTaskStatus sets the status for a task
func (r *RedisClient) SetTaskStatus(taskID, status string) error {
	key := taskStatusKey(taskID)
	err := r.client.HSet(r.ctx, key, "status", status).Err()
	if err != nil {
		logger.Error("failed to set task status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return err
	}

	r.client.Expire(r.ctx, key, TaskStatusTTL)

	logger.Debug("task status set",
		zap.String("task_id", taskID),
		zap.String("status", status),
	)
	return nil
}

// AppendTaskLog appends a log line to task logs list
func (r *RedisClient) AppendTaskLog(taskID, logLine string) error {
	key := fmt.Sprintf("task:logs:%s", taskID)
	err := r.client.RPush(r.ctx, key, logLine).Err()
	if err != nil {
		logger.Error("failed to append task log",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return err
	}

	r.client.Expire(r.ctx, key, TaskStatusTTL)

	logger.Debug("task log appended",
		zap.String("task_id", taskID),
		zap.Int("length", len(logLine)),
	)
	return nil
}

// GetTaskLogs gets task logs with offset
func (r *RedisClient) GetTaskLogs(taskID string, offset int64) ([]string, error) {
	key := fmt.Sprintf("task:logs:%s", taskID)
	logs, err := r.client.LRange(r.ctx, key, offset, -1).Result()
	if err != nil {
		logger.Error("failed to get task logs",
			zap.Error(err),
			zap.String("task_id", taskID),
			zap.Int64("offset", offset),
		)
		return nil, err
	}

	logger.Debug("task logs retrieved",
		zap.String("task_id", taskID),
		zap.Int("count", len(logs)),
	)
	return logs, nil
}

// ============================================================================
// LLM Config Cache
// Key pattern: config:llm
// Type: HASH
// TTL: No expiration (persistent)
// Fields: api_key, base_url, model_name
// ============================================================================

const LLMConfigKey = "config:llm"

// SetLLMConfig caches the LLM configuration
func (r *RedisClient) SetLLMConfig(apiKey, baseURL, modelName string) error {
	err := r.client.HSet(r.ctx, LLMConfigKey,
		"api_key", apiKey,
		"base_url", baseURL,
		"model_name", modelName,
	).Err()
	if err != nil {
		logger.Error("failed to set LLM config",
			zap.Error(err),
		)
		return err
	}

	logger.Debug("LLM config cached",
		zap.String("model_name", modelName),
	)
	return nil
}

// GetLLMConfig gets the cached LLM configuration
func (r *RedisClient) GetLLMConfig() (apiKey, baseURL, modelName string, err error) {
	result, err := r.client.HGetAll(r.ctx, LLMConfigKey).Result()
	if err != nil {
		logger.Error("failed to get LLM config",
			zap.Error(err),
		)
		return "", "", "", err
	}

	apiKey = result["api_key"]
	baseURL = result["base_url"]
	modelName = result["model_name"]

	return apiKey, baseURL, modelName, nil
}

// ============================================================================
// Self-Healing Status Management
// Key pattern: self_healing:{task_id}
// Type: HASH
// TTL: 1 hour
// Fields: attempt, status, last_error
// ============================================================================

const SelfHealingTTL = 1 * time.Hour
const SelfHealingKeyPrefix = "self_healing:"

func selfHealingKey(taskID string) string {
	return fmt.Sprintf("%s%s", SelfHealingKeyPrefix, taskID)
}

// SetHealingStatus sets the self-healing status for a task
func (r *RedisClient) SetHealingStatus(taskID, status string, attempt int, lastError string) error {
	key := selfHealingKey(taskID)
	err := r.client.HSet(r.ctx, key,
		"status", status,
		"attempt", strconv.Itoa(attempt),
		"last_error", lastError,
	).Err()
	if err != nil {
		logger.Error("failed to set healing status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return err
	}

	r.client.Expire(r.ctx, key, SelfHealingTTL)

	logger.Debug("healing status set",
		zap.String("task_id", taskID),
		zap.String("status", status),
		zap.Int("attempt", attempt),
	)
	return nil
}

// GetHealingStatus gets the self-healing status for a task
func (r *RedisClient) GetHealingStatus(taskID string) (status string, attempt int, lastError string, err error) {
	key := selfHealingKey(taskID)
	result, err := r.client.HGetAll(r.ctx, key).Result()
	if err != nil {
		logger.Error("failed to get healing status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return "", 0, "", err
	}

	status = result["status"]
	attempt, _ = strconv.Atoi(result["attempt"])
	lastError = result["last_error"]

	return status, attempt, lastError, nil
}
