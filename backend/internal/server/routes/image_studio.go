package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterImageStudioRoutes 注册生图工作台的会话直通接口(/api/v1/image-studio/*)。
//
// 用户面板使用 JWT 登录态,而 /v1/images 网关只认 API Key。本路由在 JWT 鉴权后
// 确保用户名下存在一把「生图工作台」专用密钥,再以该密钥身份重放网关中间件链
// (限流 → 鉴权计费 → 分组模型白名单 → 合成路由 → 分组校验),完整复用 /v1/images
// 的调度、计费与转发逻辑,用户全程无需接触密钥。
func RegisterImageStudioRoutes(
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

	imagesTarget := func(c *gin.Context) {
		switch getGroupPlatform(c) {
		case service.PlatformOpenAI:
			h.OpenAIGateway.Images(c)
		case service.PlatformGrok:
			h.OpenAIGateway.GrokImages(c)
		default:
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"type":    "not_found_error",
					"message": "Images API is not supported for this platform",
				},
			})
		}
	}

	// relay 把面板请求伪装成对应网关端点后按网关顺序执行中间件链。
	// canonicalPath 同时用于 handler 内部的 endpoint 归一化(使用记录归类),
	// 请求结束后恢复原始路径,避免影响 gin 的路由匹配上下文。
	relay := func(canonicalPath string, target gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			subject, ok := middleware.GetAuthSubjectFromContext(c)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "User not authenticated"})
				return
			}
			apiKey, err := apiKeyService.EnsureImageStudioKey(c.Request.Context(), subject.UserID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}

			origPath := c.Request.URL.Path
			c.Request.URL.Path = canonicalPath
			defer func() { c.Request.URL.Path = origPath }()
			c.Request.Header.Set("Authorization", "Bearer "+apiKey.Key)

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
			target(c)
		}
	}

	studio := r.Group("/api/v1/image-studio")
	studio.Use(gin.HandlerFunc(jwtAuth))
	studio.Use(middleware.BackendModeUserGuard(settingService))
	// 面板级限流:防止单会话高频刷 models/EnsureImageStudioKey 的 DB 查询。
	// 按密钥的配额与限流由下方重放的网关中间件链兜底。
	studio.Use(panelRateLimiter.Global())
	{
		studio.GET("/models", relay("/v1/models", h.Gateway.Models))
		studio.POST("/generations", relay(handler.EndpointImagesGenerations, imagesTarget))
		studio.POST("/edits", relay(handler.EndpointImagesEdits, imagesTarget))
	}
}
