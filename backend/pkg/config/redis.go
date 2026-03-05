package config

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisClient(cfg RedisConfig) *RedisClient {
	return &RedisClient{
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
		ctx: context.Background(),
	}
}

func (r *RedisClient) GetClient() *redis.Client {
	return r.client
}

func (r *RedisClient) SetHostOnline(hostID string) error {
	key := fmt.Sprintf("agent:online:%s", hostID)
	return r.client.Set(r.ctx, key, "1", 70*time.Second).Err()
}

func (r *RedisClient) SetHostOffline(hostID string) error {
	key := fmt.Sprintf("agent:online:%s", hostID)
	return r.client.Del(r.ctx, key).Err()
}

func (r *RedisClient) IsHostOnline(hostID string) bool {
	key := fmt.Sprintf("agent:online:%s", hostID)
	val, err := r.client.Get(r.ctx, key).Result()
	if err != nil {
		return false
	}
	return val == "1"
}

func (r *RedisClient) SetHostHeartbeat(hostID string, ttl time.Duration) error {
	key := fmt.Sprintf("agent:heartbeat:%s", hostID)
	return r.client.Set(r.ctx, key, time.Now().Unix(), ttl).Err()
}

func (r *RedisClient) UpdateHostMetrics(hostID string, cpuLoad, memUsage float32) error {
	key := fmt.Sprintf("agent:metrics:%s", hostID)
	return r.client.HSet(r.ctx, key, map[string]interface{}{
		"cpu_load":       cpuLoad,
		"mem_usage":      memUsage,
		"last_heartbeat": time.Now().Unix(),
	}).Err()
}

func (r *RedisClient) GetHostMetrics(hostID string) (map[string]string, error) {
	key := fmt.Sprintf("agent:metrics:%s", hostID)
	return r.client.HGetAll(r.ctx, key).Result()
}

func (r *RedisClient) GetOnlineHosts() ([]string, error) {
	keys, err := r.client.Keys(r.ctx, "agent:online:*").Result()
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0, len(keys))
	for _, key := range keys {
		hosts = append(hosts, key[13:])
	}
	return hosts, nil
}

func (r *RedisClient) Ping() error {
	return r.client.Ping(r.ctx).Err()
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
