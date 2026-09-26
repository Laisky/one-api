# Consecutive buffered-burst scheduling: quantum 64 rejected

This is a complete newly reconstructed experiment, not a resumed or re-labelled historical study. The candidate is **not adopted**. The next quantum-32 study is separately registered and must not overwrite these observations.

## Recovery and experiment identity

The actual starting PR head was `66f5271a0ac46c5bf73879713d7b27374ccfbea0`; its CI run `36150209541` passed. Production remained `299deffa58aa1e87aa699a038484cbdaae3ab450`. The old PR description and runbook lagged the later discussion. Historical token traversal/stack, scanner/trace/reader, split-only and earlier scheduling candidates are completed non-adoptions, not changes awaiting a merge. In particular, [comment 5834691701](https://github.com/Laisky/one-api/pull/427#issuecomment-5834691701) registered resetting the fairness budget after an upstream read; this continuation finishes that design with newly reconstructed source.

The later session's splitter sources and raw request archives were not recovered from mounted files or saved Library ZIPs. Therefore no byte identity with that lost candidate is claimed. Existing source/runtime archives were reused, and all 954 retained production/dependency/embed-input hashes matched the prior manifest. The [recovery declaration](https://github.com/Laisky/one-api/pull/427#issuecomment-5835864982) preceded this measurement. Local reconstruction commits are not original GitHub commits.

| Identity | Value |
| --- | --- |
| Local recovered control | `861d00fdfc06fa90d2de89f9347d27c288cf06fa` |
| Local measured candidate | `8f3314866317abbc15492913f3b6904624dd0c83` |
| Control binary SHA-256 | `5974873e2ab135d927b04f8401e194db5a197c664b96fa482268e8d8daea6fd8` |
| Candidate binary SHA-256 | `10d771fccd803693066c541af993e6f0b4040edf680475bb8f9d70f52853e3b5` |
| Shared driver SHA-256 | `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e` |
| Candidate patch SHA-256 | `0297c2f7b75aadd37a1229578f0796d4b332c7c66e67c459cccf0cabfbd45443` |

Both binaries were freshly built with recovered Go 1.27.1, `-p=2 -mod=vendor -trimpath -buildvcs=false`, identical embedded assets and dependency inputs. Host: local Linux, Intel Xeon Platinum 8370C, four-core cgroup quota, 4 GiB limit, GOMAXPROCS=2 independently for gateway, mock and client. Processes share the host without CPU-affinity isolation. No builds, tests, profiling or other local performance study overlapped the timed matrix. Different historical sessions must not be compared as absolute capacity gains.

## Workload and complete result

Five alternating A/B pairs in each of four cells. Long streams use 1,024 content chunks x 128 UTF-8 bytes, no upstream pacing, 256 requests/trial. Short streams use 32 chunks x 128 bytes, 2 ms/chunk, 512 requests/trial. Each trial starts a real gateway and fresh SQLite with one account/token/channel and 16 warm-up requests. Authentication, quota, exact counting, synchronous logging/tracing, per-event flushing and durable usage remain enabled. Only the still-enabled global rate ceilings are raised. Gateway CPU includes settlement; RSS is sampled every 20 ms, not the kernel high-water mark. Client peak RSS remains unmeasured.

**40/40 trials, 15,360 offered and verified requests, zero failed or dropped requests, exact paired durable usage.** Both variants independently pass valid/CRLF/fragmented streams, expected malformed/corrupt/truncated controls, invalid authentication and 8/8 upstream cancellation cleanup. The ten-second usage deadline is unchanged. Every request sample independently reproduces its trial's nearest-rank latency distributions. Qualification and warm-up are excluded from the measured totals. Traffic uses loopback URLs; the historical gateway listener itself is not asserted to bind only loopback.

| Profile | Concurrency | Paired median RPS change | CPU/request change | P95 first-content change | P95 completion change | Sampled RSS change | P95 request-max content-gap change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Short/paced | 8 | +5.40% | -18.61% | -34.07% | -8.58% | -1.41% | -31.58% |
| Short/paced | 64 | +2.49% | -6.71% | -3.80% | -2.96% | -22.01% | -6.47% |
| Long/unpaced | 8 | +43.96% | -28.51% | -72.61% | -36.91% | -3.31% | -58.73% |
| Long/unpaced | 64 | +36.95% | -29.80% | -43.52% | -31.23% | -2.94% | **+21.92%** |

**Reject:** long/c64 continuity increased by a paired median **21.92% / 42.75 ms**, exceeding both registered limits. Throughput, first-content and completion gains do not excuse this failure. All long-stream CPU/RPS material-benefit and repetition gates passed; no gate was relaxed. The metric is the p95 of per-request maximum observed gaps, not a pooled token-gap percentile. Five pairs are descriptive operational evidence, not confidence intervals or proof of a universal causal effect. Paired percentages are medians of per-pair changes, not ratios of independent absolute medians; RSS and latency observations have substantial variability.

## Implementation and validation

The candidate specializes only exact pinned cl100k_base/o200k_base ordinary pre-tokenization. It retains the original BPE merge kernel, dictionaries, complete token-ID output, decoding, special tokens, and generic/custom-pattern/finite-timeout fallback. A pinned internal tiktoken-go v0.1.8 copy retains five upstream files and the license unchanged; only ordinary dispatch is added to the upstream core. There is no user-text cache or approximate count.

HeartbeatLineReader yields between caller invocations after 64 consecutive complete buffered returns. A needed upstream read resets the budget. No readahead, flush batching, deferred accounting or concurrency cap is introduced. A separate instrumented diagnostic observed 1,818 fairness hits across all 64 long requests, and 57 hits across 128 short requests; these probe timings are excluded from acceptance.

Reconstruction caught a real equivalence boundary: Go's Unicode case folding includes long s U+017F where the independent regexp2 contraction oracle does not. Exact piece comparisons failed although complete token comparisons passed. Explicit ASCII contraction alternatives repaired that mismatch. The original failure and the mutation reproducing it are preserved.

Complete normal differential tests passed: 7,293 piece/full-token/decoding inputs per encoding, 5,475 Unicode category boundary inputs per encoding, all scalar whitespace classifications and exhaustive contraction-fold checks, custom patterns and shared encoders. One full exhaustive race run passed (228.8 seconds); ten shared-encoder race repetitions passed; all reader/SSE tests passed three race repetitions; native OpenAI delivery/token-focused tests passed three race repetitions. An earlier three-repeat exhaustive race invocation exceeded its 300-second timeout and is retained as a failed invocation, not relabelled passing. Missing-I/O-reset and wrong-case-fold negative controls fail the intended assertions.

## Current continuation

[Comment 5836195544](https://github.com/Laisky/one-api/pull/427#issuecomment-5836195544) registers **quantum 32** before measuring it. Only the consecutive buffered-line budget is reduced; the splitter and reset policy are unchanged. Control hash, five-pair/four-cell workloads and every accounting, correctness, CPU/RPS, latency, continuity and RSS gate stay fixed. The quantum-64 data is not substituted or pooled with the next study.

At this incremental checkpoint, raw summaries, per-request JSON, diagnostic output, patches and validation logs are retained locally for the final evidence package. This document does not claim that those raw files or a new production optimization have been committed. No new remote branch, temporary workflow, force push or merge was made.
