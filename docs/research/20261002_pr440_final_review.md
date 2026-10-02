# PR #440 final review closure — 2026-10-02

Baseline: `114e04cf188e8bf9144423b8934aee9d8cc190ce`. The baseline's ordinary
repository CI passed in run `37066571906`. These changes append to that commit;
PR #423's contributor history and all preceding billing regressions remain.

## Reproduction-first findings

| Concern | Behavior before correction | Correction and regression |
| --- | --- | --- |
| HTTP 413 reopens paid video retries | The outer Relay function resets the retry budget after `shouldRetry` rejects replay, then selects another channel. | Keep authorization separate from the budget; capacity expansion cannot override a veto. `TestAsyncVideo413CannotReopenPaidReplay` covers all three video POST routes, both cache modes, and zero/nonzero configured budgets. |
| Rejected admission advertises a nonexistent task | Reservation assigns a UUID before the transaction; insufficient owner/token quota or a failed INSERT returns it despite rollback. | Only successful reservation results establish task identity. `TestAsyncVideoRejectedAdmissionHasNoPhantomTask` uses the shipped router, actual quote HTTP, SQL faults, repeated keys, and physical task/wallet assertions. |
| Financial update can replace an accepted upstream ID | A direct valid-lease call bypassing the evidence helper can replace identity and publish another result or refund. | Lock and compare the stored ID byte-for-byte inside the financial transaction; do not rely on case/space-insensitive SQL equality. `TestAsyncTaskSettlementCannotReplaceAcceptedIdentity` and live-backend variants cover progress, completion and refund, rejected updates and matching repeated closure. This is an internal invariant hardening, not a demonstrated public exploit. |
| Quotation origin/credentials depend on mutable request metadata | A direct adaptor call with stale or shadow metadata chooses that URL/key instead of its selected channel snapshot. | Obtain origin and key from the distributor's typed `ChannelModel`; verify channel ID, UUID and type, and reject missing/mismatched selection. `TestMuAPIPricingUsesSelectedChannelOrigin` exercises actual trusted/other HTTP servers and zero-dispatch negative controls. |

A clean worktree at the unchanged baseline ran the four new regression groups:
**28 failing individual cases and 3 passing controls**, with successful compilation.
This is not 28 independent vulnerabilities. The controls include ordinary text
HTTP 413 recovery with both zero and nonzero configured retry budgets and a valid
selected quotation channel. The same negative assertions pass after correction.

## CodeQL interpretation

The preceding scan's remaining `go/request-forgery` path starts at
`c.GetString(ctxkey.BaseURL)` in request metadata construction. The real distributor
sets that value from an administrator-controlled channel; the finding is not
proof that an ordinary user's model string can change the origin. Existing URL
validation, bounded literal model/task segments, and redirect rejection remain.

The quotation boundary now directly consumes the selected typed configuration,
keeping destination and provider credentials together. It never falls back to
request-derived metadata if the selected configuration is missing or mismatched.
Configured private HTTP(S) proxies and future valid model slugs remain supported.
No destination allowlist, warning suppression, custom CodeQL model or query
exclusion is introduced. The standard Go `security-extended` suite must run on
the exact candidate tree and produce no MuAPI request-forgery finding. Full SARIF
and the candidate SHA/tree are retained separately from ordinary test results.

## Acceptance commands

```sh
go vet ./...
go test -json -count=1 -p 2 -timeout 15m \
  ./relay/adaptor/muapi ./relay/asyncvideo ./relay/channeltype ./relay/relaymode \
  ./relay/pricing ./relay ./middleware ./controller ./relay/controller ./router
go test -json -race -count=1 -p 2 -timeout 15m \
  ./relay/adaptor/muapi ./relay/asyncvideo ./relay/channeltype ./relay/relaymode \
  ./relay/pricing ./relay ./middleware ./controller ./relay/controller ./router
go test -json -race -count=1 -timeout 12m -run 'TestAsync|TestMuAPI|TestVideoQuota' ./model
ONEAPI_REQUIRE_DB_BACKENDS=1 go test -json -race -count=1 -timeout 8m \
  -run '^TestAsyncBilling(LiveDatabases|EvidenceLiveDatabases|AckBackoffLiveDatabases|IdentityLiveDatabases)$' ./model
```

The live run must execute MySQL and PostgreSQL: 18 named leaf cases, no skipped
backend. SQLite-only runs do not qualify that requirement. The nine real SIGKILL
boundaries, socket-disconnect tests, lost-COMMIT cases and physical accounting
assertions remain part of the retained suites. Final ordinary repository CI and
CodeQL results are reported on the PR only after the actual runs complete.

No paid MuAPI work is submitted. The existing policy retains uncertain debits,
collects known supplements/debt, and never repeats possibly accepted generation.
This change does not invent provider-side exactly-once execution or an automatic
reconciliation capability for an irretrievably missing upstream receipt.
