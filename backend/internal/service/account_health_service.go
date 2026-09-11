package service

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// 账号健康度指标（Redis 滚动窗口）：
//   - 首字延迟 EWMA：acct_health:ft:{id}，每次成功请求按 α=0.3 平滑，TTL 60 分钟
//   - 成功/失败计数：acct_health:ok:{id} / acct_health:fail:{id}，INCR + 10 分钟 TTL
//
// 数据只用于调度偏好与展示，允许极端并发下的轻微竞态（GET→计算→SET），
// 不需要 Lua 原子性。无数据 = 从未被调用过或窗口已过期，调度时视为中性。
const (
	accountHealthFTKeyPrefix   = "acct_health:ft:"
	accountHealthOKKeyPrefix   = "acct_health:ok:"
	accountHealthFailKeyPrefix = "acct_health:fail:"
	accountHealthFTTTL         = 60 * time.Minute
	accountHealthCounterTTL    = 10 * time.Minute
	accountHealthEWMAAlpha     = 0.3
)

// AccountHealthSnapshot 是单个账号在滚动窗口内的健康度快照。
type AccountHealthSnapshot struct {
	// AvgFirstTokenMs 首字延迟 EWMA（毫秒）；nil 表示窗口内没有流式成功样本。
	AvgFirstTokenMs *int64 `json:"avg_first_token_ms,omitempty"`
	// OK 窗口内成功次数。
	OK int64 `json:"ok"`
	// Failed 窗口内失败次数（failover 切换账号的次数）。
	Failed int64 `json:"failed"`
}

// AccountHealthService 维护账号健康度的 Redis 滚动窗口指标。
type AccountHealthService struct {
	rdb *redis.Client
}

// NewAccountHealthService 创建健康度服务；redisClient 为 nil 时所有方法为 no-op。
func NewAccountHealthService(redisClient *redis.Client) *AccountHealthService {
	return &AccountHealthService{rdb: redisClient}
}

// Available 报告服务是否具备存储后端。
func (s *AccountHealthService) Available() bool {
	return s != nil && s.rdb != nil
}

// RecordSuccess 记录一次成功请求；firstTokenMs 非空时更新首字延迟 EWMA。
func (s *AccountHealthService) RecordSuccess(ctx context.Context, accountID int64, firstTokenMs *int) {
	if !s.Available() || accountID <= 0 {
		return
	}
	okKey := accountHealthOKKeyPrefix + strconv.FormatInt(accountID, 10)
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, okKey)
	pipe.Expire(ctx, okKey, accountHealthCounterTTL)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return
	}
	if firstTokenMs != nil && *firstTokenMs > 0 {
		s.updateFirstTokenEWMA(ctx, accountID, float64(*firstTokenMs))
	}
}

// RecordFailure 记录一次账号级失败（failover 切换账号）。
func (s *AccountHealthService) RecordFailure(ctx context.Context, accountID int64) {
	if !s.Available() || accountID <= 0 {
		return
	}
	key := accountHealthFailKeyPrefix + strconv.FormatInt(accountID, 10)
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, accountHealthCounterTTL)
	_, _ = pipe.Exec(ctx)
}

// Snapshot 批量读取账号健康度；读取失败的账号不出现在返回值中。
func (s *AccountHealthService) Snapshot(ctx context.Context, accountIDs []int64) map[int64]AccountHealthSnapshot {
	if !s.Available() || len(accountIDs) == 0 {
		return nil
	}
	pipe := s.rdb.Pipeline()
	cmds := make(map[int64][3]*redis.StringCmd, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			continue
		}
		suffix := strconv.FormatInt(id, 10)
		cmds[id] = [3]*redis.StringCmd{
			pipe.Get(ctx, accountHealthFTKeyPrefix+suffix),
			pipe.Get(ctx, accountHealthOKKeyPrefix+suffix),
			pipe.Get(ctx, accountHealthFailKeyPrefix+suffix),
		}
	}
	_, _ = pipe.Exec(ctx)

	out := make(map[int64]AccountHealthSnapshot, len(cmds))
	for id, cmds3 := range cmds {
		snapshot := AccountHealthSnapshot{}
		if v, err := cmds3[0].Result(); err == nil {
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				snapshot.AvgFirstTokenMs = &ms
			}
		}
		if v, err := cmds3[1].Result(); err == nil {
			snapshot.OK, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, err := cmds3[2].Result(); err == nil {
			snapshot.Failed, _ = strconv.ParseInt(v, 10, 64)
		}
		if snapshot.AvgFirstTokenMs != nil || snapshot.OK > 0 || snapshot.Failed > 0 {
			out[id] = snapshot
		}
	}
	return out
}

func (s *AccountHealthService) updateFirstTokenEWMA(ctx context.Context, accountID int64, sample float64) {
	key := accountHealthFTKeyPrefix + strconv.FormatInt(accountID, 10)
	old, err := s.rdb.Get(ctx, key).Result()
	next := sample
	if err == nil {
		if oldVal, parseErr := strconv.ParseFloat(old, 64); parseErr == nil && oldVal > 0 {
			next = (1-accountHealthEWMAAlpha)*oldVal + accountHealthEWMAAlpha*sample
		}
	}
	s.rdb.Set(ctx, key, strconv.FormatFloat(next, 'f', 0, 64), accountHealthFTTTL)
}
