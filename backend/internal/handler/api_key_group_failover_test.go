//go:build unit

package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
