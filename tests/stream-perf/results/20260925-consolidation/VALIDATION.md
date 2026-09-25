# PR 427 consolidation validation — September 25, 2026

## Scope and decision

Keep PR 427 as the only active delivery branch. Reconcile the historical branch roles in [the runbook](../../RUNBOOK.md); do not merge temporary workflows or the rejected flush-coalescing candidate. This iteration changes comparison setup and its tests, not gateway production code. It makes **no new throughput, memory, latency or production-capacity claim**.

## Offline comparison regression

`compare.sh` previously ignored `TIKTOKEN_CACHE_DIR` and always prepared a new cache. Three new cases failed against that unchanged script: configured-cache reuse, missing-cache rejection before a build, and corrupt-cache rejection without repair. The tests use real Git worktrees and observable compiler/runner stand-ins; missing/corrupt cases execute the actual committed cache validator while forbidding downloads.

After the fix, all five comparison-script tests passed, including the existing immutable-build/overwrite behavior and a new failed-build cleanup case. Relative cache paths containing spaces resolve before entering a detached worktree; supplied caches are check-only inputs; invalid caches stop before compiler execution; no implicit repair or measurement occurs after a failed check. With no configured cache, explicit preparation into the experiment directory retains its prior behavior.

The published code blobs match the locally tested bytes:

| File | Git blob SHA-1 |
| --- | --- |
| `tests/stream-perf/compare.sh` | `2bb9c66e09fa0c5a12e202b55c24d38ecbf7906b` |
| `tests/stream-perf/test_compare.py` | `7128343d3b0b979c5dfac6691b6ca7038e488df5` |

Validation commands:

```sh
bash -n tests/stream-perf/compare.sh
python3 -m unittest discover -v -s tests/stream-perf -p 'test_*.py'
```

All **37 Python tests passed** locally using Go 1.27.1 for the real HTTP driver. These include stream corruption/truncation controls, overload visibility, resource accounting, historical evidence verifiers, continuity reporting and failed-trial evidence. The count is 33 existing tests plus four additional comparison tests, not 37 new tests.

## Recovered retained-gateway smoke

Reused immutable build artifact `10842988144` from workflow run `36084329116`; no new build branch or workflow was created. Its retained control is production revision `299deffa58aa1e87aa699a038484cbdaae3ab450`, not the rejected coalescing candidate.

- Gateway SHA-256: `60f7c847360fdae3d59ef89cf074d4013c25935cad307dd53444314e622fe9a9`.
- Driver SHA-256: `9bcd8bbc43ed55329d09c8346c1a20d86aa835ef80a927594546242da0349015`.
- Workload: one repetition; concurrency 1/8; saturated and paced profiles; 16 requests per cell; 32 chunks. All four measured cells completed: **64 verified requests, zero failed requests and zero drops**. Warm-up and qualification are excluded from those counts. Durable usage checks passed.
- Qualification: normal, CRLF and fragmented streams passed. Missing-DONE, wrong-content and malformed controls each rejected all eight intentionally bad streams. All eight cancellation producers were released within five seconds.
- This single-variant, short smoke is correctness/recovery evidence, **not an A/B comparison or a capacity measurement**. `run.py` labels the sole binary `candidate`; that label does not imply that new optimized production code was tested.

For reproduction, extract the verified artifact into a local directory and use a fresh result directory:

```sh
python3 tests/stream-perf/run.py \
  --binary /tmp/pr427-build/one-api-control \
  --driver /tmp/pr427-build/stream-perf \
  --token-cache /tmp/pr427-build/token-cache \
  --output /tmp/pr427-recovery-smoke \
  --concurrency 1,8 --requests 16 --paced-requests 16 --chunks 32 --repeats 1
```

Local session: Linux, AMD EPYC 9V74, four-core cgroup CPU quota, 4 GiB memory limit. Keep its observations separate from historical reports on different hosts.

## CI boundary

The retained production revision's full CI passed in run `36084278200`. That is not the CI status of a later consolidation commit. Check the PR's current-head required checks separately; no checks or workflows were weakened. Full repository race/coverage validation and frontend compilation are not claimed from the local Python suite or the prebuilt-binary smoke.
