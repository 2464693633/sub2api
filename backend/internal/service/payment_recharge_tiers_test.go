//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestRechargeTiersExactAmountAndCurrency(t *testing.T) {
	cfg := &PaymentConfig{BalanceRechargeMultiplier: 10, BalanceRechargeTiers: []RechargeTier{
		{Amount: 100, CreditAmount: 1100, Currency: "CNY"},
		{Amount: 500, CreditAmount: 6000, Currency: "CNY"},
		{Amount: 100, CreditAmount: 150, Currency: "USD"},
	}}
	for _, tc := range []struct {
		amount   float64
		currency string
		want     float64
	}{
		{100, "CNY", 1100}, {500, "CNY", 6000}, {100, "USD", 150},
		{100, "EUR", 1000}, {99.99, "CNY", 999.90}, {100.01, "CNY", 1000.10},
		{100.001, "CNY", 1000.01},
	} {
		require.Equal(t, tc.want, cfg.creditedBalance(tc.amount, tc.currency))
	}
}

func TestRechargeTiersConfigRoundTripAndPatch(t *testing.T) {
	ctx := context.Background()
	repo := &paymentConfigSettingRepoStub{}
	svc := &PaymentConfigService{settingRepo: repo}
	tiers := []RechargeTier{{Amount: 100, CreditAmount: 1100, Currency: "cny"}}
	require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{BalanceRechargeTiers: &tiers}))
	cfg, err := svc.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, []RechargeTier{{Amount: 100, CreditAmount: 1100, Currency: "CNY"}}, cfg.BalanceRechargeTiers)

	multiplier := 5.0
	require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{BalanceRechargeMultiplier: &multiplier}))
	cfg, err = svc.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, 1100.0, cfg.creditedBalance(100, "CNY"), "tier is final credit, not another multiplier")

	empty := []RechargeTier{}
	require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{BalanceRechargeTiers: &empty}))
	cfg, err = svc.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.Empty(t, cfg.BalanceRechargeTiers)
	require.Equal(t, 500.0, cfg.creditedBalance(100, "CNY"))
}

func TestRechargeTiersRejectInvalidConfigBeforeWriting(t *testing.T) {
	for name, tiers := range map[string][]RechargeTier{
		"zero":                  {{Amount: 0, CreditAmount: 1100, Currency: "CNY"}},
		"negative credit":       {{Amount: 100, CreditAmount: -1, Currency: "CNY"}},
		"not finite":            {{Amount: math.Inf(1), CreditAmount: 100, Currency: "CNY"}},
		"fraction":              {{Amount: 100.001, CreditAmount: 1100, Currency: "CNY"}},
		"credit fraction":       {{Amount: 100, CreditAmount: 1100.001, Currency: "CNY"}},
		"zero decimal currency": {{Amount: 100.50, CreditAmount: 1100, Currency: "JPY"}},
		"invalid currency":      {{Amount: 100, CreditAmount: 1100, Currency: "US1"}},
		"duplicate":             {{Amount: 100, CreditAmount: 1100, Currency: "CNY"}, {Amount: 100, CreditAmount: 1200, Currency: "cny"}},
		"too many":              make([]RechargeTier, 51),
	} {
		t.Run(name, func(t *testing.T) {
			repo := &paymentConfigSettingRepoStub{}
			svc := &PaymentConfigService{settingRepo: repo}
			require.Error(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{BalanceRechargeTiers: &tiers}))
			require.Empty(t, repo.updates)
		})
	}
}

func TestRechargeTierOrderFreezesCreditAndFulfillsOnce(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	user, err := client.User.Create().SetEmail("tier@example.com").SetPasswordHash("hash").SetUsername("tier-user").Save(ctx)
	require.NoError(t, err)
	cfg := &PaymentConfig{BalanceRechargeMultiplier: 10, BalanceRechargeTiers: []RechargeTier{{Amount: 100, CreditAmount: 1100, Currency: "CNY"}}}
	credited := 0.0
	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID}}
	userRepo.updateBalanceFn = func(_ context.Context, _ int64, amount float64) error { credited += amount; return nil }
	redeemRepo := &paymentFulfillmentRedeemRepo{}
	redeemSvc := NewRedeemService(redeemRepo, userRepo, nil, &paymentFulfillmentRedeemCacheStub{}, nil, client, nil, nil)
	svc := &PaymentService{entClient: client, userRepo: userRepo, redeemService: redeemSvc}
	order, err := svc.createOrderInTx(ctx,
		CreateOrderRequest{UserID: user.ID, Amount: 100, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance},
		&User{ID: user.ID, Email: user.Email}, nil, cfg, cfg.creditedBalance(100, "CNY"), 100, 2.5, 102.5,
		&payment.InstanceSelection{ProviderKey: payment.TypeAlipay},
	)
	require.NoError(t, err)
	require.Equal(t, 1100.0, order.Amount)
	require.Equal(t, 102.5, order.PayAmount)
	cfg.BalanceRechargeTiers = nil // A later configuration change must not change the pending order.
	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusPaid).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	require.Equal(t, 1100.0, credited)
	require.Equal(t, 102.5, calculateGatewayRefundAmount(order.Amount, order.PayAmount, 1100, "CNY"))
	require.Equal(t, 51.25, calculateGatewayRefundAmount(order.Amount, order.PayAmount, 550, "CNY"))
}
