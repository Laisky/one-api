package otel

// Channel health-check sweep metrics. The instrument is created lazily on first
// use so the OtelRecorder struct and constructor in recorder.go stay untouched.

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	channelHealthOnce          sync.Once
	channelHealthChecksCounter metric.Int64Counter
)

// initChannelHealthInstruments creates the channel health-check instruments
// once.
//
// Parameters: none.
//
// Return values: none; an instrument that fails to build stays nil and its
// recording method becomes a no-op.
func initChannelHealthInstruments() {
	channelHealthOnce.Do(func() {
		meter := otel.Meter("one-api")
		if c, err := meter.Int64Counter(
			"oneapi_channel_health_checks_total",
			metric.WithDescription("Channels finished by the health-check sweep, by outcome"),
		); err == nil {
			channelHealthChecksCounter = c
		}
	})
}

// RecordChannelHealthCheck tallies one channel the health-check sweep finished
// with, so fail-closed discards are observable rather than silent.
//
// Parameters:
//   - outcome: a ChannelHealthCheck* constant from
//     common/metrics/channel_health.go. It is sanitized like every other string
//     attribute because an invalid UTF-8 attribute fails the whole OTLP export
//     batch.
//
// Return values: none.
func (r *OtelRecorder) RecordChannelHealthCheck(outcome string) {
	initChannelHealthInstruments()
	if channelHealthChecksCounter == nil {
		return
	}
	channelHealthChecksCounter.Add(context.Background(), 1, metric.WithAttributes(
		strAttr("outcome", outcome),
	))
}
