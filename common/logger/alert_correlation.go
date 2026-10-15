package logger

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

// newDiagnosticAlert constructs an alert from ctx, endpoint, type, token and limiter without reading configuration or changing global loggers.
// It returns the configured transport, or a wrapped construction error.
func newDiagnosticAlert(ctx context.Context, endpoint, alertType, token string, limiter glog.RateLimiter) (*glog.Alert, error) {
	alert, err := glog.NewAlert(ctx, endpoint,
		glog.WithAlertType(alertType), glog.WithAlertToken(token),
		glog.WithAlertHookLevel(zap.ErrorLevel), glog.WithRateLimiter(limiter),
		glog.WithAlertFieldAllowlist("trace_id", "user_id"))
	return alert, errors.Wrap(err, "create diagnostic alert")
}

// diagnosticAlertHook takes the configured alert and returns a Zap hook that projects validated entry fields before the existing two-key allowlist.
// Entry.Message and LoggerName retain their existing application-authored privacy boundary; this is not a message redactor.
func diagnosticAlertHook(alert *glog.Alert) func(zapcore.Entry, []zapcore.Field) error {
	next := alert.GetZapHook()
	return func(entry zapcore.Entry, fields []zapcore.Field) error {
		return next(entry, diagnosticAlertFields(fields))
	}
}

// diagnosticAlertFields takes combined Zap fields and returns fresh approved primitives without invoking serializers or modifying other sinks' context.
// Equal duplicates collapse to one value; any malformed or conflicting occurrence suppresses that key rather than selecting a caller or inherited winner.
func diagnosticAlertFields(fields []zapcore.Field) []zapcore.Field {
	var traceID string
	var userID int64
	var traceRejected, userRejected bool
	for _, field := range fields {
		if field.Type == zapcore.NamespaceType {
			// Hook fields combine call-site context before inherited context, so a namespace makes even earlier fields ambiguous.
			return nil
		}
		switch field.Key {
		case "trace_id":
			if field.Type != zapcore.StringType || !diagnosticTraceIDValid(field.String) {
				traceRejected = true
				continue
			}
			if traceID != "" && traceID != field.String {
				traceRejected = true
			}
			traceID = field.String
		case "user_id":
			// zap.Int uses Int64Type. Strings, floats and other representations are not the canonical identity field.
			if field.Type != zapcore.Int64Type || field.Integer <= 0 {
				userRejected = true
				continue
			}
			if userID != 0 && userID != field.Integer {
				userRejected = true
			}
			userID = field.Integer
		}
	}
	selected := make([]zapcore.Field, 0, 2)
	if traceID != "" && !traceRejected {
		selected = append(selected, zap.String("trace_id", traceID))
	}
	if userID > 0 && !userRejected {
		selected = append(selected, zap.Int64("user_id", userID))
	}
	return selected
}

// diagnosticTraceIDValid takes value and returns whether it is a nonzero 32-hex native OTel ID or the bounded legacy trace:span:parent:flags shape.
// Legacy trace/span components are nonzero 1..16 hex digits; parent is empty or 1..16 hex digits, and flags are 1..2 hex digits.
// Valid text is preserved exactly for SQL/log correlation, including letter case; no identifier mapping is invented.
func diagnosticTraceIDValid(value string) bool {
	if len(value) == 32 && diagnosticHexValid(value, 32, true) {
		return true
	}
	if len(value) > 53 {
		return false
	}
	parts := strings.Split(value, ":")
	return len(parts) == 4 && diagnosticHexValid(parts[0], 16, true) &&
		diagnosticHexValid(parts[1], 16, true) &&
		(parts[2] == "" || diagnosticHexValid(parts[2], 16, false)) &&
		diagnosticHexValid(parts[3], 2, false)
}

// diagnosticHexValid takes value, its maximum width and a nonzero requirement, and returns whether value satisfies those ASCII hex bounds.
func diagnosticHexValid(value string, maximum int, nonzero bool) bool {
	if len(value) == 0 || len(value) > maximum {
		return false
	}
	hasNonzero := false
	for _, char := range []byte(value) {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return false
		}
		hasNonzero = hasNonzero || char != '0'
	}
	return !nonzero || hasNonzero
}
