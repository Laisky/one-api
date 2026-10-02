# Async video billing fault matrix — 2026-10-02

## Contract and scope

This follow-up extends PR #440 from `5962c81e531e0cb6a27c39be3173519917490492`.
It preserves contributor PR #423's history. Tests use local HTTP fixtures only;
no paid MuAPI generation is submitted. The synchronous bridge and explicit task
API use the same durable task ledger, not request-scoped accounting.

**Conservative financial policy:** reserve the entire quote before dispatch.
A disconnect, restart, HTTP error, malformed output, missing receipt, failed poll
or expired lease must never turn paid/possibly-paid work into a zero-cost failure.
For dynamic pricing, collect the greater of the quoted debit and each known actual
provider charge using the original exact conversion; round fractional quota up.
Known additional cost becomes debt when the balance is exhausted. Fixed tariffs
explicitly configured by an administrator remain authoritative. There is no
intentional duplicate charge to compensate for faulty bookkeeping.

Wallet movement, task outcome and financial revision commit atomically. Completing
an already-reserved job does not charge its original amount twice. Only provably
undispatched work or an authoritative matching failed/cancelled refund receipt
can release money. Local debit/refund/log delivery is repeat-safe; exactly-once
provider execution is not claimed.

## Reproduction-first findings

The following behavioral assertions were executed against the unmodified parent
implementation and failed before fixes. Compilation/environment failures are not
counted as reproduced bugs.

| Finding | Original failing behavior test | Correction |
| --- | --- | --- |
| Dispatch 4xx treated as safe rejection | `TestAsyncBillingHTTPErrorCannotAuthorizeRefund` | No post-dispatch status alone releases the hold. |
| Accepted ID dropped on non-2xx | `TestAsyncBillingReceiptSurvivesHTTPError` | Preserve task identity independently of HTTP status. |
| Anonymous refund trusted | `TestAsyncBillingRefundNeedsMatchingReceipt` | Require matching task identity for refund evidence. |
| Shutdown cancels receipt/completion storage | `TestAsyncBillingShutdownPreservesReceipt` | Bounded detached write within the joined worker. |
| Actual cost above quote never collected | `TestAsyncBillingActualProviderChargeCannotExceedDebit` | Immutable pricing snapshot, monotonic supplement/debt before success. |
| Unresolved paid work absent from cost/log surface | `TestAsyncBillingHeldTaskHasDurableCost` | Versioned outbox includes already-debited held work. |
| One poison item blocks later receipts | `TestAsyncBillingPoisonOutboxDoesNotBlockLaterReceipts` | Per-item errors/backoff and continuation. |
| Settlement trusts mutable worker money fields | `TestAsyncBillingSettlementUsesPersistedFinancialSnapshot` | Re-read the locked authoritative financial row. This is internal robustness, not a demonstrated external exploit. |

## Behavioral scenario matrix

Each row names executable behavior tests. Financial assertions read physical
user/token/channel rows and request-cost/log records, not just HTTP status or
mocked calls. The prior `TestAsyncTask*` / `TestAsyncVideo*` regressions remain.

| Scenario | Test / subcases | Expected accounting |
| --- | --- | --- |
| Quote invalid or no funds | `TestMuAPIVideoLifecycleFailsClosedBeforeSubmission` | No task, no debit, no generation. |
| Database reservation fails or caller cancels admission | Existing `TestAsyncTask*` admission controls | No unpaid dispatch; a committed receipt survives ambiguous acknowledgement. |
| COMMIT succeeds but driver reports failure | `TestAsyncBillingLostCommitAcknowledgements`: reservation, settlement, refund, log | Re-read committed truth; no second debit/refund/log. |
| 32 different requests compete for five funded jobs | `TestAsyncBillingConcurrentDifferentRequestsCannotOverspend` | Exactly five admissions; balance cannot overspend at admission. |
| Identical keys concurrently admitted | Existing idempotency regressions and `TestAsyncBillingLiveDatabases` | One task and one reservation. |
| Valid lease used concurrently for completion/refund | `TestAsyncBillingConcurrentSettlementAndRefundHasSingleWinner` | One financial winner; stale attempts rejected. |
| Every task state, finite/unlimited tokens | `TestAsyncBillingStateMatrix`: 16 subcases | Held/unknown/failed work remains debited; completed usage increments once; confirmed refund once. Unlimited token policy never makes owner work free. |
| Full client TCP disconnect | `TestAsyncBillingRealSocketDisconnect`: before dispatch, accepted, active poll | HTTP waiter ends; independent worker keeps receipt and collects final charge. |
| Actual network wait timeout | Same test: `wait_timeout` | HTTP 504 does not refund; identical-key replay retrieves the paid result. |
| Gateway process killed with durable reserved task | `TestAsyncBillingSIGKILLRecovery/reserved` | Replacement worker submits once using prepaid work. |
| Process killed during paid POST | `.../during_submission` | Explicit unknown outcome, retained debit, zero automatic repeat submissions. |
| Process killed after receipt is stored | `.../accepted` | Restart uses GET; no second POST or original debit. |
| Process killed during provider GET | `.../during_poll` | Safe GET resumes, full final accounting. |
| Kill before settlement SQL commits | `.../before_settlement_commit` | Transaction rollback; retry completes accounting exactly once. |
| Kill between owner and token supplemental debit | `.../during_additional_debit` | Both rollback; recovery collects supplement once. |
| Kill after settlement commit | `.../settled` | No repeated accounting; durable result remains. |
| Kill after log commit, before primary ack | `.../outbox_before_ack` | Separate log DB receipt prevents duplicate consumption logs. |
| Graceful operation cancellation after upstream success | `TestAsyncBillingShutdownPreservesReceipt`: submit and completion | Persist known evidence despite service I/O cancellation. |
| Actual USD higher/lower/equal/tiny/zero/missing; fixed tariffs; group markup | `TestAsyncBillingCostAdjustmentsAreMonotonic` | Conservative captured tariff, whole-unit ceiling, no downward repricing without refund. |
| Known higher submit cost without usable ID | `TestAsyncBillingUnknownSubmissionStillCollectsKnownCost` | Debit actual known amount; unknown task is not resubmitted. |
| Completed output malformed, wrong type, bad URL, unknown status, conflicting refund | `TestAsyncBillingMalformedOutputRetainsObservedCost` | Preserve known upward cost evidence; no result or unauthorized refund. |
| Invalid numeric format, huge exponent, overflow | `TestAsyncBillingNumericBounds` | Fail closed; no silent zero charge or unbounded numeric allocation. |
| SQL outage during supplemental collection | `TestAsyncBillingSupplementFailureWithholdsResult` | No success exposed until atomic supplement commits after recovery. |
| SQL fault at task/user/token/channel update | `TestAsyncBillingSettlementRollbackMatrix` | Financial transaction fully rolls back; replay closes once. |
| Usage counter overflow | `TestAsyncBillingChannelCounterOverflowDoesNotHideCost` | Debit retained and success withheld; no silently dropped usage. |
| Temporary provider-route DB lookup failure | `TestAsyncBillingTemporaryResolverFailureResumesPolling` | Backoff then resume original GET; no permanent early stranding, refund or failover. |
| Deleted/reused token/owner identity | Existing `TestAsyncTaskTokenDeletionDoesNotStrandPrepaidJob` and identity controls | Original owner charged; replacement identity never credited. |
| Finite/unlimited mode edited after admission | `TestAsyncBillingLiveDatabases` plus original policy controls | Persisted original token mode determines reservation accounting. |
| Bad request-cost/log/ack row | `TestAsyncBillingPoisonOutboxDoesNotBlockLaterReceipts` | Later rows progress; bad record stays retryable. |
| Entire first outbox batch is poison | `TestAsyncBillingWholePoisonBatchBacksOff` | Independent backoff frees subsequent scan slots. |
| Old held log delivered after final charge/refund | `TestAsyncBillingStaleOutboxCannotOverwriteFinalChargeOrRefund` | Final revision wins in both physical databases. |
| Retention / lost notifications / disabled channel / spent token replay | Prior async task/router suites | Never prune unresolved debits or use retrieval to create unpaid work. |
| Real MySQL and PostgreSQL, separate log DB | `TestAsyncBillingLiveDatabases` | Concurrent admission, supplemental debt, token policy, stale outbox and competing refunds use actual backend transactions. Missing DSNs fail when `ONEAPI_REQUIRE_DB_BACKENDS=1`. |

## Running and interpreting the evidence

```sh
go vet ./...
go test -count=1 -p 2 -timeout 15m \
  ./relay/adaptor/muapi ./relay/asyncvideo ./relay/channeltype \
  ./relay/relaymode ./relay/pricing ./relay ./middleware \
  ./controller ./relay/controller ./router

go test -json -race -count=1 -p 2 -timeout 12m \
  -run 'TestAsync|TestMuAPI|TestVideoQuota' \
  ./model ./relay/asyncvideo ./relay/adaptor/muapi ./relay/channeltype \
  ./relay/relaymode ./relay ./middleware ./controller ./relay/controller ./router

# Requires test-only credentials with CREATE/DROP DATABASE privileges.
# Every live subcase creates unique disposable primary and log databases.
ONEAPI_REQUIRE_DB_BACKENDS=1 go test -race -count=1 -timeout 8m \
  -run '^TestAsyncBillingLiveDatabases$' ./model
```

Local SQLite runs cannot qualify the live backend subcases: they explicitly skip
without DSNs. Remote validation must assert passing named MySQL **and** PostgreSQL
subtests. A SIGKILL child entry point skips in the parent process and is executed
by the actual crash matrix; that is not a missing restart scenario. Normal PR CI
remains responsible for the unrelated full-repository / frontend matrix.

The SQLite fault tests intentionally shorten persisted lease deadlines after an
OS kill rather than sleeping a minute. They do not fabricate provider results,
financial outcomes, balances, or refunds. HTTP fixtures use only local servers.

## Operational limits

Database commit durability, backups and eventual service recovery remain required.
An upstream acknowledgement irretrievably lost before its ID is persisted can
leave a task in `submission_unknown`; the already-debited quote/known cost remains
visible and cannot be refunded automatically. The gateway cannot discover an
unobserved higher provider cost without upstream lookup/reconciliation support.
No finite test matrix establishes correctness for arbitrary provider behavior,
catastrophic database loss, manual ledger edits, or every possible interleaving.
These cases require operator reconciliation, not an invented free failure or
unsafe replacement generation. An operator reconciliation UI is not added here.

Provider cost/refund semantics: [MuAPI API reference](https://muapi.ai/zh/docs/api-reference)
and [MuAPI pricing](https://muapi.ai/fr/docs/pricing).
