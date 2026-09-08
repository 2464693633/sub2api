//go:build unit

package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToServicePreservesTokenUsagePolicy(t *testing.T) {
	entity := &dbent.Group{
		InputTokenMultiplier: 0, OutputTokenMultiplier: 2,
		CacheCreationTokenMultiplier: 3, CacheReadTokenMultiplier: 4,
		ReturnBillableUsage: true,
	}

	group := groupEntityToService(entity)

	require.True(t, group.TokenMultipliersConfigured)
	require.Zero(t, group.InputTokenMultiplier)
	require.Equal(t, 2.0, group.OutputTokenMultiplier)
	require.Equal(t, 3.0, group.CacheCreationTokenMultiplier)
	require.Equal(t, 4.0, group.CacheReadTokenMultiplier)
	require.True(t, group.ReturnBillableUsage)
}
