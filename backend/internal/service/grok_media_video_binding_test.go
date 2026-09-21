//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type mapGatewayCache struct {
	GatewayCache
	data    map[string]int64
	pending map[string][]byte
	claimed map[string]bool
}

func (c *mapGatewayCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	if v, ok := c.data[fmt.Sprintf("%d:%s", groupID, sessionHash)]; ok {
		return v, nil
	}
	return 0, ErrStickySessionNotFound
}

func (c *mapGatewayCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.data[fmt.Sprintf("%d:%s", groupID, sessionHash)] = accountID
	return nil
}

func newVideoBindingTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{}, cache: &mapGatewayCache{
		data: map[string]int64{}, pending: map[string][]byte{}, claimed: map[string]bool{},
	}}
}

func (c *mapGatewayCache) SetGrokVideoPendingBilling(_ context.Context, key string, payload []byte, _ time.Duration) error {
	c.pending[key] = payload
	return nil
}
func (c *mapGatewayCache) GetGrokVideoPendingBilling(_ context.Context, key string) ([]byte, error) {
	return c.pending[key], nil
}
func (c *mapGatewayCache) ClaimGrokVideoBilled(_ context.Context, key string, _ time.Duration) (bool, error) {
	if c.claimed[key] {
		return false, nil
	}
	c.claimed[key] = true
	return true, nil
}
func (c *mapGatewayCache) ReleaseGrokVideoBilled(_ context.Context, key string) error {
	delete(c.claimed, key)
	return nil
}

func TestBindGrokMediaVideoRequest_ResolvesAcrossApiKeys(t *testing.T) {
	svc := newVideoBindingTestService()
	group := int64(6)
	require.NoError(t, svc.BindGrokMediaVideoRequestAccount(context.Background(), &group, "task-x", 1, 7, 4))

	// 创建时使用的密钥:可解析
	id, err := svc.ResolveGrokMediaVideoRequestAccount(context.Background(), &group, "task-x", 1, 7)
	require.NoError(t, err)
	require.Equal(t, int64(4), id)

	// 跨密钥解析(旧实现在这里 miss,状态/内容轮询 404):V2 哈希与密钥无关
	id, err = svc.ResolveGrokMediaVideoRequestAccount(context.Background(), &group, "task-x", 1, 1)
	require.NoError(t, err)
	require.Equal(t, int64(4), id)
}

func TestResolveGrokMediaVideoRequest_LegacyHashFallback(t *testing.T) {
	svc := newVideoBindingTestService()
	group := int64(6)
	// 模拟升级前创建的任务:只有旧哈希(apiKeyID 维度)的绑定
	legacyHash := GrokMediaVideoRequestSessionHash("task-old", 1, 7)
	svc.cache.(*mapGatewayCache).data[fmt.Sprintf("6:%s", svc.openAISessionCacheKey(legacyHash))] = 9

	id, err := svc.ResolveGrokMediaVideoRequestAccount(context.Background(), &group, "task-old", 1, 7)
	require.NoError(t, err)
	require.Equal(t, int64(9), id, "V2 未命中时应回落旧哈希")
}

func TestResolveGrokMediaVideoRequest_MissingBinding(t *testing.T) {
	svc := newVideoBindingTestService()
	group := int64(6)
	id, err := svc.ResolveGrokMediaVideoRequestAccount(context.Background(), &group, "task-none", 1, 7)
	require.Error(t, err)
	require.Equal(t, int64(0), id)
}

func TestGrokVideoBilling_StableAcrossApiKeys(t *testing.T) {
	svc := newVideoBindingTestService()
	pending := GrokVideoPendingBilling{Model: "grok-imagine-video-1.5", VideoResolution: "1080p", VideoDurationSeconds: 15}

	// 密钥 A 创建
	require.NoError(t, svc.StoreGrokVideoPendingBilling(context.Background(), "task-b1", 1, 7, pending))

	// 跨密钥读取:密钥 B 应能读到同一快照(旧实现这里 miss → 按默认价漏计)
	got, err := svc.LoadGrokVideoPendingBilling(context.Background(), "task-b1", 1, 9)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "1080p", got.VideoResolution)

	// 跨密钥幂等:密钥 B claim 成功后,密钥 A 再 claim 必须失败(旧实现会重复计费)
	claimedB, err := svc.ClaimGrokVideoBilling(context.Background(), "task-b1", 1, 9)
	require.NoError(t, err)
	require.True(t, claimedB, "密钥 B 首次 claim 应成功")
	claimedA, err := svc.ClaimGrokVideoBilling(context.Background(), "task-b1", 1, 7)
	require.NoError(t, err)
	require.False(t, claimedA, "换密钥重复 claim 必须被拒绝")

	// release 后可重试(计费失败回滚场景)
	require.NoError(t, svc.ReleaseGrokVideoBilling(context.Background(), "task-b1", 1, 7))
	again, err := svc.ClaimGrokVideoBilling(context.Background(), "task-b1", 1, 7)
	require.NoError(t, err)
	require.True(t, again)
}

func TestGrokVideoBilling_LegacyPendingFallbackAndNoDoubleBill(t *testing.T) {
	svc := newVideoBindingTestService()
	// 模拟升级前创建的任务:快照与 claim 都在旧键下,且已计费
	legacyKey := grokVideoPendingBillingKey("task-old", 1, 7)
	payload, _ := json.Marshal(GrokVideoPendingBilling{Model: "grok-imagine-video-1.5", VideoResolution: "720p"})
	svc.cache.(*mapGatewayCache).pending[legacyKey] = payload
	svc.cache.(*mapGatewayCache).claimed[legacyKey] = true

	// 旧任务读取回退旧键
	got, err := svc.LoadGrokVideoPendingBilling(context.Background(), "task-old", 1, 7)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "720p", got.VideoResolution)

	// 旧任务已计费:V2 claim 不得再次放行(迁移期防重复)
	claimed, err := svc.ClaimGrokVideoBilling(context.Background(), "task-old", 1, 7)
	require.NoError(t, err)
	require.False(t, claimed, "升级前已计费的任务不得被 V2 claim 重复计费")
}
