# PR 427: redundant SSE-copy experiment — rejected

## Decision and retained changes

**Do not apply the copy-elimination candidate.** The complete, predeclared matrix produced 40 trials and 15,360 verified requests, with zero failures/drops and exact paired durable usage. It nevertheless fails two operational gates: long-stream concurrency-8 first-content latency regressed by a paired median **17.75% / 12.02 ms**, exceeding both 10% and 5 ms; no long-stream metric combined at least 5% median benefit with favorable direction in at least four of five pairs.

The concurrency-64 long-stream throughput change was +11.64%, but favorable in only 3/5 pairs. Concurrency-8 long-stream CPU improved in 5/5 pairs, but its paired-median reduction was only 3.62%. These are descriptive observations, not proof of significance, equivalence, or an inherent causal slowdown. The candidate does not meet the predeclared rule and adds no production complexity to this PR.

Retained instead: `88fdd66` fixes review comment 4100712069, making the validated cache and immutable binary/driver/output paths authoritative after caller options. Three new regression cases fail against the original script and pass after repair, including separate, equals and abbreviated arguments. All 40 current Python harness tests pass locally. New differential Go tests preserve 1,038 renderer events, four failing/short-writing cases, and 2,060 normalization cases. They pass on retained production code with three race repetitions. Deliberately omitting flush or changing normalization makes the respective negative control fail. The measured zero-allocation assertion is **not** retained, because the implementation earning it was rejected.

## Workload and qualification

The [plan was posted before A/B measurement](https://github.com/Laisky/one-api/pull/427#issuecomment-5826152136). Five alternating pairs per cell; concurrency 8/64; long unpaced 1,024 x 128-byte chunks and 256 requests/run; short 32 x 128-byte chunks paced at 2 ms and 512 requests/run. Fresh real gateway/SQLite per trial, one tenant/token/channel, warm-up of 16 requests. Authentication, quota, exact token counting, logging, tracing, per-event flushing and durable usage stay enabled. Only the enabled global rate-limit ceilings are raised. Both independently built variants pass normal/CRLF/fragmented streams, expected failure for corrupt/truncated/malformed controls, invalid authentication and eight-client upstream cancellation cleanup. Usage settlement remains a ten-second hard gate. Setup, qualification and warm-up are excluded from the 15,360 requests.

Local host: Linux AMD EPYC 9V74, four-core cgroup quota, 4 GiB memory limit, GOMAXPROCS=2 independently for gateway/mock/client. All processes share the host without CPU-affinity isolation. No other local performance, build or profiling workload ran during A/B. Gateway CPU includes settlement; RSS is sampled every 20 ms, not a kernel high-water mark. Driver peak RSS remains unmeasured. Short run lengths and five repetitions limit inference; large adverse outliers remain in the ledger. No new fixed-arrival-rate capacity test, production SLO, slow-client soak or multi-tenant result is claimed.

## Why this candidate was tested

A separate 512-request, concurrency-8 diagnostic of the immutable `299deff` gateway attributed 65.49% of sampled CPU cumulatively to ordinary token encoding and 15.32% to `render.StringData`. Allocation sampling also identified normalization/rendering copies. Profiles are diagnostic and are not pooled with the A/B trials.

The candidate reuses normalized strings and existing `data: ` prefixes, and adds a string-specific event sharing the existing escaping, error and header implementation. Generic CustomEvent formatting stays unchanged. It introduces no batching, delayed flushing, asynchronous buffering, tokenizer changes or billing shortcuts. This is distinct from the previously rejected standalone formatting shortcut and flush-coalescing candidate.

Five microbenchmark repetitions on a 512-byte renderer payload changed 1,232 B/op and four allocations to 16 B/op and one allocation. Canonical normalization changed 480 B/op and one allocation to zero. Those allocation counts justified the full E2E experiment, **not adoption**. They are not live-memory reductions, and the end-to-end RSS results do not establish lower memory use.

## Exact-source limitations

Public Git clone was unavailable in the working runtime. Sources were recovered from the immutable `21dc0af` artifact, with later retained user/channel busy-retry changes whose Git blob IDs were checked against `299deff`. The 299deff harness was recovered separately. go.mod/go.sum match the recovered vendored dependency bundle. Local recovery commits are not represented as original GitHub commits. Both A/B binaries were freshly built locally with Go 1.27.1, `-p 2 -mod=vendor -trimpath -buildvcs=false`, identical embed assets, CGO enabled and GOAMD64=v1. No historical CI binary was substituted for either A/B variant.

Local baseline source: `9bfc8f2` (reconstructed retained production). Local measured candidate: `e8c1f6b85045d3ac096adf3807b20f5ec9b3b1b0`; subsequent local `13edaf521933742ea7faef46e39c497c3b666ab0` restores only the already-existing, byte-verified network-delivery test. Binary hashes and recovery artifacts are in manifest.json. An initial local build failed because a detached worktree lacked ignored embed assets; it was repaired before either qualification or A/B. That setup failure is not an omitted performance trial.

# Paired streaming E2E comparison

Values are medians across independent runs. Changes are medians of paired percentage changes, not ratios of pooled requests.
Small repeat counts are descriptive evidence, not proof of statistical significance or production capacity.

| Profile | Concurrency | Pairs | RPS baseline / candidate | Paired RPS change | CPU ms/request baseline / candidate | Paired CPU change | p95 completion ms baseline / candidate | RSS MiB baseline / candidate |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | 5 | 101.77 / 101.23 | -0.50% | 6.02 / 6.07 | +1.97% | 81.70 / 83.78 | 298.90 / 295.57 |
| paced | 64 | 5 | 259.79 / 259.79 | +3.30% | 5.96 / 5.74 | -3.75% | 513.74 / 483.42 | 343.01 / 348.33 |
| saturated | 8 | 5 | 41.63 / 41.47 | +2.19% | 47.30 / 45.70 | -3.62% | 278.41 / 287.04 | 345.58 / 344.80 |
| saturated | 64 | 5 | 33.76 / 35.73 | +11.64% | 49.18 / 49.22 | +0.71% | 4448.18 / 3382.31 | 384.38 / 390.21 |

## First-content latency and repeatability

| Profile | Concurrency | p95 TTFT ms baseline / candidate | Paired RPS change range | Paired CPU change range |
| --- | ---: | ---: | ---: | ---: |
| paced | 8 | 7.71 / 8.52 | -2.04% to +0.35% | -2.84% to +4.30% |
| paced | 64 | 351.21 / 312.47 | -9.53% to +7.95% | -11.90% to +12.76% |
| saturated | 8 | 77.74 / 98.19 | -3.94% to +3.21% | -5.81% to -2.99% |
| saturated | 64 | 2117.45 / 973.04 | -20.36% to +21.47% | -5.02% to +4.90% |

## Streaming continuity

Each request records its largest gap between observed nonempty content deltas. The metric below is the p95 of those request maxima, not a pooled token-gap percentile. First-content latency remains separate.
Zero-baseline percentage changes are undefined; absolute paired differences remain available.

| Profile | Concurrency | p95 request-max gap ms baseline / candidate | Paired absolute change ms | Paired change |
| --- | ---: | ---: | ---: | ---: |
| paced | 8 | 4.56 / 5.51 | +0.87 | +18.72% |
| paced | 64 | 45.13 / 38.22 | +3.72 | +31.79% |
| saturated | 8 | 54.74 / 53.75 | -3.94 | -7.40% |
| saturated | 64 | 256.83 / 360.13 | -20.59 | -5.41% |

## Audit and reproduction

`runs.csv` retains every trial in execution order, using lossless decimal values. `manifest.json` binds it to the complete summary, binary identities and this report. `rejected-production.patch` is stored evidence, **not applied code**. The conversation evidence ZIP contains full per-request JSON, qualification, profiles, validation logs and source/build manifests. Neither the raw per-request files nor profiles are claimed to be committed repository files.

From the repository, with the evidence archive extracted, run:

```sh
python3 tests/stream-perf/results/20260925-copy/verify.py \
  --summary /path/to/evidence/raw/paired/summary.json
```

The verifier checks recorded file hashes, exact ledger-to-summary equality, complete qualification/matrix, immutable binary/driver identities, paired usage, published totals and the rejection decision. Six local audit self-tests include incomplete/invalid evidence and synthetic acceptance/rejection controls; these archive-only tests are not additional repository-CI tests.

To perform a **new** comparison, create a detached local worktree from `88fdd66985af1ff7bfce8b6998c1923f352e3b11`, apply the stored production patch, commit it locally, and run the existing `compare.sh` with that original revision as baseline and the registered five-pair/four-cell options. Pin the same Go toolchain and prepare its cache. Normal rebuilds may differ in VCS/embed/dependency packaging from the recovered offline binaries and must record their own hashes; never relabel the resulting source or absolute measurements as this study. Do not create another remote branch.

This iteration is complete: a reproducible candidate was tested and rejected, while the review repair and behavior guards were retained. Remaining work should investigate the dominant exact-token-counting cost or stabilize the identified tail-latency mechanisms without weakening accounting or introducing synthetic-only cache gains. This experiment does not prove a global performance optimum.
