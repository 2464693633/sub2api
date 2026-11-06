package service

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// OneAPI 型中转对请求体同样有约 1MB（multipart/base64 后）限制（与 grok 媒体
// 路径 2026-10-15 实测同源），超限只回不透明 400。API Key 账号的图片编辑请求
// 携带参考图直发中转前，把超阈值的参考图重编码压缩。
const openAIImagesRefDefaultMaxBytes = 600 * 1024

func openAIImagesRefMaxBytes() int {
	if v := strings.TrimSpace(os.Getenv("SUB2API_OPENAI_REF_MAX_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return openAIImagesRefDefaultMaxBytes
}

// compressOpenAIImagesMultipartReferences 重写 multipart 请求体中超阈值的图片
// part（image / image[] 等，按内容类型判定）。非 multipart 请求原样返回；
// 解码失败或未超阈值的 part 原样保留，交由上游判定。
func compressOpenAIImagesMultipartReferences(body []byte, contentType string) ([]byte, string, bool) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, false
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return body, contentType, false
	}
	if !multipartHasOversizedImagePart(body, boundary) {
		return body, contentType, false
	}
	maxBytes := openAIImagesRefMaxBytes()

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	changed := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return body, contentType, false
		}
		data, readErr := io.ReadAll(part)
		formName := strings.TrimSpace(part.FormName())
		partHeader := cloneMultipartHeader(part.Header)
		_ = part.Close()
		if readErr != nil {
			return body, contentType, false
		}
		if !isOpenAIImagesReferencePart(formName, partHeader) || len(data) <= maxBytes {
			writeOpenAIMultipartPart(writer, partHeader, data)
			continue
		}
		compressed, ok := compressImageReferenceBytes(data, maxBytes)
		if !ok {
			writeOpenAIMultipartPart(writer, partHeader, data)
			continue
		}
		writeOpenAIMultipartCompressedPart(writer, partHeader, compressed)
		changed = true
	}
	if err := writer.Close(); err != nil {
		return body, contentType, false
	}
	if changed {
		logger.L().Info(
			"openai_images.reference_image_compressed",
			zap.String("component", "service.openai_gateway"),
			zap.Int("bytes_before", len(body)),
			zap.Int("bytes_after", buffer.Len()),
		)
	}
	return buffer.Bytes(), writer.FormDataContentType(), changed
}

// multipartHasOversizedImagePart 预扫描一遍，避免无图或全达标请求做完整重写。
func multipartHasOversizedImagePart(body []byte, boundary string) bool {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	maxBytes := openAIImagesRefMaxBytes()
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return false
		}
		if err != nil {
			return false
		}
		formName := strings.TrimSpace(part.FormName())
		header := cloneMultipartHeader(part.Header)
		isImage := isOpenAIImagesReferencePart(formName, header)
		n, _ := io.Copy(io.Discard, part)
		_ = part.Close()
		if isImage && n > int64(maxBytes) {
			return true
		}
	}
}

func isOpenAIImagesReferencePart(formName string, header textproto.MIMEHeader) bool {
	if formName != "image" && formName != "image[]" && formName != "images" && formName != "images[]" {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Get("Content-Type")))
	return contentType == "" || strings.HasPrefix(contentType, "image/")
}

func writeOpenAIMultipartPart(writer *multipart.Writer, header textproto.MIMEHeader, data []byte) {
	target, err := writer.CreatePart(header)
	if err != nil {
		return
	}
	_, _ = target.Write(data)
}

// writeOpenAIMultipartCompressedPart 用压缩后的字节替换原 part，并同步修正
// Content-Type / 文件名，避免文件名 .png + 实际 JPEG 字节的组合被严格上游拒绝。
func writeOpenAIMultipartCompressedPart(writer *multipart.Writer, header textproto.MIMEHeader, data []byte) {
	if ct := header.Get("Content-Type"); ct != "" && !strings.EqualFold(ct, "image/jpeg") {
		header.Set("Content-Type", "image/jpeg")
	}
	if filename := header.Get("Content-Disposition"); filename != "" {
		if _, params, err := mime.ParseMediaType(filename); err == nil && params["filename"] != "" {
			ext := path.Ext(params["filename"])
			if !strings.EqualFold(ext, ".jpg") && !strings.EqualFold(ext, ".jpeg") {
				params["filename"] = strings.TrimSuffix(params["filename"], ext) + ".jpg"
				header.Set("Content-Disposition", mime.FormatMediaType("form-data", params))
			}
		}
	}
	writeOpenAIMultipartPart(writer, header, data)
}

// compressImageReferenceBytes 是 grok 媒体路径压缩核心的字节流版本：
// 超阈值图片降 JPEG 质量到底线再逐级缩尺寸，达标即止；8 轮内仍超标返回
// 最小一次的编码结果（尽力而为）。无法解码（webp/gif 等）返回 false。
func compressImageReferenceBytes(raw []byte, maxBytes int) ([]byte, bool) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	img = fitGrokMediaReferenceDimension(img)
	quality := 85
	var best []byte
	for attempt := 0; attempt < 8; attempt++ {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, false
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
	return best, true
}

// compressOpenAIImagesDataURIReferences 处理 JSON 请求里 data URI 形态的参考图；
// 无图片字段或全部未超限时原样返回。
func compressOpenAIImagesDataURIReferences(body []byte) []byte {
	if !bytes.Contains(body, []byte("data:image/")) || !gjson.ValidBytes(body) {
		return body
	}
	maxBytes := openAIImagesRefMaxBytes()
	changed := false
	for _, field := range []string{"image", "image_url", "images"} {
		value := gjson.GetBytes(body, field)
		if !value.Exists() {
			continue
		}
		if value.IsArray() {
			for index := range value.Array() {
				item := gjson.GetBytes(body, field+"."+strconv.Itoa(index))
				if item.Type != gjson.String {
					continue
				}
				compressed := compressGrokMediaDataURI(item.String(), maxBytes)
				if compressed != item.String() {
					out, err := sjson.SetBytes(body, field+"."+strconv.Itoa(index), compressed)
					if err == nil {
						body = out
						changed = true
					}
				}
			}
			continue
		}
		if value.Type == gjson.String && value.String() != "" {
			compressed := compressGrokMediaDataURI(value.String(), maxBytes)
			if compressed != value.String() {
				out, err := sjson.SetBytes(body, field, compressed)
				if err == nil {
					body = out
					changed = true
				}
			}
		}
	}
	if changed {
		logger.L().Info(
			"openai_images.reference_data_uri_compressed",
			zap.String("component", "service.openai_gateway"),
			zap.Int("bytes_after", len(body)),
		)
	}
	return body
}
