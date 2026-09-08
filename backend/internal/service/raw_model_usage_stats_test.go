//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type rawModelUsageStatsStub struct {
	UsageLogRepository
	rawCalled bool
}

func (s *rawModelUsageStatsStub) GetRawModelStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.ModelStat, error) {
	s.rawCalled = true
	return []usagestats.ModelStat{{Model: "raw", TotalTokens: 17}}, nil
}

func TestGetRawAccountModelStatsUsesRawProvider(t *testing.T) {
	repo := &rawModelUsageStatsStub{}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	stats, err := getRawAccountModelStats(context.Background(), repo, start, start.Add(time.Hour), 42)

	require.NoError(t, err)
	require.True(t, repo.rawCalled)
	require.Equal(t, int64(17), stats[0].TotalTokens)
}
