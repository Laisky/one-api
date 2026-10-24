package otelbridge

import (
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"testing"
)

// TestPrivacyLifecycleDiagnostics exports exact operational enums while rejecting arbitrary values, types and negative counts.
func TestPrivacyLifecycleDiagnostics(t *testing.T) {
	core := NewPrivacyCore(NewProviderHolder(), DefaultScopeName, zapcore.InfoLevel)
	converted := core.convertFields([]zapcore.Field{
		zap.String("log_stage", "relay_final"), zap.String("relay_outcome", "helper_failed"), zap.String("retry_outcome", "no_alternative"), zap.String("client_outcome", "error_response_written"), zap.Int("relay_attempts", 1), zap.Int("client_status_code", 500),
		zap.String("transport_failure_class", "connection_reset"), zap.Bool("response_headers_received", false),
		zap.String("accounting_outcome", "refund_balance_applied"), zap.String("ledger_reconciliation", "update_returned_no_error"),
		zap.String("retry_outcome", "sentinel-secret"), zap.String("relay_attempts", "sentinel-secret"), zap.Int("relay_attempt", -1), zap.Int("client_status_code", 999), zap.String("request_id", "sentinel-secret"),
	})
	expected := []attribute.KeyValue{attribute.String("log_stage", "relay_final"), attribute.String("relay_outcome", "helper_failed"), attribute.String("retry_outcome", "no_alternative"), attribute.String("client_outcome", "error_response_written"), attribute.Int("relay_attempts", 1), attribute.Int("client_status_code", 500), attribute.String("transport_failure_class", "connection_reset"), attribute.Bool("response_headers_received", false), attribute.String("accounting_outcome", "refund_balance_applied"), attribute.String("ledger_reconciliation", "update_returned_no_error")}
	require.ElementsMatch(t, expected, converted.attrs)
	for _, kv := range expected {
		require.True(t, PrivateAttribute(kv))
	}
	require.False(t, PrivateAttribute(attribute.String("retry_outcome", "sentinel-secret")))
}
