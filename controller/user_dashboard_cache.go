package controller

// Dashboard aggregate caching (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.5).
//
// One dashboard load runs six GROUP BY aggregates over the consume log. Until
// the Phase-2 rollups exist, the only lever available is to stop recomputing
// them for every viewer and every refresh.
//
// Only the aggregates are cached. Quota and status are deliberately left live:
// they are the numbers a user checks most often, and serving them from a
// minute-old snapshot would be a surprising regression for a saving that the
// aggregates already deliver.

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/model"
)

// dashboardCacheKeyPrefix namespaces dashboard entries in Redis.
const dashboardCacheKeyPrefix = "oneapi:dashboard:agg:"

// dashboardAggregates bundles the six per-day aggregate result sets a dashboard
// request needs, so they are cached and invalidated as one unit.
type dashboardAggregates struct {
	Logs          []*dto.LogStatistic            `json:"logs"`
	UserLogs      []*dto.LogStatisticByUser      `json:"user_logs"`
	TokenLogs     []*dto.LogStatisticByToken     `json:"token_logs"`
	ToolLogs      []*dto.ToolLogStatistic        `json:"tool_logs"`
	ToolUserLogs  []*dto.ToolLogStatisticByUser  `json:"tool_user_logs"`
	ToolTokenLogs []*dto.ToolLogStatisticByToken `json:"tool_token_logs"`
}

// dashboardCacheKey builds the cache key for one aggregate window.
//
// Parameters:
//   - targetUserID: the user the aggregates describe; 0 means site-wide.
//   - start: inclusive start of the window, in Unix seconds.
//   - endExclusive: exclusive end of the window, in Unix seconds.
//
// Return values:
//   - string: the Redis key.
func dashboardCacheKey(targetUserID int, start, endExclusive int64) string {
	return dashboardCacheKeyPrefix +
		strconv.Itoa(targetUserID) + ":" +
		strconv.FormatInt(start, 10) + ":" +
		strconv.FormatInt(endExclusive, 10)
}

// dashboardCacheEnabled reports whether aggregate caching is active.
//
// The client handle is checked as well as the flag: common.IsRedisEnabled
// defaults to true before InitRedisClient runs, so the flag alone would make
// every dashboard load attempt a Redis call against a nil client and log a
// warning for it.
//
// Parameters: none.
//
// Return values:
//   - bool: true when a TTL is configured and a Redis client is connected.
func dashboardCacheEnabled() bool {
	return config.DashboardCacheTTLSec > 0 && common.IsRedisEnabled() && common.RDB != nil
}

// loadCachedDashboardAggregates returns a cached aggregate bundle when present.
//
// Parameters:
//   - ctx: request scope for the Redis call.
//   - key: the cache key.
//
// Return values:
//   - *dashboardAggregates: the cached bundle, or nil on a miss.
func loadCachedDashboardAggregates(ctx context.Context, key string) *dashboardAggregates {
	if !dashboardCacheEnabled() {
		return nil
	}

	raw, err := common.RedisGet(ctx, key)
	if err != nil || raw == "" {
		return nil
	}

	var cached dashboardAggregates
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		// A malformed entry is a cache problem, never a request problem.
		logger.FromContext(ctx).Warn("discarding malformed dashboard cache entry",
			zap.String("cache_key", key), zap.Error(err))
		return nil
	}
	return &cached
}

// storeDashboardAggregates caches an aggregate bundle, best effort.
//
// Parameters:
//   - ctx: request scope for the Redis call.
//   - key: the cache key.
//   - aggregates: the bundle to cache.
//
// Return values: none; failures are logged and never surfaced to the caller.
func storeDashboardAggregates(ctx context.Context, key string, aggregates *dashboardAggregates) {
	if !dashboardCacheEnabled() || aggregates == nil {
		return
	}

	payload, err := json.Marshal(aggregates)
	if err != nil {
		logger.FromContext(ctx).Warn("failed to encode dashboard cache entry",
			zap.String("cache_key", key), zap.Error(err))
		return
	}

	ttl := config.DashboardCacheTTL()
	if err := common.RedisSet(ctx, key, string(payload), ttl); err != nil {
		logger.FromContext(ctx).Warn("failed to cache dashboard aggregates",
			zap.String("cache_key", key), zap.Error(err))
	}
}

// collectDashboardAggregates runs the six per-day aggregate queries.
//
// Parameters:
//   - targetUserID: the user to aggregate; 0 means site-wide.
//   - start: inclusive start of the window, in Unix seconds.
//   - endExclusive: exclusive end of the window, in Unix seconds.
//
// Return values:
//   - *dashboardAggregates: the computed bundle.
//   - error: wrapped failure from the first query that could not complete.
func collectDashboardAggregates(targetUserID int, start, endExclusive int) (*dashboardAggregates, error) {
	aggregates := &dashboardAggregates{}

	var err error
	if aggregates.Logs, err = model.SearchLogsByDayAndModel(targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard data")
	}
	if aggregates.UserLogs, err = model.SearchLogsByDayAndUser(targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get user usage data")
	}
	if aggregates.TokenLogs, err = model.SearchLogsByDayAndToken(targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get token usage data")
	}
	if aggregates.ToolLogs, err = model.SearchToolLogsByDayAndTool(targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get tool usage data")
	}
	if aggregates.ToolUserLogs, err = model.SearchToolLogsByDayAndUser(targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get tool user usage data")
	}
	if aggregates.ToolTokenLogs, err = model.SearchToolLogsByDayAndToken(targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get tool token usage data")
	}

	return aggregates, nil
}

// resolveDashboardAggregates returns the aggregate bundle for a window, from
// cache when possible.
//
// Parameters:
//   - ctx: request scope for cache access.
//   - targetUserID: the user to aggregate; 0 means site-wide.
//   - start: inclusive start of the window, in Unix seconds.
//   - endExclusive: exclusive end of the window, in Unix seconds.
//
// Return values:
//   - *dashboardAggregates: the bundle.
//   - error: wrapped failure from the underlying queries.
func resolveDashboardAggregates(ctx context.Context, targetUserID int, start, endExclusive int64) (*dashboardAggregates, error) {
	key := dashboardCacheKey(targetUserID, start, endExclusive)
	if cached := loadCachedDashboardAggregates(ctx, key); cached != nil {
		return cached, nil
	}

	aggregates, err := collectDashboardAggregates(targetUserID, int(start), int(endExclusive))
	if err != nil {
		return nil, err
	}

	storeDashboardAggregates(ctx, key, aggregates)
	return aggregates, nil
}
