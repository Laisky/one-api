package controller

import (
	"context"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/errkind"
	applog "github.com/Laisky/one-api/common/logger"
)

// logInitialRelayFailure takes the request context, existing failure decisions and optional suspension result, and writes exactly one generic relay record without changing classification.
func logInitialRelayFailure(ctx context.Context, params processChannelRelayErrorParams, isUserError bool, failureKind errkind.Kind, channelHealthCounted bool, outcome string, duration time.Duration) {
	lg := gmw.GetLogger(ctx)
	switch {
	case isClientContextCancel(params.Err.StatusCode, params.Err.RawError):
		lg.Warn("relay aborted by client (context canceled/deadline)",
			appendRelayFailureDiagnosticFields(params, failureKind, outcome, duration, stacklessErrorField(params.Err.RawError))...,
		)
	case isUserError:
		lg.Warn("user-originated request error",
			appendRelayFailureDiagnosticFields(params, failureKind, outcome, duration, stacklessErrorField(params.Err.RawError))...,
		)
	case failureKind.IsClient():
		lg.Warn("relay error",
			appendRelayFailureDiagnosticFields(params, failureKind, outcome, duration,
				stacklessErrorField(params.Err.RawError),
				zap.Bool("channel_health_counted", channelHealthCounted),
			)...,
		)
	default:
		lg.Error("relay error",
			appendRelayFailureDiagnosticFields(params, failureKind, outcome, duration,
				zap.Error(applog.RedactJSONErrorValue(params.Err.RawError)),
				zap.Bool("channel_health_counted", channelHealthCounted),
			)...,
		)
	}

}

// appendRelayFailureDiagnosticFields takes relay context, its existing attribution, optional suspension result and extra fields, and returns consistent diagnostics without claiming an original upstream HTTP response.
func appendRelayFailureDiagnosticFields(params processChannelRelayErrorParams, failureKind errkind.Kind, outcome string, duration time.Duration, extra ...zap.Field) []zap.Field {
	basis := "normalized_status_fallback"
	if errkind.Of(params.Err.RawError) != errkind.Unknown {
		basis = "explicit_error_kind"
	}
	fields := []zap.Field{
		zap.String("log_stage", "relay_attempt"),
		zap.Int("normalized_status_code", params.Err.StatusCode),
		zap.String("status_semantics", "normalized_relay_status"),
		zap.String("error_kind", failureKind.String()),
		zap.String("error_kind_basis", basis),
	}
	if params.Attempt > 0 {
		fields = append(fields, zap.Int("relay_attempt", params.Attempt))
	}
	if outcome != "" {
		fields = append(fields, zap.String("suspension_outcome", outcome))
		if outcome != "not_attempted" {
			fields = append(fields, zap.Int64("suspension_duration_ms", duration.Milliseconds()))
		}
	}
	return appendRelayFailureFields(params, append(fields, extra...)...)
}
