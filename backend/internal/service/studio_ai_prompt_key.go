package service

import (
	"context"
	"fmt"
	"sync"
)

// AIPromptKeyName AI 提示词助手密钥的名称标记。
// 每个对话分组各一把,名字相同、按绑定分组区分(与工作台密钥同模式)。
const AIPromptKeyName = "AI提示词"

// EnsureAIPromptKeyForGroup 返回指定分组的 AI 提示词助手密钥,不存在时自动创建。
// 查找规则:用户名下活跃密钥中名称等于 AIPromptKeyName 且当前绑定该分组的最新一把。
func (s *APIKeyService) EnsureAIPromptKeyForGroup(ctx context.Context, userID int64, groupID int64) (*APIKey, error) {
	muAny, _ := studioKeyMu.LoadOrStore(userID, &sync.Mutex{})
	mu, ok := muAny.(*sync.Mutex)
	if !ok || mu == nil {
		return nil, fmt.Errorf("invalid studio key lock for user %d", userID)
	}
	mu.Lock()
	defer mu.Unlock()

	existing, err := s.findStudioKeyForGroup(ctx, userID, groupID, AIPromptKeyName)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return s.Create(ctx, userID, CreateAPIKeyRequest{Name: AIPromptKeyName, GroupID: &groupID})
}
