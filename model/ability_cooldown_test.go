package model

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestAdaptiveAbilityCooldownPersistsAndResets verifies the ladder, cap, expiry,
// and genuine recovery across separate database connections and cache rebuilds.
func TestAdaptiveAbilityCooldownPersistsAndResets(t *testing.T) {
	ctx := context.Background()
	dbFile := filepath.Join(t.TempDir(), "abilities.sqlite")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		return db
	}
	db := open()
	require.NoError(t, db.AutoMigrate(&Ability{}, &Channel{}))
	original := DB
	t.Cleanup(func() { DB = original })
	DB = db
	key := cachedAbilityKey{"test", "route", 725}
	other := Ability{Group: key.group, Model: "other", ChannelId: key.channelID, Enabled: true}
	require.NoError(t, db.Create(&[]Ability{{Group: key.group, Model: key.model, ChannelId: key.channelID, Enabled: true}, other}).Error)
	read := func() Ability {
		var row Ability
		require.NoError(t, exactCooldownAbility(DB, key).Take(&row).Error)
		return row
	}

	failure := time.Now().UTC().Truncate(time.Second)
	for i, expected := range []time.Duration{10, 30, 60, 180, 600, 600} {
		expected *= time.Second
		got, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, failure.Add(-time.Second), failure)
		require.NoError(t, err)
		require.Equal(t, expected, got)
		row := read()
		require.Equal(t, min(i+1, 5), row.CooldownFailures)
		require.Equal(t, failure.Add(expected), *row.AdaptiveSuspendUntil)
		if i == 1 {
			DB = open() // A new process/connection can continue the persisted streak.
			require.NoError(t, InitChannelCacheContext(ctx))
			channelSyncLock.RLock()
			cached := cachedAbilityCoolingLocked(key, failure)
			channelSyncLock.RUnlock()
			require.True(t, cached)
		}
		var count int64
		require.NoError(t, availableAbilitiesQuery(DB, key.group, key.model, failure.Add(expected).Add(time.Second), nil).Count(&count).Error)
		require.EqualValues(t, 1, count, "expiry makes a probe eligible but does not reset the streak")
		failure = failure.Add(expected + 2*time.Second)
	}

	// A success from before the latest failure retains that later failure.
	lastFailure := time.Unix(0, read().CooldownLastFailure).UTC()
	require.NoError(t, ResetAbilityCooldown(ctx, key.group, key.model, key.channelID, lastFailure.Add(-2*time.Second), lastFailure.Add(-time.Second)))
	require.Equal(t, 1, read().CooldownFailures)

	fixed := failure.Add(20 * time.Minute)
	require.NoError(t, exactCooldownAbility(DB, key).Update("suspend_until", fixed).Error)
	require.NoError(t, ResetAbilityCooldown(ctx, key.group, key.model, key.channelID, failure, failure.Add(time.Second)))
	row := read()
	require.Zero(t, row.CooldownFailures)
	require.Nil(t, row.AdaptiveSuspendUntil)
	require.Equal(t, fixed, *row.SuspendUntil)
	channelSyncLock.RLock()
	cachedFixed := cachedAbilityCoolingLocked(key, failure)
	channelSyncLock.RUnlock()
	require.True(t, cachedFixed, "success cannot remove independent auth/manual cooldown")
	got, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, failure.Add(-2*time.Second), failure.Add(-time.Second))
	require.NoError(t, err)
	require.Zero(t, got, "a delayed older failure cannot reopen cooldown after success")
	got, err = RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, failure.Add(-time.Second), failure.Add(time.Second))
	require.NoError(t, err)
	require.Zero(t, got, "a failure tied with a recovery cannot reopen cooldown")
	got, err = RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, failure.Add(2*time.Second), failure.Add(3*time.Second))
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, got)

	require.NoError(t, DB.Where("channel_id = ? AND model = ?", key.channelID, other.Model).Take(&other).Error)
	require.Zero(t, other.CooldownFailures)
	require.Nil(t, other.AdaptiveSuspendUntil)
}

// TestAdaptiveAbilityCooldownConcurrentFailures verifies that concurrent outcomes
// neither lose bounded streak increments nor move the effective deadline earlier.
func TestAdaptiveAbilityCooldownConcurrentFailures(t *testing.T) {
	db := setupTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	original := DB
	DB = db
	t.Cleanup(func() { DB = original })
	key := cachedAbilityKey{"test", "concurrent", 726}
	require.NoError(t, db.Create(&Ability{Group: key.group, Model: key.model, ChannelId: key.channelID, Enabled: true}).Error)
	at := time.Now().UTC().Truncate(time.Second)
	var wg sync.WaitGroup
	errs := make(chan error, len(abilityCooldownLadder))
	results := make(chan time.Duration, len(abilityCooldownLadder))
	for i := range len(abilityCooldownLadder) {
		wg.Go(func() {
			got, err := RecordAbilityCooldownFailure(context.Background(), key.group, key.model, key.channelID, at, at.Add(time.Duration(i)*time.Second))
			errs <- err
			results <- got
		})
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		require.NoError(t, err)
	}
	var rungs []time.Duration
	for result := range results {
		rungs = append(rungs, result)
	}
	require.ElementsMatch(t, abilityCooldownLadder[:], rungs)
	var row Ability
	require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
	require.Equal(t, len(abilityCooldownLadder), row.CooldownFailures)
	require.Equal(t, at.Add(4*time.Second+10*time.Minute), *row.AdaptiveSuspendUntil)
}

// TestAdaptiveAbilityCooldownMigrationAddsDefaults verifies that the additive
// schema migration preserves an existing fixed deadline and initializes a clean streak.
func TestAdaptiveAbilityCooldownMigrationAddsDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.Exec(`CREATE TABLE abilities ("group" varchar(32), model text, channel_id integer, enabled numeric, priority bigint DEFAULT 0, suspend_until datetime, created_at bigint, updated_at bigint, PRIMARY KEY ("group", model, channel_id))`).Error)
	fixed := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	require.NoError(t, db.Exec(`INSERT INTO abilities ("group", model, channel_id, enabled, suspend_until) VALUES (?, ?, ?, ?, ?)`, "legacy", "route", 727, true, fixed).Error)
	require.NoError(t, db.AutoMigrate(&Ability{}))
	var row Ability
	require.NoError(t, db.Take(&row).Error)
	require.Equal(t, fixed, *row.SuspendUntil)
	require.Nil(t, row.AdaptiveSuspendUntil)
	require.Zero(t, row.CooldownFailures)
	require.Zero(t, row.CooldownRevision)
}

// TestAdaptiveAbilityCooldownRetriesConflict verifies a competing database
// connection's update between the failure's read and its compare-and-swap.
func TestAdaptiveAbilityCooldownRetriesConflict(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "conflict.sqlite")
	first, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)
	second, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)
	for _, db := range []*gorm.DB{first, second} {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	}
	original := DB
	DB = first
	t.Cleanup(func() { DB = original })
	require.NoError(t, first.AutoMigrate(&Ability{}))
	key := cachedAbilityKey{"test", "conflict", 728}
	require.NoError(t, first.Create(&Ability{Group: key.group, Model: key.model, ChannelId: key.channelID, Enabled: true}).Error)
	at := time.Now().UTC().Truncate(time.Second)
	var injected bool
	require.NoError(t, first.Callback().Update().Before("gorm:update").Register("inject_cooldown_contention", func(tx *gorm.DB) {
		if injected {
			return
		}
		injected = true
		result := exactCooldownAbility(second, key).UpdateColumns(map[string]any{
			"cooldown_failures": 1, "cooldown_revision": 1,
			"adaptive_suspend_until": at.Add(10 * time.Second),
			"cooldown_last_failure":  at.UnixNano(),
			"cooldown_history":       fmt.Sprintf("[%d]", at.UnixNano()),
		})
		if result.Error != nil {
			_ = tx.AddError(result.Error)
		}
	}))
	got, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, at, at.Add(time.Second))
	require.NoError(t, err)
	require.True(t, injected)
	require.Equal(t, 30*time.Second, got)
	var row Ability
	require.NoError(t, exactCooldownAbility(second, key).Take(&row).Error)
	require.Equal(t, 2, row.CooldownFailures)
	require.EqualValues(t, 2, row.CooldownRevision)
	require.Equal(t, at.Add(31*time.Second), *row.AdaptiveSuspendUntil)
}

// TestAdaptiveAbilityCooldownCompletionOrdering checks both write permutations
// of overlapping outcomes. A later failure retains only its own rung after recovery.
func TestAdaptiveAbilityCooldownCompletionOrdering(t *testing.T) {
	db := setupTestDB(t)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)
	for _, laterFailure := range []bool{false, true} {
		for _, successFirst := range []bool{false, true} {
			name := fmt.Sprintf("later-failure=%t/success-first=%t", laterFailure, successFirst)
			t.Run(name, func(t *testing.T) {
				const channel = 729
				require.NoError(t, db.Create(&Ability{Group: "test", Model: name, ChannelId: channel, Enabled: true}).Error)
				succeed := func() {
					require.NoError(t, ResetAbilityCooldown(ctx, "test", name, channel, at.Add(2*time.Second), at.Add(4*time.Second)))
				}
				fail := func(startSeconds, finishSeconds time.Duration) {
					_, err := RecordAbilityCooldownFailure(ctx, "test", name, channel, at.Add(startSeconds*time.Second), at.Add(finishSeconds*time.Second))
					require.NoError(t, err)
				}
				fail(0, 1)
				if successFirst {
					succeed()
				}
				fail(1, 3)
				if laterFailure {
					fail(3, 5) // Started before success completed, but finished later.
					fail(3, 5) // Duplicate outcome cannot advance the streak.
				}
				if !successFirst {
					succeed()
				}
				var row Ability
				require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{"test", name, channel}).Take(&row).Error)
				require.Equal(t, at.Add(4*time.Second).UnixNano(), row.CooldownRecovery)
				if laterFailure {
					require.Equal(t, 1, row.CooldownFailures)
					require.Equal(t, at.Add(15*time.Second), *row.AdaptiveSuspendUntil)
					require.JSONEq(t, fmt.Sprintf("[%d]", at.Add(5*time.Second).UnixNano()), row.CooldownHistory)
				} else {
					require.Zero(t, row.CooldownFailures)
					require.Nil(t, row.AdaptiveSuspendUntil)
				}
			})
		}
	}
}

// TestAdaptiveAbilityCooldownUpstreamReplacement verifies a changed upstream
// discards its predecessor's streak and fences earlier dispatched outcomes.
func TestAdaptiveAbilityCooldownUpstreamReplacement(t *testing.T) {
	db := setupTestDB(t)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	ctx := context.Background()
	channel := &Channel{Id: 730, Group: "test", Models: "route", Status: ChannelStatusEnabled}
	require.NoError(t, syncAbilitiesWithDB(db, channel, false))
	key := cachedAbilityKey{"test", "route", channel.Id}
	read := func() Ability {
		var row Ability
		require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
		return row
	}
	at := time.Unix(0, read().CooldownConfigChangedAt).UTC()
	for i, rung := range abilityCooldownLadder {
		event := at.Add(time.Duration(i+1) * time.Nanosecond)
		got, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, event, event)
		require.NoError(t, err)
		require.Equal(t, rung, got)
	}
	require.Equal(t, 5, read().CooldownFailures)
	require.NoError(t, syncAbilitiesWithDB(db, channel, true))
	row := read()
	require.Zero(t, row.CooldownFailures)
	require.Zero(t, row.CooldownLastFailure)
	require.Empty(t, row.CooldownHistory)
	require.Nil(t, row.AdaptiveSuspendUntil)
	require.Greater(t, row.CooldownConfigChangedAt, at.UnixNano())
	after := time.Unix(0, row.CooldownConfigChangedAt).UTC().Add(time.Second)
	stale, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, at, after)
	require.NoError(t, err)
	require.Zero(t, stale)
	fresh, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, after, after)
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, fresh)
	require.NoError(t, ResetAbilityCooldown(ctx, key.group, key.model, key.channelID, at, after.Add(time.Second)))
	require.Equal(t, 1, read().CooldownFailures, "an old provider's late success cannot clear the new provider's streak")
}
