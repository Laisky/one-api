package otelbridge

import (
	"github.com/Laisky/zap/zapcore"
	"go.opentelemetry.io/otel/attribute"
	"strings"
)

// privateLifecycleEnums permits only fixed operational labels, never arbitrary event text or user-supplied strings.
var privateLifecycleEnums = map[string]string{
	"status_semantics":        "normalized_relay_status",
	"error_kind_basis":        "explicit_error_kind normalized_status_fallback",
	"error_kind":              "unknown invalid_request unauthorized forbidden not_found insufficient_quota rate_limited conflict config upstream server",
	"suspension_outcome":      "not_attempted succeeded failed",
	"log_stage":               "relay_attempt relay_final retry_decision transport_attempt accounting",
	"relay_outcome":           "helper_succeeded helper_failed",
	"attempt_outcome":         "succeeded failed",
	"retry_outcome":           "not_needed succeeded selected policy_veto accepted_work billing_veto output_committed request_canceled request_deadline budget_exhausted no_alternative selection_failed",
	"client_outcome":          "uncommitted response_written committed_response_preserved error_response_written",
	"transport_failure_class": "none connection_reset canceled deadline_exceeded other",
	"transport_failure_phase": "none before_response_headers response_headers after_response_headers unknown",
	"request_context_state":   "active canceled deadline_exceeded other",
	"transport_trace_state":   "observed unknown",
	"accounting_outcome":      "reservation_retained refund_balance_failed refund_balance_applied",
	"balance_update":          "not_attempted failed call_returned_no_error",
	"ledger_reconciliation":   "not_attempted skipped failed update_returned_no_error",
}

// privateLifecycleNumeric identifies strictly typed operational counters, booleans and HTTP statuses.
var privateLifecycleNumeric = map[string]string{
	"pre_consumed_quota": "quantity",
	"relay_attempt":      "count", "relay_attempts": "count", "relay_next_attempt": "count", "retry_attempt": "count",
	"transport_got_conn_count": "count", "transport_reused_conn_count": "count", "transport_write_completed_count": "count", "transport_write_failed_count": "count", "transport_first_response_byte_count": "count",
	"client_status_code": "status", "upstream_http_status": "status", "normalized_status_code": "status",
	"response_headers_received": "bool", "billing_reconciled_flag": "bool", "upstream_request_possibly_forwarded": "bool",
}

// privateLifecycleField admits only scalar candidates before conversion, avoiding evaluation of arbitrary serializers even under allowed names.
func privateLifecycleField(f zapcore.Field) bool {
	if _, ok := privateLifecycleEnums[f.Key]; ok {
		return f.Type == zapcore.StringType
	}
	if _, ok := privateLifecycleNumeric[f.Key]; !ok {
		return false
	}
	switch f.Type {
	case zapcore.BoolType, zapcore.Int8Type, zapcore.Int16Type, zapcore.Int32Type, zapcore.Int64Type, zapcore.Uint8Type, zapcore.Uint16Type, zapcore.Uint32Type, zapcore.Uint64Type:
		return true
	default:
		return false
	}
}

// privateLifecycleAttribute enforces exact enum membership and bounded numeric types at both the Zap bridge and final SDK boundary.
func privateLifecycleAttribute(kv attribute.KeyValue) bool {
	key := string(kv.Key)
	if values, ok := privateLifecycleEnums[key]; ok {
		if kv.Value.Type() != attribute.STRING {
			return false
		}
		for _, value := range strings.Fields(values) {
			if kv.Value.AsString() == value {
				return true
			}
		}
		return false
	}
	kind, ok := privateLifecycleNumeric[key]
	if !ok {
		return false
	}
	if kind == "bool" {
		return kv.Value.Type() == attribute.BOOL
	}
	if kv.Value.Type() != attribute.INT64 {
		return false
	}
	n := kv.Value.AsInt64()
	if kind == "status" {
		return n >= 100 && n <= 599
	}
	if kind == "quantity" {
		return n >= 0
	}
	return n >= 0 && n <= 1<<31-1
}
