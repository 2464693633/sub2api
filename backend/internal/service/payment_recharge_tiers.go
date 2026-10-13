package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const SettingBalanceRechargeTiers = "BALANCE_RECHARGE_TIERS"

// RechargeTier replaces the base multiplier for an exact amount and currency.
// Amount excludes the existing payment fee; CreditAmount is the total USD credit.
type RechargeTier struct {
	Amount       float64 `json:"amount"`
	CreditAmount float64 `json:"credit_amount"`
	Currency     string  `json:"currency"`
}

func normalizeRechargeTiers(tiers []RechargeTier) ([]RechargeTier, error) {
	invalid := func(message string) error {
		return infraerrors.BadRequest("INVALID_RECHARGE_TIERS", message)
	}
	if len(tiers) > 50 {
		return nil, invalid("at most 50 recharge tiers are allowed")
	}
	out := make([]RechargeTier, 0, len(tiers))
	seen := make(map[string]bool, len(tiers))
	for _, tier := range tiers {
		currency, err := payment.NormalizePaymentCurrency(tier.Currency)
		if err != nil {
			return nil, invalid("invalid recharge tier currency")
		}
		for _, value := range []float64{tier.Amount, tier.CreditAmount} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1e9 {
				return nil, invalid("recharge tier amounts must be greater than 0 and at most 1000000000")
			}
		}
		amount := decimal.NewFromFloat(tier.Amount)
		credit := decimal.NewFromFloat(tier.CreditAmount)
		digits := int32(min(2, payment.CurrencyMaxFractionDigits(currency)))
		if !amount.Equal(amount.Round(digits)) || !credit.Equal(credit.Round(2)) {
			return nil, invalid("recharge tier amounts have too many decimal places")
		}
		key := currency + ":" + amount.String()
		if seen[key] {
			return nil, invalid("duplicate recharge tier amount for the same currency")
		}
		seen[key] = true
		tier.Currency = currency
		out = append(out, tier)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Currency != out[j].Currency {
			return out[i].Currency < out[j].Currency
		}
		return out[i].Amount < out[j].Amount
	})
	return out, nil
}

func decodeRechargeTiers(raw string) ([]RechargeTier, error) {
	if raw == "" {
		return []RechargeTier{}, nil
	}
	var tiers []RechargeTier
	if err := json.Unmarshal([]byte(raw), &tiers); err != nil {
		return nil, fmt.Errorf("decode recharge tiers: %w", err)
	}
	return normalizeRechargeTiers(tiers)
}
