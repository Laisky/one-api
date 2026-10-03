# Issue #395 follow-up: administrator reporting and trace details

## Scope

[The reporter's follow-up](https://github.com/Laisky/one-api/issues/395#issuecomment-5763096730)
confirms that the original root dashboard timeout is resolved. This change is
[new PR #416](https://github.com/Laisky/one-api/pull/416), based on
`a8eb4d5170db14505305b3940ca31a6024403a16`, not another aggregation rewrite.
It addresses administrator reporting and the log-details trace read path.

The feedback does not include the failed trace HTTP status/body, database
schema, or trace-retention configuration. The defects below are independently
reproduced against repository behavior; they must not be presented as proof of
the exact cause in the reporter's production deployment. A trace that was not
stored locally cannot be reconstructed by a read-path fix.

## Reproduce-first evidence

[CI run 35625386635](https://github.com/Laisky/one-api/actions/runs/35625386635)
ran the tests before production changes, at
`5d9fc1ec5b79826e4ea46855ff88802d47404e2b`. Its packages artifact is
`10652711614` (SHA-256
`50ee4720d44880d6c159387bc5aed47b34fa5e320bf3f08f85bc27131924628a`).

Real-handler tests reproduced the role=10 cross-user read and selector denials.
The root seven-day cross-user and selector controls passed. Four earlier root
count assertions had an incorrect test JSON field; those are fixture failures,
not production regressions. The decoder now uses the verified DTO field
`RequestCount`, retaining the count and ownership assertions.

Trace tests reproduced missing durations on correlation reads, authorized
retention misses returning 404, actual trace database failures also returning
404, one uncancelled database read for a cancelled log request, and an intact
trace hidden by malformed unrelated billing metadata. The negative control
also proves that an authorized direct correlation read succeeds with one query
when the billing-log dependency is deliberately unavailable.

The Modern tests exercise the production component with controlled HTTP
responses. They cover current UUID, legacy numeric, and missing-log-reference
snapshots; opaque correlation encoding; explicit error/retention states;
manual retry; response correlation validation; and obsolete-request
cancellation. They do not claim that the reporter's actual failing response
was captured. Existing component tests retain their assertions and use the
new route and real response field shape.

## Fixes and security boundary

Administrators and root users can read site-wide and selected-user dashboards
and selector options. Ordinary users remain restricted to their own data and
seven days. The configured site-wide range cap remains enforced. No account,
password, MFA, role-management, or quota-mutation permission is broadened.

Modern uses the trace correlation already returned by the authorized log list.
The server still authorizes every read; ordinary users need an owned log for
that correlation. The API returns durations and an explicit local-retention
state. Real storage failures return HTTP 500 with safe public text and retain
the detailed error only in structured server logs.

The UUID log route remains available and UUID-only. It now passes the request
context through resolution and reads only identity, ownership, correlation,
and its existing response fields. It does not decode unrelated provider
metadata. Invalid references, missing references, and storage failures retain
distinct 400/404/500 responses using the repository's actual resolver sentinels.

The UI validates the success envelope and matching correlation, normalizes
nullable timestamps, aborts obsolete requests, and rejects late results.
Refresh is explicit: no automatic polling, result sharing between viewers,
process-wide trace flush, or additional background work is introduced.

## Deterministic acceptance measurements

`TestTrace395ReadQueryBudget` records actual GORM query/row operations for ten
paired reads per role. It checks identical correlation, timestamps and
durations, with the UUID route as the control and the Modern correlation route
as the candidate. Its required budgets are:

| Viewer | UUID route queries/read | Correlation route queries/read |
| --- | ---: | ---: |
| Root | 3 | 1 |
| Administrator | 3 | 1 |
| Ordinary owner | 3 | 2 |

The ordinary-owner path deliberately retains its ownership query. These are
query-count assertions on an isolated real SQLite fixture, not production
latency, P95, throughput, or MySQL/PostgreSQL performance claims. Every sample
is logged as `TRACE395_QUERY_BUDGET` in the existing CI test artifact. Final
observed results and the passing head/run are recorded in the PR description.

Other required behavior: a cancelled request starts zero uncancelled log
queries; an in-flight projection observes cancellation; unauthorized owners
remain hidden with 404; known retention absence is not a server error; a real
database failure is never replaced by a successful empty trace; and all trace
reads leave the process-wide writer flush count at zero.

## Source organization and verification

To follow the repository's file-size guidance, the oversized user controller
is split into authentication, self-service, administrator mutations, MFA, and
reporting files. Go AST comparison of the original and split declarations
found 41 declarations on each side: 39 unchanged and only `GetUserDashboard`
and `GetDashboardUsers` changed. Imports are regenerated per file; no unrelated
function body changes are included. The split is also exercised by the full
repository tests, not accepted on the AST comparison alone.

No CI workflow, dependency, database migration, cache TTL, or timeout is added
or changed. A temporary public-source diagnostic used by the offline editor
was removed in the test-only history and is absent from the delivered diff.

## Reproduction commands

Use the Go toolchain required by the repository and the existing frontend lockfile:

```sh
go test ./controller -race -count=1 \
  -run '^(TestDashboard395|TestTrace395|TestTraceEndpoints)'
cd web/modern
yarn test --run src/components/__tests__/LogDetailsModal.issue395.test.tsx \
  src/components/__tests__/LogDetailsModal.test.tsx
yarn build
```

The full existing CI includes repository-wide Go race/coverage shards, static
checks, and Modern tests/build. Native CI results, rather than a locally
downgraded Go module, are the acceptance evidence. Deployment verification
still needs the affected user to inspect a retained trace and an administrator
to select another user; no production deployment is performed by this PR.
