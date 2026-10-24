# Transport and relay outcome diagnostics

A `do_request_failed` normalized status of 500 is a local relay classification. It does not establish an upstream HTTP 500. A failed-attempt record can precede successful failover. Use the final controller record and the transport header evidence separately.

## Stages and meaning

| `log_stage` | Evidence | Limits |
|---|---|---|
| `transport_attempt` | Snapshot when `Client.Do` returns; exact returned upstream status only when a response exists | Local write completion is not inference acceptance; callbacks after the snapshot are not included |
| `relay_attempt` | Relay helper ordinal and result, including existing asynchronous failure diagnostics | Helper invocations include local transport validation; they are not wire-attempt counts |
| `retry_decision` | Existing selected-channel or selection-failure decision | Selection is not another provider dispatch |
| `relay_final` | Final helper result, retry outcome, writer commitment and committed writer status | A local write does not prove client receipt; uncommitted responses have no `client_status_code` |
| `accounting` | Retained reservation, failed balance refund, or refund call returning successfully | Lifecycle flags do not prove final settlement; ledger update returning no error does not verify affected-row count |

Transport failures emit WARN metadata without another raw error or stack. Successful transport and ordinary initial helper success remain DEBUG. A final failure or recovery after additional helper invocations emits one INFO summary. Existing failure severities and billing decisions are preserved. Actual refund completion emits one INFO accounting record; existing retained/error paths gain bounded fields.

Native callback counters record observed connections, reused connections, successful/failed request writes and first response bytes. They saturate at 1,000,000, where they are lower bounds. `transport_trace_state=unknown` means no callback evidence; it must not be interpreted as proof that nothing was sent. The original trace callbacks and request cancellation channel are preserved. Counts include automatic native retries and redirects where observed; they are separate from controller helper ordinals.

`relay_outcome=helper_failed` can coexist with a committed writer status of 200 after a partial SSE response. `client_outcome=committed_response_preserved` records that case without rewriting output. `uncommitted` means the helper returned before the controller committed bytes. Neither label claims a delivered response.

Accounting values distinguish `refund_balance_failed`, `refund_balance_applied` and `reservation_retained`. `balance_update=call_returned_no_error` reports the balance method result. Ledger values are `update_returned_no_error`, `failed`, `skipped` or `not_attempted`. The emergency audit refund uses a void wrapper and therefore has no newly invented success record. Generation/Jina settlement and all pricing/refund/retry policies are unchanged.

## Central queries

Use the existing VictoriaLogs API/VMUI or Grafana datasource and an explicit UTC time window. Keep the normal service/host/environment stream restriction. The production privacy core still replaces arbitrary messages, drops payloads, headers, URLs, errors and request IDs, and preserves native trace correlation. Only exact fixed operational enum values and typed numeric/boolean fields are additionally allowed; arbitrary strings under allowed keys are rejected at both the Zap and SDK boundaries.

Example filters inside an operator-selected bounded time window:

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

Correlate using the existing native trace field privately. Do not publish raw trace/request/account/token identifiers, addresses or payloads. Missing records can reflect export severity, sampling or delivery loss; absence is not proof an event did not occur. Structured records improve future attribution and cannot reconstruct missing historical transport callbacks.

## Rollout and rollback

This is source-only observability work. It adds no retry, idempotency header, provider request, timeout, financial operation, schema, collector setting or CI job. Deployment and configuration changes require the existing release process and are outside this change. A later approved rollout should verify the exact image, bounded synthetic/local acceptance and existing pipeline delivery. Roll back through the normal image process if diagnostic volume or overhead is unacceptable; preserve collected evidence. Do not weaken privacy controls to restore missing messages.
