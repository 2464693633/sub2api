package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"testing"

	"github.com/tidwall/gjson"
)

func buildImagesNoiseJPEG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: uint8((x * 7 + y * 13) % 256), G: uint8((x * 3 + y * 5) % 256), B: uint8((x * 11 + y) % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode noise jpeg: %v", err)
	}
	return buf.Bytes()
}

func buildImagesMultipart(t *testing.T, model string, images ...[]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", model)
	_ = w.WriteField("prompt", "edit this")
	for i, img := range images {
		// 对齐浏览器 FormData.append('image', file)：带 name/filename 与 image/* 内容类型
		hdr := make(map[string][]string)
		hdr["Content-Disposition"] = []string{`form-data; name="image"; filename="ref.jpg"`}
		hdr["Content-Type"] = []string{"image/jpeg"}
		part, err := w.CreatePart(hdr)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		if _, err := part.Write(img); err != nil {
			t.Fatalf("write part %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes(), w.FormDataContentType()
}

func TestCompressOpenAIImagesMultipartReferencesShrinksOversizedPart(t *testing.T) {
	t.Setenv("SUB2API_OPENAI_REF_MAX_BYTES", "")
	big := buildImagesNoiseJPEG(t, 1100)
	if len(big) <= openAIImagesRefDefaultMaxBytes {
		t.Fatalf("fixture too small: %d", len(big))
	}
	body, contentType := buildImagesMultipart(t, "gpt-image-2.5", big)

	out, outType, changed := compressOpenAIImagesMultipartReferences(body, contentType)
	if !changed {
		t.Fatal("oversized reference part was not compressed")
	}
	if len(out) >= len(body) {
		t.Fatalf("compressed body not smaller: %d -> %d", len(body), len(out))
	}
	if outType == "" || !bytes.Contains([]byte(outType), []byte("multipart")) {
		t.Fatalf("invalid content type: %s", outType)
	}
	// 模型字段在压缩后仍应可被 rewrite 正常改写
	rewritten, _, err := rewriteOpenAIImagesModel(out, outType, "mapped-model")
	if err != nil {
		t.Fatalf("rewrite after compress: %v", err)
	}
	if !bytes.Contains(rewritten, []byte("mapped-model")) {
		t.Fatal("model rewrite lost after compression")
	}
}

func TestCompressOpenAIImagesMultipartReferencesKeepsSmallUntouched(t *testing.T) {
	small := buildImagesNoiseJPEG(t, 64)
	body, contentType := buildImagesMultipart(t, "gpt-image-2", small)
	out, _, changed := compressOpenAIImagesMultipartReferences(body, contentType)
	if changed {
		t.Fatal("small reference should pass through")
	}
	if !bytes.Equal(out, body) {
		t.Fatal("small multipart body should be byte-identical")
	}
}

func TestCompressOpenAIImagesDataURIReferences(t *testing.T) {
	big := buildImagesNoiseJPEG(t, 1100)
	if len(big) <= openAIImagesRefDefaultMaxBytes {
		t.Fatalf("fixture too small: %d", len(big))
	}
	body := []byte(`{"model":"gpt-image-2.5","prompt":"edit","image":"data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString(big) + `"}`)
	out := compressOpenAIImagesDataURIReferences(body)
	if string(out) == string(body) {
		t.Fatal("oversized data uri was not compressed")
	}
	decoded := gjson.GetBytes(out, "image").String()
	if len(decoded) >= len(gjson.GetBytes(body, "image").String()) {
		t.Fatalf("data uri not smaller after compression")
	}
	smallBody := []byte(`{"model":"gpt-image-2","prompt":"t","image":"data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString(buildImagesNoiseJPEG(t, 32)) + `"}`)
	if got := compressOpenAIImagesDataURIReferences(smallBody); string(got) != string(smallBody) {
		t.Fatal("small data uri should pass through")
	}
}
