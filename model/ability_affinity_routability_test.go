package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

const (
	// routabilityGroup is the routing group every fixture channel serves.
	routabilityGroup = "default"
	// routabilityModel is the exact model the bound ability serves.
	routabilityModel = "target"
	// routabilityOtherModel is a second model of the bound channel, used to
	// prove a cooldown on another model does not affect routability.
	routabilityOtherModel = "other"
	// routabilityBoundID is the channel whose ability AbilityRoutable checks.
	routabilityBoundID = 61
	// routabilityPeerID is a second channel in the same pool.
	routabilityPeerID = 62
	// routabilityStrangerID is a channel id that no fixture channel uses.
	routabilityStrangerID = 63
)

// setupAffinityRoutabilityDB installs a single-connection in-memory SQLite
// database holding two enabled channels in routabilityGroup, both at the same
// priority: the bound channel serving routabilityModel and
// routabilityOtherModel, and a peer serving routabilityModel. It persists their
// abilities, selects the routing path by memory, builds the routing cache, and
// restores the database, the memory-cache flag and every cache global on cleanup.
//
// Parameters:
//   - t: the test handle.
//   - memory: true routes through the memory cache, false through the database.
//
// Return values:
//   - *gorm.DB: the installed fixture database.
func setupAffinityRoutabilityDB(t *testing.T, memory bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection keeps every query on the same in-memory database.
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
	DB, config.MemoryCacheEnabled = db, memory

	priority := int64(5)
	for _, channel := range []*Channel{
		{Id: routabilityBoundID, Name: "bound", Status: ChannelStatusEnabled, Group: routabilityGroup,
			Models: routabilityModel + "," + routabilityOtherModel, Priority: &priority},
		{Id: routabilityPeerID, Name: "peer", Status: ChannelStatusEnabled, Group: routabilityGroup,
			Models: routabilityModel, Priority: &priority},
	} {
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, channel.AddAbilities())
	}
	require.NoError(t, InitChannelCacheContext(context.Background()))
	return db
}

// updateRoutabilityAbility applies column updates to the exact fixture ability
// (routabilityGroup, modelName, channelID) and fails the test unless exactly one
// row changed.
//
// Parameters:
//   - t: the test handle.
//   - db: the fixture database.
//   - modelName: the exact model of the ability.
//   - channelID: the channel of the ability.
//   - updates: the column values to write.
func updateRoutabilityAbility(t *testing.T, db *gorm.DB, modelName string, channelID int, updates map[string]any) {
	t.Helper()
	result := exactCooldownAbility(db, cachedAbilityKey{routabilityGroup, modelName, channelID}).Updates(updates)
	require.NoError(t, result.Error)
	require.EqualValues(t, 1, result.RowsAffected, "fixture ability (%s, %s, %d) must exist", routabilityGroup, modelName, channelID)
}

// selectorWouldPick reports whether the shipped excluding selector can return
// channelID for (routabilityGroup, modelName) once every other fixture channel is
// excluded. With the memory cache enabled it runs the cached selector, otherwise
// its database fallback, so it is the behavioral oracle for AbilityRoutable.
//
// Parameters:
//   - t: the test handle.
//   - modelName: the exact model requested.
//   - channelID: the channel that must be returned.
//
// Return values:
//   - bool: true when the selector returned channelID, false when it found none.
func selectorWouldPick(t *testing.T, modelName string, channelID int) bool {
	t.Helper()
	exclude := map[int]bool{}
	for _, id := range []int{routabilityBoundID, routabilityPeerID, routabilityStrangerID} {
		if id != channelID {
			exclude[id] = true
		}
	}
	channel, err := CacheGetRandomSatisfiedChannelExcludingWithContext(context.Background(),
		routabilityGroup, modelName, false, exclude, false)
	if err != nil {
		return false
	}
	require.Equal(t, channelID, channel.Id, "the selector must never return an excluded channel")
	return true
}

// TestAbilityRoutableAgreesWithSelectors pins AbilityRoutable to the pooled
// selectors' eligibility on both routing paths: an ability is routable only
// when its exact (group, model, channel) row exists, is enabled and is outside
// both its fixed and adaptive cooldowns, the model match is case-sensitive, and
// a cooldown on another model of the same channel or an expired deadline keeps
// it routable. Every expectation is also checked against the shipped selector
// so the two can never drift apart.
func TestAbilityRoutableAgreesWithSelectors(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	past := time.Now().UTC().Add(-time.Hour)
	for _, tc := range []struct {
		name      string
		mutate    func(t *testing.T, db *gorm.DB)
		model     string
		channelID int
		want      bool
	}{
		{name: "routable ability", model: routabilityModel, channelID: routabilityBoundID, want: true},
		{name: "peer stays routable", model: routabilityModel, channelID: routabilityPeerID, want: true},
		{
			name: "missing ability row",
			mutate: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{routabilityGroup, routabilityModel, routabilityBoundID}).
					Delete(&Ability{}).Error)
			},
			model: routabilityModel, channelID: routabilityBoundID, want: false,
		},
		{
			name: "disabled ability row",
			mutate: func(t *testing.T, db *gorm.DB) {
				updateRoutabilityAbility(t, db, routabilityModel, routabilityBoundID, map[string]any{"enabled": false})
			},
			model: routabilityModel, channelID: routabilityBoundID, want: false,
		},
		{
			name: "fixed cooldown",
			mutate: func(t *testing.T, db *gorm.DB) {
				updateRoutabilityAbility(t, db, routabilityModel, routabilityBoundID, map[string]any{"suspend_until": future})
			},
			model: routabilityModel, channelID: routabilityBoundID, want: false,
		},
		{
			name: "adaptive cooldown",
			mutate: func(t *testing.T, db *gorm.DB) {
				updateRoutabilityAbility(t, db, routabilityModel, routabilityBoundID, map[string]any{"adaptive_suspend_until": future})
			},
			model: routabilityModel, channelID: routabilityBoundID, want: false,
		},
		{
			name: "expired cooldowns",
			mutate: func(t *testing.T, db *gorm.DB) {
				updateRoutabilityAbility(t, db, routabilityModel, routabilityBoundID,
					map[string]any{"suspend_until": past, "adaptive_suspend_until": past})
			},
			model: routabilityModel, channelID: routabilityBoundID, want: true,
		},
		{
			name: "cooldown on another model",
			mutate: func(t *testing.T, db *gorm.DB) {
				updateRoutabilityAbility(t, db, routabilityOtherModel, routabilityBoundID,
					map[string]any{"suspend_until": future, "adaptive_suspend_until": future})
			},
			model: routabilityModel, channelID: routabilityBoundID, want: true,
		},
		{name: "case-only model variant", model: "Target", channelID: routabilityBoundID, want: false},
		{name: "channel outside the pool", model: routabilityModel, channelID: routabilityStrangerID, want: false},
	} {
		for _, memory := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cache=%t", tc.name, memory), func(t *testing.T) {
				db := setupAffinityRoutabilityDB(t, memory)
				if tc.mutate != nil {
					tc.mutate(t, db)
					require.NoError(t, InitChannelCacheContext(context.Background()))
				}
				got, err := AbilityRoutable(context.Background(), routabilityGroup, tc.model, tc.channelID)
				require.NoError(t, err)
				require.Equal(t, tc.want, got, "AbilityRoutable")
				require.Equal(t, tc.want, selectorWouldPick(t, tc.model, tc.channelID),
					"the shipped selector must agree with AbilityRoutable")
			})
		}
	}
}

// TestAbilityRoutableSeesLiveSuspension proves a suspension published by
// SuspendAbility makes the exact ability unroutable immediately on both paths,
// without a cache rebuild, while the peer channel and the bound channel's other
// model stay routable.
func TestAbilityRoutableSeesLiveSuspension(t *testing.T) {
	for _, memory := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", memory), func(t *testing.T) {
			setupAffinityRoutabilityDB(t, memory)
			ctx := context.Background()
			require.NoError(t, SuspendAbility(ctx, routabilityGroup, routabilityModel, routabilityBoundID, time.Hour))

			got, err := AbilityRoutable(ctx, routabilityGroup, routabilityModel, routabilityBoundID)
			require.NoError(t, err)
			require.False(t, got, "a freshly suspended ability must not be routable")
			got, err = AbilityRoutable(ctx, routabilityGroup, routabilityModel, routabilityPeerID)
			require.NoError(t, err)
			require.True(t, got, "the suspension must not reach the peer channel")
			got, err = AbilityRoutable(ctx, routabilityGroup, routabilityOtherModel, routabilityBoundID)
			require.NoError(t, err)
			require.True(t, got, "the suspension must not reach another model of the channel")
		})
	}
}

// TestAbilityRoutableDatabaseFailures proves the database path reports, rather
// than hides, an uninitialized database and a done context, and that the cache
// path answers from memory without touching the database.
func TestAbilityRoutableDatabaseFailures(t *testing.T) {
	t.Run("database not initialized", func(t *testing.T) {
		setupAffinityRoutabilityDB(t, false)
		DB = nil
		got, err := AbilityRoutable(context.Background(), routabilityGroup, routabilityModel, routabilityBoundID)
		require.Error(t, err)
		require.False(t, got)
	})
	t.Run("cancelled context", func(t *testing.T) {
		setupAffinityRoutabilityDB(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got, err := AbilityRoutable(ctx, routabilityGroup, routabilityModel, routabilityBoundID)
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled), "the cancellation must be preserved: %v", err)
		require.False(t, got)
	})
	t.Run("cache path needs no database", func(t *testing.T) {
		setupAffinityRoutabilityDB(t, true)
		DB = nil
		got, err := AbilityRoutable(context.Background(), routabilityGroup, routabilityModel, routabilityBoundID)
		require.NoError(t, err)
		require.True(t, got)
	})
}
