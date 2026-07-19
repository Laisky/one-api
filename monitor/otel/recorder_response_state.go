package otel

// Gateway response-state metrics (proposal 20260719_stateful-responses-format-conversion.md,
// task ST-014 / rows OBS01-OBS05). The instrument is created lazily on first use
// so the OtelRecorder struct and constructor in recorder.go stay untouched.

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	responseStateEventsOnce    sync.Once
	responseStateEventsCounter metric.Int64Counter
)

// RecordResponseStateEvent records one bounded response-state decision.
//
// category and outcome MUST be compile-time registry constants; they become
// attributes and must never carry a gateway id, prompt, model, or error message.
func (r *OtelRecorder) RecordResponseStateEvent(category, outcome string) {
	responseStateEventsOnce.Do(func() {
		if c, err := otel.Meter("one-api").Int64Counter(
			"oneapi_response_state_events_total",
			metric.WithDescription("Total gateway response-state decisions by category and outcome (never content)"),
		); err == nil {
			responseStateEventsCounter = c
		}
	})
	if responseStateEventsCounter == nil {
		return
	}
	responseStateEventsCounter.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("category", category),
		attribute.String("outcome", outcome),
	))
}
