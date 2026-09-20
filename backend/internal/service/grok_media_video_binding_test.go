//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type mapGatewayCache struct {
	GatewayCache
	data map[string]int64
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
	return &OpenAIGatewayService{cfg: &config.Config{}, cache: &mapGatewayCache{data: map[string]int64{}}}
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
