package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// The cross-replica cooldown mirror.
//
// Each replica publishes the cooldowns it persists into its own cache at once,
// but a cooldown persisted by ANOTHER replica used to reach this cache only at
// the next full rebuild (SYNC_FREQUENCY, default 120 s). The adaptive ladder's
// first rungs are 10 s and 30 s, so most of them expired unseen. The mirror
// re-reads only the active deadlines every CHANNEL_SUSPENSION_SYNC_SECONDS and
// replaces cachedAbilitySuspensions with them, so foreign cooldowns and foreign
// lifts (recoveries, re-enables) take effect within one interval.

// abilityDeadlineRow is one active ability cooldown read by the mirror.
type abilityDeadlineRow struct {
	Group                string
	Model                string
	ChannelId            int
	SuspendUntil         *time.Time
	AdaptiveSuspendUntil *time.Time
	CooldownRevision     int64
}

// activeAbilityDeadlinesSQL returns the mirror's read: the enabled abilities
// whose fixed or adaptive deadline is at or after the bound time. Each UNION ALL
// branch is a range read of one deadline index (idx_abilities_suspend_until,
// idx_abilities_adaptive_suspend_until), so expired and never-suspended rows are
// not visited and the cost scales with the active cooldowns, not the table. A
// row active in both columns is returned twice; the caller merges by key. It
// selects only the key, both deadlines and the cooldown revision that orders
// them against this process's own publications. Bind (true, now, true, now).
//
// Returns:
//   - string: the dialect-quoted SQL.
func activeAbilityDeadlinesSQL() string {
	groupCol := "`group`"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
	}
	columns := "SELECT " + groupCol + ", model, channel_id, suspend_until, adaptive_suspend_until, cooldown_revision FROM abilities WHERE enabled = ? AND "
	return columns + "suspend_until >= ? UNION ALL " + columns + "adaptive_suspend_until >= ?"
}

// refreshCachedAbilitySuspensionsContext replaces the cached persisted deadlines
// with the effective deadline (the later of suspend_until and
// adaptive_suspend_until) of every enabled ability that is cooling now, exactly
// the active subset of what a rebuild publishes. Both columns are read, so an
// adaptive-only cooldown is never erased. The swap merges the snapshot with
// mergeAbilityDeadlineSnapshotLocked, so a cooldown this process persists and
// publishes concurrently survives the older snapshot, and cooldown writers never
// wait for this read (abilitySnapshotLock only orders it against rebuilds).
// Provisional cooldowns live in a separate overlay and are not touched.
//
// Parameters:
//   - ctx: lifecycle scope for the database read.
//
// Return values:
//   - error: a wrapped database failure; the previous deadlines stay in place.
func refreshCachedAbilitySuspensionsContext(ctx context.Context) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	abilitySnapshotLock.Lock()
	defer abilitySnapshotLock.Unlock()
	startSeq := beginAbilityDeadlineSnapshot()
	now := time.Now().UTC()
	var rows []abilityDeadlineRow
	if err := DB.WithContext(ctx).Raw(activeAbilityDeadlinesSQL(), true, now, true, now).Find(&rows).Error; err != nil {
		return errors.Wrap(err, "list active ability cooldown deadlines")
	}
	observed := make(map[cachedAbilityKey]cachedAbilityDeadline, len(rows))
	for i := range rows {
		row := &rows[i]
		deadline := abilityEffectiveDeadline(&Ability{SuspendUntil: row.SuspendUntil, AdaptiveSuspendUntil: row.AdaptiveSuspendUntil})
		if deadline != nil {
			observed[cachedAbilityKey{row.Group, row.Model, row.ChannelId}] = cachedAbilityDeadline{until: *deadline, revision: row.CooldownRevision}
		}
	}
	channelSyncLock.Lock()
	previous := len(cachedAbilitySuspensions)
	next := mergeAbilityDeadlineSnapshotLocked(observed, startSeq)
	cachedAbilitySuspensions = next
	channelSyncLock.Unlock()
	if previous != len(next) {
		logger.Logger.Debug("ability cooldown mirror published persisted deadlines",
			zap.Int("active_cooldowns", len(next)), zap.Int("previous_cached_deadlines", previous))
	}
	return nil
}

// SyncAbilitySuspensionsContext runs the cooldown mirror every interval until
// ctx is cancelled. It bounds how long this replica routes to an ability that
// another replica cooled (or keeps avoiding one that another replica recovered)
// to one interval plus the read, instead of up to a full rebuild period. Ticks
// are skipped while the memory cache is disabled, because database routing reads
// the deadlines directly. A failing read keeps the previous deadlines and is
// logged on the transition only.
//
// Parameters:
//   - ctx: lifecycle scope; cancellation ends the loop at the next tick.
//   - interval: time between refreshes; values <= 0 disable the mirror.
func SyncAbilitySuspensionsContext(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		logger.Logger.Info("ability cooldown mirror disabled", zap.Duration("interval", interval))
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !config.MemoryCacheEnabled {
			continue
		}
		err := refreshCachedAbilitySuspensionsContext(ctx)
		switch {
		case err != nil && ctx.Err() != nil:
			return
		case err != nil && !failing:
			failing = true
			logger.Logger.Warn("ability cooldown mirror failed; keeping cached cooldowns", zap.Error(err))
		case err != nil:
			logger.Logger.Debug("ability cooldown mirror still failing", zap.Error(err))
		case failing:
			failing = false
			logger.Logger.Info("ability cooldown mirror recovered")
		}
	}
}
