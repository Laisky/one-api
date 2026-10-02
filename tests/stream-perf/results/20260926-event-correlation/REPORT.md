# PR427: per-event SSE/trace correlation — diagnostic completed

## Scope and delivery

Tooling commit `1b50b74faf31ae0b2c24da6f1e14e448d38a84b5` adds explicit diagnostic overlays and validators above `19976b3`. No production renderer, tokenizer, limiter, scheduler, GC setting or ordinary driver file changes. Normal gateway and driver rebuilds remain byte-identical to retained hashes `b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46` and `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`. There is no new unprofiled A/B, accepted runtime optimization or speedup claim.

[Registered plan](https://github.com/Laisky/one-api/pull/427#issuecomment-5841978479), [pre-capture clock-scope amendment](https://github.com/Laisky/one-api/pull/427#issuecomment-5842057736), [initial result record](https://github.com/Laisky/one-api/pull/427#issuecomment-5842147139). Previous CPU/trace, core-storage and BPE studies were inherited, not repeated or reinterpreted as this run. Their rejected implementations remain rejected.

## Actual run and qualification

The explicit overlay records numeric render-entry/flush-entry/flush-return markers and matching client data-event observations. Only request indices divisible by 32 are observed, bounded to 8,192 requests / 1,024 content chunks / 2,048 data frames per request. Headers, payloads and credentials are not recorded by the probe. Original write order, headers, error/short-write behavior and per-event flushing remain unchanged; observation overhead exists.

One run: **8,192 requests**, c32, each 1,024 x 128-byte chunks, unpaced; gateway/mock/client GOMAXPROCS 3/2/2. Linux6.18.44, Intel Xeon Platinum8370C, four-core cgroup allowance, five-CPU affinity, 4 GiB. Loopback API and profiler descriptors were checked. Default GC, exact token counting, accounting, logging and tracing remain; the established harness raises limiter ceilings to10,000,000. Only synthetic providers/credentials were used.

After 30s warm-up, the 60s window has **59.999576s coverage, 60.417094% mean gateway-only machine CPU, 100% high-load duration and +2.114229% half-window upstream-rate drift**. Helper CPU does not qualify the gateway. Independent raw-counter reconstruction exactly reproduces every recorded window field; a separate nominal30s/60s recheck also qualifies with58.999554s coverage. The complete run has zero failed/dropped requests and exact8,192 request statistics /40,280,064 quota units. Normal/CRLF/fragmented streams and intended corruption/truncation/malformed failures, invalid authentication and8/8 cancellation cleanup passed before load. Positive usage statistics are not an independent financial-ledger audit.

The five-second trace was requested at elapsed45.005429s and returned at50.170644s, within the observation. Its10,415,873 bytes hash to `4f0e75c4afe0a46d2d32dcaacb70f6ef36a72869d959ea019197a2681a32f2b9`. The decoder retained652,974 goroutine transitions,536 complete begin/end GC range pairs and14,625 numeric markers. This run captures neither a new CPU profile nor a heap profile; it is not pooled with previous runs.

## Clock limitation and preserved failed attempt

The first attempt completed qualification but failed at its first telemetry snapshot because this kernel does not expose `/proc/*/ns/time`. It produced no trace or qualified timed window and remains marked incomplete in the archive. Before retry, the plan was narrowed: all-missing namespace APIs allow **within-process interval comparisons only**. Mixed/malformed/changed identities still fail. No wall-clock substitution or shared-epoch assumption was made.

Thus this result has `absolute_cross_process_verified=false`: **no one-way render-to-client latency or absolute post-flush lag is reported**. Instead it compares consecutive-frame intervals on each process's CLOCK_MONOTONIC; their difference is invariant to a constant epoch offset. Synthetic tests add large positive/negative client offsets and require unchanged interval results. The namespace amendment narrows claims, not the CPU/progress/behavior gates.

## Correlated observations — not causal attribution

The deterministic cohort contains256 requests overall. The trace overlaps only **six requests**, yielding4,875 complete marker triplets,4,855 content frames and **4,849 adjacent content-frame pairs**. No partial frame triplet was observed; non-content role/finish/usage/DONE frames and pairs without both trace-observed neighbors are excluded from gap statistics. This is a small, nonrandom, instrumented overlap cohort, not the latency distribution of all8,192 requests or production users.

| Observed metric in this cohort | Median ms | P95 ms | Maximum ms |
| --- | ---: | ---: | ---: |
| Render entry to flush-entry marker | 0.002644 | 0.007633 | 1.213733 |
| Instrumented Flush span | 0.007184 | 0.033205 | 32.001517 |
| Consecutive gateway flush-return gap | 0.050238 | 5.140200 | 38.996215 |
| Consecutive client data-event observation gap | 0.036338 | 5.153498 | 32.025382 |
| Previous flush return to next render entry | 0.037935 | 5.034820 | 30.813399 |

There are **83 client gaps above10ms**. One pair's trace/monotonic interval skew exceeds1ms (about-6.509ms); its client measurement remains, but scheduler/GC attribution is excluded. Of the remaining82, **78 have at least80% of the between-render interval overlapping the same gateway goroutine's Runnable state**. The reconstructed intervals total1,094.78464ms Runnable, almost entirely entered through the existing Gosched reason. These sums cross frames/requests and are not wall-clock time, Gosched CPU cost or proof that removing fairness helps.

Two deliberately contrasting observations demonstrate why aggregate delay was insufficient:

| Request/frame | Client gap ms | Gateway flush-return gap ms | Between-render Runnable overlap ms |
| --- | ---: | ---: | ---: |
| 1600/431 | **32.025382** | **0.062900** | **0.000000** |
| 1504/79 | **30.317073** | **30.836723** | **30.769728** |

The first pair's32ms client gap does not coincide with a32ms gateway flush gap or intervening recorded gateway runnable wait; observation/transport scheduling remains unresolved because no client execution trace or packet ingress timestamps were collected. The second aligns closely with the same gateway goroutine waiting runnable between renders. Neither is a controlled intervention proving causation.

For the82 alignment-usable stalls, complete global-GC-pause overlaps sum1.937023ms (maximum0.978624ms per pair); same-goroutine mark-assist overlap is zero in these specific between-render intervals. GC ranges overlap scheduling states and must not be added as additional delay. Zero in this bounded cohort does not prove no GC effect elsewhere, during Flush or outside the trace. The instrumentation itself may schedule between its clock read and trace log; the explicit interval-skew exclusion does not remove all observer effects. Signed downstream-lag changes are retained, not clipped into network latency.

## Validation and reproducibility

Twenty-one new Python tests pass alongside32 existing profile tests. Five Go probe tests pass with three race repetitions; the explicitly enabled overlay wire test also passes three times under a live trace, including empty/multiline/invalid-UTF-8/terminal frames and downstream errors/short writes. It is intentionally skipped in ordinary test builds; local explicit validation is not mislabeled as default CI coverage. A deliberately broken frame counter fails expected7 versus actual0. Source pins/unique anchors, clock-domain failures, marker bounds, duplicate IDs, phase order, trace boundary exclusions, skew, offset invariance, request percentiles, usage, corrupt windows and decoder failure/overflow are tested.

An initial decoder test used a1s startup deadline and intermittently expired; its log is retained. The successful-decoder test now allows5s startup while the intentionally silent decoder still must time out at1s. No actual workload was repeated for decoder or analyzer development. All final15 tooling file blobs match locally validated bytes.

[CORRELATION.md](../../CORRELATION.md) contains complete explicit-build/capture commands. The separately delivered `one-api-pr427-event-correlation-evidence.zip` includes the raw trace, all request observations, snapshots, filtered events, exact build overlay, source identities, failed attempt and validation logs. It excludes executables/toolchains/dictionaries/databases/credentials/private instructions. The repository commits the scoped summary, not all raw samples. The archive auditor rechecks every covered hash, all8,192 request percentiles, recorded/nominal windows, exact frame matches and complete trace overlaps. A Go decoder binary is not needed to audit saved filtered records; its hash and original trace identity are retained.

No production code from this iteration is accepted or rejected: this is new measurement capability and one finished diagnosis. The next bounded design should distinguish gateway scheduling from client/transport observation under an independently registered control; it must not simply remove Gosched, change GC targets, or revive rejected memory candidates. Keep original unprofiled A/B gates for any actual optimization and preserve this completed run rather than repeat it for context recovery.
