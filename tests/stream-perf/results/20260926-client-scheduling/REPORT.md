# Paired client/gateway trace — published diagnostic, not a runtime speedup

## Actual delivery and source boundary

The previously completed local tooling is now published as `fcd6fb6b25a3263a398eff116a9eff1fef4a3614` on the existing PR427 branch, a non-forced fast-forward from `a57e34809322ce382ebc87f5abbe353e8b88b5bd`. No production renderer/tokenizer/limiter/GC/fairness/accounting or ordinary driver source changed, and no rejected optimization was adopted. Current-head CI must be checked separately.

Capture used public parent `327808f1de2ded4bba951d23671e14346b891ef8`. After capture, the local patch was reconciled with `7849709` independent client CPU slots. Publication additionally preserves `a57e348` exact `window_started_elapsed`, strict historical window replay, the existing bounded cohort decoder and all its tests. Both `--client-procs` and `--client-trace` remain independent. The three overlapping files were reconciled, not replaced with the old patch. Local commit `5c7b3609f5b9d2312a786b66947a5e7bd6fbdf17` is not a GitHub commit.

The original separately supplied report said no remote publication was possible in its session; that historical statement is superseded by this integration, not erased from the archive. Completed traffic was not rerun, old binary measurements are not relabeled as new-head measurements, and the archived raw bytes remain unchanged. [PLAN.md](PLAN.md) preserves the locally registered pre-capture design, not a retroactively invented remote preregistration.

## Completed diagnostic

One run: c32, 8,192 requests, 1,024 x128-byte synthetic content chunks, unpaced, gateway/mock/client GOMAXPROCS3/2/2, 30s warm-up plus60s observation and concurrent5s traces. Default GC, exact token IDs, limits/quota, durable usage, logging/tracing and immediate per-event flush remain. Established harness limiter ceilings are10,000,000. Both actual listeners were IPv4-loopback; no build/test/other profile ran alongside the workload.

Host: Linux6.18.44, AMD EPYC9V74, four-core visible cgroup allowance, five-CPU affinity,4GiB, Go1.27.1. Load started2026-09-26T03:00:04Z. All8,192 requests succeeded, zero errors/drops, exact8,192 durable requests and40,280,064 quota units. Normal/CRLF/fragmented delivery, intended malformed/corrupt/truncated controls, invalid authentication and8/8 cancellation cleanup independently qualified. Usage statistics are not a financial-ledger audit.

Recorded V2 coverage59.998798115s, mean gateway-only effective-machine CPU63.197099%,100% high-load time, upstream half-rate drift+2.501309%. A nominal30s/60s replay also qualifies. Helpers cannot qualify the gateway. Gateway capture elapsed46.005021654–51.113829872s; client46.011608741–51.035838103s. Both are inside the window. Requested tracing is5s, not the HTTP transfer duration or a full-minute trace.

| Trace | Bytes | SHA-256 |
| --- | ---: | --- |
| Gateway | 18,998,780 | `b8ada0377706a1e01c93247553f1fe5b7975b5008498a1511d5884f6962c1c25` |
| Client | 9,775,036 | `fa2397e19d328fbec724f834841c6e673bed7b0298710ab01935ee5f2c62aad6` |

## Matched observations and limits

Deterministic1/32 request cohort, at most2,048 data events/request. Markers contain numeric identities and monotonic timestamps only; no credentials, headers or payload. Role/content/finish/usage/DONE share frame numbering. Client markers use the exact timestamp stored in request JSON.

11,842 complete gateway adjacent-content pairs yield **11,836 pairs across12 sampled requests** matched to both client markers; six pairs miss client trace boundaries. Among56 client gaps>10ms,55 have usable complete state coverage. Across all pairs, two client intervals exceed1ms trace/monotonic skew and retain observations without state attribution.

| Among55 usable gaps>10ms | Count |
| --- | ---: |
| Client Runnable for at least80% of its interval | 1 |
| Client Waiting/network for at least80% | 52 |
| Gateway Runnable for at least80% of between-render interval | 51 |

These categories overlap and must not be added. Among18 cases where client gap exceeds gateway flush gap by>5ms:15 primarily overlap client network wait, one client Runnable,14 gateway Runnable. Network-poller Waiting may mean waiting for application output; it does not establish a network-hardware bottleneck.

| Request/frame | Client gap ms | Gateway flush-return gap ms | Client Runnable ms | Client network Waiting ms | Gateway Runnable ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 3232/94 | 29.598908 | 27.357169 | 0.408960 | 29.164736 | 27.293632 |
| 3424/75 | 17.852261 | 0.268105 | 14.565440 | 3.258240 | 0 |

Both patterns exist. This instrumented cohort is not the latency distribution of all8,192 requests, a confidence interval, or causal proof. The host/capture differs from prior studies and results must not be pooled. Time namespace APIs are unavailable for every process: **absolute_cross_process_verified=false**. Only within-process intervals and epoch-offset-invariant differences are compared. Client parsing is not packet ingress; Flush return is not packet egress. No one-way network latency is reported. Marker skew/boundary checks do not eliminate observer effects; concurrent trace overhead was not separately quantified. GC and scheduler overlaps are not additive causal delay.

## Decoder failure preserved

The first whole-trace decode hit the unchanged128MiB retained-output limit. Its1,117,879-record partial output and stderr remain in the archive. The successful offline two-pass selection used the same traces and limit, retaining marker-owning goroutines' full captured history, including pre-marker states, global pauses and relevant assists. No traffic was repeated or favorable time slice selected.

Gateway retained37,203,665 bytes/six reused worker goroutines/35,694 markers; client33,186,665 bytes/six workers/11,892 markers. The workers served12 sampled requests. Independent audit verifies pass input/tool hashes, marker identity/order, scopes and byte/count limits. The decoder changes were after capture, with separate recorded source identities. The concurrent `correlation_cohort.py` path remains untouched; it is not silently replaced by this historical selected-pass format.

## Validation, reproduction and archive

This publication reran **93 targeted Python tests** on merged source:42 profiling, five existing client-slot, six window-replay and40 correlation/cohort. Explicit Go probe/client endpoint tests pass three race repetitions plus vet. The original archive's full gateway/overlay wire validation is historical, not a newly repeated gateway build; default overlay wire tests intentionally skip. No new E2E or A/B traffic was generated for integration.

Original archive: `one-api-pr427-client-scheduling-evidence.zip`,27,911,473 bytes, SHA-256 `b439692ec38fdd67c1c931a6c8cbfee744d1bc25d7191007a9d7b1a01e417970`. Recovery reverified191 manifest-covered files, all8,192 request quantiles/counters,11,836 pairs and four selection passes; eight archive-only self-tests passed. After extraction run `python3 -B verify_all.py` and `python3 -B test_delivery_audit.py`. These checks run no network workload. Raw traces/counters/requests and exact capture sources are separate archive artifacts, not implicitly committed repository files. The archived publication flag describes that earlier session only; `summary.json` here separates historical measurements from actual publication metadata.

Normal gateway SHA-256 `b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46`; normal driver `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`. Historical observed gateway `6921f3b1fd91041e830b977e948970ddab5c4d7b87a58d3d13b34959cc4d1f85`; observed driver `a328e3968a794711978fef63c4fa48d564fc7a9482ffe3106a021fba5e100c68`; decoder `f218734ed91f7e412b8c2c378b620609c2a5eb8745fea501071b272f4e4c9e03`. Recovery checked961 retained source/build inputs; local recovered histories are not original GitHub commits.

See [CORRELATION.md](../../CORRELATION.md) for an explicit new capture, not an instruction to rerun this completed one. Follow [RUNBOOK.md](../../RUNBOOK.md) for the next separately registered production control and unchanged exact behavior/unprofiled A/B gates. This diagnostic supports neither removing fairness nor reviving rejected memory candidates by itself. No new speedup or global optimum is claimed.
