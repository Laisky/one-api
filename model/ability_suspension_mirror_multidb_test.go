package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAbilitySuspensionMirrorMultiDB runs the mirror's dialect-specific read on
// SQLite, MySQL (MYSQL_DSN) and PostgreSQL (PG_DSN): group quoting, the UNION ALL
// of both deadline ranges, timestamp decoding, the enabled filter and the
// effective (later) deadline must agree on every backend.
func TestAbilitySuspensionMirrorMultiDB(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			if dialect == "sqlite" {
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1) // keep the in-memory database on one connection
			}
			snapshotProvisionalAbilityCooldowns(t)
			channelSyncLock.Lock()
			oldSuspensions := cachedAbilitySuspensions
			cachedAbilitySuspensions = nil
			channelSyncLock.Unlock()
			t.Cleanup(func() {
				channelSyncLock.Lock()
				cachedAbilitySuspensions = oldSuspensions
				channelSyncLock.Unlock()
			})

			// MySQL DATETIME(3) keeps milliseconds; compare at that precision everywhere.
			now := time.Now().UTC().Truncate(time.Millisecond)
			fixed, adaptive, later, past := now.Add(time.Minute), now.Add(30*time.Second), now.Add(time.Hour), now.Add(-time.Minute)
			rows := []Ability{
				{Group: "mirror", Model: "fixed-only", ChannelId: 1, Enabled: true, SuspendUntil: &fixed},
				{Group: "mirror", Model: "adaptive-only", ChannelId: 1, Enabled: true, AdaptiveSuspendUntil: &adaptive},
				{Group: "mirror", Model: "both", ChannelId: 1, Enabled: true, SuspendUntil: &fixed, AdaptiveSuspendUntil: &later},
				{Group: "mirror", Model: "expired-fixed", ChannelId: 1, Enabled: true, SuspendUntil: &past, AdaptiveSuspendUntil: &adaptive},
				{Group: "mirror", Model: "expired", ChannelId: 1, Enabled: true, SuspendUntil: &past, AdaptiveSuspendUntil: &past},
				{Group: "mirror", Model: "disabled", ChannelId: 1, Enabled: false, SuspendUntil: &later},
				{Group: "mirror", Model: "healthy", ChannelId: 1, Enabled: true},
			}
			require.NoError(t, db.Create(&rows).Error)

			require.NoError(t, refreshCachedAbilitySuspensionsContext(context.Background()))
			channelSyncLock.RLock()
			got := make(map[string]time.Time, len(cachedAbilitySuspensions))
			for key, cached := range cachedAbilitySuspensions {
				require.Equal(t, "mirror", key.group)
				require.Equal(t, 1, key.channelID)
				got[key.model] = cached.until.UTC().Truncate(time.Millisecond)
			}
			channelSyncLock.RUnlock()
			require.Equal(t, map[string]time.Time{
				"fixed-only":    fixed,
				"adaptive-only": adaptive,
				"both":          later,
				"expired-fixed": adaptive,
			}, got)
		})
	}
}
