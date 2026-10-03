# PR427: reliable sustained windows and bounded runtime diagnostics

## Delivered scope

Tooling commit `c3aae60c50ff8de4d6e705cc8d3b389b9b3b321a` updates the existing PR branch. It changes seven files under `tests/stream-perf` only. **No production runtime, tokenizer, limiter, scheduling budget, GC setting, accounting or SSE flushing changed. No new speedup or A/B acceptance is claimed.** The accepted sparse limiter and canonical tokenizer remain; the completed BPE scratch and vocabulary-index candidates remain rejected.

The [plan](https://github.com/Laisky/one-api/pull/427#issuecomment-5841604388) was posted before these diagnostics. The prior core-storage archive was recovered and its 191 covered files, 15,360 A/B requests, 16,384 diagnostic requests and rejection were independently re-audited, not rerun. Parent `36e3e994d2d9a396ea7b47af18a00cb7b25c381f` passed CI run 36204562355. Current-head CI must be checked separately.

## Fixed a measurable diagnostic false positive

The old `stable_window` qualified CPU-busy windows despite zero completed requests or 50% throughput drift, and did not reject upstream/durable counter rollback. Three negative tests reproduce four assertion failures against that old implementation. Version 2 now requires valid monotonic samples, positive upstream completion rates in both elapsed half-windows and absolute rate drift no greater than 15%. CPU means and the fraction above the 50% target are weighted by elapsed duration, not sample count. Dense sampling during brief busy periods cannot satisfy an otherwise idle window.

Coverage must include at least 58 of 60 seconds, no sampling interval may exceed 2.5 seconds, and both halves need at least 28 seconds. Boundary-straddling intervals are excluded from half-rate estimates rather than inventing individual completion timestamps. Upstream completions and durable counters remain explicitly labeled proxies, not client completion timestamps. Full delivery and final exact durable use are independently checked by the runner.

`cpu_only_qualified` records the new duration-weighted CPU/coverage sub-gate; it is not an assertion that a historical version-1 result was recomputed identically. The previous study's archived samples also pass a separate nominal-window version-2 reanalysis. Their original gate and rejected runtime conclusion are unchanged. A trace or capture failure preserves incomplete evidence instead of declaring success.

## Actual completed diagnostics on retained production

Independent CPU and trace runs each used 30 seconds of warm-up followed by a 60-second observation: concurrency 32, 8,192 requests, 1,024 content chunks/request, 128 bytes/chunk, unpaced, 600-second deadline, gateway/mock/client GOMAXPROCS 3/2/2. Actual API/pprof descriptors were verified as IPv4 loopback before traffic. Both variants are the **same unchanged binary**, not baseline/candidate comparisons.

Host: AMD EPYC 9V74, four-core cgroup allowance, five-CPU affinity, 4 GiB memory, Go 1.27.1. No build, unit-test, analysis or other local performance workload overlapped the captures. Accounting, tracing, logging, ordinary exact token counting and default GC stay enabled. As in the established harness, the two enabled global rate-limit ceilings are raised to 10,000,000.

| Separate run | Gateway / machine CPU mean | Time at or above 50% | Covered observation | Upstream half-rate drift | Verified requests |
| --- | ---: | ---: | ---: | ---: | ---: |
| CPU profile | 61.08% | 100.00% | 59.001s | +2.17% | 8,192 |
| Execution trace | 59.35% | 98.30% | 59.002s | -0.67% | 8,192 |

Both version-2 gates pass. All 16,384 measured diagnostic requests complete, zero failures/drops; each settles exactly 8,192 requests and 40,280,064 quota units. Warm-up and negative-control traffic are separate. Each run independently qualified normal/CRLF/fragmented content, intended corrupt/truncated/malformed failures, invalid authentication and 8/8 upstream cancellation cleanup.

The CPU capture requested 60 seconds. The trace requested only 5 seconds at offset 15 inside the observation: capture HTTP wall interval 45.002–50.200 seconds after workload start, decoded trace span 5.015 seconds. That is **not a full-minute trace**. Capture size is 14,535,977 bytes; the global bound is 128 MiB. Loopback URLs, redirect rejection, proxy bypass, exclusive partial files and atomic no-overwrite publication are tested. Time checks and socket timeouts bound capture operations; trace overhead is part of diagnostics, not unprofiled performance evidence.

## What the profiles establish—and do not

The independent CPU profile contains 145.41 sampled CPU-seconds over 60.01 wall seconds (rounded pprof output). Cumulative BPE merge accounts for 29.70%, canonical piece splitting 17.14%, `StringData` 17.17%, and background marking 6.66%. These call-tree percentages overlap; they are not additive shares or direct estimates of latency.

The short trace's scheduler profile sums 108.97 seconds of runnable delay **across goroutines**, of which 92.30 seconds/84.70% is attributed to `runtime.Gosched`; `HeartbeatLineReader.Next` contains the retained scheduling point. This is not 108.97 seconds of wall time, not CPU consumed by Gosched, and not a single-request or client-observed stall. Deliberate yielding can be necessary for fair streaming; the profile is not grounds to remove it.

Complete named GC ranges in the trace show 71 mark/sweep-termination global pauses totaling 22.27 ms, maximum 3.031 ms. The trace-start pause is separate, 0.0168 ms. There are 35 complete concurrent mark ranges; one already-active boundary range is excluded. There are 1,033 complete goroutine-scoped mark-assist ranges, maximum 21.12 ms; their 136.47 ms sum may overlap across goroutines and includes elapsed range time, not solely CPU. These numbers are restricted to this one 5-second trace.

**No causal link to the prior rejected candidates' tail regressions is proven.** The trace covers the gateway only, without synchronized per-event client timestamps or a changed-runtime control. The next justified investigation should connect event delivery/read boundaries with runnable delay and GC-assist timing before changing budgets, allocation behavior or GC settings. It must use a new preregistered design; do not rescue old candidates or call an unchanged-runtime diagnostic a performance gain.

## Validation and provenance

All 22 new tests pass. Restoring the exact current ten legacy profile tests gives 32 passing targeted tests. Tests cover no-progress/drift/rollback, irregular sampling/time weighting, malformed numeric input, incomplete windows, redirects/oversize/empty bodies, partial and concurrent file publication, trace placement/overrun and failure cleanup. Eight archive-only auditor tests additionally catch altered identities, missing request IDs, modified timing quantiles, use mismatch, profile corruption and partial/overlapping trace ranges. They are not additional repository CI tests.

The diagnostic gateway SHA-256 is `b194162e565267d03678863531ca2e66cd47e465770b43fa5c50039f98e81a46`; driver `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`. The recovered build reproduces the existing accepted sparse-limiter/listener baseline. Local reconstructed source history is not original GitHub history. Tooling used during capture matches all seven uploaded Git blobs; the new code changes only diagnosis. Exact source hashes, compiler/decoder commands, raw profiles, request JSON, per-second process/cgroup counters and validation logs are archived.

Postprocessing began only after both loads ended. Initial terminal decode attempts hit tool time limits; a bounded offline decode completed. An exploratory debug-text counter initially read an attribute as an event type; it was discarded and replaced by the correct Go 1.27.1 header parser. Final counts are 1,805,051 decoded events, including 1,159,150 state transitions. Range pairing uses name and scope, omitting incomplete boundary ranges, with synthetic tests. No workload or rejected matrix was repeated to repair decoding.

## Reproduction and evidence boundaries

From a clean checkout, build the gateway/driver normally, prepare the tokenizer cache explicitly, then use the existing runner:

```sh
python3 tests/stream-perf/profile_run.py --binary /path/to/gateway --driver /path/to/stream-perf \
  --token-cache /path/to/cache --output /new/cpu --mode cpu \
  --gateway-procs 3 --auxiliary-procs 2 --concurrency 32 --requests 8192 \
  --chunks 1024 --chunk-bytes 128 --warmup 30 --seconds 60 --deadline 600
# Run separately, not concurrently with CPU capture or build/tests:
python3 tests/stream-perf/profile_run.py --binary /path/to/gateway --driver /path/to/stream-perf \
  --token-cache /path/to/cache --output /new/trace --mode trace --trace-seconds 5 --trace-offset 15 \
  --gateway-procs 3 --auxiliary-procs 2 --concurrency 32 --requests 8192 \
  --chunks 1024 --chunk-bytes 128 --warmup 30 --seconds 60 --deadline 600
```

Trace subwindows must be 1–10 seconds inside the observation. A failed load/stability/capture remains failed; do not choose a favorable later slice or raise the drift threshold after seeing results. The original unprofiled A/B acceptance rule is unchanged.

The separate archive `one-api-pr427-runtime-diagnostics.zip` contains raw evidence; this repository directory contains summarized results only. Run `python3 -B verify_diagnostics.py` and `python3 -B test_diagnostic_audit.py` after extracting the archive. Its verifier binds hashes, recomputes every request percentile and version-2 window from counters, and checks capture identity and final accounting. For these saved runs, nominal 30/60 boundaries reproduce the recorded intervals exactly; that equality is checked, not generalized to arbitrary capture jitter. Decoding with another Go version requires revalidation of its debug format. Do not present archived-only tests or summaries as a full production capacity/financial-ledger/latency-causality audit.
