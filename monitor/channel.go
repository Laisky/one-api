package monitor

import (
	"context"
	"fmt"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/message"
	"github.com/Laisky/one-api/model"
)

// resolveChannelRef builds the fullest available reference for a channel that is
// being enabled or disabled. Channel status changes are rare events, so the
// lookup (served from the in-memory channel snapshot) is affordable here.
//
// Parameters:
//   - channelId: channel primary key.
//   - channelName: name already known by the caller, used as a fallback when the
//     snapshot cannot resolve the channel (e.g. it was just removed).
//
// Return values:
//   - identity.ChannelRef: reference carrying id plus whatever uuid/name exist.
func resolveChannelRef(channelId int, channelName string) identity.ChannelRef {
	ref := model.LookupChannelRef(context.Background(), channelId)
	if ref.Name == "" && channelName != "" {
		ref = identity.NewChannelRef(channelId, ref.UUID, channelName)
	}
	return ref
}

func notifyRootUser(subject string, content string) {
	if config.MessagePusherAddress != "" {
		err := message.SendMessage(subject, content, content)
		if err != nil {
			logger.Logger.Error("failed to send message", zap.Error(err))
		} else {
			return
		}
	}
	if config.RootUserEmail == "" {
		config.RootUserEmail = model.GetRootUserEmail()
	}
	err := message.SendEmail(subject, config.RootUserEmail, content)
	if err != nil {
		// Deliberately no recipient field: rule 7 forbids logging email addresses.
		logger.Logger.Error("failed to send email", zap.Error(err))
	}
}

// applyAutomaticStatusChange persists an automatic status transition and reports whether
// it happened, so its caller announces only a committed change.
//
// Parameters:
//   - channelId: the channel to transition.
//   - channelName: its name, used in logs when the channel snapshot cannot resolve it.
//   - status: the target status.
//   - from: the statuses the transition may start from.
//
// Return values:
//   - bool: true when the transition was committed; failures are logged here.
func applyAutomaticStatusChange(channelId int, channelName string, status int, from ...int) bool {
	change, err := model.TransitionChannelStatusWithContext(context.Background(), channelId, status, from...)
	if err != nil {
		logger.Logger.Error("failed to persist automatic channel status change; not announcing it",
			resolveChannelRef(channelId, channelName).AppendZap([]zap.Field{
				zap.Int("requested_status", status), zap.Error(err)})...)
		return false
	}
	if !change.Changed {
		logger.Logger.Debug("automatic channel status change not applied: current status is not eligible",
			resolveChannelRef(channelId, channelName).AppendZap([]zap.Field{
				zap.Int("current_status", change.PreviousStatus), zap.Int("requested_status", status)})...)
	}
	return change.Changed
}

// DisableChannel automatically disables an enabled channel and announces it. A channel
// that is already disabled, automatically or by an administrator, keeps its status and
// is not announced again; a disable that cannot be persisted is logged, not announced.
//
// Parameters:
//   - channelId: the channel to disable.
//   - channelName: its name, used when the channel snapshot cannot resolve it.
//   - reason: why the channel is disabled.
//
// Return values: none.
func DisableChannel(channelId int, channelName string, reason string) {
	if !applyAutomaticStatusChange(channelId, channelName, model.ChannelStatusAutoDisabled, model.ChannelStatusEnabled) {
		return
	}
	NotifyChannelDisabled(channelId, channelName, reason)
}

// NotifyChannelDisabled logs and announces to the root user that a channel was
// disabled. It changes no state, so callers that persisted the transition
// themselves (such as the health-check sweep) announce only what actually happened.
//
// Parameters:
//   - channelId: the disabled channel.
//   - channelName: its name, used when the channel snapshot cannot resolve it.
//   - reason: why the channel was disabled.
//
// Return values: none; delivery failures are logged.
func NotifyChannelDisabled(channelId int, channelName string, reason string) {
	ref := resolveChannelRef(channelId, channelName)
	logger.Logger.Info("channel has been disabled",
		ref.AppendZap([]zap.Field{zap.String("reason", reason)})...)
	subject := "Channel Status Change Reminder"
	content := message.EmailTemplate(
		subject,
		fmt.Sprintf(`
            <p>Hello!</p>
            <p><strong>%s</strong> has been disabled.</p>
            <p>Reason for disabling:</p>
            <p style="background-color: #f8f8f8; padding: 10px; border-radius: 4px;">%s</p>
        `, ref.String(), reason),
	)
	notifyRootUser(subject, content)
}

// MetricDisableChannel automatically disables an enabled channel whose success rate fell
// below the threshold and announces it, under the same rules as DisableChannel.
//
// Parameters:
//   - channelId: the channel to disable.
//   - successRate: the observed success rate, between 0 and 1.
//
// Return values: none.
func MetricDisableChannel(channelId int, successRate float64) {
	if !applyAutomaticStatusChange(channelId, "", model.ChannelStatusAutoDisabled, model.ChannelStatusEnabled) {
		return
	}
	ref := resolveChannelRef(channelId, "")
	logger.Logger.Info("channel has been disabled due to low success rate",
		ref.AppendZap([]zap.Field{zap.Float64("success_rate", successRate*100)})...)
	subject := "Channel Status Change Reminder"
	content := message.EmailTemplate(
		subject,
		fmt.Sprintf(`
            <p>Hello!</p>
            <p><strong>%s</strong> has been automatically disabled by the system.</p>
            <p>Reason for disabling:</p>
            <p style="background-color: #f8f8f8; padding: 10px; border-radius: 4px;">In the last %d calls, the success rate of this channel was <strong>%.2f%%</strong>, which is below the system threshold of <strong>%.2f%%</strong>.</p>
        `, ref.String(), config.MetricQueueSize, successRate*100, config.MetricSuccessRateThreshold*100),
	)
	notifyRootUser(subject, content)
}

// EnableChannel automatically re-enables a channel that was disabled automatically and
// announces it. A channel an administrator disabled stays disabled; a re-enable that
// cannot be persisted is logged, not announced.
//
// Parameters:
//   - channelId: the channel to re-enable.
//   - channelName: its name, used when the channel snapshot cannot resolve it.
//
// Return values: none.
func EnableChannel(channelId int, channelName string) {
	if !applyAutomaticStatusChange(channelId, channelName, model.ChannelStatusEnabled, model.ChannelStatusAutoDisabled) {
		return
	}
	NotifyChannelEnabled(channelId, channelName)
}

// NotifyChannelEnabled logs and announces to the root user that a channel was
// re-enabled. It changes no state.
//
// Parameters:
//   - channelId: the re-enabled channel.
//   - channelName: its name, used when the channel snapshot cannot resolve it.
//
// Return values: none; delivery failures are logged.
func NotifyChannelEnabled(channelId int, channelName string) {
	ref := resolveChannelRef(channelId, channelName)
	logger.Logger.Info("channel has been enabled", ref.Zap()...)
	subject := "Channel Status Change Reminder"
	content := message.EmailTemplate(
		subject,
		fmt.Sprintf(`
            <p>Hello!</p>
            <p><strong>%s</strong> has been re-enabled.</p>
            <p>You can now continue using this channel.</p>
        `, ref.String()),
	)
	notifyRootUser(subject, content)
}
