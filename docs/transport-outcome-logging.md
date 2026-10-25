# Transport and relay outcome diagnostics

A `do_request_failed` normalized status of 500 is a local relay classification. It does not establish an upstream HTTP 500. A failed-attempt record can precede successful failover. Use the final controller record and the transport header evidence separately.

## Stages and meaning

| `log_stage` | Evidence | Limits |
|---|---|---|
| `transport_attempt` | Snapshot when `Client.Do` returns; exact returned upstream status only when a response exists | Local write completion is not inference acceptance; callbacks after the snapshot are not included |
| `relay_attempt` | Relay helper ordinal and result, including existing asynchronous failure diagnostics | Helper invocations include local transport validation; they are not wire-attempt counts |
| `retry_decision` | Existing selected-channel or selection-failure decision | Selection is not another provider dispatch |
| `relay_final` | Final helper result, retry outcome, writer commitment and committed writer status | A local write does not prove client receipt; uncommitted responses have no `client_status_code` |
| `accounting` | Retained reservation, failed balance refund, or refund call returning successfully on the instrumented paths below | Lifecycle flags do not prove final settlement; ledger update returning no error does not verify affected-row count |

Transport errors emit WARN metadata without another raw error or stack. This includes the existing cancellation/deadline cases; their severity is not changed. Returned HTTP 400-599 responses emit INFO even when `Client.Do` returns no error. Such responses retain `transport_failure_class=none` and `transport_failure_phase=none`: an HTTP failure is not a network failure. Other error-free transport completions and ordinary initial helper success remain DEBUG. A final failure or recovery after additional helper invocations emits one INFO summary. Existing relay error severities and billing decisions are preserved.

At an INFO export floor, both a pre-header reset and an actual upstream HTTP 500 therefore remain distinguishable. A reset has no invented `upstream_http_status`; the HTTP response has its returned status and `response_headers_received=true`. Higher configured severity floors, sampling and delivery loss can still exclude these records.

Native callback counters record observed connections, reused connections, successful/failed request writes and first response bytes. They saturate at 1,000,000, where they are lower bounds. `transport_trace_state=unknown` means no callback evidence; it must not be interpreted as proof that nothing was sent. The original trace callbacks and request cancellation channel are preserved. Counts aggregate automatic native retries and redirects where observed; they are separate from controller helper ordinals and do not locate a failure to an individual redirect hop. Response-body processing after `Client.Do` returns is outside this snapshot.

`relay_outcome=helper_failed` can coexist with a committed writer status of 200 after a partial SSE response. `client_outcome=committed_response_preserved` records that case without rewriting output. `uncommitted` means the helper returned before the controller committed bytes. Neither label claims a delivered response. A helper panic is not a normal completion and is not guaranteed to produce this final record.

## Accounting coverage

Accounting values distinguish `refund_balance_failed`, `refund_balance_applied` and `reservation_retained`. `balance_update=call_returned_no_error` reports the balance method result, not exactly-once settlement. Ledger values are `update_returned_no_error`, `failed`, `skipped` or `not_attempted`.

| Path | New structured accounting coverage |
|---|---|
| `conservativeRefundSnapshot.refund` | Retention, balance-call failure, or successful balance call plus the separate ledger-update result |
| `ResetPerAttemptBillingForRetry` | Detached balance refund failure or successful balance call plus the separate ledger-update result |
| `billingAuditSafetyNet`, possibly forwarded request | Retained reservation requiring investigation |
| `billingAuditSafetyNet`, emergency refund | No newly invented completion: its existing wrapper does not return a result |
| Audio/video `goRollbackPreConsumed` and `RefundQuotaWithRecovery` | Existing durable intent/recovery workflow; not covered by the new completion schema |
| Provider-specific retained-admission/Jina settlement and ordinary post-billing | Existing settlement mechanisms; not a complete lifecycle ledger in this schema |

Absence of an `accounting` event is not evidence that no refund or settlement happened. A successful balance refund can coexist with a failed ledger update. `update_returned_no_error` deliberately does not certify affected-row count, unique execution or final account balances. Financial reconciliation must use the existing financial records and recovery mechanisms, not log counts. No pricing, refund ownership, retry eligibility or financial mutation is changed here.

## Export pipelines and compatibility

The configured `APP_LOG_SINK=otlp` application-log path is full-fidelity: `common/logger/otlp_sink.go` constructs `otelbridge.NewCore`, and `common/telemetry/logs.go` installs `fidelityLogProcessor` before the bounded queue and HTTP exporter. Valid application messages and attributes are preserved, subject to existing severity, sampling, encoding and explicitly configured operator limits. Existing call-site sanitization remains unchanged. Do not assume this pipeline automatically removes arbitrary messages, URLs, headers or request identifiers.

`NewPrivacyCore` and `privacyLogProcessor` are separate, explicitly wired legacy privacy components. Only that projection replaces arbitrary messages and restricts fields to admitted scalar metadata and exact lifecycle enum values. `APP_LOG_SINK=otlp` does not select those components. The lifecycle privacy tests prove their narrow contract, not the behavior of the configured full-fidelity exporter. This change does not reconnect the legacy filters or introduce a new production field allowlist.

`TestLogFidelityProductionMetadataAndBody` and `TestLogFidelityProductionNoImplicitCaps` already exercise the production provider/HTTP exporter. `TestLifecycleDiagnosticsReachFidelityExporter` adds lifecycle-specific compatibility coverage, including application-defined field values that must remain intact. `TestLifecycleDiagnosticsReachPrivateQueue` separately covers the explicitly wired legacy projection. Test source alone is not evidence that a deployment or a particular commit has passed those tests; use the validation record for the exact commit.

The diagnostic fields added here contain bounded operational evidence, but the surrounding request logger can still carry existing application metadata. Apply the normal access controls to stored logs. These events are best-effort diagnostic records, not an accounting source of truth or a promise of central delivery.

## Central queries

Use the existing VictoriaLogs API/VMUI or Grafana datasource and an explicit UTC time window. Keep the normal service/host/environment stream restriction. The filters below select added operational fields; they do not change the export policy or redact other stored data.

```logsql
{service.name="one-api",host.name="b1",deployment.environment.name="production"}
AND log_stage:="relay_final"
| fields _time, relay_attempts, relay_outcome, retry_outcome, client_outcome, client_status_code, normalized_status_code, billing_reconciled_flag
```

```logsql
{service.name="one-api",host.name="b1",deployment.environment.name="production"}
AND log_stage:="transport_attempt"
| fields _time, response_headers_received, upstream_http_status, transport_failure_class, transport_failure_phase, request_context_state, transport_trace_state, transport_got_conn_count, transport_reused_conn_count, transport_write_completed_count
```

Correlate using the existing native trace field privately. Do not publish raw trace/request/account/token identifiers, addresses or payloads. Missing records can reflect coverage, export severity, sampling or delivery loss; absence is not proof an event did not occur. Structured records improve future attribution and cannot reconstruct missing historical transport callbacks.

## Focused regression commands

Run these with the repository's supported Go toolchain and dependencies. They are manual focused checks, not additions to automatic CI selection:

```sh
GOMAXPROCS=4 go test -race -p=4 -count=20 -timeout=180s ./relay/adaptor \
  -run '^TestTransportObservation' -json
GOMAXPROCS=4 go test -race -p=4 -count=1 -timeout=180s \
  ./common/telemetry ./common/logger/otelbridge \
  -run '^(TestLifecycleDiagnosticsReach|TestPrivacyLifecycleDiagnostics|TestLogFidelityProduction)' -json
```

These checks do not replace the existing relay/accounting regression suites. Record baseline failures, skipped qualifications and unavailable environments rather than declaring a broader suite green.

## Rollout and rollback

This is source-only observability work. It adds no retry, idempotency header, provider request, timeout, financial operation, database schema, collector setting or CI job. HTTP failure diagnostics add INFO volume; successful traffic retains DEBUG. Deployment and configuration changes are outside this change. An authorized rollout can verify the exact image and field delivery through the configured pipeline. Roll back through the normal image process if diagnostic volume or overhead is unacceptable; preserve collected evidence and existing export-policy choices. Rollback does not reconcile historical reservations.
