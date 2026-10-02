package routes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/creativestudio"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type creativeStudioExecutor struct {
	gateway http.Handler
	apiKeys *service.APIKeyService
	renders chan struct{}
}
type creativeGenerationParams struct {
	Model      string `json:"model"`
	GroupID    int64  `json:"group_id"`
	Platform   string `json:"platform"`
	Size       string `json:"size"`
	Quality    string `json:"quality"`
	Format     string `json:"format"`
	Seconds    int    `json:"seconds"`
	Resolution string `json:"resolution"`
}

func (e *creativeStudioExecutor) Execute(ctx context.Context, job creativestudio.Job, inputs []creativestudio.AssetFile, progress func(string, bool) error) (creativestudio.Result, error) {
	if job.Kind == "compose" {
		return e.render(ctx, job, inputs)
	}
	var params creativeGenerationParams
	if err := json.Unmarshal(job.Params, &params); err != nil || strings.TrimSpace(params.Model) == "" || params.GroupID <= 0 {
		return creativestudio.Result{}, errors.New("请选择可用模型与分组 / Select an available model and group")
	}
	if job.Kind != "image" && job.Kind != "video" {
		return creativestudio.Result{}, errors.New("unsupported generation task")
	}
	key, platform, err := e.resolveKey(ctx, job.UserID, job.Kind, params.GroupID)
	if err != nil {
		return creativestudio.Result{}, err
	}
	// Platform comes from the authorized group; client-supplied platform labels
	// cannot influence request routing or the provider's accepted parameters.
	params.Platform = platform
	for _, input := range inputs {
		if input.Kind != "image" {
			return creativestudio.Result{}, errors.New("生成参考素材必须是图片 / Generation references must be images")
		}
	}
	if job.Kind == "image" {
		return e.image(ctx, job, params, key.Key, inputs)
	}
	if len(inputs) > 1 {
		return creativestudio.Result{}, errors.New("视频生成仅支持一张参考图 / Video accepts one reference image")
	}
	return e.video(ctx, job, params, key.Key, inputs, progress)
}

func (e *creativeStudioExecutor) resolveKey(ctx context.Context, userID int64, kind string, groupID int64) (*service.APIKey, string, error) {
	var groups []service.Group
	var err error
	if kind == "image" {
		groups, err = e.apiKeys.ImageStudioGroups(ctx, userID)
	} else {
		groups, err = e.apiKeys.VideoStudioGroups(ctx, userID)
	}
	if err != nil {
		return nil, "", err
	}
	for _, group := range groups {
		if group.ID != groupID {
			continue
		}
		var key *service.APIKey
		if kind == "image" {
			key, err = e.apiKeys.EnsureImageStudioKeyForGroup(ctx, userID, groupID)
		} else {
			key, err = e.apiKeys.EnsureVideoStudioKeyForGroup(ctx, userID, groupID)
		}
		return key, group.Platform, err
	}
	return nil, "", errors.New("生成分组已不可用 / The selected generation group is no longer available")
}

type creativeGatewayResponse struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	limit    int
	overflow bool
}

type creativeGatewayError struct {
	status  int
	message string
}

func (e *creativeGatewayError) Error() string { return e.message }

func creativeSubmissionError(err error) error {
	var gatewayError *creativeGatewayError
	if errors.As(err, &gatewayError) && gatewayError.status < 500 {
		return err
	}
	return fmt.Errorf("%w: 提交结果无法确认，请检查用量后决定是否重新生成 / Submission could not be confirmed; check usage before generating again: %v", creativestudio.ErrUncertain, err)
}

func (w *creativeGatewayResponse) Header() http.Header { return w.header }
func (w *creativeGatewayResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *creativeGatewayResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(data) > w.limit {
		w.overflow = true
		return 0, errors.New("gateway response exceeds media size limit")
	}
	return w.body.Write(data)
}
func (w *creativeGatewayResponse) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
}

func (e *creativeStudioExecutor) gatewayRequest(ctx context.Context, method, path, key, contentType string, body io.Reader, limit int) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, nil, err
	}
	req.RemoteAddr = "127.0.0.1:0"
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "Sub2API-Creative-Studio")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := &creativeGatewayResponse{header: make(http.Header), limit: limit}
	e.gateway.ServeHTTP(recorder, req)
	if recorder.overflow {
		return nil, nil, errors.New("生成结果超过大小限制 / Generated media exceeds the size limit")
	}
	if recorder.status >= 300 {
		var payload struct {
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(recorder.body.Bytes(), &payload)
		message := payload.Error.Message
		if message == "" {
			message = payload.Message
		}
		if message == "" {
			message = fmt.Sprintf("Generation gateway returned HTTP %d", recorder.status)
		}
		if len(message) > 1600 {
			message = message[:1600]
		}
		return nil, nil, &creativeGatewayError{status: recorder.status, message: message}
	}
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	return recorder.body.Bytes(), recorder.header, nil
}

func (e *creativeStudioExecutor) image(ctx context.Context, job creativestudio.Job, params creativeGenerationParams, key string, refs []creativestudio.AssetFile) (creativestudio.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	var payload bytes.Buffer
	contentType, path := "application/json", "/v1/images/generations"
	if len(refs) > 8 {
		return creativestudio.Result{}, errors.New("图片最多使用八张参考图 / Images accept up to eight references")
	}
	if len(refs) > 0 {
		path = "/v1/images/edits"
		form := multipart.NewWriter(&payload)
		fields := map[string]string{"model": params.Model, "prompt": job.Prompt}
		if params.Platform != service.PlatformGrok {
			if params.Size != "" && params.Size != "auto" {
				fields["size"] = params.Size
			}
			if params.Quality != "" && params.Quality != "auto" {
				fields["quality"] = params.Quality
			}
			if params.Format != "" {
				fields["output_format"] = params.Format
			}
		}
		for name, value := range fields {
			if err := form.WriteField(name, value); err != nil {
				return creativestudio.Result{}, err
			}
		}
		var total int64
		for _, ref := range refs {
			total += ref.Size
			if total > 64<<20 {
				return creativestudio.Result{}, errors.New("参考图片总大小不能超过 64 MB / Reference images exceed 64 MB")
			}
			headers := make(textproto.MIMEHeader)
			headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "image", "filename": ref.ID + creativeImageExtension(ref.MIME)}))
			headers.Set("Content-Type", ref.MIME)
			part, err := form.CreatePart(headers)
			if err != nil {
				return creativestudio.Result{}, err
			}
			f, err := os.Open(ref.Path)
			if err != nil {
				return creativestudio.Result{}, err
			}
			copied, copyErr := io.Copy(part, io.LimitReader(f, (32<<20)+1))
			_ = f.Close()
			if copyErr != nil {
				return creativestudio.Result{}, copyErr
			}
			if copied > 32<<20 {
				return creativestudio.Result{}, errors.New("参考图片超过 32 MB / Reference image exceeds 32 MB")
			}
		}
		if err := form.Close(); err != nil {
			return creativestudio.Result{}, err
		}
		contentType = form.FormDataContentType()
	} else {
		fields := map[string]any{"model": params.Model, "prompt": job.Prompt, "n": 1}
		if params.Platform != service.PlatformGrok {
			if params.Size != "" && params.Size != "auto" {
				fields["size"] = params.Size
			}
			if params.Quality != "" && params.Quality != "auto" {
				fields["quality"] = params.Quality
			}
			if params.Format != "" {
				fields["output_format"] = params.Format
			}
			if strings.HasPrefix(params.Model, "dall-e") {
				fields["response_format"] = "b64_json"
				delete(fields, "output_format")
			}
		}
		if err := json.NewEncoder(&payload).Encode(fields); err != nil {
			return creativestudio.Result{}, err
		}
	}
	body, _, err := e.gatewayRequest(ctx, http.MethodPost, path, key, contentType, &payload, 48<<20)
	if err != nil {
		return creativestudio.Result{}, creativeSubmissionError(err)
	}
	var result struct {
		Data []struct {
			Base64 string `json:"b64_json"`
			URL    string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Data) == 0 {
		return creativestudio.Result{}, creativeImageResultError(errors.New("生成接口未返回可解析的图片结果 / No usable image result was returned"))
	}
	var data []byte
	if result.Data[0].Base64 != "" {
		data, err = base64.StdEncoding.DecodeString(result.Data[0].Base64)
	} else {
		data, err = handler.DownloadStudioImageAsset(ctx, result.Data[0].URL)
	}
	if err != nil {
		return creativestudio.Result{}, creativeImageResultError(err)
	}
	if len(data) == 0 || len(data) > 32<<20 {
		return creativestudio.Result{}, creativeImageResultError(errors.New("图片内容为空或超过 32 MB / Image is empty or exceeds 32 MB"))
	}
	mediaType := http.DetectContentType(data)
	if !strings.HasPrefix(mediaType, "image/") {
		return creativestudio.Result{}, creativeImageResultError(errors.New("上游未返回有效图片 / Upstream returned invalid image data"))
	}
	return creativestudio.Result{Data: data, MIME: mediaType, Name: "image-" + job.ID + creativeImageExtension(mediaType)}, nil
}

func creativeImageResultError(err error) error {
	return fmt.Errorf("%w: 上游可能已生成并计费，但未能取回图片。请检查用量后决定是否重新生成 / The provider may have generated and billed the image. Check usage before generating again: %v", creativestudio.ErrUncertain, err)
}

func creativeImageExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/avif":
		return ".avif"
	default:
		return ".png"
	}
}

func (e *creativeStudioExecutor) video(ctx context.Context, job creativestudio.Job, params creativeGenerationParams, key string, refs []creativestudio.AssetFile, progress func(string, bool) error) (creativestudio.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	remoteID := job.RemoteID
	if remoteID == "" {
		payload := map[string]any{"model": params.Model, "prompt": job.Prompt, "seconds": params.Seconds, "resolution": params.Resolution}
		if params.Seconds < 1 || params.Seconds > 30 {
			return creativestudio.Result{}, errors.New("视频时长必须为 1–30 秒 / Video duration must be 1–30 seconds")
		}
		if params.Size != "" && params.Size != "auto" {
			payload["size"] = params.Size
		}
		if len(refs) == 1 {
			data, err := os.ReadFile(refs[0].Path)
			if err != nil {
				return creativestudio.Result{}, err
			}
			payload["image_url"] = "data:" + refs[0].MIME + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return creativestudio.Result{}, err
		}
		submitCtx, submitCancel := context.WithTimeout(ctx, 2*time.Minute)
		result, _, err := e.gatewayRequest(submitCtx, http.MethodPost, "/v1/videos", key, "application/json", bytes.NewReader(body), 2<<20)
		submitCancel()
		if err != nil {
			return creativestudio.Result{}, creativeSubmissionError(err)
		}
		var submitted struct {
			ID        string `json:"id"`
			RequestID string `json:"request_id"`
			TaskID    string `json:"task_id"`
		}
		if err := json.Unmarshal(result, &submitted); err != nil {
			return creativestudio.Result{}, fmt.Errorf("%w: 视频提交结果无法确认，请检查用量后重试 / Video submission could not be confirmed; check usage before retrying", creativestudio.ErrUncertain)
		}
		remoteID = submitted.ID
		if remoteID == "" {
			remoteID = submitted.RequestID
		}
		if remoteID == "" {
			remoteID = submitted.TaskID
		}
		if remoteID == "" {
			return creativestudio.Result{}, fmt.Errorf("%w: 视频接口未返回任务编号 / Video task ID was not returned", creativestudio.ErrUncertain)
		}
		// Persist before the first poll. Retrying a poll or download keeps this ID.
		if err := progress(remoteID, false); err != nil {
			return creativestudio.Result{}, err
		}
	}
	if strings.ContainsAny(remoteID, "/\\?#\r\n") || len(remoteID) > 240 || remoteID == "." || remoteID == ".." {
		return creativestudio.Result{}, errors.New("invalid upstream video task ID")
	}
	endpoint := "/v1/videos/" + url.PathEscape(remoteID)
	failures := 0
	for {
		pollCtx, pollCancel := context.WithTimeout(ctx, 45*time.Second)
		body, _, err := e.gatewayRequest(pollCtx, http.MethodGet, endpoint, key, "", nil, 2<<20)
		pollCancel()
		if err != nil {
			failures++
			if failures >= 3 {
				return creativestudio.Result{}, fmt.Errorf("视频状态查询失败，重试会继续原任务 / Video polling failed; retry resumes this task: %w", err)
			}
		} else {
			failures = 0
			var status struct {
				Status string          `json:"status"`
				State  string          `json:"state"`
				Error  json.RawMessage `json:"error"`
			}
			if err = json.Unmarshal(body, &status); err != nil {
				return creativestudio.Result{}, errors.New("invalid video status response")
			}
			phase := strings.ToLower(status.Status)
			if phase == "" {
				phase = strings.ToLower(status.State)
			}
			switch phase {
			case "failed", "expired", "cancelled", "canceled", "error":
				if err := progress(remoteID, true); err != nil {
					return creativestudio.Result{}, err
				}
				return creativestudio.Result{}, errors.New("上游视频任务失败，重试会重新生成 / Upstream video task failed; retry creates a new generation")
			case "done", "succeeded", "completed", "success":
				data, headers, err := e.gatewayRequest(ctx, http.MethodGet, endpoint+"/content", key, "", nil, 128<<20)
				if err != nil {
					return creativestudio.Result{}, fmt.Errorf("视频下载失败，重试仅取回原视频 / Video download failed; retry resumes this task: %w", err)
				}
				mediaType := strings.Split(headers.Get("Content-Type"), ";")[0]
				detected := http.DetectContentType(data)
				if !strings.HasPrefix(mediaType, "video/") {
					mediaType = detected
				}
				if len(data) == 0 || !strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(detected, "text/") || strings.HasPrefix(detected, "image/") {
					return creativestudio.Result{}, errors.New("视频接口未返回有效视频 / Video content is invalid")
				}
				extension := ".mp4"
				if mediaType == "video/webm" {
					extension = ".webm"
				}
				return creativestudio.Result{Data: data, MIME: mediaType, Name: "video-" + job.ID + extension}, nil
			}
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return creativestudio.Result{}, ctx.Err()
		case <-timer.C:
		}
	}
}
