package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type klingRoundTripFunc func(*http.Request) (*http.Response, error)

func (f klingRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestBuildKlingStudioRequest(t *testing.T) {
	for _, tt := range []struct {
		name, body, duration, aspect string
		endpoint                     KlingEndpoint
	}{
		{"defaults", `{"model":"kling-v3","prompt":"cat"}`, "5", "16:9", KlingEndpointText2Video},
		{"ui parameters", `{"model":"kling-v2-6","prompt":"cat","duration":10,"aspect_ratio":"9:16","resolution":"720p"}`, "10", "9:16", KlingEndpointText2Video},
		{"unsupported UI defaults", `{"model":"kling-v3","prompt":"cat","duration":6,"aspect_ratio":"7:4"}`, "5", "16:9", KlingEndpointText2Video},
		{"image", `{"model":"kling-v3","prompt":"cat","image":{"url":"https://example.com/cat.png"}}`, "5", "16:9", KlingEndpointImage2Video},
		{"4.0", `{"model":"kling-4.0","prompt":"cat","duration":10,"aspect_ratio":"1:1"}`, "10", "1:1", KlingEndpointKling4Text},
		{"4.0 image", `{"model":"kling-4.0","prompt":"cat","image":{"url":"https://example.com/cat.png"}}`, "5", "16:9", KlingEndpointKling4Image},
	} {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, body, model, err := BuildKlingStudioRequest([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.endpoint, endpoint)
			require.Equal(t, gjson.Get(tt.body, "model").String(), model)
			require.Equal(t, "cat", gjson.GetBytes(body, "prompt").String())
			prefix := ""
			if strings.Contains(string(endpoint), "kling4") {
				prefix = "settings."
				require.False(t, gjson.GetBytes(body, "model_name").Exists())
				require.False(t, gjson.GetBytes(body, "mode").Exists())
			} else {
				require.Equal(t, model, gjson.GetBytes(body, "model_name").String())
				require.Equal(t, "std", gjson.GetBytes(body, "mode").String())
			}
			require.Equal(t, tt.duration, gjson.GetBytes(body, prefix+"duration").String())
			require.Equal(t, tt.aspect, gjson.GetBytes(body, prefix+"aspect_ratio").String())
			require.False(t, gjson.GetBytes(body, "resolution").Exists())
			if endpoint == KlingEndpointKling4Image {
				require.Equal(t, "first_frame", gjson.GetBytes(body, "contents.0.type").String())
				require.Equal(t, "https://example.com/cat.png", gjson.GetBytes(body, "contents.0.url").String())
			}
		})
	}
	for _, body := range []string{`[]`, `{`, `{"model":"kolors","prompt":"x"}`, `{"model":"kling-image","prompt":"x"}`, `{"model":"kling-v3"}`} {
		_, _, _, err := BuildKlingStudioRequest([]byte(body))
		require.Error(t, err, body)
	}
}

func TestKlingStudioTaskStatus(t *testing.T) {
	for upstream, want := range map[string]string{"submitted": "pending", "processing": "processing", "succeed": "completed", "succeeded": "completed", "failed": "failed"} {
		got := KlingStudioResponse(KlingEndpointQueryVideo, &KlingTaskResult{TaskID: "123", Status: upstream, Message: "reason"})
		require.Equal(t, want, got["status"])
		if want == "failed" {
			require.Equal(t, "reason", got["error"].(gin.H)["message"])
		}
	}
	for submit, query := range map[KlingEndpoint]KlingEndpoint{
		KlingEndpointText2Video:  KlingEndpointQueryVideo,
		KlingEndpointImage2Video: KlingEndpointQueryImage,
		KlingEndpointKling4Text:  KlingEndpointQueryKling4,
		KlingEndpointKling4Image: KlingEndpointQueryKling4,
	} {
		endpoint, id, err := ParseKlingStudioTaskID(KlingStudioTaskID(submit, "123"))
		require.NoError(t, err)
		require.Equal(t, query, endpoint)
		require.Equal(t, "123", id)
	}
	for _, id := range []string{"", "../tasks", "kling4:", "foo?task_ids=other", "kling4:1&other=2"} {
		_, _, err := ParseKlingStudioTaskID(id)
		require.Error(t, err)
	}
}

func TestForwardKlingStudioSubmitAndQuery(t *testing.T) {
	for _, model := range []string{"kling-v3", "kling-4.0"} {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "))
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Equal(t, "cat", gjson.GetBytes(body, "prompt").String())
					if model == "kling-4.0" {
						require.Equal(t, "/text-to-video/kling-4.0", r.URL.Path)
						_, _ = w.Write([]byte(`{"code":0,"data":{"id":"123","status":"submitted"}}`))
					} else {
						require.Equal(t, "/v1/videos/text2video", r.URL.Path)
						_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"123","task_status":"submitted"}}`))
					}
					return
				}
				if model == "kling-4.0" {
					require.Equal(t, "/tasks?task_ids=123", r.URL.String())
					_, _ = w.Write([]byte(`{"code":0,"data":[{"id":"123","status":"succeeded","outputs":[{"type":"video","url":"https://v15-kling.klingai.com/result.mp4","duration":10}]}]}`))
				} else {
					require.Equal(t, "/v1/videos/text2video/123", r.URL.Path)
					_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"123","task_status":"succeed","task_result":{"videos":[{"url":"https://s15-kling.klingai.com/result.mp4","duration":"10"}]}}}`))
				}
			}))
			defer upstream.Close()
			account := &Account{Platform: PlatformKling, Credentials: map[string]any{"base_url": upstream.URL, "api_key": "ak|sk"}}
			svc := &OpenAIGatewayService{}
			endpoint, body, _, err := BuildKlingStudioRequest([]byte(`{"model":"` + model + `","prompt":"cat","duration":10}`))
			require.NoError(t, err)
			result, err := svc.ForwardKling(context.Background(), nil, account, endpoint, "", body)
			require.NoError(t, err)
			query, id, err := ParseKlingStudioTaskID(KlingStudioTaskID(endpoint, result.TaskID))
			require.NoError(t, err)
			result, err = svc.ForwardKling(context.Background(), nil, account, query, id, nil)
			require.NoError(t, err)
			require.Equal(t, "completed", KlingStudioResponse(query, result)["status"])
			require.Equal(t, 10, result.DurationSeconds)
			require.Equal(t, 2, calls)
		})
	}
}

func TestForwardKlingStudioContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, contentType := range []string{"video/mp4", "image/png"} {
		t.Run(contentType, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("Cookie"))
				require.Equal(t, "bytes=0-4", r.Header.Get("Range"))
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Content-Range", "bytes 0-4/10")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("media"))
			}))
			defer upstream.Close()
			old := klingMediaContentClient
			t.Cleanup(func() { klingMediaContentClient = old })
			klingMediaContentClient = &http.Client{Transport: klingRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "s15-kling.klingai.com", req.URL.Hostname())
				copy := req.Clone(req.Context())
				copy.URL, _ = url.Parse(upstream.URL + req.URL.RequestURI())
				return http.DefaultTransport.RoundTrip(copy)
			})}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/tasks/123/content", nil)
			c.Request.Header.Set("Range", "bytes=0-4")
			c.Request.Header.Set("Authorization", "Bearer user-secret")
			c.Request.Header.Set("Cookie", "session=secret")
			err := (&OpenAIGatewayService{}).ForwardKlingStudioContent(context.Background(), c, &KlingTaskResult{TaskID: "123", Status: "succeed", URL: "https://s15-kling.klingai.com/result"})
			require.NoError(t, err)
			require.Equal(t, http.StatusPartialContent, w.Code)
			require.Equal(t, contentType, w.Header().Get("Content-Type"))
			require.Equal(t, "bytes 0-4/10", w.Header().Get("Content-Range"))
			require.Equal(t, "media", w.Body.String())
		})
	}
}

func TestForwardKlingStudioContentRejectsPendingAndUnsafeURLs(t *testing.T) {
	for _, result := range []*KlingTaskResult{
		{Status: "processing", URL: "https://s15-kling.klingai.com/result"},
		{Status: "succeed", URL: "http://169.254.169.254/metadata"},
		{Status: "succeed", URL: "https://127.0.0.1/result"},
		{Status: "succeed", URL: "https://user:secret@example.com/result"},
	} {
		err := (&OpenAIGatewayService{}).ForwardKlingStudioContent(context.Background(), nil, result)
		require.Error(t, err)
	}
}
