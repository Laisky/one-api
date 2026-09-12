# GitHub Actions

Keep two workflow entrypoints. Ordinary regression tests belong in the existing
Go/frontend suites, not a new workflow for each provider, bug or pull request.
Filenames are retained for existing workflow URLs and badges.

| File / display name | Responsibility | Triggers |
| --- | --- | --- |
| `workflows/lint.yml` / **CI** | Tests, analysis, vulnerability scan, coverage and aggregate gate | All PRs; pushes to `main`, `master`, `test/ci`; merge queues; manual |
| `workflows/ci.yml` / **Build and deploy** | amd64/arm64 images and existing SSH deployment | Existing push branches and delivery exclusions only |

## Go tests: complete, isolated shards

The `go_test_shards` matrix contains **packages** and **model-0..3**. Each job
gets its own runner, MySQL 8.4 and PostgreSQL 17 services, all primary/log/baseline
DSNs, full history for pinned-binary compatibility tests, and ffmpeg. Every job
retains `ONEAPI_REQUIRE_DB_BACKENDS=1`, `ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1`,
`CGO_ENABLED=1` and a 90-minute job budget.

`.github/scripts/go_test_shards.py` discovers packages with `go list ./...`.
The packages shard runs everything except the root `model` package. Four model
shards discover the current top-level tests with `go test -list`, then partition
them deterministically using checked-in timing hints. New tests are discovered
automatically; stale hints cannot remove a test. No subtest is split or filtered.
Fuzz seeds and executable examples remain part of normal Go test execution.

All real runs use `-race -cover -covermode=atomic -count=1 -timeout=45m -json`.
The build cache remains enabled, but cached test results are not accepted. The
matrix uses `fail-fast: false` so a failure does not cancel evidence collection
from the other shards. Go tests remain unconditional on every CI event; the
existing opt-in scale/replication tiers are not silently promoted or removed.
Do not run the model shards concurrently against shared local databases: they
mutate global configuration, schema and fixed ports. CI isolates those resources.

The **Go test completeness and coverage** job (`go_tests`) requires every matrix
job to succeed. It then verifies exactly one artifact per shard, matching source
revision and package inventories, complete/disjoint model selection, actual
JSON test starts/completions, and successful package completion. Missing,
duplicated, failed, cancelled or incomplete results fail the gate. Existing
intentional skips remain visible and the tests' database no-skip guards remain
mandatory. Atomic coverage counters are summed; statement blocks are counted
only once. Coverage is published only after these checks pass.

Each `go-tests-<shard>-<sha>` artifact retains the manifest, JSON events, readable
log, coverage and timing summary for 14 days. Job summaries show slow top-level
tests and skips without double-counting nested subtest durations. The merged
`code-coverage/coverage.txt` artifact continues to feed the optional PR reporter.
See [Go test efficiency](../docs/testing/go-test-efficiency.md) for the baseline,
tradeoffs, reproduction commands and interpretation of the timing data.

## Other required checks

`go vet ./...` and actionlint run once in **Go static guardrails**, alongside the
type-aware entity-response analyzer and Realtime `err113` check. The goroutine
context guard and `govulncheck` remain blocking jobs. Frontend checks are selected
by changed theme/shared build inputs; manual runs test every theme. Frozen Yarn
installs, all three test suites and the Modern production build are retained.

**CI required** remains the stable branch-protection check. It rejects
failed/cancelled checks, missing change-detection output and unexpected skips.
The coverage comment is informational; tests and the merged coverage artifact
are mandatory. Forks and Dependabot do not need secrets or a writable PR token.
Repository protection settings are not changed by these workflows.

## Optional historical evidence

Current-code DeepSeek, GPT Image and Realtime regressions run in the full
package inventory. Dispatch **CI** with `historical_control=true` to additionally
replay the Realtime transcription-index regression against pinned broken
baseline `6138a94f9749312a9c31c7c2ebdcbea758b04235`. The control requires both exit
status 1 and the intended assertion failure; build/network errors do not count.
This manual job is distinct from the mandatory model old-binary compatibility
scenarios, which remain in every complete Go test run.

## Delivery policy

Delivery remains an independent push workflow. Branches, path exclusions,
commit-prefix skip rules, image tags, credential references, concurrency and
SSH commands are unchanged. PR/manual CI cannot publish or deploy. CI completion
is **not** a deployment gate under this existing policy. The native amd64 build
exports its own non-blocking cache; amd64/arm64 caches have separate scopes.

The retired provider-specific workflows and temporary frontend security patch
generators stay removed. No Windows/release job is silently re-enabled.
Dependabot and frozen installs are retained; this setup does not claim to provide
a general frontend OSV audit.

## Validate workflow changes

```sh
python3 -m pip install 'PyYAML==6.0.3'
python3 .github/scripts/test_workflows.py
python3 .github/scripts/test_go_test_shards.py
python3 .github/scripts/test_go_test_shards_integration.py
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -color
```

The integration test needs Go and a race-capable C toolchain, but uses only small
stdlib fixtures and no network. It runs after setup-go on the packages shard.
Workflow tests also execute the real aggregate-gate code for failure,
cancellation, skipping and missing-job cases. Passing orchestration tests is
not a substitute for a successful application/live-database CI run.
