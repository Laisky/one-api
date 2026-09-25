# PR 427: single execution and recovery record

## Authority and current state

Continue only in [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. Do not create another remote branch or PR, merge historical branches wholesale, force-push, delete evidence refs, or change main directly. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient.

**Integrated production checkpoint, September 25, 2026: `b6efcb89ddee034d33148d66d8c3d6c755056cf7`.** It is a fast-forward from actual prior head `f9b85f7725aac6383098abc14ed201e32a1591c5`, not the older SHA in the stale PR description. Its [complete CI run 36169997020](https://github.com/Laisky/one-api/actions/runs/36169997020) passed all five Go shards, race/coverage completeness, static and vulnerability checks, coverage report and `CI required`. Frontend tests were path-filtered, not rebuilt. Later documentation/test commits require their own checks; do not transfer this green result to a newer head.

The previously interrupted **4 KiB byte-budget + exact canonical tokenizer** candidate is now applied, not merely delivered as a local patch. It is byte-identical in implementation/tests/license to final local candidate `eee3e056801ae7ed45abc329be86a9d261765df6`. The original retained baseline was `299deffa58aa1e87aa699a038484cbdaae3ab450`. The integration rebuild exactly reproduces the measured gateway and driver hashes below. All later historical reports and helper refs are preserved. No force push, temporary workflow or automatic merge occurred.

[Accepted bundle report](results/20260925-byte-budget/REPORT.md), [40 final run records](results/20260925-byte-budget/final-runs.csv), and [source/build manifest](results/20260925-byte-budget/manifest.json) are the current evidence entry points. Three distinct studies comprise 120 trials and 46,080 verified requests: encoder-local rejected; stream-boundary passed; final timeout-compatible confirmation passed. Only final-confirmation numbers describe the retained bundle. The earlier private copies in rejected experiments remain rejected; this is a different, full-token implementation with independent oracle tests.

The separate [quantum64 report](results/20260925-burst-reset/QUANTUM64.md) is a completed rejection. [Comment 5836195544](https://github.com/Laisky/one-api/pull/427#issuecomment-5836195544) registered a quantum32 refinement, but no completed result was established at integration. Do not combine that pending line-count candidate with this byte budget or overwrite independently advancing evidence. Read the current head and comments before the next write.

## Completed experiments: do not repeat or reapply

| Evidence | Decision and boundary |
| --- | --- |
| [Original 142 trials](results/20260924/REPORT.md) | Retain Builder accumulation and already-buffered SSE-line fast path. Formatting shortcut rejected. Keep adverse observations. |
| [76-trial follow-up](results/20260924-followup/REPORT.md) | Retain equivalent `EncodeOrdinary` path. Different CPU/session; do not compare absolute capacities across studies. |
| [40-trial coalescing report](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787) | Reject bounded flush coalescing `3d671dd7a9fe18865aad1b05be4527f5295d8749`: long/c8 TTFT +16.55% / +71.01 ms. Preserve the useful delivery tests and SQLite statistics repair. |
| [40-trial copy report](results/20260925-copy/REPORT.md) | Reject redundant SSE-copy candidate: long/c8 TTFT +17.75% / +12.02 ms; no repeat-consistent material long-stream benefit. Microbenchmark allocations are not live-memory gains. |
| [80-trial token-count report](results/20260925-token-count/REPORT.md) | Two separate complete 40-trial studies, each with 15,360 verified requests and exact paired usage. Traversal lacked material repeat-consistent benefit. Stack scratch failed RSS/continuity/tail-latency gates. Neither count-only implementation nor its private dependency copy was adopted. |
| [Split-only pretokenization](results/20260925-pretokenization/SPLIT_ONLY.md) | Completed rejection; do not restore it simply because canonical splitting now belongs to a separately tested scheduling bundle. |
| [64-line scheduling](results/20260925-burst-reset/QUANTUM64.md) | Completed rejection. Its 32-line refinement is distinct from the accepted byte-budget bundle. |
| [120-trial byte-budget campaign](results/20260925-byte-budget/REPORT.md) | Preserve all three experiments independently. Integrate only the final full-token canonical + cross-call 4 KiB + finite-timeout guard candidate. |

The token traversal candidate removed all-match indices, a duplicate rune slice and the final token-ID array, retaining the regex and merge kernel. The stack candidate added bounded merge scratch with a heap fallback. These are completed rejections, not interrupted candidates. Their report, manifest and 80-run ledger were committed by `86cee65`. Preserve these exact original source and binary identities.

Retained guards: real-gateway streaming E2E; Builder accumulation; buffered-line fast path; exact full token sequences; GPT-5.2 defaults/sampling repair; local/CI host prerequisites; offline cache validation; bounded SQLite usage-statistics retry; gated real-HTTP delivery tests; content-gap telemetry; atomic failed-study accounting evidence; authoritative comparison arguments; differential SSE byte/header/error/flush and normalization tests. Authentication, quota, exact counting, logging/tracing, immediate delivery and durable usage remain enabled. Usage-statistics qualification is not an independent financial-ledger audit.

## Reconciled branch inventory

Historical tips last rechecked on September 25, 2026. These eight auxiliary refs are frozen by execution policy, not deleted or technically archived. An ahead-of-PR branch is not an approved optimization. Integration created no additional ref.

| Branch | Historical tip | Disposition |
| --- | --- | --- |
| `perf/stream-chat-e2e-20260924` | Read current PR head | Sole active delivery branch. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` | Delivery/SQLite repair retained; no temporary workflow or encoded patch to merge. |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` | Reporter, continuity and failed-study changes already retained. |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` | Rejected coalescing build/evidence, not production. |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` | Historical source-blob preparation only. |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` | Superseded coalescing patch/workflow. |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` | Tooling export; oversized artifact is not the recovery route. |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` | Workflow-only differences; production follow-up retained. |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` | Reuse runtime artifact, not obsolete source as a new baseline. |

## Reusable source and evidence

| Recovery input | Run / artifact | Archive SHA-256 |
| --- | --- | --- |
| Go 1.27.1, vendored dependencies, tokenizer cache | `36045112587` / `10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` |
| Immutable 299deff control and harness | `36084329116` / `10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` |
| 21dc0af source/build artifact | `36068339476` / `10836434460` | `14738f5b1207dcad408c138fe9eae3e30eae1dab06c06d5128f57df66e96e36d` |
| Previous copy-study conversation archive | `one-api-pr427-copy-evidence.zip` | `97a97fe2d3857bd51def974b2a66a2a519a9a504057a32e0e3055e076bda138f` |
| Accepted byte-budget conversation archive | `one-api-pr427-byte-budget-evidence.zip` | `e09795b55fc81ad0006c7b8fa9f24011bb43da6a7200252f54084e300d5c5e83` |

The previous recovery verified the first four archive hashes. Integration re-audited all 263 covered files in the byte-budget archive and all three raw studies, including eight independent audit tests. Its manifest verifies 954 baseline production/dependency/embed inputs after applying the two later user/channel changes. Their exact Git blobs are `f0a66e703258c63f3436e80e88a91bf4f07e646e` and `59862525884dae9af6705fef3af787ea7a5092bc`. Local reconstructed commits are not original GitHub commits. Record recovery and public identities separately.

Fresh integration builds exactly match measured final gateway SHA-256 `6550a116e0d978f4ce7d5f323bc69ab83321adfe484fab15f154f404f993304b` and driver `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`, using Go 1.27.1 and `-p=2 -mod=vendor -trimpath -buildvcs=false`. Tokenizer race tests passed; renderer/SSE and OpenAI token/stream-focused race tests passed three repetitions; focused vet and builds passed. Six added compact-ledger tests pass. This is an integration revalidation, not a fourth performance experiment. Public final-head CI remains a separate requirement.

Direct Git clone failed in the recovered environment. Do not repeatedly attempt clone or export another helper branch. Artifact 10844767167 is 585,766,240 bytes, exceeding the connector's 536,870,912-byte limit. Exclude sensitive local instructions from any export. Verify exact paths and hashes rather than inventing artifact locations. Retention is finite. Standard rebuilds can differ in VCS/embed metadata and must record their own binary hashes.

The older `one-api-pr427-20260925-evidence.zip`, SHA-256 `f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad`, remains separate and was not newly recovered. Conversation-only request samples, profiles and audit tests are not committed repository files unless explicitly added and verified. The new committed final-run ledger is rounded to nine significant digits and validates performance arithmetic only; full qualification and original precision require the named archive and `verify_all.py`.

## Quantitative acceptance contract

Keep the [registered thresholds](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402); document any justified change before measuring, not to rescue a rejected result.

1. Qualify both immutable variants independently for exact stream content/framing, malformed/truncated controls, authentication, cancellation and durable usage. Every paired usage total must match. Settlement deadline remains ten seconds.
2. Five alternating pairs in four cells: concurrency 8/64; long unpaced 1,024 x 128-byte chunks, 256 requests/run; short 32 x 128-byte chunks at 2 ms/chunk, 512 requests/run.
3. Require at least 5% paired-median long-stream RPS or CPU/request benefit with the same favorable direction in at least 4/5 pairs. Reject short-stream RPS/CPU regressions greater than 5%.
4. Reject paired-median regressions exceeding both 10% and 5 ms in TTFT, completion or p95 request-max content gap; reject a paired-median RSS increase greater than 10%.
5. Publish all adverse cells, failed qualifications and aborted studies. No mixed-binary resume, favorable-subset substitution or significance claims from pooled request samples. These are operational gates, not confidence intervals or production capacity guarantees.

The accepted final bundle passed those rules. Long-stream throughput/CPU/completion improved in all ten pairs. Short/c64 request-max gap increased 23.01% but only 2.44 ms; short/c8 RSS increased 0.45%, and single-run outliers remain. Do not claim universal memory savings or deterministic latency improvement.

## Next action and interruption rules

First inspect the current PR head, checks and new review findings. The production integration is already complete; do not rerun 120 trials merely to recover context or reapply the byte-budget patch. The current evidence-publication commit changes only tests/docs/records, not the measured production implementation.

Further tuning follows [PROFILING.md](PROFILING.md): calibrate and freeze a sustained workload, target gateway utilization above 50% of effective machine CPU allowance, verify stable windows, capture CPU and memory in separate diagnostic runs, identify the largest actual cost, test one mechanism, then run unprofiled immutable A/B. Report both machine and GOMAXPROCS-normalized utilization; mock/client CPU cannot satisfy the gateway-load gate. The old short trials are not retroactively labeled as a sustained-load study. Memory occupancy is not a target to fill.

Preserve full token sequences, invalid UTF-8, Unicode whitespace/contractions, special/custom encodings, finite-timeout fallback and shared-encoder concurrency. Keep the original dependency as an independent oracle and its upstream license with the internal copy. No approximate counts, disabled logging/accounting, synthetic-only caches, deferred flushing or relaxed latency gates. A single large token piece is not given a hard latency bound by a 4 KiB between-piece budget.

Register a distinct candidate before measuring. Local detached worktrees are allowed; additional remote branches are not. Keep a rejected implementation as an unapplied patch with complete evidence. Commit accepted changes and their tests incrementally to this PR only. The existing comparison fixture does not automatically enable pprof from shell variables; its gateway API binds all interfaces, so use explicit isolated diagnostic setup as documented rather than exposing it publicly.

`compare.sh` honors a prepared `TIKTOKEN_CACHE_DIR`, validates it read-only and records its actual path. Managed binary/baseline/driver/cache/output options follow workload options so the caller cannot silently replace recorded identities. Fully offline execution still needs the compiler/dependencies. Setup or build failure is not a measured trial.

Before stopping, update this file with actual phase, candidate/source hashes, completed trial count, evidence paths, current checks and concrete next step. An incomplete experiment stays incomplete; never leave its only recovery information in chat.
