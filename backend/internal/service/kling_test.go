package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestBuildKlingJWTClaims(t *testing.T) {
	now := time.Now().UTC()
	token, err := BuildKlingJWT("ak", "sk", now)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			t.Fatalf("unexpected method %v", token.Method)
		}
		return []byte("sk"), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("invalid token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["iss"] != "ak" {
		t.Fatalf("issuer = %v", claims["iss"])
	}
	if claims["exp"].(float64) != float64(now.Add(30*time.Minute).Unix()) {
		t.Fatalf("unexpected exp: %v", claims["exp"])
	}
}

func TestKlingStatusAndResultParsing(t *testing.T) {
	traditional := []byte(`{"code":0,"data":{"task_id":"t1","task_status":"succeed","task_result":{"videos":[{"url":"https://video"}]}}}`)
	got := ParseKlingTaskResult(traditional, KlingEndpointQueryVideo)
	if got.TaskID != "t1" || got.Status != "succeed" || got.URL != "https://video" || got.VideoCount != 1 {
		t.Fatalf("unexpected traditional result: %+v", got)
	}
	four := []byte(`{"code":0,"data":[{"id":"t2","status":"succeeded","outputs":[{"type":"video","url":"https://v4"}]}]}`)
	got = ParseKlingTaskResult(four, KlingEndpointQueryKling4)
	if got.TaskID != "t2" || got.Status != "succeed" || got.URL != "https://v4" {
		t.Fatalf("unexpected v4 result: %+v", got)
	}
}

func TestForwardKlingJWTAndErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing bearer token")
		}
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"img-1"}}`))
	}))
	defer server.Close()
	account := &Account{Platform: PlatformKling, Credentials: map[string]any{"base_url": server.URL, "api_key": "ak|sk"}}
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		result, err := (&OpenAIGatewayService{}).ForwardKling(c.Request.Context(), c, account, KlingEndpointImagesGeneration, "", []byte(`{"model_name":"kling-v3","prompt":"x"}`))
		if err != nil {
			t.Fatal(err)
		}
		if result.TaskID != "img-1" {
			t.Fatalf("task = %+v", result)
		}
	})
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	_ = json.Valid(rec.Body.Bytes())
}

func TestKlingErrorHTTPStatus(t *testing.T) {
	for code, want := range map[int]int{1001: 401, 1102: 402, 1201: 400, 1203: 404, 1302: 429, 5000: 502} {
		if got := KlingErrorHTTPStatus(code); got != want {
			t.Errorf("%d => %d, want %d", code, got, want)
		}
	}
}
