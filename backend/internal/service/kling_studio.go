package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var klingVideoModelPattern = regexp.MustCompile(`^kling-(v[1-4]([.-][0-9]+)*|4\.0)$`)

func IsKlingVideoModel(model string) bool {
	return klingVideoModelPattern.MatchString(strings.ToLower(strings.TrimSpace(model)))
}

// BuildKlingStudioRequest maps the workbench payload to Kling's native API.
func BuildKlingStudioRequest(body []byte) (KlingEndpoint, []byte, string, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return "", nil, "", errors.New("request body must be a JSON object")
	}
	model := KlingRequestModel(body)
	prompt := strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	if !IsKlingVideoModel(model) || prompt == "" {
		return "", nil, "", errors.New("a Kling video model and prompt are required")
	}
	aspect := gjson.GetBytes(body, "aspect_ratio").String()
	switch aspect {
	case "16:9", "9:16", "1:1":
	default:
		aspect = "16:9"
	}
	payload := map[string]any{
		"model_name": model, "prompt": prompt, "mode": "std",
		"duration": strconv.Itoa(KlingRequestDurationSeconds(body)), "aspect_ratio": aspect,
	}
	endpoint := KlingEndpointText2Video
	if model == "kling-4.0" {
		endpoint = KlingEndpointKling4Text
	}
	image := gjson.GetBytes(body, "image")
	if image.IsObject() {
		image = image.Get("url")
	}
	if value := strings.TrimSpace(image.String()); value != "" && image.Type == gjson.String {
		payload["image"] = value
		endpoint = KlingEndpointImage2Video
		if model == "kling-4.0" {
			endpoint = KlingEndpointKling4Image
		}
	}
	if model == "kling-4.0" {
		payload = map[string]any{
			"prompt":   prompt,
			"settings": map[string]any{"duration": KlingRequestDurationSeconds(body), "aspect_ratio": aspect},
			"options":  map[string]any{},
		}
		if endpoint == KlingEndpointKling4Image {
			payload["contents"] = []map[string]string{{"type": "first_frame", "url": image.String()}}
		}
	}
	out, err := json.Marshal(payload)
	return endpoint, out, model, err
}

// The opaque workbench id carries the query family across polls and restarts;
// the native task id remains the account-binding and billing key.
func KlingStudioTaskID(endpoint KlingEndpoint, taskID string) string {
	switch endpoint {
	case KlingEndpointKling4Text, KlingEndpointKling4Image, KlingEndpointQueryKling4:
		return "kling4:" + taskID
	case KlingEndpointImage2Video, KlingEndpointQueryImage:
		return "klingi2v:" + taskID
	default:
		return taskID
	}
}

func ParseKlingStudioTaskID(id string) (KlingEndpoint, string, error) {
	endpoint := KlingEndpointQueryVideo
	if strings.HasPrefix(id, "kling4:") {
		endpoint, id = KlingEndpointQueryKling4, strings.TrimPrefix(id, "kling4:")
	} else if strings.HasPrefix(id, "klingi2v:") {
		endpoint, id = KlingEndpointQueryImage, strings.TrimPrefix(id, "klingi2v:")
	}
	if id == "" || strings.ContainsAny(id, "/\\?#&:% \t\r\n") || id == "." || id == ".." {
		return "", "", errors.New("invalid Kling task id")
	}
	return endpoint, id, nil
}

func KlingStudioResponse(endpoint KlingEndpoint, result *KlingTaskResult) gin.H {
	status, progress := "pending", 0
	switch NormalizeKlingStatus(result.Status) {
	case "processing":
		status, progress = "processing", 50
	case "succeed":
		status, progress = "completed", 100
	case "failed":
		status = "failed"
	}
	out := gin.H{"id": KlingStudioTaskID(endpoint, result.TaskID), "status": status, "progress": progress}
	if status == "failed" {
		out["error"] = gin.H{"message": firstNonEmpty(result.Message, "Kling video generation failed")}
	}
	return out
}

// Use the same public-IP-pinned transport as Grok's public content proxy. CDN
// requests carry no account credentials, client cookies, or header overrides.
var klingMediaContentClient = grokMediaPublicVideoClient

func (s *OpenAIGatewayService) ForwardKlingStudioContent(ctx context.Context, c *gin.Context, result *KlingTaskResult) error {
	if result == nil || NormalizeKlingStatus(result.Status) != "succeed" || result.URL == "" {
		return &KlingUpstreamError{StatusCode: http.StatusConflict, Message: "Kling task content is not ready"}
	}
	status, _ := json.Marshal(map[string]string{"status": "completed", "video_url": result.URL})
	contentURL, err := grokMediaPublicVideoContentURL(status, result.TaskID)
	if err != nil || contentURL == "" {
		return errors.New("Kling task returned an unsupported content URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "*/*")
	if value := c.GetHeader("Range"); value != "" {
		req.Header.Set("Range", value)
	}
	resp, err := klingMediaContentClient.Do(req)
	if err != nil {
		return fmt.Errorf("Kling content download failed: %s", sanitizeStreamError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return fmt.Errorf("Kling content download returned status %d", resp.StatusCode)
	}
	return writeGrokMediaContentResponse(c, resp)
}
