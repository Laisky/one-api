package controller

// MEASUREMENT (not correctness) for W0.6 -- proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0:
//
//	"Coalesce dashboard aggregate cache misses and enforce a budget in explicit
//	 optimized mode, including when Redis is unavailable."
//
// The acceptance assertions are in user_dashboard_coalesce_test.go. This file
// reports how many underlying aggregate computations N concurrent cold viewers
// actually cause, before and after.
//
// HOW THE "BEFORE" ARM IS RECONSTRUCTED
//
// legacyResolveDashboardAggregates below is the pre-remediation body of
// resolveDashboardAggregates, verbatim, calling the same cache helpers the
// shipped code calls:
//
//	key := dashboardCacheKey(targetUserID, start, endExclusive)
//	if cached := loadCachedDashboardAggregates(ctx, key); cached != nil { return cached, nil }
//	aggregates, err := collectDashboardAggregates(...)
//	if err != nil { return nil, err }
//	storeDashboardAggregates(ctx, key, aggregates)
//	return aggregates, nil
//
// Only the six aggregate queries are replaced, by a stub that counts its
// executions and costs a fixed 25 ms -- the site-wide aggregate measured at
// 25.1 ms over 100000 users in docs/benchmarks/20260905_observability-phase0-phase1.md
// section 7. A free stub would make every arm look identical.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// measuredAggregateCost is the stubbed cost of one six-query aggregate bundle.
const measuredAggregateCost = 25 * time.Millisecond

// legacyResolveDashboardAggregates is the pre-remediation resolve path.
//
// Parameters:
//   - ctx: request scope for cache access.
//   - key: the cache key.
//   - compute: the aggregate computation.
//
// Return values:
//   - *dashboardAggregates: the bundle.
//   - error: wrapped failure from the computation.
func legacyResolveDashboardAggregates(ctx context.Context, key string,
	compute dashboardAggregateComputation) (*dashboardAggregates, error) {
	if cached := loadCachedDashboardAggregates(ctx, key); cached != nil {
		return cached, nil
	}
	aggregates, err := compute(ctx)
	if err != nil {
		return nil, err
	}
	storeDashboardAggregates(ctx, key, aggregates)
	return aggregates, nil
}

// countingAggregateComputation builds a stub bundle producer that counts its
// executions and its peak concurrency.
//
// Parameters:
//   - key: the cache key the bundle belongs to.
//   - executions: incremented once per execution.
//   - inFlight: current concurrency.
//   - peak: highest concurrency observed.
//
// Return values:
//   - dashboardAggregateComputation: the stub.
func countingAggregateComputation(key string, executions, inFlight, peak *atomic.Int64) dashboardAggregateComputation {
	return func(context.Context) (*dashboardAggregates, error) {
		executions.Add(1)
		current := inFlight.Add(1)
		for {
			observed := peak.Load()
			if current <= observed || peak.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(measuredAggregateCost)
		inFlight.Add(-1)
		return taggedDashboardAggregates(key), nil
	}
}

// useMiniredisDashboardCache points the dashboard cache at an in-process Redis.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values: none.
func useMiniredisDashboardCache(t *testing.T) {
	t.Helper()

	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	prevEnabled, prevClient, prevTTL := common.IsRedisEnabled(), common.RDB, config.DashboardCacheTTLSec
	common.SetRedisEnabled(true)
	common.RDB = client
	config.DashboardCacheTTLSec = 60
	t.Cleanup(func() {
		common.SetRedisEnabled(prevEnabled)
		common.RDB = prevClient
		config.DashboardCacheTTLSec = prevTTL
	})
	require.True(t, dashboardCacheEnabled())
}

// TestMeasureDashboardMissCoalescing reports the aggregate computations N
// concurrent cold viewers of one window cause, with and without coalescing.
func TestMeasureDashboardMissCoalescing(t *testing.T) {
	redisArms := []struct {
		name  string
		setup func(*testing.T)
	}{
		{name: "redis unavailable (cache disabled; the zero-dependency deployment)",
			setup: func(t *testing.T) { requireDashboardCacheDisabled(t) }},
		{name: "redis available (miniredis, DASHBOARD_CACHE_TTL_SEC=60)",
			setup: useMiniredisDashboardCache},
	}

	for _, redisArm := range redisArms {
		for _, viewers := range []int{1, 4, 16, 64, 256} {
			t.Run(redisArm.name, func(t *testing.T) {
				redisArm.setup(t)
				withDashboardBudget(t, 0, 5*time.Second)

				run := func(coalesced bool) (int64, int64, time.Duration) {
					var executions, inFlight, peak atomic.Int64
					// A distinct key per arm and viewer count, so no arm can be
					// served by a value another arm cached.
					key := dashboardCacheKey(0, int64(viewers), int64(len(redisArm.name)))
					if coalesced {
						key += ":coalesced"
					}
					compute := countingAggregateComputation(key, &executions, &inFlight, &peak)

					var (
						wg      sync.WaitGroup
						release = make(chan struct{})
					)
					wg.Add(viewers)
					start := time.Now()
					for range viewers {
						go func() {
							defer wg.Done()
							<-release
							var err error
							if coalesced {
								_, err = resolveDashboardAggregatesForKey(context.Background(), key, compute)
							} else {
								_, err = legacyResolveDashboardAggregates(context.Background(), key, compute)
							}
							require.NoError(t, err)
						}()
					}
					close(release)
					wg.Wait()
					return executions.Load(), peak.Load(), time.Since(start)
				}

				legacyExec, legacyPeak, legacyWall := run(false)
				coalescedExec, coalescedPeak, coalescedWall := run(true)

				t.Logf("redis=%q viewers=%d arm=%q aggregate_computations=%d peak_concurrent=%d wall=%s",
					redisArm.name, viewers, "before: no coalescing",
					legacyExec, legacyPeak, legacyWall)
				t.Logf("redis=%q viewers=%d arm=%q aggregate_computations=%d peak_concurrent=%d wall=%s",
					redisArm.name, viewers, "after: singleflight coalescing",
					coalescedExec, coalescedPeak, coalescedWall)
			})
		}
	}
}

// TestMeasureDashboardConcurrencyBudget reports how the per-node budget bounds
// concurrent computations across DISTINCT cache keys, which coalescing alone
// cannot bound.
func TestMeasureDashboardConcurrencyBudget(t *testing.T) {
	requireDashboardCacheDisabled(t)

	const scopes = 32
	for _, budget := range []int{0, 2, 8} {
		withDashboardBudget(t, budget, 30*time.Second)

		var executions, inFlight, peak atomic.Int64
		var refused atomic.Int64
		var wg sync.WaitGroup
		release := make(chan struct{})

		wg.Add(scopes)
		start := time.Now()
		for i := range scopes {
			go func() {
				defer wg.Done()
				<-release
				key := dashboardCacheKey(i+1, int64(budget), 1)
				compute := countingAggregateComputation(key, &executions, &inFlight, &peak)
				if _, err := resolveDashboardAggregatesForKey(context.Background(), key, compute); err != nil {
					refused.Add(1)
				}
			}()
		}
		close(release)
		wg.Wait()

		label := "unlimited (standalone default)"
		if budget > 0 {
			label = "bounded"
		}
		t.Logf("budget=%d (%s) distinct_scopes=%d aggregate_computations=%d peak_concurrent=%d "+
			"refused=%d wall=%s",
			budget, label, scopes, executions.Load(), peak.Load(), refused.Load(), time.Since(start))
	}
}
