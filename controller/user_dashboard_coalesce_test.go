package controller

// Coverage for W0.6 of docs/proposals/20260905_observability-data-tiering.md:
// dashboard aggregate misses must be coalesced per cache key -- with or without
// Redis -- and the per-node computation budget must actually bound concurrency
// without ever queueing without limit or serving an empty bundle.
//
// Every test drives the real production path, resolveDashboardAggregatesForKey,
// and only substitutes the computation itself so that executions can be
// counted.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/admission"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/dto"
)

// dashboardTestSettleDelay is the grace given to goroutines that have already
// passed an atomic barrier but may not yet have reached singleflight's own
// lock. It only ever hides a late arrival by making a test slower, never by
// making a wrong result pass: a caller that arrived too late would run a second
// computation and fail the call-count assertion.
const dashboardTestSettleDelay = 200 * time.Millisecond

// taggedDashboardAggregates builds a bundle carrying its cache key, so a test
// can prove which computation produced the value a caller received.
//
// Parameters:
//   - key: the cache key the bundle belongs to.
//
// Return values:
//   - *dashboardAggregates: a bundle tagged with the key.
func taggedDashboardAggregates(key string) *dashboardAggregates {
	return &dashboardAggregates{
		Logs: []*dto.LogStatistic{{ModelName: key}},
	}
}

// requireDashboardCacheDisabled asserts the Redis-backed cache is inert, which
// is the state W0.6 calls out: with no Redis every request is a miss.
//
// Parameters:
//   - t: the test.
//
// Return values: none.
func requireDashboardCacheDisabled(t *testing.T) {
	t.Helper()

	prev := config.DashboardCacheTTLSec
	config.DashboardCacheTTLSec = 60
	t.Cleanup(func() { config.DashboardCacheTTLSec = prev })

	require.Nil(t, common.RDB, "these tests must run without a Redis client")
	require.False(t, dashboardCacheEnabled(),
		"the cache must be disabled, so only in-process coalescing can protect the queries")
}

// withDashboardBudget installs a concurrency budget and wait budget for one
// test and restores the previous values afterwards.
//
// Parameters:
//   - t: the test.
//   - capacity: DASHBOARD_MAX_CONCURRENT_AGGREGATES for the test; 0 is unlimited.
//   - wait: how long a refused computation waits for a slot.
//
// Return values: none.
func withDashboardBudget(t *testing.T, capacity int, wait time.Duration) {
	t.Helper()

	prevCapacity := config.DashboardMaxConcurrentAggregates
	prevWait := dashboardAggregateWait
	config.DashboardMaxConcurrentAggregates = capacity
	dashboardAggregateWait = wait
	t.Cleanup(func() {
		config.DashboardMaxConcurrentAggregates = prevCapacity
		dashboardAggregateWait = prevWait
	})
}

// TestDashboardAggregatesCoalesceConcurrentMisses verifies that many concurrent
// misses on one cache key run the six aggregates exactly once.
func TestDashboardAggregatesCoalesceConcurrentMisses(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)

	const callers = 32
	key := dashboardCacheKey(11, 1000, 2000)

	var calls, arrived atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	compute := func(context.Context) (*dashboardAggregates, error) {
		calls.Add(1)
		<-release
		return taggedDashboardAggregates(key), nil
	}

	results := make([]*dashboardAggregates, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			arrived.Add(1)
			results[i], errs[i] = resolveDashboardAggregatesForKey(context.Background(), key, compute)
		}(i)
	}

	require.Eventually(t, func() bool { return arrived.Load() == callers },
		10*time.Second, time.Millisecond, "all callers must start")
	time.Sleep(dashboardTestSettleDelay)
	close(release)
	wg.Wait()

	require.Equal(t, int32(1), calls.Load(),
		"concurrent misses on one key must produce exactly one computation")
	for i := range callers {
		require.NoError(t, errs[i])
		require.NotNil(t, results[i])
		require.Same(t, results[0], results[i], "every coalesced caller receives the same bundle")
	}
	require.Equal(t, key, results[0].Logs[0].ModelName)
}

// TestDashboardAggregatesDoNotCoalesceDistinctKeys verifies that different
// windows are never folded into one computation.
func TestDashboardAggregatesDoNotCoalesceDistinctKeys(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)

	keys := []string{
		dashboardCacheKey(11, 1000, 2000),
		dashboardCacheKey(11, 1000, 3000),
		dashboardCacheKey(11, 500, 2000),
	}

	var calls atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	results := make([]*dashboardAggregates, len(keys))
	errs := make([]error, len(keys))

	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			results[i], errs[i] = resolveDashboardAggregatesForKey(context.Background(), key,
				func(context.Context) (*dashboardAggregates, error) {
					calls.Add(1)
					<-release
					return taggedDashboardAggregates(key), nil
				})
		}(i, key)
	}

	// All three must be running at once: if distinct keys were coalesced, the
	// count would never reach three and this would time out.
	require.Eventually(t, func() bool { return calls.Load() == int32(len(keys)) },
		10*time.Second, time.Millisecond, "distinct keys must each run their own computation")
	close(release)
	wg.Wait()

	require.Equal(t, int32(len(keys)), calls.Load())
	for i, key := range keys {
		require.NoError(t, errs[i])
		require.Equal(t, key, results[i].Logs[0].ModelName, "each caller receives its own key's bundle")
	}
}

// TestDashboardAggregatesCoalesceWithoutRedis verifies coalescing does not
// depend on the Redis-backed cache: it is the no-Redis deployment that needs it
// most, because there every single request is a miss.
func TestDashboardAggregatesCoalesceWithoutRedis(t *testing.T) {
	prev := config.DashboardCacheTTLSec
	config.DashboardCacheTTLSec = 0 // caching off even if a client existed
	t.Cleanup(func() { config.DashboardCacheTTLSec = prev })
	require.False(t, dashboardCacheEnabled())

	withDashboardBudget(t, 0, time.Second)

	const callers = 16
	key := dashboardCacheKey(0, 4000, 5000)

	var calls, arrived atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	var wg sync.WaitGroup
	failures := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			arrived.Add(1)
			_, failures[i] = resolveDashboardAggregatesForKey(context.Background(), key,
				func(context.Context) (*dashboardAggregates, error) {
					calls.Add(1)
					<-release
					return taggedDashboardAggregates(key), nil
				})
		}(i)
	}

	require.Eventually(t, func() bool { return arrived.Load() == callers },
		10*time.Second, time.Millisecond)
	time.Sleep(dashboardTestSettleDelay)
	close(release)
	wg.Wait()

	require.Equal(t, int32(1), calls.Load(),
		"coalescing must work with the cache disabled")
	for i := range callers {
		require.NoError(t, failures[i])
	}
}

// TestDashboardAggregateBudgetBoundsConcurrency verifies a configured budget
// caps how many distinct computations run at once.
func TestDashboardAggregateBudgetBoundsConcurrency(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 2, 30*time.Second)

	const callers = 4
	var inFlight, peak, calls atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	compute := func(context.Context) (*dashboardAggregates, error) {
		calls.Add(1)
		current := inFlight.Add(1)
		for {
			observed := peak.Load()
			if current <= observed || peak.CompareAndSwap(observed, current) {
				break
			}
		}
		defer inFlight.Add(-1)
		<-release
		return taggedDashboardAggregates("budgeted"), nil
	}

	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := dashboardCacheKey(20+i, 1000, 2000)
			_, errs[i] = resolveDashboardAggregatesForKey(context.Background(), key, compute)
		}(i)
	}

	require.Eventually(t, func() bool { return inFlight.Load() == 2 },
		10*time.Second, time.Millisecond, "the budget must admit its full capacity")
	time.Sleep(dashboardTestSettleDelay)
	require.Equal(t, int32(2), inFlight.Load(), "the budget must not admit beyond its capacity")
	require.Equal(t, int32(2), calls.Load(), "a refused computation must not run")

	close(release)
	wg.Wait()

	require.Equal(t, int32(2), peak.Load(), "concurrency never exceeded the budget")
	require.Equal(t, int32(callers), calls.Load(), "every caller eventually ran once a slot freed")
	for i := range callers {
		require.NoError(t, errs[i], "waiting within the budget must not fail a request")
	}
}

// TestDashboardAggregateBudgetIsInertWhenZero verifies the standalone default
// keeps today's unlimited behavior, so an upgrade adds no new failure mode.
func TestDashboardAggregateBudgetIsInertWhenZero(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)

	gate, capacity := dashboardAggregateGate()
	require.Nil(t, gate, "an unlimited budget must not build a gate")
	require.Zero(t, capacity)

	const callers = 4
	var running atomic.Int32
	allRunning := make(chan struct{})
	var once sync.Once
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	compute := func(context.Context) (*dashboardAggregates, error) {
		if running.Add(1) == callers {
			once.Do(func() { close(allRunning) })
		}
		<-release
		return taggedDashboardAggregates("unbudgeted"), nil
	}

	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := dashboardCacheKey(30+i, 1000, 2000)
			_, errs[i] = resolveDashboardAggregatesForKey(context.Background(), key, compute)
		}(i)
	}

	select {
	case <-allRunning:
	case <-time.After(10 * time.Second):
		close(release)
		wg.Wait()
		t.Fatal("a zero budget must not limit concurrency")
	}
	close(release)
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
	}
}

// TestDashboardAggregateBudgetRefusesWhenExhausted verifies exhaustion is an
// explicit, bounded refusal rather than an unbounded queue or an empty bundle.
func TestDashboardAggregateBudgetRefusesWhenExhausted(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 1, 50*time.Millisecond)

	var calls atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	compute := func(context.Context) (*dashboardAggregates, error) {
		calls.Add(1)
		<-release
		return taggedDashboardAggregates("holder"), nil
	}

	var holderErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, holderErr = resolveDashboardAggregatesForKey(context.Background(),
			dashboardCacheKey(41, 1000, 2000), compute)
	}()

	require.Eventually(t, func() bool { return calls.Load() == 1 },
		10*time.Second, time.Millisecond, "the holder must occupy the only slot")

	// The deadline is only a safety net: a regression that stopped enforcing the
	// budget would otherwise block here forever on the holder's release channel.
	// The assertions below still require a prompt, explicit refusal.
	refusedCtx, cancelRefused := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelRefused()

	started := time.Now()
	refused, err := resolveDashboardAggregatesForKey(refusedCtx,
		dashboardCacheKey(42, 1000, 2000), compute)
	waited := time.Since(started)

	require.Error(t, err, "an exhausted budget must refuse explicitly")
	require.Nil(t, refused, "a refusal must never return a bundle")
	require.True(t, errors.Is(err, admission.ErrBusy), "refusal must report the budget: %v", err)
	require.Less(t, waited, 5*time.Second, "the wait must be bounded, never an unbounded queue")
	require.Equal(t, int32(1), calls.Load(), "a refused request must not run the aggregates")

	close(release)
	wg.Wait()
	require.NoError(t, holderErr, "the admitted computation must still succeed")
}

// TestDashboardAggregatesNeverShareAcrossScopes verifies two authorization
// scopes cannot receive one another's coalesced result.
func TestDashboardAggregatesNeverShareAcrossScopes(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)

	// The site-wide scope (0) is root-only; the two user scopes are distinct
	// principals. All three ask for the same window, which is exactly the case
	// where an under-specified key would leak.
	scopes := []int{0, 7, 8}
	const callersPerScope = 8

	var calls atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	type outcome struct {
		scope  int
		bundle *dashboardAggregates
		err    error
	}
	outcomes := make([]outcome, 0, len(scopes)*callersPerScope)
	var mu sync.Mutex
	var arrived atomic.Int32

	var wg sync.WaitGroup
	for _, scope := range scopes {
		for range callersPerScope {
			wg.Add(1)
			go func(scope int) {
				defer wg.Done()
				key := dashboardCacheKey(scope, 1000, 2000)
				arrived.Add(1)
				bundle, err := resolveDashboardAggregatesForKey(context.Background(), key,
					func(context.Context) (*dashboardAggregates, error) {
						calls.Add(1)
						<-release
						return taggedDashboardAggregates(key), nil
					})
				mu.Lock()
				outcomes = append(outcomes, outcome{scope: scope, bundle: bundle, err: err})
				mu.Unlock()
			}(scope)
		}
	}

	require.Eventually(t, func() bool { return arrived.Load() == int32(len(scopes)*callersPerScope) },
		10*time.Second, time.Millisecond)
	time.Sleep(dashboardTestSettleDelay)
	close(release)
	wg.Wait()

	require.Equal(t, int32(len(scopes)), calls.Load(),
		"each scope runs its own computation; scopes are never coalesced together")
	for _, got := range outcomes {
		require.NoError(t, got.err)
		require.Equal(t, dashboardCacheKey(got.scope, 1000, 2000), got.bundle.Logs[0].ModelName,
			"a caller must only ever receive the bundle computed for its own scope")
	}
}

// TestDashboardCacheKeyHasNoScopeCollisions verifies the key's field separators
// keep scopes apart, so no pair of different scopes can produce one key.
func TestDashboardCacheKeyHasNoScopeCollisions(t *testing.T) {
	require.NotEqual(t, dashboardCacheKey(1, 23, 456), dashboardCacheKey(12, 3, 456),
		"digits must not run together across fields")
	require.NotEqual(t, dashboardCacheKey(0, 1, 2), dashboardCacheKey(1, 0, 2),
		"site-wide and per-user scopes must stay distinct")

	seen := map[string]struct{}{}
	for _, scope := range []int{0, 1, 10, 11, 100} {
		for _, start := range []int64{0, 1, 10, 100} {
			key := dashboardCacheKey(scope, start, start+1)
			_, duplicate := seen[key]
			require.False(t, duplicate, "cache keys must be unique per scope and window: %s", key)
			seen[key] = struct{}{}
		}
	}
}

// TestDashboardAggregatesSurviveCallerCancellation verifies one abandoned
// request neither cancels nor fails the computation its peers are waiting on.
func TestDashboardAggregatesSurviveCallerCancellation(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)

	key := dashboardCacheKey(51, 1000, 2000)
	var calls atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	compute := func(context.Context) (*dashboardAggregates, error) {
		calls.Add(1)
		<-release
		return taggedDashboardAggregates(key), nil
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()

	var leaderErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, leaderErr = resolveDashboardAggregatesForKey(leaderCtx, key, compute)
	}()

	require.Eventually(t, func() bool { return calls.Load() == 1 },
		10*time.Second, time.Millisecond, "the first caller must start the computation")

	var follower *dashboardAggregates
	var followerErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		follower, followerErr = resolveDashboardAggregatesForKey(context.Background(), key, compute)
	}()

	time.Sleep(dashboardTestSettleDelay)
	cancelLeader()
	time.Sleep(dashboardTestSettleDelay)
	close(release)
	wg.Wait()

	require.Error(t, leaderErr, "the cancelled caller must be told its own request ended")
	require.True(t, errors.Is(leaderErr, context.Canceled), "unexpected error: %v", leaderErr)
	require.NoError(t, followerErr, "one caller leaving must not fail the others")
	require.Equal(t, key, follower.Logs[0].ModelName)
	require.Equal(t, int32(1), calls.Load(), "cancellation must not restart the shared work")
}

// TestDashboardAggregateWorkStopsWithProcessLifecycle verifies shared work is
// owned by a bounded process context rather than context.WithoutCancel of the
// first caller. Before the lifecycle context was introduced, this computation
// waited forever after shutdown when its database call ignored client teardown.
func TestDashboardAggregateWorkStopsWithProcessLifecycle(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	t.Cleanup(stopWorkers)
	restoreLifecycle := SetDashboardAggregateLifecycleContext(workerCtx)
	t.Cleanup(restoreLifecycle)

	previousTimeout := dashboardAggregateWorkTimeout
	dashboardAggregateWorkTimeout = time.Second
	t.Cleanup(func() { dashboardAggregateWorkTimeout = previousTimeout })

	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := resolveDashboardAggregatesForKey(context.Background(),
			dashboardCacheKey(99, 1000, 2000), func(ctx context.Context) (*dashboardAggregates, error) {
				close(started)
				<-ctx.Done()
				return nil, errors.Wrap(ctx.Err(), "aggregate work cancelled")
			})
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("dashboard aggregate work did not start")
	}
	stopWorkers()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("dashboard aggregate work outlived process cancellation")
	}
}

// TestDashboardAggregateWorkJoinWaitsForLeader verifies shutdown cannot close
// the log database while a cancelled singleflight leader is still unwinding.
func TestDashboardAggregateWorkJoinWaitsForLeader(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	t.Cleanup(stopWorkers)
	restoreLifecycle := SetDashboardAggregateLifecycleContext(workerCtx)
	t.Cleanup(restoreLifecycle)

	started := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan error, 1)
	go func() {
		_, err := resolveDashboardAggregatesForKey(context.Background(),
			dashboardCacheKey(100, 1000, 2000), func(ctx context.Context) (*dashboardAggregates, error) {
				close(started)
				<-ctx.Done()
				<-release
				return nil, errors.Wrap(ctx.Err(), "aggregate cleanup")
			})
		completed <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("dashboard aggregate leader did not start")
	}
	stopWorkers()

	deadline, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, WaitForDashboardAggregateWork(deadline), context.DeadlineExceeded,
		"the join must report a leader still unwinding after cancellation")

	close(release)
	require.NoError(t, WaitForDashboardAggregateWork(context.Background()))
	require.ErrorIs(t, <-completed, context.Canceled)
}

// TestDashboardAggregateJoinClosesAdmissionBeforeWaiting verifies shutdown does
// not race a leader that singleflight scheduled but has not registered yet. The
// join closes admission first; a delayed leader then returns without invoking
// the database computation.
func TestDashboardAggregateJoinClosesAdmissionBeforeWaiting(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)
	restoreLifecycle := SetDashboardAggregateLifecycleContext(context.Background())
	t.Cleanup(restoreLifecycle)

	paused := make(chan struct{})
	release := make(chan struct{})
	previousHook := beforeDashboardWorkRegistration
	beforeDashboardWorkRegistration = func() {
		close(paused)
		<-release
	}
	t.Cleanup(func() { beforeDashboardWorkRegistration = previousHook })

	var computes atomic.Int32
	result := make(chan error, 1)
	go func() {
		_, err := resolveDashboardAggregatesForKey(context.Background(),
			dashboardCacheKey(101, 1000, 2000), func(context.Context) (*dashboardAggregates, error) {
				computes.Add(1)
				return taggedDashboardAggregates("must-not-run"), nil
			})
		result <- err
	}()

	select {
	case <-paused:
	case <-time.After(5 * time.Second):
		t.Fatal("singleflight leader did not reach registration barrier")
	}
	require.NoError(t, WaitForDashboardAggregateWork(context.Background()))
	close(release)
	require.Error(t, <-result)
	require.Zero(t, computes.Load(), "a leader admitted after shutdown must not query the database")
}
