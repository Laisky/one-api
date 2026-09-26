# Exact ordinary-token counting: two rejected candidates

Neither candidate is applied to production. Both complete studies are retained separately; each has 40 trials, 15,360 verified requests, zero failures/drops and matching paired durable usage. No performance improvement is adopted in this checkpoint.

## Design and correctness

The traversal candidate avoids the all-match index matrix, duplicate whole-input rune slice and final token-ID array. It keeps the original regex and generic byte-pair merge kernel. The stack candidate adds bounded scratch storage for pieces of at most 128 bytes, with the same merge order and a heap fallback for larger pieces. Neither caches user inputs, changes dictionary/rank/regex behavior, batches SSE flushes, approximates tokens, disables logs, or bypasses quota/usage accounting.

Private upstream state required a narrowly pinned internal copy of tiktoken-go v0.1.8 for these experiments. Its five original Go files and MIT license remain byte-identical. The original external dependency is an independent differential oracle. The copy and all count-only production changes are rejected along with the candidates; they are not an added dependency maintenance burden on this PR.

Differential tests compare complete token sequences, exact counts and decoded text for 1,293 inputs in each of cl100k_base and o200k_base, including arbitrary/invalid UTF-8, special-looking strings and long inputs. Sixteen workers each process 32 distinct inputs per encoding. Seven custom-encoding cases and the gateway's existing fallback/cache tests also pass. Stack tests add 13 length boundaries with four patterns and 512 random pieces per encoding, plus a zero-allocation assertion for a 95-byte piece. Each final candidate passed three focused race repetitions. All 40 existing Python tests pass. Gated real-HTTP delivery checks pass. An intentionally wrong count fails the independent oracle tests. These rejected-candidate tests are in the evidence archive, not claimed as new repository-CI coverage.

## Profile, setup and predeclared decisions

A separate 512-request baseline diagnostic attributes 61.41% cumulative sampled CPU and 66.53% cumulative sampled allocated bytes to ordinary token encoding. These are profiling observations, not the A/B windows. Preliminary traversal microbenchmarks reduced a long-text fixture from roughly 9.45 MB/op to 6.35 MB/op; the final stack variant measured about 6.02 MB/op. Allocation volume is not live memory or RSS.

The plans were posted before their respective A/B measurements: [traversal plan](https://github.com/Laisky/one-api/pull/427#issuecomment-5831869546), [pre-measurement refinement](https://github.com/Laisky/one-api/pull/427#issuecomment-5831955185), and [stack plan after the complete traversal rejection](https://github.com/Laisky/one-api/pull/427#issuecomment-5832060349). The existing five-pair/four-cell acceptance thresholds were unchanged.

Both variants independently pass normal/CRLF/fragmented streams, expected corrupt/truncated/malformed rejection, invalid authentication and eight-client upstream cancellation cleanup. Durable usage settlement remains a ten-second gate. The benchmark uses 16 warm-ups, fresh gateway/SQLite, one account/token/channel and no paid provider requests. Long streams: 1,024 content chunks x 128 UTF-8 bytes, unpaced, 256 requests/run. Short: 32 x 128 bytes, 2 ms/chunk, 512 requests/run. A/B order alternates each repeat. The two studies use independent qualifications and output directories against the same original baseline binary; the stack study is not compared to the rejected traversal candidate.

Local Linux AMD EPYC 9V74, four-core cgroup quota, 4 GiB RAM, GOMAXPROCS=2 for each gateway/mock/client process; CPU-affinity isolation was not used. Builds, profiles and other local performance studies were excluded from timed windows. Gateway CPU includes settlement; RSS is sampled every 20 ms, not a kernel high-water mark. Driver peak RSS is unmeasured. No capacity/SLO, long-soak, multi-tenant, slow-client or cross-protocol result is claimed.

Source was reconstructed from the immutable 21dc0af artifact and two later 299deff production changes, verified by Git blob IDs. Local baseline 4c86510a1ad1b28311a6c1feca6f3488c42ebdf9 and candidate IDs in the manifest are recovery commits, not original GitHub commits. Both baseline and candidates were freshly compiled with the same Go 1.27.1, `-p=2 -mod=vendor -trimpath -buildvcs=false`, CGO, dependencies and API-only embeds. The existing compare.sh was initially recovered from an obsolete source; six recovery tests exposed this and it was replaced with the exact 13cd2ee blob before measurement. That setup failure is preserved, not described as a production defect or omitted trial.

Changes below are medians of the five paired percentages, not percentages calculated from the independent absolute medians. All adverse cells and raw samples remain available. Small repetition counts and the shared host limit causal/statistical inference. Rejection means the operational rule was not met, not proof that the implementation inherently causes every observed regression.

## traversal: rejected

| Profile | Concurrency | Paired RPS | CPU/request | P95 first content | P95 completion | RSS | P95 request-max gap |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | -0.07% | -3.98% | +0.26% | +0.05% | -0.05% | -10.91% |
| paced | 64 | +2.61% | -0.88% | +2.72% | +3.89% | +0.76% | -2.38% |
| saturated | 8 | +4.24% | -0.68% | -5.40% | -9.86% | +0.22% | -8.97% |
| saturated | 64 | +3.07% | -0.88% | -12.10% | -6.63% | +1.97% | -18.23% |

Rejection grounds: no long-stream metric meets both material benefit and 4/5 repeatability gates.

# Paired streaming E2E comparison

Values are medians across independent runs. Changes are medians of paired percentage changes, not ratios of pooled requests.
Small repeat counts are descriptive evidence, not proof of statistical significance or production capacity.

| Profile | Concurrency | Pairs | RPS baseline / candidate | Paired RPS change | CPU ms/request baseline / candidate | Paired CPU change | p95 completion ms baseline / candidate | RSS MiB baseline / candidate |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | 5 | 100.80 / 101.02 | -0.07% | 6.88 / 6.60 | -3.98% | 84.04 / 84.07 | 284.12 / 276.79 |
| paced | 64 | 5 | 231.91 / 232.29 | +2.61% | 6.84 / 6.70 | -0.88% | 570.87 / 596.30 | 324.27 / 327.61 |
| saturated | 8 | 5 | 37.46 / 37.84 | +4.24% | 51.48 / 51.45 | -0.68% | 366.35 / 330.21 | 323.96 / 323.51 |
| saturated | 64 | 5 | 29.16 / 30.05 | +3.07% | 53.20 / 52.73 | -0.88% | 5368.34 / 5151.45 | 360.89 / 366.92 |

## First-content latency and repeatability

| Profile | Concurrency | p95 TTFT ms baseline / candidate | Paired RPS change range | Paired CPU change range |
| --- | ---: | ---: | ---: | ---: |
| paced | 8 | 9.57 / 9.90 | -0.21% to +1.40% | -6.70% to +1.42% |
| paced | 64 | 393.74 / 406.49 | -3.05% to +19.16% | -12.72% to +0.95% |
| saturated | 8 | 125.21 / 118.44 | -9.88% to +18.12% | -8.63% to +7.32% |
| saturated | 64 | 4524.16 / 3716.50 | -20.40% to +10.21% | -6.39% to +4.07% |

## Streaming continuity

Each request records its largest gap between observed nonempty content deltas. The metric below is the p95 of those request maxima, not a pooled token-gap percentile. First-content latency remains separate.
Zero-baseline percentage changes are undefined; absolute paired differences remain available.

| Profile | Concurrency | p95 request-max gap ms baseline / candidate | Paired absolute change ms | Paired change |
| --- | ---: | ---: | ---: | ---: |
| paced | 8 | 5.72 / 5.07 | -0.62 | -10.91% |
| paced | 64 | 43.26 / 11.77 | -0.73 | -2.38% |
| saturated | 8 | 62.43 / 62.70 | -5.94 | -8.97% |
| saturated | 64 | 94.99 / 66.13 | -12.79 | -18.23% |


## stack: rejected

| Profile | Concurrency | Paired RPS | CPU/request | P95 first content | P95 completion | RSS | P95 request-max gap |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | -0.73% | -0.56% | +10.87% | +1.38% | +13.71% | +16.97% |
| paced | 64 | +3.08% | -1.96% | -9.42% | -12.09% | -9.96% | +52.88% |
| saturated | 8 | +6.41% | -5.68% | -8.25% | -7.42% | -2.10% | -3.76% |
| saturated | 64 | -12.86% | +1.68% | +76.74% | +63.67% | -4.55% | -85.34% |

Rejection grounds: paced/c8: RSS regressed; paced/c64: max_inter_content_gap_p95_ms exceeds both latency limits; saturated/c64: ttft_p95_ms exceeds both latency limits; saturated/c64: completion_p95_ms exceeds both latency limits.

# Paired streaming E2E comparison

Values are medians across independent runs. Changes are medians of paired percentage changes, not ratios of pooled requests.
Small repeat counts are descriptive evidence, not proof of statistical significance or production capacity.

| Profile | Concurrency | Pairs | RPS baseline / candidate | Paired RPS change | CPU ms/request baseline / candidate | Paired CPU change | p95 completion ms baseline / candidate | RSS MiB baseline / candidate |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | 5 | 100.89 / 99.79 | -0.73% | 6.95 / 6.91 | -0.56% | 83.43 / 85.87 | 279.52 / 280.19 |
| paced | 64 | 5 | 232.49 / 235.89 | +3.08% | 6.86 / 6.54 | -1.96% | 625.11 / 533.55 | 327.23 / 318.36 |
| saturated | 8 | 5 | 37.54 / 38.67 | +6.41% | 51.45 / 49.92 | -5.68% | 328.36 / 317.58 | 325.75 / 319.19 |
| saturated | 64 | 5 | 34.42 / 30.13 | -12.86% | 53.20 / 52.15 | +1.68% | 3135.87 / 4413.80 | 372.74 / 357.48 |

## First-content latency and repeatability

| Profile | Concurrency | p95 TTFT ms baseline / candidate | Paired RPS change range | Paired CPU change range |
| --- | ---: | ---: | ---: | ---: |
| paced | 8 | 9.54 / 10.62 | -3.08% to +0.04% | -5.41% to +12.87% |
| paced | 64 | 447.61 / 358.07 | -6.55% to +8.24% | -14.16% to +11.22% |
| saturated | 8 | 116.66 / 104.30 | -9.35% to +10.32% | -7.21% to +0.38% |
| saturated | 64 | 2245.22 / 3581.50 | -22.76% to +10.94% | -4.77% to +4.63% |

## Streaming continuity

Each request records its largest gap between observed nonempty content deltas. The metric below is the p95 of those request maxima, not a pooled token-gap percentile. First-content latency remains separate.
Zero-baseline percentage changes are undefined; absolute paired differences remain available.

| Profile | Concurrency | p95 request-max gap ms baseline / candidate | Paired absolute change ms | Paired change |
| --- | ---: | ---: | ---: | ---: |
| paced | 8 | 5.34 / 5.92 | +0.84 | +16.97% |
| paced | 64 | 12.27 / 16.59 | +5.74 | +52.88% |
| saturated | 8 | 64.84 / 59.91 | -2.66 | -3.76% |
| saturated | 64 | 468.26 / 64.61 | -348.68 | -85.34% |

## Audit and reproduction

Use `gzip -dc runs.csv.gz > runs.csv` to inspect the lossless ledger as text.

`runs.csv.gz` contains all 80 trials in each study's actual execution order. `manifest.json` binds workload, qualification, source/binary identities, raw-summary hashes and expected decisions. The accompanying conversation evidence archive contains both complete summaries, every per-request JSON file, independent qualification, profiles, test logs, and unapplied candidate patches. Raw samples, binaries and rejected production code are not committed by this checkpoint.

Run `python3 tests/stream-perf/results/20260925-token-count/verify.py --evidence /path/to/extracted/archive` to require lossless ledger equality and recompute both decisions. The verifier does not pool the studies. Operational acceptance needs a repeat-consistent >=5% long-stream benefit, no >5% short-stream throughput/CPU regression, no paired-median latency increase exceeding both 10% and 5 ms, and no >10% paired-median RSS increase. A throughput gain alone cannot excuse a latency or memory regression.

These completed rejections do not establish a global optimum. Subsequent candidates must be declared separately and measured without overwriting these results.
