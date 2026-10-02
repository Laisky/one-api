# Go test efficiency and integrity

## Measured baseline

The starting point is commit `5770e82` (the merge of PR #404). Its tree is identical
to tested PR head `9802a73`. Measurements below come from the successful
[PR CI run 34664696244](https://github.com/Laisky/one-api/actions/runs/34664696244),
[Go job 103474154933](https://github.com/Laisky/one-api/actions/runs/34664696244/job/103474154933),
and that run's `go-test-evidence-9802a73efa02285975e31a33ae993e33f3298a37`
artifact. This is one observed run, not a statistically controlled benchmark.

| Observation | Baseline |
| --- | ---: |
| Full Go test command, including compilation/scheduling | 31 min 36 sec |
| Complete Go job, including preparation | 34 min 33 sec |
| `model` package | 1,706.059 sec |
| `controller` package | 393.628 sec |
| `TestCompactUUIDMixedVersionDrift` | 328.03 sec |
| `TestCompactUUIDFaultInjection` | 306.12 sec |
| `TestBehaviorDifferential` (controller) | 330.43 sec |
| Model top-level test results in the verbose log | 502 |
| Successful package summaries in that log | 106 |

Top-level test times already include their subtests: never add parent and child
elapsed times together. Packages can also overlap; summing package times is not
workflow wall time. The checked-in timing hints retain the 60 model tests taking
at least one second in this baseline; they are scheduling hints, not coverage
allowlists or pass/fail thresholds.

## Decisions

### Isolate the serial bottleneck instead of parallelizing shared globals

Model fixtures mutate global DB handles, dialect flags, configuration and health
state; live suites also reset schemas. Adding `t.Parallel()` indiscriminately
would introduce interference rather than reliable speed. The existing Go driver
already runs different packages in different processes, but that does not split
the model package's sequential top-level tests.

Run one non-model package shard and four model shards on separate runners and
separate database services. Assignment uses deterministic longest-first packing
of the current `go test -list` inventory. Every selected top-level test keeps its
entire subtest tree and internal concurrency scenarios. Names are anchored exactly,
so `TestFoo` cannot accidentally select `TestFooBar`, and no slash expression
filters child tests. No source list or timing-hint update is needed to include a
new test. Package discovery includes new packages automatically.

Applied to the baseline inventory, the four groups contain 121/122/129/130 tests,
with summed historical durations of 424.75/428.52/426.58/424.67 seconds. These are
**a scheduling simulation, not measured new CI times**. Compilation, linking,
service startup, resource contention and pinned-binary builds are additional
costs; future test changes also change the assignment.

The tradeoff is extra runner startup and compilation. The pinned-binary helper
already caches builds per process/ref; the change does not claim to fix repeated
builds inside one process. Separate runners cannot share that temporary binary
cache. Shorter wall time does not necessarily mean fewer billable runner minutes.
Keep four model shards initially rather than creating a job per test/provider.

### Replace an unconditional startup sleep with a verified startup barrier

`runPinnedOldBinary` used to sleep for the whole supplied window (typically
22 seconds) every time it launched a historical artifact. Mixed-version drift
alone starts that artifact twice at each of six migration stages.

Both pinned artifacts log server startup after synchronous database bootstrap
and root-account creation. The helper now requires **both** the schema-migration
message and the server-startup message, bounded by the existing maximum budget.
A mutex-protected output collector recognizes messages split across writes. The
existing process liveness check, process kill/wait cleanup, catalog comparisons,
row digests, old/new cycling and UUID resolution assertions remain unchanged.
This barrier proves startup reached the relevant database operations; it is not
an HTTP health check. An overridden binary must emit the same milestones.

This changes a startup deadline from a mandatory dwell into a maximum wait; it
**does not shorten fault injection, lock/outage observation, workload or traffic
windows**. No test scenario, scale fixture or production timeout is reduced.
The pure collector/deadline tests use `testing/synctest` for virtual time; real
child processes and database operations remain outside synctest bubbles.

### Make test completeness and performance observable

Every shard writes its source revision, current package inventory, selected
model tests, exit code, JSON events, readable output, atomic coverage and timing
summary. The merger refuses missing/duplicate shards, mismatched revisions,
package omissions, missing/duplicated top-level model executions, incomplete
streams, failed builds/tests or absent coverage. Go's nonzero exit code cannot
be overwritten by a successful logging step or plausible-looking JSON.

The existing database and compact-suite no-skip guards stay enabled. Existing
manual measurement/scale/replication skips remain visible in reports instead of
being silently turned into purported coverage. Neither `-short`, automatic
retry-until-green, result caching nor reduced race instrumentation is used.
Fuzz seed corpora and executable examples still run; this does not claim to
perform time-budgeted fuzz campaigns.

Each model shard instruments the same statement blocks. The coverage merger
sums atomic execution counters while keeping each statement block only once;
it rejects conflicting statement counts and incompatible modes. The existing
coverage reporter receives one complete profile, not four profiles with an
inflated denominator or a last-writer-wins overwrite.

## Reproduce and validate

The plain full-suite command remains supported:

```sh
ONEAPI_REQUIRE_DB_BACKENDS=1 ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1 \
  go test -race -cover -covermode=atomic -count=1 -timeout=45m ./...
```

Supply the same database DSNs as CI. The measured live/race run needs a larger
budget than the older local Makefile's 20-minute default; use
`make test-race GOTEST_FLAGS='-count=1 -timeout 45m'` for that full qualification.
Do not remove the no-skip flags to make a purported qualification run green.

For one shard in a disposable database environment:

```sh
python3 .github/scripts/go_test_shards.py run \
  --shard model-0 --output /tmp/go-tests-model-0
```

For all five downloaded artifact directories:

```sh
python3 .github/scripts/go_test_shards.py merge \
  --input go-test-results --output coverage.txt
go tool cover -func=coverage.txt
```

Run each shard against isolated databases (as the CI matrix does). Do not launch
several local model shards against the same DSNs. Both commands use the current
Go toolchain; CI gets Go 1.27.1 from `go.mod`.

Before accepting a speedup claim, compare completed baseline/candidate runs:
full job wall time, individual command time, all-shard completeness, executed
and skipped tests, coverage, and runner minutes. Retain raw artifacts for the
comparison. Synthetic orchestration tests and a timing simulation are not a
successful application qualification or a measured performance result.

## Additional audit finding

The controller differential fixture hashes three constant test passwords anew
for every case. Reusing production-strength fixture hashes is a separate useful
optimization; this change leaves that self-contained historical harness and its
frozen response baselines unchanged. The current non-model shard still executes
all of its cases. Do not lower password work factors, regenerate golden files,
or remove negative controls to obtain faster numbers.

## Primary references consulted

- [Go command documentation](https://pkg.go.dev/cmd/go): package discovery,
  `-list`, `-run`, `-count`, `-json`, race instrumentation and atomic coverage.
- [Go testing documentation](https://pkg.go.dev/testing): subtest selection,
  parallel tests and process-global environment constraints.
- [Testing time (Go team)](https://go.dev/blog/testing-time): synchronization and
  fake-time tests without wall-clock sleeps.
- [testing/synctest](https://pkg.go.dev/testing/synctest): bounded in-process
  concurrency tests; not virtualization of external processes or database I/O.
- [test2json event format](https://pkg.go.dev/cmd/test2json): test/package start,
  completion, skip and failure records used for evidence validation.

No new third-party Go testing framework, mocking layer, container orchestrator,
or automatic flaky-test retry system is required for these changes.
