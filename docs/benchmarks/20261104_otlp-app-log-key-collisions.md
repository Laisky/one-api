# OTLP application logs: repeated keys no longer drop request logs

## Problem, as observed in production

The production log store (VictoriaLogs, fed by the go-fluentd edge) held almost
nothing from one-api except three periodic lines (`syncing options from database`,
`syncing channels from database`, `channels synced from database, considering
suspensions`). Reconciling one-api's container output on b1 against the store on
2026-11-04 for the same six hours:

| Source on b1 stdout | Lines | In the log store |
| --- | --- | --- |
| `one-api` logger, info and above | 2,190 | 2,178 (the 12 are the window edge: 4 of each periodic line) |
| `one-api.gin` request logger, info and above | 362 | **0** |
| `go-utils.gin` (background work via `logger.FromContext`) | 6 | **0** |
| stderr `write error: application log cannot be encoded faithfully` | 362 | — |

The 362 encoding errors are exactly the 362 missing request-scoped lines. Two
defects, both reproduced by tests that failed before the fix:

1. **Key collision.** `logger.SetupEnhancedLogger` binds `host` (the container
   hostname); gin-middlewares' request logger binds `host` again (the HTTP Host
   header). The OTLP bridge rejected any record with a repeated key, so every
   request-scoped record was dropped with `dropped_encoding`.
   `test/applogexport.TestRequestScopedLogsReachTheOTLPCollector`,
   `otelbridge.TestLogFidelityBridgeDuplicateKeysResolveLastValueWins`,
   `telemetry.TestLogFidelityNativeDuplicatesResolveLastValueWins`.
2. **Context fallback.** `logger.FromContext` relied on `gmw.GetLogger`, which
   never reports a missing logger and returns gin-middlewares' stdout-only library
   logger instead, so background work never reached the file, OTLP, sampling or
   alerting. `test/applogexport.TestBackgroundContextLogsReachTheOTLPCollector`,
   `logger.TestFromContextFallsBackToTheProcessLogger`.

Volume note, so the table above is not misread: in the same window about 99% of
the container's output was GORM SQL tracing (production runs `DEBUG=true`), which
GORM writes straight to stdout and which never entered the OTLP pipeline at all.
That is tracked separately; it is not affected by this change.

## Fix

Repeated keys are resolved the way the OpenTelemetry common specification
prescribes for a key set twice — the later value overwrites the earlier one, at
every depth — instead of discarding the record. The bridge does this itself
(`otelbridge.NormalizeLogRecordAttributes`) in the same pass that already
validated the record. `TestNormalizeMatchesSDKDeduplication` pins the result to
the SDK's own deduplication, and to the spec where the pinned SDK is not
conformant (an overwriting value whose map repeats a key).

## Why not enable the SDK's deduplication instead

That was the first candidate: drop `sdklog.WithAllowKeyDuplication()` and let the
SDK resolve collisions. The SDK copies a record's attributes one at a time and,
with deduplication on, rebuilds a key index for each, so the cost grows with the
square of the attribute count. On the same request-shaped record (13 attributes,
old bridge, quiet machine, 8 runs of 200,000 operations):

| Provider | ns/op (mean) |
| --- | --- |
| SDK deduplication off (production) | 9,076 |
| SDK deduplication on | 11,564 (+27%) |

## Before/after

`common/logger/otelbridge/bench_request_test.go`,
`BenchmarkBridgeWriteRequestRecord`. "before" is the deployed commit
`3a8414dd1496`, "after" this change; the identical benchmark file was compiled
into both trees and the two binaries were run alternately. `exported/op` proves
what was measured: 0 means the record was rejected.

- Sub-benchmark `production` uses production's provider options;
  `sdk-dedup` is the rejected alternative above.
- Record `unique` has no repeated key; `host-collision` repeats `host` exactly as
  production does.

Environment: AMD Ryzen 7 5700G (16 threads), Linux 6.8.0, go1.27.3,
go.opentelemetry.io/otel v1.46.0, otel/sdk/log v0.22.0. The machine was shared
(load average 9.5–12.6), so **time** confidence intervals are wide; allocation
counts are deterministic. Run pinned to one core with `GOMAXPROCS=1 taskset -c 5`,
10 alternating runs of 200,000 operations:

```
                                                   │    before    │                 after                 │
                                                   │    sec/op    │    sec/op      vs base                │
BridgeWriteRequestRecord/production/unique           9.672µ ± 13%    7.370µ ± 67%  -23.81% (p=0.029 n=10)
BridgeWriteRequestRecord/production/host-collision   9.992µ ± 58%    8.113µ ± 86%        ~ (p=0.143 n=10)
BridgeWriteRequestRecord/sdk-dedup/unique            14.65µ ± 33%    12.72µ ± 83%        ~ (p=0.353 n=10)
BridgeWriteRequestRecord/sdk-dedup/host-collision    9.982µ ± 26%   11.075µ ± 71%        ~ (p=0.072 n=10)

                                                   │ exported/op │ exported/op │
BridgeWriteRequestRecord/production/unique           1.000         1.000
BridgeWriteRequestRecord/production/host-collision   0.000         1.000
BridgeWriteRequestRecord/sdk-dedup/unique            1.000         1.000
BridgeWriteRequestRecord/sdk-dedup/host-collision    0.000         1.000

                                                   │     B/op     │      B/op      vs base               │
BridgeWriteRequestRecord/production/unique           6.316Ki ± 0%   4.250Ki ± 0%  -32.71% (p=0.000 n=10)
BridgeWriteRequestRecord/production/host-collision   4.946Ki ± 0%   4.375Ki ± 0%  -11.55% (p=0.000 n=10)

                                                   │  allocs/op  │  allocs/op    vs base               │
BridgeWriteRequestRecord/production/unique           21.00 ± 0%    16.00 ± 0%   -23.81% (p=0.000 n=10)
BridgeWriteRequestRecord/production/host-collision   21.00 ± 0%    16.00 ± 0%   -23.81% (p=0.000 n=10)
```

An unpinned run of 8 alternating pairs gave the same picture
(`production/unique` −25.18%, p=0.021; allocations identical to the above).

Reading:

- **Collision-free records got cheaper**, not more expensive: −24% time (significant
  in both runs) and −24% allocations. The bridge no longer walks the finished
  record a second time to collect its attributes for validation; validation and
  collision resolution share one pass over the list it already built.
- **Colliding records are now exported** (`exported/op` 0 → 1) for about the cost
  of a collision-free one. Before, the same work ended in a rejection, so there is
  no like-for-like time comparison for that row.
- The `sdk-dedup` rows confirm the rejected alternative remains the slower one.

## Reproduce

```bash
git worktree add /tmp/before 3a8414dd1496
cp common/logger/otelbridge/bench_request_test.go /tmp/before/common/logger/otelbridge/
(cd /tmp/before && go test -c -o /tmp/before.test ./common/logger/otelbridge/)
go test -c -o /tmp/after.test ./common/logger/otelbridge/
for i in $(seq 10); do
  for b in before after; do
    GOMAXPROCS=1 taskset -c 5 /tmp/$b.test -test.run '^$' -test.bench BenchmarkBridgeWriteRequestRecord \
      -test.benchmem -test.benchtime 200000x 2>/dev/null | grep '^Benchmark' >> /tmp/$b.txt
  done
done
benchstat /tmp/before.txt /tmp/after.txt
```

The "before" binary's collision rows print one `write error` line per operation on
stderr, which is why stderr is discarded.
