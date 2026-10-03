# Autopsy: The Idle PostgreSQL Backends That Each Appeared to Use 250 MiB

- Status: Diagnosed; One API pool defaults fixed in the working tree; PostgreSQL tuning not deployed
- Date: 2026-09-11
- Area: PostgreSQL / Linux memory accounting / connection pools / container operations
- Audience: backend engineers and operators
- Environment: PostgreSQL 17 in Docker on a 2 GiB host
- Related files: `common/config/database_defaults.go`, `common/config/config.go`,
  `model/main.go`, `common/benchdb/benchdb.go`
- Related docs: [The External UUID Backfill That Could Never Finish](./20260910_uuid-backfill-never-converges.md),
  [The Completed Compact UUID Migration That Kept Re-Proving Itself](./20260910_compact-uuid-steady-state-cost.md)

## 1. Summary

The incident began with an alarming process list. Several PostgreSQL processes were named
`idle`, each showed roughly 230 to 260 MiB RSS, the host had only 2 GiB RAM, and about half of its
swap was in use. Adding the PostgreSQL RSS column produced roughly 2.6 GiB. The first hypothesis
was a connection leak: too many idle backends, each retaining a large allocation.

That hypothesis was wrong in two different ways.

First, there were only five idle client backends. PostgreSQL also had its normal checkpointer,
background writer, WAL writer, autovacuum launcher, and logical replication launcher. One
diagnostic connection was active while the snapshot was taken. The five application backends
were ordinary pool connections waiting on `ClientRead`; none was `idle in transaction`, none held
an XID or `xmin`, and none was blocking another session.

Second, RSS is not additive for PostgreSQL. Every backend maps the same shared buffers, shared
libraries, and inherited pages. `ps` charged those pages to every process, so adding RSS counted
the same physical memory repeatedly. Process proportional set size and private memory were about
300 MiB and 65 MiB respectively across one sampled set of PostgreSQL processes. The PostgreSQL
container's authoritative cgroup charge was about 370 to 420 MiB during the initial investigation
and later fell to roughly 270 to 320 MiB as cache and page residency changed.

The cgroup number was real, and for this low-traffic installation it was still larger than
necessary. Its main components were:

- about 148 MiB of PostgreSQL main shared-memory allocation, including 128 MiB of shared buffers;
- PostgreSQL process and inherited anonymous memory;
- roughly 95 MiB of ordinary, reclaimable file cache in one representative sample; and
- about 14 to 26 MiB of kernel accounting across the samples.

The custom image also preloaded five libraries into the postmaster: `vchord`, `vchord_bm25`,
`vector`, `pg_tokenizer`, and `pg_stat_statements`. A cluster-wide extension inventory found
`plpgsql` everywhere and only two relevant non-default extensions: `vector` in one database and
`pgcrypto` in two databases. The other preloaded extensions,
including `pg_stat_statements`, were not installed anywhere. They are the strongest candidate for
avoidable inherited process memory, but the exact saving must be measured with a controlled
restart rather than inferred from one memory map.

One API had a separate configuration problem. Its pool defaults allowed 200 idle and 2,000 open
connections per pool, despite the production PostgreSQL server allowing 100 connections total.
Those large limits were a future exhaustion risk, not the cause of the current 420 MiB: One API
had only two connections at the time. The repository defaults were reduced to 10 idle and 50 open
connections, with the existing 300-second lifetime retained, as a reasonable envelope for one
instance serving approximately 100 requests per second.

## 2. Impact

No failed request, blocked transaction, or current database throughput regression was found in
this investigation. The impact was capacity and operational ambiguity:

- PostgreSQL initially charged roughly 370 to 420 MiB of a 2 GiB host while serving very little
  traffic, then cooled below that range without a restart.
- The container held roughly 140 to 271 MiB of swapped pages across different samples, although
  there was no sustained swap-in or swap-out activity during observation.
- Summed process RSS overstated PostgreSQL physical memory by more than six times and made healthy
  pool backends look like independent 250 MiB consumers.
- One API's 2,000-connection ceiling could have overwhelmed PostgreSQL long before the application
  reached that limit.
- Blank `application_name` values and Docker-proxy source addresses made session ownership slower
  to establish than it should have been.

This was not the same defect as the previous day's migration incidents. Those incidents produced
similar fat-looking idle backends because repeated scans touched the entire shared buffer pool.
Their root causes and query-plan evidence are recorded in the two related autopsies. On 2026-09-11
the same visual symptom remained, but the repeated work had stopped. The new investigation had to
prove that difference instead of assuming the old cause was still present.

## 3. Initial Symptom and Hypotheses

The initial process list showed 11 PostgreSQL processes. Most reported approximately a quarter
gigabyte of RSS, including idle application backends and normally small background workers.

The working hypotheses were deliberately kept separate:

1. **Connection explosion.** An application pool might have opened tens or hundreds of sessions.
2. **Idle transaction leak.** A small number of sessions might retain snapshots, locks, or large
   transaction state.
3. **Backend-private memory retention.** Expensive queries might have grown allocator arenas that
   remained attached to idle sessions.
4. **Shared-memory double counting.** RSS might mostly contain the same mapped pages in every
   process.
5. **Real PostgreSQL baseline.** Shared buffers, preloaded extensions, and process infrastructure
   might genuinely be too large for the workload.
6. **Active workload.** A background worker or query loop might continually scan data and keep the
   database hot even when the sampled backend happened to be idle.
7. **Linux cache or swap artifact.** File cache or cold shared pages might make memory look scarce
   without current reclaim pressure.

The order matters. Killing sessions before distinguishing these hypotheses would destroy evidence,
the pool would probably recreate the connections, and a lower process count could falsely appear
to confirm the wrong explanation.

## 4. Investigation

### 4.1 Establish the host-level facts

The first step recorded UTC time, uptime, load, physical memory, swap, and PostgreSQL processes:

```bash
date -u
uptime
free -h
vmstat 1 10
cat /proc/pressure/memory
ps -C postgres -o pid=,ppid=,stat=,rss=,etime=,cmd= --sort=-rss
```

The representative host snapshot showed:

| Observation | Value |
| --- | ---: |
| Host memory | 2.0 GiB |
| Memory available | about 593 MiB |
| Host swap | about 1.0 GiB used of 2.0 GiB |
| Load average | below 0.3 |
| PostgreSQL processes | 11 at steady state; one temporary diagnostic backend while querying |
| Sum of PostgreSQL RSS | about 2.6 GiB |

The last two rows contradicted a literal reading of RSS: eleven processes could not privately
hold 2.6 GiB resident on a 2 GiB host while other services were running. That did not prove the
system was healthy, but it proved that summed RSS was the wrong total.

`MemAvailable` mattered more than `MemFree`. Linux intentionally uses spare memory for cache, and
the host still estimated hundreds of megabytes reclaimable without swapping. `vmstat` and memory
pressure did not show ongoing reclaim or swap thrashing. Existing swap occupancy described past
pressure or cold pages; it did not prove current contention.

### 4.2 Measure the container as one accounting unit

The next measurement moved from individual processes to the PostgreSQL cgroup:

```bash
docker stats --no-stream <postgres-container> <application-container>
docker top <postgres-container> -eo pid,ppid,rss,etime,args
docker inspect --format \
  '{{.Name}} memory={{.HostConfig.Memory}} memory_swap={{.HostConfig.MemorySwap}} shm={{.HostConfig.ShmSize}}' \
  <postgres-container>
```

Docker reported roughly 390 MiB for PostgreSQL, not 2.6 GiB. The container had no explicit memory
limit. Docker's displayed working set is convenient, but it can subtract some inactive file cache,
so the investigation also read cgroup v2 directly.

Resolve the cgroup from the container's host PID rather than guessing its path:

```bash
POSTGRES_PID=$(docker inspect --format '{{.State.Pid}}' <postgres-container>)
CGROUP_PATH=$(awk -F: '$1 == "0" {print $3}' "/proc/${POSTGRES_PID}/cgroup")
sudo cat "/sys/fs/cgroup${CGROUP_PATH}/memory.current"
sudo cat "/sys/fs/cgroup${CGROUP_PATH}/memory.swap.current"
sudo cat "/sys/fs/cgroup${CGROUP_PATH}/memory.stat"
```

Three samples taken while connection ages and page residency changed illustrate why a range is more
honest than one exact number:

| cgroup field | Sample A | Sample B | Later cool sample | Meaning |
| --- | ---: | ---: | ---: | --- |
| `memory.current` | about 404 MiB | about 371 MiB | about 319 MiB | Total physical memory charged to the container |
| `anon` | about 214 MiB | about 144 MiB | about 84 MiB | Anonymous process and shared mappings |
| `file` | about 168 MiB | about 204 MiB | about 199 MiB | File-backed charge, including `shmem` |
| `shmem` | about 93 MiB | about 109 MiB | about 118 MiB | Subset of `file`; do not add it again |
| `kernel` | about 15 MiB | about 14 MiB | about 25 MiB | Page tables, slab, and other kernel memory |
| `memory.swap.current` | about 140 MiB | about 214 MiB | about 271 MiB | Container pages currently in swap |

Physical residency and swap placement changed while the service remained healthy. This is why a
before/after tuning comparison must use several samples under comparable traffic and warm-up.

### 4.3 Count and classify PostgreSQL sessions

The process name `postgres: ... idle` is useful, but `pg_stat_activity` is authoritative about
what each backend is doing. The diagnostic query intentionally omitted the `query` column because
production SQL text can contain tokens, user data, or request payloads.

```sql
SELECT
    backend_type,
    COALESCE(state, 'not applicable') AS state,
    count(*) AS sessions
FROM pg_stat_activity
GROUP BY backend_type, state
ORDER BY backend_type, state;

SELECT
    datname,
    usename,
    application_name,
    client_addr,
    state,
    count(*) AS sessions,
    max(clock_timestamp() - state_change) AS longest_in_state,
    max(clock_timestamp() - backend_start) AS oldest_connection
FROM pg_stat_activity
WHERE backend_type = 'client backend'
GROUP BY datname, usename, application_name, client_addr, state
ORDER BY sessions DESC;
```

The result was small:

| Owner | Database workload | State | Connections | Observed behavior |
| --- | --- | --- | ---: | --- |
| GraphQL service | MCP data | `idle` | 3 | Waiting on `ClientRead` |
| One API | primary data | `idle` | 2 | Waiting on `ClientRead`; recently reused |
| Diagnostic shell | MCP data | `active` | 1 | The inspection query itself |

The five rows with no client state were PostgreSQL background processes, not leaked application
connections. The longest MCP connection was about 59 minutes old but had been idle for only about
12 minutes, showing that the pool had reused it. One API's oldest connection was below five
minutes, matching its configured connection lifetime.

`idle` means PostgreSQL is waiting for the client's next command. It is not equivalent to `idle in
transaction`, which can retain a snapshot, delay vacuum cleanup, and hold locks. The incident had
zero idle transactions.

### 4.4 Rule out open transactions and blockers

Session state was followed by explicit transaction and lock checks:

```sql
SELECT
    pid,
    datname,
    usename,
    application_name,
    state,
    clock_timestamp() - xact_start AS transaction_age,
    wait_event_type,
    wait_event,
    query_id
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
ORDER BY xact_start;

SELECT
    pid,
    state,
    wait_event_type,
    wait_event,
    pg_blocking_pids(pid) AS blocker_pids,
    query_id
FROM pg_stat_activity
WHERE cardinality(pg_blocking_pids(pid)) > 0
ORDER BY pid;

SELECT gid, prepared, owner, database
FROM pg_prepared_xacts
ORDER BY prepared;
```

No long-lived application transaction or blocker explained the memory. This ruled out the most
operationally dangerous interpretation of `idle` before spending time on memory accounting.

### 4.5 Attribute clients despite Docker proxying

Every database session reported the Docker bridge gateway as `client_addr`, and every
`application_name` was blank. Database metadata alone therefore could not name the owning
container.

The investigation correlated four pieces of metadata without inspecting secrets:

- database name and connection count from `pg_stat_activity`;
- established TCP socket count in each candidate application container;
- container network membership and IP address; and
- application role from the service configuration, inspecting key names rather than DSN values.

That correlation matched exactly: the GraphQL container owned three MCP connections and the One
API container owned two primary-database connections. They appeared to originate at the gateway
because both applications connected through a host-published PostgreSQL endpoint and Docker proxy
forwarded the traffic back into the database container.

The safer long-term observability fix is to set a distinct `application_name` in each database
client. Direct service-to-service Docker networking would also preserve more useful source
identity and avoid the host-publish round trip.

### 4.6 Split shared, proportional, and private process memory

`/proc/<pid>/smaps_rollup` provided the decisive process-level correction:

```bash
sudo grep -E \
  '^(Rss|Pss|Pss_Anon|Pss_File|Pss_Shmem|Shared_Clean|Shared_Dirty|Private_Clean|Private_Dirty|Swap|SwapPss):' \
  /proc/<postgres-pid>/smaps_rollup
```

A representative idle backend showed roughly 244 MiB RSS but only about 41 MiB PSS and 18 MiB
private memory. Most RSS was `Shared_Dirty`: the same shared pages appeared in every backend's
accounting.

Summing the rollups for the sampled PostgreSQL processes gave approximately:

| Metric | Aggregate | Correct interpretation |
| --- | ---: | --- |
| RSS | about 2.6 GiB | Invalid as a physical total because shared pages are repeated |
| PSS | about 295 MiB | Shared pages divided among mapping processes |
| Private resident memory | about 65 MiB | Pages attributable only to individual processes |
| cgroup physical charge | about 270-420 MiB across the investigation | Best whole-container total, including cache and kernel charge |

Per-process `Swap` has the same trap as RSS: a shared swapped mapping can appear in several
process rollups. `SwapPss` or the cgroup's `memory.swap.current` is the meaningful aggregate.

The five idle clients contributed only about 21 MiB private memory in a later sample. A lower idle
cap recovers backend memory only after a pool has grown beyond that cap. Because One API had two
idle connections and its new default is ten, the default change would recover nothing from this
particular snapshot; it limits retention after a future burst.

### 4.7 Ask PostgreSQL what it allocated as shared memory

The `pg_shmem_allocations` view separated configured shared memory from allocator inference:

```sql
SELECT
    name,
    pg_size_pretty(sum(size)::bigint) AS size
FROM pg_shmem_allocations
GROUP BY name
ORDER BY sum(size) DESC;

SELECT pg_size_pretty(sum(size)::bigint) AS total_shared_memory
FROM pg_shmem_allocations;
```

The cluster reported about 148 MiB total:

| Allocation | Size |
| --- | ---: |
| Buffer blocks | 128 MiB |
| Anonymous shared area | about 8.7 MiB |
| WAL control | about 4 MiB |
| Buffer descriptors | 1 MiB |
| Remaining control and status structures | about 6 MiB |

This proved that `shared_buffers=128MB` was one shared allocation, not 128 MiB per connection.
It also set a floor for configured shared allocation, not physical residency: connection-pool
changes cannot resize the main segment, but not every allocated page must be resident at once.

`pg_shmem_allocations` covers the main shared-memory segment. It does not include every dynamic
shared allocation or ordinary process heap, so it must be combined with cgroup and PSS data rather
than treated as the cluster total.

### 4.8 Inventory memory-relevant settings

The settings query included value, source, and restart status:

```sql
SELECT name, setting, unit, source, sourcefile, pending_restart
FROM pg_settings
WHERE name IN (
    'shared_buffers',
    'wal_buffers',
    'work_mem',
    'maintenance_work_mem',
    'autovacuum_work_mem',
    'max_connections',
    'autovacuum_max_workers',
    'max_worker_processes',
    'max_parallel_workers',
    'max_parallel_workers_per_gather',
    'idle_session_timeout',
    'idle_in_transaction_session_timeout',
    'shared_preload_libraries'
)
ORDER BY name;
```

The relevant active values were:

| Setting | Value | Baseline implication |
| --- | ---: | --- |
| `shared_buffers` | 128 MiB | Main shared allocation; restart required to change |
| `wal_buffers` | 4 MiB effective | Small shared allocation |
| `work_mem` | 4 MiB | On demand per executor operation, not idle baseline |
| `maintenance_work_mem` | 64 MiB | On demand for maintenance work |
| `autovacuum_work_mem` | inherited 64 MiB | Potential per autovacuum worker, not currently allocated in full |
| `max_connections` | 100 | Capacity and future risk; little immediate baseline saving from lowering |
| `idle_session_timeout` | 0 | Ordinary idle sessions are not expired by the server |
| `idle_in_transaction_session_timeout` | 0 | Irrelevant to ordinary idle sessions |

`work_mem` was not multiplied by 100 and allocated at startup. It is a limit that may apply to
multiple sort or hash nodes in each active query, and parallel/concurrent work can multiply it.
Lowering it would cap future spikes, not materially reduce this idle snapshot.

### 4.9 Compare preloaded libraries with installed consumers

The PostgreSQL process command line and the image entrypoint showed:

```text
shared_preload_libraries=vchord,vchord_bm25,vector,pg_tokenizer,pg_stat_statements
```

The installed-extension inventory was then checked in every connectable database. Besides the
default PL/pgSQL extension, only pgvector and pgcrypto were installed; no database installed
VectorChord, BM25, the tokenizer, or `pg_stat_statements`. No vector index of the inspected access
methods existed.

That distinction matters:

- a library being present in an image does not mean a database uses it;
- a library in `shared_preload_libraries` runs in the postmaster even when no current query names it;
- removing a genuinely required preload can break an extension or observability feature; and
- `CREATE EXTENSION` is database-local, so checking only the currently connected database is not
  sufficient.

The process memory map showed large inherited anonymous regions in the postmaster. The unused
preloads are a plausible contributor because libraries loaded before PostgreSQL forks are inherited
by every backend. The investigation did not assign an exact byte count to a specific extension.
That requires an A/B restart with the same data and workload, first with the existing preload list
and then with only proven consumers.

### 4.10 Inspect the application pool rather than guessing from PostgreSQL

Only the three relevant, non-secret variables were inspected in the application container. The
complete environment was never dumped because it contains credentials.

```bash
docker exec <application-container> sh -c '
  for key in SQL_MAX_IDLE_CONNS SQL_MAX_OPEN_CONNS SQL_MAX_LIFETIME; do
    value=$(printenv "$key")
    if [ -n "$value" ]; then
      printf "%s=%s\n" "$key" "$value"
    else
      printf "%s=[unset]\n" "$key"
    fi
  done
'
```

All three were unset, so the code defaults applied:

| Setting | Old default |
| --- | ---: |
| Idle connections | 200 |
| Open connections | 2,000 |
| Connection lifetime | 300 seconds |

The startup log confirmed those effective values. They were excessive relative to PostgreSQL's
100-connection server limit, but a limit is not usage: One API had only two live sessions. The
pool configuration was therefore classified as a serious future risk and a poor default, not the
root cause of the observed container baseline.

## 5. Root Cause and Contributing Factors

There was no single memory leak. Four separate facts created the incident:

### 5.1 The display symptom was duplicated shared RSS

PostgreSQL's process architecture makes `ps` RSS useful for finding which backend has touched many
pages, but unsafe to sum. Shared buffers and other mappings appeared in every process. This was the
reason several idle processes each looked like a quarter-gigabyte consumer.

### 5.2 The container had a real but mixed footprint

The real total contained PostgreSQL shared allocation, process memory, kernel charge, and file
cache. File cache was reclaimable. Shared buffers were intentionally resident cache. Anonymous
memory and swapped inherited pages deserved tuning attention, but a high cgroup total by itself did
not demonstrate a leak.

### 5.3 The image preloaded capabilities the cluster did not use

The custom entrypoint selected an observability and vector-search feature set globally. The live
cluster did not install most of those extensions. This widened the postmaster baseline and made the
image's default feature set inappropriate for a small general-purpose deployment.

The exact memory contribution remains an explicit measurement task. Removing the unused list is
well supported by the consumer inventory; claiming an exact saving before the restart is not.

### 5.4 One API's pool limits were capacity hazards, not current consumers

The application could theoretically request twenty times more connections than PostgreSQL allowed.
At the observed traffic it had opened only two, so lowering the cap would not erase the current
baseline. It would prevent a burst, regression, or replica multiplication from turning the next
incident into connection exhaustion.

Pool limits are per process and per database pool. A separate `LOG_SQL_DSN` creates another pool
with the same limits, and replicas multiply both. Capacity planning must use the sum across every
pool and leave room for migrations, workers, monitoring, and emergency access.

## 6. Remediation Decisions

### 6.1 Completed in the working tree: bounded One API defaults

The repository now defaults to:

```text
SQL_MAX_IDLE_CONNS=10
SQL_MAX_OPEN_CONNS=50
SQL_MAX_LIFETIME=300
```

The provisional target is one instance handling approximately 100 HTTP requests per second. The
sizing is based on database occupancy rather than equating one request with one connection, but it
is a policy assumption until a controlled 100-RPS test measures pool waits and tail latency.

A normal relay request performs several short database operations before and after the upstream
model call. It does not hold a SQL connection while waiting for the model provider or streaming a
response. As a hypothetical sizing model, 1,500 short SQL operations per second at 3-10 ms average
connection occupancy would imply roughly 5-15 concurrent connections. Those inputs were not
measured during this incident; the load test in section 7.1 must validate them. Ten idle connections
retain a useful warm set, and 50 open connections provide burst and slow-query headroom while
leaving capacity on a common 100-connection PostgreSQL deployment.

These are defaults, not universal limits. Operators must reduce them when several replicas or pools
share a small server, and may raise them only from measured acquisition waits and database capacity.

### 6.2 Proposed PostgreSQL change: remove unused preloads first

The highest-confidence database change is to override `shared_preload_libraries` with only proven
consumers. On the inspected cluster no preloaded library had an installed consumer that required
startup loading, and `pg_stat_statements` was not installed in any database.

This change requires a PostgreSQL restart. Before applying it, the operator must confirm that no
external monitoring system depends on `pg_stat_statements` and that no imminent VectorChord,
BM25, or tokenizer rollout assumes the image default.

### 6.3 Optional second step: test 64 MiB shared buffers

Reducing `shared_buffers` from 128 MiB to 64 MiB can lower the main shared allocation by up to
64 MiB. For a low-throughput 2 GiB multi-service host this is a reasonable experiment, but it is not
automatically an improvement: PostgreSQL and Linux cooperate through both shared buffers and the
operating-system page cache.

The correct sequence is to remove unused preloads, restart, warm the service, measure, and only then
A/B test the smaller buffer pool. Query latency, physical reads, cache hit behavior, checkpoint
activity, and host pressure decide whether the saving is worthwhile.

### 6.4 Cap potential maintenance spikes separately

`autovacuum_work_mem=-1` made each autovacuum worker inherit the 64 MiB
`maintenance_work_mem`. With three workers, that is a potential workload-dependent allocation of
roughly 192 MiB. Setting `autovacuum_work_mem=32MB` would cap that class of spike at approximately
96 MiB while retaining three workers. It does not explain or reduce the idle baseline by 96 MiB;
the memory is allocated as needed.

### 6.5 Actions deliberately rejected

- **Do not sum RSS.** It cannot be repaired with a PostgreSQL setting.
- **Do not kill ordinary idle backends as diagnosis.** Pools recreate them and erase ownership
  evidence.
- **Do not set `idle_in_transaction_session_timeout` to target ordinary idle sessions.** It governs
  a different state.
- **Do not use `idle_session_timeout` as the first pool control.** Server-side expiry can race with
  client pooling; application-owned limits are more predictable.
- **Do not lower `max_connections` first.** It saves little current memory and can turn a sizing
  mistake into an outage. Cap every client pool before reducing server capacity.
- **Do not drop Linux caches.** Cache is useful and reclaimable; forcing it out creates cold-start
  I/O and makes the graph look better without improving capacity.
- **Do not restart merely to prove a leak.** Restarting discards caches, sessions, and allocator
  state at once, so a lower number immediately afterwards cannot identify which component mattered.

## 7. Verification and Deployment Plan

### 7.1 Application-default verification

The working-tree default change passed:

| Check | Result |
| --- | --- |
| Focused configuration, model, and benchmark tests | PASS |
| `go vet ./...` | PASS |
| `go test -race ./...` | PASS |
| `make build-frontend-modern` | PASS |
| `git diff --check` | PASS |

The deployed service will not change until a new image is built and rolled out. After deployment,
the startup log should report 10 idle, 50 open, and a 300-second lifetime unless deployment-specific
overrides are present.

Before treating the profile as capacity-proven, run a representative 100-RPS test and observe
`sql.DB` in-use, idle, wait-count, and wait-duration deltas together with PostgreSQL session count,
query latency, and application tail latency. Multiple replicas and a separate logging pool must be
included in the connection budget.

### 7.2 Before changing PostgreSQL

Capture at least several minutes of comparable baseline data:

- request rate, error rate, and latency;
- cgroup `memory.current`, `memory.swap.current`, and selected `memory.stat` fields;
- host `MemAvailable`, swap-in/out, I/O wait, and memory pressure;
- session counts grouped by database, application, and state;
- oldest transaction, blockers, and connection ages;
- database reads, cache hits, temporary bytes, checkpoints, and WAL volume as interval deltas; and
- effective settings with `source` and `pending_restart`.

Never reset production statistics only to make a before/after graph easier. Record counter values
and compare deltas.

### 7.3 After changing PostgreSQL

Verify startup and active configuration:

```bash
pg_isready
```

```sql
SELECT
    pg_postmaster_start_time(),
    current_setting('shared_buffers') AS shared_buffers,
    current_setting('shared_preload_libraries') AS shared_preload_libraries;

SELECT name, setting, unit, source, pending_restart
FROM pg_settings
WHERE name IN ('shared_buffers', 'shared_preload_libraries');
```

Then wait for a representative warm-up period and compare like with like. A successful restart is
not a performance result. Acceptance requires:

- the application reconnects and serves all API formats normally;
- no required extension or query-observability feature disappears unexpectedly;
- no connection-acquisition errors or sustained wait growth appears;
- latency and error rate remain within the previous envelope;
- no new blockers or idle transactions appear; and
- cgroup physical and swap usage improve after equivalent workload and warm-up.

Rollback triggers should be defined before the restart: failed extension startup, lost required
monitoring, connection errors, sustained latency regression, or no material memory improvement.

## 8. Reusable PostgreSQL Memory and Performance Playbook

The reusable method is an evidence ladder. Each layer answers a different question, and later work
depends on the earlier answer.

### 8.1 Question one: is the machine under pressure?

```bash
date -u
free -h
vmstat 1 10
cat /proc/pressure/memory
```

Look for sustained swap-in/out, I/O wait, low `MemAvailable`, and pressure stalls. Swap occupancy
without activity is not enough. High used memory without pressure is not enough.

### 8.2 Question two: how much memory belongs to PostgreSQL as a whole?

```bash
docker stats --no-stream <postgres-container>
docker top <postgres-container> -eo pid,ppid,rss,etime,args
```

On cgroup v2, prefer `memory.current` for the whole container and use `memory.stat` to separate
anonymous memory, file cache, shared memory, and kernel charge. Remember that `shmem` is included
in `file` on this accounting path.

### 8.3 Question three: what are the sessions doing?

Start with `backend_type`, `state`, transaction age, wait event, database, application, and client.
Avoid production query text unless it is necessary and explicitly scrubbed. The most important
classification is:

| State | Meaning | First concern |
| --- | --- | --- |
| `active` | Executing a statement | Wait event, duration, plan, I/O |
| `idle` | Waiting for the next client command | Pool ownership and private retained memory |
| `idle in transaction` | Waiting while a transaction remains open | Locks, snapshots, vacuum delay |
| background backend | PostgreSQL infrastructure or extension worker | Backend type and expected count |

### 8.4 Question four: is there blocking or unfinished transactional work?

Use `xact_start`, `backend_xid`, `backend_xmin`, `pg_blocking_pids`, and prepared transactions.
Handle blockers before tuning cache or pool size; they change both latency and memory behavior.

### 8.5 Question five: is memory shared, private, cached, or swapped?

Use cgroup totals for the container, PSS for a proportional process view, private dirty/clean for
backend-specific retention, and `SwapPss` or cgroup swap for swapped shared pages. Use
`pg_shmem_allocations` to explain PostgreSQL's main shared segment.

No single metric answers all four categories:

| Metric | Good for | Common mistake |
| --- | --- | --- |
| RSS | Finding processes that mapped or touched many pages | Summing it across PostgreSQL backends |
| PSS | Dividing shared mappings proportionally | Treating it as cache-inclusive cgroup usage |
| Private memory | Finding backend-specific retention | Ignoring shared baseline and file cache |
| `memory.current` | Whole-cgroup physical charge | Assuming every page is unreclaimable application heap |
| `memory.swap.current` | Whole-cgroup swap charge | Summing per-process `Swap` instead |
| `pg_shmem_allocations` | Main PostgreSQL shared segment | Treating it as all PostgreSQL memory |

### 8.6 Question six: what workload created the footprint?

Memory diagnosis does not replace query diagnosis. Capture interval deltas rather than lifetime
counters:

```sql
SELECT datname, blks_read, blks_hit, temp_files, temp_bytes, deadlocks
FROM pg_stat_database
ORDER BY datname;
```

If `pg_stat_statements` is already enabled and operationally required, group by `queryid` and
compare calls, execution time, rows, shared-block reads, and temporary I/O over the incident
window. Do not include raw query text in a shared incident report without scrubbing it.

For a known read-only candidate, use:

```sql
EXPLAIN (ANALYZE, BUFFERS, WAL, SETTINGS, SUMMARY)
SELECT ...;
```

`EXPLAIN ANALYZE` executes the statement. Never apply it casually to a production write. Use a
transaction that is safely rolled back, a replica, or a production-shaped test database when the
statement can mutate state.

The previous migration incidents demonstrate the full loop: catch the query while active, connect
its cadence to application logs, reproduce its plan with buffers, trace it to its worker, and add a
cost-shaped regression test. This incident stopped earlier because the active-work evidence was
absent.

### 8.7 Question seven: who owns each connection?

Use `application_name` whenever possible. Otherwise correlate database name, role, client address,
socket ownership, container network metadata, and service configuration. Inspect only named,
non-secret configuration keys; never dump all container environment variables into terminal output
or documentation.

### 8.8 Question eight: which setting changes the measured component?

Map every proposal to the memory category it can affect:

| Change | Affects idle baseline? | Main tradeoff |
| --- | --- | --- |
| Lower application `MaxIdleConns` | Yes, backend-private memory | Reconnection churn after bursts |
| Lower application `MaxOpenConns` | Mostly future peak/concurrency | Queueing and acquisition waits |
| Lower `shared_buffers` | Yes, main shared segment | More reliance on OS cache and possible reads |
| Remove unused preload | Potentially yes | Lost extension or monitoring capability if ownership was missed |
| Lower `work_mem` | Usually no | More temporary I/O for active sorts/hashes |
| Lower `autovacuum_work_mem` | Usually no | Slower maintenance, smaller future spikes |
| Lower `max_connections` | Little immediate saving | Connection refusal if pools are not capped first |
| Set `idle_session_timeout` | Can reduce sessions | Pool/server lifecycle conflict |

If a proposed setting cannot affect the component that is large, it is not the primary fix.

## 9. Common Diagnostic Traps

- Treating `ps` RSS as private memory.
- Adding `shmem` to `file` even though the cgroup reports it as a subset.
- Treating `idle` as shorthand for `idle in transaction`.
- Assuming an old connection has been continuously idle; use `state_change`.
- Assuming a configured maximum is current usage; count live sessions.
- Assuming `work_mem` is reserved once per connection.
- Killing sessions before attributing them to their owner.
- Dumping full DSNs, environment variables, or production SQL text while gathering evidence.
- Removing a preload solely because no active query names it; inventory every database and external
  consumer first.
- Comparing a warm pre-restart service with a cold post-restart service.
- Calling retained memory a leak without demonstrating monotonic growth under stable workload.
- Tuning for request rate without measuring database occupancy and replica/pool multiplication.

## 10. Lessons

### 10.1 Check what the number means before explaining why it is large

The fastest decisive command in this investigation was not a query-plan tool. It was
`smaps_rollup`, which split shared from private memory and invalidated the initial mental model.
Only then did it make sense to investigate the real cgroup total.

### 10.2 The same symptom does not guarantee the same incident

On the previous day, fat idle backends were evidence left behind by repeated migration scans. On
this day, the scans were gone and the remaining idle backends were ordinary pool connections. The
name and RSS looked similar; the session, query, cadence, and I/O evidence did not.

### 10.3 Limits, usage, and allocation are different facts

`max_connections=100`, an application maximum of 2,000, five live sessions, 148 MiB allocated
shared memory, and 270-420 MiB cgroup usage across the observation window are five different
measurements. Replacing one with another produces confident but wrong diagnoses.

### 10.4 Tune the owner of the lifecycle

Application pools should own ordinary idle-connection count. PostgreSQL should enforce a final
capacity boundary. Server idle timeouts are useful safeguards, but they are a poor substitute for
pool configuration when the client can express the intended lifecycle directly.

### 10.5 Every optimization needs a before/after contract

Removing a preload, shrinking shared buffers, or lowering pool limits can all reduce a graph. The
change is successful only if request latency, acquisition waits, error rate, maintenance health,
and required observability remain acceptable under comparable load.

## 11. References

- [PostgreSQL 17: Monitoring Database Activity](https://www.postgresql.org/docs/17/monitoring-stats.html)
- [PostgreSQL 17: `pg_shmem_allocations`](https://www.postgresql.org/docs/17/view-pg-shmem-allocations.html)
- [PostgreSQL 17: Resource Consumption](https://www.postgresql.org/docs/17/runtime-config-resource.html)
- [Linux kernel: Control Group v2](https://docs.kernel.org/admin-guide/cgroup-v2.html)
- [pgvector](https://github.com/pgvector/pgvector)
