//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func newVideoStudioService(apiRepo *ensureStudioAPIKeyRepo, groups []Group) *APIKeyService {
	return NewAPIKeyService(
		apiRepo,
		&ensureStudioUserRepo{user: User{ID: 7, Status: StatusActive, Role: RoleUser}},
		&ensureStudioGroupRepo{groups: groups},
		&ensureStudioSubRepo{},
		nil,
		nil,
		&config.Config{},
	)
}

func TestEnsureVideoStudioKey_CreatesInGrokVideoGroup(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	svc := newVideoStudioService(apiRepo, []Group{
		{ID: 1, Name: "chat", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
		{ID: 2, Name: "grok视频", Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true},
	})
	key, err := svc.EnsureVideoStudioKey(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, VideoStudioKeyName, key.Name)
	require.NotNil(t, key.GroupID)
	require.Equal(t, int64(2), *key.GroupID, "应选择 grok 平台且开启生图权限的分组")
}

func TestEnsureVideoStudioKey_NoVideoGroup(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	svc := newVideoStudioService(apiRepo, []Group{
		{ID: 1, Name: "生图", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
		{ID: 2, Name: "grok无开关", Platform: PlatformGrok, Status: StatusActive},
	})
	_, err := svc.EnsureVideoStudioKey(context.Background(), 7)
	require.ErrorIs(t, err, ErrVideoStudioNoVideoGroup)
	require.Empty(t, apiRepo.created)
}
