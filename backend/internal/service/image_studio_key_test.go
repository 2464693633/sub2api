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

func TestEnsureImageStudioKey_ReusesExistingStudioKey(t *testing.T) {
	groupID := int64(3434)
	existing := APIKey{
		ID:       11,
		UserID:   7,
		Key:      "sk-studio",
		Name:     ImageStudioKeyName,
		Status:   StatusAPIKeyActive,
		GroupID:  &groupID,
		Group:    &Group{ID: groupID, Name: "生图", Platform: PlatformOpenAI},
		CreatedAt: time.Now().Add(-time.Hour),
	}
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{existing}, &pagination.PaginationResult{Total: 1}, nil
		},
	}
	svc := newEnsureStudioService(apiRepo, nil)

	key, err := svc.EnsureImageStudioKey(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(11), key.ID)
	require.Empty(t, apiRepo.created, "已存在的工作台密钥不应触发新建")
}

func TestEnsureImageStudioKey_CreatesInImageCapableGroup(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	svc := newEnsureStudioService(apiRepo, []Group{
		{ID: 1, Name: "chat", Platform: PlatformOpenAI, Status: StatusActive},
		{ID: 2, Name: "生图", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
	})

	key, err := svc.EnsureImageStudioKey(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, apiRepo.created, 1)
	require.Equal(t, ImageStudioKeyName, key.Name)
	require.NotNil(t, key.GroupID)
	require.Equal(t, int64(2), *key.GroupID, "应优先选择开启生图权限的分组")
}

func TestEnsureImageStudioKey_NoImageCapableGroup(t *testing.T) {
	apiRepo := &ensureStudioAPIKeyRepo{
		list: func(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{}, &pagination.PaginationResult{}, nil
		},
	}
	svc := newEnsureStudioService(apiRepo, []Group{
		{ID: 1, Name: "chat", Platform: PlatformOpenAI, Status: StatusActive},
	})

	_, err := svc.EnsureImageStudioKey(context.Background(), 7)
	require.ErrorIs(t, err, ErrImageStudioNoImageGroup)
	require.Empty(t, apiRepo.created)
}
