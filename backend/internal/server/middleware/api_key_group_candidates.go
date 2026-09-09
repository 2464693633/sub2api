package middleware

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const apiKeyGroupCandidatesContextKey = "api_key_group_candidates"

// APIKeyGroupCandidate is one runtime-eligible group in the order configured
// on an API key. Subscription is populated only for subscription groups.
type APIKeyGroupCandidate struct {
	Group        *service.Group
	Subscription *service.UserSubscription
}

func configuredAPIKeyGroups(apiKey *service.APIKey) []*service.Group {
	if apiKey == nil {
		return nil
	}
	if len(apiKey.Groups) > 0 {
		return apiKey.Groups
	}
	if apiKey.Group != nil {
		return []*service.Group{apiKey.Group}
	}
	return nil
}

// ResolveAPIKeyGroupCandidates filters the configured order to groups that are
// currently active, authorized for the user, and have a usable subscription.
// Request-global key/user quota checks stay in the auth middleware.
func ResolveAPIKeyGroupCandidates(
	ctx context.Context,
	apiKey *service.APIKey,
	subscriptionService *service.SubscriptionService,
	enforceBilling bool,
) []APIKeyGroupCandidate {
	groups := configuredAPIKeyGroups(apiKey)
	if len(groups) == 0 {
		return nil
	}

	candidates := make([]APIKeyGroupCandidate, 0, len(groups))
	seen := make(map[int64]struct{}, len(groups))
	expectedPlatform := ""
	for _, group := range groups {
		if group == nil || group.ID <= 0 {
			continue
		}
		if _, duplicate := seen[group.ID]; duplicate {
			continue
		}
		seen[group.ID] = struct{}{}

		candidateKey := cloneAPIKeyForCandidate(apiKey, group)
		if _, _, available := validateAPIKeyGroupAvailable(candidateKey); !available || !validateAPIKeyGroupAllowed(candidateKey) {
			continue
		}
		if expectedPlatform == "" {
			expectedPlatform = group.Platform
		} else if group.Platform != expectedPlatform {
			continue
		}

		candidate := APIKeyGroupCandidate{Group: group}
		if enforceBilling && !group.IsSubscriptionType() && apiKey.User != nil && apiKey.User.Balance <= 0 {
			continue
		}
		if enforceBilling && group.IsSubscriptionType() {
			if subscriptionService == nil {
				continue
			}
			subscription, err := subscriptionService.GetActiveSubscription(ctx, apiKey.UserID, group.ID)
			if err != nil {
				continue
			}
			needsMaintenance, validateErr := subscriptionService.ValidateAndCheckLimits(subscription, group)
			if needsMaintenance {
				subscription, err = subscriptionService.EnsureWindowMaintenance(ctx, subscription)
				if err != nil {
					continue
				}
				_, validateErr = subscriptionService.ValidateAndCheckLimits(subscription, group)
			}
			if validateErr != nil {
				continue
			}
			candidate.Subscription = subscription
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func cloneAPIKeyForCandidate(apiKey *service.APIKey, group *service.Group) *service.APIKey {
	if apiKey == nil || group == nil {
		return apiKey
	}
	cloned := *apiKey
	groupID := group.ID
	cloned.GroupID = &groupID
	cloned.Group = group
	return &cloned
}

// SetAPIKeyGroupCandidates installs the request-local ordered candidate list.
func SetAPIKeyGroupCandidates(c *gin.Context, candidates []APIKeyGroupCandidate) {
	if c == nil {
		return
	}
	c.Set(apiKeyGroupCandidatesContextKey, candidates)
}

// GetAPIKeyGroupCandidates returns a copy of the request-local ordered list.
func GetAPIKeyGroupCandidates(c *gin.Context) []APIKeyGroupCandidate {
	if c == nil {
		return nil
	}
	value, ok := c.Get(apiKeyGroupCandidatesContextKey)
	if !ok {
		return nil
	}
	candidates, ok := value.([]APIKeyGroupCandidate)
	if !ok {
		return nil
	}
	return append([]APIKeyGroupCandidate(nil), candidates...)
}

// ActivateAPIKeyGroup changes the request-local compatibility fields to the
// group that owns the current attempt. The configured Groups/GroupIDs order is
// retained on apiKey for later failover attempts.
func ActivateAPIKeyGroup(c *gin.Context, apiKey *service.APIKey, candidate APIKeyGroupCandidate) bool {
	if c == nil || apiKey == nil || candidate.Group == nil {
		return false
	}
	groupID := candidate.Group.ID
	apiKey.GroupID = &groupID
	apiKey.Group = candidate.Group
	if apiKey.User != nil {
		apiKey.User.UserGroupRPMOverride = nil
		if override, ok := apiKey.GroupRPMOverrides[groupID]; ok {
			overrideCopy := override
			apiKey.User.UserGroupRPMOverride = &overrideCopy
		}
	}
	c.Set(string(ContextKeyAPIKey), apiKey)
	// A typed nil deliberately shadows a subscription from the preceding group.
	c.Set(string(ContextKeySubscription), candidate.Subscription)
	ctx := context.WithValue(c.Request.Context(), ctxkey.Group, candidate.Group)
	c.Request = c.Request.WithContext(ctx)
	SetBillableUsageResponseGroup(c, candidate.Group)
	return true
}

func candidateForActiveGroup(candidates []APIKeyGroupCandidate, groupID *int64) (APIKeyGroupCandidate, bool) {
	if groupID == nil {
		return APIKeyGroupCandidate{}, false
	}
	for _, candidate := range candidates {
		if candidate.Group != nil && candidate.Group.ID == *groupID {
			return candidate, true
		}
	}
	return APIKeyGroupCandidate{}, false
}

func isGroupSpecificSubscriptionError(err error) bool {
	return errors.Is(err, service.ErrSubscriptionNotFound) ||
		errors.Is(err, service.ErrSubscriptionExpired) ||
		errors.Is(err, service.ErrSubscriptionSuspended) ||
		errors.Is(err, service.ErrDailyLimitExceeded) ||
		errors.Is(err, service.ErrWeeklyLimitExceeded) ||
		errors.Is(err, service.ErrMonthlyLimitExceeded)
}
