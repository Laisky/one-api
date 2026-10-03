# BPE scratch/output experiment: recovery boundary

## Completed decision — do not repeat the acceptance matrix

The authoritative retained result is [PR comment 5840674644](https://github.com/Laisky/one-api/pull/427#issuecomment-5840674644), posted after the [registered plan](https://github.com/Laisky/one-api/pull/427#issuecomment-5840427852). It reports a complete independently qualified 40-trial / 15,360-request study with no failures/drops and equal paired durable usage.

The candidate was **rejected**. Long/c8 improved RPS 16.07%, CPU/request 12.71% and P95 completion 13.20%, but the P95 of per-request maximum content gaps increased 104.36% / 20.44 ms. Long/c64 improved RPS 14.05%, CPU/request 11.35% and P95 completion 17.39%, but the same gap metric increased 11.66% / 6.67 ms. Both gap cells exceed the registered combined 10% and 5 ms limit. These are the prior execution's reported results, not a new calculation or proof of a gateway wire-level pause.

Production remains the integrated sparse-limiter and previously accepted tokenizer/4 KiB scheduler. No BPE scratch/output patch is applied. The accepted sparse-limiter commits are de6985d9e4b4a966fe7c875b54273ee0339d3ec2 and a95e9f14593f4373d914e02e340ff5ee56cc7456; both recorded public CI runs passed. Never infer later-head CI from these results.

## What survived and what did not

On resumption, the PR head was a95e9f14593f4373d914e02e340ff5ee56cc7456. The latest comment records the rejection and a proposed separate pair of CPU diagnostics. No subsequent completed diagnostic record or candidate source/binary identity was present in that discussion. The working container was reset; no experiment process, local worktree or BPE raw-output directory survived. Library retrieval located older accepted-study archives, but did not locate this BPE experiment's raw archive.

Accordingly, this document preserves the already-reported rejection without inventing per-request records, hashes, plots or an unapplied source patch. The prior full matrix is **not rerun merely to recover context**. A reconstructed candidate could not honestly be called the exact old candidate without its missing identity. The proposed baseline/candidate diagnostic comparison therefore remains unverified rather than being silently declared completed.

## Continue safely

1. Read the current PR head, comments and RUNBOOK before modifying anything; preserve independently advancing changes. Use only the existing PR427 branch, without force push or a helper branch.
2. Start any new diagnostic from the retained public production baseline and record fresh source/binary identities. Do not use an invented reconstruction of the rejected candidate as historical evidence.
3. Diagnose CPU and memory separately under the public loopback profiling runner; keep gateway-only CPU attribution, window coverage, request progress, throttling and failure evidence. Preserve the exact-token, usage and immediate-flush contracts.
4. Register a genuinely different optimization before its measurements. Apply complete differential behavior tests and the full unprofiled acceptance matrix. A rejected optimization stays out of production regardless of throughput gains.
5. Save the new source patch, raw data and manifest before publishing a result, and commit the current recovery state while the tools are still available.

This recovery note closes a stale 'A/B still running' description; it does not establish any additional speedup or global performance optimum.
