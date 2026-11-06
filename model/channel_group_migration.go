package model

import (
	"context"
	"slices"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/groupname"
	"github.com/Laisky/one-api/common/logger"
)

// channelGroupReconcileBatchSize bounds the channels one reconciliation transaction locks.
const channelGroupReconcileBatchSize = 200

// ChannelGroupReconcileResult counts what ReconcileChannelGroups changed.
type ChannelGroupReconcileResult struct {
	// Backfilled counts channels whose relation rows were written from channels.group
	// because no relation-aware writer had stored them yet.
	Backfilled int
	// Repaired counts truncated channels whose channels.group was rewritten after the
	// relation (by an older binary or by hand); their relation now follows the column.
	Repaired int
	// Unrecoverable counts truncated channels that have no relation rows at all, for
	// example after a partial copy between databases. They keep reading the column.
	Unrecoverable int
	// AbilitiesRealigned counts truncated channels whose abilities did not match their
	// complete group list. An older binary rebuilds abilities from the column alone, so
	// re-saving such a channel there drops the routes of the groups past the column.
	AbilitiesRealigned int
}

// ReconcileChannelGroups is the automatic startup migration of the channel-to-group
// relation. It runs on the master node after AutoMigrate and is safe on every start:
//
//   - Channels whose group_truncated is NULL were written before the relation existed or
//     by an older binary, and their channels.group holds the complete list. Their relation
//     rows are rewritten from the column and the flag is set to false. On the first start
//     after an upgrade this is the backfill; afterwards it picks up channels an older binary
//     created during a rollback.
//   - Channels marked truncated are checked against their relation. When an older binary
//     rewrote channels.group after the relation was written, the column is the newer intent:
//     the relation is rebuilt from it and the flag cleared. Their abilities are then brought
//     back in line with the effective list, since an older binary that re-saved the channel
//     rebuilt them from the column alone.
//
// Both steps read through the index on group_truncated: steady state touches no row, and
// channels with complete columns, the common case, are never visited. Each batch locks its
// rows and re-reads them before writing, so a concurrent channel save is never overwritten.
// Reads never depend on this migration having run (see hydrateChannelGroups); it keeps the
// relation complete and repairs drift.
//
// Parameters:
//   - ctx: lifecycle and logging scope of the migration.
//
// Return values:
//   - ChannelGroupReconcileResult: what was changed.
//   - error: a wrapped failure; batches committed before it stay committed.
func ReconcileChannelGroups(ctx context.Context) (ChannelGroupReconcileResult, error) {
	var result ChannelGroupReconcileResult
	if DB == nil {
		return result, errors.New("database not initialized")
	}
	db := DB.WithContext(ctx)

	var pending []int
	if err := db.Model(&Channel{}).Where("group_truncated IS NULL").Order("id").Pluck("id", &pending).Error; err != nil {
		return result, errors.Wrap(err, "list channels without group relation")
	}
	for batch := range slices.Chunk(pending, channelGroupReconcileBatchSize) {
		backfilled, err := backfillChannelGroups(db, batch)
		if err != nil {
			return result, errors.Wrapf(err, "backfill groups of %d channels", len(batch))
		}
		result.Backfilled += backfilled
	}

	var truncated []int
	if err := db.Model(&Channel{}).Where("group_truncated = ?", true).Order("id").Pluck("id", &truncated).Error; err != nil {
		return result, errors.Wrap(err, "list channels with truncated group column")
	}
	for batch := range slices.Chunk(truncated, channelGroupReconcileBatchSize) {
		batchResult, err := repairTruncatedChannelGroups(ctx, db, batch)
		if err != nil {
			return result, errors.Wrapf(err, "verify groups of %d truncated channels", len(batch))
		}
		result.Repaired += batchResult.Repaired
		result.Unrecoverable += batchResult.Unrecoverable
		result.AbilitiesRealigned += batchResult.AbilitiesRealigned
	}

	if result != (ChannelGroupReconcileResult{}) {
		logger.FromContext(ctx).Info("reconciled channel group relation",
			zap.Int("backfilled", result.Backfilled),
			zap.Int("repaired", result.Repaired),
			zap.Int("unrecoverable", result.Unrecoverable),
			zap.Int("abilities_realigned", result.AbilitiesRealigned))
	}
	return result, nil
}

// lockedRawChannelGroupColumns write-locks the channels in ids and reads their stored
// channels.group values, restricted by the extra condition. The read skips hydration so
// it sees exactly what the column holds.
//
// Parameters:
//   - tx: the reconciliation transaction.
//   - ids: the channels to lock and read.
//   - condition, args: the predicate a row must still satisfy once locked.
//
// Return values:
//   - []Channel: the matching rows, carrying their identity and group column only.
//   - error: a wrapped lock or read failure.
func lockedRawChannelGroupColumns(tx *gorm.DB, ids []int, condition string, args ...any) ([]Channel, error) {
	if err := tx.Model(&Channel{}).Where("id IN ?", ids).UpdateColumn("models", gorm.Expr("models")).Error; err != nil {
		return nil, errors.Wrapf(err, "lock %d channels", len(ids))
	}
	var rows []Channel
	if err := tx.Session(&gorm.Session{SkipHooks: true}).Select("id", "uuid", "name", "group").
		Where("id IN ?", ids).Where(condition, args...).Order("id").Find(&rows).Error; err != nil {
		return nil, errors.Wrapf(err, "read group column of %d channels", len(ids))
	}
	return rows, nil
}

// backfillChannelGroups writes the relation rows of the channels in ids that still have no
// group_truncated value, from their complete channels.group column, and marks them complete.
//
// Parameters:
//   - db: the context-bound primary handle.
//   - ids: the channels to backfill.
//
// Return values:
//   - int: the number of channels backfilled.
//   - error: a wrapped failure; the batch is rolled back.
func backfillChannelGroups(db *gorm.DB, ids []int) (int, error) {
	backfilled := 0
	err := db.Transaction(func(tx *gorm.DB) error {
		rows, err := lockedRawChannelGroupColumns(tx, ids, "group_truncated IS NULL")
		if err != nil {
			return errors.Wrap(err, "read channels to backfill")
		}
		if len(rows) == 0 {
			return nil
		}
		done := make([]int, 0, len(rows))
		for _, row := range rows {
			if err := replaceChannelGroupsWithDB(tx, row.Id, groupname.ParseList(row.Group)); err != nil {
				return errors.Wrapf(err, "backfill groups of channel %d", row.Id)
			}
			done = append(done, row.Id)
		}
		if err := tx.Model(&Channel{}).Where("id IN ?", done).UpdateColumn(channelGroupTruncatedColumn, false).Error; err != nil {
			return errors.Wrapf(err, "mark %d channels complete", len(done))
		}
		backfilled = len(done)
		return nil
	})
	if err != nil {
		return 0, errors.Wrap(err, "backfill channel groups transaction")
	}
	return backfilled, nil
}

// repairTruncatedChannelGroups checks the channels in ids that are marked truncated. A
// channel whose column no longer matches its relation gets the relation rebuilt from the
// column and its flag cleared, because only a writer unaware of the relation changes the
// column alone. Every checked channel then has its abilities realigned with its effective
// group list. A channel with no relation rows is reported and left as it is.
//
// Parameters:
//   - ctx: logging scope.
//   - db: the context-bound primary handle.
//   - ids: the truncated channels to check.
//
// Return values:
//   - ChannelGroupReconcileResult: the batch's Repaired, Unrecoverable and AbilitiesRealigned counts.
//   - error: a wrapped failure; the batch is rolled back.
func repairTruncatedChannelGroups(ctx context.Context, db *gorm.DB, ids []int) (ChannelGroupReconcileResult, error) {
	var result ChannelGroupReconcileResult
	err := db.Transaction(func(tx *gorm.DB) error {
		result = ChannelGroupReconcileResult{}
		rows, err := lockedRawChannelGroupColumns(tx, ids, "group_truncated = ?", true)
		if err != nil {
			return errors.Wrap(err, "read truncated channels")
		}
		if len(rows) == 0 {
			return nil
		}
		rowIDs := make([]int, 0, len(rows))
		for _, row := range rows {
			rowIDs = append(rowIDs, row.Id)
		}
		relations, err := loadChannelGroupLists(tx, rowIDs)
		if err != nil {
			return errors.Wrap(err, "read relation of truncated channels")
		}
		lg := logger.FromContext(ctx)
		checked := make([]int, 0, len(rows))
		for _, row := range rows {
			relation := relations[row.Id]
			if len(relation) == 0 {
				result.Unrecoverable++
				lg.Warn("channel group column is marked truncated but channel_groups has no rows; the channel serves only the groups in channels.group",
					row.Ref().Zap()...)
				continue
			}
			checked = append(checked, row.Id)
			if _, consistent := resolveChannelGroupList(row.Group, relation); consistent {
				continue
			}
			lg.Warn("channels.group was rewritten without the channel group relation, probably by an older binary; rebuilding the relation from the column",
				append(row.Ref().Zap(), zap.Int("relation_groups", len(relation)))...)
			if err := replaceChannelGroupsWithDB(tx, row.Id, groupname.ParseList(row.Group)); err != nil {
				return errors.Wrapf(err, "rebuild groups of channel %d", row.Id)
			}
			if err := tx.Model(&Channel{}).Where("id = ?", row.Id).UpdateColumn(channelGroupTruncatedColumn, false).Error; err != nil {
				return errors.Wrapf(err, "mark channel %d complete", row.Id)
			}
			result.Repaired++
		}
		realigned, err := realignChannelAbilities(tx, checked)
		if err != nil {
			return errors.Wrap(err, "realign abilities of truncated channels")
		}
		result.AbilitiesRealigned = realigned
		return nil
	})
	if err != nil {
		return ChannelGroupReconcileResult{}, errors.Wrap(err, "repair truncated channel groups transaction")
	}
	return result, nil
}

// realignChannelAbilities makes the abilities of the channels in ids match their effective
// group lists inside the caller's transaction, keeping the suspensions of retained ones.
// Channels whose abilities already match cost one read and no write.
//
// Parameters:
//   - tx: the transaction holding the channels' row locks.
//   - ids: the channels to realign.
//
// Return values:
//   - int: the number of channels whose abilities were rewritten.
//   - error: a wrapped read or write failure.
func realignChannelAbilities(tx *gorm.DB, ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var channels []*Channel
	if err := tx.Where("id IN ?", ids).Order("id").Find(&channels).Error; err != nil {
		return 0, errors.Wrapf(err, "read %d channels", len(ids))
	}
	realigned := 0
	for _, channel := range channels {
		var existing []Ability
		if err := tx.Select("group", "model").Where("channel_id = ?", channel.Id).Find(&existing).Error; err != nil {
			return 0, errors.Wrapf(err, "read abilities of channel %d", channel.Id)
		}
		if sameAbilityKeys(existing, buildChannelAbilities(channel)) {
			continue
		}
		if err := syncAbilitiesWithDB(tx, channel, false); err != nil {
			return 0, errors.Wrapf(err, "sync abilities of channel %d", channel.Id)
		}
		realigned++
	}
	return realigned, nil
}

// sameAbilityKeys reports whether two ability lists cover the same (group, model) pairs.
//
// Parameters:
//   - existing: the stored abilities.
//   - desired: the abilities the channel's configuration calls for.
//
// Return values:
//   - bool: true when both lists hold exactly the same pairs.
func sameAbilityKeys(existing, desired []Ability) bool {
	if len(existing) != len(desired) {
		return false
	}
	keys := make(map[abilityKey]struct{}, len(desired))
	for _, ability := range desired {
		keys[abilityKey{group: ability.Group, model: ability.Model}] = struct{}{}
	}
	for _, ability := range existing {
		if _, ok := keys[abilityKey{group: ability.Group, model: ability.Model}]; !ok {
			return false
		}
	}
	return true
}
