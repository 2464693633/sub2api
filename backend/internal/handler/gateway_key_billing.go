package handler

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const keyBillingInfoSchemaVersion = 1

const keyBillingInfoMultiGroupSchemaVersion = 2

type keyBillingGroupInfo struct {
	GroupID                      int64    `json:"group_id"`
	Position                     int      `json:"position"`
	BillingType                  string   `json:"billing_type"`
	GroupRateMultiplier          float64  `json:"group_rate_multiplier"`
	UserRateMultiplier           *float64 `json:"user_rate_multiplier,omitempty"`
	ResolvedRateMultiplier       float64  `json:"resolved_rate_multiplier"`
	PeakRateEnabled              bool     `json:"peak_rate_enabled"`
	PeakStart                    *string  `json:"peak_start,omitempty"`
	PeakEnd                      *string  `json:"peak_end,omitempty"`
	PeakRateMultiplier           *float64 `json:"peak_rate_multiplier,omitempty"`
	AppliedPeakMultiplier        *float64 `json:"applied_peak_multiplier,omitempty"`
	EffectiveRateMultiplier      float64  `json:"effective_rate_multiplier"`
	Timezone                     *string  `json:"timezone,omitempty"`
	InputTokenMultiplier         float64  `json:"input_token_multiplier"`
	OutputTokenMultiplier        float64  `json:"output_token_multiplier"`
	CacheCreationTokenMultiplier float64  `json:"cache_creation_token_multiplier"`
	CacheReadTokenMultiplier     float64  `json:"cache_read_token_multiplier"`
	ReturnBillableUsage          bool     `json:"return_billable_usage"`
}

type keyBillingInfoResponse struct {
	Object                  string                `json:"object"`
	SchemaVersion           int                   `json:"schema_version"`
	BillingScope            string                `json:"billing_scope"`
	GroupRateMultiplier     float64               `json:"group_rate_multiplier"`
	UserRateMultiplier      *float64              `json:"user_rate_multiplier,omitempty"`
	ResolvedRateMultiplier  float64               `json:"resolved_rate_multiplier"`
	PeakRateEnabled         bool                  `json:"peak_rate_enabled"`
	PeakStart               *string               `json:"peak_start,omitempty"`
	PeakEnd                 *string               `json:"peak_end,omitempty"`
	PeakRateMultiplier      *float64              `json:"peak_rate_multiplier,omitempty"`
	AppliedPeakMultiplier   *float64              `json:"applied_peak_multiplier,omitempty"`
	EffectiveRateMultiplier float64               `json:"effective_rate_multiplier"`
	Timezone                *string               `json:"timezone,omitempty"`
	RoutingMode             string                `json:"routing_mode,omitempty"`
	Groups                  []keyBillingGroupInfo `json:"groups,omitempty"`
	ObservedAt              time.Time             `json:"observed_at"`
}

// KeyBillingInfo returns the token billing multiplier effective for the authenticated API key.
// GET /v1/sub2api/billing
func (h *GatewayHandler) KeyBillingInfo(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if h.cfg != nil && h.cfg.RunMode == config.RunModeSimple {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Billing information is not supported in simple mode")
		return
	}
	if apiKey.GroupID == nil {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "API key is not assigned to a group")
		return
	}
	if apiKey.Group == nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Billing information is unavailable")
		return
	}

	groups := apiKey.Groups
	if len(groups) == 0 {
		groups = []*service.Group{apiKey.Group}
	}
	resolvedRates := make([]float64, len(groups))
	for i, group := range groups {
		resolvedRate, resolved := h.resolveKeyBillingRateForGroup(c, apiKey.UserID, group)
		if !resolved {
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "Billing information is unavailable")
			return
		}
		resolvedRates[i] = resolvedRate
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, buildMultiGroupKeyBillingInfo(apiKey, groups, resolvedRates, timezone.Now()))
}

func (h *GatewayHandler) resolveKeyBillingRate(c *gin.Context, apiKey *service.APIKey) (float64, bool) {
	return h.resolveKeyBillingRateForGroup(c, apiKey.UserID, apiKey.Group)
}

func (h *GatewayHandler) resolveKeyBillingRateForGroup(c *gin.Context, userID int64, group *service.Group) (float64, bool) {
	if group == nil {
		return 0, false
	}
	groupRate := group.RateMultiplier
	switch group.Platform {
	case service.PlatformOpenAI, service.PlatformGrok:
		if h.openAIGatewayService == nil {
			return 0, false
		}
		return h.openAIGatewayService.ResolveUserGroupRateMultiplier(c.Request.Context(), userID, group.ID, groupRate), true
	default:
		if h.gatewayService == nil {
			return 0, false
		}
		return h.gatewayService.ResolveUserGroupRateMultiplier(c.Request.Context(), userID, group.ID, groupRate), true
	}
}

func buildKeyBillingInfo(apiKey *service.APIKey, resolvedRate float64, now time.Time) keyBillingInfoResponse {
	return buildMultiGroupKeyBillingInfo(apiKey, []*service.Group{apiKey.Group}, []float64{resolvedRate}, now)
}

func buildMultiGroupKeyBillingInfo(apiKey *service.APIKey, groups []*service.Group, resolvedRates []float64, now time.Time) keyBillingInfoResponse {
	infos := make([]keyBillingGroupInfo, 0, len(groups))
	conservativeIndex := 0
	conservativeRate := 0.0
	for i, group := range groups {
		if group == nil || i >= len(resolvedRates) {
			continue
		}
		info := buildKeyBillingGroupInfo(group, resolvedRates[i], i, now)
		infos = append(infos, info)
		legacyRate := info.EffectiveRateMultiplier * legacyTokenRateFactor(info)
		if len(infos) == 1 || legacyRate > conservativeRate {
			conservativeIndex = len(infos) - 1
			conservativeRate = legacyRate
		}
	}
	if len(infos) == 0 {
		return keyBillingInfoResponse{Object: "sub2api.key_billing", SchemaVersion: keyBillingInfoSchemaVersion, BillingScope: "token", ObservedAt: now.UTC()}
	}
	selected := infos[conservativeIndex]
	legacyTokenFactor := 1.0
	if len(infos) > 1 {
		legacyTokenFactor = legacyTokenRateFactor(selected)
	}
	legacyGroupRate := selected.GroupRateMultiplier * legacyTokenFactor
	legacyResolvedRate := selected.ResolvedRateMultiplier * legacyTokenFactor
	var legacyUserRate *float64
	if selected.UserRateMultiplier != nil {
		value := *selected.UserRateMultiplier * legacyTokenFactor
		legacyUserRate = &value
	}
	response := keyBillingInfoResponse{
		Object:                  "sub2api.key_billing",
		SchemaVersion:           keyBillingInfoSchemaVersion,
		BillingScope:            "token",
		GroupRateMultiplier:     legacyGroupRate,
		UserRateMultiplier:      legacyUserRate,
		ResolvedRateMultiplier:  legacyResolvedRate,
		PeakRateEnabled:         selected.PeakRateEnabled,
		PeakStart:               selected.PeakStart,
		PeakEnd:                 selected.PeakEnd,
		PeakRateMultiplier:      selected.PeakRateMultiplier,
		AppliedPeakMultiplier:   selected.AppliedPeakMultiplier,
		EffectiveRateMultiplier: selected.EffectiveRateMultiplier * legacyTokenFactor,
		Timezone:                selected.Timezone,
		ObservedAt:              now.UTC(),
	}
	if len(infos) > 1 {
		response.SchemaVersion = keyBillingInfoMultiGroupSchemaVersion
		response.RoutingMode = "ordered_failover"
		response.Groups = infos
	}
	return response
}

func buildKeyBillingGroupInfo(group *service.Group, resolvedRate float64, position int, now time.Time) keyBillingGroupInfo {
	group.EnsureTokenMultiplierDefaults()
	groupRate := group.RateMultiplier
	var userRate *float64
	if resolvedRate != groupRate {
		userRate = &resolvedRate
	}
	appliedPeak := group.PeakMultiplierAt(now)
	info := keyBillingGroupInfo{
		GroupID:                      group.ID,
		Position:                     position,
		BillingType:                  group.SubscriptionType,
		GroupRateMultiplier:          groupRate,
		UserRateMultiplier:           userRate,
		ResolvedRateMultiplier:       resolvedRate,
		PeakRateEnabled:              group.PeakRateEnabled,
		EffectiveRateMultiplier:      resolvedRate * appliedPeak,
		InputTokenMultiplier:         group.InputTokenMultiplier,
		OutputTokenMultiplier:        group.OutputTokenMultiplier,
		CacheCreationTokenMultiplier: group.CacheCreationTokenMultiplier,
		CacheReadTokenMultiplier:     group.CacheReadTokenMultiplier,
		ReturnBillableUsage:          group.ReturnBillableUsage,
	}
	if group.PeakRateEnabled {
		info.PeakStart = &group.PeakStart
		info.PeakEnd = &group.PeakEnd
		info.PeakRateMultiplier = &group.PeakRateMultiplier
		info.AppliedPeakMultiplier = &appliedPeak
		tz := timezone.Location().String()
		info.Timezone = &tz
	}
	return info
}

func legacyTokenRateFactor(info keyBillingGroupInfo) float64 {
	if info.ReturnBillableUsage {
		return 1
	}
	factor := info.InputTokenMultiplier
	for _, multiplier := range []float64{
		info.OutputTokenMultiplier,
		info.CacheCreationTokenMultiplier,
		info.CacheReadTokenMultiplier,
	} {
		if multiplier > factor {
			factor = multiplier
		}
	}
	return factor
}
