package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// AIPromptKeyName AI 提示词助手密钥的名称标记。
// 每个对话分组各一把,名字相同、按绑定分组区分(与工作台密钥同模式)。
const AIPromptKeyName = "AI提示词"

// EnsureAIPromptKeyForGroup 返回指定分组的 AI 提示词助手密钥,不存在时自动创建。
// 查找规则:用户名下活跃密钥中名称等于 AIPromptKeyName 且当前绑定该分组的最新一把。
func (s *APIKeyService) EnsureAIPromptKeyForGroup(ctx context.Context, userID int64, groupID int64) (*APIKey, error) {
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
		if k.Name != AIPromptKeyName {
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
	return s.Create(ctx, userID, CreateAPIKeyRequest{Name: AIPromptKeyName, GroupID: &groupID})
}
