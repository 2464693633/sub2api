package handler

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// apiKeyGroupFailoverState owns only cross-group ordering. Account selection,
// same-account retry, and account exclusions remain in the existing inner
// failover loops and are reset when this state advances.
type apiKeyGroupFailoverState struct {
	candidates []middleware2.APIKeyGroupCandidate
	index      int
	accept     func(*service.Group) bool
}

func (s *apiKeyGroupFailoverState) require(accept func(*service.Group) bool) *apiKeyGroupFailoverState {
	if s != nil {
		s.accept = accept
	}
	return s
}

func groupReasoningPolicyMatches(reference, candidate *service.Group) bool {
	if reference == nil || candidate == nil {
		return reference == candidate
	}
	return reference.MaxReasoningEffort == candidate.MaxReasoningEffort &&
		reference.MaxReasoningEffortOverLimit == candidate.MaxReasoningEffortOverLimit &&
		slices.Equal(reference.ReasoningEffortMappings, candidate.ReasoningEffortMappings)
}

func requireGroupConditions(conditions ...func(*service.Group) bool) func(*service.Group) bool {
	return func(group *service.Group) bool {
		for _, condition := range conditions {
			if condition != nil && !condition(group) {
				return false
			}
		}
		return true
	}
}

func newAPIKeyGroupFailoverState(c *gin.Context, apiKey *service.APIKey) *apiKeyGroupFailoverState {
	candidates := middleware2.GetAPIKeyGroupCandidates(c)
	if len(candidates) == 0 && apiKey != nil && apiKey.Group != nil {
		candidates = []middleware2.APIKeyGroupCandidate{{Group: apiKey.Group}}
	}
	if !apiKeyGroupFailoverReplaySupported(c) && len(candidates) > 1 {
		candidates = candidates[:1]
	}
	state := &apiKeyGroupFailoverState{candidates: candidates, index: -1}
	if apiKey != nil && apiKey.GroupID != nil {
		for i := range candidates {
			if candidates[i].Group != nil && candidates[i].Group.ID == *apiKey.GroupID {
				state.index = i
				break
			}
		}
	}
	if state.index < 0 && len(candidates) > 0 {
		state.index = 0
	}
	return state
}

// apiKeyGroupFailoverReplaySupported excludes protocols where an upstream may
// own durable state before the client sees a response. Those handlers remain
// pinned to the group selected before handshake/task submission.
func apiKeyGroupFailoverReplaySupported(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := strings.ToLower(c.Request.URL.Path)
	if c.Request.Method == http.MethodGet && strings.HasSuffix(strings.TrimRight(path, "/"), "/responses") {
		return false // Responses WebSocket
	}
	for _, marker := range []string{"/async", "/batches", "/videos", "/live", "/realtime"} {
		if strings.Contains(path, marker) {
			return false
		}
	}
	return true
}

func activateFirstConfiguredAPIKeyGroup(c *gin.Context, apiKey *service.APIKey, accept func(*service.Group) bool) bool {
	for _, candidate := range middleware2.GetAPIKeyGroupCandidates(c) {
		if candidate.Group != nil && (accept == nil || accept(candidate.Group)) {
			return middleware2.ActivateAPIKeyGroup(c, apiKey, candidate)
		}
	}
	return false
}

func (s *apiKeyGroupFailoverState) hasNext() bool {
	return s != nil && s.index+1 < len(s.candidates)
}

// advance activates the next billable candidate. A candidate whose limits
// changed since authentication is skipped; a billing infrastructure failure is
// returned because trying more groups cannot make that dependency healthy.
func (s *apiKeyGroupFailoverState) advance(
	ctx context.Context,
	c *gin.Context,
	apiKey *service.APIKey,
	billing *service.BillingCacheService,
) (*service.UserSubscription, error, bool) {
	if s == nil || apiKey == nil {
		return nil, nil, false
	}
	for s.index+1 < len(s.candidates) {
		s.index++
		candidate := s.candidates[s.index]
		if candidate.Group == nil {
			continue
		}
		if s.accept != nil && !s.accept(candidate.Group) {
			continue
		}
		if candidate.Group.IsSubscriptionType() && candidate.Subscription == nil {
			continue
		}
		if billing != nil {
			err := billing.CheckBillingEligibilityForGroupFailover(
				ctx,
				apiKey.User,
				candidate.Group,
				candidate.Subscription,
				candidate.Group.Platform,
			)
			if err != nil {
				if errors.Is(err, service.ErrBillingServiceUnavailable) {
					return nil, err, false
				}
				continue
			}
		}
		if middleware2.ActivateAPIKeyGroup(c, apiKey, candidate) {
			return candidate.Subscription, nil, true
		}
	}
	return nil, nil, false
}

func isGroupRecoverableBillingError(err error) bool {
	return errors.Is(err, service.ErrInsufficientBalance) ||
		errors.Is(err, service.ErrSubscriptionInvalid) ||
		errors.Is(err, service.ErrSubscriptionNotFound) ||
		errors.Is(err, service.ErrSubscriptionExpired) ||
		errors.Is(err, service.ErrSubscriptionSuspended) ||
		errors.Is(err, service.ErrDailyLimitExceeded) ||
		errors.Is(err, service.ErrWeeklyLimitExceeded) ||
		errors.Is(err, service.ErrMonthlyLimitExceeded) ||
		errors.Is(err, service.ErrGroupRPMExceeded) ||
		errors.Is(err, service.ErrUserPlatformDailyQuotaExhausted) ||
		errors.Is(err, service.ErrUserPlatformWeeklyQuotaExhausted) ||
		errors.Is(err, service.ErrUserPlatformMonthlyQuotaExhausted)
}

// recoverInitialGroupBilling advances past a group-local billing rejection and
// runs the full preflight on the replacement. Global key limits and user RPM
// failures are returned without trying another group.
func recoverInitialGroupBilling(
	c *gin.Context,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	billing *service.BillingCacheService,
	initialErr error,
	accept func(*service.Group) bool,
) (*service.UserSubscription, bool, error) {
	if !isGroupRecoverableBillingError(initialErr) {
		return subscription, false, initialErr
	}
	state := newAPIKeyGroupFailoverState(c, apiKey).require(accept)
	lastErr := initialErr
	for state.hasNext() {
		nextSubscription, _, advanced := state.advance(c.Request.Context(), c, apiKey, nil)
		if !advanced {
			break
		}
		nextErr := billing.CheckBillingEligibility(
			c.Request.Context(), apiKey.User, apiKey, apiKey.Group, nextSubscription,
			service.QuotaPlatform(c.Request.Context(), apiKey),
		)
		if nextErr == nil {
			return nextSubscription, true, nil
		}
		lastErr = nextErr
		if !isGroupRecoverableBillingError(nextErr) {
			break
		}
	}
	return subscription, false, lastErr
}

func shouldTryNextAPIKeyGroup(c *gin.Context, failoverErr *service.UpstreamFailoverError) bool {
	if c == nil || c.Request == nil || c.Request.Context().Err() != nil || failoverErr == nil {
		return false
	}
	if service.IsResponseCommitted(c) || !failoverErr.ShouldRetryNextAccount() {
		return false
	}
	// The same request or same provider will fail in every same-platform group.
	return failoverErr.Scope != service.GatewayFailureScopeRequest &&
		failoverErr.Scope != service.GatewayFailureScopeProvider
}
