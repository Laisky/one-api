package model

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// setupAbilityCacheTest installs a single-connection in-memory database with the
// given same-group channels and their abilities, enables the memory cache, builds
// it, and restores every global on cleanup.
func setupAbilityCacheTest(t *testing.T, channels ...*Channel) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection keeps concurrent queries on the same in-memory database.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelGroup{}, &Ability{}))

	oldDB, oldMemory := DB, config.MemoryCacheEnabled
	channelSyncLock.Lock()
	oldGroups, oldChannels, oldSuspensions := group2model2channels, channelId2channel, cachedAbilitySuspensions
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		DB, config.MemoryCacheEnabled = oldDB, oldMemory
		channelSyncLock.Lock()
		group2model2channels, channelId2channel, cachedAbilitySuspensions = oldGroups, oldChannels, oldSuspensions
		channelSyncLock.Unlock()
	})
	DB, config.MemoryCacheEnabled = db, true
	for _, channel := range channels {
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, channel.AddAbilities())
	}
	require.NoError(t, InitChannelCacheContext(context.Background()))
	return db
}

// abilityCacheChannel builds an enabled channel serving model "target" in the
// default group at priority.
func abilityCacheChannel(id int, priority int64) *Channel {
	return &Channel{Id: id, Name: "c", Status: ChannelStatusEnabled, Models: "target", Group: "default", Priority: &priority}
}

// TestAbilityCacheRebuildCannotDropConcurrentSuspension pins the writer
// serialization deterministically: a suspension issued after a rebuild has read
// the abilities table must survive that rebuild's publication. The query callback
// starts SuspendAbility exactly inside the rebuild's read-to-publish window.
func TestAbilityCacheRebuildCannotDropConcurrentSuspension(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))

	var armed atomic.Bool
	suspended := make(chan error, 1)
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:suspend_during_rebuild", func(tx *gorm.DB) {
		if tx.Statement.Table != "abilities" || !armed.CompareAndSwap(true, false) {
			return
		}
		go func() {
			suspended <- SuspendAbility(context.Background(), "default", "target", 1, time.Hour)
		}()
		// Give an unserialized suspension ample time to persist and publish before
		// this rebuild publishes the snapshot it has already read.
		select {
		case err := <-suspended:
			suspended <- err
		case <-time.After(200 * time.Millisecond):
		}
	}))

	armed.Store(true)
	require.NoError(t, InitChannelCacheContext(context.Background()))
	require.NoError(t, <-suspended)

	for range 50 {
		channel, err := CacheGetRandomSatisfiedChannel("default", "target", false)
		require.NoError(t, err)
		require.Equal(t, 2, channel.Id, "a suspension written during a rebuild must not be overwritten by the older snapshot")
	}
}

// TestAbilityCacheConcurrentSelectionHonorsPublishedCooldown is the concurrency
// regression for the review's async-429 concern: selections, rebuilds and
// suspensions race (run with -race), and any selection that starts after
// SuspendAbility returned must never pick that ability, whichever rebuild runs.
func TestAbilityCacheConcurrentSelectionHonorsPublishedCooldown(t *testing.T) {
	setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10), abilityCacheChannel(3, 10))

	var cooled [4]atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	violations := make(chan int, 64)
	for range 4 {
		wg.Go(func() {
			for ctx.Err() == nil {
				var before [4]bool
				for id := range cooled {
					before[id] = cooled[id].Load()
				}
				channel, err := CacheGetRandomSatisfiedChannelExcluding("default", "target", false, nil, false)
				if err != nil || before[channel.Id] {
					select {
					case violations <- channel.Id:
					default:
					}
					return
				}
			}
		})
	}
	wg.Go(func() {
		for ctx.Err() == nil {
			if err := InitChannelCacheContext(context.Background()); err != nil {
				t.Errorf("rebuild: %v", err)
				return
			}
		}
	})
	for _, id := range []int{1, 2} {
		time.Sleep(20 * time.Millisecond)
		require.NoError(t, SuspendAbility(context.Background(), "default", "target", id, time.Hour))
		cooled[id].Store(true)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
	close(violations)
	for id := range violations {
		t.Fatalf("selection after a published cooldown returned cooling channel %d", id)
	}
	channel, err := CacheGetRandomSatisfiedChannel("default", "target", false)
	require.NoError(t, err)
	require.Equal(t, 3, channel.Id)
}

// TestAbilityCacheEligibilityPassAllocations pins the hot-path allocation budget
// that the review flagged: one candidate slice per selection, no second copy, and
// no allocation at all for retry-budget counting.
func TestAbilityCacheEligibilityPassAllocations(t *testing.T) {
	setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 5), abilityCacheChannel(3, 0))
	require.NoError(t, SuspendAbility(context.Background(), "default", "target", 1, time.Hour))

	allocs := testing.AllocsPerRun(100, func() {
		channels, err := GetChannelsFromCache("default", "target")
		if err != nil || len(channels) != 2 {
			t.Fatalf("unexpected cache result: %d channels, err=%v", len(channels), err)
		}
	})
	require.LessOrEqual(t, allocs, float64(1), "the eligibility pass must return the final caller-owned slice")

	excluded := map[int]bool{2: true}
	allocs = testing.AllocsPerRun(100, func() {
		count, err := CountCachedRetryCandidates("default", "target", excluded)
		if err != nil || count != 1 {
			t.Fatalf("unexpected retry candidate count %d, err=%v", count, err)
		}
	})
	require.Zero(t, allocs)
}

// TestAbilityCoolingDownMatchesRoutingOnBothPaths requires the shared exact-ability
// predicate to agree with the memory and database routing paths, including scope
// to the exact group/model and recovery at expiry.
func TestAbilityCoolingDownMatchesRoutingOnBothPaths(t *testing.T) {
	for _, memory := range []bool{true, false} {
		t.Run(map[bool]string{true: "memory-cache", false: "database"}[memory], func(t *testing.T) {
			setupAbilityCacheTest(t,
				&Channel{Id: 1, Name: "c", Status: ChannelStatusEnabled, Models: "target,other", Group: "default,vip", Priority: new(int64(10))},
				abilityCacheChannel(2, 0),
			)
			config.MemoryCacheEnabled = memory
			ctx := context.Background()
			require.NoError(t, SuspendAbility(ctx, "default", "target", 1, 300*time.Millisecond))

			for _, tc := range []struct {
				group, model string
				channelID    int
				cooling      bool
			}{
				{"default", "target", 1, true},
				{"default", "other", 1, false},
				{"vip", "target", 1, false},
				{"default", "target", 2, false},
				{"default", "missing", 1, false},
			} {
				got, err := AbilityCoolingDown(ctx, tc.group, tc.model, tc.channelID)
				require.NoError(t, err)
				require.Equal(t, tc.cooling, got, "%s/%s/%d", tc.group, tc.model, tc.channelID)
			}
			channel, err := CacheGetRandomSatisfiedChannelExcluding("default", "target", false, nil, false)
			require.NoError(t, err)
			require.Equal(t, 2, channel.Id, "routing must agree with the predicate")

			require.Eventually(t, func() bool {
				cooling, err := AbilityCoolingDown(ctx, "default", "target", 1)
				return err == nil && !cooling
			}, 2*time.Second, 10*time.Millisecond, "expiry must lift the cooldown on both paths")
		})
	}
}
