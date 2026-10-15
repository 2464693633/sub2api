package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // 注册 png 解码，参考图输入侧支持
	"os"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
	"golang.org/x/image/draw"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// OneAPI 型 grok 中转对参考图请求体有约 1MB（base64 后）的限制，超限时只回
// 不透明的 "xAI upstream returned status 400"（2026-10-15 实测：二进制
// 649KB 可过、874KB 被拒）。转发前把过大的 data URI 参考图压到阈值内，
// 与分辨率/时长/像素尺寸无关。
const (
	grokMediaReferenceImageDefaultMaxBytes = 600 * 1024
	// 预缩放上限：超长边参考图先降到该尺寸再迭代压缩，限制解码内存
	grokMediaReferenceImageMaxDimension = 3072
	grokMediaReferenceImageMinQuality   = 45
)

func grokMediaReferenceImageMaxBytes() int {
	if v := strings.TrimSpace(os.Getenv("SUB2API_GROK_REF_MAX_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return grokMediaReferenceImageDefaultMaxBytes
}

// grokMediaReferenceImageFields 返回该端点携带参考图的字段；mask 不压缩，
// 遮罩语义对像素更敏感且当前已知失败场景均为视频参考图。
func grokMediaReferenceImageFields(endpoint GrokMediaEndpoint) []string {
	switch endpoint {
	case GrokMediaEndpointVideosGenerations:
		return []string{"image", "images", "reference_images", "image_url", "image_urls"}
	case GrokMediaEndpointImagesEdits:
		return []string{"image", "images", "image_url", "image_urls"}
	}
	return nil
}

// compressGrokMediaReferenceImages 就地压缩请求体中超阈值的 data URI 参考图。
// 任一图片处理失败都原样保留该字段，交由上游判定。
func compressGrokMediaReferenceImages(endpoint GrokMediaEndpoint, body []byte) []byte {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body
	}
	if !bytes.Contains(body, []byte("data:image/")) {
		return body
	}
	maxBytes := grokMediaReferenceImageMaxBytes()
	out := body
	for _, field := range grokMediaReferenceImageFields(endpoint) {
		value := gjson.GetBytes(out, field)
		if !value.Exists() {
			continue
		}
		if value.IsArray() {
			for index := range value.Array() {
				out = compressGrokMediaImageValue(out, fmt.Sprintf("%s.%d", field, index), maxBytes)
			}
			continue
		}
		out = compressGrokMediaImageValue(out, field, maxBytes)
	}
	return out
}

func compressGrokMediaImageValue(body []byte, path string, maxBytes int) []byte {
	value := gjson.GetBytes(body, path)
	if !value.Exists() {
		return body
	}
	if value.Type == gjson.String {
		compressed := compressGrokMediaDataURI(value.String(), maxBytes)
		if compressed == value.String() {
			return body
		}
		out, err := sjson.SetBytes(body, path, compressed)
		if err != nil {
			return body
		}
		logGrokMediaReferenceCompression(path, len(value.String()), len(compressed))
		return out
	}
	// 对象形式：{url: data URI}（canonicalize 之后的标准形态）或残留的
	// {image_url: data URI}，保留原键名写回。
	for _, key := range []string{"url", "image_url"} {
		child := value.Get(key)
		if child.Type != gjson.String {
			continue
		}
		compressed := compressGrokMediaDataURI(child.String(), maxBytes)
		if compressed == child.String() {
			return body
		}
		out, err := sjson.SetBytes(body, path+"."+key, compressed)
		if err != nil {
			return body
		}
		logGrokMediaReferenceCompression(path+"."+key, len(child.String()), len(compressed))
		return out
	}
	return body
}

func logGrokMediaReferenceCompression(path string, before, after int) {
	logger.L().Info(
		"grok_media.reference_image_compressed",
		zap.String("component", "service.grok_media"),
		zap.String("field", path),
		zap.Int("bytes_before", before),
		zap.Int("bytes_after", after),
	)
}

// compressGrokMediaDataURI 压缩单个 data URI：先降 JPEG 质量，到底后逐步缩
// 尺寸，达标即止；8 轮内仍超标则返回最小的一次编码结果（尽力而为）。
// 非 base64、无法解码（webp/gif 等）或本就未超限的输入原样返回。
func compressGrokMediaDataURI(dataURL string, maxBytes int) string {
	comma := strings.Index(dataURL, ",")
	if comma < 0 || !strings.HasPrefix(dataURL, "data:image/") {
		return dataURL
	}
	meta := dataURL[:comma]
	if !strings.Contains(meta, ";base64") {
		return dataURL
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil || len(raw) <= maxBytes {
		return dataURL
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return dataURL
	}
	img = fitGrokMediaReferenceDimension(img)
	quality := 85
	var best []byte
	for attempt := 0; attempt < 8; attempt++ {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return dataURL
		}
		best = buf.Bytes()
		if buf.Len() <= maxBytes {
			break
		}
		if quality > grokMediaReferenceImageMinQuality {
			quality -= 15
			continue
		}
		img = scaleGrokMediaReferenceImage(img, 0.8)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(best)
}

// fitGrokMediaReferenceDimension 把超长边图片预缩到解码上限内，控制重编码
// 过程的内存占用；视频参考图不需要 4K 级别的输入。
func fitGrokMediaReferenceDimension(img image.Image) image.Image {
	bounds := img.Bounds()
	longest := bounds.Dx()
	if bounds.Dy() > longest {
		longest = bounds.Dy()
	}
	if longest <= grokMediaReferenceImageMaxDimension {
		return img
	}
	return scaleGrokMediaReferenceImage(img, float64(grokMediaReferenceImageMaxDimension)/float64(longest))
}

func scaleGrokMediaReferenceImage(src image.Image, ratio float64) image.Image {
	bounds := src.Bounds()
	width := int(float64(bounds.Dx()) * ratio)
	height := int(float64(bounds.Dy()) * ratio)
	if width < 1 || height < 1 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}
