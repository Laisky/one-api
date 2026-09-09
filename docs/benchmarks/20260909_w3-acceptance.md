# W3 acceptance evidence: OTLP application logs, operational metrics, collector topology

- Date: 2026-09-09
- Proposal: [Observability Data Tiering](../proposals/20260905_observability-data-tiering.md), §6 (W3.1, W3.2, W3.3)
- Gate: **G3**, "Compile/integration with forked Zap, real trace/span correlation,
  pinned SDK/exporters, outage/resource/shutdown tests."
  This bundle closes G3. It closes **no part of G5** and advances no capacity claim.
- Baseline tree (**before**): commit `32df60cf` — carries the W0/W1 remediation and the
  W2.4 cursor work, and no Phase 3 code.
- Measured tree (**after**): the working tree described by this document.
- Machine: Ryzen 7 5700G, 16 logical CPUs, 27 GB RAM, Linux 6.8.0-139-generic.
- Toolchain: `go1.27.1 linux/amd64`.

Companion records: [Phase 0/1](20260905_observability-phase0-phase1.md),
[W2.4 cursor plans](20260906_w24-cursor-plans.md),
[W0/W1 acceptance](20260908_w0-w1-acceptance.md). Operations guidance produced by W3.1
lives in [the collector topology runbook](../runbooks/20260909_otlp-collector-topology.md).

This record follows the house conventions: every number carries the command that produced
it, every bound is stated with the thing it bounds, and anything that is arithmetic or an
estimate is labelled as such rather than presented as a measurement.

---

## 1. Claims and verdicts

| # | Claim under test | Result | Verdict |
| --- | --- | --- | --- |
| 1 | The upstream `otelzap` bridge cannot be used, so an adapter is required | `otelzap` needs `go.uber.org/zap`; the fork's `zapcore.Core` additionally requires `Fields()` | **Confirmed** — the adapter is mandatory, not a preference |
| 2 | A hand-written adapter compiles and runs against the real module graph | `var _ zapcore.Core = (*Core)(nil)` plus 11 behavioral tests | **Supported** |
| 3 | A `gmw.GetLogger(c)` call in the request path exports with the request's trace/span ids | asserted on the record's **protocol fields**, through the real middleware order | **Supported** |
| 4 | Enabling the bridge does not change one byte of the local log line | encoder output compared with and without the correlation field | **Supported** |
| 5 | A saturated queue drops and **counts** every refused record | 10 emitted, 3 admitted, **7** counted `dropped_queue_full` | **Supported** |
| 6 | The byte ceiling is independent of the record ceiling | 10 large records, record ceiling 1000, byte ceiling 4 KiB: refused on bytes | **Supported** |
| 7 | A collector outage costs drops, never request latency | 200 emissions against a blocked exporter complete without blocking | **Supported** |
| 8 | Export failure is distinguishable from healthy export | separate `export_failed` / `exported` tallies | **Supported** |
| 9 | A shutdown that leaves work behind reports it | 5 undrained records counted `dropped_shutdown` | **Supported** |
| 10 | The bridge sits above the disk budget and below sampling | proven by consequence, and by mutation in both directions | **Supported** |
| 11 | The disabled path is free | no core installed; `+0 B/op` versus baseline | **Supported** |
| 12 | The enabled path costs about 2.6 µs and 900 B per exported line | 610 ns → 3,200 ns/op, 0 B → 903 B/op | **Supported, and it is not small** — see §6.3 |
| 13 | W3.3 records every operational quantity §W3.3 lists | 2 of the 8 named quantities have **no source in this tree** | **Not supported as written** — see §5.2 |
| 14 | The SDK bump preserves the W0/W1 exemplar-reservoir optimization | 368,764 → 440 B/op (838x) under SDK v1.46.0 | **Supported** — but its proof test was measuring the wrong thing (§6.1) |

§6 records five findings that measuring produced, rather than quietly repairing them: a
proof test whose measurement method stopped being valid once the SDK got faster (§6.1), a
pre-existing flaky test that a full-suite run surfaced (§6.2), the bridge's real per-line
cost (§6.3), a severity floor that could widen the whole process's logging (§6.4), and a
shared test helper that did not deliver what its name promised (§6.5). Two of them --
§6.1 and §6.4 -- were found only because the claim was measured instead of asserted.

---

## 2. Environment, and how to reproduce

### 2.1 Pinned modules

The G3 requirement is "pinned SDK/exporters". These are the versions this record measured:

| Module | Version | Note |
| --- | --- | --- |
| `go.opentelemetry.io/otel` | **v1.46.0** | bumped from v1.44.0 by this work |
| `go.opentelemetry.io/otel/sdk` | v1.46.0 | |
| `go.opentelemetry.io/otel/trace`, `/metric` | v1.46.0 | |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | v1.46.0 | bumped from v1.44.0 |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` | v1.46.0 | bumped from v1.44.0 |
| `go.opentelemetry.io/otel/log` | **v0.22.0** | new |
| `go.opentelemetry.io/otel/sdk/log` | **v0.22.0** | new |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp` | **v0.22.0** | new |
| `go.opentelemetry.io/contrib/.../otelgin` | v0.71.0 | bumped from v0.69.0 to match core |
| `github.com/Laisky/zap` | v1.27.1-0.20260318034917-6e5a9fb2b3d1 | unchanged |

**The core bump is a prerequisite, not tidying.** The log module line that pairs with otel
core v1.44.0 is v0.20.0, and its `BatchProcessor` busy-spins under exporter backpressure
and cannot report errors encountered while draining during `ForceFlush`/`Shutdown`; both
were fixed in v0.21.0. Shipping application logs on the v0.20.0 processor would mean a
collector outage burning gateway CPU. The v0.22.0 line requires core v1.46.0, and
`otelgin` v0.71.0 is the contrib release that pairs with it.

**The pipeline is not stable, and must not be described as such.** The Logs API and SDK
reached release-candidate status on 2026-08-31, and the exporters were explicitly excluded
from that RC's stability scope; `otlploghttp` is expected to stay on a `v0.x` line. See
[the OTel Go Logs API/SDK RC announcement](https://opentelemetry.io/blog/2026/go-logs-api-sdk-rc/).
A future move to the `v1.47.0-rc.1` log API is a separate, breaking migration: v0.21.0
replaced `log.Value`/`log.KeyValue` with `attribute.Value`/`attribute.KeyValue`, and the
root `otel` package gained `SetLoggerProvider` while `otel/log/global` was deprecated.
This tree targets v0.22.0 and therefore still installs through `otel/log/global`.

### 2.2 Commands

```
go build ./...
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./common/... ./middleware/... ./monitor/... ./model/...
go test -run XXX -bench BenchmarkBridgeWrite -benchmem -count=5 ./common/logger/otelbridge/
go test -run XXX -bench BenchmarkCollect     -benchmem -count=6 ./common/telemetry/
```

Results on the measured tree:

| Command | Result |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test -count=1 ./...` | exit 0, **104 packages ok, 0 FAIL** |
| `go test -race ...` (common, middleware, monitor, model) | exit 0, 0 failures, **0 data races**; `model` 59.8 s |

---

## 3. W3.1 — trace pipeline and collector topology

W3.1 says: "Close W1's existing-span and provider validation gates; do not duplicate the
implemented OTLP sink as new work."

**Both gates were already closed by the W0/W1 remediation, and nothing here re-opens them.**
The existing-span gate is `TestOtelginSpanIsLiveWhenTraceSinkSubmits`, which drives the
real middleware order and asserts one SERVER span per request; the provider gate is
`validateSinkRuntimeRequirements` in `common/tracing/sink.go`, covered by
`TestInitSinksRejectsOTLPWithoutInitializedProvider` and its fan-out variant. No second
OTLP trace sink was created. The Phase 3 work adds a *logger* provider beside the existing
tracer and meter providers; the trace path is untouched.

The remainder of W3.1 — "Document memory, maximum trace duration, `decision_wait`,
incomplete/late spans and collector failure behavior" — is a documentation deliverable,
now at [docs/runbooks/20260909_otlp-collector-topology.md](../runbooks/20260909_otlp-collector-topology.md).

One finding from writing it is worth surfacing here, because it changes who is exposed to
the late-span problem. one-api attaches the GORM OpenTelemetry plugin whenever
`OTEL_ENABLED=true`, so a streaming relay's **database child spans end in the first
milliseconds** and flush on the batch span processor's timer, while the enclosing
`otelgin` SERVER span ends minutes later. A collector's `decision_wait` timer therefore
starts near the beginning of the request and can expire long before the request finishes.
**The late-span re-decision case is reachable with a single service** — it does not require
a distributed deployment, which is how it would usually be encountered. The runbook covers
the mitigations.

---

## 4. W3.2 — the application-log bridge

### 4.1 The module reality, checked first

W3.2 opens with an instruction that is really a precondition: "Compile a minimal adapter
against the actual module graph before designing configuration around it." That was done
before any configuration was written, and it settled the question:

- `go.opentelemetry.io/contrib/bridges/otelzap` v0.20.1 requires `go.uber.org/zap v1.28.0`.
  This repository uses `github.com/Laisky/zap`, a **separate Go module**, so its
  `zapcore.Field` and `zapcore.Core` are distinct types no configuration can reconcile.
- The fork's `zapcore.Core` interface additionally declares `Fields() []Field`
  (`zapcore/core.go`), which `go.uber.org/zap` has no notion of. Even a type-identical
  vendoring of upstream would not satisfy it.

The adapter therefore exists at `common/logger/otelbridge/`. Its record mapping is
deliberately identical to upstream's (levels → severities, namespaces → nested
`attribute.Map`, `zap.Error` → `Record.SetErr`, caller/stack → `code.*`/`exception.*`
semconv v1.43.0), so a collector pipeline or dashboard written against `otelzap` works
unchanged. `convert.go` and `encoder.go` carry the upstream Apache-2.0 attribution.

Two deliberate divergences from upstream, both because upstream's choice is wrong here:

| Upstream | Here | Why |
| --- | --- | --- |
| `Sync()` is hard-coded `return nil` | `Sync()` force-flushes the provider | `zap.Fatal` calls `Sync` then `os.Exit`; a no-op loses every queued record at the moment they matter most. Upstream closed the issue asking for this as not-planned. |
| Correlation via a logged `context.Context` field | `SpanContextField`, a `SkipType` field carrying only the 24-byte `SpanContext` (the context field still works) | This repository keeps request loggers free of the gin context on purpose — `identity.Bind` stores the logger by value and `relayctx.Detach` snapshots it into goroutines that outlive the request, so binding a context would retain the whole request. |

### 4.2 Correlation, measured on the protocol fields

Trace correlation in OTLP is **not** an attribute: `trace_id`, `span_id` and `flags` are
top-level `LogRecord` protocol fields that the SDK fills from the context passed to
`Logger.Emit`. A bridge that wrote `trace_id` as an attribute would satisfy a naive test
and join against nothing in Loki, Tempo or ClickHouse. Every correlation assertion here is
therefore against `Record.TraceID()` / `Record.SpanID()` / `Record.TraceFlags()`.

`middleware/otlp_log_correlation_test.go` drives the production middleware order —
`otelgin` → `gmw.NewLoggerMiddleware` → `RequestId` → `TracingMiddleware` — with the tee
wired exactly as `common/logger/otlp_sink.go` wires it:

| Test | What it establishes |
| --- | --- |
| `TestRequestLoggerRecordsCarryTheRequestSpanIDs` | a plain `gmw.GetLogger(c).Info(...)` exports with the request's trace and span ids, and with the **caller's propagated** trace id (`4bf92f...4736` from a `traceparent` header) |
| `TestRequestLoggerRecordsKeepIdentityFields` | correlation is additive: `request_id` and call-site fields survive as attributes, and the correlation field itself never appears as one |
| `TestRecordsAreUncorrelatedWithoutTheBoundField` | the mutation check — with the binding removed the same handler produces records with **no valid trace id**, so the passing test above cannot be passing for another reason |
| `TestCallSiteSpanContextOverridesTheBoundOne` | a detached goroutine can log on behalf of a different span, which `relayctx.Detach`'d billing work needs |
| `TestCorrelationFieldIsNotBoundWhenTheBridgeIsOff` | the gate in `RequestId` is real (§4.6) |

`TestCorrelationFieldIsInvisibleToOtherEncoders` closes the compatibility half: the JSON
encoder's output is **byte-identical** with and without the correlation field, because the
field's type is `zapcore.SkipType` and `Field.AddTo` treats that as a no-op. §2.1 of the
proposal requires "Full application log-line fields, existing sink selection"; this is what
makes that true rather than approximately true.

### 4.3 Bounds and outage

`common/telemetry/log_pipeline.go` puts an admission gate in front of the SDK batch
processor. The gate is not a second queue — the SDK's queue is still the queue — it is what
makes the loss **countable**: the SDK ring drops the *oldest* record on overflow and reports
it only through its internal logger and an experimental, env-gated self-observability
signal. The gate's ceiling is set equal to the batch queue size so the SDK ring never
overflows first, and residency is released when the exporter is handed a batch.

| Test | Measurement |
| --- | --- |
| `TestBoundedProcessorDropsAndCountsWhenRecordCeilingIsReached` | ceiling 3, 10 emitted → **3 admitted, 7 counted** `dropped_queue_full` |
| `TestBoundedProcessorEnforcesAByteCeilingIndependently` | record ceiling 1000, byte ceiling 4 KiB, 10 × 2 KiB records → refused on **bytes**; admitted + dropped = 10 exactly |
| `TestExportReleasesResidencySoTheGateReopens` | after a batch reaches the exporter the capacity returns; the gate is a queue bound, not a lifetime quota |
| `TestExportFailureIsCountedSeparatelyFromSuccess` | 2 records `export_failed`, 0 `exported`; then 1 `exported` |
| `TestOutageDoesNotBlockTheEmittingGoroutine` | 200 emissions against a **blocked** exporter complete; `OnEmit` runs on the request goroutine, so this is the property that makes the bridge safe to enable on a relay gateway |
| `TestShutdownReportsRecordsItCouldNotDrain` | 5 undrained records counted `dropped_shutdown` |
| `TestBridgeCountsRecordsBeforeProviderInstall` | startup window counted `dropped_not_ready` — main.go configures logging before it initializes OpenTelemetry, because telemetry initialization logs |
| `TestBridgeCountsRecordsAfterShutdown` | shutdown window counted `dropped_shutdown`, and a closed holder refuses re-installation |

The outcome vocabulary is `emitted` (handed to the SDK, the denominator),
`dropped_queue_full`, `dropped_not_ready`, `dropped_shutdown`, `exported`, `export_failed`.
`exported` means the exporter accepted and transmitted a batch without error. **It is not
proof of collector persistence**, exactly as the trace sink's `span_recorded` refuses to
claim it.

One measurement error worth naming: a hand-built `sdklog.Record` is useless for size
assertions. Its attribute limits are zero, and a zero count limit means *discard every
attribute*, so records built by hand silently carry none. The byte-ceiling test initially
"passed" against empty records. The tests now obtain records from the real SDK logger.

### 4.4 The core-stack position, proven by mutation

`common/logger/otlp_sink.go` claims a specific position: the tee goes **above** the
emergency disk budget and **below** sampling. That claim is expressed only by where an
option sits in a slice, so it is exactly the kind of thing a later edit breaks silently.

| Test | Consequence asserted | Mutation applied | Result |
| --- | --- | --- | --- |
| `TestDiskPressureDoesNotStopOTLPExport` | with the disk budget engaged, local output is suppressed and **all 40** lines still export | tee moved below the budget | **FAIL** |
| `TestSamplingAppliesToOTLPExport` | with an initial budget of 2, both branches export exactly **2 of 50** | tee moved above sampling | **FAIL** |

Both mutations were reverted and the restored order passes. The reasoning behind the
position: a full local disk is not a reason to stop exporting to a collector that has room,
and charging the disk budget for records that never touch the disk would make its
suppression counters describe something that did not happen; conversely, an operator who
thinned the log stream to survive a storm did not ask for the unthinned stream to be
shipped off-box instead.

### 4.5 Configuration

`APP_LOG_SINK` gains an **additive** `otlp` token: `both,otlp`, `stdout,otlp`, `file,otlp`.

A bare `otlp` is **rejected at startup**. The bridge drops records when its queue is full,
before the provider is installed, and after it is shut down; making it the sole destination
would leave a gateway whose only log record can be discarded without a trace. Every legacy
value keeps its exact meaning — `normalizeAppLogSink` strips the token and answers
`file`/`stdout`/`both` as before, so every existing comparison is untouched
(`TestAppLogSinkOTLPTokenIsAdditive`).

Raw fail-fast validation follows the `TRACE_SINK` precedent rather than the silently
normalizing enum: an unknown token, a duplicate, two local sinks, or `otlp` without
`OTEL_ENABLED=true` are all rejected with the variable named. Eleven matrix cases were
added to `TestObservabilityConfigurationMatrix`.

New settings, all bounded by default because an absent bound is the defect:

| Setting | Default | Bounds |
| --- | --- | --- |
| `LOG_OTLP_MIN_LEVEL` | `info` | applied zap-side, so a filtered record is never converted; **raise-only** (§6.4), so `DEBUG=true` does not start shipping debug volume to a shared collector and a low floor cannot widen what the process logs |
| `LOG_OTLP_QUEUE_SIZE` | 10000 records | the gate's record ceiling, and the SDK queue size |
| `LOG_OTLP_QUEUE_MAX_MB` | 64 MiB | the gate's byte ceiling; record count alone does not bound memory |
| `LOG_OTLP_BATCH_SIZE` | 512 | SDK default |
| `LOG_OTLP_EXPORT_INTERVAL_MS` | 1000 | SDK default |
| `LOG_OTLP_EXPORT_TIMEOUT_MS` | 30000 | SDK default; the shutdown deadline must exceed it or the final drain is truncated |
| `LOG_OTLP_MAX_ATTRIBUTES` | 128 | SDK default |
| `LOG_OTLP_MAX_ATTRIBUTE_VALUE_BYTES` | 4096 | **the SDK default is unlimited**; one adaptor logging an upstream payload would otherwise put an unbounded string on the wire |

The exporter reuses `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_EXPORTER_OTLP_INSECURE` rather
than introducing log-specific endpoint settings, so one collector address configures all
three signals and cannot drift between them.

### 4.6 Cost

`go test -run XXX -bench BenchmarkBridgeWrite -benchmem -count=5 ./common/logger/otelbridge/`,
median of 5:

| Arm | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `bridge_disabled` (discard core alone) | 148 | 16 | 1 |
| `bridge_enabled` (discard core + bridge) | 2,509 | 916 | 5 |
| `encoding_local_only` (real JSON encoder → io.Discard) | 601 | 0 | 0 |
| `encoding_local_plus_bridge` | 3,187 | 903 | 4 |
| `bridge_enabled_correlated` | 2,538 | 1,045 | 7 |

Against the realistic baseline — a real JSON encoder, which is what the file sink costs
minus the syscall — **the bridge adds about 2.6 µs and about 900 B per exported line.**
That is not negligible, and it is the reason `LOG_OTLP_MIN_LEVEL` defaults to `info` and
the tee sits below sampling. Correlation itself is nearly free in time (+129 B, +2 allocs).

The **disabled** cost is zero by construction: with `APP_LOG_SINK` unchanged, no bridge
core is installed at all (`TestOTLPBridgeOptionIsOffByDefault`).

One cost was not zero and was fixed after measuring it. The correlation field was
originally bound on every request unconditionally, which measured **+192 B/op and +1
alloc/op** on the logger rebuild — small, but paid by every request of every deployment,
including the ones that never enable the bridge. `RequestId` now resolves
`config.AppLogOTLPEnabled` once at middleware construction and skips the binding entirely
when the bridge is off, pinned by `TestCorrelationFieldIsNotBoundWhenTheBridgeIsOff`.

---

## 5. W3.3 — operational metrics

### 5.1 What was added

Three new **optional** extension interfaces, following the `TracePipelineRecorder` pattern
exactly: no method was added to `MetricsRecorder`, so an out-of-tree implementation keeps
compiling. Each is implemented on `PrometheusRecorder`, `OtelRecorder` and the
`MultiRecorder` fan-out, with compile-time conformance assertions in all three assertion
blocks — the mechanism that stops a new interface from silently reading zero in production.

| Interface | Series | Source |
| --- | --- | --- |
| `RequestOutcomeRecorder` | `oneapi_request_outcomes_total{outcome}`, `oneapi_request_duration_ms{outcome}`, `oneapi_request_time_to_first_token_ms{outcome}` | `recordTraceEnd`, **above** the sampling decision |
| `RetentionRecorder` | `oneapi_retention_sweeps_total{target,result}`, `oneapi_retention_sweep_rows_total{target,result}`, `oneapi_retention_sweep_duration_ms{target,result}` | `ChunkedDeleteWithStats` callers plus the app-log file sweeper |
| `LogExportRecorder` | `oneapi_app_log_export_records_total{outcome}`, `oneapi_app_log_export_queue_{records,record_limit,bytes,byte_limit}` | the bridge and its admission gate |

Two properties are load-bearing and tested:

- **Operational metrics are sampling-independent.** They are recorded before
  `SampleDecisionFor` returns, so `TRACE_SAMPLE_RATE=0.05` does not silently turn the
  operational view into a 5 % view. Moving the call below the sampling return fails
  `TestRequestOutcomeIsRecordedWhenTheTraceIsSampledOut`.
- **`tracing.FailureKind` is never used as a label.** Its own documentation forbids it, so
  a mapping function converts `(status, failure)` into the metrics package's own closed
  vocabulary, with panic > timeout > canceled > upstream > server_error > client_error >
  success precedence. The upstream case deliberately includes requests whose client-visible
  status is 200 because the stream had already flushed — the case an HTTP status histogram
  cannot see.

Requests that never get a recorder (excluded paths, `TRACE_SINK=none`, admission-denied)
are deliberately **not** counted: they return before `Recorder.Finish`, so they have no
measured lifetime, and a fabricated 0 ms would corrupt the latency histogram. They remain
visible in the trace-pipeline series, so "not measured" stays distinguishable from
"measured and healthy". A consequence worth stating plainly: **`TRACE_SINK=none` also
disables per-request operational metrics.**

### 5.2 What was deliberately not added

W3.3 lists eight quantities. Two of them — **dashboard projection lag** and **projection
backfill backlog** — have no source in this tree, because W2.2 and W2.3 are not
implemented. They are absent rather than reported as zero: a gauge reading 0.0 for a
pipeline that does not exist is worse than a missing one, since an operator cannot
distinguish "healthy" from "not running". Claim 13 in §1 is marked not-supported-as-written
for exactly this reason.

---

## 6. Findings

### 6.1 A proof test that stopped proving its claim when the SDK got faster

`TestZeroReservoirCutsCollectBytes` (from the W0/W1 work) began failing five runs in six on
the bumped tree. The optimization was fine; the measurement was not.

The test read process-global `runtime.MemStats.TotalAlloc` across 50 iterations, which
counts **every** allocation in the process during the window, not the closure's. That only
works while the measured effect is far larger than whatever else the test binary is doing.
Measured on this machine:

| Tree | default arm | zero-reservoir arm | ratio |
| --- | --- | --- | --- |
| baseline `32df60cf` (otel v1.44.0) | 753,282 B/collect | 26,087 – 280,163 (6 runs) | 3.5 % – 37.2 % |
| bumped (otel v1.46.0) | 368,983 B/collect | 343,061 – 486 (6 runs) | 0.1 % – 96.4 % |

The SDK bump roughly **halved** the default arm's per-collect cost. The background noise
stayed where it was, so it went from negligible-against-753 KB to dominant-against-369 KB.

Per-operation accounting shows the optimization is untouched, and in fact enormous:

```
BenchmarkCollect/default_reservoir-16     200  748,581 ns/op  368,728 B/op  1,011 allocs/op
BenchmarkCollect/zero_reservoir_view-16   200   42,336 ns/op      440 B/op     11 allocs/op
```

The test now measures the same closures with `testing.Benchmark`, which attributes only
what the benchmarked function allocates. The assertion was **tightened, not relaxed** —
from a 2x margin to 10x — and it now reads 0.1 % on every run instead of five different
numbers in six.

### 6.2 A pre-existing flaky test, surfaced by a full-suite run

`TestCursorNeverDuplicatesUnderConcurrentWriters` (W2.4) failed once during full-suite
verification. It reproduces **on the pristine baseline** — `go test -count=20 -run Cursor
./model/` fails on `32df60cf` and passed on the Phase 3 tree in the same experiment — so it
is not caused by this work.

The mechanism: the walk started with a nil cursor, meaning "whatever is newest when the
first query runs", while a writer goroutine was already inserting rows newer than the
seeded ones. A row inserted between the writer starting and the first page being read
legitimately belongs to the result, and the test then fails for a reason that is not a
defect. The traversal's start is now pinned to an explicit anchor, which makes the
assertion a statement about the cursor instead of about which goroutine reached SQLite
first.

**Honest limit on this fix:** reverting the pin did *not* reproduce the failure in 20
isolated runs, so the fix is justified by removing the mechanism, not by a mutation that
re-triggers the flake. The original failure needed full-package timing to appear.

### 6.3 The bridge is not cheap

§4.6 measures about 2.6 µs and about 900 B per exported line against a realistic local
baseline. Nothing in this record establishes what that costs at load; the arithmetic that
an operator should do before enabling it is in the runbook, and the levers are
`LOG_OTLP_MIN_LEVEL`, the sampling settings, and `TRACE_EXCLUDED_PATH_PREFIXES`.

### 6.4 A severity floor that could widen the whole process's logging

`LOG_OTLP_MIN_LEVEL` was originally applied as a plain level on the bridge core.
`zapcore.NewTee` reports an entry as enabled when **any** child accepts it, so a floor
below the process log level did not merely export more — it made the whole gateway
construct and evaluate entries it is configured not to log.

Probed directly before the fix, with the local core at `info` and the bridge floor at
`debug`:

```
local entries=1  exported=2
```

The debug line was exported while the local sink recorded nothing, so the OTLP stream
contained a line that appears in no log file, and every debug call site in the relay path
paid its field cost. The floor is now clamped to at least the process level
(`effectiveBridgeLevel`), making it a raise-only knob, which is what all of its documented
use cases actually want. Pinned by `TestBridgeLevelCanOnlyNarrow`.

### 6.5 A shared test helper that promised more than it delivered

`model.setupTestDatabase` re-initialized only when `DB == nil`. Several tests in that
package install and then close their own pool to exercise a database failure; if one leaves
a closed handle reachable, every later test that shares the helper fails with "sql:
database is closed" and the failure is attributed to whichever test ran next. The helper
now checks whether the handles are **usable**, not merely non-nil.

---

## 7. What this record does NOT establish

- **No capacity claim.** G5 is untouched. Every figure here is a component measurement on
  one machine, and none of it describes a gateway under load.
- **No collector-persistence claim.** `exported` means the exporter accepted a batch. What
  the collector did with it is outside this process's knowledge, by construction.
- **No stability claim for the log pipeline.** The exporters are outside the Logs RC's
  stability scope and are expected to remain `v0.x`.
- **No claim about the two-tier collector topology.** The runbook documents it from
  upstream sources; nothing here deploys or measures one.
- **No W2 progress.** Projection lag and backfill backlog remain unimplemented and
  unmeasured (§5.2).
- **No load-shaped log volume evidence.** The queue-bound tests use ceilings of 3, 10 and
  8 records so the boundary is exact and the assertion is unambiguous. They demonstrate the
  bound and its accounting; they do not demonstrate behavior at 10,000 RPS of log volume.
