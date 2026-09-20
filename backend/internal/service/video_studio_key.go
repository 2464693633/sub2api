package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// VideoStudioKeyName 视频工作台专用密钥的名称标记。
// 每个视频分组(grok/composite)各一把,名字相同、按绑定分组区分。
const VideoStudioKeyName = "视频工作台"

// ErrVideoStudioNoVideoGroup 用户没有可用的视频分组(grok/composite 平台且开启生图开关),无法自动创建工作台密钥。
var ErrVideoStudioNoVideoGroup = errors.New("没有可用的视频分组(需要 grok/composite 平台且开启图片生成权限的分组),请联系管理员")

// ErrVideoStudioGroupNotFound 请求的分组不在用户可用视频分组内。
var ErrVideoStudioGroupNotFound = errors.New("分组不存在或未开启视频权限")

// VideoStudioGroups 返回用户可用的视频分组(开启生图权限且平台为 grok/composite,
// 与网关 /v1/videos 的平台路由一致),每个分组将各有一把工作台密钥。
func (s *APIKeyService) VideoStudioGroups(ctx context.Context, userID int64) ([]Group, error) {
	groups, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list available groups: %w", err)
	}
	out := make([]Group, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		if !g.AllowImageGeneration {
			continue
		}
		// 网关 /v1/videos 仅路由 grok/composite 平台
		if g.Platform != PlatformGrok && g.Platform != PlatformComposite {
			continue
		}
		out = append(out, *g)
	}
	if len(out) == 0 {
		return nil, ErrVideoStudioNoVideoGroup
	}
	return out, nil
}

// EnsureVideoStudioKeyForGroup 返回指定视频分组的工作台密钥,不存在时自动创建。
// 查找规则:用户名下活跃密钥中名称等于 VideoStudioKeyName 且当前绑定该分组的最新一把。
func (s *APIKeyService) EnsureVideoStudioKeyForGroup(ctx context.Context, userID int64, groupID int64) (*APIKey, error) {
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
		if k.GroupID == nil || *k.GroupID != groupID {
			continue
		}
		if existing == nil || k.CreatedAt.After(existing.CreatedAt) {
			existing = k
		}
	}
	if existing != nil {
		return existing, nil
	}
	return s.Create(ctx, userID, CreateAPIKeyRequest{Name: VideoStudioKeyName, GroupID: &groupID})
}
