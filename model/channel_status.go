package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/logger"
)

// SetChannelStatusWithContext applies the requested status to the channel and
// its routing abilities in one transaction. The context bounds database work;
// id identifies the channel, and status must be enabled, manually disabled, or
// automatically disabled. It returns a wrapped error without partial changes.
// Unlike the legacy background helper, this function lets an interactive caller
// distinguish persistence failures from successful updates.
func SetChannelStatusWithContext(ctx context.Context, id, status int) error {
	if id <= 0 || (status != ChannelStatusEnabled && status != ChannelStatusManuallyDisabled && status != ChannelStatusAutoDisabled) {
		return errors.WithStack(errors.New("invalid channel status update"))
	}
	var channel Channel
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize with reset/delete/configuration writes before reading. A no-op
		// update locks the row on all supported backends without timestamp hooks.
		// MySQL may report zero affected rows for a no-op, so read to check existence.
		if err := tx.Model(&Channel{}).Where("id = ?", id).UpdateColumn("status", gorm.Expr("status")).Error; err != nil {
			return errors.Wrap(err, "lock channel for status update")
		}
		if err := tx.Select("id", "uuid", "name", "group").First(&channel, "id = ?", id).Error; err != nil {
			return errors.Wrap(err, "read channel for status update")
		}
		if err := tx.Model(&Channel{}).Where("id = ?", id).Update("status", status).Error; err != nil {
			return errors.Wrap(err, "persist channel status")
		}
		if err := tx.Model(&Ability{}).Where("channel_id = ?", id).Update("enabled", status == ChannelStatusEnabled).Error; err != nil {
			return errors.Wrap(err, "persist channel ability status")
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "update channel status transaction")
	}
	InvalidateChannelModelCachesWithContext(ctx, channel.Group)
	logger.FromContext(ctx).Debug("channel status updated", append(channel.Ref().Zap(), zap.Int("status", status))...)
	return nil
}
