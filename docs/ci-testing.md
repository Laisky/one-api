# Automatic checks and manual qualification

The automatic pre-merge test budget is formatting plus the fastest essential unit
tests. Full integration, race/coverage, database migration, performance, stress,
fuzz, browser, and environment qualification runs manually on local development
or staging. This policy replaces the previous unconditional full-test and
frontend-build policy under the repository owner's instruction on 2026-10-08.
Existing behavior assertions remain intact.

`lint.yml` remains the sole PR validation workflow. Normal push, pull request, and
merge-group events run:

- Go formatting and the explicit essential test selection (`make test-quick`).
- Fast workflow/orchestration integrity tests and affected-frontend detection.
- Existing Go vet, entity-response, request-context, sentinel-error, and workflow
  guardrails, plus the Go dependency vulnerability scan.
- Existing complete runtime/development dependency audits for affected Modern,
  Air, and Berry themes. Security checks remain required and may take longer than
  the small unit-test budget.
- `CI required`, which fails on failed/cancelled jobs, missing mandatory results,
  invalid frontend selection, or skipped applicable security audits.

The quick runner discovers the live inventories for its selected packages and
requires every selected test to emit a passing terminal JSON result. It runs
fresh tests with `-count=1`, rejects skipped essential cases and missing outcomes,
and retains inventories, raw JSON, commands, exit codes, and elapsed times.
This is a focused gate; its evidence does not claim full-suite coverage.
The selection includes every discovered test in `relay/format`: request-format
detection and endpoint selection for Chat Completions, Responses, and Claude
Messages. Core message/usage/tool serialization and protocol adapter conversion
suites remain in manual full qualification. Including adapter package
compilation in a trial cold gate exceeded its 240-second wall budget; that failed
trial is not reported as a successful speed result.
A two-package format/model cold trial passed in 222.063 seconds, but left
insufficient room for formatting and runner setup. The final gate therefore
keeps only the smallest essential package instead of increasing the timeout.

## Local quick gate

Use Go 1.27.2, Python 3, Git, and the matching `gofmt` for the current validation
policy. All five Go setup steps in `lint.yml` explicitly select this patched
compiler independently of the minimum Go 1.27.1 requirement in `go.mod`. This
compiler selection does not change deployment/build toolchains or the module
compatibility floor. Earlier runtime measurements retain their original
compiler provenance and are not measurements of Go 1.27.2.

```sh
make test-quick
python3 .github/scripts/test_quick_tests.py
python3 .github/scripts/test_format_check.py
```

Go formatting checks tracked files without editing them. Windows checkout CRLF
is normalized only in temporary formatter input. Local evidence goes to
`.test-results/quick`; do not commit it. Clean/cached timing means separate
empty/reused `GOCACHE` with fresh tests in both runs, not cached test results.

## Full manual qualification

The optional `CI` workflow dispatch has a `qualification` boolean (default
false). Enabling it runs all five existing race/database shards, requires every
package/top-level model test and complete atomic coverage, and runs all shipped
frontend builds and browser acceptance. The existing `historical_control` input
continues to replay the original Realtime assertion failure independently.
Manual full evidence is archived; automatic PR coverage comments are removed
because a focused selection must not publish partial coverage as a full report.

Local full commands remain available:

```sh
make test
make test-race GOTEST_FLAGS='-count=1 -timeout 45m'
go vet ./...
make build-frontend-modern
make build-all-templates
```

For complete database qualification, provision isolated MySQL 8.4/PostgreSQL 17
development services and secondary log/workload databases, set the DSNs listed
in `lint.yml` (never use production), install FFmpeg and database clients, and
retain `ONEAPI_REQUIRE_DB_BACKENDS=1` and
`ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1`. Missing required services must fail the
full qualification rather than silently skip. Execute each shard and merge its
inventory/coverage evidence:

```sh
for shard in packages model-0 model-1 model-2 model-3; do
  python3 .github/scripts/go_test_shards.py run --shard "$shard" --output ".test-results/$shard"
done
python3 .github/scripts/go_test_shards.py merge --input .test-results --output .test-results/coverage.txt
```

Run frontend/browser acceptance on development machines with frozen Yarn
dependencies and Playwright 1.57.0 Chromium plus its system dependencies:

```sh
(cd web/modern && yarn install --frozen-lockfile && yarn test --run && yarn build && yarn check:i18n)
(cd web/modern && python3 scripts/test-mobile-list-browser.py && python3 scripts/test-table-toolbar-browser.py)
for theme in air berry; do
  (cd "web/$theme" && yarn install --frozen-lockfile && yarn test && yarn build)
  python3 .github/scripts/legacy-browser.py "$theme"
done
```

Keep feature-specific live-provider, stress, scale, benchmark and fuzz commands
in their existing runbooks. Choose the relevant manual suites for the change;
record any omitted environment qualification explicitly. Do not claim an
unexecuted full suite passed or change its assertions to satisfy the quick gate.

## Delivery boundary

`ci.yml` is unchanged: image publication and deployment remain separate from PR
validation. Checkout credential isolation, immutable image identity, serialized
rollout, and remote health/failure handling remain intact. This policy changes no
secrets, branch protection, security scanning, publishing triggers, deployment
commands, or production settings.
