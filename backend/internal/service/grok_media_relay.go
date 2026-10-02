package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Some relays require the legacy top-level image_url instead of xAI's image
// object. Enable that conversion only for an explicitly configured account;
// never retry an uncertain creation with a different payload.
func adaptGrokVideoReferenceForAccount(account *Account, endpoint GrokMediaEndpoint, body []byte) ([]byte, error) {
	if account == nil || endpoint != GrokMediaEndpointVideosGenerations ||
		strings.TrimSpace(account.GetCredential("video_image_field")) != "image_url" || !gjson.ValidBytes(body) {
		return body, nil
	}
	value := gjson.GetBytes(body, "image")
	if !value.Exists() {
		return body, nil
	}
	imageURL := extractGrokMediaImageURL(value)
	if value.IsArray() || imageURL == "" || gjson.GetBytes(body, "image_url").Exists() ||
		gjson.GetBytes(body, "images").Exists() || gjson.GetBytes(body, "reference_images").Exists() {
		return nil, fmt.Errorf("configured video relay requires a single unambiguous image URL")
	}
	out, err := sjson.SetBytes(body, "image_url", imageURL)
	if err != nil {
		return nil, fmt.Errorf("adapt video relay image URL: %w", err)
	}
	return sjson.DeleteBytes(out, "image")
}
