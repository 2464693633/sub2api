package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// KlingModels exposes the stable model catalog accepted by kling-multi.
func (h *OpenAIGatewayHandler) KlingModels(c *gin.Context) {
	models := service.DefaultKlingModelIDs()
	data := make([]gin.H, 0, len(models))
	for _, model := range models {
		data = append(data, gin.H{"id": model, "object": "model", "owned_by": "kling"})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// KlingSubmit forwards a traditional or Kling 4.0 generation request.
func (h *OpenAIGatewayHandler) KlingSubmit(c *gin.Context, endpoint service.KlingEndpoint) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || !service.KlingRequestJSON(body) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "request body must be valid JSON"}})
		return
	}
	model := service.KlingRequestModel(body)
	if model == "" {
		switch endpoint {
		case service.KlingEndpointImagesGeneration:
			model = "kling-v3"
		case service.KlingEndpointText2Video, service.KlingEndpointImage2Video:
			model = "kling-v1"
		default:
			model = "kling-4.0"
		}
	}
	h.handleKling(c, endpoint, "", body, model)
}

// KlingQuery retrieves a traditional or Kling 4.0 task.
func (h *OpenAIGatewayHandler) KlingQuery(c *gin.Context, endpoint service.KlingEndpoint) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		taskID = strings.TrimSpace(strings.Split(c.Query("task_ids"), ",")[0])
	}
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "task_id is required"}})
		return
	}
	h.handleKling(c, endpoint, taskID, nil, "")
}

func (h *OpenAIGatewayHandler) handleKling(c *gin.Context, endpoint service.KlingEndpoint, taskID string, body []byte, model string) {
	h.handleKlingResponse(c, endpoint, taskID, body, model, "")
}

func (h *OpenAIGatewayHandler) handleKlingResponse(c *gin.Context, endpoint service.KlingEndpoint, taskID string, body []byte, model, responseMode string) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": "authentication_error", "message": "Invalid API key"}})
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": "User context not found"}})
		return
	}
	var err error
	if apiKey.Group != nil && !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	var inflightDone func()
	if !strings.HasPrefix(string(endpoint), "query") {
		subscription, _ := middleware.GetSubscriptionFromContext(c)
		kind := service.InflightEstimateVideo
		units := 1
		if endpoint == service.KlingEndpointImagesGeneration {
			kind = service.InflightEstimateImage
			units = 1
		}
		inflightDone, err = reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, service.InflightEstimateRequest{
			Model: service.KlingBillingModel(model, body), BodyBytes: len(body), Kind: kind, Units: units,
			VideoResolution:      service.KlingRequestResolution(body),
			VideoDurationSeconds: service.KlingRequestDurationSeconds(body),
		})
		if err != nil {
			status, code, message, retryAfter := billingErrorDetails(err)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.errorResponse(c, status, code, message)
			return
		}
		defer inflightDone()
	}
	ctx := c.Request.Context()
	selection, err := h.selectKlingAccount(ctx, apiKey, subject.UserID, taskID, model)
	if err != nil || selection == nil || selection.Account == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "kling_no_eligible_account", "message": "No eligible Kling accounts"}})
		return
	}
	if selection.ReleaseFunc != nil {
		defer selection.ReleaseFunc()
	}
	account := selection.Account
	responseContext := c
	if responseMode != "" {
		responseContext = nil
	}
	result, forwardErr := h.gatewayService.ForwardKling(ctx, responseContext, account, endpoint, taskID, body)
	if forwardErr == nil && responseMode == "content" {
		forwardErr = h.gatewayService.ForwardKlingStudioContent(ctx, c, result)
	}
	if forwardErr != nil {
		if c.Writer.Written() {
			return
		}
		var klingErr *service.KlingUpstreamError
		if errors.As(forwardErr, &klingErr) {
			c.JSON(klingErr.StatusCode, gin.H{"error": gin.H{"type": "kling_upstream_error", "code": klingErr.Code, "message": klingErr.Message}})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "upstream_error", "message": forwardErr.Error()}})
		return
	}
	if result == nil {
		return
	}
	if result.TaskID != "" && !strings.HasPrefix(string(endpoint), "query") {
		_ = h.gatewayService.BindGrokMediaVideoRequestAccount(ctx, apiKey.GroupID, result.TaskID, subject.UserID, apiKey.ID, account.ID)
	}
	// Kling charges an accepted generation request once. The submit response only
	// contains a task id, so stamp the billable unit from the endpoint here.
	if !strings.HasPrefix(string(endpoint), "query") {
		switch endpoint {
		case service.KlingEndpointImagesGeneration:
			result.ImageCount = 1
		default:
			result.VideoCount = 1
		}
	}
	if !strings.HasPrefix(string(endpoint), "query") && (result.ImageCount > 0 || result.VideoCount > 0) {
		if result.DurationSeconds <= 0 {
			result.DurationSeconds = service.KlingRequestDurationSeconds(body)
		}
		h.recordKlingUsage(c, apiKey, subject, account, model, taskID, body, result)
	}
	if responseMode == "studio" {
		if result.TaskID == "" {
			result.TaskID = taskID
		}
		c.JSON(http.StatusOK, service.KlingStudioResponse(endpoint, result))
	}
}

func (h *OpenAIGatewayHandler) selectKlingAccount(ctx context.Context, apiKey *service.APIKey, userID int64, taskID, model string) (*service.AccountSelectionResult, error) {
	if taskID != "" {
		accountID, err := h.gatewayService.ResolveGrokMediaVideoRequestAccount(ctx, apiKey.GroupID, taskID, userID, apiKey.ID)
		if err == nil && accountID > 0 {
			hash := service.GrokMediaVideoRequestSessionHashV2(taskID, userID)
			selection, _, selectErr := h.gatewayService.SelectMediaVideoRequestAccount(ctx, apiKey.GroupID, hash, accountID, model, service.PlatformKling)
			return selection, selectErr
		}
	}
	hash := service.GrokMediaVideoRequestSessionHashV2(taskID, userID)
	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		ctx, apiKey.GroupID, "", hash, model, nil,
		service.OpenAIUpstreamTransportHTTPSSE, service.OpenAIEndpointCapabilityGrokMediaGeneration,
		false, false, false, service.PlatformKling,
	)
	return selection, err
}

func (h *OpenAIGatewayHandler) recordKlingUsage(c *gin.Context, apiKey *service.APIKey, subject middleware.AuthSubject, account *service.Account, model, taskID string, body []byte, result *service.KlingTaskResult) {
	if h == nil || h.gatewayService == nil || apiKey == nil || apiKey.User == nil || result == nil {
		return
	}
	duration := result.DurationSeconds
	if duration <= 0 {
		duration = 5
	}
	forward := &service.OpenAIForwardResult{ResponseID: result.TaskID, RequestID: result.TaskID, Model: model, BillingModel: service.KlingBillingModel(model, body), ImageCount: result.ImageCount, VideoCount: result.VideoCount, VideoResolution: service.KlingRequestResolution(body), VideoDurationSeconds: duration, ImageEndpoint: result.ImageCount > 0}
	if taskID != "" {
		forward.RequestID = taskID
	}
	h.submitOpenAIUsageRecordTask(c.Request.Context(), forward, func(ctx context.Context) {
		_ = h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result: forward, APIKey: apiKey, User: apiKey.User, Account: account,
			InboundEndpoint: GetInboundEndpoint(c), UpstreamEndpoint: GetUpstreamEndpoint(c, account.Platform),
			UserAgent: c.GetHeader("User-Agent"), IPAddress: c.ClientIP(),
			RequestPayloadHash: service.HashUsageRequestPayload([]byte(firstNonEmptyString(model, taskID))),
			APIKeyService:      h.apiKeyService, QuotaPlatform: service.QuotaPlatform(ctx, apiKey),
		})
	})
}
