# Runtime diagnostic admission and trace plan

Registered in PR427 comment 5841604388 before any new workload.

1. Preserve production at 36e3e99/de6985d and the completed rejected core-storage/BPE studies. Re-audit old archives; do not rerun them or recreate missing data.
2. Fix CPU-only false qualification: validate monotonic counters, duration-weighted CPU, positive completion rates in both elapsed halves and <=15% upstream-rate drift. Keep coverage/high-load rules and expose the CPU-only sub-gate separately; version the rule. Recheck archived samples without changing historical decisions.
3. Extend the existing loopback profiler with opt-in bounded 1–10 second execution-trace subwindows, capture identity/partial-file protection, and deterministic tests. No new production debug endpoint, duplicate runner or changed GC/runtime parameters.
4. Execute CPU 60s and trace 5s as separate diagnostics on the same retained binary: 30s warm-up, 60s observation, trace offset 15s, c32, 8,192 requests × 1,024 × 128-byte chunks, gateway/mock/client GOMAXPROCS 3/2/2, 600s deadline. Require >=50% gateway-only effective-machine CPU for >=80% observed duration, complete coverage, positive progress and <=15% half-rate drift. Helpers cannot qualify the gateway. Independently qualify exact delivery/auth/cancellation/use. Preserve every failed window.
5. Decode only after workloads finish. GC, runnable-delay and CPU observations identify hypotheses, not a new speedup or authority to undo rejected decisions. Commit tested tooling and exact completed evidence on the one existing branch. No remote experiment branch, forced update or merge to main.
