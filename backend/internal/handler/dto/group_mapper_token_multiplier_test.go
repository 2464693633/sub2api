//go:build unit

package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMappersExposeTokenUsagePolicy(t *testing.T) {
	group := &service.Group{
		InputTokenMultiplier: 0, OutputTokenMultiplier: 2,
		CacheCreationTokenMultiplier: 3, CacheReadTokenMultiplier: 4,
		ReturnBillableUsage: true, TokenMultipliersConfigured: true,
	}

	userDTO := GroupFromService(group)
	adminDTO := GroupFromServiceAdmin(group)

	for _, dto := range []*Group{userDTO, &adminDTO.Group} {
		require.Zero(t, dto.InputTokenMultiplier)
		require.Equal(t, 2.0, dto.OutputTokenMultiplier)
		require.Equal(t, 3.0, dto.CacheCreationTokenMultiplier)
		require.Equal(t, 4.0, dto.CacheReadTokenMultiplier)
		require.True(t, dto.ReturnBillableUsage)
	}
}
