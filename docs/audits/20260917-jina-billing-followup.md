# Jina billing audit: interrupted receipts and balance conservation

Date: 2026-09-17. Baseline: `1f88cfac1ae70060b846b5da27bd3feb4d70a646` on PR #407.
This follows [the main billing audit](20260917-jina-billing.md); its outstanding
system-wide limitations still apply. This is not an all-provider certification.

## Additional findings and fixes

| Finding | Underbilling mechanism | Correction |
| --- | --- | --- |
| Positive OCR usage was accepted before stream completion | An intermediate cumulative receipt could release most of a reservation after a disconnect | Require final upstream usage and upstream `[DONE]`; HTTP EOF or downstream completion is insufficient |
| OCR read errors were not part of receipt validity | Complete-looking JSON or an earlier SSE receipt could survive an interrupted transport as authoritative | Record read failures, distinguish EOF from Close, and retain a labelled conservative allowance |
| More output after an earlier receipt was not tracked | An early receipt did not cover later paid output | New choices invalidate receipt finality until another complete usage snapshot arrives; error events remain unverified |
| Search read failures discarded partial receipt evidence | A larger already-observed charge could fall back to a smaller reservation | Recover complete counters from interrupted JSON and merge maxima into conservative usage |
| Missing OCR token attribution assumed output was expensive | Input-heavy channel pricing overrides could make that assumption undercharge | Reserve unassigned total-token evidence at both possible rate directions; estimates remain explicitly labelled |
| Ambiguous aggregate counters could wrap negative | Summing large positive counters could look like no usage | Saturate the aggregate while retaining the individual counters for checked monetary calculation |

Normal completed receipts retain exact token pricing. Repeated cumulative usage is
not summed. These changes neither retry paid inference nor introduce a second
charge on top of the reservation: settlement remains the final total minus the
amount already deducted. Missing/zero/invalid usage for an admitted nonempty
request is not proof of free upstream work. Only explicitly configured free
pricing or free groups remain free.

## Added behavioral coverage

`relay/adaptor/jina/receipt_terminal_test.go` checks fragmented reads, bytes plus
error in one Read, EOF without DONE, cancellation, deadlines, errors after usage,
output after usage, data after DONE, early Close, truncated usage objects,
duplicate-counter maxima, and both input-expensive/output-expensive overrides.

`relay/controller/jina_billing_interrupt_test.go` drives actual relay helpers and
SQLite accounting through Chat Completions, Responses fallback, Claude Messages,
embeddings and rerank. It compares user balance, finite-token balance and usage,
consume logs and request-cost rows. Cases include a complete-response control,
retained holds, charges above the hold, and debt when known/estimated consumed
work exceeds remaining funds. Transport call counts assert no paid replay.

`model/token_billing_conservation_test.go` tests simultaneous reservations by
multiple tokens sharing one user, including an unlimited token, plus deterministic
200-request state-machine sequences across finite/unlimited tokens and aggregate
batching on/off. Every step is checked against an independent integer ledger.

## Qualification blockers found in the baseline CI

Baseline CI run `35280802626` failed two model shards. The billing debt test's
second fixture reused an empty unique `users.access_token`; fixtures now generate
unique bounded identities for both access_token and aff_code. A separate race was
caused by `setupTestDatabase` leaving the UUID catch-up worker running into another
test's configuration changes. Ordinary model fixtures now join that worker; no
production migration policy or race detector is disabled.

Baseline Go vet, package shard, other model shards and Modern/Air checks passed,
but those results do not qualify this follow-up commit. The existing PR workflow
must run against the new head. No CI workflows or dependencies are added.

Local qualification is limited to Go formatting/syntax and an isolated replay of
the actual receipt-reader/evidence logic, with `go test -race -count=20`. That replay
passed, including concurrent snapshots. It does not execute the complete project,
Gin handlers or SQL tests: local Go is 1.23.2 and the project requires Go 1.27.1.
Full results must be taken from CI, not inferred from the local replay.

## Remaining operational limits

The earlier audit's lack of a durable cross-database settlement ledger/outbox is
not fixed here. Process death or an ambiguous database commit still requires
reconciliation; do not promise exactly-once charging or assume log success is a
substitute for physical balances. Non-Jina provider-specific admission/usage and
pricing paths are not globally certified by these Jina tests. Published Jina
pricing has not been reconciled against a paid account invoice. Unbounded media
inputs remain rejected before inference rather than forwarded at a cheap guess.
