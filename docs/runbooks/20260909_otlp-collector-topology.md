# Runbook: OTLP collector topology for one-api

- Date: 2026-09-09
- Area: OpenTelemetry export (traces, metrics, optional application logs); OpenTelemetry Collector deployment
- Audience: platform operators running one-api with `OTEL_ENABLED=true`
- Design reference: [Observability Data Tiering](../proposals/20260905_observability-data-tiering.md),
  §6 **W3.1 — Reuse the Phase 1 trace pipeline**, and §4 **W1** (the OTLP and sampling rows)
- Companion records: [W0/W1 acceptance evidence](../benchmarks/20260908_w0-w1-acceptance.md),
  [OpenTelemetry Operational Manual](../manuals/open_telemetry.md),
  [Prometheus Monitoring](../manuals/PROMETHEUS.md),
  [Request Tracing System Architecture](../arch/tracing_system.md)

W3.1 says, verbatim:

> Close W1's existing-span and provider validation gates; do not duplicate the implemented
> OTLP sink as new work. A single collector is sufficient for ordinary export. Two-tier
> collectors with trace-ID-affine routing and stateful tail sampling are optional for a
> measured distributed tracing requirement. Document memory, maximum trace duration,
> `decision_wait`, incomplete/late spans and collector failure behavior.

The code gates in that sentence were closed by the W0/W1 remediation — one SERVER span per
request is measured in [§9 of the acceptance record](../benchmarks/20260908_w0-w1-acceptance.md),
and provider validation is enforced in `common/config/validation.go` and
`common/tracing/sink.go`. **What remained was this operations document**, and that is all
this file is. It writes no code and changes no default.

## House rules for this document

This document follows the conventions of the acceptance record it sits beside. Every claim
carries one of four labels, and nothing is asserted without one:

| Label | Meaning |
| --- | --- |
| **upstream default** | Quoted or paraphrased from an OpenTelemetry Collector README, `documentation.md` or `CHANGELOG.md`, with the link beside it. Verified against `main` at collector release **v1.66.0/v0.160.0** (published 2026-09-02) on 2026-09-09. |
| **repo fact** | Read out of this repository on 2026-09-09, with the file path beside it. Metric names, env var names and defaults in this document were grepped, not remembered. |
| **our recommendation** | A judgement made here. It is not upstream guidance and it is not measured. |
| **estimate / arithmetic** | A number derived by multiplication, not by running anything. Never a measurement of this gateway. |
| **uncertain** | Could not be verified against either the repository or an upstream source. Stated as a question, not a fact. |

There are **no measurements of this gateway anywhere in this document.** See §10.

---

## Table of contents

- [1. What one-api actually sends](#1-what-one-api-actually-sends)
- [2. One collector is the default recommendation](#2-one-collector-is-the-default-recommendation)
- [3. `memory_limiter` is effectively mandatory](#3-memory_limiter-is-effectively-mandatory)
- [4. `sending_queue`, and why its default sizer is wrong for logs](#4-sending_queue-and-why-its-default-sizer-is-wrong-for-logs)
- [5. Optional: the two-tier tail-sampling topology](#5-optional-the-two-tier-tail-sampling-topology)
- [6. Maximum trace duration: streaming relays and `decision_wait`](#6-maximum-trace-duration-streaming-relays-and-decision_wait)
- [7. The SDK/collector sampling relationship](#7-the-sdkcollector-sampling-relationship)
- [8. Collector failure behavior, from the gateway's side](#8-collector-failure-behavior-from-the-gateways-side)
- [9. Alert and dashboard summary](#9-alert-and-dashboard-summary)
- [10. What this document does NOT establish](#10-what-this-document-does-not-establish)
- [11. Sources](#11-sources)

---

## 1. What one-api actually sends

Configure the collector for the traffic the gateway really produces, not for a generic OTLP
client. Everything in this section is a **repo fact**.

### 1.1 Transport

| Property | Value | Where |
| --- | --- | --- |
| Protocol | **OTLP over HTTP/protobuf**, never gRPC | `common/telemetry/telemetry.go` imports `otlptracehttp` / `otlpmetrichttp`; `common/telemetry/logs.go` imports `otlploghttp` |
| Compression | gzip on all three signals | `buildTraceExporterOptions`, `buildMetricExporterOptions` (`common/telemetry/telemetry.go`), `buildLogExporterOptions` (`common/telemetry/logs.go`) |
| Endpoint form | `host:port`, no scheme. A leading `http://` or `https://` is stripped for you | `normalizeOTLPEndpoint`, `common/config/observability_matrix.go` |
| Default port used in examples | `4318` (the OTLP/HTTP port) | `docs/manuals/open_telemetry.md` |
| TLS | `OTEL_EXPORTER_OTLP_INSECURE` defaults to **`true`** — plain HTTP | `common/config/config.go` |

> **Consequence for the collector**: the `otlp` receiver **must have the `http:` protocol
> block enabled**. A collector configured with only `protocols: {grpc: ...}` will accept
> nothing from one-api, and the gateway will report transport failures on stderr (§8.3)
> rather than in any metric.

### 1.2 Signals

| Signal | Emitted when | Notes |
| --- | --- | --- |
| Traces | `OTEL_ENABLED=true` | Two producers: `otelgin` middleware spans (always, for every non-excluded request) and, when `TRACE_SINK` includes `otlp`, the gateway's own enrichment of that same span. §7 is entirely about why these are not the same thing. |
| Metrics | `OTEL_ENABLED=true` | Periodic reader; this is the `monitor/otel` recorder mirror of the Prometheus series. |
| Application logs | `OTEL_ENABLED=true` **and** `APP_LOG_SINK` contains the additive `otlp` token | Off by default in every profile, including `external`. A bare `APP_LOG_SINK=otlp` is rejected at startup — the token is additive only, so a local sink of record always survives (`common/config/observability_app_log_otlp.go`). |

### 1.3 SDK-side buffering, and the drop the gateway cannot see

The tracer provider is built with a UTF-8 attribute sanitizer followed by
`sdktrace.WithBatcher(exporter)` (`common/telemetry/telemetry.go`). No `WithSampler` is
passed anywhere in the repository — see §7.3, which turns that into an operator knob.

Batch span processor defaults, from `go.opentelemetry.io/otel/sdk@v1.46.0/trace/batch_span_processor.go`
(**upstream default**, read from the pinned module in `go.mod`):

| Setting | Default | Env override |
| --- | --- | --- |
| `MaxQueueSize` | 2048 spans | `OTEL_BSP_MAX_QUEUE_SIZE` |
| `BatchTimeout` (schedule delay) | 5000 ms | `OTEL_BSP_SCHEDULE_DELAY` |
| `ExportTimeout` | 30000 ms | `OTEL_BSP_EXPORT_TIMEOUT` |
| `MaxExportBatchSize` | 512 spans | `OTEL_BSP_MAX_EXPORT_BATCH_SIZE` |

> **This is the single largest observability blind spot in the export path.** When the BSP
> queue is full the SDK calls `enqueueDrop`, increments a private `bsp.dropped` counter, and
> returns. The count surfaces only in a `global.Debug("exporting spans", …, "total_dropped", …)`
> line, which the default OTel logger does not print. **There is no one-api metric for it**
> (`oneapi_trace_records_total` is counted *before* the span reaches the BSP — see §8.2).
> A collector that is slow to accept OTLP/HTTP requests therefore loses spans silently at
> the gateway, not at the collector. Raising `OTEL_BSP_MAX_QUEUE_SIZE` buys headroom; it
> does not make the loss visible.

### 1.4 Gateway configuration relevant to export

All **repo facts**; defaults from `common/config/config.go` and
`common/config/observability_app_log_otlp.go`, and profile columns from
[proposal §3.1](../proposals/20260905_observability-data-tiering.md).

| Variable | Default | Notes |
| --- | --- | --- |
| `OTEL_ENABLED` | `false` | The `external` profile does **not** turn it on or populate the endpoint. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(empty; required when enabled)* | `host:port`. Scheme stripped. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `true` | |
| `OTEL_SERVICE_NAME` | `one-api` | Becomes `service.name`; this is the `routing_key: service` value in §5.9. |
| `OTEL_ENVIRONMENT` | `debug` | Set it. `debug` in production is a labelling accident waiting to happen. |
| `TRACE_SINK` | `db` (standalone/scaled), `otlp` (external) | `otlp` requires `OTEL_ENABLED=true` and `TRACE_WRITE_MODE=batched`; both are enforced at startup. |
| `APP_LOG_SINK` | `both` | Add `,otlp` to enable the log bridge, e.g. `both,otlp`. |
| `LOG_OTLP_MIN_LEVEL` | `info` | Independent of `LOG_LEVEL`/`DEBUG`. |
| `LOG_OTLP_QUEUE_SIZE` | `10000` records | Bridge-side bound; drops are counted (§8.2). |
| `LOG_OTLP_QUEUE_MAX_MB` | `64` MiB | Second bound, because record count does not bound memory. |
| `LOG_OTLP_BATCH_SIZE` | `512` | SDK default. |
| `LOG_OTLP_EXPORT_INTERVAL_MS` | `1000` | SDK default. |
| `LOG_OTLP_EXPORT_TIMEOUT_MS` | `30000` | The shutdown deadline must exceed this or the final drain is cut short. |
| `LOG_OTLP_MAX_ATTRIBUTES` | `128` | |
| `LOG_OTLP_MAX_ATTRIBUTE_VALUE_BYTES` | `4096` | The SDK default is unlimited; one-api bounds it. |

---

## 2. One collector is the default recommendation

**W3.1 is explicit: "A single collector is sufficient for ordinary export."** This document
does not soften that. A single collector (or a horizontally scaled, stateless *set* of
identical collectors behind a load balancer) is the recommendation for every one-api
deployment that is not running distributed tail sampling.

### 2.1 When one collector is enough

One collector — or a stateless pool of them — is enough whenever **no component in the
pipeline needs to see all spans of a trace in the same process.** That is the entire
criterion, and it holds for:

- exporting traces to Tempo / Jaeger / VictoriaTraces / any OTLP backend;
- exporting application logs to Loki / VictoriaLogs / any OTLP log backend;
- head-sampled or unsampled trace volumes;
- probabilistic sampling in the collector (`probabilistic_sampler`), which is per-span and
  needs no cross-span state;
- attribute enrichment, redaction, resource detection, filtering.

You need the two-tier topology of §5 **only** when you have a *measured* requirement for
stateful tail sampling — that is, when you can state the trace volume, the retention target
and the decisions you cannot make head-first. "We might want tail sampling later" is not a
measured requirement; it is a reason to keep §5 bookmarked.

**Our recommendation:** scale the single tier horizontally (more replicas of the same
stateless config) before you consider adding a second tier. Two tiers double the number of
processes that can fail, and the failure mode of the routing tier (§5.9) is subtle.

### 2.2 Single-collector pipeline: traces and logs

The config below is complete and deliberately minimal. Every non-obvious line is explained
in §3 and §4. Replace `tempo:4317` / `loki:4317` with your backends.

```yaml
receivers:
  otlp:
    protocols:
      # REQUIRED for one-api: the gateway speaks OTLP/HTTP, never gRPC (§1.1).
      http:
        endpoint: 0.0.0.0:4318
      # Optional: only if other producers in your cluster speak OTLP/gRPC.
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  # MUST be first in every pipeline (§3.2). check_interval has NO useful default (§3.1).
  memory_limiter:
    check_interval: 1s
    limit_mib: 1600          # size against the container's hard limit, see §3.3
    spike_limit_mib: 320     # 20% of limit_mib is the upstream starting point

exporters:
  otlp/traces:
    endpoint: tempo:4317
    # `timeout` is a SIBLING of sending_queue, not a key inside it (§4.1).
    timeout: 5s
    sending_queue:
      enabled: true
      num_consumers: 10
      # `items` = spans. The default `requests` does not bound memory (§4.2).
      sizer: items
      queue_size: 200000
      block_on_overflow: false
      # Exporter-level batching. Prefer this over the `batch` processor (§4.5).
      batch:
        flush_timeout: 200ms
        min_size: 8192
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 300s

  otlp/logs:
    endpoint: loki:4317
    timeout: 5s
    sending_queue:
      enabled: true
      num_consumers: 10
      # `items` = log records. See §4.2 — this is the important line for logs.
      sizer: items
      queue_size: 200000
      block_on_overflow: false
      batch:
        flush_timeout: 200ms
        min_size: 8192
    retry_on_failure:
      enabled: true

service:
  telemetry:
    metrics:
      level: normal
      readers:
        - pull:
            exporter:
              prometheus:
                host: 0.0.0.0
                port: 8888
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter]
      exporters: [otlp/traces]
    logs:
      receivers: [otlp]
      processors: [memory_limiter]
      exporters: [otlp/logs]
    metrics:
      receivers: [otlp]
      processors: [memory_limiter]
      exporters: [otlp/traces]   # replace with your metrics backend
```

> **uncertain:** the `service::telemetry::metrics::readers` spelling above is the current
> form. The older `service::telemetry::metrics::address: 0.0.0.0:8888` was deprecated; this
> document did **not** verify which release removed it. Check your collector version's
> `service` documentation before copying, and confirm `curl localhost:8888/metrics` returns
> `otelcol_*` series — every alarm in §9 depends on that endpoint existing.

### 2.3 Verification after applying

```bash
# 1. the collector accepts OTLP/HTTP at all
curl -sS -X POST http://<collector>:4318/v1/traces \
  -H 'Content-Type: application/json' -d '{"resourceSpans":[]}' -i | head -1
# expect: HTTP/1.1 200 OK

# 2. the collector's own metrics endpoint is reachable
curl -sS http://<collector>:8888/metrics | grep -c '^otelcol_'

# 3. the gateway believes it initialized (one-api log line, common/telemetry/telemetry.go)
#    "OpenTelemetry initialized" with endpoint / insecure / service / environment / app_log_otlp

# 4. the gateway's own accounting is moving (see §8.2 for the label vocabulary)
curl -sS -H "Authorization: Bearer $METRICS_TOKEN" http://<one-api>:3000/metrics \
  | grep -E 'oneapi_trace_records_total|oneapi_app_log_export_records_total'
```

`METRICS_TOKEN` and the `/metrics` endpoint contract are documented in
[PROMETHEUS.md](../manuals/PROMETHEUS.md) (**repo fact**: when `METRICS_TOKEN` is unset the
endpoint returns 403).

---

## 3. `memory_limiter` is effectively mandatory

The collector has no other backstop against an OOM kill when a backend slows down. Without
`memory_limiter`, exporter queues grow, the Go heap grows, and the container is killed —
taking every buffered span and log record with it. **Our recommendation: treat
`memory_limiter` as a required processor in every pipeline, in every deployment, including
the single-collector one in §2.2.**

### 3.1 The default that gives you nothing

| Key | Upstream default | What it means |
| --- | --- | --- |
| `check_interval` | **`0s`** | *"Time between measurements of memory usage. **The recommended value is 1 second.** If the expected traffic to the Collector is very spiky then decrease the `check_interval` or increase `spike_limit_mib`."* |
| `limit_mib` | `0` (unset) | Hard limit on heap allocation, in MiB. |
| `limit_percentage` | `0` (unset) | *"Maximum amount of total memory targeted to be allocated by the process heap. This configuration is supported on **Linux systems with cgroups** and it's intended to be used in dynamic platforms like docker. This option is used to calculate `memory_limit` from the **total available memory**."* `limit_mib` takes precedence over it. |
| `spike_limit_mib` | **`20%` of `limit_mib`** | *"Maximum spike expected between the measurements of memory usage. The value must be less than `limit_mib`. The soft limit value will be equal to (`limit_mib - spike_limit_mib`)."* |
| `spike_limit_percentage` | `0` (unset) | *"intended to be used only with `limit_percentage`."* |

Source: [`processor/memorylimiterprocessor/README.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/processor/memorylimiterprocessor/README.md)
(all quotes above are **upstream defaults**, verified 2026-09-09).

> **The trap:** `check_interval` defaults to `0s`, and upstream's own text says the
> recommended value is 1 second. An operator who writes `memory_limiter: {}` — or who sets
> `limit_mib` and nothing else — gets a processor that is present in the pipeline and
> **provides no protection**. Always set `check_interval` explicitly.

Two nuances worth carrying:

1. **`limit_percentage` is of "total available memory", not literally "the pod limit."**
   Upstream never says "pod limit". On Linux with cgroups the collector reads the cgroup
   memory limit, which in a Kubernetes pod *is* the container limit — but the wording is
   about available memory, and the option does nothing on non-Linux hosts or without
   cgroups. **Our recommendation: use `limit_mib` when you know the container limit** (you
   almost always do), and reserve `limit_percentage` for platforms where the limit is
   assigned dynamically.
2. **`spike_limit_*` is coupled to `check_interval`.** Upstream: *"The value of the
   `spike_limit_mib` configuration option should be selected in a way that ensures that
   memory usage cannot increase by more than this value within a single memory check
   interval. … A good starting point for `spike_limit_mib` is 20% of the hard limit."*
   Halving `check_interval` lets you lower `spike_limit_mib`; raising it forces you to
   raise the spike allowance, because the limiter is blind between checks.

### 3.2 It must be the first processor

Upstream, from the same README: *"For the `memory_limiter` processor, the **best practice is
to add it as the first processor in a pipeline**. This is to ensure that backpressure can be
sent to applicable receivers and minimize the likelihood of dropped data when the
`memory_limiter` gets triggered."*

(Upstream words this as a best practice, not a hard "must". **Our recommendation** is to
treat it as a must: a limiter placed after an enrichment processor refuses data that has
already been copied and expanded, which is exactly the memory you were trying not to spend.)

When the limiter trips it **refuses** data, and the receiver translates that refusal into a
non-200 OTLP response. For one-api that response lands on the SDK's retry path, and — if the
BSP queue fills while retrying — into the silent drop described in §1.3.

### 3.3 `GOMEMLIMIT` is complementary, not a substitute

Upstream: *"It is highly recommended to configure the `GOMEMLIMIT` environment variable as
well as the `memory_limiter` processor on every collector. **`GOMEMLIMIT` should be set to
80% of the hard memory limit of your collector.**"* and *"it is not a replacement for
properly sizing and configuring the collector."*

Worked sizing for a 2 GiB container (**arithmetic**, not a measurement):

| Knob | Value | Rationale |
| --- | --- | --- |
| container memory limit | 2048 MiB | the hard kill boundary |
| `GOMEMLIMIT` | `1600MiB` | 80% of the hard limit, per upstream |
| `memory_limiter.limit_mib` | `1600` | keep the limiter's hard limit at or below `GOMEMLIMIT` so refusal happens before the GC starts thrashing |
| `memory_limiter.spike_limit_mib` | `320` | 20% starting point; soft limit becomes 1280 MiB |
| `memory_limiter.check_interval` | `1s` | upstream recommendation |

They do different jobs: `GOMEMLIMIT` makes the Go runtime collect harder as the heap
approaches a soft ceiling; `memory_limiter` refuses *inbound data* so the heap stops growing
in the first place. Only the second one produces backpressure.

### 3.4 Two version traps

| Trap | Affected versions | What happens | Fix |
| --- | --- | --- | --- |
| **Forced-GC CPU burn** | before **v1.62.0/v0.156.0** | With an exporter failing (backend outage) the memory the limiter wants to reclaim is held by live references in the exporter queues. The processor kept forcing GC that reclaimed nothing, *"causing permanent CPU-burning GC loop"*. The collector then cannot recover, because it has no CPU left to drain. | Upgrade. v0.156.0 added exponential backoff for ineffective forced GC, capped by **`max_gc_interval_when_soft_limited`** and **`max_gc_interval_when_hard_limited`**, both defaulting to **`30s`** (set either to `0` to disable backoff on that path). There are matching `min_gc_interval_when_*` options that set the starting interval. Issue [#4981](https://github.com/open-telemetry/opentelemetry-collector/issues/4981). |
| **Metric rename** | **v1.61.0/v0.155.0**, listed as a **breaking change** | The memory limiter's own metrics gained a `memory_limiter` prefix. Dashboards and alerts written against the old names go silent — which looks exactly like "the limiter never trips". | Update queries to the `otelcol_processor_memory_limiter_*` names below. |

Old → new names (**upstream**, from the processor's `documentation.md` at
[v0.154.0](https://github.com/open-telemetry/opentelemetry-collector/blob/v0.154.0/processor/memorylimiterprocessor/documentation.md)
and [main](https://github.com/open-telemetry/opentelemetry-collector/blob/main/processor/memorylimiterprocessor/documentation.md)):

| Old (≤ v0.154.0) | New (≥ v0.155.0) |
| --- | --- |
| `otelcol_processor_accepted_spans` | `otelcol_processor_memory_limiter_accepted_spans` |
| `otelcol_processor_accepted_metric_points` | `otelcol_processor_memory_limiter_accepted_metric_points` |
| `otelcol_processor_accepted_log_records` | `otelcol_processor_memory_limiter_accepted_log_records` |
| `otelcol_processor_refused_spans` | `otelcol_processor_memory_limiter_refused_spans` |
| `otelcol_processor_refused_metric_points` | `otelcol_processor_memory_limiter_refused_metric_points` |
| `otelcol_processor_refused_log_records` | `otelcol_processor_memory_limiter_refused_log_records` |

### 3.5 What to alarm on

| Condition | Meaning | Our recommendation |
| --- | --- | --- |
| `rate(otelcol_processor_memory_limiter_refused_spans[5m]) > 0` | The limiter is shedding. Data is being refused upstream, and one-api may be dropping in its BSP (§1.3). | Page if sustained > 5 min. |
| `rate(otelcol_processor_memory_limiter_refused_log_records[5m]) > 0` | Same, for logs. | Page if sustained > 5 min. |
| Collector CPU pinned while an exporter is failing, on a collector older than v0.156.0 | Almost certainly the forced-GC loop in §3.4. | Upgrade; do not tune around it. |

---

## 4. `sending_queue`, and why its default sizer is wrong for logs

### 4.1 Defaults

From [`exporter/exporterhelper/README.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/README.md)
(all **upstream defaults**, verified 2026-09-09 at v1.66.0/v0.160.0):

| Key | Default | Notes |
| --- | --- | --- |
| `sending_queue::enabled` | `true` | On by default; you are already using it. |
| `sending_queue::num_consumers` | `10` | Concurrent senders draining the queue. |
| `sending_queue::queue_size` | `1000` | *"Maximum size the queue can accept. Measured in units defined by `sizer`."* |
| `sending_queue::sizer` | **`requests`** | `requests` \| `items` \| `bytes`. See §4.2. |
| `sending_queue::block_on_overflow` | `false` | *"If true, blocks the request until the queue has space otherwise rejects the data immediately."* |
| `sending_queue::wait_for_result` | `false` | Not in most people's mental model; leave it off unless you know why you want it. |
| `sending_queue::batch` | *absent* → **batching OFF** | *"Batch settings are available in the sending queue. Batching is disabled, by default. To enable default batch settings, use `batch: {}`."* |
| `sending_queue::batch::flush_timeout` | `200ms` | Must be non-zero. |
| `sending_queue::batch::min_size` | `8192` | Unit is `batch::sizer`, which *"defaults to `items`"* when `sending_queue::sizer` is not set. |
| `sending_queue::batch::max_size` | `0` | `0` means no maximum; a non-zero value enables batch splitting. |
| `timeout` | `5s` | **Correction worth stating loudly: `timeout` is a SIBLING of `sending_queue`, not a key inside it.** *"Time to wait per individual attempt to send data to a backend."* |

> **Config-load trap, same class as `blocking` below:** the collector's confmap decoder sets
> `ErrorUnused` by default (`confmap/internal/decoder.go`), so an unknown key is a hard
> startup error, not a warning. Nesting `timeout:` inside `sending_queue:` will now fail
> validation outright.

### 4.2 `sizer: requests` does not bound memory for logs

This is the most consequential default in the whole exporter.

With `sizer: requests`, `queue_size: 1000` means **one thousand batches**, not one thousand
log records. A "batch" is whatever arrived as one OTLP request. One-api's log bridge sends
up to `LOG_OTLP_BATCH_SIZE=512` records per export (**repo fact**), but nothing in the
pipeline guarantees uniformity: other producers, retries, and merged batches all vary. So
the queue's memory footprint is `1000 × (whatever a batch happens to be)` — which is
**effectively unbounded**, because the multiplicand is not bounded.

The same reasoning applies to traces, but it bites hardest on logs, where a single record
can carry a large field. (One-api bounds its *own* side at 4 KiB per attribute value and
64 MiB of resident queue — `LOG_OTLP_MAX_ATTRIBUTE_VALUE_BYTES`, `LOG_OTLP_QUEUE_MAX_MB` —
but that bounds the gateway's memory, not the collector's.)

Upstream's own description of the three sizers:

> *`requests`: number of incoming batches of metrics, logs, traces (the most performant
> option); `items`: number of the smallest parts of each signal (spans, metric data points,
> log records); `bytes`: the size of serialized data in bytes (the least performant option).*

**Our recommendation:**

| If you want to bound… | Use | Set `queue_size` in units of |
| --- | --- | --- |
| a predictable record count (the usual case) | `sizer: items` | log records / spans |
| memory as literally as possible | `sizer: bytes` | serialized bytes |
| throughput above all, and you have measured your batch sizes | `sizer: requests` (the default) | batches |

Note the mixed-unit hazard when you enable batching: `queue_size: 1000` is in *requests* by
default while `batch::min_size: 8192` is in *items*. Upstream only requires them to be
comparable *"if `sending_queue::batch::sizer` matches `sending_queue::sizer`"*, and the
config `Validate()` only enforces `min_size <= queue_size` when the two sizers are equal.
Setting `sizer: items` at the queue level, as §2.2 does, makes both numbers mean the same
thing and removes the hazard.

### 4.3 `sending_queue::blocking` was REMOVED, not deprecated

**Upstream, `CHANGELOG.md` at `v1.35.0/v0.129.0`, under 🛑 Breaking changes:**
*"`exporterhelper`: Remove deprecated `sending_queue::blocking` options, use
`sending_queue::block_on_overflow`. (#13211)"* (Deprecated earlier in #12710.)

Because the field no longer exists in the config struct and the decoder rejects unknown
keys, **a config file still carrying `blocking:` fails to load** with a mapstructure
"has invalid keys" error. The collector does not start. This is a good failure — but it is
a *startup* failure, so it will be discovered during a rollout, not during config review.

```yaml
# WRONG on v0.129.0+ — collector refuses to start
sending_queue:
  blocking: true

# RIGHT
sending_queue:
  block_on_overflow: true
```

**Our recommendation for one-api specifically: leave `block_on_overflow: false`.** Blocking
on overflow pushes backpressure to the receiver and then to the gateway's SDK, where the
consequence is the invisible BSP drop of §1.3 plus added latency on the exporter goroutine.
Losing telemetry visibly (a counter you can alarm on) is better than losing it invisibly.

### 4.4 `retry_on_failure` does not cover queue-full

**Upstream:** *"**Failure behavior**: If data cannot be added to the sending queue, it is
typically dropped. … **If data is rejected before entering the queue, it does not reach the
exporter retry logic. Such enqueue failures are reported by the
`otelcol_exporter_enqueue_failed_*` metrics.**"*

So the two loss paths are separate and need separate alarms:

| Loss path | Covered by `retry_on_failure`? | Metric |
| --- | --- | --- |
| Backend rejected or timed out an in-flight send | Yes | `otelcol_exporter_send_failed_log_records` / `_spans` |
| Queue was full when the data arrived | **No** | `otelcol_exporter_enqueue_failed_log_records` / `_spans` |

Metric definitions (**upstream**, from
[`exporter/exporterhelper/documentation.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/documentation.md)):

- `otelcol_exporter_enqueue_failed_log_records` — *"Number of log records failed to be added
  to the sending queue."*
- `otelcol_exporter_queue_size` — *"Current size of the retry queue (in batches)."*
- `otelcol_exporter_queue_capacity` — *"Fixed capacity of the retry queue (in batches)."*

> The `(in batches)` wording in the help text predates configurable sizers. With
> `sizer: items` the same series is counted in items. Treat the help text as stale and trust
> your `sizer`. (**uncertain**: whether upstream intends to update the help text.)

### 4.5 `batch` processor vs exporter-level batching

Status as of collector **v1.66.0/v0.160.0**, checked 2026-09-09:

- The `batch` processor is **not formally deprecated**. `processor/batchprocessor/metadata.yaml`
  still declares `stability: beta: [traces, metrics, logs]`, the README carries no
  deprecation notice, and no deprecation entry exists in `CHANGELOG.md`.
- The **merged RFC states the intent to remove it**:
  [`docs/rfcs/batching-migration.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/rfcs/batching-migration.md),
  *"Migration for `batchprocessor` and exporterhelper batching"* — *"### What success looks
  like — The `batchprocessor` is removed from the standard distribution and, eventually,
  from the core repository."*
- The RFC's phase table (**upstream, but a plan, not a shipped fact**): a `queuebatch`
  processor landed in v0.158.0 as the replacement; deprecation of `batchprocessor` is
  scheduled for v0.160.0 and had **not** landed in that release's changelog; removal from
  the default distribution is scheduled for v0.166.0, at which point *"pipelines using the
  removed component in the default binary fail to start"*.

**Our recommendation: use `sending_queue::batch` in new configs, as §2.2 does.** Existing
`batch:` processors keep working today and there is no urgency to rip them out, but there is
no reason to add new ones either. If you keep a `batch` processor, it goes *after*
`memory_limiter` and — if you ever adopt §5 — *before* `tail_sampling` is meaningless,
because tail sampling reassembles batches anyway (§5.8e).

### 4.6 What to alarm on, and when to scale up

| Signal | Threshold | Meaning |
| --- | --- | --- |
| `otelcol_exporter_queue_size / otelcol_exporter_queue_capacity` | **> 0.6–0.7 sustained** | The queue is filling faster than `num_consumers` drains it. **Our recommendation:** scale out (more collector replicas) or up (`num_consumers`, `queue_size`) *before* it reaches 1.0, because at 1.0 you are already dropping. |
| `rate(otelcol_exporter_enqueue_failed_log_records[5m]) > 0` | any | Queue-full drops. **`retry_on_failure` did not and cannot cover these** (§4.4). Page. |
| `rate(otelcol_exporter_enqueue_failed_spans[5m]) > 0` | any | Same for traces. |
| `rate(otelcol_exporter_send_failed_log_records[5m]) > 0` | sustained | Backend rejecting or unreachable; retry is engaged and the queue is about to fill. |

---

## 5. Optional: the two-tier tail-sampling topology

### 5.1 Read this before building it

W3.1: *"Two-tier collectors with trace-ID-affine routing and stateful tail sampling are
**optional for a measured distributed tracing requirement**."*

**Do not build this section unless all of the following are true.** (**Our recommendation**,
not upstream guidance.)

1. You have a written statement of what decision you need tail sampling to make that head
   sampling cannot — e.g. "keep every trace containing an upstream 5xx, at 3% of total
   volume" — and it depends on spans that arrive *after* the decision point.
2. You have measured your span volume and your trace shape (spans per trace, bytes per
   trace, and, critically for a gateway, **trace duration** — see §6).
3. You accept that this tier is **stateful**, so it is the one component in the pipeline
   that cannot be scaled by simply adding replicas behind a round-robin load balancer.
4. You have somewhere to put two deployments instead of one.

If you cannot write down (1), the honest answer is a `probabilistic_sampler` in the single
collector of §2.2, or the SDK head sampler of §7.3, neither of which needs any of this.

Note also that one-api's own local selection is already a completion-time decision inside a
single process — `common/tracing/sampling.go` states this directly: the gateway *"already
holds the complete trace in one process, so status and duration are known for free at
decision time"*, and needs *"neither a `decision_wait` buffer nor a trace-id-aware load
balancer"*. Collector tail sampling buys you something different: decisions over spans from
**other services** in the same trace. If one-api is the only instrumented service in the
trace, collector tail sampling is buying you very little that §7.3 and `TRACE_SAMPLE_RATE`
do not already provide.

### 5.2 Topology

```
                       ┌──────────────────────────────┐
   one-api (N pods) ──▶│ Layer 1 — routing            │   stateless, scale freely
   OTLP/HTTP :4318     │  otlp receiver               │
                       │  memory_limiter   (FIRST)    │
                       │  k8sattributes / enrichment  │◀── context-dependent processors
                       │  load_balancing exporter     │    live HERE, not in layer 2
                       └───────────────┬──────────────┘
                                       │  routing_key: traceID (consistent hashing)
                       ┌───────────────▼──────────────┐
                       │ Layer 2 — sampling           │   STATEFUL, trace-affine
                       │  otlp receiver               │
                       │  memory_limiter   (FIRST)    │
                       │  tail_sampling               │
                       │  otlp exporter → backend     │
                       └──────────────────────────────┘
```

Upstream recommends exactly this split, and says why:

> *"You can achieve this by having two layers of collectors in your infrastructure: one with
> the [load balancing exporter], and one with the tail sampling processor. **While it's
> technically possible to have one layer of collectors with two pipelines on each instance,
> we recommend separating the layers in order to have better failure isolation.**"*
> — [`processor/tailsamplingprocessor/README.md`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md)

### 5.3 Layer 1 — routing

```yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 1600
    spike_limit_mib: 320
  # Context-dependent processors MUST run here, in layer 1. Tail sampling
  # reassembles batches and loses their context (§5.8e).
  k8sattributes: {}

exporters:
  # RENAMED in v0.153.0. `loadbalancing:` still resolves as a deprecated alias (§5.9a).
  load_balancing:
    # routing_key defaults to traceID for traces; stated explicitly because the
    # correctness of the whole topology depends on it (§5.9b).
    routing_key: traceID
    resolver:
      k8s:
        service: otel-tailsampling.observability
        ports: [4317]
    # ------------------------------------------------------------------
    # THE MOST-MISSED PITFALL (§5.9e): these three are OFF by default at the
    # TOP LEVEL of load_balancing. Without them the exporter will NOT re-route
    # to a healthy endpoint on delivery failure.
    # ------------------------------------------------------------------
    timeout: 10s
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 120s
    sending_queue:
      enabled: true
      sizer: items
      queue_size: 100000
      block_on_overflow: false
    # The per-backend sub-exporter. Its own queue/retry/timeout ARE enabled by
    # default; data only comes BACK to the load balancer for re-routing after
    # this sub-exporter's redelivery is exhausted.
    protocol:
      otlp:
        tls:
          insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, k8sattributes]
      exporters: [load_balancing]
```

### 5.4 Layer 2 — sampling

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 3200
    spike_limit_mib: 640
  tail_sampling:
    decision_wait: 30s          # upstream default; §6 explains why a gateway may need more
    num_traces: 50000           # upstream default
    expected_new_traces_per_sec: 0   # a PRE-ALLOCATION HINT ONLY; caps nothing (§5.5)
    # Highest-value mitigation for late spans and split traces (§5.8a).
    # Upstream: set "much greater than num_traces".
    decision_cache:
      sampled_cache_size: 500000
      non_sampled_cache_size: 500000
    policies:
      - name: errors
        type: status_code
        status_code: {status_codes: [ERROR]}
      - name: slow
        type: latency
        latency: {threshold_ms: 5000}
      - name: baseline
        type: probabilistic
        probabilistic: {sampling_percentage: 3}

exporters:
  otlp/backend:
    endpoint: tempo:4317
    timeout: 5s
    sending_queue:
      enabled: true
      sizer: items
      queue_size: 200000
      batch:
        flush_timeout: 200ms
        min_size: 8192
    retry_on_failure:
      enabled: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      # tail_sampling comes AFTER anything context-dependent — which is why
      # k8sattributes is in layer 1 and not here (§5.8e).
      processors: [memory_limiter, tail_sampling]
      exporters: [otlp/backend]
```

### 5.5 `tail_sampling` defaults

**Upstream defaults**, from
[`processor/tailsamplingprocessor/README.md`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md),
verified 2026-09-09:

| Key | Default | Upstream wording / note |
| --- | --- | --- |
| `decision_wait` | `30s` | *"Time before timer handling for a trace."* |
| `num_traces` | `50000` | *"Number of traces kept in memory."* |
| `expected_new_traces_per_sec` | `0` | *"Expected number of new traces (helps in allocating data structures)"*. **It is a pre-allocation hint and caps nothing.** Verified in code: it is used only as the initial capacity of the id batcher (`internal/idbatcher/id_batcher.go`), and a zero is replaced by 10. Setting it high does not admit more traces; setting it low does not reject any. |
| `sampling_strategy` | `trace-complete` | *"Controls decision timing and evaluation scope. `trace-complete` evaluates accumulated trace data on timer handling; `span-ingest` evaluates each incoming batch on ingest, finalizing terminal outcomes immediately and non-terminal traces on cleanup."* Only those two values. `span-ingest` rejects stateful policies, and re-scopes `decision_wait` to pending-cleanup finalization rather than decision timing. This option is recent — confirm it exists in your version before using it. |
| `decision_cache::sampled_cache_size` | `0` (**inactive**) | *"Configures amount of trace IDs to be kept in an LRU cache, persisting the 'keep' decisions for traces that may have already been released from memory. By default, the size is 0 and the cache is inactive."* |
| `decision_cache::non_sampled_cache_size` | `0` (**inactive**) | Same, for "drop" decisions. |

Other knobs that exist and are worth knowing about (**upstream**, not covered in depth here):
`num_shards` (default 1, max 256 — divides `num_traces`, `expected_new_traces_per_sec`, the
decision-cache sizes and per-second rate limits across shards),
`decision_wait_after_root_received` (default `0s`), `block_on_overflow`,
`maximum_trace_size_bytes`, `drop_pending_traces_on_shutdown`, and the feature-gated
`tail_storage` extension (gate `processor.tailsamplingprocessor.tailstorageextension`),
which is upstream's answer to memory pressure.

### 5.6 `decision_wait`: upstream gives a method, not a number

**There is no upstream numeric recommendation for `decision_wait`.** The README states the
`30s` default and shows `10s` in an example; neither is a recommendation. `opentelemetry.io`'s
scaling page gives no number either. Anyone quoting "use 5s" or "use 60s" is quoting a blog,
not the project.

What upstream *does* give is a tuning method:

> *"To track how long traces remain in the buffer use:
> `otelcol_processor_tail_sampling_sampling_trace_removal_age`" … "It may be useful to
> calculate latency percentiles like **p1** and compare that value to `decision_wait`.
> **Values close to `decision_wait` are at risk of being dropped if trace volume
> increases.**"*

And a second metric that can break the first one:

> *`otelcol_processor_tail_sampling_sampling_decision_timer_latency`* — *"This measures
> latency of sampling a batch of traces and passing sampled traces through the remainder of
> the collector pipeline. **A latency exceeding 1 second can delay sampling decisions beyond
> `decision_wait`**, increasing the chance of traces being dropped before sampling. It's
> therefore recommended to consume this component's output with components that are fast or
> trigger asynchronous processing."*

**The procedure, then:**

1. Deploy with the `30s` default.
2. Watch `..._sampling_trace_removal_age`. Compute a **low** percentile (p1) — the traces
   that left the buffer soonest. If p1 sits close to `decision_wait`, memory pressure is
   already evicting traces early and you are at risk.
3. Watch `..._sampling_decision_timer_latency`. If it exceeds 1 s, fix the *downstream*
   (exporter queue, backend latency) before touching `decision_wait`; a slow consumer
   silently lengthens the effective wait.
4. Only then adjust `decision_wait`, and re-check both metrics plus memory (§5.7).

### 5.7 Memory

**Upstream gives no memory formula.** The README's only statements are qualitative:
*"Both of those options increase memory usage"*, *"at the expense of increased memory usage"*.

The widely used approximation is **third-party**:

```
resident memory ≈ average_trace_size × num_traces
```

e.g. [OneUptime, *Tail-Based Sampling in OpenTelemetry*](https://oneuptime.com/blog/post/2026-01-25-tail-based-sampling-opentelemetry/view)
(*"Average trace size: ~10KB … num_traces: 100000 = Approximate memory: 100000 * 10KB = 1GB"*);
see also [Elastic Observability Labs](https://www.elastic.co/observability-labs/blog/tail-sampling-memory-opentelemetry).
Grafana's Alloy docs for the same component do **not** carry the formula.

Applied to the defaults, purely as **arithmetic**:

| `num_traces` | assumed avg trace size | implied buffer |
| --- | --- | --- |
| 50,000 (default) | 10 KB | ~500 MB |
| 50,000 | 50 KB | ~2.5 GB |
| 200,000 | 10 KB | ~2 GB |

**This is not a measurement of one-api traces.** One-api's own recorder bounds are per
*record*, not per exported trace (`TRACE_MAX_RECORD_BYTES` defaults to 262,144 standalone /
65,536 scaled), and the trace that reaches the collector is a set of OTLP spans, not that
record. Measure your own average with `otelcol_processor_tail_sampling_*` and the collector's
`otelcol_process_runtime_total_alloc_bytes` before sizing.

Two caveats the formula hides:
- The `decision_cache` LRUs are **additional** memory, and §5.8a tells you to make them much
  larger than `num_traces`. They hold trace IDs and decisions, not spans, so the per-entry
  cost is small — but it is not zero, and it is not in the formula.
- `num_shards > 1` divides `num_traces` and the cache sizes across shards; it does not
  reduce total memory.

### 5.8 Failure modes to plan for

**a. Late spans and split-trace RE-DECISIONS.** Once a trace's decision has been evicted
from memory, a span for that trace that arrives afterwards looks like a brand-new trace and
gets a **new, independent decision**. The result is a trace that is half-kept and half-dropped
in the backend — worse than either outcome alone, because it looks like data loss in a trace
that was "sampled".

The highest-value mitigation is the decision cache, and upstream says so directly:

> *"Additionally, if using, configure this as **much greater than `num_traces`** so decisions
> for trace IDs are kept longer than the span data for the trace."*

The cache holds *decisions* (trace ID → keep/drop) after the *spans* have been released, so a
late span is judged by the original decision instead of being re-decided.

Monitor `otelcol_processor_tail_sampling_sampling_late_span_age` — **upstream**: *"Time (in
seconds) from the sampling decision was taken and the arrival of a late span"*. A growing
tail on this histogram tells you how far past `decision_wait` your traces actually run, which
for a streaming gateway is exactly the question §6 asks.

**b. `..._sampling_trace_dropped_too_early` means `num_traces` is too small.** Upstream FAQ:

> *"**Q. Why am I seeing high values for the error metric `sampling_trace_dropped_too_early`?**
> **A.** This is likely a load issue. If the collector is processing more traces in-memory
> than the `num_traces` configuration option allows, some will have to be dropped before they
> can be sampled. Increasing the value of `num_traces` can help resolve this error, at the
> expense of increased memory usage."*

Any non-zero rate here means traces were discarded *before a policy ever looked at them* —
your sampling percentages are not what your config says they are.

**c. A collector restart loses in-flight decisions.** **This is an inference, not an upstream
statement.** Upstream never documents restart behavior; what it documents is
`drop_pending_traces_on_shutdown` (*"Drop pending traces on shutdown instead of making a
decision with the partial data already ingested"*) and the opt-in `tail_storage` extension
for spans. The inference is straightforward: the decision caches are process-local
`hashicorp/golang-lru` instances with no persistence
([`cache/lru_cache.go`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/cache/lru_cache.go)),
and the buffered spans are in the process heap — so a restart, an OOM kill, or a rolling
deployment discards both. Every trace in flight at that moment is subject to the re-decision
behavior of (a). **Our recommendation: treat layer-2 restarts as a data-quality event, drain
deliberately, and do not roll layer 2 during an incident you are trying to trace.**

**d. All spans of a trace must reach the same instance.** Upstream states this twice, as a
requirement and as a warning:

> *"All spans for a given trace **MUST** be received by the same collector instance for
> effective sampling decisions."*
> *"[Statefulness]: The processor keeps spans in memory while it waits to make a sampling
> decision. All spans for a given trace must be sent to the same Collector instance."*

This is the entire reason layer 1 exists. Putting `tail_sampling` behind an ordinary
round-robin service is a silent correctness bug: every trace gets partially evaluated by
several instances, and the policies see fragments.

**e. Tail sampling must come AFTER context-dependent processors.** Upstream:

> *"This processor must be placed in pipelines after any processors that rely on context,
> e.g. `k8sattributes`. **It reassembles spans into new batches, causing them to lose their
> original context.**"*

In the two-tier layout this is automatic *if* you put enrichment in layer 1, as §5.3 does.
It becomes a real bug the moment someone "simplifies" the deployment by collapsing both
pipelines into one collector and lists `[memory_limiter, tail_sampling, k8sattributes]`.

### 5.9 `load_balancing` exporter pitfalls

**a. The component was RENAMED.** **Upstream `CHANGELOG.md`, `v0.153.0`, 🚩 Deprecations:**
*"`exporter/load_balancing`: Rename the `loadbalancing` exporter to `load_balancing`. The old
`loadbalancing` type remains available as a **deprecated alias**. (#45339)"* The README adds:
*"users should migrate configuration to use `load_balancing:`"*. Write the new name; expect
to find the old one in every blog post and every existing config you inherit.

**b. `routing_key` defaults are signal-dependent.** **Upstream:** *"If no `routing_key` is
configured, the default routing mechanism is `traceID` for traces, `service` for logs, and
`service` for metrics."* Valid values: `service`, `traceID`, `metric`, `resource`,
`streamID`, `attributes`.

| Layer 2 runs… | Use `routing_key` | Why |
| --- | --- | --- |
| `tail_sampling` | **`traceID`** | Trace affinity is the requirement (§5.8d). |
| `spanmetrics` (or anything producing per-service Prometheus series) | **`service`** | **Upstream:** *"there is a high chance of facing label collisions on prometheus if the routing is based on `traceID` because every collector sees the `service+operation` label. With service name based routing, each collector can only see one service name and can push metrics without any label collisions."* |
| both | you have a conflict | **Our recommendation:** split them into two layer-2 deployments with different routing keys rather than compromising on one. |

For one-api specifically, `routing_key: service` would send *all* gateway traces to a single
backend instance (`service.name` is a single value, `OTEL_SERVICE_NAME`, default `one-api`) —
which destroys the point of the tier. Use `traceID`.

**c. Logs without a trace id are randomly distributed.** **Upstream:** *"`traceID`: Routes
spans based on their `traceID`. For logs, routes by the log record's traceID; **logs without
a trace ID are randomly distributed across backends**. Invalid for metrics."* One-api's log
bridge emits trace/span IDs when the record carries request context, and a background context
otherwise — so background and startup logs are the ones that scatter. Harmless for storage,
relevant if you ever route logs for stateful processing.

**d. Resolvers.** **Upstream:** *"The `resolver` accepts a `static` node, a `dns`, a `k8s`
service or `aws_cloud_map`. If all four are specified, an `errMultipleResolversProvided`
error will be thrown."* There is no Azure resolver.

The `k8s` resolver has an RBAC requirement that is easy to miss because its failure is quiet:

> *"**RBAC requirement:** the Collector pod must run with a service account that is allowed
> to `get`, `list`, and `watch` `discovery.k8s.io/v1` `EndpointSlice` objects in the target
> namespace; otherwise the resolver cache remains empty and the exporter logs
> `couldn't find the exporter for the endpoint ""`."*

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: otel-lb-endpointslices
  namespace: observability
rules:
  - apiGroups: ["discovery.k8s.io"]
    resources: ["endpointslices"]
    verbs: ["get", "list", "watch"]
```

**e. THE MOST-MISSED PITFALL: top-level `timeout` / `retry_on_failure` / `sending_queue` are
OFF by default.** **Upstream, verbatim:**

> *"Importantly, the `loadbalancingexporter`, by default, will **NOT** attempt to re-route
> data to a healthy endpoint on delivery failure, because in-memory queue, retry and timeout
> setting are disabled by default."*

and, on the two levels of resiliency settings:

> *"Resiliency options 1 (`timeout`, `retry_on_failure` and `sending_queue` settings in
> `load_balancing` section) — are useful for highly elastic environment (like k8s) … In case
> of permanent change of list of resolved exporters this options provide capability to
> re-route data into new set of healthy backends. **Disabled by default.**"*
> *"Resiliency options 2 (`timeout`, `retry_on_failure` and `sending_queue` settings in
> `otlp` section) … **Enabled by default.**"*

The distinction matters and is the part people get wrong: the **sub-exporter** (`protocol:
otlp:`) has its own queue, retry and timeout, and those *are* on. Data returns to the load
balancer for **re-routing** only after that sub-exporter's redelivery is exhausted **and**
the top-level options are enabled. So a layer-2 pod that dies mid-rollout will, with the
default config, exhaust the sub-exporter's retries against the dead endpoint and then drop —
never trying a healthy one. §5.3 enables them explicitly for that reason.

**f. Monitor the ring.** **Upstream metrics:**

| Metric | Upstream description |
| --- | --- |
| `otelcol_loadbalancer_num_backends` | *"informs how many backends are currently in use"* |
| `otelcol_loadbalancer_num_backend_updates` | *"records how many of the resolutions resulted in a new list of backends. Use this information to understand how frequent your backend updates are and how often the ring is rebalanced"* |
| `otelcol_loadbalancer_backend_latency` | *"measures the latency for each backend"* |
| `otelcol_loadbalancer_num_resolutions` | split by `success=true\|false` |
| `otelcol_loadbalancer_backend_outcome` | per-backend delivery outcome |

**Our recommendation:** alarm on `otelcol_loadbalancer_num_backends` dropping below your
layer-2 replica count, and on a sustained non-zero rate of `_num_backend_updates` — every
ring rebalance reshuffles trace-to-instance assignment, which manufactures exactly the
split-trace re-decisions of §5.8a.

---

## 6. Maximum trace duration: streaming relays and `decision_wait`

This is the interaction W3.1 asks to be documented, and it is the one that makes a
*gateway* different from an ordinary web service.

### 6.1 A relay trace is not complete when the request starts producing spans

A one-api relay request can stay open for minutes. A streaming completion holds the HTTP
response open for as long as the client reads it; the trace is not complete until the stream
ends. The gateway's own sampling code says so plainly (**repo fact**,
`common/tracing/sampling.go`):

> *"A streaming relay stays open for as long as the client reads, so its total lifetime says
> almost nothing about whether the request was slow."*

Now put that next to how `decision_wait` works. Upstream describes it only as *"Time before
timer handling for a trace"*. **Inference (not an upstream sentence, but the reading the
metrics require):** the timer starts when the processor first sees the trace — i.e. on
**first span arrival** — not when the trace is complete, because there is no way for the
processor to know a trace is complete. The existence of
`..._sampling_late_span_age` (*"Time … from the sampling decision was taken and the arrival
of a late span"*) only makes sense if decisions are routinely taken before the last span
arrives.

**Therefore: a trace whose real duration exceeds `decision_wait` has its decision made on
partial data, and every later span is a late span.** With `30s` default `decision_wait` and a
two-minute stream, the decision is made 90 seconds before the request finishes.

### 6.2 Why this bites one-api even with no other instrumented service

You might expect a gateway-only trace to be safe: one-api's SERVER span is `otelgin`'s, it
ends when the request ends, and a span is exported only after it ends — so for a purely
one-api trace, "first span arrival" and "last span arrival" would coincide.

They do not, because **one-api also attaches the GORM OpenTelemetry plugin whenever
`OTEL_ENABLED=true`** (**repo fact**: `enableGormOpenTelemetry` in `model/main.go`, using
`gorm.io/plugin/opentelemetry v0.1.16`). Database spans for token lookup, channel selection
and quota checks happen in the **first milliseconds** of a request, end immediately, and are
handed to the batch span processor, which flushes on a 5-second `BatchTimeout` (§1.3).

So for a two-minute stream the collector sees:

| t | Event |
| --- | --- |
| ~0 s | DB child spans end |
| ~5 s | BSP flushes them → **collector sees the trace for the first time; `decision_wait` timer starts** |
| ~35 s | `decision_wait` (30 s default) expires → **decision made on DB spans only** — no status, no duration, no error, no relay attributes |
| ~120 s | Stream ends; `otelgin` SERVER span ends |
| ~125 s | BSP flushes the SERVER span → **arrives late.** If the decision is still in the decision cache it is honoured; if not, this is a fresh trace and gets a **new, independent decision** (§5.8a) |

That is the split-trace re-decision case, produced by a single service, with no distributed
system involved. It is also the case where §5.8a's advice — decision caches *much greater
than* `num_traces` — earns its keep.

### 6.3 What to do about it

**Our recommendations**, in order of preference:

1. **Set `decision_cache::sampled_cache_size` and `non_sampled_cache_size` well above
   `num_traces`.** This is the highest-value mitigation and the one upstream names. It does
   not stop the decision being made early, but it stops the *second* decision, so the trace
   at least ends up consistently kept or consistently dropped.
2. **Size `decision_wait` from your own `..._sampling_late_span_age` histogram, not from your
   p99 request duration.** The late-span histogram tells you directly how far past the
   decision your spans actually arrive. Follow the §5.6 method; do not pick a number from a
   blog.
3. **Do not simply set `decision_wait` to your longest stream.** `decision_wait` and
   `num_traces` multiply: holding traces for 5 minutes instead of 30 seconds means roughly
   10x as many concurrent traces resident, and `num_traces` is a hard ceiling above which
   traces are `dropped_too_early` (§5.8b). A long `decision_wait` with an unchanged
   `num_traces` trades a late-span problem for a silent-eviction problem.
4. **Consider whether the decision needs the end of the stream at all.** If your policies are
   `status_code` and `latency`, they do. If they are attribute- or service-based, they may be
   satisfiable from the early spans — in which case the early decision is not a bug.
5. **Prefer the gateway's own completion-time selection where it suffices** (§5.1). One-api
   decides at request completion, in-process, with the full lifetime known, and needs no
   buffer at all.

### 6.4 The same problem, solved differently, inside the gateway

`TRACE_ALWAYS_SAMPLE_SLOW_MS` exists for exactly this reason, and its implementation is the
in-process answer to the question `decision_wait` asks (**repo fact**,
`common/tracing/sampling.go`):

> *"at `TRACE_ALWAYS_SAMPLE_SLOW_MS=5000` every stream longer than five seconds would be
> retained unconditionally, driving `q` — and therefore the effective fraction — towards 1.
> The slow rule therefore evaluates **time-to-first-token** when it is known, and falls back
> to the total lifetime only when the request never produced a first client byte."*

That distinction — TTFT as the latency signal, total lifetime only as a fallback — is the
thing a collector `latency` policy cannot do, because the collector sees a span duration and
has no idea that most of it was the client reading bytes it asked for. **Our recommendation:
if you configure a `latency` tail-sampling policy for one-api traffic, expect it to select
"every long stream", not "every slow request", unless you can key it off a TTFT attribute.**

### 6.5 The formula that makes all of this add up wrong

From [proposal §4, W1](../proposals/20260905_observability-data-tiering.md), restated in
`common/tracing/sampling.go` and measured by `TestEffectiveRetainedFractionMatchesFormula`
(`common/tracing/sampling_test.go`) — **repo fact**:

> With `p` the base rate and `q` the fraction selected by any unconditional rule (errors,
> semantic failures, slow requests, forced traces), effective local selection is
>
> ```
> q + (1 - q) * p
> ```
>
> before path exclusions and delivery failures. **At `p = 0.05` and `q = 0.5` that is 52.5%,
> not 5%.**

This matters to a collector operator for two reasons:

1. **Capacity.** If you size the collector for "5% of gateway traffic" because
   `TRACE_SAMPLE_RATE=0.05`, and unconditional rules select half the requests, you have
   under-provisioned by roughly 10x. And that is only the *local* fraction — see §7, where
   the OTLP span volume is 100% regardless.
2. **A `latency` tail policy behaves like an unconditional rule for streaming traffic.** Set
   `threshold_ms: 5000` against relay traffic where most streams last longer than five
   seconds and `q` approaches 1; the probabilistic policy beside it becomes decorative.
   §6.4's TTFT-versus-lifetime distinction is precisely the fix the collector does not have.

---

## 7. The SDK/collector sampling relationship

W1 already established this as a correctness point. It is restated here because it is the
single most common operator misunderstanding of one-api's telemetry, and because getting it
wrong produces a capacity error of one to two orders of magnitude.

### 7.1 What the `TRACE_*` settings actually govern

**Repo fact**, from `common/tracing/sink_otlp.go`, `common/tracing/sampling.go` and
[proposal §4, W1](../proposals/20260905_observability-data-tiering.md):

`TRACE_SAMPLE_RATE`, `TRACE_EXCLUDED_PATH_PREFIXES` and `TRACE_SINK=none` govern
**local recorder / detail / SQL selection only**. Concretely, they decide:

- whether the in-process recorder retains external calls, events and timings for a request;
- whether a row is written to the `traces` table;
- whether the OTLP sink *enriches* the request span with that detail.

### 7.2 What they do not govern

They do **not** govern whether a span is exported over OTLP. The source says so at the point
of the code (`common/tracing/sink_otlp.go`):

> *"Note also that enriching the otelgin span does NOT suppress its independent SDK export:
> `TRACE_SAMPLE_RATE` governs local enrichment and SQL rows only. Reducing OTLP span volume
> requires the separately configured SDK/collector sampling policy."*

`otelgin.Middleware` is installed in `main.go` for every request whenever `OTEL_ENABLED=true`
(**repo fact**), and it starts and ends its own SERVER span regardless of every `TRACE_*`
setting. The GORM plugin does the same for database spans (§6.2). Both go straight to the
SDK's batch processor and out over OTLP.

> **Therefore: "we run at 5% sampling" must NEVER be stated as "the collector receives 5% of
> spans."** With `TRACE_SAMPLE_RATE=0.05` and no SDK or collector sampling policy, the
> collector receives **100%** of `otelgin` and GORM spans. The 5% describes how many of them
> got *enriched*, and how many SQL rows were written.

Path exclusions are the one partial exception, and only partially: `TRACE_EXCLUDED_PATH_PREFIXES`
suppresses one-api's own recorder (counted as `oneapi_trace_records_total{outcome="excluded"}`,
`common/tracing/outcome.go`) but does not remove `otelgin`'s span for that path. To keep
`/metrics` and `/api/status` scrapes out of your tracing backend you need a collector
`filter` processor or an SDK sampler, not the gateway's exclusion list.

### 7.3 How to actually reduce OTLP span volume

There are exactly two places, and they are configured and tested separately from anything in
`TRACE_*`.

**(a) The SDK head sampler — available today with no code change.**

**Repo fact:** one-api never calls `sdktrace.WithSampler` (verified by grep across the
repository). **Upstream fact:** `go.opentelemetry.io/otel/sdk@v1.46.0` reads the sampler from
the environment in `tracerProviderOptionsFromEnv` → `samplerFromEnv`
(`sdk/trace/sampler_env.go`), and falls back to `ParentBased(AlwaysSample())` when nothing is
set (`sdk/trace/provider.go`). Because no code-level sampler overrides it, **the standard
environment variables work**:

| Variable | Accepted values (v1.46.0) |
| --- | --- |
| `OTEL_TRACES_SAMPLER` | `always_on`, `always_off`, `traceidratio`, `parentbased_always_on`, `parentbased_always_off`, `parentbased_traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | float in `[0,1]` for the ratio samplers; out-of-range or unparseable values fall back to ratio `1.0` and are reported through the OTel error handler |

```bash
# export ~5% of traces from the SDK, honouring an incoming sampling decision
export OTEL_TRACES_SAMPLER=parentbased_traceidratio
export OTEL_TRACES_SAMPLER_ARG=0.05
```

> **uncertain:** this is read from the pinned SDK version in `go.mod` (v1.46.0) and is not
> covered by any one-api test in the repository. Verify on your build with a low-volume
> canary before relying on it, and re-verify after an SDK bump.

**(b) A collector policy** — `probabilistic_sampler` in the single collector of §2.2, or
`tail_sampling` in §5. This is the only option if you need the decision to depend on data the
gateway does not have at span start.

### 7.4 Head sampling and local error selection do not compose

From [proposal §4, W1](../proposals/20260905_observability-data-tiering.md):

> *"SDK head sampling can make a parent nonrecording, and local error selection cannot recover
> its missing spans."*

The gateway's own code documents the mechanism (`common/tracing/sink_otlp.go`, **repo fact**):
when SDK head sampling has made the enclosing span nonrecording, the OTLP sink's fallback span
*"inherits the nonrecording parent decision and the sink reports `span_record_failed`."*

**The practical consequence:** if you set `OTEL_TRACES_SAMPLER_ARG=0.05` and also rely on
`TRACE_ALWAYS_SAMPLE_ERRORS=true` to guarantee that every failed request is traceable, those
two policies fight. The error is known at request *completion*; the head sampler decided at
request *start*, and 95% of the time it decided not to record. The gateway cannot resurrect
those spans; it can only count the failure.

| Goal | Do this | Not this |
| --- | --- | --- |
| Full-fidelity error traces in the backend | Export everything from the SDK, sample in the collector (tail sampling, §5), or accept SQL-side traces (`TRACE_SINK=db`) | SDK head sampling plus `TRACE_ALWAYS_SAMPLE_ERRORS` |
| Cheap, uniform span volume reduction | `OTEL_TRACES_SAMPLER=parentbased_traceidratio` (§7.3a) | `TRACE_SAMPLE_RATE` — it does not touch OTLP volume |
| Keep scrape endpoints out of the backend | collector `filter` processor, or an SDK sampler | `TRACE_EXCLUDED_PATH_PREFIXES` alone (§7.2) |

Watch for a rising `oneapi_trace_records_total{outcome="span_record_failed"}` after enabling
an SDK sampler: that series is the gateway telling you the two policies are colliding.

---

## 8. Collector failure behavior, from the gateway's side

### 8.1 A valid config plus a dead collector is a runtime transport failure

[Proposal §3.2](../proposals/20260905_observability-data-tiering.md) sets the contract:

> *"`OTEL_ENABLED=true` requires a configured provider even when local tracing is disabled;
> existing OTel instrumentation is separate from `TRACE_SINK=none`. **Syntactically valid
> configuration followed by a collector outage is a runtime transport failure: report it,
> apply bounded queues, and do not silently restore SQL tracing.** Do not require a reachable
> collector for every restart if the SDK can initialize offline with the configured
> exporter."*

All three halves of that are implemented (**repo facts**):

| Requirement | Where | Behavior |
| --- | --- | --- |
| Startup does not require a reachable collector | `common/telemetry/telemetry.go` (`InitOpenTelemetry`) | `otlptracehttp.New` constructs an exporter without dialing. A collector that is down at boot does not stop one-api. |
| Configuration is validated, not guessed | `common/config/validation.go`, `common/config/observability_matrix.go` | `TRACE_SINK` including `otlp` with `OTEL_ENABLED=false` is **rejected at startup**, as is `otlp` with `TRACE_WRITE_MODE=sync`, an empty `OTEL_EXPORTER_OTLP_ENDPOINT`, and ambiguous sink lists such as `db,none`. |
| **No silent SQL fallback** | `common/tracing/sink.go` | The sink list is built once from `TRACE_SINK`. With `TRACE_SINK=otlp` there is no SQL sink in the chain at all, so a collector outage cannot cause SQL writes to resume. If you want both, ask for both: `TRACE_SINK=db,otlp`. |
| Bounded queues | `common/config/observability_app_log_otlp.go`, `common/telemetry/log_pipeline.go` | The app-log bridge has record and byte ceilings and drops rather than blocking a request goroutine. |

### 8.2 The metric vocabulary, and what each name deliberately does not claim

All names below were read out of the repository on 2026-09-09 (**repo facts**). The
Prometheus definitions live in `monitor/prometheus/recorder_trace_pipeline.go` and
`monitor/prometheus/recorder_log_export.go`; the label vocabularies are compile-time
constants in `common/metrics/trace_pipeline.go` and `common/metrics/log_export.go`; the same
series are mirrored to OTLP by `monitor/otel/recorder_trace_pipeline.go` and
`monitor/otel/recorder_log_export.go`.

#### Trace pipeline: `oneapi_trace_records_total{outcome}`

| `outcome` | Meaning |
| --- | --- |
| `excluded` | Never recorded: a configured path prefix matched, or `TRACE_SINK=none`. A deliberate saving, deliberately not conflated with a drop. |
| `sampled_out` | The local sampler declined the trace. |
| `dropped_active_limit` | `TRACE_MAX_ACTIVE_RECORDERS` was already reached; the request ran untraced and was otherwise unaffected. |
| `truncated` | Retained content was cut to fit `TRACE_MAX_RECORD_BYTES` / `TRACE_MAX_EXTERNAL_CALLS`. The trace is still persisted; only its detail is reduced. |
| `queued` | Accepted into the writer queue. |
| `dropped_queue_full` | The writer queue was saturated — the database cannot keep pace. |
| `dropped_closed` | The sink was already shut down. |
| `written` | A trace row was durably written by a sink. |
| `write_failed` | A sink failed to write the row. |
| **`span_recorded`** | The local SDK span was recording and was enriched (or started) by the OTLP sink. |
| **`span_record_failed`** | The sink could not record at all — non-recording provider, rejected span, or invalid trace data. |

> **The naming is the point.** `span_recorded` and `span_record_failed` deliberately name
> **local SDK state**. From `common/metrics/trace_pipeline.go`: *"It is not an export-delivery
> metric: batch processor queue overflow and collector transport failures happen later and
> must be measured at the exporter boundary."*
>
> **Dashboard trap:** the Go constants `TraceOutcomeExported` and `TraceOutcomeExportFailed`
> still exist as source-compatible aliases, but their **values** are `span_recorded` and
> `span_record_failed`. A PromQL query written against `outcome="exported"` matches **nothing**
> and renders as a flat zero — which is indistinguishable from "no traces". Query
> `outcome="span_recorded"`.

Accompanying gauges: `oneapi_trace_queue_depth`, `oneapi_trace_queue_capacity`,
`oneapi_trace_active_recorders`, `oneapi_trace_active_recorders_limit` (the last is `0` when
admission is unlimited).

#### Application logs: `oneapi_app_log_export_records_total{outcome}`

| `outcome` | Meaning |
| --- | --- |
| `emitted` | Admitted into the export pipeline. *"the denominator every other outcome is measured against."* |
| `dropped_queue_full` | The bridge's bounded queue was at its record or byte limit. A sustained non-zero rate means the collector cannot keep pace with the gateway's log volume. |
| `dropped_not_ready` | No real `LoggerProvider` was installed yet — startup logs before telemetry initialization. They still reach the file and stdout sinks. |
| `dropped_shutdown` | The logger provider was already shut down. By design: the exporter closes before the database, so the last shutdown lines are file/stdout only. |
| `exported` | *"records the SDK handed to the OTLP exporter without a transport error. It is **NOT** proof of collector persistence."* |
| `export_failed` | The exporter rejected or could not deliver the batch. **The records are lost; the SDK does not re-queue them.** |

Accompanying gauges: `oneapi_app_log_export_queue_records`,
`oneapi_app_log_export_queue_record_limit`, `oneapi_app_log_export_queue_bytes`,
`oneapi_app_log_export_queue_byte_limit`. Publishing the bounds as their own series keeps
the saturation ratio computable without hard-coding `LOG_OTLP_QUEUE_SIZE` into a dashboard.

Note the asymmetry between the two vocabularies, and that it is deliberate: **the log bridge
has an `export_failed`; the trace pipeline does not.** The log path wraps the exporter
(`countingExporter`, `common/telemetry/log_pipeline.go`) so it can observe the transport
result. The trace path hands spans to the SDK's batch processor and loses sight of them,
which is exactly why its outcome is named `span_recorded` and not `exported`.

### 8.3 What a collector outage looks like from each side

| Time | Traces | Application logs |
| --- | --- | --- |
| Outage begins | `oneapi_trace_records_total{outcome="span_recorded"}` keeps climbing — the SDK is still recording spans | `..._app_log_export_records_total{outcome="emitted"}` keeps climbing |
| Seconds later | BSP retries; nothing visible in one-api metrics | `outcome="export_failed"` starts climbing — **visible** |
| BSP/queue fills | **Silent span loss** (§1.3). No metric. Errors appear on **stderr** via the OTel default handler | `outcome="dropped_queue_full"` climbs and `oneapi_app_log_export_queue_records` pins at `..._record_limit` — **visible** |
| Throughout | No SQL fallback. With `TRACE_SINK=otlp` the `traces` table stays empty (§8.1) | File and stdout sinks are unaffected; the local log of record survives |

Two details worth internalizing:

1. **OTel SDK errors do not go to one-api's logger.** The repository never calls
   `otel.SetErrorHandler` (**repo fact**, verified by grep), so exporter transport errors
   reach the SDK default handler, which writes through `stdr` to **stderr**
   (`go.opentelemetry.io/otel/internal/global/internal_logging.go`). They will not appear in
   the rotating file log with your other errors, they are not JSON, and they are not sampled
   by `LOG_SAMPLE_*`. **Our recommendation: make sure your container's stderr is collected,
   or you will debug a collector outage blind.**
2. **`span_recorded` continuing to climb during an outage is correct, not a bug.** It is the
   metric refusing to claim something it cannot observe. The collector's own
   `otelcol_receiver_accepted_spans` (or its absence) is the other half of the picture, which
   is why §2.2 exposes the collector's telemetry endpoint.

### 8.4 What to alarm on, gateway side

| Condition | Meaning |
| --- | --- |
| `rate(oneapi_trace_records_total{outcome="span_record_failed"}[5m]) > 0` | Provider not installed, span rejected, or an SDK head sampler made the parent non-recording (§7.4). |
| `rate(oneapi_app_log_export_records_total{outcome="export_failed"}[5m]) > 0` | Collector unreachable or rejecting. These records are gone. |
| `oneapi_app_log_export_queue_records / oneapi_app_log_export_queue_record_limit > 0.7` | The bridge is about to start dropping. |
| `oneapi_app_log_export_queue_bytes / oneapi_app_log_export_queue_byte_limit > 0.7` | Same, but driven by a few very large records rather than many small ones. |
| `rate(oneapi_app_log_export_records_total{outcome="dropped_queue_full"}[5m]) > 0` | Already dropping. |
| `rate(oneapi_trace_records_total{outcome="dropped_queue_full"}[5m]) > 0` | The **SQL** writer queue, not the collector — relevant only with `TRACE_SINK=db`. |
| Collector `otelcol_receiver_accepted_spans` flat while `oneapi_trace_records_total{outcome="span_recorded"}` climbs | The classic outage signature: the gateway thinks it is recording, the collector is receiving nothing. |

---

## 9. Alert and dashboard summary

One table, for copying into an alerting config. Thresholds marked **our recommendation** are
starting points, not certified values.

| # | Source | Expression (sketch) | Severity | §  |
| --- | --- | --- | --- | --- |
| 1 | collector | `rate(otelcol_processor_memory_limiter_refused_spans[5m]) > 0` | page if sustained 5 min | [3.5](#35-what-to-alarm-on) |
| 2 | collector | `rate(otelcol_processor_memory_limiter_refused_log_records[5m]) > 0` | page if sustained 5 min | [3.5](#35-what-to-alarm-on) |
| 3 | collector | `otelcol_exporter_queue_size / otelcol_exporter_queue_capacity > 0.7` | warn — scale before 1.0 | [4.6](#46-what-to-alarm-on-and-when-to-scale-up) |
| 4 | collector | `rate(otelcol_exporter_enqueue_failed_log_records[5m]) > 0` | page — not covered by retry | [4.4](#44-retry_on_failure-does-not-cover-queue-full) |
| 5 | collector | `rate(otelcol_exporter_enqueue_failed_spans[5m]) > 0` | page | [4.4](#44-retry_on_failure-does-not-cover-queue-full) |
| 6 | collector | `rate(otelcol_exporter_send_failed_log_records[5m]) > 0` | warn | [4.6](#46-what-to-alarm-on-and-when-to-scale-up) |
| 7 | layer 2 | `rate(otelcol_processor_tail_sampling_sampling_trace_dropped_too_early[5m]) > 0` | page — `num_traces` too small | [5.8b](#58-failure-modes-to-plan-for) |
| 8 | layer 2 | p1 of `..._sampling_trace_removal_age` approaching `decision_wait` | warn | [5.6](#56-decision_wait-upstream-gives-a-method-not-a-number) |
| 9 | layer 2 | `..._sampling_decision_timer_latency > 1s` | warn — fix downstream first | [5.6](#56-decision_wait-upstream-gives-a-method-not-a-number) |
| 10 | layer 2 | growing tail on `..._sampling_late_span_age` | informational — this is your real trace duration | [6.3](#63-what-to-do-about-it) |
| 11 | layer 1 | `otelcol_loadbalancer_num_backends < <replica count>` | page | [5.9f](#59-load_balancing-exporter-pitfalls) |
| 12 | layer 1 | sustained `rate(otelcol_loadbalancer_num_backend_updates[5m]) > 0` | warn — ring churn causes re-decisions | [5.9f](#59-load_balancing-exporter-pitfalls) |
| 13 | one-api | `rate(oneapi_trace_records_total{outcome="span_record_failed"}[5m]) > 0` | warn | [8.4](#84-what-to-alarm-on-gateway-side) |
| 14 | one-api | `rate(oneapi_app_log_export_records_total{outcome="export_failed"}[5m]) > 0` | page | [8.4](#84-what-to-alarm-on-gateway-side) |
| 15 | one-api | `oneapi_app_log_export_queue_records / oneapi_app_log_export_queue_record_limit > 0.7` | warn | [8.4](#84-what-to-alarm-on-gateway-side) |
| 16 | correlation | collector `otelcol_receiver_accepted_spans` flat while `oneapi_trace_records_total{outcome="span_recorded"}` climbs | page | [8.3](#83-what-a-collector-outage-looks-like-from-each-side) |

Reminder from §8.2: query `outcome="span_recorded"`, never `outcome="exported"` — the latter
matches nothing on the trace metric.

---

## 10. What this document does NOT establish

Matching the honesty conventions of
[§11 of the W0/W1 acceptance record](../benchmarks/20260908_w0-w1-acceptance.md) and
[§8.1 of the proposal](../proposals/20260905_observability-data-tiering.md).

1. **No capacity claim follows from any of this.** **G5 remains open and is not advanced by
   this document.** G5 requires an open-arrival-rate full-relay harness with a controlled
   upstream simulator, offered/achieved/failed RPS reported separately, seeded retained data
   at production scale, and at least one hour at target after warmup, with the collector
   included in the recorded topology (proposal §9.2). None of that was run here. Nothing in
   this file was run here.
2. **Nothing in this document is a measurement of this gateway.** Every collector number is
   an **upstream default** read from a README, `documentation.md` or `CHANGELOG.md` on
   2026-09-09 at release v1.66.0/v0.160.0. Every one-api number is a **configured default**
   read from the source. No collector was deployed, no span was exported, no queue was
   filled, and no memory was measured while writing this.
3. **The tail-sampling memory formula is third-party, and applying it here is arithmetic.**
   `avg_trace_size × num_traces` appears in vendor blogs, not in the upstream README, which
   offers only qualitative statements (§5.7). The table of implied buffer sizes is
   multiplication, not observation, and the "average trace size" input is assumed rather than
   measured for one-api traffic.
4. **The `decision_wait` guidance is a method, not a number.** Upstream publishes no
   recommended value (§5.6). Any specific value in this document is either the upstream
   default (`30s`) or an illustration in a YAML block. Choosing one for your deployment
   requires running the §5.6 procedure against your own metrics.
5. **The streaming timeline in §6.2 is a worked example, not a trace capture.** The
   components in it are verified — the GORM plugin is attached, the BSP flushes on a 5-second
   timer, `decision_wait` defaults to 30s — but no one-api trace was captured to confirm the
   ordering end to end. Treat it as a model to test, not as evidence.
6. **The `OTEL_TRACES_SAMPLER` path is unexercised by any test in this repository.** It is
   read from the pinned SDK source (§7.3a) and follows from one-api never calling
   `WithSampler`. It is not covered by a one-api test, so verify it on a canary and re-verify
   after an SDK upgrade.
7. **"Collector received it" is still not "collector persisted it."** `otelcol_exporter_*`
   metrics describe the collector's own attempt to deliver onward. Whether Tempo, Loki or any
   other backend durably stored the data is outside every metric named in this document, on
   both sides.
8. **The inference in §5.8c about restart behavior is labelled as an inference.** Upstream
   does not document what happens to tail-sampling state across a restart; the conclusion is
   drawn from the caches being process-local LRUs with no persistence.
9. **The `service::telemetry::metrics::readers` form in §2.2 was not version-verified.** It is
   flagged **uncertain** in place. If your collector rejects it, consult your version's
   `service` documentation rather than assuming this document is right.
10. **No claim is made that the two-tier topology is needed by any current deployment.**
    W3.1 calls it optional and conditional on a measured requirement. §5.1 restates the
    conditions. This document describes how to build it correctly if you have that
    requirement; it does not assert that anyone does.

---

## 11. Sources

### Upstream — OpenTelemetry Collector core (verified at v1.66.0/v0.160.0, 2026-09-02)

- [`processor/memorylimiterprocessor/README.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/processor/memorylimiterprocessor/README.md) — `check_interval` default and recommendation, `limit_mib`/`limit_percentage`, `spike_limit_*`, first-processor best practice, `GOMEMLIMIT` at 80%
- [`processor/memorylimiterprocessor/documentation.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/processor/memorylimiterprocessor/documentation.md) and its [v0.154.0 predecessor](https://github.com/open-telemetry/opentelemetry-collector/blob/v0.154.0/processor/memorylimiterprocessor/documentation.md) — the metric rename table in §3.4
- [`exporter/exporterhelper/README.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/README.md) — `sending_queue` defaults, sizers, `batch` sub-block, the `timeout` sibling, failure behavior
- [`exporter/exporterhelper/documentation.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/documentation.md) — `otelcol_exporter_enqueue_failed_*`, `otelcol_exporter_queue_size`, `otelcol_exporter_queue_capacity`
- [`CHANGELOG.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/CHANGELOG.md) — v1.35.0/v0.129.0 removal of `sending_queue::blocking`; v1.61.0/v0.155.0 memory-limiter metric rename; v1.62.0/v0.156.0 forced-GC backoff; v1.64.0/v0.158.0 `queuebatchprocessor`
- [Issue #4981](https://github.com/open-telemetry/opentelemetry-collector/issues/4981) — "Degenerate collector performance when exporter has problems"
- [`docs/rfcs/batching-migration.md`](https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/rfcs/batching-migration.md) — merged RFC stating the intent to remove `batchprocessor`
- [Collector scaling guide](https://opentelemetry.io/docs/collector/scaling/)

### Upstream — OpenTelemetry Collector contrib

- [`processor/tailsamplingprocessor/README.md`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md) — every tail-sampling default, the `decision_wait` tuning method, the decision cache wording, the same-instance warning, processor ordering, and the two-layer recommendation
- [`processor/tailsamplingprocessor/documentation.md`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/documentation.md) — `..._sampling_late_span_age`, `..._sampling_trace_dropped_too_early`, `..._sampling_trace_removal_age`, `..._sampling_decision_timer_latency`
- [`processor/tailsamplingprocessor/cache/lru_cache.go`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/cache/lru_cache.go) — the decision caches are `hashicorp/golang-lru`, in memory, unpersisted (basis for the §5.8c inference)
- [`processor/tailsamplingprocessor/internal/idbatcher/id_batcher.go`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/internal/idbatcher/id_batcher.go) — proof that `expected_new_traces_per_sec` is a pre-allocation hint only
- [`exporter/loadbalancingexporter/README.md`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/loadbalancingexporter/README.md) — routing keys, resolvers, the EndpointSlice RBAC requirement, the two resiliency levels, the metric list
- [contrib `CHANGELOG.md`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/CHANGELOG.md) — v0.153.0 `loadbalancing` → `load_balancing` rename (#45339)

### Third-party (labelled as such where used)

- [OneUptime — *Tail-Based Sampling in OpenTelemetry*](https://oneuptime.com/blog/post/2026-01-25-tail-based-sampling-opentelemetry/view) — the `avg_trace_size × num_traces` memory formula
- [Elastic Observability Labs — tail sampling memory](https://www.elastic.co/observability-labs/blog/tail-sampling-memory-opentelemetry)
- [Grafana Alloy — `otelcol.processor.tail_sampling`](https://grafana.com/docs/alloy/latest/reference/components/otelcol/otelcol.processor.tail_sampling/) — checked, and notable for **not** carrying the memory formula

### This repository (all paths verified 2026-09-09)

| Claim area | File |
| --- | --- |
| OTLP transport, exporters, provider lifecycle | `common/telemetry/telemetry.go`, `common/telemetry/logs.go` |
| App-log bridge and its bounded queue | `common/telemetry/log_pipeline.go`, `common/logger/otelbridge/` |
| Trace sink selection; no SQL fallback | `common/tracing/sink.go` |
| Span enrichment, `span_recorded` semantics, head-sampling interaction | `common/tracing/sink_otlp.go` |
| Sampling formula, TTFT-versus-lifetime rule | `common/tracing/sampling.go`, `common/tracing/sampling_test.go` |
| Exclusion outcome | `common/tracing/outcome.go`; admission: `common/tracing/admission.go` |
| Metric label vocabularies | `common/metrics/trace_pipeline.go`, `common/metrics/log_export.go` |
| Prometheus series | `monitor/prometheus/recorder_trace_pipeline.go`, `monitor/prometheus/recorder_log_export.go` |
| OTLP mirror of the same series | `monitor/otel/recorder_trace_pipeline.go`, `monitor/otel/recorder_log_export.go` |
| Env vars, defaults, endpoint normalization, config matrix | `common/config/config.go`, `common/config/observability.go`, `common/config/observability_env.go`, `common/config/observability_app_log_otlp.go`, `common/config/observability_matrix.go`, `common/config/validation.go` |
| `otelgin` middleware order | `main.go`; measured by `middleware/tracing_otlp_span_test.go` |
| GORM OpenTelemetry plugin | `model/main.go` (`enableGormOpenTelemetry`) |
| Pinned SDK versions | `go.mod` (`go.opentelemetry.io/otel` v1.46.0, `otel/log` and `otel/sdk/log` v0.22.0, `otelgin` v0.71.0) |
