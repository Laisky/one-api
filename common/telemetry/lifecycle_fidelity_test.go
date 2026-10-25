package telemetry

import (
	"context"
	"testing"

	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// TestLifecycleDiagnosticsReachFidelityExporter preserves lifecycle and arbitrary
// application fields through the configured provider, queue and HTTP exporter.
// It accepts t, uses only synthetic data and a local collector, and returns no value.
func TestLifecycleDiagnosticsReachFidelityExporter(t *testing.T) {
	provider, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))
	// Match otlpBridgeOption: the configured bridge is NewCore, not NewPrivacyCore.
	lg := zap.New(otelbridge.NewCore(holder, otelbridge.DefaultScopeName, zapcore.InfoLevel))
	lg.Info("synthetic transport outcome",
		zap.String("log_stage", "transport_attempt"),
		zap.Int("upstream_http_status", 500),
		zap.Bool("response_headers_received", true),
		zap.String("transport_failure_class", "none"),
		zap.String("request_id", "synthetic-request"),
		zap.String("retry_outcome", "application-defined-outcome"))
	lg.Info("synthetic accounting outcome",
		zap.String("log_stage", "accounting"),
		zap.String("accounting_outcome", "refund_balance_applied"),
		zap.String("balance_update", "call_returned_no_error"),
		zap.String("ledger_reconciliation", "failed"))
	lg.Debug("synthetic filtered success", zap.String("log_stage", "transport_attempt"))

	var native otellog.Record
	native.SetBody(attribute.StringValue("synthetic native outcome"))
	native.SetSeverity(otellog.SeverityInfo)
	native.AddAttributes(attribute.String("log_stage", "application-defined-stage"),
		attribute.String("retry_outcome", "application-defined-outcome"))
	provider.Logger(otelbridge.DefaultScopeName).Emit(context.Background(), native)

	records := make(map[string]*logspb.LogRecord)
	for _, group := range collect() {
		for _, scope := range group.ScopeLogs {
			for _, record := range scope.LogRecords {
				message := record.Body.GetStringValue()
				require.NotContains(t, records, message, "each synthetic event must be exported once")
				records[message] = record
			}
		}
	}
	require.Len(t, records, 3)
	require.Contains(t, records, "synthetic transport outcome")
	require.Contains(t, records, "synthetic accounting outcome")
	require.Contains(t, records, "synthetic native outcome")
	require.NotContains(t, records, "synthetic filtered success")

	transport := fidelityPBAttrs(records["synthetic transport outcome"].Attributes)
	require.Contains(t, transport, "upstream_http_status")
	require.EqualValues(t, 500, transport["upstream_http_status"].GetIntValue())
	require.True(t, transport["response_headers_received"].GetBoolValue())
	require.Equal(t, "none", transport["transport_failure_class"].GetStringValue())
	require.Equal(t, "synthetic-request", transport["request_id"].GetStringValue())
	require.Equal(t, "application-defined-outcome", transport["retry_outcome"].GetStringValue())

	accounting := fidelityPBAttrs(records["synthetic accounting outcome"].Attributes)
	require.Equal(t, "refund_balance_applied", accounting["accounting_outcome"].GetStringValue())
	require.Equal(t, "call_returned_no_error", accounting["balance_update"].GetStringValue())
	require.Equal(t, "failed", accounting["ledger_reconciliation"].GetStringValue())

	direct := fidelityPBAttrs(records["synthetic native outcome"].Attributes)
	require.Equal(t, "application-defined-stage", direct["log_stage"].GetStringValue())
	require.Equal(t, "application-defined-outcome", direct["retry_outcome"].GetStringValue())
}
