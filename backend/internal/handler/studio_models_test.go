package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type studioModelsUserRepoStub struct{ service.UserRepository }

func (studioModelsUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Status: service.StatusActive}, nil
}

type studioModelsGroupRepoStub struct {
	service.GroupRepository
	groups []service.Group
}

func (s studioModelsGroupRepoStub) ListActive(context.Context) ([]service.Group, error) {
	return s.groups, nil
}

type studioModelsSubscriptionRepoStub struct {
	service.UserSubscriptionRepository
}

func (studioModelsSubscriptionRepoStub) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}

type studioModelForTest struct {
	ID        string `json:"id"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Platform  string `json:"platform"`
}

type studioModelsEndpointForTest struct {
	name            string
	groupPlatform   string
	accountPlatform string
	model           string
	handle          func(*GatewayHandler, *gin.Context, *service.APIKeyService)
}

var studioModelsEndpointsForTest = []studioModelsEndpointForTest{
	{"image/openai", service.PlatformOpenAI, service.PlatformOpenAI, "gpt-image-1", (*GatewayHandler).ImageStudioModels},
	{"image/grok", service.PlatformGrok, service.PlatformGrok, "grok-imagine-image", (*GatewayHandler).ImageStudioModels},
	{"video/grok", service.PlatformGrok, service.PlatformGrok, "grok-imagine-video", (*GatewayHandler).VideoStudioModels},
	{"video/composite", service.PlatformComposite, service.PlatformGrok, "grok-imagine-video", (*GatewayHandler).VideoStudioModels},
}

func listStudioModelsForTest(t *testing.T, endpoint studioModelsEndpointForTest, groups []service.Group, accounts map[int64][]service.Account) []studioModelForTest {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: accounts})
	apiKeys := service.NewAPIKeyService(nil, studioModelsUserRepoStub{}, studioModelsGroupRepoStub{groups: groups}, studioModelsSubscriptionRepoStub{}, nil, nil, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/studio/models", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
	endpoint.handle(h, c, apiKeys)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var response struct {
		Object string               `json:"object"`
		Data   []studioModelForTest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "list", response.Object)
	return response.Data
}

func studioGroupForTest(id int64, name, platform string) service.Group {
	return service.Group{ID: id, Name: name, Platform: platform, Status: service.StatusActive, AllowImageGeneration: true}
}

func TestStudioModels_EmptyGroupDoesNotClaimHealthyGroupModels(t *testing.T) {
	for _, endpoint := range studioModelsEndpointsForTest {
		t.Run(endpoint.name, func(t *testing.T) {
			for _, unavailable := range []struct {
				name     string
				accounts []service.Account
			}{
				{name: "no schedulable accounts"},
				{name: "only unrelated platform", accounts: []service.Account{{ID: 1, Platform: service.PlatformAnthropic}}},
			} {
				t.Run(unavailable.name, func(t *testing.T) {
					groups := []service.Group{
						studioGroupForTest(10, "unavailable", endpoint.groupPlatform),
						studioGroupForTest(20, "healthy", endpoint.groupPlatform),
					}
					// An allowlist on the empty group must not recreate a claim either.
					groups[0].ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{endpoint.model}}
					got := listStudioModelsForTest(t, endpoint, groups, map[int64][]service.Account{
						10: unavailable.accounts,
						20: {{ID: 2, Platform: endpoint.accountPlatform, Credentials: map[string]any{
							"model_mapping": map[string]any{endpoint.model: "upstream-model"},
						}}},
					})
					require.Equal(t, []studioModelForTest{{ID: endpoint.model, GroupID: 20, GroupName: "healthy", Platform: endpoint.groupPlatform}}, got)
				})
			}
		})
	}
}

func TestStudioModels_AllGroupsEmptyReturnsEmptyList(t *testing.T) {
	for _, endpoint := range studioModelsEndpointsForTest {
		t.Run(endpoint.name, func(t *testing.T) {
			got := listStudioModelsForTest(t, endpoint, []service.Group{
				studioGroupForTest(10, "unavailable", endpoint.groupPlatform),
			}, nil)
			require.NotNil(t, got)
			require.Empty(t, got)
		})
	}
}

func TestStudioModels_UnmappedAccountKeepsDefaultsAndAllowlist(t *testing.T) {
	for _, endpoint := range studioModelsEndpointsForTest {
		t.Run(endpoint.name, func(t *testing.T) {
			group := studioGroupForTest(10, "healthy", endpoint.groupPlatform)
			group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{endpoint.model}}
			got := listStudioModelsForTest(t, endpoint, []service.Group{group}, map[int64][]service.Account{
				10: {{ID: 1, Platform: endpoint.accountPlatform}},
			})
			require.Equal(t, []studioModelForTest{{ID: endpoint.model, GroupID: 10, GroupName: "healthy", Platform: endpoint.groupPlatform}}, got)
		})
	}
}
