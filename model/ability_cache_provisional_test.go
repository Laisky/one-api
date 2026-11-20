package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// provisionalTestKey is the ability every provisional-cooldown test cools.
var provisionalTestKey = cachedAbilityKey{"default", "target", 1}

// snapshotProvisionalAbilityCooldowns clears the provisional overlay for one test
// and restores the previous overlay on cleanup.
//
// Parameters:
//   - t: the test handle whose cleanup restores the overlay.
func snapshotProvisionalAbilityCooldowns(t *testing.T) {
	t.Helper()
	channelSyncLock.Lock()
	previous := provisionalAbilityCooldowns
	provisionalAbilityCooldowns = nil
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		provisionalAbilityCooldowns = previous
		channelSyncLock.Unlock()
	})
}

// setupProvisionalCooldownTest builds the shared two-channel routing fixture: the
// preferred channel 1 and the fallback channel 2, both serving default/target.
//
// Parameters:
//   - t: the test handle; cleanup restores every routing global.
func setupProvisionalCooldownTest(t *testing.T) {
	t.Helper()
	snapshotProvisionalAbilityCooldowns(t)
	setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 0))
}

// cachedAbilityCoolingAt reports, under the routing read lock, whether key is
// excluded from cached routing at now.
//
// Parameters:
//   - key: the exact ability.
//   - now: the evaluation time.
//
// Returns:
//   - bool: true while cached routing must skip the ability.
func cachedAbilityCoolingAt(key cachedAbilityKey, now time.Time) bool {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	return cachedAbilityCoolingLocked(key, now)
}

// requireRoutesTo asserts that 50 consecutive cached selections all pick want.
//
// Parameters:
//   - t: the test handle.
//   - want: the only channel id selection may return.
//   - msg: the failure explanation.
func requireRoutesTo(t *testing.T, want int, msg string) {
	t.Helper()
	for range 50 {
		channel, err := CacheGetRandomSatisfiedChannel("default", "target", false)
		require.NoError(t, err)
		require.Equal(t, want, channel.Id, msg)
	}
}

// TestProvisionalAbilityCooldownHidesAbilityUntilReleased is the model-level
// reproduction of the 429 publication window: between the relay's failure and
// the asynchronous persistence, the provisional overlay alone must keep the
// failed ability out of cached routing, and releasing it without a persisted
// outcome (a discarded or failed write) must restore routing at once.
func TestProvisionalAbilityCooldownHidesAbilityUntilReleased(t *testing.T) {
	setupProvisionalCooldownTest(t)
	requireRoutesTo(t, 1, "control: the healthy preferred channel routes")

	release, duration := PublishProvisionalAbilityCooldown("default", "target", 1)
	require.Equal(t, abilityCooldownLadder[0], duration, "the provisional cooldown is the first rung, never longer")
	requireRoutesTo(t, 2, "a just-failed ability must not route before its cooldown is persisted")
	cooling, err := AbilityCoolingDown(context.Background(), "default", "target", 1)
	require.NoError(t, err)
	require.True(t, cooling, "the point predicate (provider affinity) must agree with selection")
	count, err := CountCachedRetryCandidates("default", "target", nil)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	release()
	requireRoutesTo(t, 1, "a released provisional cooldown with no persisted outcome must not linger")
	release() // idempotent
	channelSyncLock.RLock()
	require.Empty(t, provisionalAbilityCooldowns, "released entries must not accumulate")
	channelSyncLock.RUnlock()
}

// TestProvisionalAbilityCooldownYieldsToPersistedDeadline verifies that the
// persisted outcome replaces the provisional cooldown in both directions: an
// applied failure keeps the exact persisted deadline after release, and a
// persisted recovery that shortened it reopens routing after release instead of
// being masked by the provisional deadline.
func TestProvisionalAbilityCooldownYieldsToPersistedDeadline(t *testing.T) {
	setupProvisionalCooldownTest(t)
	ctx := context.Background()

	release, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
	completed := time.Now().UTC()
	duration, err := RecordAbilityCooldownFailure(ctx, "default", "target", 1, completed.Add(-time.Millisecond), completed)
	require.NoError(t, err)
	require.Equal(t, abilityCooldownLadder[0], duration)
	release()
	channelSyncLock.RLock()
	cached, ok := cachedAbilitySuspensions[provisionalTestKey]
	channelSyncLock.RUnlock()
	require.True(t, ok)
	require.True(t, cached.until.Equal(completed.Add(duration)), "after release the cache holds exactly the persisted deadline")
	requireRoutesTo(t, 2, "the persisted cooldown stays in force after release")

	// A second failure's provisional cooldown is outstanding when a newer success
	// clears the streak: the release must expose the persisted (cleared) state.
	release, _ = PublishProvisionalAbilityCooldown("default", "target", 1)
	recovered := time.Now().UTC()
	require.NoError(t, ResetAbilityCooldown(ctx, "default", "target", 1, recovered.Add(-time.Millisecond), recovered))
	requireRoutesTo(t, 2, "the provisional cooldown still covers its own pending write")
	release()
	requireRoutesTo(t, 1, "the provisional cooldown must not extend a persisted recovery")
}

// TestProvisionalAbilityCooldownSurvivesCacheWriters verifies that full rebuilds
// and the cross-replica mirror, which replace the persisted deadline map, cannot
// drop a provisional cooldown whose persistence is still pending.
func TestProvisionalAbilityCooldownSurvivesCacheWriters(t *testing.T) {
	setupProvisionalCooldownTest(t)
	release, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
	defer release()

	require.NoError(t, InitChannelCacheContext(context.Background()))
	requireRoutesTo(t, 2, "a rebuild must not reopen an ability whose cooldown is being persisted")
	require.NoError(t, refreshCachedAbilitySuspensionsContext(context.Background()))
	requireRoutesTo(t, 2, "the cooldown mirror must not reopen an ability whose cooldown is being persisted")
}

// TestProvisionalAbilityCooldownOverlappingFailures verifies that concurrent
// failures of one ability each hold the provisional cooldown until their own
// persistence finished: releasing the first must not expose the second.
func TestProvisionalAbilityCooldownOverlappingFailures(t *testing.T) {
	setupProvisionalCooldownTest(t)
	first, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
	second, _ := PublishProvisionalAbilityCooldown("default", "target", 1)

	first()
	first() // a repeated release must not consume the second failure's hold
	requireRoutesTo(t, 2, "the second failure's persistence is still pending")
	second()
	requireRoutesTo(t, 1, "every hold released")
}

// TestProvisionalAbilityCooldownReleaseRemovesOwnDeadline verifies that a
// released hold takes its own deadline with it: when a later failure's write
// finished while an earlier, stuck failure is still held, only the earlier
// failure's (older) deadline may remain in force.
func TestProvisionalAbilityCooldownReleaseRemovesOwnDeadline(t *testing.T) {
	setupProvisionalCooldownTest(t)
	stuck, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
	defer stuck()
	stuckExpiry := time.Now().UTC().Add(abilityCooldownLadder[0])
	time.Sleep(20 * time.Millisecond)
	later, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
	probe := stuckExpiry.Add(5 * time.Millisecond) // after the stuck hold, before the later one
	require.True(t, cachedAbilityCoolingAt(provisionalTestKey, probe), "control: the later hold covers the probe time")

	later()
	require.False(t, cachedAbilityCoolingAt(provisionalTestKey, probe),
		"a released hold must not leave its deadline behind for another hold")
	require.True(t, cachedAbilityCoolingAt(provisionalTestKey, time.Now().UTC()), "the stuck hold still applies")
}

// TestProvisionalAbilityCooldownExpiresAtFirstRung bounds a provisional cooldown
// whose persistence never finishes: it expires with the first ladder rung.
func TestProvisionalAbilityCooldownExpiresAtFirstRung(t *testing.T) {
	setupProvisionalCooldownTest(t)
	before := time.Now().UTC()
	release, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
	defer release()
	after := time.Now().UTC()

	require.True(t, cachedAbilityCoolingAt(provisionalTestKey, before.Add(abilityCooldownLadder[0]-time.Millisecond)))
	require.False(t, cachedAbilityCoolingAt(provisionalTestKey, after.Add(abilityCooldownLadder[0]+time.Millisecond)),
		"a stuck write must not hide the ability beyond the first rung")
}

// TestProvisionalAbilityCooldownScope verifies the publication is a no-op when
// the memory cache is disabled (database routing reads the persisted row) or the
// identity is incomplete, and that it is scoped to the exact ability.
func TestProvisionalAbilityCooldownScope(t *testing.T) {
	setupProvisionalCooldownTest(t)

	config.MemoryCacheEnabled = false
	release, duration := PublishProvisionalAbilityCooldown("default", "target", 1)
	release()
	require.Zero(t, duration)
	config.MemoryCacheEnabled = true
	for _, key := range []cachedAbilityKey{{"", "target", 1}, {"default", "", 1}, {"default", "target", 0}} {
		release, duration := PublishProvisionalAbilityCooldown(key.group, key.model, key.channelID)
		release()
		require.Zero(t, duration)
	}
	channelSyncLock.RLock()
	require.Empty(t, provisionalAbilityCooldowns)
	channelSyncLock.RUnlock()

	release, _ = PublishProvisionalAbilityCooldown("default", "target", 1)
	defer release()
	now := time.Now().UTC()
	require.False(t, cachedAbilityCoolingAt(cachedAbilityKey{"default", "other", 1}, now))
	require.False(t, cachedAbilityCoolingAt(cachedAbilityKey{"other", "target", 1}, now))
	require.False(t, cachedAbilityCoolingAt(cachedAbilityKey{"default", "target", 2}, now))
}

// TestProvisionalAbilityCooldownHotPathAllocations keeps the retry-budget count
// allocation-free while a provisional cooldown is outstanding.
func TestProvisionalAbilityCooldownHotPathAllocations(t *testing.T) {
	setupProvisionalCooldownTest(t)
	release, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
	defer release()
	allocs := testing.AllocsPerRun(100, func() {
		count, err := CountCachedRetryCandidates("default", "target", nil)
		if err != nil || count != 1 {
			t.Fatalf("unexpected retry candidate count %d, err=%v", count, err)
		}
	})
	require.Zero(t, allocs)
}

// TestProvisionalAbilityCooldownConcurrentWriters races publications and
// releases against selections, rebuilds and the mirror (run with -race): while a
// hold is outstanding no selection may return the ability, and once every hold
// is released nothing remains.
func TestProvisionalAbilityCooldownConcurrentWriters(t *testing.T) {
	setupProvisionalCooldownTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	anchor, _ := PublishProvisionalAbilityCooldown("default", "target", 1)

	var wg sync.WaitGroup
	violations := make(chan string, 8)
	for range 4 {
		wg.Go(func() {
			for ctx.Err() == nil {
				channel, err := CacheGetRandomSatisfiedChannel("default", "target", false)
				if err != nil || channel.Id != 2 {
					select {
					case violations <- "selection returned a provisionally cooling ability":
					default:
					}
					return
				}
			}
		})
	}
	for range 4 {
		wg.Go(func() {
			for ctx.Err() == nil {
				release, _ := PublishProvisionalAbilityCooldown("default", "target", 1)
				release()
			}
		})
	}
	wg.Go(func() {
		for ctx.Err() == nil {
			if err := InitChannelCacheContext(context.Background()); err != nil {
				violations <- "rebuild: " + err.Error()
				return
			}
			if err := refreshCachedAbilitySuspensionsContext(context.Background()); err != nil {
				violations <- "mirror: " + err.Error()
				return
			}
		}
	})
	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()
	close(violations)
	for violation := range violations {
		t.Fatal(violation)
	}
	anchor()
	requireRoutesTo(t, 1, "all holds released")
	channelSyncLock.RLock()
	require.Empty(t, provisionalAbilityCooldowns)
	channelSyncLock.RUnlock()
}
