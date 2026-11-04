package metrics

// Channel health-check sweep outcomes. These are the ONLY values that may be passed as
// the outcome label of RecordChannelHealthCheck, so the label stays bounded.
const (
	// ChannelHealthCheckApplied counts a probe result persisted without a status change.
	ChannelHealthCheckApplied = "applied"
	// ChannelHealthCheckStatusChanged counts a probe result that disabled or re-enabled the channel.
	ChannelHealthCheckStatusChanged = "status_changed"
	// ChannelHealthCheckTransitionRefused counts a result whose status change was refused
	// because the status changed while the probe was in flight.
	ChannelHealthCheckTransitionRefused = "transition_refused"
	// ChannelHealthCheckOptedOut counts a channel set to SKIP: one the sweep did not probe,
	// or one whose result it discarded because SKIP was saved while the probe was in flight.
	ChannelHealthCheckOptedOut = "opted_out"
	// ChannelHealthCheckNotApplicable counts a channel skipped before probing because it
	// has no chat-capable surface or no model known to be served through a chat API format.
	ChannelHealthCheckNotApplicable = "not_applicable"
	// ChannelHealthCheckConfigChanged counts a result discarded because the probed
	// configuration (key, base URL, models, mapping, testing model, ...) changed mid-probe.
	ChannelHealthCheckConfigChanged = "config_changed"
	// ChannelHealthCheckApplyFailed counts a result discarded because it could not be
	// persisted atomically (database error or timeout); the sweep fails closed.
	ChannelHealthCheckApplyFailed = "apply_failed"
	// ChannelHealthCheckUnreadable counts a channel skipped before probing because its
	// current configuration could not be read.
	ChannelHealthCheckUnreadable = "unreadable"
)

// ChannelHealthRecorder is the OPTIONAL extension a recorder may implement to receive
// channel health-check sweep outcomes.
//
// It is separate from MetricsRecorder for the same reason as RetentionRecorder: adding
// a method to an existing exported interface breaks every out-of-tree implementation.
type ChannelHealthRecorder interface {
	// RecordChannelHealthCheck tallies one channel the health-check sweep finished with.
	RecordChannelHealthCheck(outcome string)
}

// RecordChannelHealthCheck tallies one channel the health-check sweep finished with, so
// fail-closed discards are observable rather than silent.
//
// Parameters:
//   - outcome: one of the ChannelHealthCheck* constants. Never a channel id or name.
//
// Return values: none.
func RecordChannelHealthCheck(outcome string) {
	if hr, ok := Recorder().(ChannelHealthRecorder); ok {
		hr.RecordChannelHealthCheck(outcome)
	}
}
