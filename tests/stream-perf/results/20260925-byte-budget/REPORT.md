# Exact tokenizer and 4 KiB stream scheduling: accepted and integrated

## Decision

Retain the **final** measured bundle: exact canonical ordinary tokenization, its finite-regex-timeout fallback, and a 4 KiB accumulated already-buffered SSE-line scheduling budget. Production commit: `b6efcb89ddee034d33148d66d8c3d6c755056cf7`, fast-forwarded from `f9b85f7` on PR427's existing branch. The production implementation and tests use the 14 exact code/license Git blobs from final local candidate `eee3e056801ae7ed45abc329be86a9d261765df6`; only the upstream-maintenance documentation was updated. No new remote branch, force push, temporary workflow or automatic merge.

The later token-count, split-only and line-quantum reports remain unchanged. The separate quantum64 experiment was rejected; its quantum32 refinement was only registered at the integration read, not treated as measured or combined with this byte budget. These are different experiments, not competing active delivery branches.

## Final-source confirmation

Five alternating A/B pairs in each of four cells. Long streams: 1,024 x 128-byte content chunks, unpaced upstream, 256 requests per run. Short streams: 32 x 128 bytes, 2 ms per chunk, 512 requests per run. One real gateway, one local mock, one independent validating client; fresh SQLite and one account/token/channel per trial; 16 warm-up requests excluded. Authentication, quota, exact token counting, usage persistence, logging, tracing and immediate per-event flushing remain enabled. Both variants independently qualified normal/CRLF/fragmented streams, corrupt/truncated/malformed controls, invalid authentication and eight-client upstream cancellation cleanup. Usage settlement retains the ten-second deadline.

**40 complete trials, 15,360 verified requests, zero failed or dropped requests, exact paired durable usage.** Values below are medians of the five paired percentage changes, not percentages of independent absolute medians. They are descriptive operational measurements, not confidence intervals.

| Profile | Concurrency | RPS change | CPU/request change | P95 first-content change | P95 completion change | Sampled peak RSS change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Short/paced | 8 | +4.15% | -14.60% | -36.79% | -7.74% | +0.45% |
| Short/paced | 64 | +3.19% | -9.01% | -8.47% | -11.67% | -5.55% |
| Long/unpaced | 8 | +70.91% | -40.96% | -65.26% | -41.34% | -2.56% |
| Long/unpaced | 64 | +56.84% | -37.28% | -33.47% | -40.08% | -1.06% |

All ten long-stream pairs improved throughput, CPU/request and P95 completion. The bundle passes the existing rule: at least 5% long-stream throughput or CPU benefit with at least 4/5 favorable pairs; no short-stream throughput/CPU regression above 5%; reject latency regressions exceeding **both 10% and 5 ms**, or paired-median RSS increases above 10%.

**Adverse evidence remains:** short/c64 P95 request-max inter-content gap rose 23.01%, but by 2.44 ms, below the absolute latency gate. Short/c8 sampled RSS rose 0.45%; one pair rose 22.79%. Individual long/c64 first-content, gap and RSS outliers also remain in the ledger. Do not claim every run is faster, universal memory savings, or a hard scheduling deadline. The strongest repeat-consistent gains concern CPU and long-stream completion; memory gains are modest. The content-gap metric is the P95 of each request's maximum observed gap, not a pooled token-gap percentile.

## Why the bundle differs from rejected candidates

The earlier profile attributed most sampled CPU to exact ordinary token encoding. The canonical path eliminates generic regexp2 match/rune materialization only for two byte-exact standard patterns. It preserves Unicode whitespace, contraction behavior, invalid UTF-8 replacement and complete token IDs. The byte-pair merge algorithm, dictionaries, special-token path and decoding remain copied unchanged from MIT-licensed tiktoken-go v0.1.8. Unsupported patterns and finite regex deadlines retain the original matcher. The original module remains an independent test oracle. The internal copy is an explicit maintenance cost; see `internal/tokenizer/UPSTREAM.md`.

Scheduling inside a single encoding call did not address many small streaming deltas: their individual calls never reached 4 KiB. The reader therefore accumulates already-buffered bytes across calls and yields synchronously before the next read, after the previous caller has processed/flushed its line. It checks cancellation again before consuming a new line and resets the budget on the asynchronous I/O path. There is no queue, semaphore, token approximation, text cache, prefetch or flush coalescing. A single large token piece is not subdivided, so its processing latency is not bounded by this budget.

## Preserve the three independent experiments

| Study | Trials / requests | Decision | Disposition |
| --- | ---: | --- | --- |
| encoder-local | 40 / 15,360 | Rejected | Encoder-only yield did not satisfy latency gates. |
| stream-boundary | 40 / 15,360 | Passed | Cross-call stream budget; superseded by timeout-compatible final source. |
| final-confirmation | 40 / 15,360 | Passed | Final timeout guard included; this report's performance table. |

Totals are 120 trials and 46,080 verified requests, excluding setup, warm-up, qualification and microbenchmarks. They are not one pooled experiment. Integration re-audited all three instead of repeating them merely because the previous session ended. The separate line-count and other historical studies are excluded.

## Integration and source proof

The interrupted-session archive is `one-api-pr427-byte-budget-evidence.zip`, SHA-256 `e09795b55fc81ad0006c7b8fa9f24011bb43da6a7200252f54084e300d5c5e83`. Integration verified all 263 covered files, reran its independent auditor and eight self-tests, and recovered the retained baseline with 954 production/dependency/embed inputs verified. No inaccessible public clone or original local Git history is claimed.

A fresh integration build using the recovered Go 1.27.1 toolchain, `-p=2 -mod=vendor -trimpath -buildvcs=false`, reproduced **exactly** the measured final gateway SHA-256 `6550a116e0d978f4ce7d5f323bc69ab83321adfe484fab15f154f404f993304b` and driver `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`. Every uploaded implementation/test/license blob was pinned to its verified Git hash. Standard CI builds may carry different VCS/embed metadata; they must not be relabeled as these measured binaries.

Fresh integration validation: internal tokenizer race tests passed; renderer/SSE race tests passed three repetitions; OpenAI token/encoder/stream-focused race tests passed three repetitions; focused go vet and both binary builds passed. Differential tests compare 16,256 inputs in each encoding, plus special tokens, invalid bytes, custom patterns, timeout fallback and shared encoders. Reader tests check accumulated budgets, prior flushes, cancellation/Close and I/O reset. Existing public tests/reports are preserved. Current public-head CI is recorded in the PR checks, not inferred from this local result.

## Audit and scope

`final-runs.csv` contains all 40 final runs, with six performance metrics at nine significant digits. `manifest.json` records original identities and hashes. The compact verifier recomputes complete-matrix performance arithmetic; **it does not replace raw stream/usage qualification**. Full-precision rows, all three studies' request samples, qualification, profiles, negatives, plans and audit code remain in the separate archive delivered in the conversation, not in this directory.

```sh
python3 tests/stream-perf/results/20260925-byte-budget/verify.py
# Separately, from the extracted original evidence archive:
python3 -B verify_all.py
```

The original environment was Linux, Intel Xeon Platinum 8573C, four-core cgroup quota, 4 GiB, GOMAXPROCS=2 per process, shared CPU without affinity isolation. Gateway CPU includes usage settlement; RSS is sampled at 20 ms; client peak RSS is unmeasured. No sustained production capacity, multi-tenant/PostgreSQL/MySQL/Redis, slow-client soak, WAN/TLS or cross-protocol performance claim follows. Different host/session results must not be compared as gains.

Next experiments follow `tests/stream-perf/PROFILING.md`: sustained fixed workloads, explicit CPU allowance and stable-window qualification, separate CPU and memory profiling, one measured bottleneck at a time, then unprofiled A/B acceptance. The historical short trials above are not retroactively claimed to meet that new sustained-load protocol.
