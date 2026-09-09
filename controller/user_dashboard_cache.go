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
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/sync/singleflight"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/admission"
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
	return collectDashboardAggregatesWithContext(context.Background(), targetUserID, start, endExclusive)
}

// collectDashboardAggregatesWithContext runs the dashboard aggregate queries
// with cancellation propagated to the database.
//
// Parameters:
//   - ctx: lifecycle and deadline scope for all aggregate queries.
//   - targetUserID: the user to aggregate; 0 means site-wide.
//   - start: inclusive start of the window, in Unix seconds.
//   - endExclusive: exclusive end of the window, in Unix seconds.
//
// Return values:
//   - *dashboardAggregates: the completed aggregate bundle.
//   - error: wrapped failure from the first query that could not complete.
func collectDashboardAggregatesWithContext(ctx context.Context, targetUserID int, start, endExclusive int) (*dashboardAggregates, error) {
	aggregates := &dashboardAggregates{}

	var err error
	if aggregates.Logs, err = model.SearchLogsByDayAndModelWithContext(ctx, targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard data")
	}
	if aggregates.UserLogs, err = model.SearchLogsByDayAndUserWithContext(ctx, targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get user usage data")
	}
	if aggregates.TokenLogs, err = model.SearchLogsByDayAndTokenWithContext(ctx, targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get token usage data")
	}
	if aggregates.ToolLogs, err = model.SearchToolLogsByDayAndToolWithContext(ctx, targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get tool usage data")
	}
	if aggregates.ToolUserLogs, err = model.SearchToolLogsByDayAndUserWithContext(ctx, targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get tool user usage data")
	}
	if aggregates.ToolTokenLogs, err = model.SearchToolLogsByDayAndTokenWithContext(ctx, targetUserID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get tool token usage data")
	}

	return aggregates, nil
}

// Miss coalescing and the per-node concurrency budget (W0.6).
//
// The TTL cache above amortizes repeated reads but does nothing for a cold or
// expired key: without coalescing, every concurrent miss on the same key runs
// all six aggregates, and because dashboardCacheEnabled reports false whenever
// Redis is missing, the zero-dependency deployment has EVERY request on that
// path with no protection at all. Coalescing therefore lives in process and is
// unconditional; it never consults the cache's availability.

// dashboardAggregateGateName names the concurrency budget in logs and errors.
const dashboardAggregateGateName = "dashboard_aggregates"

// dashboardAggregateWait bounds how long one computation waits for a budget
// slot before its request is refused.
//
// It is a variable only so tests can shorten the wait; nothing outside this
// package changes it, and it is not configurable.
var dashboardAggregateWait = 2 * time.Second

// dashboardAggregateWorkTimeout bounds shared work independently from any one
// HTTP caller. A cancelled caller must not strand a singleflight leader, but a
// detached leader must also never wait forever for a database or admission slot.
var dashboardAggregateWorkTimeout = 30 * time.Second

var (
	dashboardLifecycleMu  sync.RWMutex
	dashboardLifecycleCtx = context.Background()
	dashboardWorkMu       sync.Mutex
	dashboardWorkActive   int
	dashboardWorkDone     = make(chan struct{})
	dashboardWorkClosed   bool
)

// beforeDashboardWorkRegistration is a test hook used to prove shutdown closes
// work admission before a scheduled singleflight leader can touch the database.
var beforeDashboardWorkRegistration func()

// SetDashboardAggregateLifecycleContext installs the process lifecycle context
// used by shared aggregate work.
//
// Parameters:
//   - ctx: process lifecycle context; nil uses context.Background.
//
// Return values:
//   - func(): restores the previous lifecycle context.
func SetDashboardAggregateLifecycleContext(ctx context.Context) func() {
	if ctx == nil {
		ctx = context.Background()
	}
	dashboardLifecycleMu.Lock()
	previous := dashboardLifecycleCtx
	dashboardLifecycleCtx = ctx
	dashboardLifecycleMu.Unlock()
	dashboardWorkMu.Lock()
	dashboardWorkClosed = false
	dashboardWorkMu.Unlock()
	return func() {
		dashboardLifecycleMu.Lock()
		dashboardLifecycleCtx = previous
		dashboardLifecycleMu.Unlock()
	}
}

// dashboardAggregateWorkContext returns a bounded process-owned context for
// singleflight leaders.
//
// Parameters: none.
//
// Return values:
//   - context.Context: context cancelled by process shutdown or timeout.
//   - context.CancelFunc: releases the timeout timer.
func dashboardAggregateWorkContext() (context.Context, context.CancelFunc) {
	dashboardLifecycleMu.RLock()
	base := dashboardLifecycleCtx
	dashboardLifecycleMu.RUnlock()
	return context.WithTimeout(base, dashboardAggregateWorkTimeout)
}

// beginDashboardAggregateWork registers one singleflight leader before it can
// touch the database, so shutdown can join it after cancelling the lifecycle.
//
// Parameters: none.
//
// Return values: none.
func beginDashboardAggregateWork() bool {
	dashboardWorkMu.Lock()
	defer dashboardWorkMu.Unlock()
	if dashboardWorkClosed {
		return false
	}
	if dashboardWorkActive == 0 {
		dashboardWorkDone = make(chan struct{})
	}
	dashboardWorkActive++
	return true
}

// finishDashboardAggregateWork records that one shared aggregate leader ended.
//
// Parameters: none.
//
// Return values: none.
func finishDashboardAggregateWork() {
	dashboardWorkMu.Lock()
	dashboardWorkActive--
	if dashboardWorkActive == 0 {
		close(dashboardWorkDone)
	}
	dashboardWorkMu.Unlock()
}

// WaitForDashboardAggregateWork waits for shared aggregate leaders to return.
//
// Parameters:
//   - ctx: shutdown deadline; nil uses context.Background.
//
// Return values:
//   - error: wrapped context error when shared work is still running.
func WaitForDashboardAggregateWork(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	dashboardWorkMu.Lock()
	dashboardWorkClosed = true
	if dashboardWorkActive == 0 {
		dashboardWorkMu.Unlock()
		return nil
	}
	done := dashboardWorkDone
	dashboardWorkMu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return errors.Wrap(ctx.Err(), "wait for dashboard aggregate work")
		}
	}
}

// dashboardAggregateGroup coalesces concurrent misses so that all callers of
// one cache key share a single execution of the six aggregate queries.
//
// The key is the cache key, which already carries the effective authorization
// scope (see dashboardCacheKey), so two callers can only share a result when
// they were resolved to the same target user and the same window.
var dashboardAggregateGroup singleflight.Group

// The budget gate is built lazily and rebuilt when its inputs change, because
// config.DashboardMaxConcurrentAggregates is read at process start and an
// admission.Gate fixes its capacity at construction.
var (
	dashboardGateMu       sync.Mutex
	dashboardGate         *admission.Gate
	dashboardGateCapacity int
	dashboardGateWait     time.Duration
)

// dashboardAggregateGate returns the node's aggregate-computation budget.
//
// A non-positive DASHBOARD_MAX_CONCURRENT_AGGREGATES means unlimited, which is
// the standalone default: an upgrade must not introduce a refusal that the
// previous release never produced. admission.NewGate clamps capacities below
// one up to one, so the unlimited case cannot be expressed as a gate and is
// reported as a nil gate instead.
//
// Parameters: none.
//
// Return values:
//   - *admission.Gate: the gate, or nil when the budget is unlimited.
//   - int: the configured capacity; zero when unlimited.
func dashboardAggregateGate() (*admission.Gate, int) {
	capacity := config.DashboardMaxConcurrentAggregates
	if capacity <= 0 {
		return nil, 0
	}

	dashboardGateMu.Lock()
	defer dashboardGateMu.Unlock()

	if dashboardGate == nil || dashboardGateCapacity != capacity || dashboardGateWait != dashboardAggregateWait {
		dashboardGate = admission.NewGate(dashboardAggregateGateName, capacity, dashboardAggregateWait)
		dashboardGateCapacity = capacity
		dashboardGateWait = dashboardAggregateWait
	}
	return dashboardGate, dashboardGateCapacity
}

// acquireDashboardAggregateBudget reserves one of this node's concurrent
// aggregate-computation slots.
//
// Exhaustion is a bounded, explicit refusal: a caller waits at most
// dashboardAggregateWait and is then refused with an error. It is never an
// unbounded queue, and it never degrades into an empty or partial bundle,
// because a dashboard that silently renders zeros is worse than one that says
// it is busy. Only the coalescing leader for a key reaches this function, so N
// viewers of the same window consume one slot, not N.
//
// Parameters:
//   - ctx: scope of the wait.
//   - key: the cache key being computed, for log attribution.
//
// Return values:
//   - func(): releases the slot; never nil when the error is nil.
//   - error: wrapped admission.ErrBusy when the budget stayed full for the
//     whole wait, or the context's error.
func acquireDashboardAggregateBudget(ctx context.Context, key string) (func(), error) {
	gate, capacity := dashboardAggregateGate()
	if gate == nil {
		return func() {}, nil
	}

	release, err := gate.Acquire(ctx)
	if err != nil {
		if errors.Is(err, admission.ErrBusy) {
			// Load shedding is an expected, client-visible condition, not a
			// server fault: WARN, and no stack.
			logger.FromContext(ctx).Warn("dashboard aggregate budget exhausted",
				zap.String("cache_key", key),
				zap.Int("budget", capacity),
				zap.Int("in_flight", gate.InFlight()),
				zap.Duration("waited", dashboardAggregateWait))
			return nil, errors.Wrapf(err,
				"dashboard is busy: %d concurrent aggregate computations already running on this node",
				capacity)
		}
		return nil, errors.Wrap(err, "wait for a dashboard aggregate budget slot")
	}
	return release, nil
}

// dashboardAggregateComputation produces one aggregate bundle for a cache key.
//
// It is a named type so that the coalescing, budget and caching path can be
// driven by a counting stub in tests without duplicating any of it.
//
// Parameters:
//   - ctx: scope of the computation, already detached from any single caller's
//     cancellation because the result is shared by every coalesced caller.
//
// Return values:
//   - *dashboardAggregates: the computed bundle.
//   - error: wrapped failure from the underlying work.
type dashboardAggregateComputation func(ctx context.Context) (*dashboardAggregates, error)

// resolveDashboardAggregatesForKey serves one cache key: a cache hit, a
// coalesced wait on an in-flight computation, or a budgeted computation.
//
// Authorization: the key is the whole authorization decision. It is built from
// the EFFECTIVE target scope the handler already resolved and authorized, so a
// caller can only join a computation started for the scope it was itself
// resolved to; nothing here can widen a scope, and no re-check is possible or
// needed at this layer.
//
// Parameters:
//   - ctx: the caller's request scope.
//   - key: the cache key, from dashboardCacheKey.
//   - compute: the work to run on a miss.
//
// Return values:
//   - *dashboardAggregates: the bundle, shared read-only with coalesced callers.
//   - error: wrapped failure from the cache miss path, the budget, or the
//     caller's own cancellation.
func resolveDashboardAggregatesForKey(ctx context.Context, key string, compute dashboardAggregateComputation) (*dashboardAggregates, error) {
	if cached := loadCachedDashboardAggregates(ctx, key); cached != nil {
		return cached, nil
	}

	results := dashboardAggregateGroup.DoChan(key, func() (any, error) {
		if beforeDashboardWorkRegistration != nil {
			beforeDashboardWorkRegistration()
		}
		if !beginDashboardAggregateWork() {
			return nil, errors.WithStack(errors.New("dashboard aggregate work stopped by process shutdown"))
		}
		defer finishDashboardAggregateWork()
		// The shared work is owned by the process, not its first caller. It is
		// still bounded by both the process lifecycle and a finite timeout.
		workCtx, cancel := dashboardAggregateWorkContext()
		defer cancel()

		// Another leader may have finished and cached between the read above and
		// this call; re-reading the cache is far cheaper than six aggregates.
		if cached := loadCachedDashboardAggregates(workCtx, key); cached != nil {
			return cached, nil
		}

		release, err := acquireDashboardAggregateBudget(workCtx, key)
		if err != nil {
			// Already wrapped, and deliberately propagated to every coalesced
			// caller: a refusal must be visible, never an empty bundle.
			return nil, err
		}
		defer release()

		aggregates, err := compute(workCtx)
		if err != nil {
			// Already wrapped by the computation, per query.
			return nil, err
		}

		storeDashboardAggregates(workCtx, key, aggregates)
		return aggregates, nil
	})

	select {
	case result := <-results:
		if result.Err != nil {
			return nil, errors.Wrap(result.Err, "resolve dashboard aggregates")
		}
		aggregates, ok := result.Val.(*dashboardAggregates)
		if !ok || aggregates == nil {
			// Unreachable unless this file's own contract is broken; that is a
			// server fault. Return it for the request boundary to log once.
			return nil, errors.WithStack(errors.Errorf("dashboard aggregates resolved to an unusable %T", result.Val))
		}
		return aggregates, nil
	case <-ctx.Done():
		// The shared computation keeps running for the callers still waiting.
		return nil, errors.Wrap(ctx.Err(), "wait for dashboard aggregates")
	}
}

// resolveDashboardAggregates returns the aggregate bundle for a window, from
// cache when possible, coalescing concurrent misses and honoring the node's
// concurrency budget.
//
// Parameters:
//   - ctx: request scope for cache access and for the caller's own wait.
//   - targetUserID: the ALREADY AUTHORIZED user to aggregate; 0 means site-wide.
//   - start: inclusive start of the window, in Unix seconds.
//   - endExclusive: exclusive end of the window, in Unix seconds.
//
// Return values:
//   - *dashboardAggregates: the bundle.
//   - error: wrapped failure from the underlying queries or the budget.
func resolveDashboardAggregates(ctx context.Context, targetUserID int, start, endExclusive int64) (*dashboardAggregates, error) {
	key := dashboardCacheKey(targetUserID, start, endExclusive)

	return resolveDashboardAggregatesForKey(ctx, key, func(workCtx context.Context) (*dashboardAggregates, error) {
		return collectDashboardAggregatesWithContext(workCtx, targetUserID, int(start), int(endExclusive))
	})
}
