package otel

// This file holds the compact UUID storage metrics required by
// docs/proposals/20260715_compact-uuid-storage.md (work item AUTO-012). They
// live apart from recorder.go only to keep both files within the 600-line limit
// that §9.3 of that proposal mandates; they are otherwise an ordinary part of
// the OtelRecorder implementation of metrics.MetricsRecorder. The instruments
// themselves are declared on OtelRecorder and created in NewOtelRecorder, both
// in recorder.go.

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// UpdateCompactUUIDState publishes whether one state is the current state of a role.
//
// Callers set active=true for the current state and active=false for every
// other known state, so exactly one state per role/process reports 1.
//
// role and state must be compile-time registry constants; they become metric
// attributes and must never carry an ID, UUID, DSN, or error message.
func (r *OtelRecorder) UpdateCompactUUIDState(role, state string, active bool) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		strAttr("role", role),
		strAttr("state", state),
	}
	var value int64
	if active {
		value = 1
	}
	r.compactUUIDState.Record(ctx, value, metric.WithAttributes(attrs...))
}

// UpdateCompactUUIDBacklog publishes the last bounded gap/mismatch/blocker observation.
//
// The value is one bounded observation, never a claimed global total. role,
// target, and kind must be compile-time registry constants.
func (r *OtelRecorder) UpdateCompactUUIDBacklog(role, target, kind string, rows float64) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		strAttr("role", role),
		strAttr("target", target),
		strAttr("kind", kind),
	}
	r.compactUUIDBacklogRows.Record(ctx, rows, metric.WithAttributes(attrs...))
}

// RecordCompactUUIDAction records one DDL, fill, validation, marker, audit, or repair outcome.
//
// role, action, and result must be compile-time registry constants.
func (r *OtelRecorder) RecordCompactUUIDAction(role, action, result string) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		strAttr("role", role),
		strAttr("action", action),
		strAttr("result", result),
	}
	r.compactUUIDActionsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordCompactUUIDLookupFallback records one compact UUID lookup fallback.
//
// role and reason must be compile-time registry constants.
func (r *OtelRecorder) RecordCompactUUIDLookupFallback(role, reason string) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		strAttr("role", role),
		strAttr("reason", reason),
	}
	r.compactUUIDLookupFallbackTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// UpdateCompactUUIDLastProgress publishes the UTC timestamp of the last durable progress.
//
// role must be a compile-time registry constant.
func (r *OtelRecorder) UpdateCompactUUIDLastProgress(role string, unixTime float64) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		strAttr("role", role),
	}
	r.compactUUIDLastProgressUnixtime.Record(ctx, unixTime, metric.WithAttributes(attrs...))
}

// RecordCompactUUIDDuration records the duration of one compact UUID operation.
//
// role and operation must be compile-time registry constants.
func (r *OtelRecorder) RecordCompactUUIDDuration(role, operation string, duration time.Duration) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		strAttr("role", role),
		strAttr("operation", operation),
	}
	r.compactUUIDDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}
