package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// TestSitewideRangeDays verifies the range measurement that gates site-wide
// dashboard queries, including partial-day rounding.
func TestSitewideRangeDays(t *testing.T) {
	const day = int64(24 * 60 * 60)

	require.Equal(t, 1, sitewideRangeDays(0, day))
	require.Equal(t, 7, sitewideRangeDays(0, 7*day))
	require.Equal(t, 366, sitewideRangeDays(0, 365*day+1), "a partial day still counts")
	require.Equal(t, 1, sitewideRangeDays(0, 1), "any non-empty range is at least a day")
	require.Zero(t, sitewideRangeDays(day, day), "an empty range spans nothing")
	require.Zero(t, sitewideRangeDays(2*day, day), "an inverted range spans nothing")
}

// TestDashboardCacheKeyIsScoped verifies the cache key separates users and
// windows, so one user's aggregates can never be served to another.
func TestDashboardCacheKeyIsScoped(t *testing.T) {
	base := dashboardCacheKey(7, 100, 200)

	require.NotEqual(t, base, dashboardCacheKey(8, 100, 200), "user must be part of the key")
	require.NotEqual(t, base, dashboardCacheKey(7, 101, 200), "window start must be part of the key")
	require.NotEqual(t, base, dashboardCacheKey(7, 100, 201), "window end must be part of the key")
	require.Equal(t, base, dashboardCacheKey(7, 100, 200), "the key must be stable")
	require.Contains(t, base, dashboardCacheKeyPrefix)
}

// TestDashboardCacheDisabledWithoutRedis verifies the cache stays inert in the
// default zero-dependency deployment, where there is no Redis.
func TestDashboardCacheDisabledWithoutRedis(t *testing.T) {
	prev := config.DashboardCacheTTLSec
	config.DashboardCacheTTLSec = 60
	t.Cleanup(func() { config.DashboardCacheTTLSec = prev })

	// common.IsRedisEnabled defaults to true before InitRedisClient runs, so the
	// nil client is what must keep caching off here.
	require.True(t, common.IsRedisEnabled())
	require.Nil(t, common.RDB)
	require.False(t, dashboardCacheEnabled())
	require.Nil(t, loadCachedDashboardAggregates(context.Background(), dashboardCacheKey(1, 0, 1)))
	require.NotPanics(t, func() {
		storeDashboardAggregates(context.Background(), dashboardCacheKey(1, 0, 1), &dashboardAggregates{})
	})
}

// TestDashboardCacheDisabledByZeroTTL verifies a zero TTL disables caching even
// when Redis is available.
func TestDashboardCacheDisabledByZeroTTL(t *testing.T) {
	prev := config.DashboardCacheTTLSec
	config.DashboardCacheTTLSec = 0
	t.Cleanup(func() { config.DashboardCacheTTLSec = prev })

	require.False(t, dashboardCacheEnabled())
}
