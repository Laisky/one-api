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

// DisableChannel disable & notify
func DisableChannel(channelId int, channelName string, reason string) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusAutoDisabled)
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

func MetricDisableChannel(channelId int, successRate float64) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusAutoDisabled)
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

// EnableChannel enable & notify
func EnableChannel(channelId int, channelName string) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusEnabled)
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
