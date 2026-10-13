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
	"github.com/tidwall/gjson"
)

func TestForwardGrokVideoReferenceUsesConfiguredRelayFormat(t *testing.T) {
	for _, format := range []string{"", "image_url"} {
		t.Run("format="+format, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"request_id":"task-1"}`)}
			account := grokMediaContentTestAccount()
			account.Credentials["video_image_field"] = format
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
			body := []byte(`{"model":"grok-imagine-video","prompt":"animate","duration":4,"resolution":"480p","image":{"url":"data:image/jpeg;base64,dGVzdA=="}}`)
			_, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "", body, "application/json")
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			forwarded, err := io.ReadAll(upstream.request.Body)
			require.NoError(t, err)
			require.Equal(t, "4", gjson.GetBytes(forwarded, "duration").String())
			require.Equal(t, "480p", gjson.GetBytes(forwarded, "resolution").String())
			if format == "image_url" {
				require.False(t, gjson.GetBytes(forwarded, "image").Exists())
				require.Equal(t, "data:image/jpeg;base64,dGVzdA==", gjson.GetBytes(forwarded, "image_url").String())
			} else {
				require.False(t, gjson.GetBytes(forwarded, "image_url").Exists())
				require.Equal(t, "data:image/jpeg;base64,dGVzdA==", gjson.GetBytes(forwarded, "image.url").String())
			}
		})
	}
}

func TestAdaptGrokVideoReferenceDoesNotDropAmbiguousInputs(t *testing.T) {
	account := grokMediaContentTestAccount()
	account.Credentials["video_image_field"] = "image_url"
	for _, extra := range []string{`"image_url":"other"`, `"images":[{"url":"other"}]`, `"reference_images":[{"url":"other"}]`} {
		_, err := adaptGrokVideoReferenceForAccount(account, GrokMediaEndpointVideosGenerations, []byte(`{"image":{"url":"source"},`+extra+`}`))
		require.Error(t, err)
	}
	for _, body := range []string{`{"image":{"file_id":"file-1"}}`, `{"image":[{"url":"one"},{"url":"two"}]}`} {
		_, err := adaptGrokVideoReferenceForAccount(account, GrokMediaEndpointVideosGenerations, []byte(body))
		require.Error(t, err)
	}
	for _, body := range []string{`{"prompt":"text only"}`, `{"image_url":"source"}`} {
		out, err := adaptGrokVideoReferenceForAccount(account, GrokMediaEndpointVideosGenerations, []byte(body))
		require.NoError(t, err)
		require.Equal(t, body, string(out))
	}
	body := `{"image":{"url":"source"}}`
	out, err := adaptGrokVideoReferenceForAccount(account, GrokMediaEndpointImagesEdits, []byte(body))
	require.NoError(t, err)
	require.Equal(t, body, strings.TrimSpace(string(out)))
}
