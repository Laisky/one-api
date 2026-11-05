package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTransitionChannelStatusMultiDB pins the compare-and-set writer behind the automatic
// disable and re-enable paths on SQLite, MySQL (MYSQL_DSN), and PostgreSQL (PG_DSN): it
// applies only an eligible transition, reports whether it did, always realigns the
// ability routing flags, and commits even when the triggering request was cancelled.
// Parameters: t is the test handle. Returns: no values.
func TestTransitionChannelStatusMultiDB(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			ctx := context.Background()
			withStatus := func(t *testing.T, label string, status int) *Channel {
				t.Helper()
				channel := insertTestingModelMatrixChannel(t, dialect, label, nil)
				if status != ChannelStatusEnabled {
					require.NoError(t, SetChannelStatusWithContext(ctx, channel.Id, status))
				}
				return channel
			}

			t.Run("an eligible transition is applied", func(t *testing.T) {
				channel := withStatus(t, "apply", ChannelStatusEnabled)
				change, err := TransitionChannelStatusWithContext(ctx, channel.Id,
					ChannelStatusAutoDisabled, ChannelStatusEnabled)
				require.NoError(t, err)
				require.True(t, change.Changed)
				require.Equal(t, ChannelStatusEnabled, change.PreviousStatus)
				require.Equal(t, ChannelStatusAutoDisabled, change.Channel.Status)
				require.Equal(t, channel.UUID, change.Channel.UUID)
				require.Empty(t, change.Channel.Key, "the change must never carry the key")
				require.Equal(t, ChannelStatusAutoDisabled, channelStatusOf(t, channel.Id))
				require.Equal(t, []bool{false, false}, abilitiesEnabled(t, db, channel.Id))
			})

			t.Run("an ineligible status is kept", func(t *testing.T) {
				for _, status := range []int{ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled} {
					channel := withStatus(t, "keep", status)
					change, err := TransitionChannelStatusWithContext(ctx, channel.Id,
						ChannelStatusAutoDisabled, ChannelStatusEnabled)
					require.NoError(t, err)
					require.False(t, change.Changed)
					require.Equal(t, status, change.PreviousStatus)
					require.Equal(t, status, channelStatusOf(t, channel.Id))
				}
			})

			t.Run("drifted abilities are realigned without a transition", func(t *testing.T) {
				channel := withStatus(t, "drift", ChannelStatusAutoDisabled)
				require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).
					Update("enabled", true).Error)
				change, err := TransitionChannelStatusWithContext(ctx, channel.Id,
					ChannelStatusAutoDisabled, ChannelStatusEnabled)
				require.NoError(t, err)
				require.False(t, change.Changed)
				require.Equal(t, []bool{false, false}, abilitiesEnabled(t, db, channel.Id))
			})

			t.Run("a cancelled request still commits", func(t *testing.T) {
				channel := withStatus(t, "cancel", ChannelStatusAutoDisabled)
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				change, err := TransitionChannelStatusWithContext(cancelled, channel.Id,
					ChannelStatusEnabled, ChannelStatusAutoDisabled)
				require.NoError(t, err)
				require.True(t, change.Changed)
				require.Equal(t, ChannelStatusEnabled, channelStatusOf(t, channel.Id))
				require.Equal(t, []bool{true, true}, abilitiesEnabled(t, db, channel.Id))
			})

			t.Run("invalid arguments are refused", func(t *testing.T) {
				channel := withStatus(t, "invalid", ChannelStatusEnabled)
				_, err := TransitionChannelStatusWithContext(ctx, channel.Id, ChannelStatusAutoDisabled)
				require.Error(t, err, "a transition must name the statuses it may start from")
				_, err = TransitionChannelStatusWithContext(ctx, channel.Id, ChannelStatusUnknown, ChannelStatusEnabled)
				require.Error(t, err)
				require.Equal(t, ChannelStatusEnabled, channelStatusOf(t, channel.Id))
			})
		})
	}
}

// channelStatusOf reads a channel's stored status from the package database.
// Parameters: t is the test handle and id identifies the channel. Returns: the status.
func channelStatusOf(t *testing.T, id int) int {
	t.Helper()
	var channel Channel
	require.NoError(t, DB.Select("id", "status").First(&channel, "id = ?", id).Error)
	return channel.Status
}
