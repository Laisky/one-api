package telemetry

import (
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// newPrivateMetricView applies the production label allowlist together with the
// existing no-exemplar reservoir. Unknown labels are removed before aggregation
// so raw strings cannot reach the exporter or increase series cardinality.
func newPrivateMetricView() sdkmetric.View {
	base := newZeroExemplarReservoirView()
	return func(instrument sdkmetric.Instrument) (sdkmetric.Stream, bool) {
		stream, ok := base(instrument)
		stream.AttributeFilter = privateMetricAttribute
		return stream, ok
	}
}

// privateMetricEnums contains only fixed operational vocabularies. Model,
// provider, path, group, names, and identifiers require a trusted catalog before
// any such string label can be exported.
var privateMetricEnums = map[string]map[string]bool{
	"success":    {"true": true, "false": true},
	"token_type": {"prompt": true, "completion": true},
	"sink":       {"db": true, "otlp": true, "sql": true, "file": true, "stdout": true},
	"source":     {"mcp": true, "external": true},
	"outcome": {
		"success": true, "client_error": true, "server_error": true, "upstream_error": true,
		"timeout": true, "canceled": true, "panic": true, "emitted": true,
		"dropped_queue_full": true, "dropped_not_ready": true, "dropped_shutdown": true,
		"dropped_privacy": true,
		"exported":        true, "export_failed": true, "sampled_out": true, "queued": true,
		"dropped_closed": true, "written": true, "write_failed": true,
		"span_recorded": true, "span_record_failed": true, "excluded": true,
		"truncated": true, "dropped_active_limit": true, "hydrated": true,
		"conversation": true, "stateless": true, "portable": true,
		"sidecar_dropped": true, "not_portable": true, "committed": true,
		"commit_failed": true, "no_store": true, "pinned": true, "unpinned": true,
		"not_found": true, "store_error": true,
	},
	"reason":   {"disk_pressure": true, "active_file_cap": true, "writer_failure": true},
	"result":   {"completed": true, "failed": true, "canceled": true, "success": true, "error": true, "skipped": true},
	"category": {"path": true, "portability": true, "commit": true, "affinity": true, "miss": true},
	"target":   {"logs": true, "traces": true, "app_log_files": true, "async_task_bindings": true},
}

// privateMetricAttribute admits only numeric IDs/statuses and closed enum
// strings. A sensitive value under an otherwise allowed key is also rejected.
func privateMetricAttribute(kv attribute.KeyValue) bool {
	key := string(kv.Key)
	if kv.Value.Type() == attribute.STRING {
		value := kv.Value.AsString()
		switch key {
		case "method", "http.request.method", "http.method":
			return isPrivateMethod(value)
		case "status_code", "http.response.status_code", "http.status_code":
			number, err := strconv.Atoi(value)
			return err == nil && number >= 100 && number <= 599 && strconv.Itoa(number) == value
		case "channel_id", "server_id":
			number, err := strconv.ParseInt(value, 10, 32)
			return err == nil && number >= 0 && strconv.FormatInt(number, 10) == value
		}
		return privateMetricEnums[key][value]
	}
	if kv.Value.Type() == attribute.INT64 {
		switch key {
		case "channel_id", "server_id":
			return kv.Value.AsInt64() >= 0 && kv.Value.AsInt64() <= 1<<31-1
		case "http.response.status_code", "http.status_code":
			return kv.Value.AsInt64() >= 100 && kv.Value.AsInt64() <= 599
		}
	}
	return false
}
