package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyFromService_MapsLastUsedAt(t *testing.T) {
	lastUsed := time.Now().UTC().Truncate(time.Second)
	lastUsedIP := "203.0.113.10"
	src := &service.APIKey{
		ID:                 1,
		UserID:             2,
		Key:                "sk-map-last-used",
		Name:               "Mapper",
		Status:             service.StatusActive,
		LastUsedAt:         &lastUsed,
		LastUsedIP:         &lastUsedIP,
		CurrentConcurrency: 3,
	}

	out := APIKeyFromService(src)
	require.NotNil(t, out)
	require.NotNil(t, out.LastUsedAt)
	require.WithinDuration(t, lastUsed, *out.LastUsedAt, time.Second)
	require.NotNil(t, out.LastUsedIP)
	require.Equal(t, lastUsedIP, *out.LastUsedIP)
	require.Equal(t, 3, out.CurrentConcurrency)
}

func TestAPIKeyFromService_MapsNilLastUsedAt(t *testing.T) {
	src := &service.APIKey{
		ID:     1,
		UserID: 2,
		Key:    "sk-map-last-used-nil",
		Name:   "MapperNil",
		Status: service.StatusActive,
	}

	out := APIKeyFromService(src)
	require.NotNil(t, out)
	require.Nil(t, out.LastUsedAt)
	require.Nil(t, out.LastUsedIP)
}

func TestAPIKeyFromServiceMapsOrderedGroupsAndLegacyPrimary(t *testing.T) {
	first := &service.Group{ID: 7, Name: "first"}
	second := &service.Group{ID: 8, Name: "second"}
	src := &service.APIKey{
		ID: 1, GroupID: &first.ID, Group: first,
		GroupIDs: []int64{first.ID, second.ID}, Groups: []*service.Group{first, second},
	}

	out := APIKeyFromService(src)
	require.Equal(t, []int64{7, 8}, out.GroupIDs)
	require.Len(t, out.Groups, 2)
	require.Equal(t, int64(7), *out.GroupID)
	require.Equal(t, int64(7), out.Group.ID)
	require.Equal(t, int64(8), out.Groups[1].ID)
}
