package telemetry

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// privateMetricInstruments is the exact registry declared in monitor/otel,
// otelgin v0.71.0, and gorm metrics v0.1.16. Prefix matching would let arbitrary
// user text enter metric names. New instruments require explicit review here.
var privateMetricInstruments = map[string]bool{
	"oneapi_metric_privacy_dropped_instruments_total": true,
	"one_api_channel_balance_usd":                     true, "one_api_channel_requests_in_flight": true,
	"one_api_channel_response_time_ms": true, "one_api_channel_status": true,
	"one_api_channel_success_rate": true, "one_api_db_queries_total": true,
	"one_api_errors_total": true, "one_api_http_active_requests": true,
	"one_api_http_request_duration_seconds": true, "one_api_http_requests_total": true,
	"one_api_model_usage_duration_seconds": true, "one_api_rate_limit_hits_total": true,
	"one_api_redis_command_duration_seconds": true, "one_api_redis_commands_total": true,
	"one_api_relay_quota_used_total": true, "one_api_relay_request_duration_seconds": true,
	"one_api_relay_requests_total": true, "one_api_relay_tokens_total": true,
	"one_api_site_active_users": true, "one_api_site_total_quota": true,
	"one_api_site_total_users": true, "one_api_site_used_quota": true,
	"one_api_user_balance": true, "one_api_user_quota_used_total": true,
	"one_api_user_requests_total": true, "one_api_user_tokens_total": true,
	"oneapi_app_log_export_queue_byte_limit": true, "oneapi_app_log_export_queue_bytes": true,
	"oneapi_app_log_export_queue_record_limit": true, "oneapi_app_log_export_queue_records": true,
	"oneapi_app_log_export_records_total": true, "oneapi_compact_uuid_actions_total": true,
	"oneapi_compact_uuid_backlog_rows": true, "oneapi_compact_uuid_duration_seconds": true,
	"oneapi_compact_uuid_last_progress_unixtime": true, "oneapi_compact_uuid_lookup_fallback_total": true,
	"oneapi_compact_uuid_state": true, "oneapi_log_disk_pressure_active": true,
	"oneapi_log_suppressed_bytes_total": true, "oneapi_log_suppressed_lines_total": true,
	"oneapi_request_duration_ms": true, "oneapi_request_outcomes_total": true,
	"oneapi_request_time_to_first_token_ms": true, "oneapi_response_state_events_total": true,
	"oneapi_retention_sweep_duration_ms": true, "oneapi_retention_sweep_rows_total": true,
	"oneapi_retention_sweeps_total": true, "oneapi_trace_active_recorders": true,
	"oneapi_trace_active_recorders_limit": true, "oneapi_trace_queue_capacity": true,
	"oneapi_trace_queue_depth": true, "oneapi_trace_records_total": true,
	"oneapi_uuid_backfill_cycle_duration_seconds": true, "oneapi_uuid_backfill_finalizer_total": true,
	"oneapi_uuid_backfill_last_backlog": true, "oneapi_uuid_backfill_rows_total": true,
	"http.server.request.body.size": true, "http.server.response.body.size": true,
	"http.server.request.duration": true,
	"go.sql.connections_max_open":  true, "go.sql.connections_open": true,
	"go.sql.connections_in_use": true, "go.sql.connections_idle": true,
	"go.sql.connections_wait_count": true, "go.sql.connections_wait_duration": true,
	"go.sql.connections_closed_max_idle": true, "go.sql.connections_closed_max_idle_time": true,
	"go.sql.connections_closed_max_lifetime": true,
}

// privacyMetricExporter normalizes metadata outside the SDK datapoint view,
// including arbitrary instrument names, descriptions, units, and scope fields.
type privacyMetricExporter struct{ sdkmetric.Exporter }

// metricPrivacyDrops counts rejected instrument exports without their names.
var metricPrivacyDrops atomic.Int64

// metricPrivacyStarted supplies a stable start for the cumulative drop counter.
var metricPrivacyStarted = time.Now()

// Export passes private copies to the transport while retaining original SDK
// aggregation values and timestamps. Unknown instruments/aggregations are omitted.
func (e privacyMetricExporter) Export(ctx context.Context, original *metricdata.ResourceMetrics) error {
	if original == nil {
		return nil
	}
	private := metricdata.ResourceMetrics{Resource: privateResource(original.Resource)}
	for _, scope := range original.ScopeMetrics {
		kept := metricdata.ScopeMetrics{Scope: instrumentation.Scope{Name: "one-api"}}
		for _, metric := range scope.Metrics {
			if !privateMetricInstruments[metric.Name] {
				metricPrivacyDrops.Add(1)
				continue
			}
			data := privateMetricAggregation(metric.Data, privateMetricFilter(metric.Name))
			if data == nil {
				metricPrivacyDrops.Add(1)
				continue
			}
			kept.Metrics = append(kept.Metrics, metricdata.Metrics{Name: metric.Name,
				Unit: privateMetricUnit(metric.Name), Data: data})
		}
		if len(kept.Metrics) != 0 {
			private.ScopeMetrics = append(private.ScopeMetrics, kept)
		}
	}
	if dropped := metricPrivacyDrops.Load(); dropped > 0 {
		private.ScopeMetrics = append(private.ScopeMetrics, metricdata.ScopeMetrics{
			Scope: instrumentation.Scope{Name: "one-api"},
			Metrics: []metricdata.Metrics{{Name: "oneapi_metric_privacy_dropped_instruments_total",
				Data: metricdata.Sum[int64]{Temporality: metricdata.CumulativeTemporality, IsMonotonic: true,
					DataPoints: []metricdata.DataPoint[int64]{{StartTime: metricPrivacyStarted, Time: time.Now(), Value: dropped}}}}},
		})
	}
	return e.Exporter.Export(ctx, &private)
}

// privateMetricUnit returns only source-declared units, never caller options.
func privateMetricUnit(name string) string {
	switch name {
	case "oneapi_request_duration_ms", "oneapi_request_time_to_first_token_ms", "oneapi_retention_sweep_duration_ms":
		return "ms"
	case "oneapi_app_log_export_queue_bytes", "oneapi_app_log_export_queue_byte_limit", "http.server.request.body.size", "http.server.response.body.size":
		return "By"
	case "http.server.request.duration":
		return "s"
	case "go.sql.connections_wait_duration":
		return "nanoseconds"
	}
	return ""
}

// privateMetricAggregation filters each supported aggregation's attributes and
// removes exemplars, which have their own unfiltered attribute surface.
func privateMetricAggregation(data metricdata.Aggregation, filter attribute.Filter) metricdata.Aggregation {
	switch value := data.(type) {
	case metricdata.Sum[int64]:
		value.DataPoints = privateMetricPoints(value.DataPoints, filter)
		return value
	case metricdata.Sum[float64]:
		value.DataPoints = privateMetricPoints(value.DataPoints, filter)
		return value
	case metricdata.Gauge[int64]:
		value.DataPoints = privateMetricPoints(value.DataPoints, filter)
		return value
	case metricdata.Gauge[float64]:
		value.DataPoints = privateMetricPoints(value.DataPoints, filter)
		return value
	case metricdata.Histogram[int64]:
		value.DataPoints = privateHistogramPoints(value.DataPoints, filter)
		return value
	case metricdata.Histogram[float64]:
		value.DataPoints = privateHistogramPoints(value.DataPoints, filter)
		return value
	case metricdata.ExponentialHistogram[int64]:
		value.DataPoints = privateExponentialPoints(value.DataPoints, filter)
		return value
	case metricdata.ExponentialHistogram[float64]:
		value.DataPoints = privateExponentialPoints(value.DataPoints, filter)
		return value
	}
	return nil
}

// privateMetricPoints returns independent scalar datapoint views.
func privateMetricPoints[N int64 | float64](original []metricdata.DataPoint[N], filter attribute.Filter) []metricdata.DataPoint[N] {
	points := slices.Clone(original)
	for i := range points {
		points[i].Attributes, _ = points[i].Attributes.Filter(filter)
		points[i].Exemplars = nil
	}
	return points
}

// privateHistogramPoints returns independent histogram datapoint views.
func privateHistogramPoints[N int64 | float64](original []metricdata.HistogramDataPoint[N], filter attribute.Filter) []metricdata.HistogramDataPoint[N] {
	points := slices.Clone(original)
	for i := range points {
		points[i].Attributes, _ = points[i].Attributes.Filter(filter)
		points[i].Exemplars = nil
	}
	return points
}

// privateExponentialPoints returns independent exponential histogram views.
func privateExponentialPoints[N int64 | float64](original []metricdata.ExponentialHistogramDataPoint[N], filter attribute.Filter) []metricdata.ExponentialHistogramDataPoint[N] {
	points := slices.Clone(original)
	for i := range points {
		points[i].Attributes, _ = points[i].Attributes.Filter(filter)
		points[i].Exemplars = nil
	}
	return points
}
