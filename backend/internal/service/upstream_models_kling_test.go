package service_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type klingModelsHTTPUpstream struct {
	service.HTTPUpstream
}

func klingModelSyncTestConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true},
	}}
}

func (u *klingModelsHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func TestKlingUpstreamModelSync(t *testing.T) {
	for _, tc := range []struct {
		name, body, key string
		status          int
		kind            service.UpstreamModelSyncErrorKind
	}{
		{name: "success", status: 200, key: "ak|sk", body: `{"code":0,"data":{"models":[{"id":" kling-v4 "},{"id":"kling-v4-flash"},{"id":"kling-v4"},{"id":""}]}}`},
		{name: "separate credentials", status: 200, body: `{"code":0,"data":{"models":[{"id":"kling-v4"},{"id":"kling-v4-flash"}]}}`},
		{name: "not found must not use configured models", status: 404, key: "ak|sk", body: `{}`, kind: service.UpstreamModelSyncErrorUpstream},
		{name: "unauthorized", status: 401, key: "ak|sk", body: `{"code":1001}`, kind: service.UpstreamModelSyncErrorUpstream},
		{name: "bad credentials", status: 200, key: "bad", body: `{}`, kind: service.UpstreamModelSyncErrorConfiguration},
		{name: "empty secret", status: 200, key: "ak| ", body: `{}`, kind: service.UpstreamModelSyncErrorConfiguration},
		{name: "empty models", status: 200, key: "ak|sk", body: `{"code":0,"data":{"models":[]}}`, kind: service.UpstreamModelSyncErrorUpstream},
		{name: "blank ids", status: 200, key: "ak|sk", body: `{"code":0,"data":{"models":[{"id":" "}]}}`, kind: service.UpstreamModelSyncErrorUpstream},
		{name: "error envelope", status: 200, key: "ak|sk", body: `{"code":1001,"message":"secret","data":{"models":[{"id":"kling-v4"}]}}`, kind: service.UpstreamModelSyncErrorUpstream},
		{name: "missing code", status: 200, key: "ak|sk", body: `{"data":{"models":[{"id":"kling-v4"}]}}`, kind: service.UpstreamModelSyncErrorUpstream},
		{name: "malformed response", status: 200, key: "ak|sk", body: `{`, kind: service.UpstreamModelSyncErrorUpstream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/v1/models", r.URL.Path)
				require.Equal(t, "application/json", r.Header.Get("Accept"))
				require.Empty(t, r.Header.Get("Content-Type"))
				token, err := jwt.Parse(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), func(token *jwt.Token) (any, error) {
					require.Equal(t, jwt.SigningMethodHS256, token.Method)
					return []byte("sk"), nil
				}, jwt.WithIssuer("ak"))
				require.NoError(t, err)
				require.True(t, token.Valid)
				claims := token.Claims.(jwt.MapClaims)
				require.InDelta(t, time.Now().Add(30*time.Minute).Unix(), claims["exp"], 2)
				require.InDelta(t, time.Now().Add(-5*time.Second).Unix(), claims["nbf"], 2)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			cfg := klingModelSyncTestConfig()
			svc := service.NewAccountTestService(nil, nil, nil, nil, nil, &klingModelsHTTPUpstream{}, cfg, nil)
			account := &service.Account{ID: 1, Platform: service.PlatformKling, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
				"base_url": server.URL + "/v1/", "api_key": tc.key,
				"model_mapping": map[string]any{"old": "old"},
			}}
			if tc.key == "" {
				account.Credentials["access_key"] = "ak"
				account.Credentials["secret_key"] = "sk"
			}
			catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
			if tc.kind != "" {
				var syncErr *service.UpstreamModelSyncError
				require.ErrorAs(t, err, &syncErr)
				require.Equal(t, tc.kind, syncErr.Kind)
				require.Nil(t, catalog)
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{"kling-v4", "kling-v4-flash"}, catalog.Models)
				require.Empty(t, catalog.Warnings)
				require.Equal(t, "upstream", catalog.DiscoverySource)
			}
			if tc.kind == service.UpstreamModelSyncErrorConfiguration {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls, "Kling must not call the capability registry")
			}
			require.Equal(t, map[string]any{"old": "old"}, account.Credentials["model_mapping"])
		})
	}
}

func TestKlingUpstreamModelSyncValidatesURL(t *testing.T) {
	cfg := klingModelSyncTestConfig()
	cfg.Security.URLAllowlist.AllowInsecureHTTP = false
	svc := service.NewAccountTestService(nil, nil, nil, nil, nil, &klingModelsHTTPUpstream{}, cfg, nil)
	for _, baseURL := range []string{"file:///tmp/models", "http://127.0.0.1:8020"} {
		_, err := svc.FetchUpstreamSupportedModels(context.Background(), &service.Account{
			Platform: service.PlatformKling, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": baseURL, "api_key": "ak|sk"},
		})
		var syncErr *service.UpstreamModelSyncError
		require.ErrorAs(t, err, &syncErr)
		require.Equal(t, service.UpstreamModelSyncErrorConfiguration, syncErr.Kind)
	}
}
