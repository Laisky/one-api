# Streaming E2E follow-up: exact token counting and PR fixes

Date: September 24, 2026. This report supplements, rather than replaces, the [original study](../20260924/REPORT.md).

## Decision and delivery

Retain the `EncodeOrdinary` token-counting path. It removes unused special-token scanning while preserving the complete token sequence produced by `Encode(text, nil, nil)`. No stream batching, flush policy, quota checks, logging or billing was disabled. The predeclared initial acceptance rule was met: long-stream concurrency 8 improved paired throughput by a median 9.73% and reduced CPU/request by 9.05%; all five pairs improved both. Other cells did not cross the predeclared median regression limits. This is not a universal latency or memory improvement.

The published production-code revision is **`21dc0af145560e3ff2df01bcc8383212b8d2e628`**. Its full [CI run 36068214241](https://github.com/Laisky/one-api/actions/runs/36068214241) passed every applicable required check, including all five Go shards, race/coverage completeness, static checks and the dependency vulnerability scan. The previously reported E2E host-requirements review thread was resolved. Subsequent evidence-only commits do not change the measured production code; consult current PR checks for their status.

Two correctness/portability fixes accompany the optimization:

- Local smoke tests explicitly skip unsupported hosts, missing Python, `-short`, and an unset tokenizer cache. GitHub Actions or `ONEAPI_REQUIRE_STREAM_E2E=1` fail instead of silently skipping required coverage. A supplied cache is verified offline; missing or corrupt assets fail without implicit downloads. Explicit setup can download only the pinned, checksum-verified dictionaries. Failed atomic publication cleans its temporary file.
- The GPT-5.2 alias and dated snapshot now advertise canonical `none` reasoning and its documented default. This repairs the three original sampling-policy failures without changing GPT-5, Pro or Codex contracts. See the [official model contract](https://developers.openai.com/api/docs/models/gpt-5.2). Existing mapped Chat/Responses tests and new snapshot/default/negative-model cases pass; failures were not bypassed.

## Environment and immutable controls

Measurements ran locally, **not on the GitHub build runners**: AMD EPYC 9V74, five visible CPUs, a four-core cgroup quota, 4 GiB memory, and `GOMAXPROCS=2` independently for gateway, mock and client. Processes share the host without CPU-affinity isolation. The initial report used a different reported CPU model; do not compare its absolute throughput to this session or attribute that difference to this optimization.

All binaries use Go 1.27.1 and `go build -trimpath`. Baseline is the previous PR code `36829cfdfddb29f6a7a2d4e20a42eb02b0f3ef4c`, already containing the earlier Builder/SSE optimizations. Initial candidate build is `ada1af2dd8f793a64360a6d1c6f5ecd87a9344a4`. The subsequent code-head build is exactly `21dc0af`; the differences from the initial candidate are test formatting, documentation and removal of the temporary build workflow, not production logic. Its embedded VCS metadata reports `vcs.modified=false`.

The baseline binary SHA-256 is `30f0fc305f47fca20652b1e769bcadd05f584ea8c4744534374a128a16f3563d`; initial candidate is `1f9d2d11fd3668f74f68ea603f956be926d86964d0c7f54913e9831590e3c544`; code-head candidate is `f74c0ed56a2bd37fefe94edc5ceee1c484fd40e2b8077c9d701ffcb86a8cddbc`. All experiments use the same driver SHA-256 `409b9820553395cce0c1714bbdb4d3805905815926dfcc19cece90262ef74dc1`. Build artifact IDs and raw-summary hashes are in [manifest.json](manifest.json).

Each trial starts a fresh real gateway and SQLite database with default WAL, pools, tracing, logging and quota/billing enabled, one account/token/channel and `gpt-4o-mini`. Sixteen warm-up requests and their settled billing precede each timed trial. Only the still-enabled API/relay rate-limit ceilings are raised. No other local performance study or build ran concurrently. CPU/request includes billing settlement; RSS is sampled every 20 ms, not the exact kernel high-water mark.

## Initial five-pair A/B matrix

Long/saturated: 1,024 content chunks of 128 UTF-8 bytes, no upstream pacing, 256 measured requests/run. Short/paced: 32 chunks of 128 bytes, 2 ms per chunk, 512 requests/run. Five alternating A/B pairs at each concurrency/profile: **40 trials, 15,360 verified requests, zero failed or dropped requests**. Each pair has identical durable request and quota totals.

Values are medians across runs. Percentage changes below are **medians of matched-pair percentage changes**, not percentages calculated from the two independent medians. These small repeat counts are descriptive measurements, not confidence intervals. Requests within a run are not independent experiment repetitions.

| Profile | Concurrency | RPS baseline / candidate | Paired RPS change | CPU ms/request baseline / candidate | Paired CPU change | Sampled RSS MiB baseline / candidate |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | 100.33 / 100.70 | +0.11% | 6.99 / 6.88 | -1.71% | 354.44 / 293.59 |
| paced | 64 | 232.91 / 232.53 | +1.47% | 6.66 / 6.74 | -0.78% | 322.90 / 321.12 |
| saturated | 8 | 31.61 / 35.81 | +9.73% | 58.32 / 52.81 | -9.05% | 327.85 / 325.79 |
| saturated | 64 | 26.55 / 27.72 | +3.00% | 58.95 / 56.21 | -4.99% | 438.35 / 363.02 |

| Profile | Concurrency | p95 TTFT ms baseline / candidate | Paired TTFT change | p95 completion ms baseline / candidate | Paired completion change |
| --- | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | 9.64 / 9.23 | -4.72% | 84.57 / 83.91 | +0.38% |
| paced | 64 | 404.31 / 394.66 | -3.10% | 579.75 / 576.41 | +5.97% |
| saturated | 8 | 175.85 / 126.31 | +3.16% | 412.81 / 370.36 | -3.11% |
| saturated | 64 | 3890.91 / 3972.31 | +4.34% | 5272.97 / 5199.16 | -3.21% |

Every long-stream pair improved throughput and CPU/request. Concurrency-8 paired throughput gains range from +1.00% to +17.47%, with CPU reductions of 4.60% to 12.26%. Concurrency-64 gains range from +0.51% to +11.84%, with CPU reductions of 4.30% to 11.02%. Short-stream throughput is essentially unchanged in this initial experiment. Adverse latency observations remain visible: paced concurrency 64 has +5.97% median paired p95 completion, and one long concurrency-8 pair has +14.54% p95 completion. The two independently calculated medians can even move differently from the median paired change; neither is substituted for the other.

## Separate published-code confirmation

The exact production code `21dc0af` was rebuilt and independently requalified, then tested against the same baseline at concurrency 64: three alternating pairs per profile, same request counts, **12 trials and 4,608 verified requests**. This is a separate experiment, not pooled with or substituted for the initial matrix.

| Profile | RPS baseline / candidate | Paired RPS change | CPU ms/request baseline / candidate | Paired CPU change | p95 completion ms baseline / candidate | Paired completion change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 239.25 / 261.87 | +10.53% | 6.31 / 5.94 | -5.52% | 585.39 / 462.88 | -1.45% |
| saturated | 27.80 / 28.79 | +10.85% | 59.61 / 54.06 | -7.99% | 4873.07 / 4962.68 | -10.97% |

All six pairs improve throughput and CPU/request. However, long-stream repetitions include **22.34%** worse p95 completion, **29.76%** worse p95 TTFT, and, in a different pair, **21.80%** higher sampled RSS. The median paired long-stream RSS change is only -0.23%; the larger initial RSS improvement does not reproduce consistently. No universal tail-latency, RSS or short-stream speedup is claimed.

## Calibration and offered-rate capacity observations

Three direct mock/client repetitions per profile/concurrency, 12 trials and 4,608 requests, establish apparatus headroom. Median direct throughput is 177.67 RPS for long concurrency 8, 164.23 for long concurrency 64, and 734.37 for paced concurrency 64. Paced concurrency 8 reaches 107.80 RPS and is naturally pacing-bound. Do not subtract independently measured percentiles to estimate gateway latency.

The published-code gateway then received the paced profile at four offered rates, concurrency/admitted-population bound 64, 1,024 scheduled arrivals/run, three repetitions/rate. All qualification checks are repeated for every rate.

| Offered RPS | Verified completed / offered | Admission drops | Median successful RPS | Median p95 completion ms | Worst run p95 completion ms | Median scheduling-inclusive p95 ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 3072 / 3072 | 0 | 99.36 | 81.60 | 84.05 | 82.02 |
| 200 | 3072 / 3072 | 0 | 197.14 | 90.71 | 93.01 | 91.35 |
| 300 | 2765 / 3072 | 307 | 248.10 | 511.44 | 550.61 | 511.45 |
| 500 | 1696 / 3072 | 1376 | 253.01 | 512.22 | 568.53 | 512.26 |

**200 offered RPS is the highest tested zero-drop rate in all three bounded trials**, not a sustained production capacity or SLO. At 300/500, the bounded client sheds arrivals before HTTP. These are **load-generator admission drops, not gateway HTTP failures**. Report them alongside successful-only latency. No admitted request failed stream verification, and every completed gateway request matches durable billing. The earlier report's 100-RPS observation is from another session/CPU; this does not prove capacity doubled.

## Validation, mechanism and audit

A separate CPU profile attributes 65.66% of sampled gateway CPU cumulatively to token counting in the long-stream fixture. Profiling is diagnostic and excluded from timed comparisons. The one-line performance change avoids the special-token scan because no special tokens were enabled in the old call. See the pinned [tokenizer API](https://github.com/pkoukk/tiktoken-go/blob/v0.1.8/tiktoken.go) and [implementation](https://github.com/pkoukk/tiktoken-go/blob/v0.1.8/core_bpe.go).

The differential regression compares complete token sequences and production counts over 1,295 inputs for each of two encodings: Unicode, combining characters, emoji, special-token literals, invalid UTF-8, every single byte, deterministic random bytes and long inputs. Shared-encoder concurrent counts and approximate/nil fallback checks pass. The focused code-head race run covers SSE, rendering, OpenAI adaptor, controller and real gateway E2E; full CI also passes. Six new cache tests prove offline and failure cleanup behavior, and nine prerequisite cases cover required/local test modes.

A five-repeat diagnostic benchmark of the 128-byte fixture gives cl100k median 21,440 to 19,471 ns/op, and o200k 23,216 to 20,557 ns/op. Both avoid **800 bytes and three allocations per call**. These are local encoder measurements, not substitutes for E2E acceptance. CPU and allocation gains do not imply the same reduction in whole-process RSS.

All new studies together contain **76 trials: 36,864 offered requests, 35,181 verified completions, zero failed admitted requests, and 1,683 intentional overload admission drops**. Totals include direct calibration but exclude setup, warm-up, qualification controls, profiling and microbenchmarks. Initial and code-head A/B matrices have identical pairwise billing. The original 142 historical trials are preserved separately and are not added to these totals.

[runs.csv](runs.csv) contains all new run-level observations. [manifest.json](manifest.json) binds workloads, binary hashes, driver, qualification outcomes, source-summary hashes and totals. CSV floats retain nine significant digits; the accompanying raw evidence archive retains full JSON and per-request distributions. [verify.py](verify.py) rejects omitted cells, duplicate trials, changed provenance, failed/unaccounted traffic and unequal paired billing before recomputing the reported comparisons. New negative-control tests exercise those checks through the existing Python test entrypoint.

```sh
python3 tests/stream-perf/results/20260924-followup/verify.py
# Fresh measurement of the exact production code, not the later evidence-only commit:
git checkout 21dc0af145560e3ff2df01bcc8383212b8d2e628
bash tests/stream-perf/compare.sh 36829cfdfddb29f6a7a2d4e20a42eb02b0f3ef4c /tmp/fresh-stream-followup \
  --repeats 5 --concurrency 8,64 --requests 256 --paced-requests 512
```

This round retains one measured, low-complexity optimization and repairs the PR blockers. The previous unsuccessful formatting candidate remains rejected. Further work needs a new hypothesis and the same immutable paired controls. Redis, other databases, multiple tenants, slow external clients, TLS/WAN, long soaks and other provider protocols remain outside this experiment. No global optimum or universal performance improvement is established.
