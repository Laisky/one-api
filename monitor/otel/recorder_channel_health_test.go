package otel

import (
	"context"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Laisky/one-api/common/metrics"
)

// resetChannelHealthInstruments clears the lazily built channel health-check
// instrument so the next recording binds to the meter provider that is global
// at that moment.
//
// Parameters: none.
//
// Return values: none.
func resetChannelHealthInstruments() {
	channelHealthOnce = sync.Once{}
	channelHealthChecksCounter = nil
}

// TestChannelHealthCheckRecordsBoundedOutcome verifies the instrument exports a
// data point per outcome, counts repeated outcomes, and only ever carries a
// sanitized outcome attribute.
//
// The instrument is built once against whatever global meter provider is
// installed at first use, so this test resets it before installing its own
// provider and again on cleanup, keeping it independent of test order.
func TestChannelHealthCheckRecordsBoundedOutcome(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	resetChannelHealthInstruments()
	t.Cleanup(func() {
		resetChannelHealthInstruments()
		_ = provider.Shutdown(context.Background())
	})

	rec := &OtelRecorder{}
	rec.RecordChannelHealthCheck(metrics.ChannelHealthCheckConfigChanged)
	rec.RecordChannelHealthCheck(metrics.ChannelHealthCheckConfigChanged)
	rec.RecordChannelHealthCheck("applied-\xc0")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	got := map[string]int64{}
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "oneapi_channel_health_checks_total" {
				continue
			}
			found = true
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "the health-check counter must be an int64 sum")
			for _, dp := range sum.DataPoints {
				v, ok := dp.Attributes.Value("outcome")
				require.True(t, ok, "every data point must carry the outcome attribute")
				require.True(t, utf8.ValidString(v.AsString()), "the outcome attribute must be valid UTF-8")
				got[v.AsString()] += dp.Value
			}
		}
	}

	require.True(t, found, "oneapi_channel_health_checks_total must export a data point")
	require.Equal(t, int64(2), got[metrics.ChannelHealthCheckConfigChanged])
	require.Equal(t, int64(1), got["applied-�"],
		"a non-UTF-8 outcome must be recorded under its sanitized value")
}

// TestChannelHealthCheckRecordingNeverPanics pins that recording is safe
// regardless of which meter provider, if any, the instrument was bound to, so a
// metrics problem can never stop the health-check sweep.
func TestChannelHealthCheckRecordingNeverPanics(t *testing.T) {
	rec := &OtelRecorder{}
	require.NotPanics(t, func() {
		rec.RecordChannelHealthCheck(metrics.ChannelHealthCheckApplyFailed)
		rec.RecordChannelHealthCheck("")
	})
}
