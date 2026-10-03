# PR430 — measured dashboard and logs Web API improvement

## Decision and delivered scope

**Accept the additive `logs(user_id, created_at, id)` index for this qualified SQLite fixture.** The independent steady-connection confirmation completes **160 endpoint trials / 51,200 timed HTTP responses**, zero failures/drops, complete response oracles, both independent authorization/filter/pagination qualifications and post-write freshness. All four personal dashboard/log targets improve P95 in all40 paired endpoint comparisons. The unchanged gate passes; adverse administrator/control observations remain below.

Production code changes only the three GORM index tags on `Log`. Existing indexes, query SQL, cache keys/TTL, singleflight, selected columns, JSON DTOs, limits, logging, quota and permissions remain unchanged. Cursor APIs are exercised explicitly, not newly enabled by default. No streaming PR427 change is reopened; it is already merged into main.

This is a **new, separately identified recovery study** on public runtime head8090181, not the lost raw continuation of the earlier32-request trial. That earlier study was publicly recorded as rejected because administrator logs/c8 P95 regressed70.68%/11.335ms. The original raw files were not found during recovery. The later8090181 commit message records an earlier accepted confirmation and completed costs/profiles, despite the stale RECOVERY.md checkpoint. Those earlier raw results also were not recovered; they are recorded as historical claims, not counted or independently reverified here. Those numbers are historical statements, not reverified samples; they are never pooled with or replaced by the current accepted data. See [registered protocol](PLAN.md), original comments5846867443/5846944353 and recovery comments5848140289/5848803766.

## Exact inputs and HTTP contract

Baseline production `c13183989f47d1ee1d797151b81a1e36d5fb48a4`; tested candidate production is the index metadata already published in `809018189b96c1224693c949c07ae1eb5c1e9a1c`. The public source/vendor export7494f6b differs from baseline only in the then-temporary build workflow and plan; that workflow is absent from this PR. Go 1.27.1, identical vendor/frontend embed inputs, `-mod=vendor -p=2 -trimpath -buildvcs=false`. A 974-file production/dependency/embed inventory differs only in `model/log.go`. The original package download SHA256 is35a5184ced396534d113949a236492287e7cba0893f1a9a639a9a447a5959c14. Direct clone was unavailable; recovered local commits are not represented as original public Git history.

Baseline binary SHA256: `5f32abdcdb53d10d970c7367d41ea9421d23f7f40a76c0035d50b2243783e3fd`.
Candidate binary SHA256: `b0f1b08c18b18e1dbe864d9ab973ba1324b6da7d3cbc53849b0f2debda1d455a`.
The source/export/hash records bind actual binaries, not an unmeasured future head. Later evidence/test-only edits do not change their runtime inputs; they still require their own CI.

Linux AMD EPYC9V74, four-core cgroup quota,4GiB memory; gateway GOMAXPROCS2, one independent Python load process. A private file-backed SQLite schema is created by the real gateway then seeded with200,000 rows over90days,32 users,80% heavy-user history and1,024 UTF-8 content bytes per row. The timed seven-day range is2026-09-20 through2026-09-26. Every run starts from a fresh database copy and normally migrated gateway. Runtime defaults, including dashboard TTL0 and request coalescing, are preserved. Identical concurrent dashboard reads can still coalesce; this is not a distinct-user concurrency test. The enabled API limiter ceiling is10,000,000 and cursor APIs are an explicit identical fixture opt-in, not a product-default change.

Five alternating pairs per endpoint/concurrency. Concurrency1/8; four targets issue128 measured requests/run, four controls512. Each worker reuses its own connection after two independently validated warmups, before a shared start barrier. **1,440 worker warmup calls are excluded** from the51,200 timed observations; first-positive qualification and post-write checks are also separate. Full-body HTTP latency stops before JSON decoding and oracle execution; throughput includes that client work. Resource counters isolate gateway from client, with20ms sampled RSS, not a kernel high-water mark. Short CPU deltas can be quantized to clock ticks.

Independent oracles check every complete log DTO, permitted owner, filter/date predicates, exact legacy count, cursor continuation and count provenance, all six dashboard aggregate views, quotas/status and empty shape. Negative HTTP checks cover no authentication, denied administrator endpoints, cross-user dashboard/cursor access and invalid date ranges. A committed fixture insertion after timing must update the legacy page/count and dashboard. This proves read freshness, not production-writer or financial-ledger correctness. Existing legacy timestamp ties retain their SQL's unspecified tie order; no new stable-ID order is promised. Cursor's explicit `(created_at,id)` ordering has separate native migration tests.

The before/after startup query plans verify the index actually survives migration: baseline searches the user-only index and builds a temporary ordering tree, while candidate range-scans the user/time/id index without a sort. No manual index forcing, ANALYZE/statistics refresh or OS-cache purge is performed. Better statistics, different distributions, native engines or cache conditions may change baseline plans and benefit magnitude.

## Complete steady-connection results

Milliseconds and MiB are medians of five independent runs. Changes are medians of the five paired percentage changes; they need not equal ratios of the separately displayed medians. No confidence intervals or pooled-request significance are claimed.

| Endpoint | Concurrency | Baseline P95 ms | Candidate P95 ms | Paired P95 change | Improved pairs | CPU/request change | RSS change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| self-dashboard | 1 | 97.561 | 25.354 | -72.76% | 5/5 | -74.12% | +1.45% |
| self-dashboard | 8 | 101.267 | 34.589 | -64.76% | 5/5 | -66.45% | +0.30% |
| self-logs | 1 | 189.008 | 8.573 | -95.35% | 5/5 | -95.66% | -0.72% |
| self-logs | 8 | 570.527 | 28.854 | -94.78% | 5/5 | -96.35% | -4.69% |
| self-deep | 1 | 233.409 | 8.999 | -96.15% | 5/5 | -96.57% | -0.21% |
| self-deep | 8 | 1039.211 | 32.034 | -96.90% | 5/5 | -97.69% | -9.63% |
| self-cursor | 1 | 103.427 | 2.063 | -98.02% | 5/5 | -98.44% | -0.18% |
| self-cursor | 8 | 451.933 | 23.196 | -94.87% | 5/5 | -98.96% | -5.94% |
| site-dashboard | 1 | 39.068 | 39.279 | +0.54% | 2/5 | +0.37% | -1.75% |
| site-dashboard | 8 | 84.718 | 87.233 | +4.21% | 2/5 | -0.22% | -7.28% |
| admin-logs | 1 | 2.338 | 2.301 | -5.64% | 3/5 | +5.15% | -1.90% |
| admin-logs | 8 | 18.555 | 20.069 | +14.21% | 2/5 | -2.83% | -5.48% |
| admin-stat | 1 | 9.873 | 8.440 | -8.39% | 4/5 | -5.32% | -2.18% |
| admin-stat | 8 | 33.657 | 36.239 | -10.78% | 3/5 | +3.81% | -3.75% |
| user-self | 1 | 0.882 | 0.862 | -2.43% | 4/5 | -0.00% | -2.16% |
| user-self | 8 | 13.495 | 12.612 | -4.63% | 3/5 | +2.44% | -5.81% |

All 40 target endpoint pairs improved P95. All eight target/concurrency cells pass the material benefit rule. The administrator-log c8 control has a **+14.21% / +2.636 ms** paired-median P95 regression and improves in only 2/5 pairs: it does not cross both the 10% and 5 ms rejection thresholds, and is not described as a speedup or proved harmless. Site-wide dashboard c8 has +4.21% P95; global dashboard gains are not claimed. All adverse pairs remain in the summary and raw requests.

P95 ranges, P50, successful RPS, gateway CPU and RSS medians/ranges remain available in the verifier output. Throughput includes Python JSON/oracle validation and is not server-only capacity. The caller-side observation does not measure browser layout or paint.

### First observations (descriptive only)

Each variant has one independent first-positive qualification observation per endpoint, prior to persistent-worker warmups. These are not five-repeat cold-cache measurements. Startup migration, UUID maintenance and fixture creation can already warm filesystem pages.

| Endpoint | Baseline first ms | Candidate first ms |
| --- | ---: | ---: |
| self-dashboard | 84.783 | 21.410 |
| self-logs | 146.550 | 6.141 |
| self-deep | 198.002 | 6.011 |
| self-cursor | 142.851 | 5.848 |
| site-dashboard | 35.912 | 31.831 |
| admin-logs | 2.086 | 1.960 |
| admin-stat | 6.320 | 6.469 |
| user-self | 0.915 | 1.196 |

## Index size, migration and write tradeoff

External inspection used Python SQLite **3.46.1**, not a claim about native Go engine bytecode. The new index occupies **3,760,128 bytes (3.586 MiB), 918 pages** for 200,000 fixture rows. File page counts grew from 84,756 to 85,674 at 4096-byte pages. No previous index was removed.

Five independent direct CREATE INDEX + COMMIT operations took 124.666 ms median (range 118.280–131.305 ms). Normal gateway startup with actual migration took 153.388 ms baseline and 254.129 ms candidate medians. Startup is not isolated DDL time and none of it is hidden inside steady-request latency.

| Pair | Baseline 5,000-row INSERT+COMMIT ms | Candidate ms | Paired change |
| ---: | ---: | ---: | ---: |
| 0 | 196.271 | 191.032 | -2.67% |
| 1 | 200.062 | 210.036 | +4.99% |
| 2 | 203.127 | 194.562 | -4.22% |
| 3 | 182.697 | 202.414 | +10.79% |
| 4 | 191.340 | 208.680 | +9.06% |

The paired-median bulk-write cost is **+4.99%**; separate time medians are 196.271 and 202.414 ms. These are Python SQLite full-row batch inserts and commit, journal mode `wal` / synchronous `2`, after payload generation and starting-row count. All before/after row counts match. They are not production gateway writer throughput, transaction p95, concurrent ingest/read contention or a promise of constant cost on larger data.

### Rollout boundaries

The new nonunique index is created by the existing normal startup migration. It is not an online/concurrent-index migration implementation. Schedule and measure DDL against the deployment engine and actual table size, retain backups/free space, and verify the index on the log database before declaring rollout complete. PostgreSQL normal CREATE INDEX blocks table writes while building; MySQL DDL locking/algorithm depends on server/version/table settings. No zero-lock rollout guarantee is made. A code rollback alone may leave an already-created index present; any deliberate manual removal must be coordinated with migrators that would recreate it.

GORM supports explicit index field priorities and creates declared indexes through AutoMigrate: https://gorm.io/docs/indexes.html . SQLite explains composite-index range search and ordered retrieval: https://www.sqlite.org/queryplanner.html . PostgreSQL distinguishes ordinary and concurrent builds: https://www.postgresql.org/docs/current/sql-createindex.html . The native-engine CI evidence here checks creation/recreation/idempotence and query results; it does not benchmark PostgreSQL/MySQL performance or qualify a production DDL maintenance window.

## Separate CPU diagnostics

After all unprofiled requests and validation/build work, each variant ran a separate30s warm-up/60s observation inside a95s workload. Eight persistent workers used distinct disjoint seven-day dashboard windows, preserving normal singleflight and authorization rather than disabling them. Gateway GOMAXPROCS3; helper CPU and shared cgroup throttling were recorded separately. The inherited stable gate uses verified completed HTTP responses in its progress slot, not a provider counter; the zero durable-request adapter is explicitly not billing evidence.

| Variant | Successful diagnostic responses | Mean gateway / effective machine CPU | Time above50% | Covered window | Half-window response-rate drift |
| --- | ---: | ---: | ---: | ---: | ---: |
| baseline | 4,053 | 96.85% | 100.00% | 59.000s | -0.88% |
| candidate | 14,814 | 93.70% | 100.00% | 59.000s | -2.79% |

Both windows pass their original high-load/progress gates. All18,867 diagnostic responses validate and are separate from the51,200 timed A/B responses; warmup requests and a small final drain are not window throughput. CPU profiles last60s and are never enabled in acceptance trials. The native SQLite stepping path dominates cumulative CPU samples (baseline about97.93%, candidate about93.24%). Go pprof primarily resolves the cgo SQLite boundary, not the internal SQL optimizer's per-operator cost. At fixed busy duration, candidate processes more requests; profile-total CPU alone is not per-request latency or a new independent speedup claim. SQL plans and unprofiled HTTP measurements identify the saved work more directly.

## Validation and persistence

Before timing, eight restored fixture/worker/audit contracts passed. After timing, all14 existing Web harness tests and eight new published-evidence negative controls passed (22 total). The index tests passed three local race repetitions and model vet; local native-server arms are unavailable and not counted as passes. Independently downloaded original CI artifacts verify **all three SQLite/PostgreSQL/MySQL migration/query arms executed and passed**, plus schema-definition tests, on public head8090181 / merge e334d31. Artifact10912681037 SHA256ceaf0189530ba00587e1a3084f60eec444c0b772f50a005a1e96a3e3e483e7c2 and10912420901 SHA2568b1985c22942f70fd2321175f3c901e96badfb25e3241c511e3cf76751096afa bind those results. [That full CI](https://github.com/Laisky/one-api/actions/runs/36260390183) passed; it is not certification of a later evidence commit.

The committed compressed summary preserves every original run-level field and full decimal precision. `manifest.json` binds exact summary bytes, fixture, binaries, endpoint/principal contracts and file hashes. `verify.py` recomputes the complete ordered matrix and acceptance. With the separately delivered raw directory it additionally calls the original full-request auditor and verifies all51,200 worker/index identities, nearest-rank quantiles, body-size observations and1,440 warmups. It does not claim that timing JSON can recreate unsaved private HTTP bodies; independent payload checks ran on every actual response.

The separately supplied `one-api-pr430-web-api-evidence.zip` contains all original request JSON, both CPU profiles and per-second telemetry, index cost records, CI artifacts, exact harness snapshots/source-input inventories and independent extraction auditor. No gateway executable, compiler, fonts, production credentials, private gateway logs or database files are included. Earlier missing raw studies remain explicitly unavailable, not invented. No new remote branch, permanent workflow, production deployment or automatic merge is part of this task.

### Reproduction

From the repository, audit the committed summary alone:

```sh
python3 tests/web-perf/results/20260926-user-time-index/verify.py
```

After extracting the raw evidence archive, also check all request observations:

```sh
python3 tests/web-perf/results/20260926-user-time-index/verify.py \
  --raw-directory /path/to/extracted/raw/steady-confirmation
python3 /path/to/extracted/verify_all.py
```

For a new experiment, build both pinned source revisions with the same recorded toolchain/dependencies/embed and invoke the existing `tests/web-perf/run.py` defaults; it refuses overwriting results. Run costs.py and diagnose.py separately, never alongside timed traffic. The published source and dataset generator are the reproducibility path after build-artifact retention expires; a new experiment records its own hashes and all outcomes, not this run's identities.

This result concerns personal seven-day dashboard/log HTTP reads under the stated distribution. It does not prove browser rendering, cold-disk latency, native PostgreSQL/MySQL speed, a low-cardinality/light-user gain, arbitrary filters/sorts, write concurrency, multi-instance behavior, WAN/TLS latency, production capacity or a global optimum. Site-wide dashboard is a control and has no claimed material improvement. Native DDL rollout, disk headroom and write cost must be considered before deploying an index to a much larger live table.
