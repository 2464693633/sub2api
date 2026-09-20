package routes

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterVideoStudioRoutes 注册视频工作台的会话直通接口(/api/v1/video-studio/*)。
//
// 与 image_studio.go 同一桥接模式:JWT 鉴权后确保用户名下存在「视频工作台」密钥,
// 以该密钥身份重放网关中间件链,完整复用 /v1/videos 的 Grok 媒体转发
// (提交/状态/内容),调度计费与外部直连密钥口径一致。
func RegisterVideoStudioRoutes(
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

	// 网关侧 /v1/videos 只对 grok/composite 分组开放;状态与内容查询按
	// videoStatusHandler 的平台判定路由(grok/composite 之外的分组 404)。
	videoTarget := func(c *gin.Context) {
		if platform := getGroupPlatform(c); platform == service.PlatformGrok || platform == service.PlatformComposite {
			h.OpenAIGateway.GrokVideoGeneration(c)
			return
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Videos API is not supported for this platform",
			},
		})
	}
	videoStatusTarget := func(c *gin.Context) {
		if platform := getGroupPlatform(c); platform == service.PlatformGrok || platform == service.PlatformComposite {
			h.OpenAIGateway.GrokVideoStatus(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Videos API is not supported for this platform",
			},
		})
	}
	videoContentTarget := func(c *gin.Context) {
		if platform := getGroupPlatform(c); platform == service.PlatformGrok || platform == service.PlatformComposite {
			h.OpenAIGateway.GrokVideoContent(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Videos API is not supported for this platform",
			},
		})
	}

	// resolveStudioKey 按 X-Video-Studio-Group 头选定视频分组并确保对应工作台密钥;
	// 未带头时回落到第一个视频分组。密钥按分组绑定(网关限制单密钥同平台),
	// 因此多个视频分组同台时由前端按模型所属分组传该头。
	resolveStudioKey := func(c *gin.Context, userID int64) (*service.APIKey, error) {
		groups, err := apiKeyService.VideoStudioGroups(c.Request.Context(), userID)
		if err != nil {
			return nil, err
		}
		chosen := groups[0]
		if idStr := strings.TrimSpace(c.GetHeader("X-Video-Studio-Group")); idStr != "" {
			if gid, parseErr := strconv.ParseInt(idStr, 10, 64); parseErr == nil {
				for _, g := range groups {
					if g.ID == gid {
						chosen = g
						break
					}
				}
			}
		}
		return apiKeyService.EnsureVideoStudioKeyForGroup(c.Request.Context(), userID, chosen.ID)
	}

	relay := func(canonicalPath string, pathSuffix string, target gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			subject, ok := middleware.GetAuthSubjectFromContext(c)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "User not authenticated"})
				return
			}
			apiKey, err := resolveStudioKey(c, subject.UserID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}

			origPath := c.Request.URL.Path
			// 状态/内容查询的 request_id 在路径里,拼到规范网关路径之后再改写。
			reqID := c.Param("request_id")
			if reqID != "" {
				c.Request.URL.Path = canonicalPath + "/" + reqID + pathSuffix
			} else {
				c.Request.URL.Path = canonicalPath
			}
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

	studio := r.Group("/api/v1/video-studio")
	studio.Use(gin.HandlerFunc(jwtAuth))
	studio.Use(middleware.BackendModeUserGuard(settingService))
	studio.Use(panelRateLimiter.Global())
	{
		studio.GET("/models", relay("/v1/models", "", func(c *gin.Context) {
			h.Gateway.VideoStudioModels(c, apiKeyService)
		}))
		studio.POST("/generations", relay(handler.EndpointVideos, "", videoTarget))
		studio.GET("/tasks/:request_id", relay("/v1/videos", "", videoStatusTarget))
		studio.GET("/tasks/:request_id/content", relay("/v1/videos", "/content", videoContentTarget))
	}
}
