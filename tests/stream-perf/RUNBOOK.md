# PR 427: single execution and recovery record

## Authority and latest published checkpoint

Use only [PR #427](https://github.com/Laisky/one-api/pull/427), branch `perf/stream-chat-e2e-20260924`. Read actual HEAD/CI/comments before each write. Preserve concurrent changes; never force-push, create another remote experiment branch, merge historical branches wholesale, delete evidence refs or merge main without authorization. The owner requires unchanged behavior, lower memory and lower latency; throughput alone is insufficient.

**The completed paired client/gateway diagnostic has been published**, not left as a local patch. Code commit `fcd6fb6b25a3263a398eff116a9eff1fef4a3614` fast-forwards the actual parent `a57e34809322ce382ebc87f5abbe353e8b88b5bd`. [Report](results/20260926-client-scheduling/REPORT.md), [original local plan](results/20260926-client-scheduling/PLAN.md), [scoped identities/summary](results/20260926-client-scheduling/summary.json) and [reproduction guide](CORRELATION.md) are the current entry points. The reporting commit follows that code commit. Check CI on the actual final head, not a stale PR body or earlier green build; path-filtered frontend skips are not rebuilt frontend tests.

This integration preserves `7849709` independent client CPU slots and `a57e348` exact window start/replay plus bounded cohort decoding. The three overlapping files were merged; existing `run.py`, `test_client_procs.py`, window/cohort implementations and tests remain. All14 staged code/test files match validated local Git blobs. No production renderer, tokenizer, limiter, 4KiB fairness, GC, accounting or ordinary load-driver source changed. Normal builds do not import the diagnostic probe. No new runtime speedup or A/B acceptance is claimed.

Publication reverified the original191-file archive, all8,192 requests/11,836 correlated pairs and eight archive audit controls without repeating traffic. The integrated source passes93 targeted Python tests:42 profile, five client-slot, six window-replay and40 correlation/cohort. Explicit Go probe/client endpoint checks pass three race repetitions plus vet. Historical full gateway/overlay wire checks belong to the old capture validation, not a newly repeated full-repository build. Public CI remains authoritative for the combined repository.

## Completed paired diagnostic: preserve its boundaries

Capture parent was327808f; later local reconciliation with7849709 and public integration witha57e348 occurred after capture. Do not relabel old source or binary measurements. The original local commit5c7b360 is not a GitHub identity. The original report/plan's no-write statements describe that historical session only; this publication supersedes their delivery status, not their immutable raw evidence.

One new run completed8,192 requests, zero failures/drops, exact8,192 durable requests and40,280,064 quota units. It independently qualified normal/CRLF/fragmented delivery, expected malformed/corrupt/truncated failures, auth and8/8 cancellation cleanup. Configuration c32,1024x128-byte chunks, unpaced, gateway/mock/client GOMAXPROCS3/2/2;30s warm-up,60s observation, two concurrent5s traces. Linux6.18.44, AMD EPYC9V74, four-core allowance, five-CPU affinity,4GiB. Window coverage59.998798115s, mean gateway-only machine CPU63.197099%,100% high-load time, half-rate drift+2.501309%. No new CPU/heap profile or unprofiled optimization comparison was collected.

The five-second overlap contains11,836 adjacent content pairs across12 sampled requests. Six gateway pairs lack client boundary markers; two client intervals fail1ms marker-skew alignment. Among56 client gaps>10ms,55 are usable:52 predominantly client network Waiting, one client Runnable,51 gateway Runnable. Categories overlap. Client network waiting may mean waiting for application output; it does not prove a network bottleneck. A clear client-Runnable example prevents attributing every delay to the gateway.

All time-namespace APIs were unavailable, so absolute cross-process epoch is unverified. Only within-process intervals and epoch-offset-invariant differences are used. No one-way network latency; client parsing is not packet ingress and Flush return is not egress. Sampling/trace observer effects are not eliminated. These findings do not authorize deleting Gosched or retuning GC. A4KiB fairness budget does not bound one large BPE piece.

The first full offline decode hit128MiB and its partial/error are preserved. Two bounded offline passes then used the same traces, retaining full captured histories for marker-owning goroutines, including pre-marker states and GC scopes; no live traffic rerun or cap increase. The existing independent cohort decoder's metadata format is preserved, not silently substituted for the historical selected-pass format.

Archive supplied separately: `one-api-pr427-client-scheduling-evidence.zip`,27,911,473 bytes, SHA-256 `b439692ec38fdd67c1c931a6c8cbfee744d1bc25d7191007a9d7b1a01e417970`. After extraction run `python3 -B verify_all.py` and `python3 -B test_delivery_audit.py`. The original archived publication flag describes the old local session. Repository summaries are not raw request/trace archives. No local workload remains running after delivery.

## Intervening tools are retained

`--client-procs` alters only explicit diagnostic client slots; gateway/mock slots, default2/2/2 A/B and qualification forwarding remain independent. `--client-trace` requires trace mode and a diagnostic overlay; its private listener permits only a bounded loopback trace. Ordinary client builds remain inert.

`window_started_elapsed` records the actual new observation origin. `window_replay.py` must reproduce every stored field and failure reason at its exact recorded origin; legacy nominal/first-interval witnesses are not invented exact timestamps or permission to promote failed data. `correlation_cohort.py` preserves all selected goroutine history and a labeled synthetic trace-end boundary under the original size/deadline limits. Its tests and the paired selected-decoder tests coexist. The integrated audit retains current strict window replay, not the old first-sample assumption.

## Completed optimization/diagnostic history

Do not rerun these studies merely to recover context, pool different hosts or reapply rejected patches.

| Evidence | Decision and boundary |
| --- | --- |
| [Original142 trials](results/20260924/REPORT.md) | Retain Builder/buffered SSE; formatting shortcut rejected. |
| [76-trial follow-up](results/20260924-followup/REPORT.md) | Retain equivalent EncodeOrdinary; different host/session. |
| [Coalescing40](https://github.com/Laisky/one-api/pull/427#issuecomment-5825675787) | Reject3d671dd; retain delivery tests and SQLite usage repair. |
| [Copy40](results/20260925-copy/REPORT.md) | Rejected TTFT/repeatability gates. |
| [Token-count80](results/20260925-token-count/REPORT.md) | Two separate40-run rejections; count-only/stack copies not adopted. |
| [Split-only](results/20260925-pretokenization/SPLIT_ONLY.md) | Rejected, distinct from accepted full-token bundle. |
| [Quantum64](results/20260925-burst-reset/QUANTUM64.md) | Rejected; quantum32 comment5836195544 has no verified completion at integration. |
| [Byte-budget120](results/20260925-byte-budget/REPORT.md) | Three independent40-run studies; only final timeout-compatible confirmation describes the accepted bundle. |
| [Sparse limiter40](results/20260925-sparse-limiter/REPORT.md) | Accepted memory improvement for high limits/sparse histories, not a default/full-history guarantee. Keep adverse CPU/RPS observations. |
| [BPE scratch/output](results/20260925-bpe-scratch/RECOVERY.md) | Rejected per5840674644; raw/candidate identity unavailable after interruption. Never invent or repeat missing evidence. |
| [Core-storage40](results/20260926-core-storage/REPORT.md) | Rejected despite20/20 RSS improvement: long/c8 RPS-5.10%; long/c64 RPS-17.41%,CPU+9.70%,TTFT+93.27%,completion+42.13%. |
| [Runtime diagnostics](results/20260926-runtime-diagnostics/REPORT.md) | Separate qualified CPU/trace,8,192 requests each, plus V2 tool repair; no new runtime speedup. |
| [Gateway event correlation](results/20260926-event-correlation/REPORT.md) | Completed8,192-request diagnostic,4,849 pairs/six sampled requests. No absolute clock origin or causal/runtime gain. |
| [Paired client/gateway](results/20260926-client-scheduling/REPORT.md) | Completed8,192-request diagnostic,11,836 pairs/12requests; now published after concurrent-source integration. |

Earlier runtime trace108.97 aggregate goroutine-seconds/84.70% Gosched attribution is not wall time, CPU expense or individual client latency. Complete STW total22.27ms/max3.031ms and assist max21.12ms were descriptive. BPE29.70% cumulative CPU remains a hotspot, not proof that a rejected optimization is safe. The core-storage191-file and runtime90-file archives were independently audited; their recorded decisions remain separate. Usage-statistics equality is not a financial-ledger audit.

Accepted production remains sparse limiter `de6985d`, canonical tokenizer/4KiB budget `b6efcb8`, overflow guard `1783b49`, and listener/profiling controls `3b22ffb`. Preserve auth/quota, exact full token IDs, Unicode/invalid-UTF8/special/custom/finite-timeout behavior, original oracle/license, logs/traces, durable accounting, immediate per-event flush, cancellation and failed-study evidence. No approximate counting, synthetic text cache, deferred accounting, batching or semaphore is authorized.

## Historical helper refs

Frozen by policy, not deleted or technically archived. Historical verification was in the core-storage resumption; no new scan or fresh unchanged claim is implied here. Use only the live delivery branch; ahead-of-PR is not approval.

| Branch | Historical tip |
| --- | --- |
| `perf/stream-chat-e2e-20260924` | Read actual current HEAD; sole delivery branch. |
| `perf/stream-427-acceptance-20260925` | `dc0bff5f4d14b8335ee8c4311c80f8c68296fbb7` |
| `perf/stream-427-harness-20260925` | `2644bd4e2b177cb9486f8e702b4680f2a89f769a` |
| `perf/stream-427-immutable-20260925` | `1d58202a0d74e85a39e9760ad2d20710847e159b` |
| `perf/stream-427-objects-20260925` | `dbef1e4b802bc44b4d6c6b78cf84e0f6dafcee3e` |
| `perf/stream-coalescing-experiment-20260924` | `dde3342a15716f1ec2b2e2231c5f76e491aad0cf` |
| `perf/stream-local-kit-20260925` | `276df2f6301a64f7b7a40a19da3b3840ba14ddd8` |
| `work/stream-perf-followup-20260924` | `201852a4f00b47aabaac5e0d88afe5505909f037` |
| `work/stream-perf-runtime-20260924` | `f64ce699fff8d81299e4d1c6e00beca6812930a0` |

Carrier workflows are not production changes. Coalescing/immutable refs preserve rejected evidence, objects/runtime are recovery only. Artifact10844767167 exceeds536,870,912 bytes; do not repeat that route or create another helper branch.

## Source and evidence recovery

| Input | Run/artifact or archive | SHA-256 |
| --- | --- | --- |
| Go1.27.1/vendor/cache | `36045112587/10828256842` | `3d461291142c615be6184921d77e7361f5d30b1c39069d0270aba5c4a8d451c4` |
| Immutable299deff/harness | `36084329116/10842988144` | `348204d2866e7c136abb237327c984b2a10a1abdbc0f7807095b9c746198be76` |
| Source21dc0af | `36068339476/10836434460` | `14738f5b1207dcad408c138fe9eae3e30eae1dab06c06d5128f57df66e96e36d` |
| Copy | `one-api-pr427-copy-evidence.zip` | `97a97fe2d3857bd51def974b2a66a2a519a9a504057a32e0e3055e076bda138f` |
| Byte-budget | `one-api-pr427-byte-budget-evidence.zip` | `e09795b55fc81ad0006c7b8fa9f24011bb43da6a7200252f54084e300d5c5e83` |
| Limiter | `one-api-pr427-sparse-limiter-evidence.zip` | `64dd0e7fd92b688b578541177234f45b0257ca9fd6b27c77e83f99aed6556c9a` |
| Core storage | `one-api-pr427-core-storage-evidence.zip` | `8936eb113776cb93698193fb77668ede924e337bfca06e31b9fcbfefbe4f783d` |
| Runtime diagnostics | `one-api-pr427-runtime-diagnostics.zip` | `38567026945bde885a2d11e07a4d812e3a96554bdc8bf42a3ea2573b4ced8bc8` |
| Gateway event correlation | `one-api-pr427-event-correlation-evidence.zip` | `3806534c583546ac4c33b63f9f6fc37723e9b4fa43ced3ba44b1dc72f1290524` |
| Paired client/gateway | `one-api-pr427-client-scheduling-evidence.zip` | `b439692ec38fdd67c1c931a6c8cbfee744d1bc25d7191007a9d7b1a01e417970` |

Verify actual paths/hashes and finite retention. Historical954-input recovery with retained user/channel blobs f0a66e703258c63f3436e80e88a91bf4f07e646e /59862525884dae9af6705fef3af787ea7a5092bc plus exact accepted overlays was extended to961 inputs for listener/tooling recovery. Public source still must be checked before reuse. Go1.27.1, identical vendor/embed, `-p=2 -mod=vendor -trimpath -buildvcs=false`. Local recovery commits are not public history.

Normal gateway b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46 and driver7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e were reproduced before/after original instrumentation. Paired observed gateway6921f3b1fd91041e830b977e948970ddab5c4d7b87a58d3d13b34959cc4d1f85 and drivera328e3968a794711978fef63c4fa48d564fc7a9482ffe3106a021fba5e100c68 are diagnostic-only. Earlier single-process observed08f41773/5687629c and historical pre-guard6550a116/guarded13a1ec75 builds remain separate. The640-request guard smoke is correctness, not performance; [integration](results/20260925-byte-budget/INTEGRATION.md) preserves CodeQL4107214497.

Do not repeatedly retry failed direct clone routes. Never export private instruction files, credentials, databases, toolchain or fonts. The older f4bb2eb295c9c438c236a136bf78b121d76615760fdcdeffe804c4dfc17b4bad archive was not newly verified. Raw archives/audit-only tests are distinct from repository CI and committed summaries.

## Next bounded action and acceptance

First finish actual final-head CI/reviews. This integration and both historical traces are completed; do not rerun their traffic for context recovery. The next runtime mechanism must be preregistered and compared under the full unchanged no-regression contract. Most sampled stalls are not predominantly client Runnable, but the clear exception prevents attributing every stall to the gateway. Correlation alone does not justify changing Gosched, GC targets or reviving rejected BPE/storage/copy candidates.

Keep [original A/B gates](https://github.com/Laisky/one-api/pull/427#issuecomment-5825472402): independent qualification, ten-second settlement, five alternating pairs per short/long c8/c64 cell; long1,024x128bytes/256 requests and short32x128bytes/2ms/512 requests. Require>=5% long-cell CPU/RPS benefit in>=4/5 pairs; reject short CPU/RPS regression>5%, latency growth exceeding both10% and5ms, or RSS growth>10%. Register justified memory rules before results; never select favorable windows/thresholds to rescue failure.

Follow [PROFILING.md](PROFILING.md): freeze workload, qualify>=50% gateway-only effective CPU and stable progress, collect CPU/heap/trace separately and evaluate unprofiled immutable A/B. Helpers cannot qualify gateway load; memory occupancy is not a fill target. Use the existing profiler and source-pinned overlay, not a duplicate runner. Keep read-only token cache, authoritative comparison paths, normal unprofiled defaults and actual loopback listener checks. Archive raw evidence before a final result, commit finished work incrementally to this branch and update this record; do not leave the only recovery instructions in chat.
