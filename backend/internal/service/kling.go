package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tidwall/gjson"
)

// KlingEndpoint identifies one official-compatible Kling operation.
type KlingEndpoint string

const (
	KlingEndpointText2Video           KlingEndpoint = "text2video"
	KlingEndpointImage2Video          KlingEndpoint = "image2video"
	KlingEndpointImagesGeneration     KlingEndpoint = "images_generation"
	KlingEndpointKling4Text           KlingEndpoint = "kling4_text"
	KlingEndpointKling4Image          KlingEndpoint = "kling4_image"
	KlingEndpointKling4Omni           KlingEndpoint = "kling4_omni"
	KlingEndpointQueryVideo           KlingEndpoint = "query_video"
	KlingEndpointQueryImage           KlingEndpoint = "query_image"
	KlingEndpointQueryImageGeneration KlingEndpoint = "query_image_generation"
	KlingEndpointQueryKling4          KlingEndpoint = "query_kling4"
)

const KlingEndpointImagesGenerations = KlingEndpointImagesGeneration

// DefaultKlingModelIDs is the stable model catalog exposed to admin model
// selectors when kling-multi does not provide a models endpoint.
func DefaultKlingModelIDs() []string {
	// Matches what kling-multi actually accepts: legacy 1.x-2.1 versions were
	// dropped by the web upstream and kling-v2-5 was never a valid model_name.
	return []string{"kling-v4", "kling-v4-flash", "kling-v3-omni", "kling-v3", "kling-v2-6", "kolors"}
}

// NormalizeKlingStatus keeps the upstream distinction between traditional
// succeed and Kling 4.0 succeeded while exposing a common internal state.
func NormalizeKlingStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "submitted":
		return "submitted"
	case "processing", "running":
		return "processing"
	case "succeed", "succeeded", "completed", "success":
		return "succeed"
	case "failed", "failure", "error":
		return "failed"
	default:
		return strings.TrimSpace(status)
	}
}

// KlingTaskResult is the normalized subset used for scheduling and billing.
type KlingTaskResult struct {
	TaskID          string
	Status          string
	Message         string
	URL             string
	ImageCount      int
	VideoCount      int
	DurationSeconds int
}

// KlingUpstreamError preserves the official code/message while exposing the
// HTTP status expected by OpenAI-compatible clients.
type KlingUpstreamError struct {
	Code       int
	Message    string
	StatusCode int
	Body       []byte
}

func (e *KlingUpstreamError) Error() string {
	if e == nil {
		return "kling upstream error"
	}
	return fmt.Sprintf("kling upstream code %d: %s", e.Code, e.Message)
}

// KlingErrorHTTPStatus maps the official error table to an HTTP response.
func KlingErrorHTTPStatus(code int) int {
	switch code {
	case 1000, 1001, 1002, 1004:
		return http.StatusUnauthorized
	case 1201:
		return http.StatusBadRequest
	case 1203:
		return http.StatusNotFound
	case 1102:
		return http.StatusPaymentRequired
	case 1302:
		return http.StatusTooManyRequests
	case 5000, 5001, 5002:
		return http.StatusBadGateway
	default:
		if code >= 5000 {
			return http.StatusBadGateway
		}
		return http.StatusBadRequest
	}
}

// BuildKlingJWT creates the per-request HS256 token required by kling-multi.
func BuildKlingJWT(accessKey, secretKey string, now time.Time) (string, error) {
	accessKey, secretKey = strings.TrimSpace(accessKey), strings.TrimSpace(secretKey)
	if accessKey == "" || secretKey == "" {
		return "", errors.New("kling access key and secret key are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	claims := jwt.MapClaims{"iss": accessKey, "exp": now.Add(30 * time.Minute).Unix(), "nbf": now.Add(-5 * time.Second).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secretKey))
}

// ParseKlingTaskResult parses both traditional data.task_status responses and
// the Kling 4.0 data[]/outputs[] response shape.
func ParseKlingTaskResult(body []byte, endpoint KlingEndpoint) KlingTaskResult {
	var out KlingTaskResult
	data := gjson.GetBytes(body, "data")
	if endpoint == KlingEndpointQueryKling4 || data.IsArray() {
		if data.IsArray() {
			data = data.Get("0")
		}
		out.TaskID = strings.TrimSpace(data.Get("id").String())
		out.Status = NormalizeKlingStatus(data.Get("status").String())
		out.Message = strings.TrimSpace(data.Get("message").String())
		if output := data.Get("outputs.0"); output.Exists() {
			out.URL = strings.TrimSpace(output.Get("url").String())
			if strings.EqualFold(output.Get("type").String(), "video") {
				out.VideoCount = 1
				out.DurationSeconds = int(output.Get("duration").Int())
			} else if out.URL != "" {
				out.ImageCount = 1
			}
		}
		return out
	}
	out.TaskID = strings.TrimSpace(data.Get("task_id").String())
	out.Status = NormalizeKlingStatus(data.Get("task_status").String())
	out.Message = strings.TrimSpace(data.Get("task_status_msg").String())
	if out.Status == "" {
		out.Status = strings.TrimSpace(data.Get("status").String())
	}
	if video := data.Get("task_result.videos.0.url"); video.Exists() {
		out.URL, out.VideoCount = strings.TrimSpace(video.String()), 1
		out.DurationSeconds = int(data.Get("task_result.videos.0.duration").Int())
	}
	if image := data.Get("task_result.images.0.url"); image.Exists() {
		out.URL, out.ImageCount = strings.TrimSpace(image.String()), 1
	}
	return out
}

func klingPath(endpoint KlingEndpoint, taskID string) string {
	taskID = strings.TrimSpace(taskID)
	switch endpoint {
	case KlingEndpointText2Video, KlingEndpointQueryVideo:
		if endpoint == KlingEndpointQueryVideo {
			return "/v1/videos/text2video/" + taskID
		}
		return "/v1/videos/text2video"
	case KlingEndpointImage2Video, KlingEndpointQueryImage:
		if endpoint == KlingEndpointQueryImage {
			return "/v1/videos/image2video/" + taskID
		}
		return "/v1/videos/image2video"
	case KlingEndpointImagesGeneration:
		return "/v1/images/generations"
	case KlingEndpointQueryImageGeneration:
		return "/v1/images/generations/" + taskID
	case KlingEndpointKling4Text:
		return "/text-to-video/kling-4.0"
	case KlingEndpointKling4Image:
		return "/image-to-video/kling-4.0"
	case KlingEndpointKling4Omni:
		return "/omni-video/kling-4.0"
	case KlingEndpointQueryKling4:
		return "/tasks?task_ids=" + taskID
	default:
		return ""
	}
}

func klingIsQuery(endpoint KlingEndpoint) bool {
	return endpoint == KlingEndpointQueryVideo || endpoint == KlingEndpointQueryImage || endpoint == KlingEndpointQueryImageGeneration || endpoint == KlingEndpointQueryKling4
}

// ForwardKling forwards one request and writes the compatible upstream body to
// the client. It returns normalized task data for the caller's billing logic.
func (s *OpenAIGatewayService) ForwardKling(ctx context.Context, c *gin.Context, account *Account, endpoint KlingEndpoint, taskID string, body []byte) (*KlingTaskResult, error) {
	if account == nil || !account.IsKling() {
		return nil, errors.New("kling account is required")
	}
	accessKey, secretKey := account.GetKlingAccessSecret()
	token, err := BuildKlingJWT(accessKey, secretKey, time.Now())
	if err != nil {
		return nil, err
	}
	baseURL := account.GetKlingBaseURL()
	if s != nil && s.cfg != nil {
		validated, validateErr := s.validateUpstreamBaseURL(baseURL)
		if validateErr != nil {
			return nil, validateErr
		}
		baseURL = strings.TrimRight(validated, "/")
	}
	path := klingPath(endpoint, taskID)
	if path == "" {
		return nil, errors.New("unsupported kling endpoint")
	}
	method := http.MethodPost
	if klingIsQuery(endpoint) {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	account.ApplyHeaderOverrides(req.Header)
	var resp *http.Response
	if s != nil && s.httpUpstream != nil {
		proxyURL := ""
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
		resp, err = s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	} else {
		resp, err = http.DefaultClient.Do(req)
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if c != nil {
		for key, values := range resp.Header {
			for _, value := range values {
				c.Header(key, value)
			}
		}
	}
	if resp.StatusCode >= 400 {
		code := int(gjson.GetBytes(respBody, "code").Int())
		message := strings.TrimSpace(gjson.GetBytes(respBody, "message").String())
		if message == "" {
			message = resp.Status
		}
		if code == 0 {
			code = resp.StatusCode
		}
		return nil, &KlingUpstreamError{Code: code, Message: message, StatusCode: KlingErrorHTTPStatus(code), Body: respBody}
	}
	if code := int(gjson.GetBytes(respBody, "code").Int()); code != 0 {
		message := strings.TrimSpace(gjson.GetBytes(respBody, "message").String())
		return nil, &KlingUpstreamError{Code: code, Message: message, StatusCode: KlingErrorHTTPStatus(code), Body: respBody}
	}
	if !klingIsQuery(endpoint) && strings.TrimSpace(gjson.GetBytes(respBody, "data.task_id").String()) == "" && strings.TrimSpace(gjson.GetBytes(respBody, "data.id").String()) == "" {
		return nil, &KlingUpstreamError{Code: 1201, Message: "Kling response missing task id", StatusCode: http.StatusBadGateway, Body: respBody}
	}
	if c != nil {
		c.Data(resp.StatusCode, "application/json; charset=utf-8", respBody)
	}
	result := ParseKlingTaskResult(respBody, endpoint)
	if !klingIsQuery(endpoint) {
		if result.TaskID == "" {
			result.TaskID = strings.TrimSpace(gjson.GetBytes(respBody, "data.id").String())
		}
	}
	return &result, nil
}

// KlingRequestModel extracts model_name (and accepts OpenAI's model alias).
func KlingRequestModel(body []byte) string {
	model := strings.TrimSpace(gjson.GetBytes(body, "model_name").String())
	if model == "" {
		model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	return model
}

// KlingRequestDurationSeconds returns the requested video duration in seconds.
// The compatibility layer currently accepts the official 5/10 second values.
func KlingRequestDurationSeconds(body []byte) int {
	for _, path := range []string{"duration", "settings.duration"} {
		value := strings.TrimSpace(gjson.GetBytes(body, path).String())
		if value == "" {
			continue
		}
		if value == "10" {
			return 10
		}
		if value == "5" {
			return 5
		}
	}
	return 5
}

// KlingRequestResolution returns the requested video resolution tier
// ("480p"/"720p"/"1080p"/"4k"), or "" when the request leaves it to defaults.
// Unknown values are reported as "" so runtime billing falls back to the
// documented default tier instead of guessing.
func KlingRequestResolution(body []byte) string {
	for _, path := range []string{"resolution", "settings.resolution"} {
		value := strings.TrimSpace(gjson.GetBytes(body, path).String())
		if value == "" {
			continue
		}
		if tier, ok := LookupVideoBillingResolution(value); ok {
			return tier
		}
	}
	return ""
}

// KlingRequestMode reports the traditional std/pro quality switch; the Kling
// 4.0 protocol has no mode field and bills as std.
func KlingRequestMode(body []byte) string {
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, "mode").String()), "pro") {
		return "pro"
	}
	return "std"
}

// KlingBillingModel folds the pro quality switch into the billing model id so
// group VideoModelPrices can price std and pro separately (pro costs ~1.5x on
// Kling). Flash/omni variants are already distinct model ids.
func KlingBillingModel(model string, body []byte) string {
	model = strings.TrimSpace(model)
	if body != nil && KlingRequestMode(body) == "pro" {
		return model + "-pro"
	}
	return model
}

// KlingRequestJSON validates that a request body is JSON and non-empty.
func KlingRequestJSON(body []byte) bool {
	var v any
	return len(bytes.TrimSpace(body)) > 0 && json.Unmarshal(body, &v) == nil
}
