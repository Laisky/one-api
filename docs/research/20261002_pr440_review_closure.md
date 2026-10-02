# PR #440 review hardening — 2026-10-02

Baseline: `e33616889fce2e0116f8df1372de02887459f77e`. Contributor history and the
previous billing, real-socket and SIGKILL regressions remain unchanged.

## Confirmed acknowledgement/backoff defect

The initialized GORM query was reused after the acknowledgement update failed.
Its retained error prevented the subsequent backoff SQL from running. The old
regression asserted only that a second item progressed, not that the failing
item actually backed off. A full batch of such failures could keep occupying
all scan slots and delay later financial records, without reversing the primary
database wallet debit.

Each acknowledgement/backoff now gets a new query with the same immutable
ID/revision/state/quota fence. Tests assert physical retry count and deadline,
32 failing acknowledgements followed by a healthy item, successful recovery,
one log and one request-cost row per task, and unchanged user/token debits.
The same batch test runs on SQLite and disposable MySQL/PostgreSQL databases.

## URL boundary hardening

The old identifier validator admitted `.` and `..`. They can change the intended
path when an upstream proxy canonicalizes the request. String concatenation also
accepted administrator bases containing queries, fragments, userinfo, or relative
and encoded paths. These are distinct from proving an arbitrary end user could
choose a different host: provider origins are administrator-controlled.

A shared typed URL constructor now supplies scheme and authority exclusively
from a validated configured base. Regex-validated bounded identifiers are added
only as literal path segments; dot traversal is explicitly forbidden. All quote,
submit, poll and legacy adaptor URL construction goes through that boundary.
No compiled model catalog allowlist, global host restriction, warning suppression
or CodeQL exclusion is added. Configured internal proxies remain supported.
Validation errors do not echo potentially credential-bearing configuration.

All network tests use local HTTP servers. They verify zero outgoing requests
for invalid segments/bases and actual paths/headers for accepted versioned model
IDs, API suffixes and proxy prefixes. Redirect and paid-receipt regressions stay
in place. A legacy request URI with a query is an intentional positive control:
it is not an opaque identifier, and stripping that query remains compatible.

## Reproduction-first and acceptance commands

New assertions were run before modifying production code on the baseline. The
corrected negative control has **57 failing individual behavior cases** (excluding
three parent test summaries), plus 48 passing controls. These are assertion
failures, not compilation/environment failures or 57 independent vulnerabilities.
The initially overstrict legacy-query assertion was corrected and rerun before
fixing production code. Corresponding repaired assertions pass.

```sh
go test -json -count=1 -p 2 \
  -run 'TestAsyncBilling(PoisonOutbox|AckBatch)|TestMuAPI(URL|BaseURL)' \
  ./model ./relay/adaptor/muapi

go vet ./...
go test -json -race -count=1 -p 2 -timeout 12m \
  -run 'TestAsync|TestMuAPI|TestVideoQuota|TestEstimateVideoPricing|TestGetRequestURL' \
  ./model ./relay/adaptor/muapi ./relay/asyncvideo ./relay/channeltype \
  ./relay/relaymode ./relay ./middleware ./controller ./relay/controller ./router

go test -json -race -count=1 -p 2 -timeout 15m \
  ./relay/adaptor/muapi ./relay/asyncvideo ./relay/channeltype ./relay/relaymode \
  ./relay/pricing ./relay ./middleware ./controller ./relay/controller ./router

ONEAPI_REQUIRE_DB_BACKENDS=1 go test -json -race -count=1 -timeout 8m \
  -run '^TestAsyncBilling(LiveDatabases|EvidenceLiveDatabases|AckBackoffLiveDatabases)$' ./model
```

Live acceptance must execute both MySQL and PostgreSQL; missing DSNs/skips are
not qualification. The local full/focused runs and the remote CodeQL and live
backend runs are reported separately in the PR acceptance comment. CodeQL is
run with its standard Go security-extended suite on baseline/candidate source,
with uploads disabled so this check cannot overwrite repository alert state.
The ordinary full-repository CI must also qualify the final published head.

No paid provider job is submitted. Earlier billing ambiguity and database
durability limitations continue to apply; this change does not release holds,
forgive debt, resubmit paid work, or alter provider-side exactly-once semantics.
