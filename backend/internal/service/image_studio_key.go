package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ImageStudioKeyName 生图工作台专用密钥的名称标记。
// 工作台会话直通依赖它按名字找到(或自动创建)用户的专属密钥。
const ImageStudioKeyName = "生图工作台"

// ErrImageStudioNoImageGroup 用户没有开启生图权限的可用分组,无法自动创建工作台密钥。
var ErrImageStudioNoImageGroup = errors.New("没有开启生图权限的可用分组,请联系管理员为分组开启生图")

// EnsureImageStudioKey 返回用户的生图工作台密钥,不存在时自动创建。
//
// 查找规则:用户名下活跃密钥中名称等于 ImageStudioKeyName 且已绑定分组的最新一把。
// 创建规则:在用户可绑定的分组里优先选开了生图权限(AllowImageGeneration)的,
// 都没有则报 ErrImageStudioNoImageGroup,避免创建一把必然 403 的密钥。
func (s *APIKeyService) EnsureImageStudioKey(ctx context.Context, userID int64) (*APIKey, error) {
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
		if groups[i].AllowImageGeneration {
			chosen = &groups[i]
			break
		}
	}
	if chosen == nil {
		return nil, ErrImageStudioNoImageGroup
	}
	return s.Create(ctx, userID, CreateAPIKeyRequest{Name: ImageStudioKeyName, GroupID: &chosen.ID})
}
