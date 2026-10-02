# Independent Web API recovery confirmation

This is the predeclared steady-connection protocol recorded in PR430 comments
5846944353 and 5848140289, with this recovery's exact binaries recorded in
5848803766 before measurement. It is not continuation, reconstruction or pooling
of the earlier unavailable 32-request raw study. That study's recorded control
regression remains a rejection.

Source baseline: main `c13183989f47d1ee1d797151b81a1e36d5fb48a4`.
Candidate runtime: the sole Log schema-index addition already in PR430 head
`809018189b96c1224693c949c07ae1eb5c1e9a1c`. Read-only source/vendor export
`7494f6b15670968f23c25e7fb747a272b0ee8e85` differs from the baseline only in its
now-removed temporary workflow and original plan. Existing required CI is not
changed. No deployment, production fixture or upstream model request is allowed.

Binaries, built locally with Go1.27.1, identical vendor/embed inputs,
`-mod=vendor -p=2 -trimpath -buildvcs=false`:

- Baseline SHA256: `5f32abdcdb53d10d970c7367d41ea9421d23f7f40a76c0035d50b2243783e3fd`.
- Candidate SHA256: `b0f1b08c18b18e1dbe864d9ab973ba1324b6da7d3cbc53849b0f2debda1d455a`.

The normal gateway migrates a fresh private file-backed SQLite database. Seed
200,000 deterministic log rows over90days,32 users,80% owned by user2,1,024 UTF-8
content bytes per row, with ordinary/tool/provisional types and independent full
DTO/aggregate oracles. Credentials are random, temporary and excluded from output.
Logging, GC, dashboard TTL/defaults and authentication stay unchanged. Existing
cursor APIs are explicitly enabled in both fixtures; product default staysOFF.
Only the enabled API-rate ceiling is raised to10,000,000.

Four targets: personal dashboard, legacy first page, legacy page50 (offset1,000),
and explicit cursor first page. Four controls: site dashboard, administrator logs,
administrator quota statistic, and user self. For each of concurrency1/8 and five
repetitions, alternate baseline/candidate order and create a fresh database clone
and gateway. Validate two warm-up reads on every persistent worker connection
before opening the timed barrier. Measure128 responses per target and512 per
control. Total160 endpoint trials and51,200 measured responses; retain all request
samples, warmups, first-hit qualification calls, resources and completed failures.

Require every response and independent auth/filter/pagination/cursor/ownership
qualification plus read-after-committed-write freshness. The acceptance gate is
unchanged: >=20% paired-median P95 reduction in at least two distinct targets,
each favorable in>=4/5 pairs; reject any error/drop/incorrect or missing data,
control P95 regression exceeding BOTH10% and5ms, or gateway RSS growth>20% in
any endpoint/concurrency cell. No significance inference from pooled requests.
No sample removal, optional favorable reruns, gate changes or partial resumption.

After this entire unprofiled matrix, measure index bytes, startup/migration,
standalone index construction and bulk insert/commit overhead separately. Run
CPU profiles separately on eight disjoint dashboard windows with30s warm-up,
60s observation, private loopback pprof and independent gateway/client accounting.
Report when a DB-bound profile does not meet the inherited50% CPU threshold;
do not increase the workload after seeing results to manufacture qualification.

First hits are startup/application-cache observations, not cold OS-page-cache
latency. Shared-host SQLite timing is not browser paint, WAN/TLS, native
PostgreSQL/MySQL performance, concurrent-writer throughput or production capacity.
