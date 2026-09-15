package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ImageStudioKeyName 生图工作台专用密钥的名称标记。
// 每个生图分组(openai/grok)各一把,名字相同、按绑定分组区分。
const ImageStudioKeyName = "生图工作台"

// ErrImageStudioNoImageGroup 用户没有开启生图权限的可用分组,无法自动创建工作台密钥。
var ErrImageStudioNoImageGroup = errors.New("没有开启生图权限的可用分组,请联系管理员为分组开启生图")

// ErrImageStudioGroupNotFound 请求的分组不在用户可用生图分组内。
var ErrImageStudioGroupNotFound = errors.New("分组不存在或未开启生图权限")

// studioKeyMu 按用户串行化工作台密钥的查找/创建,避免并发首请求创建重复密钥。
// 生图与视频工作台共用。
var studioKeyMu sync.Map // map[int64]*sync.Mutex

// ImageStudioGroups 返回用户可用的生图分组(开启生图权限且平台为 openai/grok,
// 与网关 /v1/images 的平台路由一致),并确保每个分组都有对应的工作台密钥。
func (s *APIKeyService) ImageStudioGroups(ctx context.Context, userID int64) ([]Group, error) {
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
		// 网关 /v1/images 仅路由 openai/grok 平台
		if g.Platform != PlatformOpenAI && g.Platform != PlatformGrok {
			continue
		}
		out = append(out, *g)
	}
	if len(out) == 0 {
		return nil, ErrImageStudioNoImageGroup
	}
	return out, nil
}

// EnsureImageStudioKeyForGroup 返回指定生图分组的工作台密钥,不存在时自动创建。
// 查找规则:用户名下活跃密钥中名称等于 ImageStudioKeyName 且当前绑定该分组的最新一把。
func (s *APIKeyService) EnsureImageStudioKeyForGroup(ctx context.Context, userID int64, groupID int64) (*APIKey, error) {
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
		if k.Name != ImageStudioKeyName {
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
	return s.Create(ctx, userID, CreateAPIKeyRequest{Name: ImageStudioKeyName, GroupID: &groupID})
}
