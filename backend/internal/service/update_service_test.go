//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
	latestRepo     string
	recentRepo     string
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.latestRepo = repo
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(_ context.Context, repo string, _ int) ([]*GitHubRelease, error) {
	s.recentRepo = repo
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
		defaultUpdateRepository,
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
		defaultUpdateRepository,
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
		defaultUpdateRepository,
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}

func TestUpdateServiceUsesConfiguredRepository(t *testing.T) {
	t.Setenv("UPDATE_GITHUB_REPO", "")
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v1.1.0"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.0.0", "release", "owner/custom-sub2api")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, "owner/custom-sub2api", client.latestRepo)
	require.Equal(t, "owner/custom-sub2api", info.Repository)
	require.True(t, info.HasUpdate)
}

func TestUpdateServiceRuntimeRepositoryOverridesBuildDefault(t *testing.T) {
	t.Setenv("UPDATE_GITHUB_REPO", " runtime-owner/runtime-repo ")
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v1.0.0"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.0.0", "release", "build-owner/build-repo")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, "runtime-owner/runtime-repo", client.latestRepo)
	require.Equal(t, "runtime-owner/runtime-repo", info.Repository)
}

func TestUpdateServiceInvalidRuntimeRepositoryFallsBackToBuildDefault(t *testing.T) {
	t.Setenv("UPDATE_GITHUB_REPO", "https://github.com/attacker/repo")
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v1.0.0"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.0.0", "release", "build-owner/build-repo")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, "build-owner/build-repo", client.latestRepo)
	require.Equal(t, "build-owner/build-repo", info.Repository)
}

func TestUpdateServiceInvalidRepositoriesFallBackToForkDefault(t *testing.T) {
	t.Setenv("UPDATE_GITHUB_REPO", "../runtime-repo")
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v1.0.0"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.0.0", "release", "owner/repo/extra")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, defaultUpdateRepository, client.latestRepo)
	require.Equal(t, defaultUpdateRepository, info.Repository)
}

func TestNormalizeUpdateRepository(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{input: " Acme/Sub2API ", valid: true},
		{input: "owner/repo.name_1-rc", valid: true},
		{input: "https://github.com/owner/repo", valid: false},
		{input: "owner/repo/extra", valid: false},
		{input: "../repo", valid: false},
		{input: "owner/..", valid: false},
		{input: "owner/repo;whoami", valid: false},
		{input: "owner\\repo", valid: false},
		{input: "", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, valid := normalizeUpdateRepository(tt.input)
			require.Equal(t, tt.valid, valid)
		})
	}
}

func TestUpdateServiceUsesForkFallbackRepository(t *testing.T) {
	t.Setenv("UPDATE_GITHUB_REPO", "")
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v1.0.0"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.0.0", "release", "")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, defaultUpdateRepository, client.latestRepo)
	require.Equal(t, defaultUpdateRepository, info.Repository)
}

func TestUpdateServiceCacheIsScopedToRepository(t *testing.T) {
	t.Setenv("UPDATE_GITHUB_REPO", "")
	cache := &updateServiceCacheStub{}
	firstClient := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v1.1.0"}}
	first := NewUpdateService(cache, firstClient, "1.0.0", "release", "owner/first")
	_, err := first.CheckUpdate(context.Background(), true)
	require.NoError(t, err)

	secondClient := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v2.0.0"}}
	second := NewUpdateService(cache, secondClient, "1.0.0", "release", "owner/second")
	info, err := second.CheckUpdate(context.Background(), false)

	require.NoError(t, err)
	require.Equal(t, "owner/second", secondClient.latestRepo)
	require.Equal(t, "owner/second", info.Repository)
	require.Equal(t, "2.0.0", info.LatestVersion)
}

func TestCompareVersionsSemVer(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		latest   string
		expected int
	}{
		{name: "patch", current: "1.2.3", latest: "1.2.4", expected: -1},
		{name: "optional v prefix", current: "v1.2.3", latest: "1.2.3", expected: 0},
		{name: "numeric fork suffix", current: "1.2.3-custom.9", latest: "1.2.3-custom.10", expected: -1},
		{name: "stable follows fork prerelease", current: "1.2.3-custom.10", latest: "1.2.3", expected: -1},
		{name: "build metadata ignored", current: "1.2.3+fork.1", latest: "1.2.3+fork.2", expected: 0},
		{name: "valid latest follows invalid development version", current: "development", latest: "1.0.0", expected: -1},
		{name: "invalid latest rejected", current: "1.0.0", latest: "not-a-version", expected: 1},
		{name: "two invalid versions cannot be ordered", current: "development", latest: "nightly", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, compareVersions(tt.current, tt.latest))
		})
	}
}

func TestUpdateServiceRollbackVersionsSkipInvalidTags(t *testing.T) {
	t.Setenv("UPDATE_GITHUB_REPO", "")
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{
		{TagName: "release-old"},
		{TagName: "v1.1.9-custom.2"},
		{TagName: "v1.1.9-custom.1"},
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "1.2.0-custom.1", "release", "owner/repo")

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Equal(t, "owner/repo", client.recentRepo)
	require.Equal(t, []RollbackVersion{
		{Version: "1.1.9-custom.2"},
		{Version: "1.1.9-custom.1"},
	}, versions)
}
