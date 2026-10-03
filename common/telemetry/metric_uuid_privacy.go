package telemetry

import (
	"slices"

	"go.opentelemetry.io/otel/attribute"
)

// privateUUIDRoles mirrors the physical database roles in model/uuid_migration_topology.go.
var privateUUIDRoles = []string{"primary", "log"}

// privateUUIDTargets mirrors the compile-time owned/FK registries in
// model/uuid_migration_registry.go. These are schema identifiers, never row IDs.
var privateUUIDTargets = []string{
	"users.uuid", "tokens.uuid", "channels.uuid", "redemptions.uuid",
	"token_transactions.uuid", "user_request_costs.uuid", "traces.uuid",
	"async_task_bindings.uuid", "mcp_servers.uuid", "mcp_tools.uuid",
	"passkey_credentials.uuid", "logs.uuid", "users.inviter_uuid",
	"tokens.user_uuid", "redemptions.user_uuid", "token_transactions.token_uuid",
	"token_transactions.user_uuid", "user_request_costs.user_uuid",
	"async_task_bindings.user_uuid", "async_task_bindings.token_uuid",
	"async_task_bindings.channel_uuid", "mcp_tools.server_uuid",
	"passkey_credentials.user_uuid", "logs.user_uuid", "logs.channel_uuid",
	"logs.token_uuid", "token_transactions.log_uuid",
}

// privateCompactStates mirrors model/compact_uuid_state.go, including zero-valued
// inactive states. Removing these labels before aggregation corrupts the gauge.
var privateCompactStates = []string{
	"waiting_prerequisite", "expanding", "backfilling", "indexing", "validating",
	"ready", "degraded", "blocked_validation", "retry_wait", "passive_legacy",
}

// privateUUIDInstrumentEnums admits only each instrument's declared keys and
// finite source vocabularies. Values safe for one instrument do not widen others.
var privateUUIDInstrumentEnums = map[string]map[string][]string{
	"oneapi_compact_uuid_state": {
		"role": privateUUIDRoles, "state": privateCompactStates,
	},
	"oneapi_compact_uuid_backlog_rows": {
		"role": privateUUIDRoles, "target": privateUUIDTargets,
		"kind": {"gap", "mismatch", "blocker"},
	},
	"oneapi_compact_uuid_actions_total": {
		"role": privateUUIDRoles, "action": {"cycle", "audit"}, "result": {"success", "failure"},
	},
	"oneapi_compact_uuid_duration_seconds": {
		"role": privateUUIDRoles, "operation": {"lock", "cycle", "audit"},
	},
	"oneapi_compact_uuid_last_progress_unixtime": {"role": privateUUIDRoles},
	"oneapi_compact_uuid_lookup_fallback_total": {
		"role": privateUUIDRoles, "reason": {"missing", "mismatch", "expired_health", "capability"},
	},
	"oneapi_uuid_backfill_rows_total": {
		"role": privateUUIDRoles, "phase": {"owned", "fk", "token_name"},
		"target": privateUUIDTargets, "result": {"updated", "unresolved"},
	},
	"oneapi_uuid_backfill_last_backlog": {"role": privateUUIDRoles, "target": {"all"}},
	"oneapi_uuid_backfill_cycle_duration_seconds": {
		"role": privateUUIDRoles, "mode": {"catchup", "finalizer"}, "result": {"success", "failure"},
	},
	"oneapi_uuid_backfill_finalizer_total": {"role": privateUUIDRoles, "result": {"success", "failure"}},
}

// privateMetricFilter selects the same instrument-specific filter before SDK
// aggregation and again at the final export boundary.
func privateMetricFilter(name string) attribute.Filter {
	if enums, ok := privateUUIDInstrumentEnums[name]; ok {
		return func(kv attribute.KeyValue) bool {
			return kv.Value.Type() == attribute.STRING && slices.Contains(enums[string(kv.Key)], kv.Value.AsString())
		}
	}
	return privateMetricAttribute
}
