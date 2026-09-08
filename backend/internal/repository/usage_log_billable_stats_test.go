//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestUsageLogTokenExprUsesBillableSnapshotWithHistoricalFallback(t *testing.T) {
	require.Equal(t,
		"COALESCE(ul.billable_input_tokens, ul.input_tokens)",
		usageLogTokenExpr("ul", "input_tokens", true),
	)
	require.Equal(t,
		"ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens",
		usageLogTotalTokensExpr("ul", false),
	)
}

func TestGetStatsWithFiltersAggregatesBillableTokens(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	mock.ExpectQuery(regexp.QuoteMeta("COALESCE(billable_input_tokens, input_tokens) AS input_tokens")).
		WillReturnRows(sqlmock.NewRows([]string{
			"inbound_grouped", "upstream_grouped", "inbound_endpoint", "upstream_endpoint",
			"requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens",
			"cost", "actual_cost", "account_cost", "avg_duration_ms",
		}))

	stats, err := repo.GetStatsWithFilters(context.Background(), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelStatsSeparatesBillableDisplayFromRawAccountUsage(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	t.Run("display", func(t *testing.T) {
		db, mock := newSQLMock(t)
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery(regexp.QuoteMeta("COALESCE(SUM(COALESCE(billable_input_tokens, input_tokens)), 0) as input_tokens")).
			WithArgs(start, end).
			WillReturnRows(sqlmock.NewRows([]string{"model"}))

		stats, err := repo.GetModelStatsWithFilters(context.Background(), start, end, 0, 0, 0, 0, nil, nil, nil)
		require.NoError(t, err)
		require.Empty(t, stats)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("raw account usage", func(t *testing.T) {
		db, mock := newSQLMock(t)
		repo := &usageLogRepository{sql: db}
		mock.ExpectQuery(regexp.QuoteMeta("COALESCE(SUM(input_tokens), 0) as input_tokens")).
			WithArgs(start, end, int64(42)).
			WillReturnRows(sqlmock.NewRows([]string{"model"}))

		stats, err := repo.GetRawModelStatsWithFilters(context.Background(), start, end, 0, 0, 42, 0, nil, nil, nil)
		require.NoError(t, err)
		require.Empty(t, stats)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestDashboardHourlyAggregationStoresBillableTokens(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectExec(regexp.QuoteMeta("COALESCE(SUM(COALESCE(billable_input_tokens, input_tokens)), 0) AS input_tokens")).
		WithArgs(start, end, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.upsertHourlyAggregates(context.Background(), start, end))
	require.NoError(t, mock.ExpectationsWereMet())
}
