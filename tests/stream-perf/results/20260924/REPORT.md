# Streaming chat E2E: measured experiment, 2026-09-24

## Decision

Retain the combined `strings.Builder` accumulation and already-buffered SSE-line fast path. Reject the subsequent `fmt.Sprint` string shortcut. The strongest repeat-consistent result is the long-stream concurrency-64 profile: median throughput 11.38 -> 15.99 requests/s, CPU 147.42 -> 101.33 ms/request, sampled peak RSS 479.65 -> 365.11 MiB, and p95 completion 10.26 -> 7.38 seconds. Every one of its three paired runs improved throughput, CPU, RSS and p95 completion.

This is a scoped optimization decision, not proof that no further optimization exists. Short/paced streams were noisy, and the original matrix contains unfavorable latency observations. Those observations are retained below and in the raw data, not replaced by favorable confirmation runs.

## Environment and provenance

All request traffic ran locally on Linux using immutable GitHub Actions build artifacts. Timings are **not** GitHub runner timings. The host reported Intel Xeon Platinum 8370C, five visible logical CPUs, a cgroup CPU quota of four cores, a 4 GiB memory limit, and `GOMAXPROCS=2` independently for gateway, mock and client. Processes shared the host; they were not CPU-affinity-isolated. No other performance experiment ran concurrently. Setup and validation are excluded from the timed windows.

The baseline production revision is `a8782e3dc0dad7704acb0d008e7d30a26096338d`, built by the workflow-only revision `0eec195cd35d3ea33185a8b6de879345a87fedd2`. The optimized production binary is from `ce0c12f84e5fab29d0c8a778f7b98ca735dcc4d9`. Both use Go 1.27.1 and `go build -trimpath`. Exact binary/driver hashes and build artifact IDs are in [manifest.json](manifest.json).

The branch subsequently merged main revision `4ab196e0bcefbeaae2a4fe9e1bdb07a0ceece86c`, containing non-overlapping model catalog changes. The streaming implementation is unchanged, but the merged catalog revision was **not re-benchmarked**. Do not attribute these absolute figures to an unmeasured later binary.

Each trial starts a fresh real gateway and SQLite database, creates one local OpenAI-compatible channel, warms 16 requests, and waits for their billing to settle. Authentication, routing, quota, tracing/logging and durable billing stay enabled. Only the still-enabled global rate-limit ceilings are raised. A/B order alternates by repetition. Request-specific Unicode content, ordering, stop, usage and exactly one DONE through EOF are required for success. Gateway CPU and sampled RSS are separate from mock/client resources. CPU/request includes billing settlement; RSS is sampled every 20 ms, not an exact kernel high-water mark.

## Initial A/B matrix

Long/saturated: 1,024 content chunks x 128 UTF-8 bytes, no upstream delay, 128 measured requests per trial. Short/paced: 32 chunks x 128 bytes, 2 ms delay per chunk, 256 requests per trial. Three A/B pairs per cell; 48 trials and 9,216/9,216 successful requests. Billing counts and used quota match across every pair.

Values below are medians across runs. **Paired changes are medians of per-pair percentage changes**, which are not generally equal to percentage changes between the two separately calculated medians. Observed ranges are descriptive, not confidence intervals. Requests within a run are not treated as independent experiments.

| Profile | Concurrency | RPS baseline / optimized | Paired RPS change | CPU ms/request baseline / optimized | Paired CPU change | RSS MiB baseline / optimized |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 1 | 12.40 / 12.50 | +0.15% | 20.04 / 18.98 | -0.61% | 248.28 / 253.45 |
| paced | 8 | 87.01 / 86.08 | -1.75% | 13.01 / 13.20 | +1.50% | 257.96 / 255.76 |
| paced | 32 | 138.61 / 138.41 | +3.41% | 11.99 / 11.91 | -9.12% | 277.86 / 275.40 |
| paced | 64 | 128.55 / 103.14 | -13.60% | 12.70 / 15.55 | +15.18% | 281.10 / 280.87 |
| saturated | 1 | 6.54 / 11.51 | +64.10% | 175.86 / 92.50 | -43.54% | 310.01 / 299.40 |
| saturated | 8 | 14.15 / 19.63 | +30.04% | 138.20 / 99.53 | -26.51% | 455.45 / 325.82 |
| saturated | 32 | 11.75 / 15.25 | +10.77% | 143.52 / 106.64 | -17.82% | 480.71 / 348.23 |
| saturated | 64 | 11.38 / 15.99 | +41.59% | 147.42 / 101.33 | -31.32% | 479.65 / 365.11 |

| Profile | Concurrency | p95 TTFT ms baseline / optimized | Paired TTFT change | p95 completion ms baseline / optimized | Paired completion change |
| --- | ---: | ---: | ---: | ---: | ---: |
| paced | 1 | 9.86 / 8.84 | -7.14% | 87.80 / 85.32 | -1.42% |
| paced | 8 | 23.30 / 34.99 | +50.22% | 118.28 / 117.75 | +3.11% |
| paced | 32 | 370.90 / 385.93 | -10.31% | 535.02 / 554.20 | +3.58% |
| paced | 64 | 891.65 / 1334.91 | +62.56% | 1073.43 / 1425.59 | +62.27% |
| saturated | 1 | 17.08 / 13.21 | -11.39% | 301.92 / 127.74 | -50.80% |
| saturated | 8 | 165.59 / 161.44 | -2.51% | 967.74 / 647.32 | -26.12% |
| saturated | 32 | 4736.91 / 5333.80 | -1.34% | 6086.31 / 5638.82 | +29.06% |
| saturated | 64 | 7920.80 / 6555.20 | -15.55% | 10258.30 / 7378.72 | -28.07% |

**Unfavorable observations:** the initial paced concurrency-64 cell had -13.60% paired throughput, +15.18% CPU/request and worse latency. The initial saturated concurrency-32 cell had +29.06% paired p95 completion despite a lower separately calculated median. This disagreement is why we do not use a single pooled score or claim a universal latency improvement.

## Longer confirmation experiments

These are separate experiments with different request counts; they are not pooled with the initial matrix and do not erase it.

| Profile | Concurrency | A/B pairs | Requests per trial | RPS baseline / optimized | Paired RPS change | CPU ms/request baseline / optimized | p95 completion ms baseline / optimized |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 64 | 10 | 1024 | 129.05 / 141.89 | +15.50% | 12.63 / 11.57 | 1180.33 / 1061.56 |
| saturated | 32 | 5 | 256 | 11.64 / 16.47 | +41.25% | 145.23 / 103.79 | 7417.74 / 5290.63 |

paced concurrency 64: paired RPS changes ranged from -18.01% to +114.68%; paired p95 completion changes ranged from -61.73% to +42.45%.

saturated concurrency 32: paired RPS changes ranged from +13.60% to +54.61%; paired p95 completion changes ranged from -43.19% to +7.59%.

The paced result does not establish a repeat-consistent short-stream speedup or statistical equivalence. The longer long-stream experiment supports retaining the resource/throughput improvement without reproducing the initial median paired concurrency-32 tail regression; one confirmation pair still had +7.59% p95 completion. All 23,040 requests in these confirmation experiments succeeded, with matching pairwise billing.

## Rejected subsequent optimization

The experimental binary avoids `fmt.Sprint(data)` when SSE data is already a string. Five paired runs per cell, with the currently optimized binary as its baseline; 40 trials and 7,680 successful requests. The exact [rejected patch](rejected-format.patch) is preserved for reproduction but is **not applied to production code**.

| Profile | Concurrency | RPS baseline / experiment | Paired RPS change | Paired CPU/request change |
| --- | ---: | ---: | ---: | ---: |
| paced | 8 | 88.03 / 88.78 | +2.15% | +6.01% |
| paced | 64 | 143.23 / 134.71 | -7.28% | +7.82% |
| saturated | 8 | 19.61 / 19.88 | +1.38% | -2.04% |
| saturated | 64 | 15.78 / 17.69 | +7.19% | -3.79% |

The shortcut did not provide a repeat-consistent, broad improvement: paced concurrency-64 throughput regressed while CPU/request rose. Its long-stream benefit was workload-dependent. It fails our decision rule of a material benefit (roughly 5% or greater in throughput or CPU/request), correct delivery/billing, and no repeat-supported unacceptable tradeoff in the companion profile. No extra production complexity is retained for that candidate.

## Apparatus calibration and fixed-rate pressure

Three direct mock/client calibration repetitions produced median throughput of 101.65 RPS at long-stream concurrency 8 and 91.26 RPS at concurrency 64; paced concurrency 64 reached 651.84 RPS. This indicates headroom relative to the gateway profiles, not a basis for subtracting independently measured percentiles.

The optimized gateway then received 1,024 scheduled requests per run, three runs per offered rate, using the paced profile and a bounded population of 64 admitted requests (queued plus active). Excess arrivals are **load-generator admission drops before HTTP**, not gateway HTTP errors. Scheduling delay is recorded in the full JSON. Successful-only latency must not conceal these drops.

| Offered RPS | Completed / offered across 3 runs | Admission drops | Median successful RPS | Median p95 TTFT ms | Median p95 completion ms | Worst run p95 completion ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 50 | 3072 / 3072 | 0 | 49.85 | 118.46 | 230.04 | 1211.69 |
| 100 | 3072 / 3072 | 0 | 99.29 | 13.30 | 99.27 | 114.59 |
| 200 | 2270 / 3072 | 802 | 145.06 | 688.21 | 1061.19 | 1270.62 |
| 400 | 1121 / 3072 | 1951 | 141.95 | 753.09 | 1031.10 | 1546.81 |

**100 offered RPS is the highest tested rate with zero drops in all three bounded runs.** This is not a sustained production capacity or SLO guarantee. At 200 and 400 offered RPS, completed throughput plateaued near 142-145 RPS while shedding load. The anomalously slower 50-RPS run is deliberately shown; this host/workload has substantial variability. There were no protocol/HTTP failures among admitted requests, and completed requests still matched durable billing.

## Validation and CI

Local validation outside timed measurements: eight harness/reporter/build-orchestration tests passed; twelve existing workflow-contract tests passed; twelve existing shard-orchestration tests passed; the committed evidence verifier passes all 142 records. The Go driver is exercised over real local HTTP, including nine valid/invalid stream modes, overload visibility, loopback restrictions and process resource accounting. Both original A/B binaries passed real-gateway malformed/truncated/corrupt stream controls, invalid-auth rejection and eight-client upstream cancellation cleanup.

The existing `go test ./...` entrypoint discovers `TestStreamingGatewayE2E`, so no third workflow is required. The temporary experiment workflow was removed rather than weakening the two-workflow contract. Prior CI run `36024642561` passed focused race/correctness validation. In main CI run `36024642522`, `TestStreamingGatewayE2E` passed; the packages shard failed three gpt-5.2 sampling-policy tests. The same three failures are independently present in the pre-optimization run `36018329005`: `TestMappedChatSamplingPreservesNone`, `TestModelSamplingCatalogPolicy`, and `TestNativeResponsesPreservesSupportedSampling`. They are not silently counted as passing. Consult the PR checks for the final head status; this report does not certify merge readiness.

## Reproduce and audit

```sh
bash tests/stream-perf/compare.sh a8782e3dc0dad7704acb0d008e7d30a26096338d /tmp/new-stream-study \
  --repeats 3 --concurrency 1,8,32,64 --requests 128 --paced-requests 256
python3 tests/stream-perf/report.py /tmp/new-stream-study/results/summary.json --expected-repeats 3
python3 tests/stream-perf/results/20260924/verify.py
```

The build helper checks out committed HEAD and the immutable baseline into disposable worktrees and pins one Go toolchain. Uncommitted edits are excluded. It preserves source revisions, binary hashes and raw observations, and refuses to overwrite an existing experiment directory. Final runner summaries checkpoint every trial atomically and explicitly distinguish incomplete studies.

[runs.csv](runs.csv) contains all 142 run-level observations needed to audit the reported throughput, p95 latency, CPU/request, RSS, success/drop counts and billing. [manifest.json](manifest.json) defines every workload and binary and hashes the CSV and original full summaries. [verify.py](verify.py) validates integrity, traffic accounting, complete matrices and paired billing before recomputing the comparisons. Across all matrices: 54,528 offered requests, 51,775 verified completions, zero failed admitted requests, and 2,753 intentional overload admission drops. This excludes setup, warm-up and correctness controls.

Full per-request JSON, p50/p95/p99/DONE/EOF/scheduled-latency distributions, gateway/mock/client resource details, runner snapshots and local validation logs are provided with the accompanying evidence archive. The initial matrix used a legacy `average_cores` denominator; that field is not used in any reported comparison. CPU/request and RSS are unaffected, and subsequent experiments use the corrected billing-settlement measurement interval.

## Scope and stop condition

Only OpenAI-compatible streaming chat is qualified here. Redis, PostgreSQL/MySQL, multiple tenants/tokens, TLS, WAN/proxies, slow external consumers, long soaks, Responses conversion and other provider protocols require additional profiles. We retain the two production changes as a measured bundle; this study does not separately attribute their individual contributions. Remaining latency mechanisms have not been isolated by profiling, so no unsupported bottleneck diagnosis is claimed.

The current loop stops after the subsequent formatting candidate fails the measured acceptance rule and the unfavorable initial cells receive explicit longer-run checks. Future work should use this same harness and immutable controls, not claim a global optimum or discard adverse observations.
