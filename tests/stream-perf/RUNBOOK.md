# PR 427: single execution and recovery record

## Authority and current state

Continue only in [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. Do not create remote experiment branches or PRs, merge historical branches wholesale, force-push, delete evidence refs, or change main directly. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient.

Retained production remains `299deffa58aa1e87aa699a038484cbdaae3ab450`. Commit `88fdd66985af1ff7bfce8b6998c1923f352e3b11` fixes managed comparison arguments. Its [full CI run 36090527420](https://github.com/Laisky/one-api/actions/runs/36090527420) passed; the preceding consolidation head d1f7c95 also passed run 36088839322. Always inspect the actual current head: an earlier green run does not certify a later evidence/test commit. Frontend tests were path-filtered, not rebuilt.

**Latest iteration is complete and the candidate is rejected.** [Report, exact-source limitations and reproduction](results/20260925-copy/REPORT.md), [40-run ledger](results/20260925-copy/runs.csv), [identities](results/20260925-copy/manifest.json), and [unapplied production patch](results/20260925-copy/rejected-production.patch) are committed together. The full per-request/qualification/profile archive is linked in the PR discussion and conversation, not embedded in the repository.

The redundant-copy candidate completed 40 trials / 15,360 verified requests, zero failures/drops, exact paired usage. It fails both the long/c8 TTFT gate (+17.75% / +12.02 ms) and the material, repeat-consistent long-stream benefit gate. Long/c64 RPS gained 11.64% but improved in only 3/5 pairs; long/c8 CPU improved 3.62%, below 5%. Allocation microbenchmarks are not evidence of lower end-to-end RSS. Do not reapply this patch or the older rejected flush-coalescing implementation.

Retained from this iteration: authoritative binary/baseline/driver/cache/output arguments, three negative-controlled option regressions, and differential SSE byte/framing/header/error/flush and normalization tests. All 40 Python tests passed locally. The new Go guards passed three race repetitions on retained production; deliberately broken flush/normalization controls failed. Six archive-only audit self-tests passed, but are not claimed as extra repository CI tests. The copy candidate's zero-allocation assertion is not retained.

Earlier retained work: streaming E2E framework, Builder accumulation, already-buffered SSE-line fast path, equivalent ordinary token encoding, GPT-5.2 sampling/default fixes, host/offline-cache checks, bounded SQLite usage-statistics busy retry, gated HTTP delivery tests, content-gap telemetry and failed-study accounting evidence. Authentication, quota, exact token counting, logging, tracing and durable accounting stay enabled.

## Reconciled branch inventory

These nine names and original tips were inspected during consolidation on September 25, 2026; the continuation page was empty. The delivery branch has advanced since this historical snapshot. The eight auxiliary refs are frozen by execution policy, not deleted or technically archived. Ahead-of-PR does not mean approved.

| Branch | Original inspected tip | Disposition |
| --- | --- | --- |
| `perf/stream-chat-e2e-20260924` | `299deffa58aa1e87aa699a038484cbdaae3ab450` | Sole active delivery branch; use its current head. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` | Delivery/SQLite repair already retained; do not merge temporary workflow or encoded patch. |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` | Reporter, gap and failure-evidence work already retained; no carrier workflow to merge. |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` | Builds rejected coalescing code; preserve as evidence, not production. |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` | Historical source-blob preparation; no outstanding production feature. |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` | Superseded experimental patch/workflow. |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` | Tooling export only; archive exceeds connector download limit. |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` | Extra changes are build-workflow-only; production follow-up is retained. |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` | Reuse runtime artifact, not obsolete source as a new baseline. |

## Evidence and source recovery

[Original 142-trial report](results/20260924/REPORT.md), [76-trial ordinary-encoding follow-up](results/20260924-followup/REPORT.md), [coalescing rejection and 40-run ledger](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787), and [consolidation validation](results/20260925-consolidation/VALIDATION.md) remain distinct studies. Do not pool measurements from different sessions/CPUs or relabel historical binaries.

The older raw archive `one-api-pr427-20260925-evidence.zip` was reported with SHA-256 `f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad`. It is not a repository file and was not reverified during this iteration. The latest copy study has a separate archive and manifest.

| Recovery input | Run / artifact | Archive SHA-256 | Limits |
| --- | --- | --- | --- |
| Go 1.27.1, vendored dependencies, tokenizer cache | `36045112587` / `10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` | Source is historical 36829cf; replace with verified retained sources. |
| Immutable 299deff control, rejected 3d671dd candidate and harness | `36084329116` / `10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` | Diagnostic control is usable; rejected candidate must not be deployed. |
| 21dc0af source/build artifact | `36068339476` / `10836434460` | `14738f5b1207dcad408c138fe9eae3e30eae1dab06c06d5128f57df66e96e36d` | Apply and verify later source changes; local recovery commits are not GitHub originals. |

Availability is finite. Verify availability and hashes; never infer a local path. Artifact 10844767167 is 585,766,240 bytes, exceeding the 536,870,912-byte connector limit. Do not repeat that route or create another helper branch. Exclude sensitive local instruction files from exported recovery packages.

The latest study rebuilt both variants locally from recovered sources with the same Go 1.27.1, vendored dependencies and embed assets, using `-p 2 -mod=vendor -trimpath -buildvcs=false`. User/channel blobs were verified against 299deff, and go.mod/go.sum match the dependency bundle. Exact local identities, binary hashes and limitations are in the latest manifest/report. Do not label these reconstructed commits as the original GitHub commits or future heads.

## Quantitative decision contract

Keep the [original rule](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402), also explicitly retained by the [copy experiment plan](https://github.com/Laisky/one-api/pull/427#issuecomment-5826152136). Any justified change must be recorded before a new measurement, not chosen to rescue an unfavorable result.

1. Both immutable variants independently pass exact content, framing, cancellation and usage qualification. Paired durable usage matches; settlement deadline remains ten seconds.
2. Five alternating pairs in each of four cells: concurrency 8/64; long unpaced 1,024 x 128-byte chunks, 256 requests/run; short 32 x 128-byte chunks at 2 ms/chunk, 512 requests/run.
3. At least 5% paired-median long-stream RPS or CPU/request benefit with favorable direction in at least 4/5 pairs. Reject short-stream RPS/CPU median regression greater than 5%.
4. Reject paired-median regressions exceeding both 10% and 5 ms in TTFT, completion or p95 request-max content gap. Reject RSS paired-median increase greater than 10%.
5. Retain adverse cells, qualification failures and aborted studies. No mixed-binary resume, favorable-subset replacement, pooled-request significance claims, or production capacity guarantee.

## Next bounded action

First inspect current-head CI/review status and finish any genuine failures. For another performance iteration, use the retained branch and a detached local worktree. The diagnostic identifies exact token encoding as the dominant sampled CPU cost; investigate allocation/algorithm costs while preserving complete token sequences, quota enforcement, final usage and cancellation. Do not substitute approximate counting, disable logging/accounting, cache synthetic-only repeated content, or reintroduce delayed SSE delivery.

Profile and predeclare one candidate before measurement. Preserve unchanged behavior with differential and real-HTTP tests; run the complete registered A/B matrix; retain only a candidate that meets every gate. Rejected code belongs in an unapplied patch and evidence, not an active remote branch. No global optimum is claimed by this completed rejection.

`compare.sh` honors `TIKTOKEN_CACHE_DIR`, resolves relative paths before worktree changes, validates supplied assets without downloading/writing, and fails before compilation on missing/corrupt data. The generated five managed arguments follow caller workload options and cannot be replaced by them. It records the actual cache path. A fully offline build still requires the compiler and dependencies to be available.

Before ending a session, update this record, commit finished work, and record exact head, current CI and evidence location in the PR. Keep unfinished studies explicitly incomplete; never leave the only recovery instructions in chat.
