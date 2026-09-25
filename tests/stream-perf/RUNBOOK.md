# PR 427: single execution and recovery record

## Authority and current state

Continue this work only in [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. The owner requested consolidation on September 25, 2026. Do not create another remote experiment branch or PR, merge historical experiment branches wholesale, force-push this branch, or change `main` directly.

The retained production revision is `299deffa58aa1e87aa699a038484cbdaae3ab450`. Its [full CI run 36084278200](https://github.com/Laisky/one-api/actions/runs/36084278200) passed. Later documentation or harness changes do not turn that run into evidence for a newer head: inspect the current PR checks separately. Frontend tests at the retained revision were path-filtered, not rebuilt.

Retained work: streaming E2E framework; Builder accumulation; already-buffered SSE-line fast path; equivalent ordinary token encoding; GPT-5.2 sampling/default fixes; host/offline-cache prerequisites; SQLite usage-statistics busy retry; gated real-HTTP delivery tests; inter-content-gap telemetry; and failed-study accounting evidence. Keep authentication, quota, durable usage accounting, logging and tracing enabled.

The native SSE flush-coalescing candidate `3d671dd7a9fe18865aad1b05be4527f5295d8749` was **rejected**, not left awaiting adoption. Its throughput gains did not excuse its first-content latency regression. Do not restore its buffered writer simply because a helper branch is ahead of the PR.

## Reconciled branch inventory

Snapshot verified against GitHub on September 25, 2026. The search for branch names containing `stream` returned these nine branches; the continuation page was empty. Ahead/behind counts compare each recorded tip with `299deff`, not with a later documentation commit. Diverged comparisons describe changes from their merge base and are not instructions to overwrite the PR tree.

| Branch | Recorded tip | Ahead / behind | Disposition |
| --- | --- | ---: | --- |
| `perf/stream-chat-e2e-20260924` | `299deffa58aa1e87aa699a038484cbdaae3ab450` | 0 / 0 | Sole active delivery branch. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` | 2 / 1 | Historical delivery/SQLite validation. The PR retains the plain-code repair and tests; do not merge its temporary workflow or encoded patch. |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` | 1 / 1 | Historical content-gap/failed-study patch carrier. The corresponding reporter/runner changes and tests are already in the retained PR. Do not reapply the carrier or its workflow. |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` | 2 / 0 | Builds the rejected coalescing candidate. Ahead is not acceptance. Keep its patch and measurements as historical evidence, not production code. |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` | 3 / 1 | Historical Git-blob preparation, based on acceptance work. It explicitly stored source objects without moving a ref. No additional production feature to merge. |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` | 1 / 1 | Older coalescing patch carrier and temporary build workflow; superseded by the completed rejection experiment. |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` | 1 / 0 | Tooling export only. Successful artifact is larger than the connector download limit; use the recovered smaller runtime below. Do not merge its workflow. |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` | 1 / 3 | Extra changes are build-workflow-only; production follow-up is already retained in the PR. |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` | 1 / 13 | Historical offline runtime export; reuse its artifact, not its obsolete source revision as the new baseline. |

These eight auxiliary refs are frozen by execution policy, not deleted or technically archived in GitHub. Preserve their exact tips until evidence retention and owner-authorized deletion are settled. No unresolved candidate is made safe by consolidating its branch name.

## Evidence and environment recovery

- [Original 142-trial study](results/20260924/REPORT.md): immutable historical baseline and Builder/SSE improvements; retains the rejected formatting shortcut and adverse cells.
- [76-trial ordinary-encoding follow-up](results/20260924-followup/REPORT.md): separate host/session and binaries; do not pool its absolute capacity with the original study.
- [Completed coalescing rejection report and 40-run ledger](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787): 40 A/B trials, 15,360 verified requests, rejected latency tradeoff. [Predeclared decision rule](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402).
- The latest full raw archive was reported separately as `one-api-pr427-20260925-evidence.zip`, SHA-256 `f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad`. It is **not a committed repository file**. The PR discussion is accessible; do not claim that the raw archive has been recovered or independently reverified from that discussion alone.

Recovered existing build artifacts, without creating a branch or running another build workflow:

| Purpose | Workflow run / artifact ID | Archive SHA-256 | Limits |
| --- | --- | --- | --- |
| Go 1.27.1, vendored dependencies and tokenizer cache | `36045112587` / `10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` | Source is historical `36829cf`; replace it with verified retained source before building a new candidate. |
| Immutable `299deff` control, rejected `3d671dd` candidate, harness and patch | `36084329116` / `10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` | Reuse the control/harness for diagnostics; do not deploy the rejected candidate. |

Artifact retention is finite. Check availability and hashes when recovering; do not invent a local path from an artifact name. The newer `10844767167` kit is 585,766,240 bytes and exceeds the connector's 536,870,912-byte limit. Its workflow passed, but that does not make it downloadable through that action. Do not repeat this failed route or create another helper branch to work around it.

## Quantitative decision contract

Keep the existing [predeclared rule](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402) unless an explicit new experiment plan records a justified change **before** measurement:

1. Both immutable variants independently pass exact content, framing, cancellation and durable accounting qualification. Every A/B pair has matching usage totals; the accounting deadline remains ten seconds.
2. Five alternating pairs in each of four cells: concurrency 8/64; long unpaced 1,024 x 128-byte chunks, 256 requests/run; short paced 32 x 128-byte chunks at 2 ms/chunk, 512 requests/run.
3. At least 5% paired-median throughput or CPU/request improvement in a long-stream cell, with the same direction in at least four of five pairs. Reject a greater-than-5% paired-median short-stream throughput/CPU regression.
4. Reject paired-median regressions that exceed **both 10% and 5 ms** in first-content latency, completion latency or p95 request-max inter-content gap. Reject a paired-median RSS increase greater than 10%.
5. Preserve every adverse cell, qualification failure and aborted study. Never resume mixed binaries in an existing output directory or describe a completed subset as a completed matrix. Operational thresholds are not confidence intervals.

The owner prioritizes unchanged behavior, lower memory and lower latency. A throughput improvement alone is not sufficient. Profile one selected workload before proposing a new production change; keep first-content delivery, cancellation, framing and accounting as hard constraints.

## Next actions and stop/restart protocol

The next small deliverable is to make `compare.sh` reuse an explicitly configured, verified offline tokenizer cache, with real-worktree behavior tests. This changes setup, not production streaming behavior, and earns no new performance claim. Commit it directly to this PR and check that head's CI before starting another optimization.

After that, recover/verify the exact retained source; select one profiled CPU/allocation bottleneck; add a behavior regression test; and evaluate one candidate in detached **local** worktrees. Keep a rejected candidate as an unapplied patch plus complete evidence. Only accepted code belongs on this PR. No new remote branches or temporary third workflow.

Before stopping a session, update this file's current state and next action, commit completed work, and record exact source/binary identities and evidence location in the PR. An interrupted benchmark stays incomplete. Do not replace known results with estimates or leave the only recovery instructions in chat.
