package model

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// setupMirrorTest builds a three-channel default/target routing fixture with a
// clean provisional overlay and returns its database.
//
// Parameters:
//   - t: the test handle; cleanup restores every routing global.
//
// Returns:
//   - *gorm.DB: the isolated in-memory database.
func setupMirrorTest(t *testing.T) *gorm.DB {
	t.Helper()
	snapshotProvisionalAbilityCooldowns(t)
	return setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10), abilityCacheChannel(3, 0))
}

// persistForeignCooldown writes both deadline columns of one default/target
// ability directly, as another replica's persistence would, without touching
// this process's cache. A nil deadline is stored as SQL NULL.
//
// Parameters:
//   - t: the test handle.
//   - db: the shared database.
//   - channelID: the ability's channel.
//   - fixed, adaptive: the suspend_until and adaptive_suspend_until values.
func persistForeignCooldown(t *testing.T, db *gorm.DB, channelID int, fixed, adaptive *time.Time) {
	t.Helper()
	columns := map[string]any{"suspend_until": gorm.Expr("NULL"), "adaptive_suspend_until": gorm.Expr("NULL")}
	if fixed != nil {
		columns["suspend_until"] = *fixed
	}
	if adaptive != nil {
		columns["adaptive_suspend_until"] = *adaptive
	}
	result := exactCooldownAbility(db, cachedAbilityKey{"default", "target", channelID}).UpdateColumns(columns)
	require.NoError(t, result.Error)
	require.EqualValues(t, 1, result.RowsAffected)
}

// cachedMirrorDeadline returns the persisted deadline the cache holds for one
// default/target ability.
//
// Parameters:
//   - channelID: the ability's channel.
//
// Returns:
//   - time.Time: the cached deadline, zero when absent.
//   - bool: whether an entry exists.
func cachedMirrorDeadline(channelID int) (time.Time, bool) {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	cached, ok := cachedAbilitySuspensions[cachedAbilityKey{"default", "target", channelID}]
	return cached.until, ok && !cached.until.IsZero()
}

// TestAbilitySuspensionMirrorPublishesForeignDeadlines reproduces the
// cross-replica gap: a cooldown persisted by another replica, in either deadline
// column, must reach this replica's cached routing on the next mirror refresh
// instead of at the next full rebuild, and a lift persisted elsewhere must
// reopen routing on that refresh too.
func TestAbilitySuspensionMirrorPublishesForeignDeadlines(t *testing.T) {
	db := setupMirrorTest(t)
	ctx := context.Background()
	fixed := time.Now().UTC().Add(time.Minute)
	adaptive := time.Now().UTC().Add(10 * time.Second)
	persistForeignCooldown(t, db, 1, &fixed, nil)
	persistForeignCooldown(t, db, 2, nil, &adaptive)
	_, cached := cachedMirrorDeadline(1)
	require.False(t, cached, "control: another replica's persistence does not touch this cache by itself")

	require.NoError(t, refreshCachedAbilitySuspensionsContext(ctx))
	requireRoutesTo(t, 3, "foreign fixed and adaptive-only cooldowns must be honored after one mirror refresh")
	until, ok := cachedMirrorDeadline(1)
	require.True(t, ok)
	require.True(t, until.Equal(fixed))
	until, ok = cachedMirrorDeadline(2)
	require.True(t, ok)
	require.True(t, until.Equal(adaptive), "the adaptive-only deadline must be mirrored")

	// Both columns set: the later one is the effective deadline.
	later := fixed.Add(time.Minute)
	persistForeignCooldown(t, db, 2, &fixed, &later)
	require.NoError(t, refreshCachedAbilitySuspensionsContext(ctx))
	until, _ = cachedMirrorDeadline(2)
	require.True(t, until.Equal(later), "the mirror publishes the later of both deadlines")

	// A lift persisted elsewhere (recovery, re-enable) reopens routing: the mirror
	// replaces the map instead of only extending it.
	persistForeignCooldown(t, db, 1, nil, nil)
	require.NoError(t, refreshCachedAbilitySuspensionsContext(ctx))
	_, ok = cachedMirrorDeadline(1)
	require.False(t, ok, "a persisted lift must reach the cache")
	channel, err := CacheGetRandomSatisfiedChannel("default", "target", false)
	require.NoError(t, err)
	require.Equal(t, 1, channel.Id)
}

// TestAbilitySuspensionMirrorKeepsLocalAdaptiveDeadline guards the adaptation
// of the unlanded fixed-only mirror: a refresh must never erase an adaptive
// ladder deadline that this process persisted and published.
func TestAbilitySuspensionMirrorKeepsLocalAdaptiveDeadline(t *testing.T) {
	setupMirrorTest(t)
	ctx := context.Background()
	completed := time.Now().UTC()
	duration, err := RecordAbilityCooldownFailure(ctx, "default", "target", 1, completed.Add(-time.Millisecond), completed)
	require.NoError(t, err)
	require.NoError(t, SuspendAbility(ctx, "default", "target", 2, time.Minute))

	for range 3 {
		require.NoError(t, refreshCachedAbilitySuspensionsContext(ctx))
	}
	until, ok := cachedMirrorDeadline(1)
	require.True(t, ok, "the mirror must not erase an adaptive-only cooldown")
	require.True(t, until.Equal(completed.Add(duration)))
	requireRoutesTo(t, 3, "local adaptive and fixed cooldowns survive mirror refreshes")
}

// TestAbilitySuspensionMirrorSkipsInactiveRows verifies that the mirror
// publishes only enabled abilities with a deadline at or after now, exactly the
// active subset of what a rebuild publishes, and drops entries that expired.
func TestAbilitySuspensionMirrorSkipsInactiveRows(t *testing.T) {
	db := setupMirrorTest(t)
	ctx := context.Background()
	future := time.Now().UTC().Add(time.Hour)
	past := time.Now().UTC().Add(-time.Hour)
	persistForeignCooldown(t, db, 1, &past, &past)
	persistForeignCooldown(t, db, 2, &future, nil)
	require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{"default", "target", 2}).UpdateColumn("enabled", false).Error)
	persistForeignCooldown(t, db, 3, nil, &future)

	require.NoError(t, InitChannelCacheContext(ctx))
	channelSyncLock.RLock()
	rebuilt := make(map[cachedAbilityKey]time.Time, len(cachedAbilitySuspensions))
	now := time.Now().UTC()
	for key, cached := range cachedAbilitySuspensions {
		if abilityCoolingAt(cached.until, now) {
			rebuilt[key] = cached.until
		}
	}
	channelSyncLock.RUnlock()

	require.NoError(t, refreshCachedAbilitySuspensionsContext(ctx))
	channelSyncLock.RLock()
	mirrored := make(map[cachedAbilityKey]time.Time, len(cachedAbilitySuspensions))
	for key, cached := range cachedAbilitySuspensions {
		mirrored[key] = cached.until.UTC()
	}
	channelSyncLock.RUnlock()
	for key, until := range rebuilt {
		rebuilt[key] = until.UTC()
	}
	require.Equal(t, rebuilt, mirrored, "the mirror publishes the active subset of a rebuild")
	require.Len(t, mirrored, 1)
	_, ok := mirrored[cachedAbilityKey{"default", "target", 3}]
	require.True(t, ok)
}

// TestAbilitySuspensionMirrorCannotDropConcurrentLocalPublication pins the
// writer serialization: a local adaptive failure recorded while the mirror is
// reading the abilities table must survive the mirror's publication of its
// older snapshot. The query callback starts the failure inside the mirror's
// read-to-publish window.
func TestAbilitySuspensionMirrorCannotDropConcurrentLocalPublication(t *testing.T) {
	db := setupMirrorTest(t)
	var armed atomic.Bool
	recorded := make(chan error, 1)
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:fail_during_mirror", func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "adaptive_suspend_until >=") || !armed.CompareAndSwap(true, false) {
			return
		}
		go func() {
			completed := time.Now().UTC()
			_, err := RecordAbilityCooldownFailure(context.Background(), "default", "target", 1, completed.Add(-time.Millisecond), completed)
			recorded <- err
		}()
		// Give an unserialized writer ample time to persist and publish before
		// the mirror publishes the snapshot it has already read.
		select {
		case err := <-recorded:
			recorded <- err
		case <-time.After(200 * time.Millisecond):
		}
	}))
	armed.Store(true)
	require.NoError(t, refreshCachedAbilitySuspensionsContext(context.Background()))
	require.False(t, armed.Load(), "the mirror read must have run")
	require.NoError(t, <-recorded)
	_, ok := cachedMirrorDeadline(1)
	require.True(t, ok, "a cooldown published during the mirror's read must not be overwritten by its older snapshot")
}

// TestAbilitySuspensionMirrorUsesDeadlineIndexes pins the query shape on
// SQLite: each branch of the mirror is a range read of its deadline index, so
// expired and never-suspended rows are not visited.
func TestAbilitySuspensionMirrorUsesDeadlineIndexes(t *testing.T) {
	db := setupMirrorTest(t)
	now := time.Now().UTC()
	rows, err := db.Raw("EXPLAIN QUERY PLAN "+activeAbilityDeadlinesSQL(), true, now, true, now).Rows()
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	columns, err := rows.Columns()
	require.NoError(t, err)
	var plan []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))
		detail, ok := values[len(values)-1].(string)
		require.True(t, ok)
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	joined := strings.Join(plan, "\n")
	require.Contains(t, joined, "USING INDEX idx_abilities_suspend_until (suspend_until>?)", joined)
	require.Contains(t, joined, "USING INDEX idx_abilities_adaptive_suspend_until (adaptive_suspend_until>?)", joined)
	require.NotContains(t, joined, "SCAN abilities", "the mirror must never scan the abilities table")
}

// TestSyncAbilitySuspensionsContextLoop verifies the worker: it mirrors a
// foreign cooldown within a few intervals, stops on cancellation, returns at
// once when disabled, and leaves the cache alone while the memory cache is off.
func TestSyncAbilitySuspensionsContextLoop(t *testing.T) {
	db := setupMirrorTest(t)
	done := make(chan struct{})
	go func() {
		SyncAbilitySuspensionsContext(context.Background(), 0)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a disabled mirror must return immediately")
	}

	// startWorker runs the mirror every 10ms and returns its stop function, which
	// cancels it and waits for it to return.
	startWorker := func() func() {
		ctx, cancel := context.WithCancel(context.Background())
		stopped := make(chan struct{})
		go func() {
			SyncAbilitySuspensionsContext(ctx, 10*time.Millisecond)
			close(stopped)
		}()
		return func() {
			cancel()
			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("the worker must stop when its context is cancelled")
			}
		}
	}

	stop := startWorker()
	future := time.Now().UTC().Add(time.Minute)
	persistForeignCooldown(t, db, 1, nil, &future)
	require.Eventually(t, func() bool {
		_, ok := cachedMirrorDeadline(1)
		return ok
	}, 2*time.Second, 5*time.Millisecond, "the worker must mirror a foreign cooldown")
	stop()

	// The flag is toggled only while no worker runs, so -race sees no conflict.
	config.MemoryCacheEnabled = false
	stop = startWorker()
	persistForeignCooldown(t, db, 2, nil, &future)
	time.Sleep(100 * time.Millisecond)
	stop()
	config.MemoryCacheEnabled = true
	_, ok := cachedMirrorDeadline(2)
	require.False(t, ok, "the worker must not touch the cache while the memory cache is disabled")
}
