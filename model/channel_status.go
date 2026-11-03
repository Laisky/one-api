package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/helper"
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
		// Serialize with reset/delete/configuration writes before reading. MySQL
		// may report zero affected rows for the no-op lock, so read to check existence.
		if err := lockChannelRow(tx, id); err != nil {
			return errors.Wrap(err, "lock channel for status update")
		}
		if err := tx.Select("id", "uuid", "name", "group").First(&channel, "id = ?", id).Error; err != nil {
			return errors.Wrap(err, "read channel for status update")
		}
		if err := persistChannelStatusWithDB(tx, id, status); err != nil {
			return errors.Wrap(err, "persist channel status update")
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

// persistChannelStatusWithDB writes a channel status and the matching ability
// routing flag inside the caller's transaction.
//
// Parameters:
//   - tx: the transaction holding the channel row lock.
//   - id: the channel to update.
//   - status: the new channel status.
//
// Return values:
//   - error: a wrapped database failure.
func persistChannelStatusWithDB(tx *gorm.DB, id, status int) error {
	if err := tx.Model(&Channel{}).Where("id = ?", id).Update("status", status).Error; err != nil {
		return errors.Wrap(err, "persist channel status")
	}
	if err := tx.Model(&Ability{}).Where("channel_id = ?", id).Update("enabled", status == ChannelStatusEnabled).Error; err != nil {
		return errors.Wrap(err, "persist channel ability status")
	}
	return nil
}

// ChannelProbeResult is what a finished automatic health-check probe asks to persist.
type ChannelProbeResult struct {
	// ObservedStatus is the channel status the sweep saw when it probed the channel.
	ObservedStatus int
	// TargetStatus is the status the result calls for; zero keeps the current status.
	TargetStatus int
	// ResponseTimeMs is the measured probe latency in milliseconds.
	ResponseTimeMs int64
}

// ChannelProbeOutcome reports what ApplyChannelProbeResultWithContext persisted.
type ChannelProbeOutcome struct {
	// OptedOut is true when the channel was set to SKIP; nothing was persisted.
	OptedOut bool
	// StatusChanged is true when the requested status transition was applied.
	StatusChanged bool
	// Channel identifies the probed channel as read under the row lock (id, uuid,
	// name, group, status after the update, and testing_model).
	Channel Channel
}

// ApplyChannelProbeResultWithContext persists an automatic health-check result
// atomically with the checks that make it valid. In one transaction that holds the
// channel row lock it:
//   - discards the result when the channel is now set to SKIP, so an opt-out saved
//     while the probe was in flight can never be overridden;
//   - applies TargetStatus (status and ability routing) only if the status still
//     equals ObservedStatus, so a manual status change made during the probe wins and
//     a transition that already happened is not repeated;
//   - records the response time.
//
// The probe itself must run before this call; no lock is held during network I/O.
// Any failure rolls everything back, so a caller that cannot persist the result also
// cannot act on it (fail closed).
//
// Parameters:
//   - ctx: sweep scope for the transaction.
//   - id: the probed channel.
//   - result: the probe's observed status, requested status, and latency.
//
// Return values:
//   - ChannelProbeOutcome: whether the result was discarded or changed the status.
//   - error: a wrapped database failure; nothing was persisted.
func ApplyChannelProbeResultWithContext(ctx context.Context, id int, result ChannelProbeResult) (ChannelProbeOutcome, error) {
	var outcome ChannelProbeOutcome
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChannelRow(tx, id); err != nil {
			return errors.Wrap(err, "lock channel for probe result")
		}
		var current Channel
		if err := tx.Select("id", "uuid", "name", "group", "status", ChannelTestingModelField).
			First(&current, "id = ?", id).Error; err != nil {
			return errors.Wrap(err, "read locked channel for probe result")
		}
		outcome.Channel = current
		if current.IsTestingSkipped() {
			outcome.OptedOut = true
			return nil
		}
		target := result.TargetStatus
		if target != 0 && target != current.Status && current.Status == result.ObservedStatus {
			if err := persistChannelStatusWithDB(tx, id, target); err != nil {
				return errors.Wrap(err, "apply probe status transition")
			}
			outcome.StatusChanged = true
			outcome.Channel.Status = target
		}
		if err := tx.Model(&Channel{}).Where("id = ?", id).Select("response_time", "test_time").
			Updates(Channel{TestTime: helper.GetTimestamp(), ResponseTime: int(result.ResponseTimeMs)}).Error; err != nil {
			return errors.Wrap(err, "record probe response time")
		}
		return nil
	})
	if err != nil {
		return ChannelProbeOutcome{}, errors.Wrapf(err, "apply probe result to channel %d", id)
	}
	if outcome.StatusChanged {
		InvalidateChannelModelCachesWithContext(ctx, outcome.Channel.Group)
	}
	return outcome, nil
}
