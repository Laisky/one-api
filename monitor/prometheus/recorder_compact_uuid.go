package prometheus

// This file holds the compact UUID storage metrics required by
// docs/proposals/20260715_compact-uuid-storage.md (work item AUTO-012). They
// live apart from recorder.go only to keep both files within the 600-line limit
// that §9.3 of that proposal mandates; they are otherwise an ordinary part of
// the PrometheusRecorder implementation of metrics.MetricsRecorder.

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Compact UUID storage metric definitions
//
// NOTE: like the external UUID backfill metrics in recorder.go, these
// intentionally use the "oneapi_" prefix rather than the "one_api_" prefix used
// elsewhere, because the names are specified literally by the compact UUID
// storage proposal (§11).
//
// Every label is bounded: role, state, target, kind, action, result, reason,
// and operation are all drawn from compile-time registries. No ID, UUID, DSN,
// credential, row content, fingerprint, or error message may ever reach these
// labels.
var (
	compactUUIDState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oneapi_compact_uuid_state",
		Help: "Compact UUID storage state (1=current state for the role, 0=otherwise)",
	}, []string{"role", "state"})

	compactUUIDBacklogRows = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oneapi_compact_uuid_backlog_rows",
		Help: "Last bounded compact UUID gap/mismatch/blocker observation, not a claimed global total",
	}, []string{"role", "target", "kind"})

	compactUUIDActionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "oneapi_compact_uuid_actions_total",
		Help: "Total compact UUID DDL, fill, validation, marker, audit, and repair outcomes",
	}, []string{"role", "action", "result"})

	compactUUIDLookupFallbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "oneapi_compact_uuid_lookup_fallback_total",
		Help: "Total compact UUID lookup fallbacks by reason",
	}, []string{"role", "reason"})

	compactUUIDLastProgressUnixtime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oneapi_compact_uuid_last_progress_unixtime",
		Help: "UTC unix timestamp of the last durable compact UUID progress",
	}, []string{"role"})

	// Buckets span sub-second lock waits through multi-hour DDL and validation
	// work, so the same histogram can characterise both ends of §11.
	compactUUIDDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "oneapi_compact_uuid_duration_seconds",
		Help:    "Duration of compact UUID lock, DDL, fill, validation, and audit operations in seconds",
		Buckets: []float64{.005, .025, .1, .5, 1, 5, 15, 60, 300, 900, 1800, 3600, 7200, 14400},
	}, []string{"role", "operation"})
)

// UpdateCompactUUIDState publishes whether one state is the current state of a role.
//
// Callers set active=true for the current state and active=false for every
// other known state, so exactly one state per role/process reports 1.
//
// role and state must be compile-time registry constants.
func (p *PrometheusRecorder) UpdateCompactUUIDState(role, state string, active bool) {
	var value float64
	if active {
		value = 1
	}
	compactUUIDState.WithLabelValues(labelValues(role, state)...).Set(value)
}

// UpdateCompactUUIDBacklog publishes the last bounded gap/mismatch/blocker observation.
//
// The value is one bounded observation, never a claimed global total. role,
// target, and kind must be compile-time registry constants.
func (p *PrometheusRecorder) UpdateCompactUUIDBacklog(role, target, kind string, rows float64) {
	compactUUIDBacklogRows.WithLabelValues(labelValues(role, target, kind)...).Set(rows)
}

// RecordCompactUUIDAction records one DDL, fill, validation, marker, audit, or repair outcome.
//
// role, action, and result must be compile-time registry constants; they become
// metric labels and must never carry an ID, UUID, DSN, or error message.
func (p *PrometheusRecorder) RecordCompactUUIDAction(role, action, result string) {
	compactUUIDActionsTotal.WithLabelValues(labelValues(role, action, result)...).Inc()
}

// RecordCompactUUIDLookupFallback records one compact UUID lookup fallback.
//
// role and reason must be compile-time registry constants.
func (p *PrometheusRecorder) RecordCompactUUIDLookupFallback(role, reason string) {
	compactUUIDLookupFallbackTotal.WithLabelValues(labelValues(role, reason)...).Inc()
}

// UpdateCompactUUIDLastProgress publishes the UTC timestamp of the last durable progress.
//
// role must be a compile-time registry constant.
func (p *PrometheusRecorder) UpdateCompactUUIDLastProgress(role string, unixTime float64) {
	compactUUIDLastProgressUnixtime.WithLabelValues(labelValues(role)...).Set(unixTime)
}

// RecordCompactUUIDDuration records the duration of one compact UUID operation.
//
// role and operation must be compile-time registry constants.
func (p *PrometheusRecorder) RecordCompactUUIDDuration(role, operation string, duration time.Duration) {
	compactUUIDDuration.WithLabelValues(labelValues(role, operation)...).Observe(duration.Seconds())
}
