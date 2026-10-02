//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type grokPublicVideoRoundTripFunc func(*http.Request) (*http.Response, error)

func (f grokPublicVideoRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func installGrokPublicVideoTransport(t *testing.T, roundTrip grokPublicVideoRoundTripFunc) {
	t.Helper()
	original := grokMediaPublicVideoClient
	client := *original
	client.Transport = roundTrip
	grokMediaPublicVideoClient = &client
	t.Cleanup(func() { grokMediaPublicVideoClient = original })
}

func TestForwardGrokMediaContentDownloadsRelayPublicVideoWithoutCredentials(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(
		`{"status":"done","video":{"url":"/v1/videos/task-1/content","duration":4},"video_url":"https://cdn.example/video.mp4?signature=asset-token"}`,
	)}
	account := grokMediaContentTestAccount()
	account.Credentials[credKeyHeaderOverrideEnabled] = true
	account.Credentials[credKeyHeaderOverrides] = map[string]any{"user-agent": "private-agent", "x-private-header": "private-value"}
	var downloaded *http.Request
	installGrokPublicVideoTransport(t, func(req *http.Request) (*http.Response, error) {
		downloaded = req
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-12/100"}, "Content-Length": {"13"}},
			Body:       io.NopCloser(strings.NewReader("video-payload")),
		}, nil
	})
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task-1/content", map[string]string{"Range": "bytes=0-12", "Cookie": "client-cookie"})
	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "video-payload", recorder.Body.String())
	require.Equal(t, "bytes 0-12/100", recorder.Header().Get("Content-Range"))
	require.Len(t, upstream.requests, 1, "the broken relay content path must not be requested")
	require.Equal(t, "Bearer upstream-key", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, []string{"private-value"}, headerValuesEqualFold(upstream.requests[0].Header, "X-Private-Header"))
	require.NotNil(t, downloaded)
	require.Equal(t, "https://cdn.example/video.mp4?signature=asset-token", downloaded.URL.String())
	require.Equal(t, "bytes=0-12", downloaded.Header.Get("Range"))
	require.Equal(t, "Go-http-client/1.1", downloaded.Header.Get("User-Agent"))
	for _, header := range []string{"Authorization", "Cookie", "X-Private-Header"} {
		require.Empty(t, headerValuesEqualFold(downloaded.Header, header), header)
	}
}

func TestForwardGrokMediaContentRejectsPublicVideoRedirect(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(
		`{"status":"done","video_url":"https://cdn.example/video.mp4"}`,
	)}
	requests := 0
	installGrokPublicVideoTransport(t, func(req *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"http://169.254.169.254/latest/meta-data"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, _ := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task-1/content", nil)
	_, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.ErrorContains(t, err, "redirect is not allowed")
	require.Equal(t, 1, requests)
}

func TestForwardGrokMediaContentPublicRelayCompletionRemainsBillable(t *testing.T) {
	for _, body := range []string{
		`{"status":"done","model":"grok-imagine-video","duration":4,"video_url":"https://cdn.example/video.mp4"}`,
		`{"status":"completed","model":"grok-imagine-video","video":{"url":"/v1/videos/task-1/content","duration":4},"video_url":"https://cdn.example/video.mp4"}`,
		`{"status":"succeeded","model":"grok-imagine-video","duration":4,"video_url":"https://cdn.example/video.mp4"}`,
	} {
		t.Run(body, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(body)}
			installGrokPublicVideoTransport(t, func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"video/mp4"}}, Body: io.NopCloser(strings.NewReader("video-payload"))}, nil
			})
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, recorder := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task-1/content", nil)
			result, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
			require.NoError(t, err)
			require.Equal(t, "video-payload", recorder.Body.String())
			require.Equal(t, 1, result.VideoCount)
			require.Equal(t, "grok-imagine-video", result.Model)
			require.Equal(t, 4, result.VideoDurationSeconds)
			require.Equal(t, "task-1", result.ResponseID)
		})
	}
}

func TestForwardGrokMediaContentPublicRelayIncompleteDownloadDoesNotBill(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		status  int
		body    io.Reader
		wantErr bool
	}{
		{name: "unsatisfied range", status: http.StatusRequestedRangeNotSatisfiable, body: strings.NewReader("bad range")},
		{name: "truncated video", status: http.StatusOK, body: io.MultiReader(strings.NewReader("partial"), grokPublicVideoErrorReader{}), wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"status":"completed","video_url":"https://cdn.example/video.mp4"}`)}
			installGrokPublicVideoTransport(t, func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: testCase.status, Header: http.Header{"Content-Type": {"video/mp4"}}, Body: io.NopCloser(testCase.body)}, nil
			})
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, _ := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task-1/content", nil)
			result, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
			if testCase.wantErr {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Zero(t, result.VideoCount)
			}
		})
	}
}

type grokPublicVideoErrorReader struct{}

func (grokPublicVideoErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestGrokMediaPublicVideoContentURLRejectsUnsafeTargets(t *testing.T) {
	for _, raw := range []string{
		"http://cdn.example/video.mp4", "https://user:secret@cdn.example/video.mp4",
		"https://cdn.example:444/video.mp4", "https://cdn.example/video.mp4#fragment",
		"https://localhost/video.mp4", "https://localhost./video.mp4", "https://metadata.google.internal/video.mp4",
		"https://127.0.0.1/video.mp4", "https://10.0.0.1/video.mp4", "https://169.254.169.254/video.mp4",
		"https://100.64.0.1/video.mp4", "https://[::1]/video.mp4", "https://[::ffff:127.0.0.1]/video.mp4",
		"https://224.0.0.1/video.mp4", "https://[ff02::1]/video.mp4", "https://[fc00::1]/video.mp4",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := grokMediaPublicVideoContentURL([]byte(`{"status":"done","video_url":"`+raw+`"}`), "task-1")
			require.Error(t, err)
		})
	}
}

func TestGrokMediaPublicVideoContentURLRequiresCompletionAndSkipsRelayPaths(t *testing.T) {
	for _, body := range []string{
		`{"status":"pending","video_url":"https://cdn.example/video.mp4"}`,
		`{"status":"done","video_url":"/v1/videos/task-1/content"}`,
		`{"status":"done","video_url":"https://relay.example/v1/videos/task-1/content"}`,
	} {
		got, err := grokMediaPublicVideoContentURL([]byte(body), "task-1")
		require.NoError(t, err)
		require.Empty(t, got)
	}
}

func TestGrokMediaPublicVideoTransportRejectsNonPublicAddresses(t *testing.T) {
	transport, ok := grokMediaPublicVideoClient.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.Proxy, "public asset downloads must not inherit proxy environment variables")
	for _, address := range []string{"127.0.0.1:443", "10.0.0.1:443", "169.254.169.254:443", "100.64.0.1:443", "[::1]:443", "[::ffff:127.0.0.1]:443", "224.0.0.1:443", "localhost:443"} {
		conn, err := transport.DialContext(context.Background(), "tcp", address)
		require.Nil(t, conn)
		require.ErrorContains(t, err, "blocked by SSRF policy")
	}
}

func TestGrokMediaPublicVideoResponseRejectsNonVideoAndOversize(t *testing.T) {
	for _, resp := range []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"video/mp4"}}, ContentLength: grokMediaPublicVideoMaxBytes + 1},
	} {
		require.Error(t, validateGrokMediaPublicVideoResponse(resp))
	}
}
