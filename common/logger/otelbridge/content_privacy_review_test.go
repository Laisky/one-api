package otelbridge

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// TestSourceBackedBusinessStatusesRemainTyped covers actual channel numeric and
// native response/video string status callsites without relaxing HTTP/ID guards.
func TestSourceBackedBusinessStatusesRemainTyped(t *testing.T) {
	for _, status := range []attribute.KeyValue{
		attribute.Int("status", 1), attribute.Int("status", 2), attribute.Int("status", 3),
		attribute.String("status", "completed"), attribute.String("status", "in_progress"),
	} {
		safe, ok := SanitizeLogAttribute(status)
		require.True(t, ok)
		require.Equal(t, status, safe)
	}
	for _, invalid := range []attribute.KeyValue{
		attribute.Int("status", -1), attribute.Int64("status", 1<<40), attribute.Float64("status", 1.5),
		attribute.Int("status_code", 1), attribute.String("status_code", "completed"),
		attribute.Int("channel_id", -1), attribute.Int64("user_id", 1<<40),
	} {
		_, ok := SanitizeLogAttribute(invalid)
		require.False(t, ok, string(invalid.Key))
	}
	safe, ok := SanitizeLogAttribute(attribute.String("status", "failed token=SYNTHETIC_PRIVATE_STATUS"))
	require.True(t, ok)
	require.Equal(t, "failed token=[REDACTED]", safe.Value.AsString())
}
