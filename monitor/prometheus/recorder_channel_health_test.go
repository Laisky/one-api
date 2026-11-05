package prometheus

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/metrics"
)

// TestRecordChannelHealthCheckCountsOutcome proves each recorded outcome moves
// exactly its own series by one and leaves the other outcomes untouched, so a
// fail-closed discard cannot be misattributed to a successful apply.
func TestRecordChannelHealthCheckCountsOutcome(t *testing.T) {
	rec := &PrometheusRecorder{}
	const outcome = metrics.ChannelHealthCheckConfigChanged
	const other = metrics.ChannelHealthCheckApplied

	before := counterValue(t, channelHealthChecksTotal.WithLabelValues(outcome))
	beforeOther := counterValue(t, channelHealthChecksTotal.WithLabelValues(other))

	rec.RecordChannelHealthCheck(outcome)
	rec.RecordChannelHealthCheck(outcome)

	require.Equal(t, before+2, counterValue(t, channelHealthChecksTotal.WithLabelValues(outcome)))
	require.Equal(t, beforeOther, counterValue(t, channelHealthChecksTotal.WithLabelValues(other)),
		"recording one outcome must not move another outcome's series")
}

// TestRecordChannelHealthCheckSanitizesOutcome verifies the outcome label goes
// through labelValues. The outcomes are a closed set today, but a
// WithLabelValues panic inside the health-check sweep would stop it entirely,
// so the sanitization must not depend on that staying true.
func TestRecordChannelHealthCheckSanitizesOutcome(t *testing.T) {
	rec := &PrometheusRecorder{}
	before := counterValue(t, channelHealthChecksTotal.WithLabelValues("applied-�"))
	require.NotPanics(t, func() { rec.RecordChannelHealthCheck("applied-\xc0") })
	require.Equal(t, before+1, counterValue(t, channelHealthChecksTotal.WithLabelValues("applied-�")))
}
