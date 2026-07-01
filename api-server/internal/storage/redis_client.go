package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"api-server/config"
	"api-server/internal/model"
	"api-server/pkg/logger"

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

// DeleteHealingStatus deletes the self-healing status for a task
func (r *RedisClient) DeleteHealingStatus(taskID string) error {
	key := selfHealingKey(taskID)
	err := r.client.Del(r.ctx, key).Err()
	if err != nil {
		logger.Error("failed to delete healing status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return err
	}
	logger.Debug("healing status deleted",
		zap.String("task_id", taskID),
	)
	return nil
}

// ============================================================================
// Vulnerability Scan Status Cache
// Key pattern: vulnerability:scan:session:{session_id}
// Type: HASH
// TTL: 2 hours
// ============================================================================

const ScanStatusTTL = 2 * time.Hour
const ScanStatusKeyPrefix = "vulnerability:scan:session:"

func scanStatusKey(scanID string) string {
	return fmt.Sprintf("%s%s", ScanStatusKeyPrefix, scanID)
}

// SetScanStatus caches the vulnerability scan status
func (r *RedisClient) SetScanStatus(scanID string, status *model.ScanStatus) error {
	key := scanStatusKey(scanID)
	data, err := json.Marshal(status)
	if err != nil {
		logger.Error("failed to marshal scan status",
			zap.Error(err),
			zap.String("scan_id", scanID),
		)
		return fmt.Errorf("failed to marshal scan status: %w", err)
	}

	err = r.client.HSet(r.ctx, key, "data", string(data)).Err()
	if err != nil {
		logger.Error("failed to set scan status",
			zap.Error(err),
			zap.String("scan_id", scanID),
		)
		return err
	}

	r.client.Expire(r.ctx, key, ScanStatusTTL)

	logger.Debug("scan status cached",
		zap.String("scan_id", scanID),
		zap.String("status", status.Status),
	)
	return nil
}

// GetScanStatus retrieves the cached vulnerability scan status
func (r *RedisClient) GetScanStatus(scanID string) (*model.ScanStatus, error) {
	key := scanStatusKey(scanID)
	result, err := r.client.HGet(r.ctx, key, "data").Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		logger.Error("failed to get scan status",
			zap.Error(err),
			zap.String("scan_id", scanID),
		)
		return nil, err
	}

	var status model.ScanStatus
	if err := json.Unmarshal([]byte(result), &status); err != nil {
		logger.Error("failed to unmarshal scan status",
			zap.Error(err),
			zap.String("scan_id", scanID),
		)
		return nil, fmt.Errorf("failed to unmarshal scan status: %w", err)
	}

	return &status, nil
}

// DeleteScanStatus deletes the cached vulnerability scan status
func (r *RedisClient) DeleteScanStatus(scanID string) error {
	key := scanStatusKey(scanID)
	err := r.client.Del(r.ctx, key).Err()
	if err != nil {
		logger.Error("failed to delete scan status",
			zap.Error(err),
			zap.String("scan_id", scanID),
		)
		return err
	}
	logger.Debug("scan status deleted", zap.String("scan_id", scanID))
	return nil
}

// ============================================================================
// Host Software (Packages) Cache
// Key pattern: vulnerability:host:packages:{host_id}
// Type: HASH
// TTL: 10 minutes
// ============================================================================

const HostPackagesTTL = 10 * time.Minute
const HostPackagesKeyPrefix = "vulnerability:host:packages:"

func hostPackagesKey(hostID string) string {
	return fmt.Sprintf("%s%s", HostPackagesKeyPrefix, hostID)
}

// SetHostPackages caches the software package list for a host
func (r *RedisClient) SetHostPackages(hostID string, packages []model.SoftwareInfo) error {
	key := hostPackagesKey(hostID)
	data, err := json.Marshal(packages)
	if err != nil {
		logger.Error("failed to marshal host packages",
			zap.Error(err),
			zap.String("host_id", hostID),
		)
		return fmt.Errorf("failed to marshal host packages: %w", err)
	}

	err = r.client.HSet(r.ctx, key, "data", string(data)).Err()
	if err != nil {
		logger.Error("failed to set host packages",
			zap.Error(err),
			zap.String("host_id", hostID),
		)
		return err
	}

	r.client.Expire(r.ctx, key, HostPackagesTTL)

	logger.Debug("host packages cached",
		zap.String("host_id", hostID),
		zap.Int("count", len(packages)),
	)
	return nil
}

// GetHostPackages retrieves the cached software package list for a host
func (r *RedisClient) GetHostPackages(hostID string) ([]model.SoftwareInfo, error) {
	key := hostPackagesKey(hostID)
	result, err := r.client.HGet(r.ctx, key, "data").Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		logger.Error("failed to get host packages",
			zap.Error(err),
			zap.String("host_id", hostID),
		)
		return nil, err
	}

	var packages []model.SoftwareInfo
	if err := json.Unmarshal([]byte(result), &packages); err != nil {
		logger.Error("failed to unmarshal host packages",
			zap.Error(err),
			zap.String("host_id", hostID),
		)
		return nil, fmt.Errorf("failed to unmarshal host packages: %w", err)
	}

	return packages, nil
}

// ============================================================================
// CVE Detail Cache
// Key pattern: vulnerability:cve:{cve_id}
// Type: STRING
// TTL: No expiration
// ============================================================================

const CVEDetailKeyPrefix = "vulnerability:cve:"

func cveDetailKey(cveID string) string {
	return fmt.Sprintf("%s%s", CVEDetailKeyPrefix, cveID)
}

// SetCVEDetail caches raw CVE detail bytes (no expiration)
func (r *RedisClient) SetCVEDetail(cveID string, detail []byte) error {
	key := cveDetailKey(cveID)
	err := r.client.Set(r.ctx, key, detail, 0).Err()
	if err != nil {
		logger.Error("failed to set CVE detail",
			zap.Error(err),
			zap.String("cve_id", cveID),
		)
		return err
	}
	logger.Debug("CVE detail cached",
		zap.String("cve_id", cveID),
		zap.Int("bytes", len(detail)),
	)
	return nil
}

// GetCVEDetail retrieves the cached CVE detail bytes
func (r *RedisClient) GetCVEDetail(cveID string) ([]byte, error) {
	key := cveDetailKey(cveID)
	result, err := r.client.Get(r.ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		logger.Error("failed to get CVE detail",
			zap.Error(err),
			zap.String("cve_id", cveID),
		)
		return nil, err
	}
	return result, nil
}

// ============================================================================
// Script Generation Status Cache
// Key pattern: vuln:gen:{cve_id}:{mode}
// Type: STRING (JSON)
// TTL: 30 minutes
// ============================================================================

const GenerationKeyTTL = 30 * time.Minute
const GenerationKeyPrefix = "vuln:gen:"

func generationKey(cveID, mode string) string {
	return fmt.Sprintf("%s%s:%s", GenerationKeyPrefix, cveID, mode)
}

// GenerationStatus represents the cached generation status
type GenerationStatus struct {
	ScriptID  string   `json:"script_id"`
	Status    string   `json:"status"`
	StartedAt string   `json:"started_at"`
	HostIDs   []string `json:"host_ids"`
	Error     string   `json:"error,omitempty"`
}

// SetGenerationStatus caches the script generation status
func (r *RedisClient) SetGenerationStatus(cveID, mode string, status *GenerationStatus) error {
	key := generationKey(cveID, mode)
	data, err := json.Marshal(status)
	if err != nil {
		logger.Error("failed to marshal generation status",
			zap.Error(err),
			zap.String("cve_id", cveID),
			zap.String("mode", mode),
		)
		return fmt.Errorf("failed to marshal generation status: %w", err)
	}

	err = r.client.Set(r.ctx, key, data, GenerationKeyTTL).Err()
	if err != nil {
		logger.Error("failed to set generation status",
			zap.Error(err),
			zap.String("cve_id", cveID),
			zap.String("mode", mode),
		)
		return err
	}

	logger.Debug("generation status cached",
		zap.String("cve_id", cveID),
		zap.String("mode", mode),
		zap.String("status", status.Status),
	)
	return nil
}

// GetGenerationStatus retrieves the cached script generation status
func (r *RedisClient) GetGenerationStatus(cveID, mode string) (*GenerationStatus, error) {
	key := generationKey(cveID, mode)
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		logger.Error("failed to get generation status",
			zap.Error(err),
			zap.String("cve_id", cveID),
			zap.String("mode", mode),
		)
		return nil, err
	}

	var status GenerationStatus
	if err := json.Unmarshal(data, &status); err != nil {
		logger.Error("failed to unmarshal generation status",
			zap.Error(err),
			zap.String("cve_id", cveID),
			zap.String("mode", mode),
		)
		return nil, fmt.Errorf("failed to unmarshal generation status: %w", err)
	}

	return &status, nil
}

// DeleteGenerationStatus deletes the cached script generation status
func (r *RedisClient) DeleteGenerationStatus(cveID, mode string) error {
	key := generationKey(cveID, mode)
	err := r.client.Del(r.ctx, key).Err()
	if err != nil {
		logger.Error("failed to delete generation status",
			zap.Error(err),
			zap.String("cve_id", cveID),
			zap.String("mode", mode),
		)
		return err
	}
	logger.Debug("generation status deleted",
		zap.String("cve_id", cveID),
		zap.String("mode", mode),
	)
	return nil
}

// ============================================================================
// Login Rate Limiting
// Key pattern: auth:login:fail:{username}
// TTL: 10 minutes
// Max attempts: 3
// ============================================================================

const LoginFailTTL = 10 * time.Minute
const LoginFailKeyPrefix = "auth:login:fail:"
const LoginMaxAttempts = 3

func loginFailKey(username string) string {
	return fmt.Sprintf("%s%s", LoginFailKeyPrefix, username)
}

// IncrementLoginFail increments the failed login attempt counter for a username.
// Returns the new attempt count.
func (r *RedisClient) IncrementLoginFail(username string) (int, error) {
	key := loginFailKey(username)
	count, err := r.client.Incr(r.ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		r.client.Expire(r.ctx, key, LoginFailTTL)
	}
	return int(count), nil
}

// GetLoginFailCount returns the current failed login attempt count for a username.
func (r *RedisClient) GetLoginFailCount(username string) (int, error) {
	key := loginFailKey(username)
	val, err := r.client.Get(r.ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	return val, nil
}

// GetLoginFailTTL returns the remaining TTL for the login fail key.
func (r *RedisClient) GetLoginFailTTL(username string) (time.Duration, error) {
	key := loginFailKey(username)
	ttl, err := r.client.TTL(r.ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if ttl < 0 {
		return 0, nil
	}
	return ttl, nil
}

// ClearLoginFail clears the failed login attempt counter for a username.
func (r *RedisClient) ClearLoginFail(username string) error {
	key := loginFailKey(username)
	return r.client.Del(r.ctx, key).Err()
}

// ============================================================================
// Healing Status Management
// Key pattern: healing:status:{task_id}
// TTL: 10 minutes (longer than 5-minute timeout)
// ============================================================================

const HealingStatusTTL = 10 * time.Minute
const HealingStatusKeyPrefix = "healing:status:"

type HealingStatus struct {
	TaskID         string    `json:"task_id"`
	Status         string    `json:"status"` // healing, healed, failed, timeout
	StartedAt      time.Time `json:"started_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	TotalAttempts  int       `json:"total_attempts"`
	MaxAttempts    int       `json:"max_attempts"`
	LastError      string    `json:"last_error,omitempty"`
	UserSuggestion string    `json:"user_suggestion,omitempty"`
	ScriptType     string    `json:"script_type"`
}

func healingStatusKey(taskID string) string {
	return fmt.Sprintf("%s%s", HealingStatusKeyPrefix, taskID)
}

// SetHealingStatusStruct sets the healing status for a task
func (r *RedisClient) SetHealingStatusStruct(status *HealingStatus) error {
	key := healingStatusKey(status.TaskID)
	status.UpdatedAt = time.Now()
	data, err := json.Marshal(status)
	if err != nil {
		logger.Error("failed to marshal healing status",
			zap.Error(err),
			zap.String("task_id", status.TaskID),
		)
		return fmt.Errorf("failed to marshal healing status: %w", err)
	}
	err = r.client.Set(r.ctx, key, data, HealingStatusTTL).Err()
	if err != nil {
		logger.Error("failed to set healing status",
			zap.Error(err),
			zap.String("task_id", status.TaskID),
		)
		return err
	}
	logger.Debug("healing status set",
		zap.String("task_id", status.TaskID),
		zap.String("status", status.Status),
	)
	return nil
}

// GetHealingStatusStruct gets the healing status for a task
func (r *RedisClient) GetHealingStatusStruct(taskID string) (*HealingStatus, error) {
	key := healingStatusKey(taskID)
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		logger.Error("failed to get healing status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return nil, err
	}

	var status HealingStatus
	if err := json.Unmarshal(data, &status); err != nil {
		logger.Error("failed to unmarshal healing status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return nil, fmt.Errorf("failed to unmarshal healing status: %w", err)
	}

	return &status, nil
}

// DeleteHealingStatusStruct deletes the healing status for a task
func (r *RedisClient) DeleteHealingStatusStruct(taskID string) error {
	key := healingStatusKey(taskID)
	err := r.client.Del(r.ctx, key).Err()
	if err != nil {
		logger.Error("failed to delete healing status",
			zap.Error(err),
			zap.String("task_id", taskID),
		)
		return err
	}
	logger.Debug("healing status deleted",
		zap.String("task_id", taskID),
	)
	return nil
}
