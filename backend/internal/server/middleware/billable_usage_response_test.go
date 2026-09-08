//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func billableResponseTestGroup() *service.Group {
	return &service.Group{
		InputTokenMultiplier: 2, OutputTokenMultiplier: 3,
		CacheCreationTokenMultiplier: 4, CacheReadTokenMultiplier: 5,
		ReturnBillableUsage: true, TokenMultipliersConfigured: true,
	}
}

func TestRewriteBillableUsageJSON_OpenAIResponses(t *testing.T) {
	payload := []byte(`{"id":"r","usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110,"input_tokens_details":{"cached_tokens":20,"cache_write_tokens":5}}}`)
	rewritten, changed := rewriteBillableUsageJSON(payload, billableResponseTestGroup())
	require.True(t, changed)
	require.JSONEq(t, `{"id":"r","usage":{"input_tokens":270,"output_tokens":30,"total_tokens":300,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":20}}}`, string(rewritten))
}

func TestRewriteBillableUsageJSON_ProviderSpecificDetails(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "anthropic cache creation breakdown",
			in:   `{"usage":{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}}}`,
			want: `{"usage":{"input_tokens":20,"output_tokens":6,"cache_creation_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":8,"ephemeral_1h_input_tokens":12}}}`,
		},
		{
			name: "kimi cache aliases",
			in:   `{"usage":{"input_tokens":1200,"prompt_tokens":1200,"output_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}}`,
			want: `{"usage":{"input_tokens":4800,"prompt_tokens":4800,"output_tokens":90,"prompt_cache_hit_tokens":4000,"prompt_cache_miss_tokens":800}}`,
		},
		{
			name: "kimi delta uncached input",
			in:   `{"usage":{"input_tokens":400,"prompt_tokens":1200,"output_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}}`,
			want: `{"usage":{"input_tokens":800,"prompt_tokens":4800,"output_tokens":90,"prompt_cache_hit_tokens":4000,"prompt_cache_miss_tokens":800}}`,
		},
		{
			name: "kimi delta without miss alias",
			in:   `{"usage":{"input_tokens":250,"prompt_tokens":173306,"output_tokens":166,"cache_read_input_tokens":173056,"cached_tokens":173056}}`,
			want: `{"usage":{"input_tokens":500,"prompt_tokens":865780,"output_tokens":498,"cache_read_input_tokens":865280,"cached_tokens":865280}}`,
		},
		{
			name: "xai independent reasoning",
			in:   `{"usage":{"input_tokens":32,"output_tokens":9,"total_tokens":135,"output_tokens_details":{"reasoning_tokens":94}}}`,
			want: `{"usage":{"input_tokens":64,"output_tokens":309,"total_tokens":373,"output_tokens_details":{"reasoning_tokens":282}}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rewritten, changed := rewriteBillableUsageJSON([]byte(test.in), billableResponseTestGroup())
			require.True(t, changed)
			require.JSONEq(t, test.want, string(rewritten))
		})
	}
}

func TestRewriteBillableUsageJSON_RejectsTrailingJSONValue(t *testing.T) {
	payload := []byte(`{"usage":{"input_tokens":1}} {"usage":{"input_tokens":2}}`)
	rewritten, changed := rewriteBillableUsageJSON(payload, billableResponseTestGroup())
	require.False(t, changed)
	require.Equal(t, payload, rewritten)
}

func TestRewriteBillableUsageJSON_DoesNotRewriteToolArguments(t *testing.T) {
	payload := []byte(`{"usage":{"input_tokens":2,"output_tokens":1},"response":{"output":[{"type":"function_call","arguments":{"usage":{"input_tokens":7,"output_tokens":8}}}]}}`)

	rewritten, changed := rewriteBillableUsageJSON(payload, billableResponseTestGroup())

	require.True(t, changed)
	require.JSONEq(t, `{"usage":{"input_tokens":4,"output_tokens":3},"response":{"output":[{"type":"function_call","arguments":{"usage":{"input_tokens":7,"output_tokens":8}}}]}}`, string(rewritten))
}

func TestRewriteBillableUsageJSON_RewritesKnownDataWrappers(t *testing.T) {
	payload := []byte(`{"data":{"usage":{"input_tokens":2,"output_tokens":1},"response":{"usage":{"input_tokens":3,"output_tokens":2}}}}`)

	rewritten, changed := rewriteBillableUsageJSON(payload, billableResponseTestGroup())

	require.True(t, changed)
	require.JSONEq(t, `{"data":{"usage":{"input_tokens":4,"output_tokens":3},"response":{"usage":{"input_tokens":6,"output_tokens":6}}}}`, string(rewritten))
}

func TestRewriteBillableUsageJSON_ForceCacheBilling(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "anthropic",
			in:   `{"usage":{"input_tokens":10,"output_tokens":2}}`,
			want: `{"usage":{"input_tokens":0,"output_tokens":6,"cache_read_input_tokens":50}}`,
		},
		{
			name: "gemini",
			in:   `{"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":13}}`,
			want: `{"usageMetadata":{"promptTokenCount":50,"cachedContentTokenCount":50,"candidatesTokenCount":9,"totalTokenCount":59}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rewritten, changed := rewriteBillableUsageJSON([]byte(test.in), billableResponseTestGroup(), true)
			require.True(t, changed)
			require.JSONEq(t, test.want, string(rewritten))
		})
	}
}

func TestRewriteBillableUsageJSON_AnthropicAndGemini(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "anthropic",
			in:   `{"usage":{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":3,"cache_read_input_tokens":4}}`,
			want: `{"usage":{"input_tokens":20,"output_tokens":6,"cache_creation_input_tokens":12,"cache_read_input_tokens":20}}`,
		},
		{
			name: "gemini",
			in:   `{"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":2,"candidatesTokenCount":3,"thoughtsTokenCount":1,"totalTokenCount":14,"promptTokensDetails":[{"modality":"TEXT","tokenCount":7},{"modality":"IMAGE","tokenCount":3}],"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":2},{"modality":"IMAGE","tokenCount":1}]}}`,
			want: `{"usageMetadata":{"promptTokenCount":26,"cachedContentTokenCount":10,"candidatesTokenCount":9,"thoughtsTokenCount":3,"totalTokenCount":38,"promptTokensDetails":[{"modality":"TEXT","tokenCount":18},{"modality":"IMAGE","tokenCount":8}],"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":6},{"modality":"IMAGE","tokenCount":3}]}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rewritten, changed := rewriteBillableUsageJSON([]byte(test.in), billableResponseTestGroup())
			require.True(t, changed)
			require.JSONEq(t, test.want, string(rewritten))
		})
	}
}

func TestBillableUsageWriter_ReassemblesSplitSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	base, _ := gin.CreateTestContext(recorder)
	base.Header("Content-Type", "text/event-stream")
	w := &billableUsageWriter{ResponseWriter: base.Writer, group: billableResponseTestGroup()}

	first := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":10,"
	second := "\"output_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"
	_, err := w.Write([]byte(first))
	require.NoError(t, err)
	require.Empty(t, recorder.Body.String())
	_, err = w.Write([]byte(second))
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"input_tokens":23`)
	require.Contains(t, recorder.Body.String(), `"output_tokens":6`)
	require.Contains(t, recorder.Body.String(), `"cached_tokens":5`)
}

func TestRewriteBillableSSEEvent_RewritesLargeFinalEvent(t *testing.T) {
	payload := `{"type":"response.completed","response":{"output_text":"` +
		strings.Repeat("x", 70*1024) +
		`","usage":{"input_tokens":10,"output_tokens":2}}}`
	event := []byte("event: response.completed\r\ndata: " + payload)

	rewritten := rewriteBillableSSEEvent(event, billableResponseTestGroup())

	require.Contains(t, string(rewritten), `"input_tokens":20`)
	require.Contains(t, string(rewritten), `"output_tokens":6`)
	require.Contains(t, string(rewritten), "\r\n")
}

func TestBillableUsageWriter_PassesThroughOversizedUnframedSSEEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	base, _ := gin.CreateTestContext(recorder)
	base.Header("Content-Type", "text/event-stream")
	w := &billableUsageWriter{
		ResponseWriter: base.Writer,
		group:          billableResponseTestGroup(),
		pending:        make([]byte, maxBillableUsageSSEEventBytes),
	}

	written, err := w.Write([]byte{'x'})

	require.NoError(t, err)
	require.Equal(t, 1, written)
	require.Empty(t, w.pending)
	require.True(t, w.ssePassthrough)
	require.Equal(t, maxBillableUsageSSEEventBytes+1, recorder.Body.Len())
}

func TestBillableUsageResponse_OptInAndContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := []byte(`{"usage":{"input_tokens":10,"output_tokens":2}}`)

	for _, test := range []struct {
		name            string
		group           *service.Group
		wantInput       string
		wantContentSize string
	}{
		{
			name:            "disabled returns raw usage",
			group:           &service.Group{},
			wantInput:       `"input_tokens":10`,
			wantContentSize: strconv.Itoa(len(payload)),
		},
		{
			name:      "enabled returns billable usage",
			group:     billableResponseTestGroup(),
			wantInput: `"input_tokens":20`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: test.group})
				c.Next()
			})
			router.Use(BillableUsageResponse())
			router.GET("/", func(c *gin.Context) {
				c.Header("Content-Type", "application/json")
				c.Header("Content-Length", strconv.Itoa(len(payload)))
				c.Data(http.StatusOK, "application/json", payload)
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), test.wantInput)
			require.Equal(t, test.wantContentSize, recorder.Header().Get("Content-Length"))
		})
	}
}

func TestBillableUsageWriter_UsesFinalBillingGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Header("Content-Type", "application/json")
	w := &billableUsageWriter{
		ResponseWriter: context.Writer,
		context:        context,
		group:          &service.Group{},
	}

	SetBillableUsageResponseGroup(context, billableResponseTestGroup())
	_, err := w.Write([]byte(`{"usage":{"input_tokens":10,"output_tokens":2}}`))

	require.NoError(t, err)
	require.JSONEq(t, `{"usage":{"input_tokens":20,"output_tokens":6}}`, recorder.Body.String())
}
