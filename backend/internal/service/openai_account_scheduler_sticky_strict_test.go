package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// newStickyStrictTestService 构造带粘性/严格优先级开关设置的调度测试服务。
// toggles: [0]=session_sticky, [1]=previous_response_sticky, [2]=strict_priority（空字符串=不写键）。
func newStickyStrictTestService(accounts []Account, cache *schedulerTestGatewayCache, toggles ...string) *OpenAIGatewayService {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	repo := &openAIAdvancedSchedulerSettingRepoStub{
		values: map[string]string{
			openAIAdvancedSchedulerSettingKey: "true",
		},
	}
	if len(toggles) > 0 && toggles[0] != "" {
		repo.values[SettingKeyOpenAISessionStickyEnabled] = toggles[0]
	}
	if len(toggles) > 1 && toggles[1] != "" {
		repo.values[SettingKeyOpenAIPreviousResponseStickyEnabled] = toggles[1]
	}
	if len(toggles) > 2 && toggles[2] != "" {
		repo.values[SettingKeyOpenAIStrictPriorityEnabled] = toggles[2]
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 3
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 0.8
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 0.5
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.SessionSticky = 3
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   &RateLimitService{settingService: NewSettingService(repo, cfg)},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	return svc
}

func TestOpenAIAdvancedSchedulerRuntimeSettings_StickyAndStrictDefaults(t *testing.T) {
	svc := newStickyStrictTestService(nil, &schedulerTestGatewayCache{})
	ctx := context.Background()
	// 存量部署 settings 表缺键：粘性必须默认开启，严格优先级默认关闭。
	require.True(t, svc.isOpenAISessionStickyEnabled(ctx))
	require.True(t, svc.isOpenAIPreviousResponseStickyEnabled(ctx))
	require.False(t, svc.isOpenAIStrictPriorityEnabled(ctx))
}

func TestOpenAIAdvancedSchedulerRuntimeSettings_StickyOffStrictOn(t *testing.T) {
	svc := newStickyStrictTestService(nil, &schedulerTestGatewayCache{}, "false", "false", "true")
	ctx := context.Background()
	require.False(t, svc.isOpenAISessionStickyEnabled(ctx))
	require.False(t, svc.isOpenAIPreviousResponseStickyEnabled(ctx))
	require.True(t, svc.isOpenAIStrictPriorityEnabled(ctx))
}

func TestOpenAISessionStickySwitch_GatesCacheReadsAndWrites(t *testing.T) {
	ctx := context.Background()

	// 开关关闭：预置绑定也读不到，写入是 no-op。
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"openai:hash_off": 9001,
	}}
	svc := newStickyStrictTestService(nil, cache, "false")
	got, err := svc.getStickySessionAccountID(ctx, nil, "hash_off")
	require.NoError(t, err)
	require.Zero(t, got)
	require.NoError(t, svc.setStickySessionAccountID(ctx, nil, "hash_new", 9002, time.Hour))
	require.NotContains(t, cache.sessionBindings, "openai:hash_new")

	// 开关开启：读恢复正常，写入生效。
	cacheOn := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"openai:hash_on": 9001,
	}}
	svcOn := newStickyStrictTestService(nil, cacheOn)
	got, err = svcOn.getStickySessionAccountID(ctx, nil, "hash_on")
	require.NoError(t, err)
	require.Equal(t, int64(9001), got)
	require.NoError(t, svcOn.setStickySessionAccountID(ctx, nil, "hash_new", 9002, time.Hour))
	require.Equal(t, int64(9002), cacheOn.sessionBindings["openai:hash_new"])
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyOffIgnoresStickyBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(101081)
	accounts := []Account{
		{
			ID: 37201, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Priority: 100, GroupIDs: []int64{groupID},
		},
		{
			ID: 37202, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Priority: 0, GroupIDs: []int64{groupID},
		},
	}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"openai:hash_sticky_off": 37201,
	}}

	// 粘性开启：命中绑定，选高优先级数字的 37201（旧行为）。
	svc := newStickyStrictTestService(accounts, cache)
	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "hash_sticky_off", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(37201), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	// 粘性关闭：忽略绑定，按优先级选 37202。
	svcOff := newStickyStrictTestService(accounts, cache, "false")
	selection, _, err = svcOff.SelectAccountWithScheduler(ctx, &groupID, "", "hash_sticky_off", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(37202), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_StrictPriorityDeterministicOrder(t *testing.T) {
	ctx := context.Background()
	groupID := int64(101082)
	// priority 0 最优但名字在最后；非严格模式下 topK=3 加权随机可能选中任何候选，
	// 严格模式必须稳定选中 priority 0 且 TopK 覆盖全部候选。
	accounts := []Account{
		{
			ID: 37301, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Priority: 5, GroupIDs: []int64{groupID},
		},
		{
			ID: 37302, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Priority: 1, GroupIDs: []int64{groupID},
		},
		{
			ID: 37303, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Priority: 0, GroupIDs: []int64{groupID},
		},
	}
	for round := 0; round < 5; round++ {
		svc := newStickyStrictTestService(accounts, &schedulerTestGatewayCache{}, "", "", "true")
		selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, int64(37303), selection.Account.ID, "strict priority must select the lowest-priority-number account")
		require.Equal(t, 3, decision.TopK, "strict priority considers all candidates")
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}
}

func TestSortOpenAIAccountCandidatesByStrictPriority(t *testing.T) {
	now := time.Now()
	old := now.Add(-time.Hour)
	pool := []openAIAccountCandidateScore{
		{account: &Account{ID: 1, Priority: 1}, loadInfo: &AccountLoadInfo{LoadRate: 0, WaitingCount: 0}},
		{account: &Account{ID: 2, Priority: 0}, loadInfo: &AccountLoadInfo{LoadRate: 50, WaitingCount: 0}},
		{account: &Account{ID: 3, Priority: 0}, loadInfo: &AccountLoadInfo{LoadRate: 0, WaitingCount: 5}},
		{account: &Account{ID: 4, Priority: 0}, loadInfo: &AccountLoadInfo{LoadRate: 0, WaitingCount: 0}, loadKnown: true},
		{account: &Account{ID: 5, Priority: 0}, loadInfo: &AccountLoadInfo{LoadRate: 0, WaitingCount: 0, AccountID: 5}, loadKnown: true},
	}
	// 4 与 5 负载/排队相同，用 LastUsedAt 区分：从未用过的 4 在前。
	pool[3].account.LastUsedAt = &old
	pool[4].account.LastUsedAt = &now
	pool[2].account.LastUsedAt = &old

	sortOpenAIAccountCandidatesByStrictPriority(pool)
	got := make([]int64, 0, len(pool))
	for _, candidate := range pool {
		got = append(got, candidate.account.ID)
	}
	// 优先级 0 的候选按（负载率 → 排队数 → LRU）排在前；优先级 1 的垫底。
	require.Equal(t, []int64{4, 5, 3, 2, 1}, got)
}

func TestParseSettingExplicitlyFalse(t *testing.T) {
	require.True(t, parseSettingExplicitlyFalse("false"))
	require.True(t, parseSettingExplicitlyFalse(" FALSE "))
	require.False(t, parseSettingExplicitlyFalse(""))
	require.False(t, parseSettingExplicitlyFalse("true"))
	require.False(t, parseSettingExplicitlyFalse("unexpected"))
}
