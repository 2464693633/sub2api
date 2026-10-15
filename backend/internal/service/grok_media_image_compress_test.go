package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// buildNoiseJPEG 生成指定边长的噪声图（无法高效压缩），用于稳定构造超限参考图。
func buildNoiseJPEG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	rng := rand.New(rand.NewSource(42))
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.Set(x, y, color.RGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatalf("encode noise jpeg: %v", err)
	}
	return buf.Bytes()
}

func dataURI(raw []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
}

func TestCompressGrokMediaReferenceImagesShrinksOversizedDataURI(t *testing.T) {
	t.Setenv("SUB2API_GROK_REF_MAX_BYTES", "")
	big := dataURI(buildNoiseJPEG(t, 1100))
	if len(big) <= grokMediaReferenceImageDefaultMaxBytes {
		t.Fatalf("fixture too small: %d bytes", len(big))
	}
	body := []byte(`{"model":"grok-imagine-video-1.5","prompt":"a cat","seconds":6,"resolution":"480p","image_url":"` + big + `"}`)

	out := compressGrokMediaReferenceImages(GrokMediaEndpointVideosGenerations, body)
	compressed := gjson.GetBytes(out, "image_url").String()
	if compressed == big {
		t.Fatal("oversized reference image was not compressed")
	}
	if !strings.HasPrefix(compressed, "data:image/jpeg;base64,") {
		t.Fatalf("compressed image is not a jpeg data URI: %.60s", compressed)
	}
	if raw, err := base64.StdEncoding.DecodeString(strings.SplitN(compressed, ",", 2)[1]); err != nil {
		t.Fatalf("compressed payload is not valid base64: %v", err)
	} else if len(raw) > grokMediaReferenceImageDefaultMaxBytes {
		t.Fatalf("compressed payload still over limit: %d > %d", len(raw), grokMediaReferenceImageDefaultMaxBytes)
	}
}

func TestCompressGrokMediaReferenceImagesLeavesSmallImagesUntouched(t *testing.T) {
	small := dataURI(buildNoiseJPEG(t, 64))
	body := []byte(`{"model":"grok-imagine-video-1.5","prompt":"a cat","images":[{"url":"` + small + `"}]}`)

	out := compressGrokMediaReferenceImages(GrokMediaEndpointVideosGenerations, body)
	if string(out) != string(body) {
		t.Fatal("small reference image should pass through byte-identical")
	}
}

func TestCompressGrokMediaReferenceImagesObjectArrayForms(t *testing.T) {
	big := dataURI(buildNoiseJPEG(t, 1100))
	body := []byte(`{"model":"grok-imagine-video","reference_images":[{"url":"` + big + `"},{"image_url":"` + big + `"}]}`)

	out := compressGrokMediaReferenceImages(GrokMediaEndpointVideosGenerations, body)
	changed := 0
	for _, item := range gjson.GetBytes(out, "reference_images").Array() {
		for _, key := range []string{"url", "image_url"} {
			if v := item.Get(key); v.Exists() && v.String() != big {
				changed++
			}
		}
	}
	if changed != 2 {
		t.Fatalf("expected both array entries compressed, got %d", changed)
	}
}

func TestCompressGrokMediaReferenceImagesSkipsMask(t *testing.T) {
	big := dataURI(buildNoiseJPEG(t, 1100))
	body := []byte(`{"model":"grok-imagine-image-quality","prompt":"edit","image":{"url":"` + big + `"},"mask":{"url":"` + big + `"}}`)

	out := compressGrokMediaReferenceImages(GrokMediaEndpointImagesEdits, body)
	if got := gjson.GetBytes(out, "mask.url").String(); got != big {
		t.Fatal("mask must not be compressed")
	}
	if got := gjson.GetBytes(out, "image.url").String(); got == big {
		t.Fatal("image field should be compressed")
	}
}

func TestCompressGrokMediaReferenceImagesIgnoresNonImageEndpoints(t *testing.T) {
	big := dataURI(buildNoiseJPEG(t, 1100))
	body := []byte(`{"model":"grok-imagine-video-1.5","image_url":"` + big + `"}`)

	if out := compressGrokMediaReferenceImages(GrokMediaEndpointVideoStatus, body); string(out) != string(body) {
		t.Fatal("non-request-body endpoints must not be rewritten")
	}
}
