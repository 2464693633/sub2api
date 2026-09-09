//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthCacheTokenMultiplierRoundTrip(t *testing.T) {
	groupID := int64(9)
	apiKey := &APIKey{
		ID: 1, UserID: 2, GroupID: &groupID, Key: "sk-token-policy", Status: StatusActive,
		User: &User{ID: 2, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "token-policy", Platform: PlatformOpenAI,
			Status: StatusActive, SubscriptionType: SubscriptionTypeStandard,
			InputTokenMultiplier: 0, OutputTokenMultiplier: 2,
			CacheCreationTokenMultiplier: 3, CacheReadTokenMultiplier: 4,
			ReturnBillableUsage: true, TokenMultipliersConfigured: true,
		},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.TokenMultipliersConfigured)
	require.Zero(t, materialized.Group.InputTokenMultiplier)
	require.Equal(t, 2.0, materialized.Group.OutputTokenMultiplier)
	require.Equal(t, 3.0, materialized.Group.CacheCreationTokenMultiplier)
	require.Equal(t, 4.0, materialized.Group.CacheReadTokenMultiplier)
	require.True(t, materialized.Group.ReturnBillableUsage)
}

func TestAPIKeyAuthCacheLegacyConstructedGroupGetsMultiplierDefaults(t *testing.T) {
	groupID := int64(10)
	apiKey := &APIKey{
		ID: 1, UserID: 2, GroupID: &groupID, Status: StatusActive,
		User:  &User{ID: 2, Status: StatusActive},
		Group: &Group{ID: groupID, Status: StatusActive},
	}
	svc := &APIKeyService{}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)

	require.NotNil(t, snapshot.Group)
	require.Equal(t, 1.0, snapshot.Group.InputTokenMultiplier)
	require.Equal(t, 1.0, snapshot.Group.OutputTokenMultiplier)
	require.Equal(t, 1.0, snapshot.Group.CacheCreationTokenMultiplier)
	require.Equal(t, 1.0, snapshot.Group.CacheReadTokenMultiplier)
}

func TestAPIKeyAuthCacheOrderedGroupsRoundTrip(t *testing.T) {
	first := &Group{ID: 21, Name: "primary", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}
	second := &Group{ID: 22, Name: "fallback", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 2, ReturnBillableUsage: true}
	apiKey := &APIKey{
		ID: 1, UserID: 2, Key: "sk-group-chain", Status: StatusActive,
		GroupID: &first.ID, Group: first,
		GroupIDs: []int64{first.ID, second.ID}, Groups: []*Group{first, second},
		GroupRPMOverrides: map[int64]int{second.ID: 17},
		User:              &User{ID: 2, Status: StatusActive},
	}
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)
	require.Equal(t, []int64{21, 22}, snapshot.GroupIDs)
	require.Len(t, snapshot.Groups, 2)

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))
	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, []int64{21, 22}, materialized.GroupIDs)
	require.Len(t, materialized.Groups, 2)
	require.Equal(t, "fallback", materialized.Groups[1].Name)
	require.True(t, materialized.Groups[1].Hydrated)
	require.True(t, materialized.Groups[1].ReturnBillableUsage)
	require.Equal(t, 17, materialized.GroupRPMOverrides[22])
	require.Equal(t, int64(21), *materialized.GroupID)
	require.Same(t, materialized.Groups[0], materialized.Group)
}
