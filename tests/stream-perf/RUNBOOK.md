# PR 427: single execution and recovery record

## Authority and current checkpoint

Continue only in [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. Do not create remote experiment branches/PRs, merge historical branches wholesale, force-push, delete evidence refs or change main directly. Preserve concurrent work. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient.

**Latest diagnostic-tool iteration is complete; no new production optimization was adopted.** Commit `c3aae60c50ff8de4d6e705cc8d3b389b9b3b321a` fixes stable-window false positives and adds bounded execution-trace capture to the existing profiler. [Report](results/20260926-runtime-diagnostics/REPORT.md), [registered plan](results/20260926-runtime-diagnostics/PLAN.md) and [exact summary identities](results/20260926-runtime-diagnostics/summary.json) describe the completed two-run study. The original parent `36e3e994d2d9a396ea7b47af18a00cb7b25c381f` passed CI36204562355; c3aae60 has its own CI36206443998. Check the actual final head and its checks, not a previous green run or stale PR description.

The old profiler could admit busy CPU with zero request progress or 50% throughput drift and did not reject progress-counter rollback. Three negative tests reproduce four assertion failures. New rule version2 validates all counters/times, duration-weights CPU and high-load coverage, requires positive progress in both elapsed halves and <=15% half-rate drift. It preserves a separate CPU-only sub-gate, but does not relabel historical decisions. Twenty-two new tests and ten exact existing profile tests pass locally. Eight archive-only audit tests are separate from repository CI.

CPU60s and trace5s were collected independently on the **same retained runtime binary**, each after30s warm-up with60s observation, c32,8,192 requests of1,024x128-byte chunks, gateway/mock/client GOMAXPROCS3/2/2. Both actual loopback listeners were checked. Each run verified all8,192 requests and exact40,280,064 quota units; no failures/drops. Gateway-only machine CPU means were61.08%/59.35%, time above50% was100%/98.30%, covered observations59.001s/59.002s, half-rate drift+2.17%/-0.67%. These are qualified diagnostics, **not a new A/B or capacity claim**. No GC/runtime/limiter/accounting/flush setting changed.

The5.015-second trace has108.97 aggregate goroutine-seconds of runnable delay,84.70% attributed to Gosched. That is not wall time, CPU consumed by Gosched or a client stall. Complete GC mark/sweep-termination pauses total22.27ms with maximum3.031ms; goroutine-scoped mark-assist maximum21.12ms can include scheduling and is not an independent request-latency measurement. CPU profiling still attributes29.70% cumulatively to BPE merge. These observations motivate correlated delivery/scheduler/GC investigation; they do not prove why previously rejected memory candidates regressed or justify removing the accepted fairness budget.

The separately supplied `one-api-pr427-runtime-diagnostics.zip` retains raw request samples, CPU profile, trace, process/cgroup counters, source/decoder identities, tests and independent auditor. Run `python3 -B verify_diagnostics.py` and `python3 -B test_diagnostic_audit.py` after extraction. The repository stores summarized results; do not claim raw profiles are committed here. Both workloads ended before decoding/tests began. No experiment remains running after delivery. Decode retries were offline postprocessing, never repeated workloads.

## Retained runtime and completed studies

Current runtime still includes sparse limiter `de6985d9e4b4a966fe7c875b54273ee0339d3ec2`, evidence `a95e9f14593f4373d914e02e340ff5ee56cc7456`, public listener/profile controls `3b22ffba0e0144a6bd7dd943531df9c20f7448b1`, accepted canonical tokenizer/4KiB byte budget `b6efcb89ddee034d33148d66d8c3d6c755056cf7`, and arithmetic guard `1783b4990bbf79030452fdbe262e561d6d2b3207`. Historical sparse-limiter CI36195696825/36196554110 and bundle CI36169997020 passed; they do not certify later heads. Frontend path-filtered jobs are not fresh frontend builds.

| Evidence | Decision and recovery boundary |
| --- | --- |
| [Original142 trials](results/20260924/REPORT.md) | Retain Builder accumulation and buffered SSE-line path; formatting shortcut rejected. |
| [76-trial follow-up](results/20260924-followup/REPORT.md) | Retain equivalent EncodeOrdinary. Different host/session; absolute capacities not comparable. |
| [40-trial coalescing](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787) | Reject3d671dd; preserve delivery tests/SQLite statistics repair. |
| [40-trial copy study](results/20260925-copy/REPORT.md) | Reject SSE-copy elimination on TTFT and repeatability gates. |
| [80-trial token-count study](results/20260925-token-count/REPORT.md) | Two complete40-trial rejections; count-only and stack-count copies not adopted. |
| [Split-only](results/20260925-pretokenization/SPLIT_ONLY.md) | Rejected, distinct from accepted full-token/scheduling bundle. |
| [Quantum64](results/20260925-burst-reset/QUANTUM64.md) | Rejected; comment5836195544 registered quantum32 but no completed result was established at integration. Do not silently combine it with byte scheduling. |
| [Byte-budget120 trials](results/20260925-byte-budget/REPORT.md) | Three separate40-trial studies: encoder-local rejected, stream-boundary passed, final timeout-compatible confirmation passed. Only final confirmation describes the accepted measured bundle. |
| [Sparse limiter](results/20260925-sparse-limiter/REPORT.md) | Accepted40-trial memory optimization for high configured limits with sparse histories. Preserve adverse long/c8 and fixed-rate CPU observations; no default/full-history RSS guarantee. |
| [BPE scratch/output](results/20260925-bpe-scratch/RECOVERY.md) | Rejected per comment5840674644; raw archive/candidate identity unavailable after interruption. Do not recreate or claim to reverify missing evidence. |
| [Unused vocabulary index](results/20260926-core-storage/REPORT.md) | Complete40-trial rejection despite20/20 favorable RSS pairs. Long/c8 RPS-5.10%; long/c64 RPS-17.41%, CPU+9.70%, TTFT+93.27%, completion+42.13%. No production removal adopted. |
| [Runtime diagnostics](results/20260926-runtime-diagnostics/REPORT.md) | Two qualified baseline-only diagnostics and a tooling correctness fix. No runtime candidate or new optimization gate result. |

The prior core-storage archive was re-audited (191 covered files,15,360 A/B requests,16,384 diagnostic requests, eight audit tests) without rerunning traffic. Its retained tests are in d5dbc1d and full run-level evidence in ef15d5b; its production removal remains rejected. Its archived samples separately pass nominal-window version2 analysis, without changing their historical gate or rejection.

Retain auth/quota, exact complete token IDs, logging/tracing, durable usage, immediate per-event flush, bad-stream/auth controls, cancellation cleanup, SQLite busy retries, content-gap metrics, immutable comparison identities and failed-study evidence. Counter equality is not an independent financial-ledger audit. No approximate counting, synthetic text cache, asynchronous batching, semaphore or deferred accounting is authorized. A4KiB budget does not bound one large BPE piece.

## Historical helper refs

These eight helpers were last verified during the preceding core-storage resumption and were unchanged then. They are frozen by policy, not deleted or technically archived. No helper is needed for this continuation; ahead-of-PR is not adoption approval.

| Branch | Historical tip |
| --- | --- |
| `perf/stream-chat-e2e-20260924` | Read current PR head: sole active delivery branch. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` |

Acceptance/harness changes were already retained; omit their carrier workflows. Immutable/coalescing refs preserve rejected code only. Objects/runtime refs are recovery material, not new baselines. The local-kit artifact10844767167 exceeds the connector536,870,912-byte limit; do not retry or create another helper branch.

## Source and artifact recovery

| Recovery input | Run/artifact | SHA-256 |
| --- | --- | --- |
| Go1.27.1/vendor/cache | `36045112587/10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` |
| Immutable299deff/harness | `36084329116/10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` |
| Source21dc0af | `36068339476/10836434460` | `14738f5b1207dcad408c138fe9eae3e30eae1dab06c06d5128f57df66e96e36d` |
| Copy evidence | `one-api-pr427-copy-evidence.zip` | `97a97fe2d3857bd51def974b2a66a2a519a9a504057a32e0e3055e076bda138f` |
| Byte-budget evidence | `one-api-pr427-byte-budget-evidence.zip` | `e09795b55fc81ad0006c7b8fa9f24011bb43da6a7200252f54084e300d5c5e83` |
| Limiter evidence | `one-api-pr427-sparse-limiter-evidence.zip` | `64dd0e7fd92b688b578541177234f45b0257ca9fd6b27c77e83f99aed6556c9a` |
| Core-storage evidence | `one-api-pr427-core-storage-evidence.zip` | `8936eb113776cb93698193fb77668ede924e337bfca06e31b9fcbfefbe4f783d` |

Verify actual paths, hashes and availability; archive retention is finite. Historical recovery verifies954 production/dependency/embed inputs with retained user/channel blobs f0a66e703258c63f3436e80e88a91bf4f07e646e and59862525884dae9af6705fef3af787ea7a5092bc, then exact accepted overlays. Public listener/main/run/profile files must be checked separately. Never label a local recovered commit as original GitHub history or include sensitive local instructions in exports.

Retained production rebuilt for the current diagnostic reproduces gateway SHA-256 `b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46` and driver `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`, using Go1.27.1, identical vendor/embed inputs and `-p=2 -mod=vendor -trimpath -buildvcs=false`. This includes the sparse limiter/public listener, not rejected core-storage code. Its hash differs from historical pre-guard6550a116 and guarded13a1ec75 builds; keep identities separate. The earlier640-request arithmetic-guard smoke is correctness evidence, not an optimization matrix. [Integration details](results/20260925-byte-budget/INTEGRATION.md) preserve CodeQL4107214497 and bounds tests.

Direct clone failed in the recovered runtime; do not repeatedly retry that route. The older f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad archive remains separate and was not reverified. Repository summaries/CSVs are not raw trace/request archives. Archive-only auditing tests are not repository CI tests.

## Acceptance and next bounded action

First check actual-head CI and review feedback. All diagnostics from this checkpoint are complete; do not rerun them for context recovery. The next experiment needs correlated per-event delivery/client observations and runtime scheduling/GC evidence, not another uninstrumented memory reduction. The short trace's Gosched attribution is an observation, not proof that removing fairness is safe. No rejected runtime patch should be resurrected or GC threshold retuned without a new registered design and complete no-regression evidence.

Preserve the [registered A/B contract](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402): per-variant qualification; ten-second exact usage settlement; five alternating pairs in short/long c8/c64 cells; long1,024x128 bytes/256 requests, short32x128 bytes/2ms/512 requests. Require >=5% long-cell CPU or throughput benefit in>=4/5 pairs; reject short CPU/throughput regressions>5%, latency regressions exceeding both10% and5ms, and RSS growth>10%. Register justified memory materiality rules before measuring; never relax safety gates after seeing outcomes.

Follow [PROFILING.md](PROFILING.md) and the [version2 diagnostic report](results/20260926-runtime-diagnostics/REPORT.md). CPU/heap/trace runs are separate. Trace is opt-in with a1–10 second subwindow,128MiB capture bound, explicit source/hash/timing and partial-file failure. Qualify stable progress and >=50% effective-machine gateway CPU by elapsed time, not helper load or sample count. Never fill memory merely to meet a target. Compare unprofiled immutable binaries only after behavior is preserved.

Public fixtures bind LISTEN_HOST=127.0.0.1 and verify actual API/pprof sockets; normal deployments keep the existing all-interface default. `compare.sh` retains the read-only verified token cache and authoritative managed arguments, without inheriting profiler settings. Do not add a duplicate profiler or weaken network isolation. Preserve exact token/UTF-8/Unicode/special/custom/finite-timeout behavior, original tokenizer oracle and license.

Use detached local worktrees and the one remote delivery branch. Preserve all failed/adverse/incomplete evidence and source identities. Commit completed work incrementally, archive raw records before the final result, and keep this record current. Do not leave recovery instructions only in chat or treat partial/mixed-binary observations as complete.
