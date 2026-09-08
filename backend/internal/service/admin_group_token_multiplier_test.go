//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminServiceCreateGroupTokenMultiplierDefaultsAndExplicitZero(t *testing.T) {
	t.Run("omitted fields default to one", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{}
		svc := &adminServiceImpl{groupRepo: repo}

		created, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
			Name: "defaults", RateMultiplier: 1,
		})

		require.NoError(t, err)
		require.True(t, created.TokenMultipliersConfigured)
		require.Equal(t, 1.0, created.InputTokenMultiplier)
		require.Equal(t, 1.0, created.OutputTokenMultiplier)
		require.Equal(t, 1.0, created.CacheCreationTokenMultiplier)
		require.Equal(t, 1.0, created.CacheReadTokenMultiplier)
		require.False(t, created.ReturnBillableUsage)
	})

	t.Run("explicit zero is preserved", func(t *testing.T) {
		zero := 0.0
		repo := &groupRepoStubForAdmin{}
		svc := &adminServiceImpl{groupRepo: repo}

		created, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
			Name: "free-buckets", RateMultiplier: 1,
			InputTokenMultiplier: &zero, OutputTokenMultiplier: &zero,
			CacheCreationTokenMultiplier: &zero, CacheReadTokenMultiplier: &zero,
			ReturnBillableUsage: true,
		})

		require.NoError(t, err)
		require.Zero(t, created.InputTokenMultiplier)
		require.Zero(t, created.OutputTokenMultiplier)
		require.Zero(t, created.CacheCreationTokenMultiplier)
		require.Zero(t, created.CacheReadTokenMultiplier)
		require.True(t, created.ReturnBillableUsage)
	})
}

func TestAdminServiceRejectsOutOfRangeTokenMultipliers(t *testing.T) {
	for _, value := range []float64{-0.01, 100.01} {
		t.Run("create", func(t *testing.T) {
			svc := &adminServiceImpl{groupRepo: &groupRepoStubForAdmin{}}
			_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
				Name: "invalid", RateMultiplier: 1, InputTokenMultiplier: &value,
			})
			require.ErrorContains(t, err, "input_token_multiplier must be between 0 and 100")
		})

		t.Run("update", func(t *testing.T) {
			existing := &Group{
				ID: 1, Name: "existing", Platform: PlatformAnthropic,
				Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1,
			}
			repo := &groupRepoStubForAdmin{getByID: existing}
			svc := &adminServiceImpl{groupRepo: repo}
			_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{CacheReadTokenMultiplier: &value})
			require.ErrorContains(t, err, "cache_read_token_multiplier must be between 0 and 100")
			require.Nil(t, repo.updated)
		})
	}
}

func TestAdminServiceUpdateTokenMultipliersOmittedAndExplicit(t *testing.T) {
	existing := &Group{
		ID: 1, Name: "existing", Platform: PlatformAnthropic,
		Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1,
		InputTokenMultiplier: 2, OutputTokenMultiplier: 3,
		CacheCreationTokenMultiplier: 4, CacheReadTokenMultiplier: 5,
		ReturnBillableUsage: false, TokenMultipliersConfigured: true,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	updated, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{})
	require.NoError(t, err)
	require.Equal(t, []float64{2, 3, 4, 5}, []float64{
		updated.InputTokenMultiplier, updated.OutputTokenMultiplier,
		updated.CacheCreationTokenMultiplier, updated.CacheReadTokenMultiplier,
	})
	require.False(t, updated.ReturnBillableUsage)

	zero, hundred, enabled := 0.0, 100.0, true
	updated, err = svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		InputTokenMultiplier: &zero, OutputTokenMultiplier: &hundred,
		CacheCreationTokenMultiplier: &zero, CacheReadTokenMultiplier: &hundred,
		ReturnBillableUsage: &enabled,
	})
	require.NoError(t, err)
	require.Equal(t, []float64{0, 100, 0, 100}, []float64{
		updated.InputTokenMultiplier, updated.OutputTokenMultiplier,
		updated.CacheCreationTokenMultiplier, updated.CacheReadTokenMultiplier,
	})
	require.True(t, updated.ReturnBillableUsage)
}

func TestCloneGroupForDuplicatePreservesTokenUsagePolicy(t *testing.T) {
	source := &Group{
		Name: "source", InputTokenMultiplier: 0, OutputTokenMultiplier: 2.5,
		CacheCreationTokenMultiplier: 3, CacheReadTokenMultiplier: 4,
		ReturnBillableUsage: true, TokenMultipliersConfigured: true,
	}

	duplicate := cloneGroupForDuplicate(source, "operation")

	require.True(t, duplicate.TokenMultipliersConfigured)
	require.Equal(t, source.InputTokenMultiplier, duplicate.InputTokenMultiplier)
	require.Equal(t, source.OutputTokenMultiplier, duplicate.OutputTokenMultiplier)
	require.Equal(t, source.CacheCreationTokenMultiplier, duplicate.CacheCreationTokenMultiplier)
	require.Equal(t, source.CacheReadTokenMultiplier, duplicate.CacheReadTokenMultiplier)
	require.Equal(t, source.ReturnBillableUsage, duplicate.ReturnBillableUsage)
}
