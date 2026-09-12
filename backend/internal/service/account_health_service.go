package service

import (
	"context"
	"strconv"
	"time"
)

// 账号健康度指标（滚动窗口，Redis 由 repository 层的 store 实现承载）：
//   - 首字延迟 EWMA：acct_health:ft:{id}，每次成功请求按 α=0.3 平滑，TTL 60 分钟
//   - 成功/失败计数：acct_health:ok:{id} / acct_health:fail:{id}，INCR + 10 分钟 TTL
//
// 数据只用于调度偏好与展示，允许极端并发下的轻微竞态（GET→计算→SET），
// 不需要 Lua 原子性。无数据 = 从未被调用过或窗口已过期，调度时视为中性。

// AccountHealthStore 是健康度指标的最小 KV 能力抽象（repository 层提供 Redis 实现）。
type AccountHealthStore interface {
	// IncrWithTTL 原子自增并设置过期时间（每次调用都刷新 TTL）。
	IncrWithTTL(ctx context.Context, key string, ttl time.Duration) error
	// Get 返回键值；键不存在返回错误。
	Get(ctx context.Context, key string) (string, error)
	// SetWithTTL 写入键值并设置过期时间。
	SetWithTTL(ctx context.Context, key, value string, ttl time.Duration) error
	// MGet 批量读取；不存在的键不出现在返回 map 中。
	MGet(ctx context.Context, keys []string) (map[string]string, error)
}

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

// AccountHealthService 维护账号健康度的滚动窗口指标。
type AccountHealthService struct {
	store AccountHealthStore
}

// NewAccountHealthService 创建健康度服务；store 为 nil 时所有方法为 no-op。
func NewAccountHealthService(store AccountHealthStore) *AccountHealthService {
	return &AccountHealthService{store: store}
}

// Available 报告服务是否具备存储后端。
func (s *AccountHealthService) Available() bool {
	return s != nil && s.store != nil
}

// RecordSuccess 记录一次成功请求；firstTokenMs 非空时更新首字延迟 EWMA。
func (s *AccountHealthService) RecordSuccess(ctx context.Context, accountID int64, firstTokenMs *int) {
	if !s.Available() || accountID <= 0 {
		return
	}
	if err := s.store.IncrWithTTL(ctx, accountHealthOKKeyPrefix+strconv.FormatInt(accountID, 10), accountHealthCounterTTL); err != nil {
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
	_ = s.store.IncrWithTTL(ctx, accountHealthFailKeyPrefix+strconv.FormatInt(accountID, 10), accountHealthCounterTTL)
}

// snapshotTimeout 限制调度热路径上健康快照的最长等待；超时返回空快照（退回原有排序）。
const snapshotTimeout = 50 * time.Millisecond

// SnapshotWithTimeout 在独立超时 context 下批量读取健康快照，绝不阻塞调度。
func (s *AccountHealthService) SnapshotWithTimeout(ctx context.Context, accountIDs []int64) map[int64]AccountHealthSnapshot {
	if !s.Available() || len(accountIDs) == 0 {
		return nil
	}
	snapCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()
	return s.Snapshot(snapCtx, accountIDs)
}

// Snapshot 批量读取账号健康度；读取失败的账号不出现在返回值中。
func (s *AccountHealthService) Snapshot(ctx context.Context, accountIDs []int64) map[int64]AccountHealthSnapshot {
	if !s.Available() || len(accountIDs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(accountIDs)*3)
	keyAccount := make(map[string]int64, len(accountIDs)*3)
	for _, id := range accountIDs {
		if id <= 0 {
			continue
		}
		suffix := strconv.FormatInt(id, 10)
		for _, prefix := range []string{accountHealthFTKeyPrefix, accountHealthOKKeyPrefix, accountHealthFailKeyPrefix} {
			key := prefix + suffix
			keys = append(keys, key)
			keyAccount[key] = id
		}
	}
	values, err := s.store.MGet(ctx, keys)
	if err != nil {
		return nil
	}

	out := make(map[int64]AccountHealthSnapshot, len(accountIDs))
	for id := range accountIDKeySet(accountIDs) {
		parsed := AccountHealthSnapshot{}
		if v, ok := values[accountHealthFTKeyPrefix+strconv.FormatInt(id, 10)]; ok {
			if ms, parseErr := strconv.ParseInt(v, 10, 64); parseErr == nil {
				parsed.AvgFirstTokenMs = &ms
			}
		}
		if v, ok := values[accountHealthOKKeyPrefix+strconv.FormatInt(id, 10)]; ok {
			parsed.OK, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := values[accountHealthFailKeyPrefix+strconv.FormatInt(id, 10)]; ok {
			parsed.Failed, _ = strconv.ParseInt(v, 10, 64)
		}
		if parsed.AvgFirstTokenMs != nil || parsed.OK > 0 || parsed.Failed > 0 {
			out[id] = parsed
		}
	}
	return out
}

// accountIDKeySet 返回去重后的账号 ID 集合。
func accountIDKeySet(ids []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			set[id] = struct{}{}
		}
	}
	return set
}

func (s *AccountHealthService) updateFirstTokenEWMA(ctx context.Context, accountID int64, sample float64) {
	key := accountHealthFTKeyPrefix + strconv.FormatInt(accountID, 10)
	next := sample
	if old, err := s.store.Get(ctx, key); err == nil {
		if oldVal, parseErr := strconv.ParseFloat(old, 64); parseErr == nil && oldVal > 0 {
			next = (1-accountHealthEWMAAlpha)*oldVal + accountHealthEWMAAlpha*sample
		}
	}
	_ = s.store.SetWithTTL(ctx, key, strconv.FormatFloat(next, 'f', 0, 64), accountHealthFTTTL)
}
