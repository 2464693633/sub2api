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
	if group.Platform == service.PlatformKling {
		if len(ids) == 0 {
			ids = append(service.DefaultKlingModelIDs(), "kling-4.0")
		}
		videos := make([]string, 0, len(ids))
		for _, id := range ids {
			if service.IsKlingVideoModel(id) {
				videos = append(videos, id)
			}
		}
		return videos
	}
	return modelListingSource(group.Platform, ids, defaultModelIDsForPlatform(group.Platform))
}
