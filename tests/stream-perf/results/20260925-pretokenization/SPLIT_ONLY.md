# Canonical pre-tokenization: split-only candidate rejected

This is a completed experiment, not a performance improvement adopted by this PR. The candidate completed all 40 trials and 15,360 verified requests with zero failures/drops and exact paired durable usage, but failed the existing latency gates. The subsequent scheduling refinement is a separate experiment and must not replace these observations.

## Registered comparison and decision

[Plan before measurement](https://github.com/Laisky/one-api/pull/427#issuecomment-5834086117). Five alternating baseline/candidate pairs in each of four cells. Long streams: 1,024 content chunks x 128 UTF-8 bytes, no upstream delay, 256 requests/run. Short streams: 32 x 128 bytes, 2 ms/chunk, 512 requests/run. Both binaries independently passed normal/CRLF/fragmented streams, expected corrupt/truncated/malformed rejection, invalid authentication and 8/8 upstream cancellation cleanup. The ten-second usage settlement gate was unchanged.

| Profile | Concurrency | Median RPS baseline / candidate | Paired RPS change | CPU ms/request baseline / candidate | Paired CPU change | P95 completion ms baseline / candidate |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | 101.21 / 101.35 | -0.06% | 6.39 / 5.78 | -8.64% | 83.18 / 83.00 |
| paced | 64 | 242.39 / 267.86 | +14.11% | 6.23 / 5.57 | -15.68% | 570.87 / 428.30 |
| saturated | 8 | 35.98 / 46.36 | +26.75% | 52.62 / 34.96 | -33.56% | 343.03 / 400.05 |
| saturated | 64 | 31.07 / 39.83 | +26.61% | 54.02 / 37.30 | -30.38% | 4261.13 / 3729.48 |

| Profile | Concurrency | Paired P95 first-content change | Paired P95 completion change | Paired sampled RSS change | Paired P95 request-max content-gap change |
| --- | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | -0.93% | +0.28% | +0.66% | -1.78% |
| paced | 64 | -19.72% | -24.97% | +3.45% | -29.01% |
| saturated | 8 | **+53.63%** | **+16.93%** | -1.95% | -42.26% |
| saturated | 64 | +1.40% | -16.00% | -1.00% | -92.87% |

All ten long-stream pairs improved throughput and CPU/request. Nevertheless, long/c8 first-content latency increased by a paired median **72.19 ms / 53.63%**, and completion by **58.09 ms / 16.93%**. Both exceed the registered conjunction of 10% and 5 ms. The implementation is therefore **rejected**, despite its CPU improvement. All thresholds remain unchanged for subsequent experiments.

Absolute values are medians across runs; percentage changes are medians of the paired percentages, not ratios of the independent medians. Five pairs and a shared host provide descriptive operational evidence, not confidence intervals or a universal causal latency diagnosis. RSS is sampled every 20 ms, not the kernel high-water mark. The continuity metric is the p95 of each request's maximum observed inter-content gap, not a pooled token-gap percentile.

## Implementation and behavior checks

The candidate specializes only the exact cl100k_base/o200k_base ordinary pre-tokenization patterns using Go's leftmost-first regexp engine and explicit Unicode whitespace lookahead handling. The BPE merge kernel, dictionary ranks, decoder, special-token path, complete token-ID output and custom-pattern fallback remain unchanged. Invalid UTF-8 replacement matches the original dependency. It adds no content cache, approximate count, deferred accounting, flush batching or concurrency cap.

A pinned internal copy of tiktoken-go v0.1.8 was used because the original core is private. Five upstream files, including the MIT license, are byte-identical; only ordinary-encoding dispatch changes in an upstream file. This maintenance burden is not adopted with a failed candidate.

Behavior checks passed 11,016 exact-piece/full-token differential inputs per encoding, 10,068 Unicode boundary inputs per encoding, all Unicode scalar whitespace classifications, custom-pattern/metadata-mismatch cases, special tokens and shared-encoder concurrency. Focused internal/gateway race tests passed three repetitions. The 16 causal real-HTTP delivery cases passed three race repetitions. All 40 Python harness tests passed. Deliberately changing whitespace backtracking failed the oracle on the expected `  a` case.

## Provenance and recovery

Local Linux AMD EPYC 9V74, four-core cgroup quota, 4 GiB memory limit, GOMAXPROCS=2 per gateway/mock/client process, fresh SQLite, one account/token/channel, default accounting/logging/tracing. All processes shared the host without CPU affinity isolation. No build, test, profile or other local performance study ran during the timed matrix. No paid providers or production credentials were used.

Both variants were built locally with recovered Go 1.27.1, `-p=2 -mod=vendor -trimpath -buildvcs=false`, identical CGO/dependency/embed inputs. Retained production was recovered from the 21dc0af artifact plus the two verified 299deff user/channel changes; 954 production/dependency/embed-input hashes matched the prior inventory. Local recovery commits are not original GitHub revisions.

| Identity | Value |
| --- | --- |
| Local recovered baseline | `e103715ee8c240aa65a37b3b5a269f52ec134a5c` |
| Local measured candidate | `6f91cc0a6afaf644cf672c6fea25f85496ed2fd8` |
| Baseline binary SHA-256 | `c0851c6dc3329fd8a809b6eba9313e80a8ac01b5a102878f86d11bf42e1e080a` |
| Candidate binary SHA-256 | `c07ae15398438f1e89ccbff137ea803ce499906db03d4328f3a6cbc19593e66a` |
| Shared driver SHA-256 | `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e` |

At this incremental checkpoint, lossless summaries and all request samples are retained in the active session under `evidence/paired/`; their complete publication is pending the separately registered scheduling experiment. This document does not claim they have already been committed. No absolute measurement is attributed to an unmeasured final GitHub-head binary.
