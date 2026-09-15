//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type ensureStudioAPIKeyRepo struct {
	APIKeyRepository
	list    func(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
	created []*APIKey
	deleted []int64
}

func (s *ensureStudioAPIKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	return s.list(ctx, userID, params, filters)
}

func (s *ensureStudioAPIKeyRepo) Create(ctx context.Context, key *APIKey) error {
	key.ID = int64(len(s.created) + 1)
	key.CreatedAt = time.Now()
	s.created = append(s.created, key)
	return nil
}

func (s *ensureStudioAPIKeyRepo) DeleteWithAudit(ctx context.Context, id int64) error {
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *ensureStudioAPIKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	return "sk-studio", 7, nil
}

func (s *ensureStudioAPIKeyRepo) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	return nil, nil
}

type ensureStudioUserRepo struct {
	UserRepository
	user User
}

func (s *ensureStudioUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	u := s.user
	return &u, nil
}

type ensureStudioGroupRepo struct {
	GroupRepository
	groups []Group
}

func (s *ensureStudioGroupRepo) ListActive(ctx context.Context) ([]Group, error) {
	return s.groups, nil
}

func (s *ensureStudioGroupRepo) GetByID(ctx context.Context, id int64) (*Group, error) {
	for i := range s.groups {
		if s.groups[i].ID == id {
			return &s.groups[i], nil
		}
	}
	return nil, errors.New("group not found")
}

type ensureStudioSubRepo struct {
	UserSubscriptionRepository
}

func (s *ensureStudioSubRepo) ListActiveByUserID(ctx context.Context, userID int64) ([]UserSubscription, error) {
	return nil, nil
}

func newEnsureStudioService(apiRepo *ensureStudioAPIKeyRepo, groups []Group) *APIKeyService {
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

func TestImageStudioGroups_FiltersImageCapablePlatformGroups(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	svc := newEnsureStudioService(apiRepo, []Group{
		{ID: 1, Name: "chat", Platform: PlatformOpenAI, Status: StatusActive},
		{ID: 2, Name: "生图", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
		{ID: 3, Name: "grok生图", Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true},
		{ID: 4, Name: "gemini生图", Platform: PlatformGemini, Status: StatusActive, AllowImageGeneration: true},
	})

	groups, err := svc.ImageStudioGroups(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, groups, 2, "只保留 openai/grok 平台的生图分组")
	ids := []int64{groups[0].ID, groups[1].ID}
	require.ElementsMatch(t, []int64{2, 3}, ids)
}

func TestEnsureImageStudioKeyForGroup_ReusesExisting(t *testing.T) {
	groupID := int64(3434)
	existing := APIKey{
		ID:       11,
		UserID:   7,
		Key:      "sk-studio",
		Name:     ImageStudioKeyName,
		Status:   StatusAPIKeyActive,
		GroupID:  &groupID,
		GroupIDs: []int64{groupID},
		Group:    &Group{ID: groupID, Name: "生图", Platform: PlatformOpenAI},
		CreatedAt: time.Now().Add(-time.Hour),
	}
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{existing}, &pagination.PaginationResult{Total: 1}, nil
		},
	}
	svc := newEnsureStudioService(apiRepo, nil)

	key, err := svc.EnsureImageStudioKeyForGroup(context.Background(), 7, groupID)
	require.NoError(t, err)
	require.Equal(t, int64(11), key.ID)
	require.Empty(t, apiRepo.created, "同分组已有工作台密钥时不应新建")
}

func TestEnsureImageStudioKeyForGroup_CreatesPerGroup(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	openaiGroup := int64(2)
	grokGroup := int64(9)
	svc := newEnsureStudioService(apiRepo, []Group{
		{ID: openaiGroup, Name: "生图", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
		{ID: grokGroup, Name: "grok", Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true},
	})

	k1, err := svc.EnsureImageStudioKeyForGroup(context.Background(), 7, openaiGroup)
	require.NoError(t, err)
	require.Equal(t, ImageStudioKeyName, k1.Name)
	k2, err := svc.EnsureImageStudioKeyForGroup(context.Background(), 7, grokGroup)
	require.NoError(t, err)
	require.Len(t, apiRepo.created, 2, "不同分组各建一把工作台密钥")
	require.Equal(t, openaiGroup, *k1.GroupID)
	require.Equal(t, grokGroup, *k2.GroupID)
}
