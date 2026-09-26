# Registered unused-vocabulary-index experiment

Authoritative registration: https://github.com/Laisky/one-api/pull/427#issuecomment-5841256299 . Preserve this plan independently of the outcome.

Baseline: accepted sparse limiter, canonical tokenizer, guarded BPE and existing 4 KiB scheduler. Hypothesis: remove only the unread sortedTokenBytes index and its constructor allocation/sort; keep every encoder/decoder map, token ID, custom/special handling, GC setting, accounting and flush policy.

Before traffic, validate exact behavior against the independent original tokenizer, constructor error contracts and concurrent use. Require >=4 MiB lower isolated retained o200k heap and >=20% lower constructor allocated bytes. Then run the unchanged complete five-pair/four-cell matrix: c8/c64, unpaced 1,024 x 128-byte chunks / 256 requests; paced 32 x 128-byte chunks / 2 ms / 512 requests. Independent qualification and exact paired durable usage remain mandatory.

For this constructor-memory-only study, require lower median sampled RSS in both long cells with at least 4/5 favorable pairs. Reject >5% paired-median CPU/request or RPS regression in any cell, >10% RSS growth, or latency/continuity worsening by BOTH 10% and 5 ms. Do not relax these thresholds after observing the candidate. Preserve all adverse cells.

Collect separate baseline/candidate heap diagnostics with 30-second warm-up / 60-second observation, gateway/mock/client GOMAXPROCS 3/2/2, c32 and 8,192 long-stream requests, unchanged loopback-only public profiler. Record gateway-only effective-machine CPU, window coverage, drift and all failures. Profiles are not timed acceptance trials.

No new remote branch. If any gate fails, retain behavior tests and evidence only; store the production change as an unapplied patch. No rerun or adoption of earlier rejected count-only/scratch/output/copy/scheduling studies is authorized by this plan.
