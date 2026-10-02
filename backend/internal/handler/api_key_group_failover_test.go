//go:build unit

package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type failoverBalanceCache struct {
	service.BillingCache
}

type groupInflightEstimator map[int64]float64

func (e groupInflightEstimator) EstimateInflightReservation(_ context.Context, key *service.APIKey, _ service.InflightEstimateRequest) (float64, bool) {
	cost, ok := e[key.Group.ID]
	return cost, ok
}

func TestAPIKeyGroupFailoverRefreshesInflightReservation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		otherCost  float64
		nextCosts  []float64
		wantGroup  int64
		wantAmount float64
		wantError  bool
	}{
		{name: "does not count its own reservation twice", nextCosts: []float64{0.9}, wantGroup: 2, wantAmount: 0.9},
		{name: "skips unaffordable group", otherCost: 0.4, nextCosts: []float64{0.8, 0.5}, wantGroup: 3, wantAmount: 0.5},
		{name: "rejected replacement preserves original", otherCost: 0.4, nextCosts: []float64{0.8}, wantGroup: 1, wantAmount: 0.2, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			cache := newHandlerInflightCache(1)
			cfg := &config.Config{}
			cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
			billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			first := &service.Group{ID: 1, Platform: service.PlatformOpenAI}
			key := &service.APIKey{GroupID: &first.ID, Group: first, User: &service.User{ID: 7}}
			other, err := billing.ReserveInflight(c.Request.Context(), key.User, first, nil, tc.otherCost)
			require.NoError(t, err)
			defer other.HandlerDone()
			candidates := []middleware2.APIKeyGroupCandidate{{Group: first}}
			estimator := groupInflightEstimator{first.ID: 0.2}
			for i, cost := range tc.nextCosts {
				group := &service.Group{ID: int64(i + 2), Platform: service.PlatformOpenAI}
				candidates = append(candidates, middleware2.APIKeyGroupCandidate{Group: group})
				estimator[group.ID] = cost
			}
			middleware2.SetAPIKeyGroupCandidates(c, candidates)
			done, err := reserveInflightBalance(c, billing, estimator, key, nil, tokenInflightEstimate("model", nil))
			require.NoError(t, err)
			defer done()
			original := service.InflightReservationFromContext(c.Request.Context())

			_, err, advanced := newAPIKeyGroupFailoverState(c, key).advance(c.Request.Context(), c, key, nil)
			if tc.wantError {
				require.ErrorIs(t, err, service.ErrInsufficientBalance)
				require.False(t, advanced)
			} else {
				require.NoError(t, err)
				require.True(t, advanced)
			}
			require.Equal(t, tc.wantGroup, key.Group.ID)
			current := service.InflightReservationFromContext(c.Request.Context())
			require.Same(t, original, current)
			require.InDelta(t, tc.wantAmount, current.Amount(), 1e-9)
			usageDone := current.Acquire()
			done()
			wantCount := 1
			if tc.otherCost > 0 {
				wantCount++
			}
			require.Equal(t, wantCount, cache.count(), "the final reservation survives until async billing finishes")
			usageDone()
			require.Equal(t, wantCount-1, cache.count())
		})
	}
}

func TestAPIKeyGroupFailoverInflightSubscriptionTransitions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	cache := newHandlerInflightCache(1)
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
	billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	first := &service.Group{ID: 1, Platform: service.PlatformAnthropic}
	subscribed := &service.Group{ID: 2, Platform: service.PlatformAnthropic, SubscriptionType: service.SubscriptionTypeSubscription}
	third := &service.Group{ID: 3, Platform: service.PlatformAnthropic}
	key := &service.APIKey{GroupID: &first.ID, Group: first, User: &service.User{ID: 7}}
	subscription := &service.UserSubscription{ID: 20}
	middleware2.SetAPIKeyGroupCandidates(c, []middleware2.APIKeyGroupCandidate{{Group: first}, {Group: subscribed, Subscription: subscription}, {Group: third}})
	done, err := reserveInflightBalance(c, billing, groupInflightEstimator{1: 0.2, 3: 0.8}, key, nil, tokenInflightEstimate("model", nil))
	require.NoError(t, err)
	defer done()
	state := newAPIKeyGroupFailoverState(c, key)

	gotSub, err, advanced := state.advance(c.Request.Context(), c, key, nil)
	require.NoError(t, err)
	require.True(t, advanced)
	require.Same(t, subscription, gotSub)
	require.Nil(t, service.InflightReservationFromContext(c.Request.Context()), "subscription must shadow the released source handle")
	require.Zero(t, cache.count())

	gotSub, err, advanced = state.advance(c.Request.Context(), c, key, nil)
	require.NoError(t, err)
	require.True(t, advanced)
	require.Nil(t, gotSub)
	current := service.InflightReservationFromContext(c.Request.Context())
	require.NotNil(t, current)
	require.InDelta(t, 0.8, current.Amount(), 1e-9)
	require.Equal(t, 1, cache.count())
	usageDone := current.Acquire()
	done()
	require.Equal(t, 1, cache.count())
	usageDone()
	require.Zero(t, cache.count())
}

func TestAPIKeyGroupFailoverInflightStartsOnSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	cache := newHandlerInflightCache(1)
	cfg := &config.Config{}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
	billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	first := &service.Group{ID: 1, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeSubscription}
	second := &service.Group{ID: 2, Platform: service.PlatformOpenAI}
	subscription := &service.UserSubscription{ID: 20}
	key := &service.APIKey{GroupID: &first.ID, Group: first, User: &service.User{ID: 7}}
	middleware2.SetAPIKeyGroupCandidates(c, []middleware2.APIKeyGroupCandidate{{Group: first, Subscription: subscription}, {Group: second}})
	done, err := reserveInflightBalance(c, billing, groupInflightEstimator{2: 0.8}, key, subscription, tokenInflightEstimate("model", nil))
	require.NoError(t, err)
	defer done()
	require.Nil(t, service.InflightReservationFromContext(c.Request.Context()))

	_, err, advanced := newAPIKeyGroupFailoverState(c, key).advance(c.Request.Context(), c, key, nil)
	require.NoError(t, err)
	require.True(t, advanced)
	require.Equal(t, 1, cache.count())
	require.InDelta(t, 0.8, service.InflightReservationFromContext(c.Request.Context()).Amount(), 1e-9)
	done()
	require.Zero(t, cache.count())
}

func (*failoverBalanceCache) GetUserBalance(context.Context, int64) (float64, error) {
	return 100, nil
}

type failoverRPMCache struct {
	service.UserRPMCache
	calls int
}

func (s *failoverRPMCache) IncrementUserGroupRPM(context.Context, int64, int64) (int, error) {
	s.calls++
	return 2, nil
}

type failoverRPMRepo struct {
	service.UserGroupRateRepository
	calls   int
	groupID int64
}

func (s *failoverRPMRepo) GetRPMOverrideByUserAndGroup(_ context.Context, _ int64, groupID int64) (*int, error) {
	s.calls++
	s.groupID = groupID
	limit := 1
	return &limit, nil
}

func TestAPIKeyGroupFailoverResolvesDestinationRPM(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cached     map[int64]int
		wantAllow  bool
		wantLookup int
		wantCount  int
	}{
		{name: "missing cache looks up destination override", wantLookup: 1, wantCount: 1},
		{name: "source-only cache looks up destination override", cached: map[int64]int{11: 0}, wantLookup: 1, wantCount: 1},
		{name: "explicit destination exemption", cached: map[int64]int{22: 0}, wantAllow: true},
		{name: "cached destination limit", cached: map[int64]int{22: 3}, wantAllow: true, wantCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			first := &service.Group{ID: 11, Platform: service.PlatformOpenAI}
			second := &service.Group{ID: 22, Platform: service.PlatformOpenAI, RPMLimit: 10}
			sourceOverride := 0
			key := &service.APIKey{
				GroupID: &first.ID, Group: first,
				User: &service.User{ID: 7, UserGroupRPMOverride: &sourceOverride}, GroupRPMOverrides: tc.cached,
			}
			middleware2.SetAPIKeyGroupCandidates(c, []middleware2.APIKeyGroupCandidate{{Group: first}, {Group: second}})
			rpm := &failoverRPMCache{}
			repo := &failoverRPMRepo{}
			billing := service.NewBillingCacheService(&failoverBalanceCache{}, nil, nil, nil, rpm, repo, &config.Config{}, nil)
			t.Cleanup(billing.Stop)

			_, err, advanced := newAPIKeyGroupFailoverState(c, key).advance(context.Background(), c, key, billing)

			require.NoError(t, err)
			require.Equal(t, tc.wantAllow, advanced)
			require.Equal(t, tc.wantLookup, repo.calls)
			require.Equal(t, tc.wantCount, rpm.calls)
			if tc.wantLookup > 0 {
				require.Equal(t, second.ID, repo.groupID)
			}
		})
	}
}

func TestAPIKeyGroupFailoverStateAdvancesInConfiguredOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	first := &service.Group{ID: 11, Platform: service.PlatformAnthropic}
	second := &service.Group{ID: 22, Platform: service.PlatformAnthropic}
	key := &service.APIKey{GroupID: &first.ID, Group: first, User: &service.User{ID: 7}}
	middleware2.SetAPIKeyGroupCandidates(c, []middleware2.APIKeyGroupCandidate{{Group: first}, {Group: second}})

	state := newAPIKeyGroupFailoverState(c, key)
	subscription, err, advanced := state.advance(context.Background(), c, key, nil)
	require.NoError(t, err)
	require.True(t, advanced)
	require.Nil(t, subscription)
	require.Equal(t, second.ID, *key.GroupID)
	require.Same(t, second, key.Group)
	require.False(t, state.hasNext())
}

func TestAPIKeyGroupFailoverSkipsDifferentReasoningPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	first := &service.Group{ID: 11, Platform: service.PlatformOpenAI, MaxReasoningEffort: "high", MaxReasoningEffortOverLimit: "downgrade"}
	stricter := &service.Group{ID: 22, Platform: service.PlatformOpenAI, MaxReasoningEffort: "medium", MaxReasoningEffortOverLimit: "deny"}
	compatible := &service.Group{ID: 33, Platform: service.PlatformOpenAI, MaxReasoningEffort: "high", MaxReasoningEffortOverLimit: "downgrade"}
	firstID := first.ID
	key := &service.APIKey{GroupID: &firstID, Group: first, User: &service.User{ID: 7}}
	middleware2.SetAPIKeyGroupCandidates(c, []middleware2.APIKeyGroupCandidate{{Group: first}, {Group: stricter}, {Group: compatible}})

	state := newAPIKeyGroupFailoverState(c, key).require(func(group *service.Group) bool {
		return groupReasoningPolicyMatches(first, group)
	})
	_, err, advanced := state.advance(context.Background(), c, key, nil)
	require.NoError(t, err)
	require.True(t, advanced)
	require.Equal(t, compatible.ID, *key.GroupID)
}

func TestShouldTryNextAPIKeyGroupRequiresReplayableAccountFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	require.True(t, shouldTryNextAPIKeyGroup(c, &service.UpstreamFailoverError{StatusCode: 503}))
	require.False(t, shouldTryNextAPIKeyGroup(c, &service.UpstreamFailoverError{StatusCode: 503, Scope: service.GatewayFailureScopeRequest}))
	require.False(t, shouldTryNextAPIKeyGroup(c, &service.UpstreamFailoverError{StatusCode: 503, Scope: service.GatewayFailureScopeProvider}))
	require.False(t, shouldTryNextAPIKeyGroup(c, &service.UpstreamFailoverError{StatusCode: 400, NextAccountAction: service.NextAccountStop}))

	service.MarkResponseCommitted(c)
	require.False(t, shouldTryNextAPIKeyGroup(c, &service.UpstreamFailoverError{StatusCode: 503}))
}

func TestAPIKeyGroupFailoverReplayUnsupportedForStatefulEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, request := range []struct {
		method string
		path   string
	}{
		{"GET", "/v1/responses"},
		{"POST", "/v1/images/generations/async"},
		{"POST", "/v1/images/batches"},
		{"POST", "/v1/videos"},
		{"POST", "/v1/live"},
		{"GET", "/v1/realtime"},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(request.method, request.path, nil)
		require.False(t, apiKeyGroupFailoverReplaySupported(c), request.path)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	require.True(t, apiKeyGroupFailoverReplaySupported(c))
}
