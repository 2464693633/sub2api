//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type apiKeyGroupValidationRepo struct {
	GroupRepository
	groups map[int64]*Group
}

func (r apiKeyGroupValidationRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	group := r.groups[id]
	if group == nil {
		return nil, ErrGroupNotFound
	}
	return group, nil
}

type apiKeyGroupValidationSubRepo struct {
	UserSubscriptionRepository
}

func (apiKeyGroupValidationSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	return &UserSubscription{UserID: userID, GroupID: groupID, Status: SubscriptionStatusActive}, nil
}

func TestAPIKeyGroupRequestCompatibility(t *testing.T) {
	legacyID := int64(7)
	ids, err := createAPIKeyGroupIDs(CreateAPIKeyRequest{GroupID: &legacyID})
	require.NoError(t, err)
	require.Equal(t, []int64{7}, ids)

	ids, err = createAPIKeyGroupIDs(CreateAPIKeyRequest{GroupIDs: []int64{3, 2}})
	require.NoError(t, err)
	require.Equal(t, []int64{3, 2}, ids)

	_, err = createAPIKeyGroupIDs(CreateAPIKeyRequest{GroupID: &legacyID, GroupIDs: []int64{7}})
	require.ErrorIs(t, err, ErrAPIKeyGroupsConflict)

	empty := []int64{}
	ids, changed, err := updateAPIKeyGroupIDs(UpdateAPIKeyRequest{GroupIDs: &empty})
	require.NoError(t, err)
	require.True(t, changed)
	require.Empty(t, ids)

	_, changed, err = updateAPIKeyGroupIDs(UpdateAPIKeyRequest{})
	require.NoError(t, err)
	require.False(t, changed)
}

func TestValidateAPIKeyGroupsPolicy(t *testing.T) {
	groups := map[int64]*Group{
		1: {ID: 1, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
		2: {ID: 2, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription},
		3: {ID: 3, Platform: PlatformAnthropic, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
		4: {ID: 4, Platform: PlatformComposite, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
		5: {ID: 5, Platform: PlatformComposite, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
	}
	svc := &APIKeyService{
		groupRepo:   apiKeyGroupValidationRepo{groups: groups},
		userSubRepo: apiKeyGroupValidationSubRepo{},
	}
	user := &User{ID: 9, Status: StatusActive}

	got, err := svc.validateAPIKeyGroups(context.Background(), user, []int64{1, 2})
	require.NoError(t, err, "standard and subscription groups may be mixed")
	require.Equal(t, []int64{1, 2}, []int64{got[0].ID, got[1].ID})

	_, err = svc.validateAPIKeyGroups(context.Background(), user, []int64{1, 3})
	require.ErrorIs(t, err, ErrAPIKeyGroupPlatform)
	_, err = svc.validateAPIKeyGroups(context.Background(), user, []int64{1, 1})
	require.ErrorIs(t, err, ErrAPIKeyGroupIDInvalid)
	_, err = svc.validateAPIKeyGroups(context.Background(), user, []int64{4, 5})
	require.ErrorIs(t, err, ErrAPIKeyCompositeChain)

	tooMany := make([]int64, MaxAPIKeyGroups+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	_, err = svc.validateAPIKeyGroups(context.Background(), user, tooMany)
	require.ErrorIs(t, err, ErrAPIKeyGroupsTooMany)
}
