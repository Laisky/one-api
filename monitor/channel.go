package monitor

import (
	"fmt"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/message"
	"github.com/Laisky/one-api/model"
)

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
		logger.Logger.Error("failed to send email", zap.String("email", config.RootUserEmail), zap.Error(err))
	}
}

// DisableChannel disable & notify
func DisableChannel(channelId int, channelName string, reason string) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusAutoDisabled)
	logger.Logger.Info("channel has been disabled", zap.Int("id", channelId), zap.String("reason", reason))
	subject := fmt.Sprintf("Channel Status Change Reminder")
	content := message.EmailTemplate(
		subject,
		fmt.Sprintf(`
            <p>Hello!</p>
            <p>Channel “<strong>%s</strong>” (#%d) has been disabled.</p>
            <p>Reason for disabling:</p>
            <p style="background-color: #f8f8f8; padding: 10px; border-radius: 4px;">%s</p>
        `, channelName, channelId, reason),
	)
	notifyRootUser(subject, content)
}

func MetricDisableChannel(channelId int, successRate float64) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusAutoDisabled)
	logger.Logger.Info("channel has been disabled due to low success rate", zap.Int("id", channelId), zap.Float64("success_rate", successRate*100))
	subject := fmt.Sprintf("Channel Status Change Reminder")
	content := message.EmailTemplate(
		subject,
		fmt.Sprintf(`
            <p>Hello!</p>
            <p>Channel #%d has been automatically disabled by the system.</p>
            <p>Reason for disabling:</p>
            <p style="background-color: #f8f8f8; padding: 10px; border-radius: 4px;">In the last %d calls, the success rate of this channel was <strong>%.2f%%</strong>, which is below the system threshold of <strong>%.2f%%</strong>.</p>
        `, channelId, config.MetricQueueSize, successRate*100, config.MetricSuccessRateThreshold*100),
	)
	notifyRootUser(subject, content)
}

// EnableChannel enable & notify
func EnableChannel(channelId int, channelName string) {
	model.UpdateChannelStatusById(channelId, model.ChannelStatusEnabled)
	logger.Logger.Info("channel has been enabled", zap.Int("id", channelId))
	subject := fmt.Sprintf("Channel Status Change Reminder")
	content := message.EmailTemplate(
		subject,
		fmt.Sprintf(`
            <p>Hello!</p>
            <p>Channel “<strong>%s</strong>” (#%d) has been re-enabled.</p>
            <p>You can now continue using this channel.</p>
        `, channelName, channelId),
	)
	notifyRootUser(subject, content)
}
