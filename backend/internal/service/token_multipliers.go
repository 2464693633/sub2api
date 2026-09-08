package service

import "math"

const maxTokenMultiplier = 100.0

// TokenMultipliers is the per-request snapshot used to derive customer-facing
// token counts from the mutually exclusive raw token buckets.
type TokenMultipliers struct {
	Input         float64
	Output        float64
	CacheCreation float64
	CacheRead     float64
}

func DefaultTokenMultipliers() TokenMultipliers {
	return TokenMultipliers{Input: 1, Output: 1, CacheCreation: 1, CacheRead: 1}
}

// EffectiveTokenMultipliers returns the configured group policy. A Group built
// by legacy callers has no configured marker and therefore retains 1x behavior.
func (g *Group) EffectiveTokenMultipliers() TokenMultipliers {
	if g == nil || !g.TokenMultipliersConfigured {
		return DefaultTokenMultipliers()
	}
	return TokenMultipliers{
		Input:         normalizeTokenMultiplier(g.InputTokenMultiplier),
		Output:        normalizeTokenMultiplier(g.OutputTokenMultiplier),
		CacheCreation: normalizeTokenMultiplier(g.CacheCreationTokenMultiplier),
		CacheRead:     normalizeTokenMultiplier(g.CacheReadTokenMultiplier),
	}
}

// ConservativeTokenRevenueMultiplier is the guaranteed revenue factor before
// the request's token-bucket mix is known. Profit admission uses the lowest
// bucket multiplier so every possible token-only request remains protected.
func (g *Group) ConservativeTokenRevenueMultiplier() float64 {
	multipliers := g.EffectiveTokenMultipliers()
	return min(multipliers.Input, multipliers.Output, multipliers.CacheCreation, multipliers.CacheRead)
}

// ResolveBillableUsageTokens applies the configured group multipliers exactly
// once. Both billing and response rewriting must use this function so rounding
// cannot diverge between the charged amount and the usage returned downstream.
func ResolveBillableUsageTokens(raw UsageTokens, group *Group) (UsageTokens, TokenMultipliers) {
	multipliers := group.EffectiveTokenMultipliers()
	return ApplyTokenMultipliers(raw, multipliers), multipliers
}

// ApplyTokenMultipliers is the shared pure transformation for callers that
// already hold a persisted multiplier snapshot instead of a live Group.
func ApplyTokenMultipliers(raw UsageTokens, multipliers TokenMultipliers) UsageTokens {
	multipliers = TokenMultipliers{
		Input:         normalizeTokenMultiplier(multipliers.Input),
		Output:        normalizeTokenMultiplier(multipliers.Output),
		CacheCreation: normalizeTokenMultiplier(multipliers.CacheCreation),
		CacheRead:     normalizeTokenMultiplier(multipliers.CacheRead),
	}
	if raw.CacheCreationTokens <= 0 {
		cacheCreation5m, cacheCreation1h := normalizeCacheCreationBreakdown(raw)
		raw.CacheCreationTokens = cacheCreation5m + cacheCreation1h
	}
	billable := UsageTokens{
		InputTokens:         roundedTokenCount(raw.InputTokens, multipliers.Input),
		ImageInputTokens:    roundedTokenCount(raw.ImageInputTokens, multipliers.Input),
		OutputTokens:        roundedTokenCount(raw.OutputTokens, multipliers.Output),
		CacheCreationTokens: roundedTokenCount(raw.CacheCreationTokens, multipliers.CacheCreation),
		CacheReadTokens:     roundedTokenCount(raw.CacheReadTokens, multipliers.CacheRead),
		ImageOutputTokens:   roundedTokenCount(raw.ImageOutputTokens, multipliers.Output),
	}
	if billable.ImageInputTokens > billable.InputTokens {
		billable.ImageInputTokens = billable.InputTokens
	}
	if billable.ImageOutputTokens > billable.OutputTokens {
		billable.ImageOutputTokens = billable.OutputTokens
	}
	billable.CacheCreation5mTokens, billable.CacheCreation1hTokens =
		scaleCacheCreationBreakdown(raw, billable.CacheCreationTokens, multipliers.CacheCreation)
	return billable
}

func normalizeTokenMultiplier(multiplier float64) float64 {
	switch {
	case math.IsNaN(multiplier), multiplier <= 0:
		return 0
	case math.IsInf(multiplier, 1), multiplier > maxTokenMultiplier:
		return maxTokenMultiplier
	default:
		return multiplier
	}
}

func roundedTokenCount(raw int, multiplier float64) int {
	if raw <= 0 || multiplier <= 0 || math.IsNaN(multiplier) {
		return 0
	}
	value := math.Round(float64(raw) * normalizeTokenMultiplier(multiplier))
	maxInt := int(^uint(0) >> 1)
	if value >= float64(maxInt) {
		return maxInt
	}
	return int(value)
}

func nonNegativeTokenCount(raw int) int {
	if raw < 0 {
		return 0
	}
	return raw
}

// scaleCacheCreationBreakdown preserves the 5m/1h ratio while making the
// rounded detail total consistent with the rounded aggregate token bucket.
func scaleCacheCreationBreakdown(raw UsageTokens, billableAggregate int, multiplier float64) (int, int) {
	raw5m, raw1h := normalizeCacheCreationBreakdown(raw)
	detailTotal := raw5m + raw1h
	if detailTotal <= 0 {
		return 0, 0
	}
	billableDetailTotal := roundedTokenCount(detailTotal, multiplier)
	if raw.CacheCreationTokens > 0 && billableDetailTotal > billableAggregate {
		billableDetailTotal = billableAggregate
	}
	billable5m := int(math.Round(float64(billableDetailTotal) * float64(raw5m) / float64(detailTotal)))
	if billable5m < 0 {
		billable5m = 0
	}
	if billable5m > billableDetailTotal {
		billable5m = billableDetailTotal
	}
	return billable5m, billableDetailTotal - billable5m
}

func snapshotTokenCount(value int) *int {
	value = nonNegativeTokenCount(value)
	return &value
}

func snapshotTokenMultiplier(value float64) *float64 {
	value = normalizeTokenMultiplier(value)
	return &value
}

// applyBillableTokenSnapshot attaches both billable counts and the exact
// multiplier policy to a newly-created usage log.
func applyBillableTokenSnapshot(log *UsageLog, billable UsageTokens, multipliers TokenMultipliers) {
	if log == nil {
		return
	}
	log.BillableInputTokens = snapshotTokenCount(billable.InputTokens)
	log.BillableOutputTokens = snapshotTokenCount(billable.OutputTokens)
	log.BillableCacheCreationTokens = snapshotTokenCount(billable.CacheCreationTokens)
	log.BillableCacheReadTokens = snapshotTokenCount(billable.CacheReadTokens)
	log.InputTokenMultiplier = snapshotTokenMultiplier(multipliers.Input)
	log.OutputTokenMultiplier = snapshotTokenMultiplier(multipliers.Output)
	log.CacheCreationTokenMultiplier = snapshotTokenMultiplier(multipliers.CacheCreation)
	log.CacheReadTokenMultiplier = snapshotTokenMultiplier(multipliers.CacheRead)
}

// useBillableActualCost keeps upstream/raw accounting on rawCost while using
// billableCost only for the customer charge.
func useBillableActualCost(rawCost, billableCost *CostBreakdown) *CostBreakdown {
	if rawCost == nil {
		return billableCost
	}
	if billableCost != nil {
		rawCost.ActualCost = billableCost.ActualCost
		rawCost.LongContextBillingApplied = billableCost.LongContextBillingApplied
	}
	return rawCost
}
