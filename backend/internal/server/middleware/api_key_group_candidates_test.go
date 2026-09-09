//go:build unit

package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveAPIKeyGroupCandidatesLocksPlatformToFirstValidGroup(t *testing.T) {
	anthropicFirst := &service.Group{ID: 1, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	driftedOpenAI := &service.Group{ID: 2, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	anthropicLast := &service.Group{ID: 3, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	key := &service.APIKey{
		UserID: 9,
		User:   &service.User{ID: 9, Status: service.StatusActive, Balance: 1},
		Groups: []*service.Group{anthropicFirst, driftedOpenAI, anthropicLast},
	}

	candidates := ResolveAPIKeyGroupCandidates(context.Background(), key, nil, false)
	require.Len(t, candidates, 2)
	require.Equal(t, int64(1), candidates[0].Group.ID)
	require.Equal(t, int64(3), candidates[1].Group.ID)
}

func TestActivateAPIKeyGroupUpdatesBillingAndRPMContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	first := &service.Group{ID: 1, Platform: service.PlatformAnthropic}
	second := &service.Group{ID: 2, Platform: service.PlatformAnthropic}
	firstID := first.ID
	key := &service.APIKey{
		GroupID:           &firstID,
		Group:             first,
		Groups:            []*service.Group{first, second},
		GroupIDs:          []int64{first.ID, second.ID},
		GroupRPMOverrides: map[int64]int{second.ID: 17},
		User:              &service.User{ID: 9},
	}

	require.True(t, ActivateAPIKeyGroup(c, key, APIKeyGroupCandidate{Group: second}))
	require.Equal(t, second.ID, *key.GroupID)
	require.Same(t, second, key.Group)
	require.NotNil(t, key.User.UserGroupRPMOverride)
	require.Equal(t, 17, *key.User.UserGroupRPMOverride)
	stored, ok := GetAPIKeyFromContext(c)
	require.True(t, ok)
	require.Same(t, key, stored)
}
