# Async video lifecycle validation — 2026-10-02

## Scope and provenance

This work extends contributor PR #423 without rewriting its commits. The new
branch starts directly at `refs/pull/423/head`; production fixes and behavior
regressions are added afterwards. The original PR remains open.

## Red controls

Before implementation, GitHub Actions run
<https://github.com/Laisky/one-api/actions/runs/37029147259> compiled and executed
both behavioral controls, and both failed for the expected reasons:

- `TestAsyncVideoSubmissionRetryBoundary`: the new `/v1/async/videos` POST was
  incorrectly replayable after it could have reached the provider.
- `TestMuAPINativeVideoCapabilityIsIsolated`: native MuAPI advertised the legacy
  video capability.

An isolated original-source checkout also failed `TestCatalogReviewInventory`
because MuAPI had no disposition in the provider catalog review inventory. A
catalog row was added; the complete relay package now passes.

During implementation, new behavioral red controls found two further problems:
`TestAsyncTaskTokenDeletionDoesNotStrandPrepaidJob` showed that requiring the
original token row at terminal settlement stranded prepaid work, and
`TestAsyncTaskPayloadColumnCapacity` showed that a MySQL TEXT column did not
represent the gateway's 1 MiB input limit. The former now settles the original
owner and fences replacement token identities; the latter uses dialect-generated
capacity-aware string columns. PostgreSQL TEXT already had sufficient capacity;
the PostgreSQL assertion checks the new bounded schema, not a prior capacity bug.

## Local checks completed

Using the repository's actual Go 1.27.1 toolchain, with public tokenizer fixtures
cached locally (no provider generation):

```sh
go vet ./...

go test -count=1 -p 2 -timeout 15m \
  ./relay/adaptor/muapi ./relay/asyncvideo ./relay/channeltype \
  ./relay/relaymode ./relay/pricing ./relay ./middleware \
  ./controller ./relay/controller ./router

go test -race -count=1 -p 2 -timeout 12m \
  -run 'TestAsync|TestMuAPI|TestVideoQuota' \
  ./model ./relay/asyncvideo ./relay/adaptor/muapi ./relay/channeltype \
  ./relay/relaymode ./relay ./middleware ./controller \
  ./relay/controller ./router

go test -race -count=1 -run '^TestAsyncTask' ./model
```

All commands passed. The last command includes the token-removal, owner/UUID
reuse, storage-capacity, and split-database receipt regressions.

An earlier offline complete-package attempt timed out while loading public
network-fetched tokenizer encodings in an unrelated Claude billing test. The
same full command passed after caching those encodings; no product code or test
was weakened to hide that environment failure.

The shipped-router test uses actual token authentication, endpoint selection,
quota transactions, worker startup, HTTP submit/poll fixtures, and synchronous
response delivery. It proves that `/v1/videos/generations` serves both a real
synchronous fixture and MuAPI's asynchronous fixture, while `/v1/videos` does not
select MuAPI. It also exercises exhausted-token retrieval/reattachment and
explicitly disabled-token denial.

## Remaining qualification boundaries

Local task persistence tests use real SQLite databases, including close/reopen
and separate primary/log databases. MySQL/PostgreSQL column tests exercise the
actual dialect DDL generators without a live server. Live backend integration,
full-repository race coverage, and the Modern production build remain subject
to the normal PR CI results; local results must not be described as those checks.

All upstream traffic in behavioral tests is served by local fixtures. No paid
MuAPI generation was submitted. Provider-side idempotency and operator recovery
of an ambiguous upstream submission are not claimed to be fully automated.
