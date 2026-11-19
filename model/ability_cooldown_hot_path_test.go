package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestAdaptiveAbilityCooldownHealthySuccessDoesNotWaitForCache pins the fast
// path's SQL and lock costs while the global cache writer is held.
func TestAdaptiveAbilityCooldownHealthySuccessDoesNotWaitForCache(t *testing.T) {
	reset, updates, reads := prepareCooldownWatermarkFixture(t)
	abilitySnapshotLock.Lock()
	done := make(chan error, 1)
	go func() { done <- reset(1) }()
	select {
	case err := <-done:
		abilitySnapshotLock.Unlock()
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		abilitySnapshotLock.Unlock()
		<-done
		t.Fatal("healthy success waited on the cache writer")
	}
	require.Equal(t, 1, *updates)
	require.Zero(t, *reads)
}

// BenchmarkAdaptiveAbilityCooldownHealthySuccess measures a serial successful
// watermark update against a disposable on-disk SQLite routing table.
func BenchmarkAdaptiveAbilityCooldownHealthySuccess(b *testing.B) {
	reset, updates, reads := prepareCooldownWatermarkFixture(b)
	i := 0
	for b.Loop() {
		i++
		if err := reset(i); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(*updates)/float64(i), "updates/op")
	b.ReportMetric(float64(*reads)/float64(i), "reads/op")
}

// prepareCooldownWatermarkFixture returns a healthy ability's success recorder
// and counters for update and query callbacks on an isolated file database.
func prepareCooldownWatermarkFixture(t testing.TB) (func(int) error, *int, *int) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "watermark.sqlite")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Ability{}))
	key := cachedAbilityKey{"benchmark", "healthy", 6250}
	require.NoError(t, db.Create(&Ability{Group: key.group, Model: key.model, ChannelId: key.channelID, Enabled: true}).Error)
	var updates, reads int
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test_watermark_update_count", func(*gorm.DB) { updates++ }))
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test_watermark_query_count", func(*gorm.DB) { reads++ }))
	return func(i int) error {
		start := time.Unix(1000+int64(i), 0).UTC()
		return ResetAbilityCooldown(context.Background(), key.group, key.model, key.channelID, start, start.Add(time.Millisecond))
	}, &updates, &reads
}
