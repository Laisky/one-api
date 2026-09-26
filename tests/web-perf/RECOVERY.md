# Web API performance — single recovery record

## Authority and completed delivery

Continue only PR430 / `perf/web-api-e2e-20260926`, above main `c13183989f47d1ee1d797151b81a1e36d5fb48a4` (merged PR427). Do not reopen PR427, create another remote experiment branch, force-push, add a workflow or merge to main. Read the actual head before writing and preserve concurrent work.

**The independently identified recovery confirmation is complete and accepted.** [Report](results/20260926-user-time-index/REPORT.md), [original/recovery protocol](results/20260926-user-time-index/PLAN.md), [manifest](results/20260926-user-time-index/manifest.json) and [auditor](results/20260926-user-time-index/verify.py) are the current entry points. Production is the already-published index/test change in8090181; this publication adds evidence and audit tests, not another runtime change.

160 endpoint trials /51,200 measured responses, zero failures/drops, all payload/authorization/filter/page/count/cursor/freshness checks pass. Every one of40 personal endpoint pairs improves P95: personal dashboard64.76–72.76%, logs94.78–95.35%, deep logs96.15–96.90%, cursor94.87–98.02% paired-median reductions. Administrator logs/c8 still regresses14.21%/2.636ms and improves in only2/5 pairs; it does not exceed both10% and5ms and must remain visible. No material global-dashboard gain is claimed. RSS changes remain inside the20% guard; some personal-dashboard cells grow slightly.

Five separate cost pairs: index3,760,128 bytes (3.586MiB), direct build124.666ms median, normal startup153.388→254.129ms medians,5,000-row bulk INSERT+COMMIT paired-median cost+4.99%. These are Python SQLite storage/DDL/batch-write observations, not native PostgreSQL/MySQL or production writer throughput. Migration may lock writes and needs deployment-specific planning.

Separate CPU diagnostics complete4,053 baseline and14,814 candidate validated responses. Each uses30s warm-up,60s observation inside95s, eight distinct personal seven-day windows. Gateway-only effective-machine CPU96.85%/93.70%, high-load duration100%/100%,59s covered windows, response-rate drift-0.88%/-2.79%; both qualify. Profiles point to SQLite stepping. Do not pool those18,867 responses or their instrumented timing with the unprofiled matrix.

All22 Python contracts/audit tests pass. The index tests pass three local race repetitions and model vet; native server arms are not local passes. Actual CI36260390183 on head8090181/merge e334d31 passed all required jobs, and downloaded model0/model1 artifacts verify SQLite, PostgreSQL and MySQL migration/query cases executed without skips. The final evidence/test commit requires its own CI; record its observed status in the PR body rather than inheriting8090181's green badge. Frontend paths are unchanged; skipped frontend jobs are not new browser builds.

## Precise recovery boundaries

The first recorded32-request study completed160 trials/5,120 calls and was rejected for admin-log/c8 P95+70.68%/+11.335ms. Its raw files are unavailable. Comment5846944353 preregistered a separate warmed-connection confirmation. The8090181 commit message later records an accepted confirmation plus cost/profile completion, while the old RECOVERY.md was stale. That earlier raw evidence was not found either. Preserve those historical statements without inventing raw rows or combining their results with this new run.

This recovery's run was identified before timing in comment5848803766, using the already-published stable protocol. It began2026-09-26T18:36:29Z and is complete; exact timestamps and all original fields are in the compressed summary. Do not rerun these completed workloads merely to recover context.

The source/vendor/fixture archive from run36246623576/artifact10908065782 has SHA25635a5184ced396534d113949a236492287e7cba0893f1a9a639a9a447a5959c14. Its source7494f6b differs from c131839 only by the temporary workflow and plan; that workflow was removed before8090181. It expires2026-10-03; check retention instead of creating more bootstrap branches. Go1.27.1 was recovered separately; no compiler/dictionary/font files are supplied in the result archive.

Fresh baseline SHA2565f32abdcdb53d10d970c7367d41ea9421d23f7f40a76c0035d50b2243783e3fd; index candidate b0f1b08c18b18e1dbe864d9ab973ba1324b6da7d3cbc53849b0f2debda1d455a. Identical vendor/embed/toolchain/flags;974 production/dependency/embed inputs differ only in model/log.go. Local recovery commits are not original GitHub identities, and the earlier4091/5cae binaries are not relabeled.

## Reproduce, preserve, and finish

The separately delivered `one-api-pr430-web-api-evidence.zip` retains every raw request and warmup, separate diagnostics/costs, source-input inventories, actual CI artifacts, validation logs and `verify_all.py`. Private databases, access tokens, gateway logs, compiler/binaries and fonts are excluded. A repository summary is not a substitute for the raw archive. Default `results/20260926-user-time-index/verify.py` verifies the committed complete summary and acceptance; `--raw-directory` additionally checks all51,200 request observations. The extraction auditor also verifies profile windows and supplementary evidence. Archive-only tests are not extra repository-CI tests.

Current runtime behavior remains query/cache/API unchanged except the nonunique user/time/id index. Preserve existing index set, exact authorization/DTO/filter/date semantics, counts, cursor defaults and freshness. Legacy timestamp ties were not given a new total order. SQLite measurements are not browser paint, cold OS-cache, light-user/native-engine/WAN performance or capacity. Do not disable singleflight, replace exact counts with estimates, add a stale cache, or tune workload after inspecting adverse results.

Next: inspect the actual final-head CI/review and fix genuine failures. Do not repeat the completed matrix, costs or CPU profiles. Any further query/frontend/writer optimization requires a separate preregistered mechanism and immutable A/B evidence, retaining controls and DDL/write costs. All local experiment processes were stopped after delivery; persistent changes exist only on this PR and in explicitly delivered evidence.
