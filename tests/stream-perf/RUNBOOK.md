# PR 427: single execution and recovery record

## Authority and current checkpoint

Only [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. No remote experiment branch/PR, forced update, historical branch merge, evidence-ref deletion or main merge. Read actual HEAD and comments before writes, preserving concurrent work. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient.

**Per-event correlation is complete; no new production optimization is adopted.** Tooling commit `1b50b74faf31ae0b2c24da6f1e14e448d38a84b5` adds explicit source-pinned build overlays, numeric-only probes, clock identity checks and independent interval/trace analysis. [Report](results/20260926-event-correlation/REPORT.md), [registered plan and pre-capture amendment](results/20260926-event-correlation/PLAN.md), [scoped summary](results/20260926-event-correlation/summary.json) and [CORRELATION.md](CORRELATION.md) are the latest entry points. The prior HEAD19976b3 passed CI36207149861; the new tooling and final documentation HEAD require their own CI, not a transferred green result.

One instrumented run completed8,192 requests, zero failures/drops, exact8,192 request statistics/40,280,064 quota units. It independently passed stream/fault/auth/cancellation qualification. Thirty-second warm-up,60-second observation, five-second trace inside it, c32,1,024x128-byte chunks, process GOMAXPROCS3/2/2. Raw counters reproduce59.999576s covered observation,60.417094% gateway-only effective-machine CPU,100% high-load duration and+2.114229% upstream half-rate drift. A nominal30s/60s recheck independently qualifies. These are diagnostic observations, not A/B or a speedup.

The deterministic1/32 cohort selects256 requests; the trace overlaps six, with4,875 complete marker triplets,4,855 content frames and4,849 adjacent content pairs. There are83 client gaps>10ms; one pair with>1ms trace/monotonic skew is excluded from state attribution. Of the remaining82,78 have at least80% of the between-render interval in the same gateway goroutine's Runnable state. A contrasting pair has32.025ms client gap but0.0629ms gateway flush-return gap and zero recorded between-render Runnable wait. Another has30.317ms client gap with30.770ms Runnable overlap. This distinguishes timing patterns, not causation, and does not justify removing fairness or blaming every stall on GC/network.

**Clock limitation is explicit.** The first attempt failed at its first telemetry snapshot because all `/proc/*/ns/time` APIs were absent; it produced no trace/window and remains incomplete. Before capture, comment5842057736 restricted the diagnostic to within-process intervals when all namespace APIs are absent. Mixed/invalid/changed evidence still fails. Absolute one-way render-to-client and post-flush lag are omitted. Client-gap minus gateway-flush-gap is invariant to constant epoch offsets; it is not network latency. State/GC intersections stay on the gateway trace clock; complete ranges only, overlaps not summed as extra time. One large BPE piece still has no latency bound.

Twenty-one new Python tests and32 existing profile tests passed locally. Five Go probe tests and the explicit-overlay wire/error/flush test passed three race repetitions; the latter is intentionally skipped in normal tests. A broken frame counter fails. Normal rebuilt gateway/driver hashes are unchanged, so no probe is imported by ordinary builds. Instrumentation overhead is not calibrated away; do not use observed binaries as performance baselines.

The full archive `one-api-pr427-event-correlation-evidence.zip` is delivered separately, not implicitly committed. It contains every request/timestamp, raw trace, filtered events, exact overlay, failed attempt, identities, logs and auditor. Run its `verify_all.py` and targeted tests after extraction; do not rerun the completed load to recover context. No workload remains running after delivery.

## Previous diagnostics and retained runtime

The prior stable-window/trace-tool work (`c3aae60`, evidence19976b3) remains. V2 fixes CPU-busy/no-progress and drifting-window false positives, validates every counter, weights coverage by duration, requires positive progress in both halves and <=15% upstream-rate drift. Trace capture remains opt-in1–10s,128MiB, loopback-only, bounded and identity-recorded. Historical gate decisions are not relabeled.

The previous CPU60s and trace5s diagnostics were separate runs on unchanged production, each8,192 requests after30s warm-up/60s observation. Gateway-only CPU61.08%/59.35%, high-load time100%/98.30%, coverage59.001s/59.002s, drift+2.17%/-0.67%; both qualified. The5.015s trace contained108.97 aggregate goroutine-seconds runnable delay,84.70% at Gosched, not wall time/CPU/client latency. Complete STW total22.27ms/max3.031ms and mark-assist max21.12ms were descriptive. BPE29.70% cumulative CPU remains a hotspot. Those prior requests/CPU samples are not pooled with this frame-correlation run. Its90-file archive and eight audit controls remain distinct.

Accepted production remains sparse limiter `de6985d9e4b4a966fe7c875b54273ee0339d3ec2`, evidence `a95e9f14593f4373d914e02e340ff5ee56cc7456`, public listener/profile controls `3b22ffba0e0144a6bd7dd943531df9c20f7448b1`, canonical tokenizer/4KiB budget `b6efcb89ddee034d33148d66d8c3d6c755056cf7`, arithmetic guard `1783b4990bbf79030452fdbe262e561d6d2b3207`. Sparse-limiter CI36195696825/36196554110, bundle CI36169997020 and diagnostic-tool CI36206443998 passed historically; none certifies a later HEAD. Path-filtered frontend jobs are not rebuilt frontend tests.

## Completed studies: do not repeat or reapply

| Evidence | Decision and boundary |
| --- | --- |
| [Original142 trials](results/20260924/REPORT.md) | Retain Builder/buffered SSE; formatting rejected. |
| [76-trial follow-up](results/20260924-followup/REPORT.md) | Retain equivalent EncodeOrdinary; different host/session. |
| [40-trial coalescing](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787) | Reject3d671dd; retain delivery tests and SQLite statistics repair. |
| [40-trial copy](results/20260925-copy/REPORT.md) | Rejected TTFT/repeatability. |
| [80-trial token-count](results/20260925-token-count/REPORT.md) | Two separate40-trial rejections; count-only/stack copies not adopted. |
| [Split-only](results/20260925-pretokenization/SPLIT_ONLY.md) | Rejected, distinct from accepted full-token bundle. |
| [Quantum64](results/20260925-burst-reset/QUANTUM64.md) | Rejected; separate quantum32 comment5836195544 has no verified complete result at integration. |
| [Byte-budget120 trials](results/20260925-byte-budget/REPORT.md) | Three separate40-trial studies; only final timeout-compatible confirmation describes accepted bundle. |
| [Sparse limiter](results/20260925-sparse-limiter/REPORT.md) | Accepted40-run memory study for high configured limits/sparse histories, not default/full-history guarantee; retain adverse CPU/throughput cells. |
| [BPE scratch/output](results/20260925-bpe-scratch/RECOVERY.md) | Rejected per5840674644; raw archive/candidate identity unavailable after interruption. Never fabricate/repeat to replace missing data. |
| [Unused vocabulary index](results/20260926-core-storage/REPORT.md) | Complete40-run rejection despite20/20 favorable RSS; long/c8 RPS-5.10%, long/c64 RPS-17.41%, CPU+9.70%, TTFT+93.27%, completion+42.13%. |
| [Runtime diagnostics](results/20260926-runtime-diagnostics/REPORT.md) | Two qualified baseline-only CPU/trace runs and V2 repair; no runtime candidate. |
| [Event correlation](results/20260926-event-correlation/REPORT.md) | One completed instrumented trace run and independent frame matching; no absolute clock origin or causal/runtime gain claimed. |

Core-storage191-file archive/15,360 A/B plus16,384 heap-diagnostic requests and eight audit tests were previously reverified, not rerun; retained tests d5dbc1d/evidence ef15d5b are unchanged. Nominal V2 reanalysis does not rewrite historical rejection. Counter equality is not a financial-ledger audit. Keep authentication/quota, exact full token IDs, invalid UTF-8/Unicode/special/custom/finite-timeout behavior, original oracle/license, logging/tracing, durable usage, immediate per-event flush, cancellation, SQLite retries and failed-study evidence. No approximate count, synthetic text cache, deferred accounting, batching or semaphore.

## Historical helper refs

These eight refs were verified in the core-storage resumption and frozen by policy, not deleted/technically archived. No new helper is needed; ahead-of-PR is not approval.

| Branch | Historical tip |
| --- | --- |
| `perf/stream-chat-e2e-20260924` | Read current HEAD; sole delivery branch. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` |

Acceptance/harness production changes are retained; carrier workflows are not. Immutable/coalescing branches preserve rejected code, objects/runtime are recovery only. Artifact10844767167 exceeds536,870,912 bytes; do not repeat that route or create a helper branch.

## Source and evidence recovery

| Input | Run/artifact or archive | SHA-256 |
| --- | --- | --- |
| Go1.27.1/vendor/cache | `36045112587/10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` |
| Immutable299deff/harness | `36084329116/10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` |
| Source21dc0af | `36068339476/10836434460` | `14738f5b1207dcad408c138fe9eae3e30eae1dab06c06d5128f57df66e96e36d` |
| Copy | `one-api-pr427-copy-evidence.zip` | `97a97fe2d3857bd51def974b2a66a2a519a9a504057a32e0e3055e076bda138f` |
| Byte-budget | `one-api-pr427-byte-budget-evidence.zip` | `e09795b55fc81ad0006c7b8fa9f24011bb43da6a7200252f54084e300d5c5e83` |
| Sparse limiter | `one-api-pr427-sparse-limiter-evidence.zip` | `64dd0e7fd92b688b578541177234f45b0257ca9fd6b27c77e83f99aed6556c9a` |
| Core storage | `one-api-pr427-core-storage-evidence.zip` | `8936eb113776cb93698193fb77668ede924e337bfca06e31b9fcbfefbe4f783d` |
| Runtime diagnostics | `one-api-pr427-runtime-diagnostics.zip` | `38567026945bde885a2d11e07a4d812e3a96554bdc8bf42a3ea2573b4ced8bc8` |

Verify actual paths/hashes and finite retention. Historical954-input recovery plus retained user/channel blobs f0a66e703258c63f3436e80e88a91bf4f07e646e /59862525884dae9af6705fef3af787ea7a5092bc and exact accepted overlays must be reconciled with current public listener/main/harness blobs. This resumption verified961 retained source/build inputs and reproduced normal gateway `b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46` and driver `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`, before and after tooling additions. Go1.27.1, identical vendor/embed, `-p=2 -mod=vendor -trimpath -buildvcs=false`. Local recovery commits are not original GitHub history.

Observed gateway `08f4177394eba0b189f655e440d5f7ea96c73edc03df4cfbda973a5a7f61a7fc` and driver `5687629c373d253daafeaaa42217e4beaa02443e7f1a1d0cb1aaa6c29279ce42` are diagnostic-only binaries. Never relabel them as performance baselines or ship the overlay as a production patch. Original pre-guard6550a116/guarded13a1ec75 binaries and640-request safety smoke remain separate; [integration details](results/20260925-byte-budget/INTEGRATION.md) preserve CodeQL4107214497.

Direct Git clone failed in recovered environments; do not repeatedly retry it. Never export sensitive `.github/instructions/laisky.instructions.md`, credentials, databases or toolchain/font files. The older f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad archive was not newly reverified. Repository summaries are not raw request/trace archives, and archive-only audits are not extra repository-CI tests.

## Next bounded action and acceptance

First finish actual-HEAD CI/review findings. Event correlation and its namespace-failure attempt are now fully recorded; do not rerun them for recovery. The next mechanism must distinguish the gateway-Runnable pattern from the separate client-observation pattern under a preregistered control. Potential client scheduling/transport instrumentation or a controlled fairness-placement design requires its own observer bounds and full behavioral/latency evidence; current correlations alone do not approve removal of Gosched, GC retuning or rejected BPE/core-storage patches.

Keep [original A/B gates](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402): independent qualification, ten-second settlement, five alternating pairs per short/long c8/c64 cell, long1,024x128 bytes/256 requests and short32x128 bytes/2ms/512 requests. Require >=5% long-cell CPU/RPS benefit in>=4/5 pairs; reject short CPU/RPS regression>5%, latency regression exceeding both10% and5ms, or RSS growth>10%. Justified memory-benefit rules are registered before measurements, never selected to rescue adverse results. No global optimum, capacity or significance from pooled requests.

Follow [PROFILING.md](PROFILING.md): freeze workload, require >=50% gateway-only effective CPU and stable progress by elapsed duration, capture CPU/heap/trace separately, and evaluate unprofiled immutable binaries after behavior is preserved. Helpers cannot qualify gateway load; memory occupancy is not a fill target. [CORRELATION.md](CORRELATION.md) is an explicit diagnostic overlay using the existing runner, not a second profiler. Default fixtures remain unprofiled and loopback-only; compare.sh preserves validated read-only cache and authoritative binary/output identities. Preserve adverse/failed/incomplete evidence; commit finished work and archive raw records before final delivery. Never leave recovery instructions only in chat.
