# PR 427: single execution and recovery record

## Authority and current state

Continue only in [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. Do not create another remote branch or PR, merge historical branches wholesale, force-push, delete evidence refs, or change main directly. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient. Inspect the actual head and comments before every write, preserving independently advancing work.

**The accepted 4 KiB byte-budget + exact canonical tokenizer is already integrated.** Production commit `b6efcb89ddee034d33148d66d8c3d6c755056cf7` fast-forwards `f9b85f7725aac6383098abc14ed201e32a1591c5`; it passed [full CI 36169997020](https://github.com/Laisky/one-api/actions/runs/36169997020), including all Go shards, race/coverage completeness, static/vulnerability checks and `CI required`. Frontend tests were path-filtered, not rebuilt. Evidence publication `298d239451aaab7a21478904ee9c8ac7a3e8d0e5` changes seven test/documentation files only.

**This tree additionally guards BPE allocation-size arithmetic following CodeQL review 4107214497.** See [integration validation](results/20260925-byte-budget/INTEGRATION.md): exact token/race/vet/boundary checks, an intended failing negative control, and an independent 640-request correctness comparison passed. This guard changes the binary identity; it does not create a new speedup claim or replace the full earlier A/B study. Final-head CI and CodeQL must be checked separately; do not transfer b6efcb8's green result to this newer tree or call an old PR description authoritative.

[Accepted report](results/20260925-byte-budget/REPORT.md), [40 final runs](results/20260925-byte-budget/final-runs.csv), [manifest](results/20260925-byte-budget/manifest.json), and [profiling protocol](PROFILING.md) are the current entry points. The original local final candidate was `eee3e056801ae7ed45abc329be86a9d261765df6`, not an original GitHub commit. Its implementation/tests/license were uploaded as the exact verified blobs in b6efcb8. The later arithmetic guard is explicitly documented in the upstream maintenance note.

## Completed experiments: do not repeat or reapply

| Evidence | Decision and boundary |
| --- | --- |
| [Original 142 trials](results/20260924/REPORT.md) | Retain Builder accumulation and buffered SSE-line fast path. Formatting shortcut rejected; adverse cells retained. |
| [76-trial follow-up](results/20260924-followup/REPORT.md) | Retain equivalent EncodeOrdinary counting. Different CPU/session; do not compare absolute capacities across studies. |
| [40-trial coalescing](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787) | Reject `3d671dd7a9fe18865aad1b05be4527f5295d8749`: long/c8 TTFT +16.55% / +71.01 ms. Keep delivery tests and SQLite statistics repair. |
| [40-trial copy study](results/20260925-copy/REPORT.md) | Reject SSE-copy elimination: long/c8 TTFT +17.75% / +12.02 ms and insufficient repeat-consistent material benefit. |
| [80-trial token-count study](results/20260925-token-count/REPORT.md) | Two separate complete rejections, each 40 trials / 15,360 requests. Count-only traversal lacked material repeat-consistent benefit; stack scratch failed RSS/continuity/tail gates. Their dependency copies are not adopted. |
| [Split-only](results/20260925-pretokenization/SPLIT_ONLY.md) | Completed rejection, distinct from the subsequently accepted full-token/scheduling bundle. |
| [Quantum64](results/20260925-burst-reset/QUANTUM64.md) | Completed rejection. Comment 5836195544 registered a separate quantum32 refinement; no completed result was established at integration. Do not silently combine it with the byte budget. |
| [Byte-budget campaign](results/20260925-byte-budget/REPORT.md) | Three separate 40-trial studies: encoder-local rejected; stream-boundary passed; final timeout-compatible confirmation passed. Total 120 trials / 46,080 verified requests, not one pooled experiment. Only final-confirmation numbers describe the measured accepted bundle. |

The accepted study's ten long-stream pairs improved throughput, CPU/request and P95 completion. Memory gains are modest; short/c8 RSS +0.45% and short/c64 request-max gap +23.01% / +2.44 ms remain visible. Do not claim universal latency/memory gains, significance from pooled requests, a global optimum, or sustained production capacity. The guard's 640-request smoke is separate correctness evidence, not a fourth optimization study.

Retained guards include authentication/quota, exact full-token counting, production logging/tracing, durable usage, immediate per-event flush, invalid-auth/bad-stream controls, cancellation cleanup, SQLite usage-statistics retries, content-gap telemetry, atomic failed-study evidence and authoritative comparison arguments. Usage-statistics checks are not an independent financial-ledger audit. There is no count approximation, synthetic-only text cache, asynchronous batching, semaphore or deferred accounting in this bundle. A 4 KiB scheduling budget does not bound a single large BPE piece's processing time.

## Historical branch inventory

These eight helper refs were last rechecked on September 25, 2026 and are frozen by execution policy, not deleted or technically archived. Integration created no additional ref. Ahead-of-PR is not approval to adopt an experiment.

| Branch | Historical tip | Disposition |
| --- | --- | --- |
| `perf/stream-chat-e2e-20260924` | Read current PR head | Sole active delivery branch. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` | Delivery/SQLite repair retained; omit temporary workflow/encoded patch. |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` | Reporter/continuity/failed-study changes already retained. |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` | Rejected coalescing build/evidence, not production. |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` | Historical blob preparation only. |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` | Superseded coalescing patch/workflow. |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` | Oversized tooling export; do not use as recovery route. |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` | Workflow-only differences; production follow-up retained. |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` | Reuse runtime artifact, not obsolete source as baseline. |

## Source, builds and evidence recovery

| Recovery input | Run / artifact | Archive SHA-256 |
| --- | --- | --- |
| Go 1.27.1 / vendor / token cache | `36045112587` / `10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` |
| Immutable 299deff / harness | `36084329116` / `10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` |
| 21dc0af source/build | `36068339476` / `10836434460` | `14738f5b1207dcad408c138fe9eae3e30eae1dab06c06d5128f57df66e96e36d` |
| Copy-study archive | `one-api-pr427-copy-evidence.zip` | `97a97fe2d3857bd51def974b2a66a2a519a9a504057a32e0e3055e076bda138f` |
| Accepted byte-budget archive | `one-api-pr427-byte-budget-evidence.zip` | `e09795b55fc81ad0006c7b8fa9f24011bb43da6a7200252f54084e300d5c5e83` |

Integration re-audited all 263 covered byte-budget archive files, all three study decisions and eight independent audit self-tests. The source manifest verifies 954 baseline production/dependency/embed inputs; later retained user/channel blobs are `f0a66e703258c63f3436e80e88a91bf4f07e646e` and `59862525884dae9af6705fef3af787ea7a5092bc`. Local reconstructed commits are not original GitHub commits. Verify actual downloaded paths and archive hashes; retention is finite.

The pre-guard integration rebuild exactly reproduced measured gateway SHA-256 `6550a116e0d978f4ce7d5f323bc69ab83321adfe484fab15f154f404f993304b` and driver `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`, using Go 1.27.1, `-p=2 -mod=vendor -trimpath -buildvcs=false` and identical embed/dependency inputs. The later guarded build is `13a1ec75315310d1ada3f9b343d63ccb3770ffcc604d4790e92a0e0344a51f3b`. Keep these identities distinct. Standard public CI builds may differ in VCS/embed metadata and cannot be relabeled as measured binaries.

Direct clone failed in the recovered runtime; do not repeatedly retry it or create another helper branch. Artifact 10844767167 is 585,766,240 bytes, beyond the connector's 536,870,912-byte limit. Exclude sensitive local instructions from exports. The older `one-api-pr427-20260925-evidence.zip` / `f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad` remains separate and was not newly reverified.

The committed final-run CSV has nine-significant-digit metrics and validates performance arithmetic only. Full-precision per-request timings, raw qualifications, profiles and `verify_all.py` remain in the separately delivered byte-budget archive, not in this tree. The post-guard integration logs/raw smoke records are separate again. Do not conflate archive-only audit tests with repository CI tests.

## Acceptance and next action

Keep the [registered acceptance contract](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402): independent stream/cancellation/auth/usage qualification; ten-second settlement; five alternating pairs per short/long c8/c64 cell; long unpaced 1,024 x 128 bytes / 256 requests and short 32 x 128 bytes / 2 ms / 512 requests. Require at least 5% paired-median long-stream throughput or CPU benefit with favorable direction in at least 4/5 pairs. Reject short throughput/CPU regressions above 5%, latency regressions exceeding both 10% and 5 ms, and median paired RSS growth above 10%. Register justified changes before measurements, never to rescue a failed candidate.

First inspect current-head CI, CodeQL and new review findings. The byte-budget optimization is already applied: do not repeat 120 completed trials or reapply the patch merely to recover context. Verify the arithmetic guard's final public checks rather than relying on b6efcb8. The bounded smoke does not certify its exact performance delta.

Further tuning follows [PROFILING.md](PROFILING.md): calibrate/freeze sustained workload, qualify stable windows above 50% of effective machine CPU allowance **for the gateway**, capture CPU and memory in separate diagnostic runs, identify the largest actual cost, test one mechanism, and then run unprofiled immutable A/B. Report machine and GOMAXPROCS-normalized utilization separately; mock/client load cannot satisfy the gateway gate. Memory occupancy is not a target to fill. The old short trials do not retroactively satisfy the new sustained protocol.

Preserve full token IDs, invalid UTF-8, Unicode whitespace/contractions, special/custom patterns, finite-timeout fallback and concurrency. Keep original tiktoken as independent oracle and retain upstream license/maintenance notes. `compare.sh` reuses a validated read-only token cache and places managed binary/baseline/driver/cache/output arguments after caller options. It deliberately does not forward profiling environment variables. Explicit diagnostic setup must isolate the network: the pprof endpoint binds loopback, but the gateway API currently binds all interfaces.

Use detached local worktrees for one registered candidate; no new remote branch. Preserve all adverse/failed/aborted runs. Commit accepted work incrementally here. Before stopping, record actual phase, source/binary identities, completed trial counts, evidence locations, checks and the concrete next action. Never leave the only recovery instructions in chat or treat a partial/mixed-binary study as complete.
