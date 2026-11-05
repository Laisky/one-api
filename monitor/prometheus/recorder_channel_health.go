package prometheus

// Channel health-check sweep metrics. They are an ordinary part of the
// PrometheusRecorder implementation and live in their own file only to keep
// recorder.go small.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// channelHealthChecksTotal counts channels the health-check sweep finished
// with, by outcome. The outcome label is drawn from the compile-time registry
// in common/metrics/channel_health.go; no channel id, channel name, key, or
// error message may ever reach it.
var channelHealthChecksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_channel_health_checks_total",
	Help: "Channels finished by the health-check sweep, by outcome",
}, []string{"outcome"})

// RecordChannelHealthCheck tallies one channel the health-check sweep finished
// with, so fail-closed discards are observable rather than silent.
//
// Parameters:
//   - outcome: a ChannelHealthCheck* constant from
//     common/metrics/channel_health.go. It is sanitized like every other label
//     value because a WithLabelValues panic here would take down the sweep.
//
// Return values: none.
func (p *PrometheusRecorder) RecordChannelHealthCheck(outcome string) {
	channelHealthChecksTotal.WithLabelValues(labelValues(outcome)...).Inc()
}
