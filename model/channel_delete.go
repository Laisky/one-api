package model

import (
	"context"
	"slices"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// deleteChannelsWithStatus removes every channel whose status is one of statuses, together
// with its abilities and channel_groups rows, in one transaction, then refreshes the
// model-list caches of the groups those channels served.
//
// The candidate rows are write-locked and their status re-read under the lock before
// anything is deleted, so a channel re-enabled concurrently is neither deleted nor left
// without its abilities and groups.
//
// Parameters:
//   - ctx: lifecycle and logging scope of the delete.
//   - statuses: the channel statuses to delete.
//
// Return values:
//   - int64: the number of channels removed.
//   - error: a wrapped failure; nothing is removed when it is non-nil.
func deleteChannelsWithStatus(ctx context.Context, statuses ...int) (int64, error) {
	var (
		removed int64
		groups  []string
	)
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidates []int
		if err := tx.Model(&Channel{}).Where("status IN ?", statuses).Order("id").Pluck("id", &candidates).Error; err != nil {
			return errors.Wrap(err, "list channels to delete")
		}
		for batch := range slices.Chunk(candidates, abilitySyncBatchSize) {
			if err := tx.Model(&Channel{}).Where("id IN ?", batch).UpdateColumn("status", gorm.Expr("status")).Error; err != nil {
				return errors.Wrapf(err, "lock %d channels before deletion", len(batch))
			}
			var doomed []Channel
			if err := tx.Select("id", "group").Where("id IN ? AND status IN ?", batch, statuses).Find(&doomed).Error; err != nil {
				return errors.Wrapf(err, "re-read %d locked channels before deletion", len(batch))
			}
			if len(doomed) == 0 {
				continue
			}
			ids := make([]int, 0, len(doomed))
			for _, channel := range doomed {
				ids = append(ids, channel.Id)
				groups = append(groups, channel.Group)
			}
			if err := tx.Where("channel_id IN ?", ids).Delete(&Ability{}).Error; err != nil {
				return errors.Wrapf(err, "delete abilities of %d channels", len(ids))
			}
			if err := deleteChannelGroupsWithDB(tx, ids...); err != nil {
				return errors.Wrapf(err, "delete groups of %d channels", len(ids))
			}
			result := tx.Where("id IN ?", ids).Delete(&Channel{})
			if result.Error != nil {
				return errors.Wrapf(result.Error, "delete %d channels", len(ids))
			}
			removed += result.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, errors.Wrap(err, "delete channels by status transaction")
	}
	if removed > 0 {
		InvalidateChannelModelCachesWithContext(ctx, groups...)
	}
	return removed, nil
}
