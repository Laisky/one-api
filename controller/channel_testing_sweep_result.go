package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/message"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/monitor"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// channelSweepNotice is a notification sent when auto-disable is off and a probe failed.
type channelSweepNotice struct {
	title   string
	content string
}

// channelSweepPlan is what a finished sweep probe calls for, decided before anything is
// persisted so the database transaction alone decides whether it still applies.
type channelSweepPlan struct {
	// targetStatus is the status transition requested; zero keeps the status.
	targetStatus int
	// reason explains an automatic disable.
	reason string
	// notices are sent instead of disabling when auto-disable is off.
	notices []channelSweepNotice
}

// channelSweepNotifier delivers the notifications a persisted sweep result calls for.
// It is a variable so tests can observe exactly which announcements are made.
var channelSweepNotifier = struct {
	disabled func(channelID int, channelName, reason string)
	enabled  func(channelID int, channelName string)
	notice   func(title, content string) error
}{
	disabled: monitor.NotifyChannelDisabled,
	enabled:  monitor.NotifyChannelEnabled,
	notice: func(title, content string) error {
		return message.Notify(message.ByAll, title, "", content)
	},
}

// channelProbeWriteTimeout bounds each database write a sweep makes for one channel,
// including waits for another transaction's row lock.
const channelProbeWriteTimeout = 10 * time.Second

// planChannelSweepResult decides what a finished probe calls for without touching state.
// An enabled channel that is too slow or fails is auto-disabled (or reported when
// auto-disable is off). Only an automatically disabled channel is re-enabled, and only
// when its probe succeeded within the latency threshold: a manual disable is the
// administrator's decision, and re-enabling a slow channel would disable it again on the
// next sweep.
// Parameters: channel is the probed channel as reloaded before the probe (its Status is
// the observed status), milliseconds is the probe latency, disableThreshold is the
// latency limit in milliseconds, and err/openaiErr are the probe outcome.
// Returns: the plan to apply.
func planChannelSweepResult(channel *model.Channel, milliseconds, disableThreshold int64,
	err error, openaiErr *relaymodel.Error) channelSweepPlan {
	var plan channelSweepPlan
	isChannelEnabled := channel.Status == model.ChannelStatusEnabled
	if isChannelEnabled && milliseconds > disableThreshold {
		err = errors.Errorf("Response time %.2fs exceeds threshold %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
		if config.AutomaticDisableChannelEnabled {
			plan.targetStatus, plan.reason = model.ChannelStatusAutoDisabled, err.Error()
		} else {
			plan.notices = append(plan.notices, channelSweepNotice{
				title: fmt.Sprintf("Channel test timed out: %s", channel.Ref().String()), content: err.Error()})
		}
	}
	// Only disable a channel on failure when AutomaticDisableChannelEnabled is true.
	if isChannelEnabled && (err != nil || monitor.ShouldDisableChannel(openaiErr, -1)) {
		reason := "channel test failed"
		if err != nil {
			reason = err.Error()
		} else if openaiErr != nil {
			reason = openaiErr.Message
		}
		if config.AutomaticDisableChannelEnabled {
			plan.targetStatus, plan.reason = model.ChannelStatusAutoDisabled, reason
		} else {
			plan.notices = append(plan.notices, channelSweepNotice{
				title: fmt.Sprintf("Channel test failed: %s", channel.Ref().String()), content: reason})
		}
	}
	if channel.Status == model.ChannelStatusAutoDisabled && err == nil && milliseconds <= disableThreshold &&
		monitor.ShouldEnableChannel(err, openaiErr) {
		plan.targetStatus = model.ChannelStatusEnabled
	}
	return plan
}

// applyChannelSweepResult persists a probe result through the atomic, fail-closed model
// transaction and then announces only what that transaction actually did. No database
// lock is held while the probe ran; the transaction re-checks SKIP, the probed
// configuration revision, and the status.
// Parameters: ctx scopes the transaction, lg is the channel-bound logger, channel is the
// probed channel, revision is its ProbeConfigRevision when probed, plan is what the
// probe calls for, and milliseconds is the probe latency.
// Returns: no values; failures discard the result and are logged and counted.
func applyChannelSweepResult(ctx context.Context, lg glog.Logger, channel *model.Channel, revision string,
	plan channelSweepPlan, milliseconds int64) {
	writeCtx, cancel := context.WithTimeout(ctx, channelProbeWriteTimeout)
	defer cancel()
	outcome, err := model.ApplyChannelProbeResultWithContext(writeCtx, channel.Id, model.ChannelProbeResult{
		Revision:       revision,
		ObservedStatus: channel.Status,
		TargetStatus:   plan.targetStatus,
		ResponseTimeMs: milliseconds,
	})
	switch {
	case err != nil:
		// The opt-out and configuration cannot be confirmed, so do nothing
		// irreversible; the next sweep retries the channel.
		lg.Warn("discarded channel test result: could not persist it atomically", zap.Error(err))
		metrics.RecordChannelHealthCheck(metrics.ChannelHealthCheckApplyFailed)
		return
	case outcome.OptedOut:
		lg.Info("discarded channel test result: testing model set to SKIP during the probe")
		metrics.RecordChannelHealthCheck(metrics.ChannelHealthCheckOptedOut)
		return
	case outcome.ConfigChanged:
		lg.Info("discarded channel test result: channel configuration changed during the probe")
		metrics.RecordChannelHealthCheck(metrics.ChannelHealthCheckConfigChanged)
		return
	}
	switch {
	case outcome.StatusChanged:
		metrics.RecordChannelHealthCheck(metrics.ChannelHealthCheckStatusChanged)
		switch plan.targetStatus {
		case model.ChannelStatusAutoDisabled:
			channelSweepNotifier.disabled(channel.Id, channel.Name, plan.reason)
		case model.ChannelStatusEnabled:
			channelSweepNotifier.enabled(channel.Id, channel.Name)
		}
	case plan.targetStatus != 0:
		metrics.RecordChannelHealthCheck(metrics.ChannelHealthCheckTransitionRefused)
		lg.Info("kept channel status: it changed while the probe was in flight",
			zap.Int("observed_status", channel.Status), zap.Int("current_status", outcome.Channel.Status),
			zap.Int("requested_status", plan.targetStatus))
	default:
		metrics.RecordChannelHealthCheck(metrics.ChannelHealthCheckApplied)
	}
	// Probe-failure notices (auto-disable off) report a failed probe, not a status change.
	for _, notice := range plan.notices {
		if err := channelSweepNotifier.notice(notice.title, notice.content); err != nil {
			lg.Warn("failed to send channel test notification", zap.Error(err))
		}
	}
}
