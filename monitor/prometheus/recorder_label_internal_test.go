package prometheus

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// TestRecorderNeverPanicsOnNonUTF8LabelValues pins the contract that a metrics
// recorder must never take a request down: prometheus/client_golang panics in
// WithLabelValues when a label value is not valid UTF-8, and request paths,
// model names and other caller-provided strings can carry raw bytes.
func TestRecorderNeverPanicsOnNonUTF8LabelValues(t *testing.T) {
	rec := &PrometheusRecorder{}
	bad := "/\xc0"
	start := time.Now()

	calls := map[string]func(){
		"RecordHTTPActiveRequest": func() { rec.RecordHTTPActiveRequest(bad, "GET", 1); rec.RecordHTTPActiveRequest(bad, "GET", -1) },
		"RecordHTTPRequest":       func() { rec.RecordHTTPRequest(start, bad, "GET", "200") },
		"RecordRelayRequest": func() {
			rec.RecordRelayRequest(start, 1, "openai", bad, "1", bad, "1", bad, "chat", true, 1, 1, 1)
		},
		"UpdateChannelMetrics":            func() { rec.UpdateChannelMetrics(1, bad, "openai", 1, 0, 0, 0) },
		"UpdateChannelRequestsInFlight":   func() { rec.UpdateChannelRequestsInFlight(1, bad, "openai", 1) },
		"RecordUserMetrics":               func() { rec.RecordUserMetrics("1", bad, bad, 1, 1, 1, 1) },
		"RecordDBQuery":                   func() { rec.RecordDBQuery(start, bad, bad, true) },
		"RecordRedisCommand":              func() { rec.RecordRedisCommand(start, bad, true) },
		"RecordRateLimitHit":              func() { rec.RecordRateLimitHit(bad, bad) },
		"UpdateRateLimitRemaining":        func() { rec.UpdateRateLimitRemaining(bad, bad, 1) },
		"UpdateActiveTokens":              func() { rec.UpdateActiveTokens("1", bad, 1) },
		"RecordError":                     func() { rec.RecordError(bad, bad) },
		"RecordModelUsage":                func() { rec.RecordModelUsage(bad, bad, time.Second) },
		"RecordBillingOperation":          func() { rec.RecordBillingOperation(start, bad, true, 1, 1, bad, 1) },
		"RecordBillingTimeout":            func() { rec.RecordBillingTimeout(1, 1, bad, 1, time.Second) },
		"RecordBillingError":              func() { rec.RecordBillingError(bad, bad, 1, 1, bad) },
		"RecordUUIDBackfillRows":          func() { rec.RecordUUIDBackfillRows(bad, bad, bad, bad, 1) },
		"UpdateUUIDBackfillBacklog":       func() { rec.UpdateUUIDBackfillBacklog(bad, bad, 1) },
		"RecordUUIDBackfillCycle":         func() { rec.RecordUUIDBackfillCycle(bad, bad, bad, time.Second) },
		"RecordUUIDBackfillFinalizer":     func() { rec.RecordUUIDBackfillFinalizer(bad, bad) },
		"UpdateCompactUUIDState":          func() { rec.UpdateCompactUUIDState(bad, bad, true) },
		"UpdateCompactUUIDBacklog":        func() { rec.UpdateCompactUUIDBacklog(bad, bad, bad, 1) },
		"RecordCompactUUIDAction":         func() { rec.RecordCompactUUIDAction(bad, bad, bad) },
		"RecordCompactUUIDLookupFallback": func() { rec.RecordCompactUUIDLookupFallback(bad, bad) },
		"UpdateCompactUUIDLastProgress":   func() { rec.UpdateCompactUUIDLastProgress(bad, 1) },
		"RecordCompactUUIDDuration":       func() { rec.RecordCompactUUIDDuration(bad, bad, time.Second) },
		"RecordResponseStateEvent":        func() { rec.RecordResponseStateEvent(bad, bad) },
		"InitSystemMetrics":               func() { rec.InitSystemMetrics(bad, bad, bad, start) },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, call)
		})
	}
}

// TestRecorderRecordsSanitizedLabelValue verifies that a non-UTF-8 path is
// recorded under its U+FFFD-sanitized label rather than being dropped, so the
// request still counts toward the metrics.
func TestRecorderRecordsSanitizedLabelValue(t *testing.T) {
	rec := &PrometheusRecorder{}
	before := counterValue(t, httpRequestsTotal.WithLabelValues("/�", "GET", "200"))
	rec.RecordHTTPRequest(time.Now(), "/\xc0", "GET", "200")
	after := counterValue(t, httpRequestsTotal.WithLabelValues("/�", "GET", "200"))
	require.Equal(t, before+1, after)
}

// counterValue reads the current value of a counter child.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

// TestLabelValuesLeavesValidStringsUntouched pins that sanitization is a
// no-op for valid label values, including non-ASCII ones, so existing series
// names do not change.
func TestLabelValuesLeavesValidStringsUntouched(t *testing.T) {
	require.Equal(t, []string{"/api/channel/:id", "GET", "模型", "é"}, labelValues("/api/channel/:id", "GET", "模型", "é"))
	require.Equal(t, []string{"/�", "a�b"}, labelValues("/\xc0", "a\xff\xfeb"))
}
