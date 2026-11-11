package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestKlingModelsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OpenAIGatewayHandler{}
	r := gin.New()
	r.GET("/models", h.KlingModels)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/models", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Body.String(); got == "" || !strings.Contains(got, "kling-v3") {
		t.Fatalf("model catalog missing kling-v3: %s", got)
	}
}
