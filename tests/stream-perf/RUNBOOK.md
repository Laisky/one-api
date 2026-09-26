# PR 427: single execution and recovery record

## Authority and current checkpoint

Continue only in [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. Do not create another remote branch or PR, merge historical branches wholesale, force-push, delete evidence refs, or change main directly. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient. Inspect the actual head and comments before every write and preserve independent work.

**The latest bounded experiment is COMPLETE and REJECTED.** The [unused vocabulary-index report](results/20260926-core-storage/REPORT.md), [40 full-precision runs](results/20260926-core-storage/runs.csv), [diagnostic summary](results/20260926-core-storage/diagnostics.json), [identity manifest](results/20260926-core-storage/manifest.json) and [unapplied patch](results/20260926-core-storage/rejected-production.patch) are published in `ef15d5bdf719fd697143e201e60d23b4b4b4d066`. Behavior/probe/benchmark tests were retained in `d5dbc1d0c8f5014bdc3d6680f293abbc879ed7a8`. Neither commit changes production behavior or removes the vocabulary index.

The study completed 40 unprofiled trials / 15,360 successful requests, no drops/errors and exact paired durable usage. Isolated o200k retained heap fell 6.54 MiB; constructor allocated bytes fell 49.44%; all 20 HTTP pairs reduced RSS. However, long/c8 throughput -5.10%, long/c64 throughput -17.41%, CPU/request +9.70%, TTFT +93.27% / +1,384.01 ms and completion +42.13% / +1,026.49 ms fail the preregistered no-regression rules. Do not tune GC, select favorable cells or repeat the matrix to rescue this candidate.

Both separate heap diagnostics are also COMPLETE: 30s warm-up / 60s observation, 8,192 requests per variant, gateway-only mean machine CPU 60.30% / 60.36%, qualifying coverage 98.33% / 100%, upstream half-window rate drift +5.64% / -7.26%. Raw timing, counters, heap profiles and the audit are in `one-api-pr427-core-storage-evidence.zip`, supplied separately in the conversation/Library; this is not implicitly a repository file. The committed report clearly distinguishes raw-archive verification from run-level repository evidence. No process should remain running after delivery.

**Interruption recovery is explicit.** Commit `22aac043d875caf592feb80e07f00ec1f3894e5c` records the preceding BPE scratch/output rejection in [RECOVERY.md](results/20260925-bpe-scratch/RECOVERY.md). Its complete 40-trial result is reported in comment 5840674644, but its raw archive, candidate identity and proposed subsequent diagnostics were unavailable after the reset. Do not fabricate or claim to reverify missing data. That rejected matrix was not repeated.

Current production still contains the accepted sparse limiter (`de6985d9e4b4a966fe7c875b54273ee0339d3ec2`), its evidence (`a95e9f14593f4373d914e02e340ff5ee56cc7456`), the public listener/profile tools (`3b22ffba0e0144a6bd7dd943531df9c20f7448b1`), and the previously accepted exact canonical tokenizer / 4 KiB byte budget / arithmetic guard. The two sparse-limiter CI runs [36195696825](https://github.com/Laisky/one-api/actions/runs/36195696825) and [36196554110](https://github.com/Laisky/one-api/actions/runs/36196554110) passed. Inspect CI for the actual new test/evidence head; do not transfer older green checks to it or infer frontend rebuilding from path-filtered jobs.

## Completed studies: preserve boundaries, do not repeat or reapply

| Evidence | Decision and boundary |
| --- | --- |
| [Original 142 trials](results/20260924/REPORT.md) | Retain Builder accumulation and buffered SSE-line fast path. Formatting shortcut rejected; adverse cells retained. |
| [76-trial follow-up](results/20260924-followup/REPORT.md) | Retain equivalent EncodeOrdinary counting. Different CPU/session; do not compare absolute capacities across studies. |
| [40-trial coalescing](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787) | Reject 3d671dd7a9fe18865aad1b05be4527f5295d8749: long/c8 TTFT +16.55% / +71.01 ms. Keep delivery tests and SQLite statistics repair. |
| [40-trial copy study](results/20260925-copy/REPORT.md) | Reject SSE-copy elimination: long/c8 TTFT +17.75% / +12.02 ms and insufficient repeat-consistent material benefit. |
| [80-trial token-count study](results/20260925-token-count/REPORT.md) | Two complete rejections, each 40 trials / 15,360 requests. Count-only traversal lacked repeat-consistent benefit; stack scratch failed RSS/continuity/tail gates. Dependency copies not adopted. |
| [Split-only](results/20260925-pretokenization/SPLIT_ONLY.md) | Completed rejection, distinct from accepted full-token/scheduling bundle. |
| [Quantum64](results/20260925-burst-reset/QUANTUM64.md) | Rejected. Comment 5836195544 registered quantum32; no completed result was established at integration. Do not silently combine it with the byte budget. |
| [Byte-budget campaign](results/20260925-byte-budget/REPORT.md) | Three separate 40-trial studies: encoder-local rejected; stream-boundary passed; final timeout-compatible confirmation passed. Total 120 trials / 46,080 requests, not a pooled experiment. Only final-confirmation data describes the accepted bundle. |
| [Sparse limiter](results/20260925-sparse-limiter/REPORT.md) | Accepted two initial-capacity changes; 40 trials / 15,360 requests. Large RSS benefit concerns high configured ceilings with sparse histories, not default/full-history deployment guarantees. Preserve the fixed-rate CPU cost and adverse long/c8 metrics. |
| [BPE scratch/output](results/20260925-bpe-scratch/RECOVERY.md) | Rejected per comment 5840674644: both long-cell request-max gap regressions exceed 10% and 5 ms. Raw archive unavailable during recovery; do not recreate it from the reported percentages. |
| [Unused vocabulary index](results/20260926-core-storage/REPORT.md) | New, complete rejection despite 20/20 favorable RSS pairs. No runtime change adopted; full raw evidence and qualified heap diagnostics preserved separately. |

Retained guards include auth/quota, exact full-token counting, production logging/tracing, durable usage, immediate per-event flush, bad-stream controls, cancellation cleanup, SQLite usage-statistics retries, content-gap telemetry, atomic failed-study evidence and authoritative comparison arguments. Usage-statistics equality is not an independent financial-ledger audit. There is no count approximation, synthetic text cache, asynchronous batching, semaphore or deferred accounting. A 4 KiB scheduling budget does not bound one large BPE piece.

## Historical branch inventory

The eight helper refs below were rechecked during this resumption and remained unchanged. They are frozen by execution policy, not deleted or technically archived. No new ref was created. Ahead-of-PR is not approval to adopt work.

| Branch | Historical tip | Disposition |
| --- | --- | --- |
| `perf/stream-chat-e2e-20260924` | Read actual PR head | Sole active delivery branch. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` | Delivery/SQLite repair retained; omit temporary workflow/encoded patch. |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` | Reporter/continuity/failed-study work already retained. |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` | Rejected coalescing build/evidence, not production. |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` | Historical blob preparation only. |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` | Superseded coalescing patch/workflow. |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` | Oversized tooling export; avoid this recovery route. |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` | Workflow-only differences; production follow-up retained. |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` | Reuse runtime artifact, not obsolete source as baseline. |

## Source, build and evidence recovery

| Recovery input | Run / artifact | Archive SHA-256 |
| --- | --- | --- |
| Go 1.27.1 / vendor / token cache | `36045112587` / `10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` |
| Immutable 299deff / harness | `36084329116` / `10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` |
| 21dc0af source/build | `36068339476` / `10836434460` | `14738f5b1207dcad408c138fe9eae3e30eae1dab06c06d5128f57df66e96e36d` |
| Copy-study archive | `one-api-pr427-copy-evidence.zip` | `97a97fe2d3857bd51def974b2a66a2a519a9a504057a32e0e3055e076bda138f` |
| Accepted byte-budget archive | `one-api-pr427-byte-budget-evidence.zip` | `e09795b55fc81ad0006c7b8fa9f24011bb43da6a7200252f54084e300d5c5e83` |
| Sparse-limiter archive | `one-api-pr427-sparse-limiter-evidence.zip` | `64dd0e7fd92b688b578541177234f45b0257ca9fd6b27c77e83f99aed6556c9a` |

The accepted byte-budget integration audited 263 covered archive files and eight audit self-tests. The historical source manifest verifies 954 production/dependency/embed inputs; later retained user/channel blobs are f0a66e703258c63f3436e80e88a91bf4f07e646e and 59862525884dae9af6705fef3af787ea7a5092bc. Apply exact accepted overlays and verify current public blobs, never label a recovered local commit as original GitHub history. Availability is finite; verify actual paths and archive hashes.

The pre-guard integration reproduced gateway SHA-256 6550a116e0d978f4ce7d5f323bc69ab83321adfe484fab15f154f404f993304b and driver 7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e using Go1.27.1 and `-p=2 -mod=vendor -trimpath -buildvcs=false`. The later guarded gateway is 13a1ec75315310d1ada3f9b343d63ccb3770ffcc604d4790e92a0e0344a51f3b. Its independent 640-request validation is correctness evidence, not a new performance matrix. [Integration details](results/20260925-byte-budget/INTEGRATION.md) retain CodeQL review 4107214497 and bounds tests. Production b6efcb8 passed [CI36169997020](https://github.com/Laisky/one-api/actions/runs/36169997020); newer heads require their own checks.

For the new storage study, local baseline gateway b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46 includes the sparse limiter and public listener. Local rejected candidate 9f9a42908e63fa85adecc44f4d19beedb20d4fbb builds 76c651c0d03174973f34d59d31117665958b53da88983f7b731fa748065703ff. Only core_bpe.go differs in production inputs. The public run/profile files were restored to exact current blobs 125548e64471a6193c84c9912dc73ec9aab34c23, 599f95ec4940149aaa0c13bef83f9a03662ef78a and 49161e86a26ae9ef2ffa06c4b08d7497bdc43a55. The standalone host pprof reader decoded profiles after workloads ended. No helper build or diagnostic workload overlapped A/B.

Direct clone failed in the runtime. Do not repeatedly retry or create another helper branch. Artifact10844767167 is 585,766,240 bytes, beyond the connector's 536,870,912-byte limit. Exclude sensitive local instructions and credentials from exports. The older `one-api-pr427-20260925-evidence.zip` / f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad remains separate and was not newly reverified. Committed summary/CSV files are not raw request/profile archives. Do not conflate archive-only audit self-tests with repository CI.

## Acceptance and next bounded action

Keep the [original registered contract](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402): independent qualification, ten-second usage settlement, five alternating pairs per short/long c8/c64 cell; long1,024 x128 bytes/256 requests, short32 x128 bytes/2ms/512 requests. Require >=5% long-cell median throughput or CPU benefit in >=4/5 pairs; reject short throughput/CPU regression >5%, latency regression exceeding both10% and5ms, and RSS growth >10%. Justified memory-specific benefit rules must be registered before measurement, as in the limiter and rejected storage studies; do not relax safety gates after observing results.

First finish actual current-head CI/review issues. The new storage study and both heap diagnostics are complete: do not repeat them to restore context. The next diagnostic question is whether allocation-rate/GC/scheduler behavior and client-observed latency explain the repeated memory-versus-tail tradeoff. That is an unproven hypothesis; do not remove the index or retune GC without a separate registered experiment. The older BPE scratch result remains rejected even though its raw archive is missing.

Follow [PROFILING.md](PROFILING.md): freeze a stable workload, qualify >50% gateway-only effective machine CPU with raw progress/drift records, collect CPU and memory separately, change one mechanism and validate unprofiled immutable A/B. Helper CPU cannot qualify the gateway; memory occupancy is not a target to fill. Preserve full token IDs, invalid UTF-8, Unicode/special/custom patterns, finite-timeout fallback, concurrency, accounting and immediate delivery. Keep the independent original tokenizer oracle and license.

`compare.sh` reuses a read-only validated tokenizer cache and places managed binary/baseline/driver/cache/output arguments after caller options. It does not inherit profiling settings. Public fixtures now set LISTEN_HOST=127.0.0.1 and the profiler checks actual API/pprof sockets; the normal deployment default still binds all interfaces. Do not regress this isolation or reintroduce a duplicate profile runner.

Use detached local worktrees, never a new remote branch. Preserve failed/adverse/aborted evidence and exact source identities. Commit completed tests and reports incrementally here, and archive full raw records before a final result comment. Never leave the only recovery instructions in chat or present a partial/mixed-binary study as complete.
