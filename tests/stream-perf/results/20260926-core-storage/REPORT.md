# PR427: unused vocabulary index — rejected after complete measurement

## Decision and scope

**Do not apply the runtime change.** This is a new constructor-memory experiment, not a repetition of the rejected BPE scratch/output study. The [plan was posted before measurements](https://github.com/Laisky/one-api/pull/427#issuecomment-5841256299). Removal of the unread `sortedTokenBytes` index saves memory, but fails the fixed no-regression rules. Only behavior tests, opt-in diagnostic probes, reproducible evidence and this rejection are retained in the PR. The runtime index remains in production.

The independent o200k process probes retain 6,860,568 fewer bytes (6.54 MiB), and constructor-probe allocated bytes decrease 49.44%. All 20 gateway pairs reduce peak RSS. However, long/c8 throughput regresses 5.10%; long/c64 throughput regresses 17.41%, CPU/request increases 9.70%, TTFT increases 93.27% / 1,384.01 ms, and completion increases 42.13% / 1,026.49 ms. Five separate gates fail. No selected-cell rerun, GC tuning or revised threshold is used to rescue the candidate. These observations establish rejection under this operational rule, not statistical significance or a universal causal slowdown.

## Resumption and exact-source boundary

The resumption started at a95e9f14593f4373d914e02e340ff5ee56cc7456. Recovery note 22aac043d875caf592feb80e07f00ec1f3894e5c preserves the earlier BPE scratch rejection and the fact that its raw archive/candidate identities were unavailable after interruption. Its 40 trials were not rerun or fabricated. Eight historical auxiliary branch tips were unchanged. No new remote branch or production deployment was created.

This experiment's baseline is the retained sparse-limiter production de6985d9e4b4a966fe7c875b54273ee0339d3ec2, including the public loopback listener and existing 4 KiB schedule. Public source could not be cloned directly in the runtime. Existing source/vendor/Go/tokenizer artifacts were recovered and the 954 historical production/dependency/embed inputs were verified before replaying exact accepted byte-budget/overflow/sparse-limiter changes. Main/listener and current harness blobs were independently matched to public source. Recovered Git commits are local identities, not the original GitHub commits. Both binaries were built in this session with Go 1.27.1, the same vendor/embed inputs and `-p=2 -mod=vendor -trimpath -buildvcs=false`.

The sole candidate production difference is removal of the unconsumed `sortedTokenBytes` field, its copied/sorted vocabulary, and now-unused `bytes`/`sort` imports in `internal/tokenizer/core_bpe.go`. Encoder/decoder maps, regexes, ranks, BPE merges, complete token IDs, finite-timeout fallback, 4 KiB yields, accounting and immediate SSE flushes stay unchanged. The original external module remains the independent oracle. The unapplied patch records the candidate; it is not an adopted implementation.

Baseline binary: `b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46`.
Candidate binary: `76c651c0d03174973f34d59d31117665958b53da88983f7b731fa748065703ff`.
Shared driver: `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`.
Local candidate: `9f9a42908e63fa85adecc44f4d19beedb20d4fbb`.

## Workload and gates

Four cells, five alternating pairs each: concurrency 8/64; long 1,024 x 128-byte content chunks, unpaced, 256 requests/run; short 32 x 128-byte chunks at 2 ms, 512 requests/run. A fresh real gateway/SQLite fixture, one tenant/token/channel and 16 excluded warm-up requests per run. Gateway/mock/client GOMAXPROCS 2/2/2, default GC, enabled exact counting, auth/quotas/logs/traces and durable statistics. Only existing enabled global rate ceilings are raised to 10,000,000. Loopback binding is explicit. No other local test/build/profile workload overlapped timed runs.

The complete unprofiled matrix has **40 trials / 15,360 requests, zero failures/drops and exact paired durable usage**, with separate positive/negative stream, auth and eight-client cancellation qualification. Settlement remains a hard ten-second gate. The initialization-memory benefit gate requires >=4 MiB lower retained o200k heap, >=20% fewer constructor allocated bytes and lower median RSS in both long cells with at least 4/5 favorable pairs. Safety gates reject >5% median CPU or RPS regression in any cell, >10% RSS growth, and latency or content-gap regression exceeding BOTH 10% and 5 ms. This memory criterion was set before candidate measurement, not substituted afterward.

Host: Linux AMD EPYC 9V74; four-core cgroup quota, five-CPU affinity and 4 GiB memory. Gateway CPU includes settlement, RSS sampled at 20 ms. Driver peak RSS is unmeasured in A/B. Shared-host scheduling, GC and short run lengths limit causal interpretation; five pairs are descriptive evidence, not a confidence interval or production capacity guarantee.

## Complete paired results

Each percentage is the median of five within-pair percentage changes, not a ratio of separate medians. Lower is better except RPS. The gap metric is p95 of each request's maximum client-observed interval between nonempty content deltas, not pooled inter-token latency or a wire-level clock.

| Profile | Concurrency | RPS | CPU/request | Peak RSS | P95 first content | P95 completion | P95 request-max gap |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| saturated | 8 | -5.10% | +2.35% | -18.57% | -2.53% | +4.50% | -4.49% |
| saturated | 64 | -17.41% | +9.70% | -15.51% | +93.27% | +42.13% | -17.66% |
| paced | 8 | -0.36% | +0.93% | -17.81% | -3.48% | +2.08% | +5.12% |
| paced | 64 | +1.23% | -1.59% | -15.51% | +0.53% | -9.81% | -77.81% |

All 20 paired RSS observations improve. Long/c8 sampled peak RSS medians are 171.04 / 139.50 MiB; long/c64 216.13 / 182.29 MiB. This memory result does not excuse the long/c64 latency/throughput regression. Full signed pair changes and separate absolute medians are reproduced by the verifier; no adverse pair was omitted.

## Independent constructor measurements

Five alternating isolated-process pairs per encoding use dictionary loading before the measurement and forced GC before/after construction; no user text is cached. o200k retained heap medians are 13,855,200 / 6,994,632 bytes. These measurements are not sampled whole-gateway RSS. Separately, five constructor-only benchmark repetitions (three operations each) exclude dictionary loading: o200k medians are 13,877,792 / 7,017,144 B/op and 201,004 / 1,002 allocations/op. The probe and benchmark agree on storage removal, not on HTTP acceptance. cl100k measurements and all probe records are retained in the compressed study.

## Separate high-load heap diagnostics

Both binaries independently passed exact delivery/usage qualification and completed 8,192 requests with equal durable usage, GOMAXPROCS 3/2/2, concurrency 32, 30-second warm-up and 60-second heap observation. These are two additional diagnostic runs, never pooled with the A/B. The unchanged public `profile_run.py` and `profile_support.py` were byte-verified, validate actual loopback sockets and record helper CPU separately.

| Variant | Mean gateway / effective machine CPU | Intervals >=50% | Coverage | Half-window upstream-rate drift |
| --- | ---: | ---: | ---: | ---: |
| baseline | 60.30% | 98.33% | 59.81 s | +5.64% |
| candidate | 60.36% | 100.00% | 59.84 s | -7.26% |

Both load windows qualify. Upstream progress is a proxy, not client completion timestamps; the independently recomputed half-window rate drift is below 15%. The public load gate itself checks CPU coverage, not drift, so these are separately reported checks.

Post-GC sampled live heap totals are 64,975,117 / 43,670,597 bytes. `NewCoreBPE` flat live-heap attribution falls from 21,719,879 to 10,222,724 bytes. Sampling, concurrent activity and GC timing make this different from an exact heap census or the isolated 6.54 MiB estimate. Cumulative allocation still has BPE merging at 30.62% / 30.87%; these profiles are snapshots at the end of each window, not normalized per-request allocation deltas. They cannot reverse the rejected A/B result or establish GC as the cause of slower runs. No new CPU profile was collected in this heap-only study.

## Validation and reproduction

Complete tokenizer race validation, three focused stream/render/SSE race repetitions, and vet passed for the candidate. The added independent-oracle tests cover 519 ordinary inputs per encoding, exact decode/special-token behavior, custom/empty/invalid constructor inputs and 64 concurrent inputs. The candidate-only allocation guard fails against the retained baseline as expected; it is present only in the unapplied candidate patch. The retained behavior/probe/benchmark file excludes that guard and passes three race repetitions plus vet on unchanged production. A deliberately corrupted decoder is separately checked as a negative control. No public-head CI result is inferred from local tests; inspect the exact publication head.

The repository retains the full-precision 40-run `runs.csv`, diagnostic-window summary, registered plan and unapplied production patch. These run-level records alone are not a fresh audit of raw requests. The separately delivered archive contains `study.json.gz` with the original complete run summary, all isolated probes, parsed constructor results and identities without rounding; `verify.py` checks its checksum, qualification, exact paired usage and rejection gates. The archive also contains individual request JSON, raw diagnostic counters and heap profiles, scripts, source manifests and logs. It excludes compiler/gateway binaries, tokenizer dictionaries, vendor, databases and credentials.

After extracting `one-api-pr427-core-storage-evidence.zip`, run `python3 -B verify_all.py`. It checks every manifest-covered file, all 15,360 A/B and 16,384 diagnostic request percentiles, paired usage and diagnostic CPU attribution/progress intervals. Eight archive-only audit self-tests cover synthetic acceptance/rejection, missing/reordered rows, wrong binaries/usage, corruption, invalid numerics and percentile checks. They are not claimed as additional repository-CI tests, and do not reinterpret missing historical BPE scratch files as new evidence.

This bounded experiment is complete and rejected. The retained implementation is unchanged. Further work should first examine allocation-rate/GC/scheduler interaction with client-observed tail latency under matched load; that is a hypothesis, not an established bottleneck diagnosis or a new accepted optimization. Do not repeat this completed matrix simply because the conversation stops.
