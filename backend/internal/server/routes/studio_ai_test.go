package routes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type studioAIRouteKeyRepo struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (s *studioAIRouteKeyRepo) GetByID(context.Context, int64) (*service.APIKey, error) {
	return s.key, nil
}

func newStudioAIRouteKeyService(userID int64) *service.APIKeyService {
	groupID := int64(10)
	repo := &studioAIRouteKeyRepo{key: &service.APIKey{
		ID: 11, UserID: userID, Key: "test-owned-key", Status: service.StatusAPIKeyActive, GroupID: &groupID,
	}}
	return service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)
}

func TestStudioAIRoutesKeepOpsCaptureActiveUntilGatewayReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var outerWriter gin.ResponseWriter
	router.Use(func(c *gin.Context) {
		outerWriter = c.Writer
		c.Next()
		require.Same(t, outerWriter, c.Writer, "Ops must restore its pooled response writer")
		require.Equal(t, "/api/v1/studio/ai-prompt", c.Request.URL.Path)
	})
	called := false
	RegisterStudioAIRoutes(router, &handler.Handlers{}, middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Next()
	}), middleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		called = true
		require.NotSame(t, outerWriter, c.Writer, "Ops capture must wrap the downstream gateway")
		require.Contains(t, fmt.Sprintf("%T", c.Writer), "opsCaptureWriter")
		require.Equal(t, "/v1/chat/completions", c.Request.URL.Path)
		require.Equal(t, "Bearer test-owned-key", c.GetHeader("Authorization"))
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "upstream unavailable"}})
	}), newStudioAIRouteKeyService(7), nil, nil, nil, nil,
		&config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024}}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/studio/ai-prompt",
		strings.NewReader(`{"prompt":"test","kind":"image","key_id":11,"model":"chat-model"}`))
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)

	require.True(t, called)
	require.Equal(t, http.StatusBadGateway, writer.Code)
	require.Contains(t, writer.Body.String(), "upstream unavailable")
}

func TestStudioAIRoutesPrepareFailureStopsGatewayChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name    string
		userID  int64
		keyUser int64
		body    string
		status  int
	}{
		{name: "missing subject", keyUser: 7, body: `{"prompt":"test","key_id":11,"model":"chat-model"}`, status: http.StatusUnauthorized},
		{name: "invalid body", userID: 7, keyUser: 7, body: `{`, status: http.StatusBadRequest},
		{name: "missing key", userID: 7, keyUser: 7, body: `{"prompt":"test","model":"chat-model"}`, status: http.StatusBadRequest},
		{name: "foreign key", userID: 7, keyUser: 8, body: `{"prompt":"test","key_id":11,"model":"chat-model"}`, status: http.StatusBadRequest},
		{name: "missing model", userID: 7, keyUser: 7, body: `{"prompt":"test","key_id":11}`, status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			called := false
			RegisterStudioAIRoutes(router, &handler.Handlers{}, middleware.JWTAuthMiddleware(func(c *gin.Context) {
				if tc.userID > 0 {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: tc.userID})
				}
				c.Next()
			}), middleware.APIKeyAuthMiddleware(func(c *gin.Context) {
				called = true
				c.AbortWithStatus(http.StatusTeapot)
			}), newStudioAIRouteKeyService(tc.keyUser), nil, nil, nil, nil,
				&config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024}}, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/studio/ai-prompt", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, req)

			require.Equal(t, tc.status, writer.Code)
			require.False(t, called, "prepare failure must abort all downstream handlers")
		})
	}
}
