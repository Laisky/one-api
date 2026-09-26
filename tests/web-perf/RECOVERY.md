# Web API performance — single recovery record

## Authority and current checkpoint

Continue only PR430 / `perf/web-api-e2e-20260926`, above main `c13183989f47d1ee1d797151b81a1e36d5fb48a4` (merged PR427). Do not reopen PR427, create another remote experiment branch, force-push, add a workflow or merge to main. Read actual HEAD before writing and preserve concurrent work.

The current published 160-trial/51,200-response confirmation is complete and its measured acceptance remains unchanged. Its reviewed head `100aab0303d46828e862b613f1a5dd16470cd096` passed full CI36265304352. This review repair adds no runtime change and requires its own exact-head CI. All previous index/query/cache/authentication/DTO/cursor/timeout behavior is preserved. Public head8090181 introduced the index; later commits added harness/evidence.

**Review follow-up:** the standalone reporter now requires all three literal-true freshness flags for both variants and the candidate's ordered index columns. Five new regression tests fail in78 subcases on the old reporter and pass after repair; all13 locally executed existing/new fixture, worker and audit tests pass. The actual production binaries were rebuilt with identical input trees and match BOTH hashes in the published confirmation exactly. [Build-input audit and chronology](results/20260926-user-time-index/BUILD_INPUT_AUDIT.md) and [compact build identities](results/20260926-user-time-index/build-inputs.json) resolve review4112471386 without rerunning completed HTTP traffic. Reviews4112471382/4112471384 identify the repaired standalone acceptance gaps.

The reported `.gitkeep` mismatch belongs to the earlier17:45 study, documented at17:51 in comment5848484441. It stays pre-alignment evidence. The currently published study was separately registered at18:36 in5848803766, measured18:36:29–18:51:43, and uses the exact aligned binary pair reproduced now. Never conflate these studies, call the old mismatch harmless, or pool their observations. The full original Web API raw archive was not found during this review recovery; no new raw-request/profile audit is claimed.

## Completed scope and results

[Report](results/20260926-user-time-index/REPORT.md), [registered protocol](results/20260926-user-time-index/PLAN.md), [manifest](results/20260926-user-time-index/manifest.json), [auditor](results/20260926-user-time-index/verify.py) remain the primary evidence. The compressed summary and all measurements are unchanged by this repair.

160 endpoint trials /51,200 measured responses, zero failures/drops, complete response and independent authorization/filter/page/count/cursor/freshness checks. All40 personal pairs improve P95: personal dashboard64.76–72.76%, logs94.78–95.35%, deep logs96.15–96.90%, cursor94.87–98.02% paired-median reductions. Administrator logs/c8 regresses14.21%/2.636ms and improves only2/5 pairs; it remains below the combined10%/5ms rejection gate and is not described as a speedup. No material site-wide-dashboard gain. Some personal-dashboard RSS cells grow slightly; none exceeds20%.

Five cost pairs: index3,760,128 bytes/3.586MiB; direct build124.666ms median; startup153.388→254.129ms medians;5,000-row bulk INSERT+COMMIT paired-median cost+4.99%. These are SQLite observations, not native PostgreSQL/MySQL timing or production writer throughput. Normal AutoMigrate can block writes; deployment-specific DDL, disk, interruption recovery and rollback coordination remain required.

Separate CPU diagnostics completed4,053 baseline and14,814 candidate validated responses. Each used30s warm-up/60s observation in95s with eight distinct seven-day windows, gateway-only effective-machine CPU96.85%/93.70%,100% high-load time,59s coverage and drift-0.88%/-2.79%. SQLite stepping dominates. Do not pool those18,867 diagnostic responses with the unprofiled matrix.

The original delivery's22 Python tests, local model race/vet and CI36260390183 remain historical validation. Downloaded CI artifacts verified SQLite/PostgreSQL/MySQL migration/query cases without skips on8090181/merge e334d31; native timings were not measured. CI36265304352 certifies100aab, not this later repair. Skipped frontend paths are not new browser builds.

## Source and evidence recovery

Source/vendor/fixture export: run36246623576/artifact10908065782, source7494f6b, SHA25635a5184ced396534d113949a236492287e7cba0893f1a9a639a9a447a5959c14. It differs from baseline c131839 only by the temporary workflow and plan. The workflow was removed before8090181; do not recreate it or weaken normal CI. Artifact retention ends2026-10-03; check availability before reuse.

The two rebuilt SHA256 identities are baseline5f32abdcdb53d10d970c7367d41ea9421d23f7f40a76c0035d50b2243783e3fd and candidateb0f1b08c18b18e1dbe864d9ab973ba1324b6da7d3cbc53849b0f2debda1d455a. Same Go1.27.1/vendor/flags/embed,974 input files per variant, onlymodel/log.go differs. Actual Go EmbedFiles contains the same emptyweb/build/.gitkeep on both sides. Full inventories and build/package outputs are supplied in the new review-validation archive; these are build reproductions, not new performance samples or original GitHub commit identities.

Earlier32-request160-trial study was rejected for administrator logs/c8+70.68%/11.335ms; its raw data is unavailable. Earlier confirmation/cost/profile claims in8090181 and comments remain distinct from the current study. Do not reconstruct lost samples from prose or replace rejected outcomes by reruns.

The separately delivered `one-api-pr430-web-api-evidence.zip` was named by the original delivery as holding all raw requests, warmups, profiles, costs and its extraction auditor. It was not found in this review session's runtime or targeted Library lookup. The committed summary remains available; lack of a mounted raw archive does not invalidate the newly reproduced binaries or authorize claiming a repeated raw audit. Default `verify.py` verifies the committed summary; `--raw-directory` additionally requires the exact original raw directory.

## Next action and finish criteria

Confirm this repair's actual public CI/review status and record it in the PR body. Do not repeat the completed matrix, costs or diagnostics for context recovery. No local load process is left running. New production/query/UI optimization would need a separately registered mechanism and immutable A/B study; this iteration only closes review and source-provenance questions.

Preserve nonunique/existing index sets, inclusive dates, scopes, full DTOs, exact counts, cursor defaults, cache keys/TTL, singleflight, logging/quota and post-write freshness. Legacy timestamp ties remain unspecified by their old SQL. SQLite shared-host HTTP timings do not certify browser paint, cold OS cache, light-user/native-engine/WAN/TLS improvement, write concurrency, capacity or a global optimum. Never use stale caching, estimated counts or disabled checks to obtain a speedup. Do not merge or deploy without a separate instruction.
