# Jina integration and shared billing audit

Date: 2026-09-17. Scope: PR #407, the Jina request/receipt pipeline, the shared
reservation and final-debit functions, refund/retry boundaries, and representative
Chat, Responses and Messages settlement paths. This is a code/behavior audit, not
a reconciliation against a paid Jina invoice or a certification of all providers.

## Required accounting contract

A paid upstream request must have a durable reservation before dispatch. A valid
receipt settles at the configured price, with one upward rounding to a quota
unit. Missing evidence must not be converted into zero usage. An uncertain paid
attempt retains a labelled estimate and is not automatically replayed. Confirmed
pre-inference rejection can refund, but only a committed refund permits replay.
Known work beyond a remaining balance records debt; admission of new work still
requires available funds. An intentionally free group or model is not an error.

The balance equation is `final debit = reservation + incremental debit + final
adjustment`. The log and request-cost record must describe that same amount.
Amounts estimated because of incomplete receipts are not presented as measured.

## Findings and changes

| Finding | Change | Behavioral evidence added |
| --- | --- | --- |
| A trusted Jina user could skip reservation, then receive a missing-usage error and never be charged | Every positive Jina budget is reserved synchronously, including unlimited tokens | Actual balances read inside the fake upstream, followed by final user/token/log/request-cost assertions |
| Structured embedding input was invisible to the string-only token estimator | A model-aware text/image admission budget replaces that estimator for Jina | Text/image/structured input tests; opaque PDF/audio/video/grouped work rejected before dispatch |
| Cheap text-only OCR estimates could replace missing visual-token receipts | Observe raw JSON and fragmented SSE receipts independently of shared chat conversion | Three client API formats, streaming/non-streaming, measured/missing receipts |
| A malformed result envelope could erase otherwise valid paid usage | Parse and retain usage before checking data/results | Invalid envelope still settles its valid receipt and reports an error |
| Duplicate counters, zero, missing and malformed usage could yield undercharges | Strict receipt decoding; maxima of partial evidence can only increase the labelled fallback | Duplicate root/usage keys, negative/null/fractional/overflow counters, cumulative stream snapshots |
| Pre-consumption wrote token and user separately | One guarded transaction updates both, synchronously even when batching is enabled | Rollback, cancellation, missing rows, concurrent admissions and balance conservation |
| The final debit could fail when concurrent work exhausted funds | A distinct consumed-work settlement transaction allows debt, but admission remains guarded | Finite/unlimited token debt; subsequent request rejection; owner mismatch and integer-overflow rollback |
| Core billing published completed logs even after the balance debit failed | Return on debit failure; preserve provisional evidence instead of publishing success | Transaction failure tests and explicit residual-risk review below |
| Refund logs could be zeroed before a failed refund | Void/reconcile only after successful physical refund | Confirmed Jina rejection refunds exactly once; failed refund blocks replay |
| Cross-channel retry could replay a possibly charged Jina attempt | Block retries after possible paid dispatch or a response queued for settlement | Cancellation/retained-hold and safe rejection/reset tests |
| Zero ordinary token counts discarded tool/cache-only charges | Shared billable-usage predicate includes tool, cache and modality buckets | Cross-API tool-only, cache-read-only and cache-write-only cases |
| Binary floating-point arithmetic could cross rounding boundaries | Jina flat prices use decimal rationals and a single final ceiling | Exact decimal examples, invalid rates, extreme positive rates and seeded fuzz invariants |

The scope deliberately distinguishes these changes from a complete rewrite of
all provider accounting. Shared synchronous balance updates increase database
work when batch mode was previously used; this is an intentional durability
tradeoff. Nonfinancial aggregate batching is unchanged.

## Jina admission and estimation policy

Only researched model IDs and flat token pricing are admitted. Text allowances
include model context plus a byte-based margin; supported image inputs use the
model context, never the length of a remote URL. Rerank includes every document
and repeats query work per document; `top_n` does not reduce the budget. OCR has
an explicit upstream completion limit (default/maximum 8,192).

PDF, audio, video and grouped inputs have no validated billing bound in this
integration and are rejected before inference. The gateway catalog advertises
only its admitted text/image modalities, not every capability of the upstream
model. Tiered or per-call Jina price overrides are rejected until their admission
contracts are implemented. These are intentional compatibility restrictions.

For an admitted nonempty request, all-zero usage is uncertain, not proof of a
free request. Missing/invalid receipts settle at least the reservation. Higher
observed counters can increase the estimate. Metadata includes `billing_estimated`,
`estimated_charge` and `billing_estimate_reason`. Normal positive receipts settle
actual usage and refund unused reservation. Estimated charges need operational
review and reconciliation; the allowance is a conservative policy estimate, not
a mathematical upper bound on any arbitrary future provider invoice.

Upstream 401/403/404/422/429 admission rejections are eligible for a synchronous
refund. A generic 400, 5xx, timeout or broken stream is not treated as proof of
no cost. No automatic retry is made for an uncertain potentially paid Jina call.
A client-visible failure can therefore have a retained estimated charge. Publish
this behavior in the service's billing terms and expose the estimate marker to
operators and customers; do not represent it as exact provider usage.

## Validation and reproducibility

New tests are in `model/token_billing_audit_test.go`,
`relay/adaptor/jina/billing_test.go`, and
`relay/controller/jina_billing_test.go`, with regression cases in existing
receipt, catalog and shared behavior suites. The HTTP tests use local `httptest`
servers and the repository's real database fixtures, not paid provider calls.
They compare actual user/token balances, token used quota, consume logs and
request-cost records after detached billing work drains.

Run the normal repository CI (`go vet`, race-enabled test shards, and frontend
qualification). For repeated focused qualification, use the repository-required
Go version and database fixture setup, then run:

```sh
go test -race ./relay/adaptor/jina -count=10
go test -race ./relay/controller -run 'TestJinaBilling|Test.*BillingBehavior' -count=3
go test -race ./model -run '^TestBillingAudit' -count=3
go test ./relay/adaptor/jina -run '^$' -fuzz '^FuzzBillingBudgetQuota$' -fuzztime=30s
```

Do not treat adding a test as executing it. The PR conversation records the exact
commit and actual CI outcomes; local syntax/gofmt checks do not substitute for
compilation, database behavior or race detection. A passing old commit is not
proof of a new commit. No paid live Jina requests are part of this audit.

Two failures in the original PR CI are addressed separately: a stale Z.AI test
pinned the channel-count sentinel before Jina was appended, and a cache invalid-ID
test reached an uninitialized database instead of rejecting its invalid ID.

## Residual risks and release boundary

**Do not claim a system-wide guarantee of no underbilling from this patch.**

1. Non-Jina providers retain their existing trusted-reservation bypass and some
   uncertain-retry policies. They require provider-specific conservative budgets
   and equivalent end-to-end tests before claiming the Jina guarantee globally.
2. Balance writes are atomic, but balances, logs, request-cost records and
   aggregates are not one durable cross-database receipt transaction. A process
   crash, database outage or ambiguous commit can leave incomplete accounting.
   An idempotent settlement ledger/outbox and reconciliation worker are still
   needed for crash-recoverable, exactly-once cross-store finalization. The core
   now avoids publishing a completed log after a known failed debit; it does not
   make caller-owned request-cost writes transactional with that debit.
3. General non-Jina quota computation still uses the existing floating-point and
   counter arithmetic. The exact-decimal Jina calculation does not establish
   correctness for arbitrary other-provider extreme rates, modality weights,
   cached bucket semantics or all possible overflow inputs.
4. The pricing catalog and context limits are researched defaults. Provider
   contract changes, negotiated/prepaid pricing and unexpected higher bills need
   reconciliation against authoritative provider records. No invoice was checked.
5. Synchronous durable quota writes trade throughput for correctness. Load-test
   the deployment database before enabling broad high-volume rollout. Tests of
   SQLite behavior alone do not prove MySQL/PostgreSQL isolation under failure.

Release only after the changed commit passes its required checks. Do not enable
opaque Jina modalities, relax reservations or re-enable ambiguous automatic
retries merely to make a demo succeed. Track the residual system-wide items as
separate audited work rather than silently treating them as complete.

## Primary references

- https://api.jina.ai/openapi.json (schema `2026.09.17.0130`, inspected 2026-09-17)
- https://api.jina.ai/v1/models (model IDs and published token prices)
- https://jina.ai/models/ (model catalog; not by itself an inference contract)
