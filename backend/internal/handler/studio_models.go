package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// studioAvailableModels only advertises defaults for platforms with schedulable
// accounts, so an empty group cannot claim models before a usable group does.
func (h *GatewayHandler) studioAvailableModels(ctx context.Context, group *service.Group) []string {
	platforms := h.gatewayService.GetSchedulablePlatforms(ctx, &group.ID)
	if len(platforms) == 0 {
		return nil
	}
	if group.Platform == service.PlatformComposite {
		return h.compositeAvailableModels(ctx, &group.ID, "", false)
	}
	if _, ok := platforms[group.Platform]; !ok {
		return nil
	}
	ids := h.gatewayService.GetAvailableModels(ctx, &group.ID, group.Platform)
	return modelListingSource(group.Platform, ids, defaultModelIDsForPlatform(group.Platform))
}
