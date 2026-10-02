package routes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/creativestudio"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func creativeTestPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestCreativeGatewayAuthorizationErrorsAndResponseLimit(t *testing.T) {
	e := &creativeStudioExecutor{gateway: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer unit-test-key" || r.RemoteAddr != "127.0.0.1:0" {
			t.Error("gateway authentication context was not supplied")
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"group rate limit"}}`))
	})}
	_, _, err := e.gatewayRequest(context.Background(), http.MethodPost, "/v1/images/generations", "unit-test-key", "application/json", strings.NewReader(`{}`), 1024)
	var gatewayError *creativeGatewayError
	if !errors.As(err, &gatewayError) || gatewayError.status != 429 || err.Error() != "group rate limit" {
		t.Fatalf("gateway error was hidden: %v", err)
	}
	if errors.Is(creativeSubmissionError(err), creativestudio.ErrUncertain) {
		t.Fatal("known rejection was marked as an unknown submission")
	}
	e.gateway = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("123456789")) })
	if _, _, err = e.gatewayRequest(context.Background(), http.MethodGet, "/test", "unit-test-key", "", nil, 4); err == nil {
		t.Fatal("oversized gateway response accepted")
	}
	if !errors.Is(creativeSubmissionError(context.DeadlineExceeded), creativestudio.ErrUncertain) {
		t.Fatal("timed-out submission could be retried as known failure")
	}
}

func TestCreativeImageGenerationAndReferenceMIME(t *testing.T) {
	data := creativeTestPNG(t)
	for _, withReference := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "edit"}[withReference], func(t *testing.T) {
			e := &creativeStudioExecutor{gateway: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if withReference {
					if r.URL.Path != "/v1/images/edits" {
						t.Errorf("wrong edit path: %s", r.URL.Path)
					}
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Fatal(err)
					}
					defer func() { _ = r.MultipartForm.RemoveAll() }()
					file, header, err := r.FormFile("image")
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = file.Close() }()
					if header.Header.Get("Content-Type") != "image/png" {
						t.Errorf("reference MIME lost: %+v", header.Header)
					}
					if payload, _ := io.ReadAll(file); !bytes.Equal(payload, data) {
						t.Error("reference content changed")
					}
					if r.FormValue("prompt") != "Keep this product" || r.FormValue("size") != "1024x1024" {
						t.Error("edit parameters lost")
					}
				} else {
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if r.URL.Path != "/v1/images/generations" || payload["n"] != float64(1) || payload["prompt"] != "Keep this product" {
						t.Errorf("incorrect generation request: %s %+v", r.URL.Path, payload)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
			})}
			var refs []creativestudio.AssetFile
			if withReference {
				path := filepath.Join(t.TempDir(), "reference")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				refs = []creativestudio.AssetFile{{Asset: creativestudio.Asset{ID: "ref", Kind: "image", MIME: "image/png", Size: int64(len(data))}, Path: path}}
			}
			result, err := e.image(context.Background(), creativestudio.Job{ID: "job", Prompt: "Keep this product"}, creativeGenerationParams{Model: "gpt-image-2", Size: "1024x1024", Platform: service.PlatformOpenAI}, "test-key", refs)
			if err != nil || result.MIME != "image/png" || !bytes.Equal(result.Data, data) {
				t.Fatalf("image result failed: %+v %v", result, err)
			}
		})
	}
}

func TestCreativeVideoPersistsIDBeforePollingAndResumesWithoutPOST(t *testing.T) {
	media := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
	for _, resuming := range []bool{false, true} {
		t.Run(map[bool]string{false: "submit", true: "resume"}[resuming], func(t *testing.T) {
			paths := make([]string, 0)
			persisted := resuming
			e := &creativeStudioExecutor{gateway: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.Method+" "+r.URL.Path)
				switch r.URL.Path {
				case "/v1/videos":
					_, _ = w.Write([]byte(`{"id":"remote-123"}`))
				case "/v1/videos/remote-123":
					if !persisted {
						t.Error("video polled before its ID was durably recorded")
					}
					_, _ = w.Write([]byte(`{"status":"completed"}`))
				case "/v1/videos/remote-123/content":
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = w.Write(media)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			})}
			job := creativestudio.Job{ID: "job", Kind: "video", Prompt: "Pan slowly"}
			if resuming {
				job.RemoteID = "remote-123"
			}
			result, err := e.video(context.Background(), job, creativeGenerationParams{Model: "video", Seconds: 6}, "test-key", nil, func(id string, terminal bool) error {
				if id != "remote-123" || terminal {
					t.Error("incorrect remote task state")
				}
				persisted = true
				return nil
			})
			if err != nil || !bytes.Equal(result.Data, media) {
				t.Fatalf("video failed: %v", err)
			}
			expected := []string{"GET /v1/videos/remote-123", "GET /v1/videos/remote-123/content"}
			if !resuming {
				expected = append([]string{"POST /v1/videos"}, expected...)
			}
			if !reflect.DeepEqual(paths, expected) {
				t.Fatalf("duplicate submission or incorrect flow: %+v", paths)
			}
		})
	}
}

func TestCreativeVideoTerminalAndDownloadFailures(t *testing.T) {
	for _, terminalStatus := range []string{"failed", "expired", "cancelled", "canceled"} {
		t.Run(terminalStatus, func(t *testing.T) {
			marked := false
			e := &creativeStudioExecutor{gateway: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"status": terminalStatus})
			})}
			_, err := e.video(context.Background(), creativestudio.Job{RemoteID: "remote"}, creativeGenerationParams{}, "key", nil, func(id string, terminal bool) error { marked = id == "remote" && terminal; return nil })
			if err == nil || !marked {
				t.Fatalf("terminal failure was not persisted: marked=%v error=%v", marked, err)
			}
		})
	}
	t.Run("download", func(t *testing.T) {
		e := &creativeStudioExecutor{gateway: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/content") {
				w.WriteHeader(502)
				return
			}
			_, _ = w.Write([]byte(`{"status":"completed"}`))
		})}
		_, err := e.video(context.Background(), creativestudio.Job{RemoteID: "remote"}, creativeGenerationParams{}, "key", nil, func(string, bool) error {
			t.Error("download failure must not mark the remote task terminal")
			return nil
		})
		if err == nil || errors.Is(err, creativestudio.ErrUncertain) {
			t.Fatalf("download failure should remain resumable: %v", err)
		}
	})
	t.Run("missing submission ID", func(t *testing.T) {
		e := &creativeStudioExecutor{gateway: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) })}
		_, err := e.video(context.Background(), creativestudio.Job{}, creativeGenerationParams{Seconds: 6}, "key", nil, func(string, bool) error { return nil })
		if !errors.Is(err, creativestudio.ErrUncertain) {
			t.Fatalf("unknown submission should require conscious retry: %v", err)
		}
	})
}
