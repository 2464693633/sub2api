package repository

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisAccountHealthStore 是 service.AccountHealthStore 的 Redis 实现。
// service 层禁止直接依赖 redis（depguard），健康度指标的 KV 访问统一收口到这里。
type RedisAccountHealthStore struct {
	rdb *redis.Client
}

// NewAccountHealthStore 创建健康度指标的 Redis store；redisClient 为 nil 时方法返回错误。
func NewAccountHealthStore(redisClient *redis.Client) *RedisAccountHealthStore {
	return &RedisAccountHealthStore{rdb: redisClient}
}

func (s *RedisAccountHealthStore) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) error {
	if s.rdb == nil {
		return errHealthStoreUnavailable
	}
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisAccountHealthStore) Get(ctx context.Context, key string) (string, error) {
	if s.rdb == nil {
		return "", errHealthStoreUnavailable
	}
	return s.rdb.Get(ctx, key).Result()
}

func (s *RedisAccountHealthStore) SetWithTTL(ctx context.Context, key, value string, ttl time.Duration) error {
	if s.rdb == nil {
		return errHealthStoreUnavailable
	}
	return s.rdb.Set(ctx, key, value, ttl).Err()
}

func (s *RedisAccountHealthStore) MGet(ctx context.Context, keys []string) (map[string]string, error) {
	if s.rdb == nil {
		return nil, errHealthStoreUnavailable
	}
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(values))
	for i, v := range values {
		if str, ok := v.(string); ok {
			out[keys[i]] = str
		}
	}
	return out, nil
}

var errHealthStoreUnavailable = errHealthStoreUnavailableError{}

type errHealthStoreUnavailableError struct{}

func (errHealthStoreUnavailableError) Error() string { return "account health store unavailable" }
