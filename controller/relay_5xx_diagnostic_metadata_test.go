package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// suspensionDiagnosticOutcomeEntries takes observed logs and returns old or current suspension outcome events for behavioral baseline comparison.
func suspensionDiagnosticOutcomeEntries(logs *observer.ObservedLogs) []observer.LoggedEntry {
	var entries []observer.LoggedEntry
	for _, entry := range logs.All() {
		switch entry.Message {
		case suspension5XXSuccess, suspension5XXFailure, "ability suspended due to server error (5xx)", "failed to suspend ability for 5xx":
			entries = append(entries, entry)
		}
	}
	return entries
}

// requireSuspensionDiagnosticFields takes t, fields and expected relay attribution and verifies the bounded identity and status schema without claiming an upstream HTTP response.
func requireSuspensionDiagnosticFields(t *testing.T, fields map[string]any, params processChannelRelayErrorParams, category, basis, outcome string, duration time.Duration) {
	t.Helper()
	require.EqualValues(t, params.ChannelId, fields["channel_id"])
	require.EqualValues(t, params.Err.StatusCode, fields["normalized_status_code"])
	require.Equal(t, "normalized_relay_status", fields["status_semantics"])
	require.Equal(t, category, fields["error_kind"])
	require.Equal(t, basis, fields["error_kind_basis"])
	require.Equal(t, params.OriginalModel, fields["origin_model"])
	require.Equal(t, params.ActualModel, fields["actual_model"])
	require.Equal(t, params.Group, fields["group"])
	require.Equal(t, outcome, fields["suspension_outcome"])
	if outcome == "not_attempted" {
		require.NotContains(t, fields, "suspension_duration_ms")
	} else {
		require.EqualValues(t, duration.Milliseconds(), fields["suspension_duration_ms"])
	}
	require.NotContains(t, fields, "upstream_http_status_code", "normalized status has no original response provenance")
}

// TestProcessChannelRelayError5XXDiagnosticMetadata takes t and verifies persisted and failed events report exact bounded metadata, attribution basis and signed configured duration.
func TestProcessChannelRelayError5XXDiagnosticMetadata(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		status   int
		kind     errkind.Kind
		duration time.Duration
		fail     bool
	}{
		{"fallback_500", 500, errkind.Unknown, 37 * time.Second, false},
		{"explicit_upstream_503", 503, errkind.Upstream, 37 * time.Second, false},
		{"explicit_server_599", 599, errkind.Server, 37 * time.Second, false},
		{"rejected_update", 502, errkind.Upstream, 37 * time.Second, true},
		{"zero_duration", 500, errkind.Unknown, 0, false},
		{"negative_duration", 500, errkind.Unknown, -time.Second, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			fixture.params.Err.StatusCode = scenario.status
			fixture.params.Err.RawError = errkind.Mark(errors.New("synthetic relay failure without original HTTP status"), scenario.kind)
			config.ChannelSuspendSecondsFor5XX = scenario.duration
			if scenario.fail {
				require.NoError(t, fixture.db.Exec("CREATE TRIGGER reject_diagnostic_suspension BEFORE UPDATE OF suspend_until ON abilities BEGIN SELECT RAISE(ABORT, 'synthetic diagnostic write rejected'); END").Error)
			}
			processChannelRelayError(fixture.ctx, fixture.params)
			outcomes := suspensionDiagnosticOutcomeEntries(fixture.logs)
			require.Len(t, outcomes, 1)
			category, basis := scenario.kind.String(), "explicit_error_kind"
			if scenario.kind == errkind.Unknown {
				category, basis = "upstream", "normalized_status_fallback"
			}
			outcome, message := "persisted", suspension5XXSuccess
			if scenario.fail {
				outcome, message = "failed", suspension5XXFailure
			}
			requireSuspensionDiagnosticFields(t, outcomes[0].ContextMap(), fixture.params, category, basis, outcome, scenario.duration)
			require.Equal(t, message, outcomes[0].Message)
			require.NotContains(t, outcomes[0].Message, "upstream")
			generic := fixture.logs.FilterMessage("relay error").All()
			require.Len(t, generic, 1, "deferred logging must retain exactly one original relay record")
			require.Equal(t, zapcore.ErrorLevel, generic[0].Level)
			requireSuspensionDiagnosticFields(t, generic[0].ContextMap(), fixture.params, category, basis, outcome, scenario.duration)
		})
	}
}

// TestProcessChannelRelayError5XXOutcomePrecedesGenericRecord takes t and verifies the complete outcome wins the existing one-per-second alert bucket on both persistence paths.
func TestProcessChannelRelayError5XXOutcomePrecedesGenericRecord(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed_%t", fail), func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			if fail {
				require.NoError(t, fixture.db.Exec("CREATE TRIGGER reject_ordered_suspension BEFORE UPDATE OF suspend_until ON abilities BEGIN SELECT RAISE(ABORT, 'synthetic ordering write rejected'); END").Error)
			}
			processChannelRelayError(fixture.ctx, fixture.params)
			outcomes := suspensionDiagnosticOutcomeEntries(fixture.logs)
			require.Len(t, outcomes, 1)
			records := fixture.logs.All()
			genericIndex, outcomeIndex := -1, -1
			for i, entry := range records {
				if entry.Message == "relay error" {
					genericIndex = i
				}
				if entry.Message == outcomes[0].Message {
					outcomeIndex = i
				}
			}
			require.GreaterOrEqual(t, outcomeIndex, 0)
			require.Greater(t, genericIndex, outcomeIndex, "generic relay logging must follow the complete persistence outcome")
		})
	}
}

// TestProcessChannelRelayError5XXNotAttemptedDiagnostic takes t and verifies excluded normalized failures remain unsuspended and report no fabricated suspension duration.
func TestProcessChannelRelayError5XXNotAttemptedDiagnostic(t *testing.T) {
	for _, scenario := range []string{"internal_adaptor", "cancelled", "retry_hint"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			genericMessage := "relay error"
			switch scenario {
			case "internal_adaptor":
				fixture.params.Err.Type = relaymodel.ErrorTypeOneAPI
			case "cancelled":
				fixture.params.Err.RawError = context.Canceled
				genericMessage = "relay aborted by client (context canceled/deadline)"
			case "retry_hint":
				fixture.params.Err.Message = "synthetic provider temporarily unavailable; please retry"
			}
			processChannelRelayError(fixture.ctx, fixture.params)
			require.Empty(t, suspensionDiagnosticOutcomeEntries(fixture.logs))
			generic := fixture.logs.FilterMessage(genericMessage).All()
			require.Len(t, generic, 1)
			requireSuspensionDiagnosticFields(t, generic[0].ContextMap(), fixture.params, "upstream", "normalized_status_fallback", "not_attempted", 0)
		})
	}
}
