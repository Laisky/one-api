# Secure OTLP edge export

This opt-in topology sends one-api telemetry to a local persistent go-fluentd
agent, then an OTel Collector gateway and Victoria backends on home. Billing
records, quota writes and the SQL usage ledger remain unchanged.

## Configuration

Use a reviewed build and set these through the deployment secret mechanism:

```text
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=100.122.41.16:14319
OTEL_EXPORTER_OTLP_INSECURE=false
OTEL_EXPORTER_OTLP_CERTIFICATE=/run/observability/ca.crt
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer%20<edge-token>
OTEL_SERVICE_NAME=one-api
OTEL_ENVIRONMENT=production
OTEL_RESOURCE_ATTRIBUTES=host.name=b1
APP_LOG_SINK=stdout,otlp
LOG_OTLP_MIN_LEVEL=info
LOG_OTLP_QUEUE_SIZE=2000
LOG_OTLP_QUEUE_MAX_MB=16
LOG_OTLP_BATCH_SIZE=256
LOG_OTLP_EXPORT_TIMEOUT_MS=5000
OTEL_BSP_EXPORT_TIMEOUT=5000
OTEL_BSP_SCHEDULE_DELAY=1000
OTEL_METRIC_EXPORT_INTERVAL=60000
TRACE_WRITE_MODE=batched
TRACE_SINK=db,otlp
```

The endpoint is a host and port, not a URL path. Explicitly set `INSECURE=false`:
adding `https://` alone is insufficient because endpoint normalization strips
schemes. Percent-encode the header's space as `%20`, not `+`. The three real SDK
exporters use the shared certificate and header environment variables; the edge
transport test checks HTTPS, bearer, gzip, protobuf and nonempty records together.
Remove accidental per-signal SDK header/certificate overrides before rollout.
Never put a token in the endpoint URL, Git history, command line or diagnostics.

`stdout,otlp` retains a local application log sink; bare `otlp` is invalid.
Configure Docker log rotation with both max-size and max-file. The bounded SDK
queues deliberately do not block application requests on home availability.
Their contents can be lost before go-fluentd durably receives them. Setting
`db,otlp` preserves diagnostic SQL traces during comparison, but batching changes
in-flight trace visibility: completed records arrive after the request ends.
Do not remove SQL usage/billing rows after seeing an OTLP export succeed.

## Resource privacy and correlation

Automatic resource discovery now exports explicit runtime/PID fields, not
`process.command_args`, `process.owner` or executable paths. This is an
intentional privacy change from `WithProcess()`. Explicit operator attributes
from `OTEL_RESOURCE_ATTRIBUTES` remain supported and override host detection.
Do not place secrets there. Both `deployment.environment` (compatibility) and
`deployment.environment.name` are emitted; dashboards should migrate to the
latter without double-counting identical series.

This is not a generic redactor for log bodies, span attributes or request data.
Audit those fields before enabling export: filtering only on home is too late
because raw payloads already passed through the b1 WAL. Keep full prompts,
responses, API keys, credentials and cookies out of telemetry by source policy.

Use native OTel TraceID for log-to-trace correlation. Business request IDs remain
separate fields. Do not put request IDs, user IDs or token values into metric
labels. Existing no-op metric exemplars remain unchanged; metric-to-trace
exemplar navigation is not enabled by this integration. `TRACE_SAMPLE_RATE` does
not independently control every middleware OTel span; do not infer wire volume
from that value.

## Rollout, validation and rollback

Start the home shadow stack and local edge first. Verify the edge certificate
SAN matches the actual address and the private CA is mounted in one-api. A
container's 127.0.0.1 is not another container; this example uses b1's Tailscale
address and TLS. Network ACLs must limit the collector, query and management
ports independently.

Test a synthetic request through one-api, then locate its log and span and
observe application metrics. During home disconnection, measure one-api latency,
SDK drops, journal bytes and oldest backlog. The initial queue sizes above are
starting limits, not a 24-hour outage or throughput certification.

Rollback by restoring the previous OTLP endpoint and application-log settings,
then restarting one-api deliberately. Preserve the edge state for pending
exports. A go-fluentd v2 receipt-GC checkpoint is not compatible with older edge
binaries; never delete its metadata to make an old binary start. No database
schema migration or billing-read cutover is performed by this change.

Run `go test -race -count=3 ./common/telemetry ./common/config`; CI retains the
exact source, toolchain and JSON test results. The VPS companion workflow tests
real edge/gateway process kills and queries the Victoria stores, separately from
these SDK transport tests.
