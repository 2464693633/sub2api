//go:build unit

package service

import (
	"context"
	"testing"
	"time"

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

func TestVideoStudioGroups_FiltersVideoCapablePlatformGroups(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	svc := newVideoStudioService(apiRepo, []Group{
		{ID: 1, Name: "chat", Platform: PlatformOpenAI, Status: StatusActive},
		{ID: 2, Name: "grok视频", Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true},
		{ID: 3, Name: "composite视频", Platform: PlatformComposite, Status: StatusActive, AllowImageGeneration: true},
		{ID: 4, Name: "grok无开关", Platform: PlatformGrok, Status: StatusActive},
	})

	groups, err := svc.VideoStudioGroups(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, groups, 2, "只保留 grok/composite 平台且开启生图权限的分组")
	ids := []int64{groups[0].ID, groups[1].ID}
	require.ElementsMatch(t, []int64{2, 3}, ids)
}

func TestVideoStudioGroups_NoVideoGroup(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	svc := newVideoStudioService(apiRepo, []Group{
		{ID: 1, Name: "生图", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
		{ID: 2, Name: "grok无开关", Platform: PlatformGrok, Status: StatusActive},
	})
	_, err := svc.VideoStudioGroups(context.Background(), 7)
	require.ErrorIs(t, err, ErrVideoStudioNoVideoGroup)
	require.Empty(t, apiRepo.created)
}

func TestEnsureVideoStudioKeyForGroup_ReusesExisting(t *testing.T) {
	groupID := int64(9)
	existing := APIKey{
		ID:        11,
		UserID:    7,
		Key:       "sk-studio",
		Name:      VideoStudioKeyName,
		Status:    StatusAPIKeyActive,
		GroupID:   &groupID,
		GroupIDs:  []int64{groupID},
		Group:     &Group{ID: groupID, Name: "grok视频", Platform: PlatformGrok},
		CreatedAt: time.Now().Add(-time.Hour),
	}
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{existing}, &pagination.PaginationResult{Total: 1}, nil
		},
	}
	svc := newVideoStudioService(apiRepo, nil)

	key, err := svc.EnsureVideoStudioKeyForGroup(context.Background(), 7, groupID)
	require.NoError(t, err)
	require.Equal(t, int64(11), key.ID)
	require.Empty(t, apiRepo.created, "同分组已有工作台密钥时不应新建")
}

func TestEnsureVideoStudioKeyForGroup_CreatesPerGroup(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	grokGroupA := int64(2)
	grokGroupB := int64(9)
	svc := newVideoStudioService(apiRepo, []Group{
		{ID: grokGroupA, Name: "grok视频A", Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true},
		{ID: grokGroupB, Name: "grok视频B", Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true},
	})

	k1, err := svc.EnsureVideoStudioKeyForGroup(context.Background(), 7, grokGroupA)
	require.NoError(t, err)
	require.Equal(t, VideoStudioKeyName, k1.Name)
	k2, err := svc.EnsureVideoStudioKeyForGroup(context.Background(), 7, grokGroupB)
	require.NoError(t, err)
	require.Len(t, apiRepo.created, 2, "不同分组各建一把工作台密钥")
	require.Equal(t, grokGroupA, *k1.GroupID)
	require.Equal(t, grokGroupB, *k2.GroupID)
}
