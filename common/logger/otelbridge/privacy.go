package otelbridge

import (
	"context"
	"math"

	"github.com/Laisky/zap/zapcore"
	"go.opentelemetry.io/otel/attribute"
)

// privateNumericFields lists content-free operational metadata allowed off-box.
// Unknown keys and all string, object, error, and byte fields fail closed.
var privateNumericFields = map[string]bool{
	"status": true, "status_code": true, "body_size": true, "body_bytes": true,
	"prompt_bytes": true, "prompt_tokens": true, "completion_tokens": true,
	"total_tokens": true, "quota": true, "duration_ms": true, "latency_ms": true,
	"user_id": true, "token_id": true, "channel_id": true, "server_id": true,
	"retry_attempts": true, "retry_times": true, "retry_count": true,
	"body_logging_suppressed": true, "body_truncated": true, "is_error": true,
	"channel_health_counted": true, "user_originated": true,
}

// NewPrivacyCore returns a production OTLP core that exports correlation,
// severity, caller location, and allowlisted numeric metadata.
// Local stdout/file sinks remain unaffected. Arbitrary messages and field values
// cannot reach the OTLP queue or edge journal through this core.
func NewPrivacyCore(holder *ProviderHolder, scope string, enab zapcore.LevelEnabler) *Core {
	core := NewCore(holder, scope, enab)
	core.private = true
	return core
}

// convertFields filters private fields before marshaling them, preserving the
// invisible span-context carrier while avoiding arbitrary object serializers.
func (c *Core) convertFields(fields []zapcore.Field) convertedFields {
	if !c.private {
		return convertFields(fields)
	}
	allowed := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		if _, ok := spanContextFromField(field); ok {
			allowed = append(allowed, field)
			continue
		}
		if !privateNumericFields[field.Key] {
			continue
		}
		switch field.Type {
		case zapcore.BoolType, zapcore.Int8Type, zapcore.Int16Type, zapcore.Int32Type,
			zapcore.Int64Type, zapcore.Uint8Type, zapcore.Uint16Type, zapcore.Uint32Type,
			zapcore.Uint64Type, zapcore.Float32Type, zapcore.Float64Type, zapcore.DurationType:
			allowed = append(allowed, field)
		}
	}
	converted := convertFields(allowed)
	kept := converted.attrs[:0]
	for _, kv := range converted.attrs {
		if PrivateAttribute(kv) {
			kept = append(kept, kv)
		}
	}
	converted.attrs = kept
	return converted
}

// privateContextKey marks caller metadata derived by the production Zap core.
type privateContextKey struct{}

// IsPrivateContext reports whether caller fields came from the privacy core,
// rather than arbitrary direct OpenTelemetry LogRecord attributes.
func IsPrivateContext(ctx context.Context) bool {
	return ctx != nil && ctx.Value(privateContextKey{}) == true
}

// PrivateAttribute applies the shared key/type/value policy to numeric log
// attributes. Identifiers and statuses must be bounded integers, never floats,
// strings, negative values, or uint64 values converted into decimal strings.
func PrivateAttribute(kv attribute.KeyValue) bool {
	key := string(kv.Key)
	if !privateNumericFields[key] {
		return false
	}
	switch key {
	case "body_logging_suppressed", "body_truncated", "is_error", "channel_health_counted", "user_originated":
		return kv.Value.Type() == attribute.BOOL
	case "user_id", "token_id", "channel_id", "server_id":
		return kv.Value.Type() == attribute.INT64 && kv.Value.AsInt64() >= 0 && kv.Value.AsInt64() <= 1<<31-1
	case "status", "status_code":
		return kv.Value.Type() == attribute.INT64 && kv.Value.AsInt64() >= 100 && kv.Value.AsInt64() <= 599
	}
	if kv.Value.Type() == attribute.INT64 {
		return kv.Value.AsInt64() >= 0
	}
	if kv.Value.Type() == attribute.FLOAT64 {
		value := kv.Value.AsFloat64()
		return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
	}
	return false
}
