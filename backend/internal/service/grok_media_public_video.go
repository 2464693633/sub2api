package service

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const grokMediaPublicVideoMaxBytes = 128 << 20

// Relay status responses can contain a public CDN video_url alongside a broken
// authenticated /content URL. Resolve only server-returned completed-task URLs;
// this client must never inherit credentials or proxy settings from an account.
// safeDialContext pins each connection to a checked public IP, preventing DNS
// rebinding. Disabling redirects preserves the initial HTTPS-only URL policy.
var grokMediaPublicVideoClient = &http.Client{
	Timeout: 5 * time.Minute,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		DialContext:           safeDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

func grokMediaPublicVideoContentURL(body []byte, requestID string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String())) {
	case "done", "completed", "succeeded", "success":
	default:
		return "", nil
	}
	raw := strings.TrimSpace(gjson.GetBytes(body, "video_url").String())
	if raw == "" || isGrokMediaVideoContentURL(raw, requestID) {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || (parsed.Port() != "" && parsed.Port() != "443") ||
		isBlockedHostname(strings.TrimSuffix(parsed.Hostname(), ".")) {
		return "", fmt.Errorf("grok media status returned an unsupported public video URL")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && isPrivateIP(ip) {
		return "", fmt.Errorf("grok media status returned a non-public video URL")
	}
	return parsed.String(), nil
}

func validateGrokMediaPublicVideoResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("grok public video download returned unexpected status %d", resp.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || (!strings.HasPrefix(contentType, "video/") && contentType != "application/octet-stream") {
		return fmt.Errorf("grok public video download returned unsupported content type")
	}
	if resp.ContentLength > grokMediaPublicVideoMaxBytes {
		return fmt.Errorf("grok public video download exceeds size limit")
	}
	return nil
}

// Normalize only the private completion snapshot used after a verified public
// download. Existing pending-billing/claim handling still owns pricing and
// deduplication; an alias status or a top-level URL must not bypass charging.
func grokMediaPublicVideoBillingStatus(body []byte, contentURL string) ([]byte, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, fmt.Errorf("grok public video status is not a valid object")
	}
	out, err := sjson.SetBytes(body, "status", "done")
	if err != nil {
		return nil, err
	}
	out, err = sjson.SetBytes(out, "video.url", contentURL)
	if err != nil {
		return nil, err
	}
	if !gjson.GetBytes(out, "video.duration").Exists() {
		if duration := gjson.GetBytes(body, "duration"); duration.Type == gjson.Number {
			out, err = sjson.SetRawBytes(out, "video.duration", []byte(duration.Raw))
		}
	}
	return out, err
}
