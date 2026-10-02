//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestStudioModelFiltersClassifyGrokImagineAliases(t *testing.T) {
	for _, id := range []string{"grok-imagine", "grok-imagine-1", "grok-imagine-edit", "xai/grok-imagine-edit"} {
		require.True(t, isImageStudioModel(id, service.PlatformGrok), id)
		require.False(t, isVideoStudioModel(id, service.PlatformGrok), id)
	}
	for _, id := range []string{"grok-imagine-video", "grok-imagine-video-1.5", "grok-video-1.5"} {
		require.False(t, isImageStudioModel(id, service.PlatformGrok), id)
		require.True(t, isVideoStudioModel(id, service.PlatformGrok), id)
	}
}

func TestImageStudioOpenAIDefaultsContainImageModels(t *testing.T) {
	found := false
	for _, id := range defaultModelIDsForPlatform(service.PlatformOpenAI) {
		if isImageStudioModel(id, service.PlatformOpenAI) {
			found = true
			break
		}
	}
	require.True(t, found)
}
