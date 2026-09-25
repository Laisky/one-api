# PR 427: single execution and recovery record

## Authority and current state

Continue only in [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. Do not create another remote branch or PR, merge historical branches wholesale, force-push, delete evidence refs, or change main directly. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient.

Recovery checkpoint, September 25, 2026: actual head before this documentation repair is `86cee651ea3cc1521b6e2bf3ea1752a8c41fef2a`, not the stale `13cd2ee` in the PR description. Its [CI run 36134339374](https://github.com/Laisky/one-api/actions/runs/36134339374) and CodeQL run 36134334032 both passed. Always inspect current-head checks separately. Frontend tests were path-filtered, not newly built.

Retained production remains `299deffa58aa1e87aa699a038484cbdaae3ab450`. The interruption occurred after publishing two complete rejected token-count experiments but before updating this recovery record and the PR description. There is no pending production candidate to adopt. The eight historical helper refs below were rechecked and have not advanced. No new remote branch is needed.

Current work: investigate the remaining ordinary-token pre-tokenization cost without replaying rejected changes. Recover and verify existing source/runtime artifacts, then test a distinct candidate locally. No new candidate has passed E2E acceptance at this checkpoint. Do not label profiling, microbenchmarks or a partial matrix as a retained performance improvement.

## Completed experiments: do not repeat or reapply

| Evidence | Decision and boundary |
| --- | --- |
| [Original 142 trials](results/20260924/REPORT.md) | Retain Builder accumulation and already-buffered SSE-line fast path. Formatting shortcut rejected. Keep adverse observations. |
| [76-trial follow-up](results/20260924-followup/REPORT.md) | Retain equivalent `EncodeOrdinary` path. Different CPU/session; do not compare absolute capacities across studies. |
| [40-trial coalescing report](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787) | Reject bounded flush coalescing `3d671dd7a9fe18865aad1b05be4527f5295d8749`: long/c8 TTFT +16.55% / +71.01 ms. Preserve the useful delivery tests and SQLite statistics repair. |
| [40-trial copy report](results/20260925-copy/REPORT.md) | Reject redundant SSE-copy candidate: long/c8 TTFT +17.75% / +12.02 ms; no repeat-consistent material long-stream benefit. Microbenchmark allocations are not live-memory gains. |
| [80-trial token-count report](results/20260925-token-count/REPORT.md) | Two separate complete 40-trial studies, each with 15,360 verified requests and exact paired usage. Traversal lacked material repeat-consistent benefit. Stack scratch failed RSS/continuity/tail-latency gates. Neither count-only implementation nor its private dependency copy was adopted. |

The token traversal candidate removed all-match indices, a duplicate rune slice and the final token-ID array, retaining the regex and merge kernel. The stack candidate added bounded merge scratch with a heap fallback. These are completed rejections, not interrupted candidates. Their report, manifest and 80-run ledger were committed by `86cee65`. Preserve these exact original source and binary identities.

Retained implementation and guards: real-gateway streaming E2E; Builder accumulation; buffered-line fast path; equivalent ordinary encoding; GPT-5.2 defaults/sampling repair; local/CI host prerequisites; offline cache validation; bounded SQLite usage-statistics retry; gated real-HTTP delivery tests; content-gap telemetry; atomic failed-study accounting evidence; authoritative comparison arguments; differential SSE byte/header/error/flush and normalization tests. Authentication, quota, exact counting, logging/tracing, immediate delivery and durable usage remain enabled. Usage-statistics qualification is not an independent financial-ledger audit.

## Reconciled branch inventory

Exact historical tips rechecked on September 25, 2026. These eight auxiliary refs are frozen by execution policy, not deleted or technically archived. An ahead-of-PR branch is not an approved optimization.

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

These four archive hashes were reverified during recovery. The copy-study source manifest verifies 954 retained production/dependency/embed-input files after applying the two later user/channel changes. Their exact Git blobs are `f0a66e703258c63f3436e80e88a91bf4f07e646e` and `59862525884dae9af6705fef3af787ea7a5092bc`. Local reconstructed commits are not original GitHub commits. Record both the recovery identity and original provenance; freshly build both sides of an A/B study with the same compiler, flags, embeds and dependencies.

Direct Git clone remains unavailable in the recovered execution environment. Do not repeatedly attempt clone or export another helper branch. Artifact 10844767167 is 585,766,240 bytes, exceeding the connector's 536,870,912-byte limit. Exclude sensitive local instructions from any export. Verify exact paths and hashes rather than inventing artifact locations. Retention is finite.

The older `one-api-pr427-20260925-evidence.zip`, SHA-256 `f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad`, is separate and was not newly recovered. Conversation-only request samples, profiles and audit tests are not committed repository files unless explicitly added and verified. Preserve each experiment's provenance independently.

## Quantitative acceptance contract

Keep the [registered thresholds](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402); document any justified change before measuring, not to rescue a rejected result.

1. Qualify both immutable variants independently for exact stream content/framing, malformed/truncated controls, authentication, cancellation and durable usage. Every paired usage total must match. Settlement deadline remains ten seconds.
2. Five alternating pairs in four cells: concurrency 8/64; long unpaced 1,024 x 128-byte chunks, 256 requests/run; short 32 x 128-byte chunks at 2 ms/chunk, 512 requests/run.
3. Require at least 5% paired-median long-stream RPS or CPU/request benefit with the same favorable direction in at least 4/5 pairs. Reject short-stream RPS/CPU regressions greater than 5%.
4. Reject paired-median regressions exceeding both 10% and 5 ms in TTFT, completion or p95 request-max content gap; reject a paired-median RSS increase greater than 10%.
5. Publish all adverse cells, failed qualifications and aborted studies. No mixed-binary resume, favorable-subset substitution or significance claims from pooled request samples. These are operational gates, not confidence intervals or production capacity guarantees.

## Next action and interruption rules

Investigate a distinct pre-tokenization improvement, using the original dependency as an independent behavioral oracle. Do not repeat the count-only traversal or stack candidates. Keep full token sequences, invalid-UTF-8 behavior, Unicode categories, whitespace/contractions, custom-encoding fallback and concurrency correct. No approximate counts, disabled logging/accounting, synthetic-only content cache, deferred flushing or relaxed latency gate.

Register the candidate before its E2E matrix. Local detached worktrees are allowed; additional remote branches are not. Keep a rejected implementation as an unapplied patch with complete evidence. Commit accepted changes and their tests incrementally to this PR only.

`compare.sh` honors a prepared `TIKTOKEN_CACHE_DIR`, validates it read-only and records its actual path. Managed binary/baseline/driver/cache/output options follow workload options so the caller cannot silently replace recorded identities. Fully offline execution still needs the compiler/dependencies. Setup or build failure is not a measured trial.

Before stopping, update this file with the actual phase, candidate/source hashes, completed trial count, evidence paths, current checks and concrete next step. An incomplete experiment stays incomplete; never leave its only recovery information in chat.
