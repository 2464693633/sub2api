package routes

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterStudioAIRoutes 注册 AI 提示词助手接口(/api/v1/studio/*)。
//
// 与 image_studio.go 同一桥接模式:JWT 鉴权后按用户可用分组自动选定对话模型,
// 确保一把「AI提示词」工作台密钥,把请求改写为 /v1/chat/completions 载荷后
// 以该密钥身份重放网关中间件链,计费限流与外部直连密钥口径一致。
// 响应为标准 chat completions JSON,由前端解析 choices[0].message.content。
func RegisterStudioAIRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	apiKeyAuth middleware.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	compositeResolver *service.CompositeRouteResolver,
	cfg *config.Config,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	bodyLimit := middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize)
	clientRequestID := middleware.ClientRequestID()
	opsErrorLogger := handler.OpsErrorLoggerMiddleware(opsService)
	endpointNorm := handler.InboundEndpointMiddleware()
	groupModelAllowlist := middleware.GroupModelAllowlist()
	compositeTarget := compositeTargetPlatformMiddleware(compositeResolver)
	requireGroup := middleware.RequireGroupAssignment(settingService, middleware.OpenAIErrorWriter)
	billableUsageResponse := middleware.BillableUsageResponse()

	// 与网关 /v1/chat/completions 相同的平台分发(openai/grok/国产兼容走 OpenAI 网关)
	chatTarget := func(c *gin.Context) {
		switch getGroupPlatform(c) {
		case service.PlatformOpenAI, service.PlatformGrok,
			service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek, service.PlatformMiniMax:
			h.OpenAIGateway.ChatCompletions(c)
		default:
			h.Gateway.ChatCompletions(c)
		}
	}

	ai := r.Group("/api/v1/studio")
	ai.Use(gin.HandlerFunc(jwtAuth))
	ai.Use(middleware.BackendModeUserGuard(settingService))
	ai.Use(panelRateLimiter.Global())
	{
		ai.POST("/ai-prompt", func(c *gin.Context) {
			subject, ok := middleware.GetAuthSubjectFromContext(c)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "User not authenticated"})
				return
			}
			var req struct {
				Prompt string `json:"prompt" binding:"required"`
				Kind   string `json:"kind"`
				KeyID  int64  `json:"key_id"`
				Model  string `json:"model"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}

			// 仅使用用户自己创建的 API 密钥:不再提供自动选组/托管密钥模式。
			// 密钥必须归属当前用户、启用且绑定分组;模型由用户显式选择。
			if req.KeyID <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"message": "请选择用于优化提示词的 API 密钥"})
				return
			}
			key, keyErr := apiKeyService.GetByID(c.Request.Context(), req.KeyID)
			if keyErr != nil || key == nil || key.UserID != subject.UserID || key.Status != service.StatusAPIKeyActive {
				c.JSON(http.StatusBadRequest, gin.H{"message": "密钥不存在或不可用"})
				return
			}
			hasGroup := key.GroupID != nil && *key.GroupID > 0 || len(key.GroupIDs) > 0
			if !hasGroup {
				c.JSON(http.StatusBadRequest, gin.H{"message": "该密钥未绑定分组,无法路由"})
				return
			}
			chatModel := strings.TrimSpace(req.Model)
			if chatModel == "" {
				c.JSON(http.StatusBadRequest, gin.H{"message": "请选择用于优化提示词的模型"})
				return
			}
			authKeyValue := key.Key

			payload, err := json.Marshal(map[string]any{
				"model": chatModel,
				"messages": []gin.H{
					{"role": "system", "content": handler.AIPromptSystemMessage(req.Kind)},
					{"role": "user", "content": req.Prompt},
				},
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			// 把面板请求改写为 /v1/chat/completions 载荷,伪装成网关端点后按网关顺序执行中间件链
			c.Request.Body = io.NopCloser(bytes.NewReader(payload))
			c.Request.ContentLength = int64(len(payload))
			origPath := c.Request.URL.Path
			c.Request.URL.Path = "/v1/chat/completions"
			defer func() { c.Request.URL.Path = origPath }()
			c.Request.Header.Set("Authorization", "Bearer "+authKeyValue)

			bodyLimit(c)
			clientRequestID(c)
			opsErrorLogger(c)
			endpointNorm(c)
			apiKeyAuth(c)
			if c.IsAborted() {
				return
			}
			billableUsageResponse(c)
			groupModelAllowlist(c)
			compositeTarget(c)
			requireGroup(c)
			if c.IsAborted() {
				return
			}
			chatTarget(c)
		})
	}
}
