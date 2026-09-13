package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogFromService_IncludesOpenAIWSMode(t *testing.T) {
	t.Parallel()

	wsLog := &service.UsageLog{
		RequestID:    "req_1",
		Model:        "gpt-5.3-codex",
		OpenAIWSMode: true,
	}
	httpLog := &service.UsageLog{
		RequestID:    "resp_1",
		Model:        "gpt-5.3-codex",
		OpenAIWSMode: false,
	}

	require.True(t, UsageLogFromService(wsLog).OpenAIWSMode)
	require.False(t, UsageLogFromService(httpLog).OpenAIWSMode)
	require.True(t, UsageLogFromServiceAdmin(wsLog).OpenAIWSMode)
	require.False(t, UsageLogFromServiceAdmin(httpLog).OpenAIWSMode)
}

func TestUsageLogFromService_PreservesNativeCompactionAndStream(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		RequestID:          "resp_compaction",
		Model:              "gpt-5.6-sol",
		RequestType:        service.RequestTypeStream,
		Stream:             true,
		NativeCompactionV2: true,
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)
	require.Equal(t, "stream", userDTO.RequestType)
	require.True(t, userDTO.Stream)
	require.True(t, userDTO.NativeCompactionV2)
	require.Equal(t, "stream", adminDTO.RequestType)
	require.True(t, adminDTO.Stream)
	require.True(t, adminDTO.NativeCompactionV2)
}

func TestUsageLogFromService_PrefersRequestTypeForLegacyFields(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		RequestID:    "req_2",
		Model:        "gpt-5.3-codex",
		RequestType:  service.RequestTypeWSV2,
		Stream:       false,
		OpenAIWSMode: false,
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.Equal(t, "ws_v2", userDTO.RequestType)
	require.True(t, userDTO.Stream)
	require.True(t, userDTO.OpenAIWSMode)
	require.Equal(t, "ws_v2", adminDTO.RequestType)
	require.True(t, adminDTO.Stream)
	require.True(t, adminDTO.OpenAIWSMode)
}

func TestUsageCleanupTaskFromService_RequestTypeMapping(t *testing.T) {
	t.Parallel()

	requestType := int16(service.RequestTypeStream)
	task := &service.UsageCleanupTask{
		ID:     1,
		Status: service.UsageCleanupStatusPending,
		Filters: service.UsageCleanupFilters{
			RequestType: &requestType,
		},
	}

	dtoTask := UsageCleanupTaskFromService(task)
	require.NotNil(t, dtoTask)
	require.NotNil(t, dtoTask.Filters.RequestType)
	require.Equal(t, "stream", *dtoTask.Filters.RequestType)
}

func TestRequestTypeStringPtrNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, requestTypeStringPtr(nil))
}

func TestUsageLogFromService_IncludesServiceTierForUserAndAdmin(t *testing.T) {
	t.Parallel()

	serviceTier := "priority"
	inboundEndpoint := "/v1/chat/completions"
	upstreamEndpoint := "/v1/responses"
	log := &service.UsageLog{
		RequestID:             "req_3",
		Model:                 "gpt-5.4",
		ServiceTier:           &serviceTier,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		AccountRateMultiplier: f64Ptr(1.5),
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.NotNil(t, userDTO.ServiceTier)
	require.Equal(t, serviceTier, *userDTO.ServiceTier)
	require.NotNil(t, userDTO.InboundEndpoint)
	require.Equal(t, inboundEndpoint, *userDTO.InboundEndpoint)
	require.Nil(t, userDTO.UpstreamEndpoint)
	require.NotNil(t, adminDTO.ServiceTier)
	require.Equal(t, serviceTier, *adminDTO.ServiceTier)
	require.NotNil(t, adminDTO.InboundEndpoint)
	require.Equal(t, inboundEndpoint, *adminDTO.InboundEndpoint)
	require.NotNil(t, adminDTO.UpstreamEndpoint)
	require.Equal(t, upstreamEndpoint, *adminDTO.UpstreamEndpoint)
	require.NotNil(t, adminDTO.AccountRateMultiplier)
	require.InDelta(t, 1.5, *adminDTO.AccountRateMultiplier, 1e-12)
}

func TestUsageLogFromService_UsesRequestedModelAndKeepsUpstreamAdminOnly(t *testing.T) {
	t.Parallel()

	upstreamModel := "claude-sonnet-4-20250514"
	upstreamResponseModel := "claude-sonnet-4-20250513"
	upstreamModelMismatch := true
	log := &service.UsageLog{
		RequestID:             "req_4",
		Model:                 upstreamModel,
		RequestedModel:        "claude-sonnet-4",
		UpstreamModel:         &upstreamModel,
		UpstreamResponseModel: &upstreamResponseModel,
		UpstreamModelMismatch: &upstreamModelMismatch,
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.Equal(t, "claude-sonnet-4", userDTO.Model)
	require.Equal(t, "claude-sonnet-4", adminDTO.Model)

	userJSON, err := json.Marshal(userDTO)
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "upstream_model")
	require.NotContains(t, string(userJSON), "upstream_response_model")
	require.NotContains(t, string(userJSON), "upstream_model_mismatch")

	adminJSON, err := json.Marshal(adminDTO)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"upstream_model":"claude-sonnet-4-20250514"`)
	require.Contains(t, string(adminJSON), `"upstream_response_model":"claude-sonnet-4-20250513"`)
	require.Contains(t, string(adminJSON), `"upstream_model_mismatch":true`)
}

func TestUsageLogFromService_KeepsUserBillingAndIPWithoutAdminCostFields(t *testing.T) {
	t.Parallel()

	ipAddress := "203.0.113.10"
	accountRateMultiplier := 1.5
	accountStatsCost := 0.21
	log := &service.UsageLog{
		RequestID:             "req_user_visible_billing",
		Model:                 "gpt-5.4",
		InputCost:             0.01,
		OutputCost:            0.02,
		CacheCreationCost:     0.03,
		CacheReadCost:         0.04,
		TotalCost:             0.10,
		ActualCost:            0.08,
		RateMultiplier:        0.8,
		IPAddress:             &ipAddress,
		AccountRateMultiplier: &accountRateMultiplier,
		AccountStatsCost:      &accountStatsCost,
	}

	userDTO := UsageLogFromService(log)
	require.Equal(t, 0.01, userDTO.InputCost)
	require.Equal(t, 0.02, userDTO.OutputCost)
	require.Equal(t, 0.03, userDTO.CacheCreationCost)
	require.Equal(t, 0.04, userDTO.CacheReadCost)
	// total_cost 与实扣同口径(用户侧不暴露税前成本)
	require.Equal(t, 0.08, userDTO.TotalCost)
	require.Equal(t, 0.08, userDTO.ActualCost)
	require.Equal(t, 0.8, userDTO.RateMultiplier)
	require.NotNil(t, userDTO.IPAddress)
	require.Equal(t, ipAddress, *userDTO.IPAddress)

	userJSON, err := json.Marshal(userDTO)
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "account_rate_multiplier")
	require.NotContains(t, string(userJSON), "account_stats_cost")
	require.NotContains(t, string(userJSON), "account_cost")
}

func TestUsageLogFromService_UsersSeeRequestedReasoningEffortOnly(t *testing.T) {
	t.Parallel()

	requested := "max"
	forwarded := "xhigh"
	log := &service.UsageLog{
		RequestID:                "req_effort",
		Model:                    "gpt-5.4",
		ReasoningEffort:          &forwarded,
		RequestedReasoningEffort: &requested,
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.NotNil(t, userDTO.ReasoningEffort)
	require.Equal(t, requested, *userDTO.ReasoningEffort)
	require.NotNil(t, adminDTO.ReasoningEffort)
	require.Equal(t, requested, *adminDTO.ReasoningEffort)
	require.NotNil(t, adminDTO.UpstreamReasoningEffort)
	require.Equal(t, forwarded, *adminDTO.UpstreamReasoningEffort)

	userJSON, err := json.Marshal(userDTO)
	require.NoError(t, err)
	require.Contains(t, string(userJSON), `"reasoning_effort":"max"`)
	require.NotContains(t, string(userJSON), "upstream_reasoning_effort")
	require.NotContains(t, string(userJSON), "requested_reasoning_effort")

	adminJSON, err := json.Marshal(adminDTO)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"reasoning_effort":"max"`)
	require.Contains(t, string(adminJSON), `"upstream_reasoning_effort":"xhigh"`)
}

func TestUsageLogFromService_OmitsUpstreamReasoningEffortWhenUnmapped(t *testing.T) {
	t.Parallel()

	effort := "high"
	log := &service.UsageLog{
		RequestID:                "req_effort_same",
		Model:                    "gpt-5.4",
		ReasoningEffort:          &effort,
		RequestedReasoningEffort: &effort,
	}

	adminDTO := UsageLogFromServiceAdmin(log)
	require.NotNil(t, adminDTO.ReasoningEffort)
	require.Equal(t, effort, *adminDTO.ReasoningEffort)
	require.Nil(t, adminDTO.UpstreamReasoningEffort)

	adminJSON, err := json.Marshal(adminDTO)
	require.NoError(t, err)
	require.NotContains(t, string(adminJSON), "upstream_reasoning_effort")
}

func TestUsageLogFromService_FallsBackToLegacyModelWhenRequestedModelMissing(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		RequestID: "req_legacy",
		Model:     "claude-3",
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.Equal(t, "claude-3", userDTO.Model)
	require.Equal(t, "claude-3", adminDTO.Model)
}

func TestUsageLogFromService_IncludesImageBillingMetadataForUserAndAdmin(t *testing.T) {
	t.Parallel()

	imageSize := "4K"
	inputSize := "1024x1024"
	outputSize := "3840x2160"
	source := "output"
	log := &service.UsageLog{
		RequestID:          "req_image_metadata",
		Model:              "gpt-image-2",
		ImageCount:         2,
		ImageSize:          &imageSize,
		ImageInputSize:     &inputSize,
		ImageOutputSize:    &outputSize,
		ImageSizeSource:    &source,
		ImageSizeBreakdown: map[string]int{"4K": 2},
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	for _, got := range []*UsageLog{userDTO, &adminDTO.UsageLog} {
		require.Equal(t, 2, got.ImageCount)
		require.NotNil(t, got.ImageSize)
		require.Equal(t, imageSize, *got.ImageSize)
		require.NotNil(t, got.ImageInputSize)
		require.Equal(t, inputSize, *got.ImageInputSize)
		require.NotNil(t, got.ImageOutputSize)
		require.Equal(t, outputSize, *got.ImageOutputSize)
		require.NotNil(t, got.ImageSizeSource)
		require.Equal(t, source, *got.ImageSizeSource)
		require.Equal(t, map[string]int{"4K": 2}, got.ImageSizeBreakdown)
	}
}

func TestUsageLogFromService_PreservesHistoricalMissingImageSize(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		RequestID:  "req_legacy_image_missing_size",
		Model:      "gpt-image-2",
		ImageCount: 1,
		ImageSize:  nil,
	}

	dto := UsageLogFromService(log)
	require.Equal(t, 1, dto.ImageCount)
	require.Nil(t, dto.ImageSize)
	require.Nil(t, dto.ImageInputSize)
	require.Nil(t, dto.ImageOutputSize)
	require.Nil(t, dto.ImageSizeSource)
	require.Nil(t, dto.ImageSizeBreakdown)

	body, err := json.Marshal(dto)
	require.NoError(t, err)
	require.Contains(t, string(body), `"image_size":null`)
	require.NotContains(t, string(body), `"image_size":"2K"`)
}

func TestUsageLogFromService_UsesBillableTokenSnapshots(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		InputTokens:                  10,
		OutputTokens:                 6,
		CacheCreationTokens:          5,
		CacheReadTokens:              4,
		CacheCreation5mTokens:        2,
		CacheCreation1hTokens:        3,
		ImageInputTokens:             4,
		ImageOutputTokens:            1,
		BillableInputTokens:          intPtr(20),
		BillableOutputTokens:         intPtr(3),
		BillableCacheCreationTokens:  intPtr(13),
		BillableCacheReadTokens:      intPtr(0),
		InputTokenMultiplier:         f64Ptr(2),
		OutputTokenMultiplier:        f64Ptr(0.5),
		CacheCreationTokenMultiplier: f64Ptr(2.5),
		CacheReadTokenMultiplier:     f64Ptr(0),
	}

	userDTO := UsageLogFromService(log)
	require.Equal(t, 20, userDTO.InputTokens)
	require.Equal(t, 3, userDTO.OutputTokens)
	require.Equal(t, 13, userDTO.CacheCreationTokens)
	require.Zero(t, userDTO.CacheReadTokens)
	require.Equal(t, 5, userDTO.CacheCreation5mTokens)
	require.Equal(t, 8, userDTO.CacheCreation1hTokens)
	require.Equal(t, 8, userDTO.ImageInputTokens)
	require.Equal(t, 1, userDTO.ImageOutputTokens)

	userJSON, err := json.Marshal(userDTO)
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "raw_input_tokens")
	require.NotContains(t, string(userJSON), "input_token_multiplier")

	adminDTO := UsageLogFromServiceAdmin(log)
	require.Equal(t, userDTO.InputTokens, adminDTO.InputTokens)
	require.Equal(t, 10, adminDTO.RawInputTokens)
	require.Equal(t, 6, adminDTO.RawOutputTokens)
	require.Equal(t, 5, adminDTO.RawCacheCreationTokens)
	require.Equal(t, 4, adminDTO.RawCacheReadTokens)
	require.Equal(t, 4, adminDTO.RawImageInputTokens)
	require.Equal(t, 1, adminDTO.RawImageOutputTokens)
	require.Equal(t, 2.0, *adminDTO.InputTokenMultiplier)
	require.Equal(t, 0.0, *adminDTO.CacheReadTokenMultiplier)

	adminJSON, err := json.Marshal(adminDTO)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"raw_input_tokens":10`)
	require.Contains(t, string(adminJSON), `"input_token_multiplier":2`)
	require.Contains(t, string(adminJSON), `"cache_read_token_multiplier":0`)
}

func TestUsageLogFromService_HistoricalTokensFallBackToRaw(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		InputTokens:           11,
		OutputTokens:          12,
		CacheCreationTokens:   13,
		CacheReadTokens:       14,
		CacheCreation5mTokens: 5,
		CacheCreation1hTokens: 8,
		ImageInputTokens:      3,
		ImageOutputTokens:     4,
	}

	userDTO := UsageLogFromService(log)
	require.Equal(t, 11, userDTO.InputTokens)
	require.Equal(t, 12, userDTO.OutputTokens)
	require.Equal(t, 13, userDTO.CacheCreationTokens)
	require.Equal(t, 14, userDTO.CacheReadTokens)
	require.Equal(t, 5, userDTO.CacheCreation5mTokens)
	require.Equal(t, 8, userDTO.CacheCreation1hTokens)
	require.Equal(t, 3, userDTO.ImageInputTokens)
	require.Equal(t, 4, userDTO.ImageOutputTokens)

	adminDTO := UsageLogFromServiceAdmin(log)
	require.Nil(t, adminDTO.InputTokenMultiplier)
	require.Nil(t, adminDTO.OutputTokenMultiplier)
	require.Nil(t, adminDTO.CacheCreationTokenMultiplier)
	require.Nil(t, adminDTO.CacheReadTokenMultiplier)
}

func f64Ptr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}
