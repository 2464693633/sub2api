package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// VideoStudioKeyName 视频工作台专用密钥的名称标记。
const VideoStudioKeyName = "视频工作台"

// ErrVideoStudioNoVideoGroup 用户没有可用的视频分组(grok/composite 平台且开启生图开关),无法自动创建工作台密钥。
var ErrVideoStudioNoVideoGroup = fmt.Errorf("没有可用的视频分组(需要 grok/composite 平台且开启图片生成权限的分组),请联系管理员")

// EnsureVideoStudioKey 返回用户的视频工作台密钥,不存在时自动创建。
//
// 查找规则:用户名下活跃密钥中名称等于 VideoStudioKeyName 且已绑定分组的最新一把。
// 创建规则:在用户可绑定的分组里选第一个「平台为 grok 或 composite 且开启生图权限」的
// 分组(网关 /v1/videos 只路由这两类平台,且生成时校验 allow_image_generation);
// 都没有则报 ErrVideoStudioNoVideoGroup。
func (s *APIKeyService) EnsureVideoStudioKey(ctx context.Context, userID int64) (*APIKey, error) {
	muAny, _ := studioKeyMu.LoadOrStore(userID, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	keys, _, err := s.List(ctx, userID, pagination.PaginationParams{Page: 1, PageSize: 100}, APIKeyListFilters{Status: StatusAPIKeyActive})
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	var existing *APIKey
	for i := range keys {
		k := &keys[i]
		if k.Name != VideoStudioKeyName {
			continue
		}
		hasGroup := k.Group != nil || len(k.Groups) > 0 || (k.GroupID != nil && *k.GroupID > 0)
		if !hasGroup {
			continue
		}
		if existing == nil || k.CreatedAt.After(existing.CreatedAt) {
			existing = k
		}
	}
	if existing != nil {
		return existing, nil
	}

	groups, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list available groups: %w", err)
	}
	var chosen *Group
	for i := range groups {
		g := &groups[i]
		if !g.AllowImageGeneration {
			continue
		}
		if g.Platform == PlatformGrok || g.Platform == PlatformComposite {
			chosen = g
			break
		}
	}
	if chosen == nil {
		return nil, ErrVideoStudioNoVideoGroup
	}
	return s.Create(ctx, userID, CreateAPIKeyRequest{Name: VideoStudioKeyName, GroupID: &chosen.ID})
}
