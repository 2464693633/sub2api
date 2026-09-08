//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveBillableUsageTokens(t *testing.T) {
	group := &Group{
		TokenMultipliersConfigured:   true,
		InputTokenMultiplier:         1.5,
		OutputTokenMultiplier:        0.5,
		CacheCreationTokenMultiplier: 0.3,
		CacheReadTokenMultiplier:     2,
	}
	raw := UsageTokens{
		InputTokens: 3, OutputTokens: 3, CacheCreationTokens: 5,
		CacheCreation5mTokens: 2, CacheCreation1hTokens: 3, CacheReadTokens: 4,
		ImageInputTokens: 2, ImageOutputTokens: 2,
	}

	got, snapshot := ResolveBillableUsageTokens(raw, group)

	require.Equal(t, UsageTokens{
		InputTokens: 5, OutputTokens: 2, CacheCreationTokens: 2, CacheReadTokens: 8,
		CacheCreation5mTokens: 1, CacheCreation1hTokens: 1,
		ImageInputTokens: 3, ImageOutputTokens: 1,
	}, got)
	require.Equal(t, TokenMultipliers{Input: 1.5, Output: 0.5, CacheCreation: 0.3, CacheRead: 2}, snapshot)
}

func TestResolveBillableUsageTokens_DefaultAndBounds(t *testing.T) {
	raw := UsageTokens{InputTokens: 7, OutputTokens: -2, CacheCreationTokens: 3, CacheReadTokens: 2}
	got, snapshot := ResolveBillableUsageTokens(raw, &Group{})
	require.Equal(t, 7, got.InputTokens)
	require.Zero(t, got.OutputTokens)
	require.Equal(t, 3, got.CacheCreationTokens)
	require.Equal(t, 2, got.CacheReadTokens)
	require.Equal(t, DefaultTokenMultipliers(), snapshot)

	configured := &Group{
		TokenMultipliersConfigured:   true,
		InputTokenMultiplier:         -1,
		OutputTokenMultiplier:        math.NaN(),
		CacheCreationTokenMultiplier: 101,
		CacheReadTokenMultiplier:     math.Inf(1),
	}
	got, snapshot = ResolveBillableUsageTokens(raw, configured)
	require.Zero(t, got.InputTokens)
	require.Zero(t, got.OutputTokens)
	require.Equal(t, 300, got.CacheCreationTokens)
	require.Equal(t, 200, got.CacheReadTokens)
	require.Equal(t, TokenMultipliers{CacheCreation: 100, CacheRead: 100}, snapshot)
}

func TestApplyTokenMultipliers_DerivesCacheCreationAggregateFromDetails(t *testing.T) {
	raw := UsageTokens{CacheCreation5mTokens: 2, CacheCreation1hTokens: 3}

	got := ApplyTokenMultipliers(raw, TokenMultipliers{Input: 1, Output: 1, CacheCreation: 2, CacheRead: 1})

	require.Equal(t, 10, got.CacheCreationTokens)
	require.Equal(t, 4, got.CacheCreation5mTokens)
	require.Equal(t, 6, got.CacheCreation1hTokens)
}

func TestUseBillableActualCost_UsesCustomerLongContextFlag(t *testing.T) {
	raw := &CostBreakdown{TotalCost: 1, ActualCost: 1}
	billable := &CostBreakdown{TotalCost: 3, ActualCost: 3, LongContextBillingApplied: true}

	got := useBillableActualCost(raw, billable)

	require.Same(t, raw, got)
	require.Equal(t, 1.0, got.TotalCost)
	require.Equal(t, 3.0, got.ActualCost)
	require.True(t, got.LongContextBillingApplied)
}

func TestGroupConservativeTokenRevenueMultiplier(t *testing.T) {
	require.Equal(t, 1.0, (&Group{}).ConservativeTokenRevenueMultiplier())
	require.Equal(t, 1.0, (*Group)(nil).ConservativeTokenRevenueMultiplier())

	group := &Group{
		TokenMultipliersConfigured: true,
		InputTokenMultiplier:       2, OutputTokenMultiplier: 3,
		CacheCreationTokenMultiplier: 0.5, CacheReadTokenMultiplier: 4,
	}
	require.Equal(t, 0.5, group.ConservativeTokenRevenueMultiplier())

	large := ApplyTokenMultipliers(
		UsageTokens{InputTokens: 30_000_000},
		TokenMultipliers{Input: 100, Output: 1, CacheCreation: 1, CacheRead: 1},
	)
	require.Equal(t, 3_000_000_000, large.InputTokens)
}

func TestUsageLogBillableTokens_HistoricalFallback(t *testing.T) {
	log := &UsageLog{InputTokens: 11, OutputTokens: 12, CacheCreationTokens: 13, CacheReadTokens: 14}
	require.Equal(t, UsageTokens{InputTokens: 11, OutputTokens: 12, CacheCreationTokens: 13, CacheReadTokens: 14}, log.BillableTokens())
	require.Equal(t, 50, log.BillableTotalTokens())

	zero := 0
	log.BillableInputTokens = &zero
	require.Zero(t, log.BillableTokens().InputTokens)
}

func TestGatewayBillableUsageChangesOnlyCustomerActualCost(t *testing.T) {
	billing := NewBillingService(nil, nil)
	svc := &GatewayService{billingService: billing}
	result := &ForwardResult{
		Model: "claude-sonnet-4",
		Usage: ClaudeUsage{InputTokens: 100, OutputTokens: 10},
	}
	raw := UsageTokens{InputTokens: 100, OutputTokens: 10}
	billable := UsageTokens{InputTokens: 200, OutputTokens: 5}

	cost := svc.calculateRecordUsageCostWithBillable(
		context.Background(), result, &APIKey{Group: &Group{}}, result.Model,
		1, 1, time.Time{}, raw, billable,
	)

	require.InDelta(t, 0.00045, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.000675, cost.ActualCost, 1e-12)
	require.InDelta(t, 0.0003, cost.InputCost, 1e-12)
	require.InDelta(t, 0.00015, cost.OutputCost, 1e-12)
}

func TestOpenAIBillableUsageChangesOnlyCustomerActualCost(t *testing.T) {
	billing := NewBillingService(nil, nil)
	svc := &OpenAIGatewayService{billingService: billing}
	raw := UsageTokens{InputTokens: 100, OutputTokens: 10}
	billable := UsageTokens{InputTokens: 200, OutputTokens: 5}
	longContextBilling := false

	cost, err := svc.calculateOpenAIRecordUsageCostWithBillable(
		context.Background(), &OpenAIForwardResult{Model: "claude-sonnet-4"},
		&APIKey{Group: &Group{}}, []string{"claude-sonnet-4"},
		1, 1, 1, 1, raw, billable, "", &longContextBilling, time.Time{},
	)

	require.NoError(t, err)
	require.InDelta(t, 0.00045, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.000675, cost.ActualCost, 1e-12)
	require.InDelta(t, 0.0003, cost.InputCost, 1e-12)
	require.InDelta(t, 0.00015, cost.OutputCost, 1e-12)
}

func TestGatewayRecordUsagePersistsRawAndBillableSnapshots(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
	groupID := int64(91)
	group := &Group{
		ID: groupID, RateMultiplier: 1, TokenMultipliersConfigured: true,
		InputTokenMultiplier: 2, OutputTokenMultiplier: 0.5,
		CacheCreationTokenMultiplier: 1, CacheReadTokenMultiplier: 1,
	}

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{RequestID: "billable-gateway", Model: "claude-sonnet-4", Usage: ClaudeUsage{InputTokens: 100, OutputTokens: 10}},
		APIKey: &APIKey{ID: 1, GroupID: &groupID, Group: group},
		User:   &User{ID: 2}, Account: &Account{ID: 3},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 100, usageRepo.lastLog.InputTokens)
	require.Equal(t, 10, usageRepo.lastLog.OutputTokens)
	require.Equal(t, 200, *usageRepo.lastLog.BillableInputTokens)
	require.Equal(t, 5, *usageRepo.lastLog.BillableOutputTokens)
	require.Equal(t, 2.0, *usageRepo.lastLog.InputTokenMultiplier)
	require.InDelta(t, 0.00045, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.000675, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, 0.000675, userRepo.lastAmount, 1e-12)
}

func TestOpenAIRecordUsagePersistsRawAndBillableSnapshots(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	groupID := int64(92)
	group := &Group{
		ID: groupID, RateMultiplier: 1, TokenMultipliersConfigured: true,
		InputTokenMultiplier: 2, OutputTokenMultiplier: 0.5,
		CacheCreationTokenMultiplier: 1, CacheReadTokenMultiplier: 1,
	}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "billable-openai", Model: "claude-sonnet-4", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 10}},
		APIKey: &APIKey{ID: 1, GroupID: &groupID, Group: group},
		User:   &User{ID: 2}, Account: &Account{ID: 3},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 100, usageRepo.lastLog.InputTokens)
	require.Equal(t, 10, usageRepo.lastLog.OutputTokens)
	require.Equal(t, 200, *usageRepo.lastLog.BillableInputTokens)
	require.Equal(t, 5, *usageRepo.lastLog.BillableOutputTokens)
	require.Equal(t, 0.5, *usageRepo.lastLog.OutputTokenMultiplier)
	require.InDelta(t, 0.00045, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.000675, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, 0.000675, userRepo.lastAmount, 1e-12)
}
