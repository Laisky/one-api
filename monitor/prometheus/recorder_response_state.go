package prometheus

// Gateway response-state metrics (proposal 20260719_stateful-responses-format-conversion.md,
// task ST-014 / rows OBS01-OBS05). They are an ordinary part of the
// PrometheusRecorder implementation of metrics.MetricsRecorder and live in their
// own file only to keep recorder.go small.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// responseStateEventsTotal counts gateway response-state decisions. Both labels
// are drawn from the compile-time registry in common/metrics/response_state.go;
// no gateway id, prompt, model, or error message may ever reach these labels.
var responseStateEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_response_state_events_total",
	Help: "Total gateway response-state decisions by category and outcome (never content)",
}, []string{"category", "outcome"})

// RecordResponseStateEvent records one bounded response-state decision.
//
// category and outcome MUST be compile-time registry constants.
func (p *PrometheusRecorder) RecordResponseStateEvent(category, outcome string) {
	responseStateEventsTotal.WithLabelValues(labelValues(category, outcome)...).Inc()
}
