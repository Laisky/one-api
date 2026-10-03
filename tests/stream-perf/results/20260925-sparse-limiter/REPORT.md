# PR427: sparse-limiter allocation — integrated

## Delivery and exact evidence boundary

The audited candidate is now published as `de6985d9e4b4a966fe7c875b54273ee0339d3ec2`, directly on the existing PR branch above `3b22ffba0e0144a6bd7dd943531df9c20f7448b1`. Only the two new-key timestamp allocations and their behavior tests changed. Existing listener, profiling, tokenizer, accounting and stream-flush code were preserved. Check CI for the actual current head; local historical results do not certify a later combined build.

This report reuses the completed experiment rather than repeating it after recovery. Original baseline: guarded production `1783b4990bbf79030452fdbe262e561d6d2b3207`. Original baseline binary SHA-256: `13a1ec75315310d1ada3f9b343d63ccb3770ffcc604d4790e92a0e0344a51f3b`; candidate: `57a589f490a38a87dea56b0f2d84a582cdecb3c9676846c18882c7952735318e`; driver: `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`. Both used Go 1.27.1 and identical recovered vendor/embed inputs with `-p=2 -mod=vendor -trimpath -buildvcs=false`. Recovered local commits are not GitHub commit identities. The newer public listener/profiler changes were not in those measured binaries.

Raw evidence archive: `one-api-pr427-sparse-limiter-evidence.zip`, SHA-256 `64dd0e7fd92b688b578541177234f45b0257ca9fd6b27c77e83f99aed6556c9a`. Recovery verified all 143 manifest-covered files and reran its eight audit controls. The archive contains full per-request observations, profiles, per-second counters, qualification, exact source inputs, original PLAN/REPORT and auditor. These are separately delivered files, not implicitly present in the repository. `runs.csv` is the original full-precision 40-run ledger, SHA-256 `6fd2af37a39c80f76366686a63991858fa8cc0927b6db7b6ef8060ad698bf97b`.

## Actual profile-guided mechanism

Three distinct diagnostic runs used 30 seconds of warm-up plus a 60-second observation, on a four-core CPU allowance, 4 GiB, Intel Xeon Platinum 8272CL, gateway/mock/client GOMAXPROCS 3/2/2. Each completed 4,096 valid requests. Baseline CPU, baseline memory and candidate memory runs achieved gateway-only mean machine utilization 53.71%, 53.40% and 52.39%; respectively 91.7%, 95.0% and 90.0% of intervals met 50%. Request-rate half-window drift stayed within 15%. Calibration was separate and did not qualify. Helpers' CPU was not counted as gateway CPU.

The immediate retained-memory hotspot was `InMemoryRateLimiter.Request`: 152.59 MB / 70.54% of 216.32 MB sampled live heap (pprof's printed units). First use reserved the full configured ceiling. The change uses `make([]int64, 0, min(maxRequestNum, 16))` in Request and Record; original append operations grow according to actual history. Admission decisions, timestamp order, expiration boundaries, dynamic limits, Record/Peek behavior, locking and legacy invalid-input behavior remain unchanged. No disabled limiter, text cache, GC tuning, deferred usage or flush batching was introduced.

**Applicability:** this harness explicitly raises both global limiter ceilings to 10,000,000. The large RSS reduction concerns high configured limits with sparse histories, not ordinary low-limit deployments or histories already at capacity. Geometric growth can leave different spare capacity in full histories. This is not a memory-leak claim.

## Completed unprofiled acceptance

Five alternating pairs per cell: short/long streams at concurrency 8/64, 40 trials, 15,360 offered/completed requests, zero errors/drops, exact paired durable usage. Both variants independently qualified exact content, CRLF/fragmented delivery, corruption/truncation/malformed failure controls, authentication and eight-client cancellation. Settlement remained a ten-second gate. Fresh gateway/SQLite and 16 excluded warm-up requests per trial; gateway/mock/client GOMAXPROCS 2/2/2. Long: 1,024 x 128-byte chunks, unpaced, 256 requests/run. Short: 32 x 128-byte chunks at 2 ms/chunk, 512 requests/run.

Values below are medians of five paired percentage changes, not ratios of separate medians. RSS is the gateway's 20-ms sampled peak.

| Profile | Concurrency | RPS | CPU/request | P95 first content | P95 completion | Peak RSS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| paced | 8 | +10.65% | -29.08% | -31.00% | -20.14% | -47.43% |
| paced | 64 | +26.02% | -28.98% | -22.49% | -25.76% | -40.96% |
| saturated | 8 | -3.49% | +1.44% | -15.53% | -33.63% | -47.61% |
| saturated | 64 | +4.40% | -7.42% | -17.38% | -9.26% | -42.49% |

All 20 pairs reduced RSS. Median peak RSS MiB changed 288.39 to 150.98 (short/c8), 307.03 to 181.83 (short/c64), 318.72 to 167.21 (long/c8), and 354.25 to 203.15 (long/c64). All four paired-median first-content/completion/content-gap metrics improved.

The predeclared memory gate requires >=10% median RSS reduction in both long cells, each favorable in >=4/5 pairs. It retains all prior latency/continuity/RSS safeguards, rejects >5% short CPU/RPS regression, and additionally rejects >5% long CPU/RPS regression. It passes. The original material CPU/RPS gate also passes: long/c64 CPU/request improved 7.42%, favorable in 4/5 pairs. **Adverse observations:** long/c8 RPS -3.49%, CPU/request +1.44%, only 2/5 favorable pairs; long/c64 RPS favorable only 3/5 and one completion-latency pair regressed. No claim of universal per-run improvement or statistical significance is made.

## Supplementary retention and reprofile

A separate fixed-20-RPS pair completed 2,200 requests/variant, exact usage and no failures/drops, with 30-second warm-up and 60-second window. Window median RSS was 389.38 to 151.54 MiB (-61.08%), but gateway window CPU increased 4.59%. Both were below 50% machine CPU, so this is matched-rate retention evidence, not a qualified high-load profile or five-repeat latency conclusion.

Candidate reprofile passed the high-load gate and showed 69.16 MB sampled live heap versus 216.32 MB in the baseline. The former large limiter allocation no longer appears among retained-heap samples; this is not proof of zero limiter allocation. BPE merging remained the largest cumulative allocation path at 30.47%; no BPE change belongs to this limiter experiment.

## Behavior validation and audit

The published tests compare 20,000 deterministic operations with the independent legacy state machine, growth, expiry equality, dynamic/zero/negative limits and exact concurrent admission. Targeted tests, three race repetitions and vet passed in the original run; unchanged baseline fails the new capacity assertions. All 47 locally recovered Python tests passed, including local diagnostic tests; this is not a claim that the newer public-head suite was already run. Code publication triggers its own public CI.

After extracting the separately delivered archive, run `python3 -B verify.py .` and `python3 -B test_audit.py`. That audit checks all raw per-request percentiles, ordered complete matrices, binary/driver identity, paired usage and diagnostic/retention counters. The historical local profile_load runner remains archival only; the public profile_run/profile_support tools are not replaced. Do not pool earlier copy/token/count/line-quantum campaigns, relabel old binaries, or repeat this completed experiment solely to recover context.

No production capacity, low-limit RSS saving, multi-tenant or external-provider result, confidence interval, hours-long soak or global optimum is certified.
