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
| Responses fallback left the reservation safety net active during final settlement | The deferred hold settlement could overwrite the final request cost and duplicate accounting side effects | Hand the reservation to final settlement synchronously on both fallback branches, before spawning writes |
| Shared streaming handlers can return without closing an erroneous response | A failed relay could leave the upstream response transport open | Close the OCR observer on every normal return path, with idempotent transport closure and persistent close-error evidence |
| Chat's generic streaming tracker could reject already-consumed Jina work when its final fee exceeded available funds | Finalize used an admission check and returned before debt-capable settlement, leaving only the smaller reservation charged | Fully reserved Jina requests bypass incremental tracking and settle the raw receipt or conservative allowance directly; other providers' tracking is unchanged |

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
`receipt_close_test.go` verifies exactly one physical close under concurrent
callers, including consistent propagation of an underlying close failure.

`relay/controller/jina_billing_interrupt_test.go` drives actual relay helpers and
SQLite accounting through Chat Completions, Responses fallback, Claude Messages,
embeddings and rerank. It compares user balance, finite-token balance and usage,
consume logs and request-cost rows. Cases include a complete-response control,
retained holds, charges above the hold, and debt when known/estimated consumed
work exceeds remaining funds. Transport call counts assert no paid replay; close
counts assert cleanup even after early error returns. Explicit lifecycle draining
precedes assertions and fixture cleanup, including an assertion failure.
`jina_billing_debt_test.go` additionally requires complete positive streaming
receipts to settle beyond available funds for finite/unlimited tokens and both
published/channel-override prices, without turning the paid request into another
admission failure.

`model/token_billing_conservation_test.go` tests simultaneous reservations by
multiple tokens sharing one user, including an unlimited token, plus deterministic
200-request state-machine sequences across finite/unlimited tokens and aggregate
batching on/off. Every step is checked against an independent integer ledger.

## Qualification findings

Baseline CI run `35280802626` failed two model shards. The billing debt test's
second fixture reused an empty unique `users.access_token`; fixtures now generate
unique bounded identities for both access_token and aff_code. A separate race was
caused by `setupTestDatabase` leaving the UUID catch-up worker running into another
test's configuration changes. Ordinary model fixtures now join that worker; no
production migration policy or race detector is disabled.

CI run `35284249868` executed the new behavioral tests. All four model shards
passed, including the 800-step aggregate state-machine coverage, concurrent
multi-token reservation, and finite/unlimited debt settlement. The package shard
caught the Responses fallback cost overwrite (streaming and non-streaming) and an
interrupted OCR debt-ledger assertion. All original charge expectations remain.
The subsequent run `35285641809` confirmed the Responses correction but still
failed the Chat debt case even after explicit lifecycle draining. Inspection
identified the generic quota tracker's post-inference admission failure as the
cause; it was not merely a test waiting problem. The final change separates Jina's
fully reserved exact settlement from that incremental tracker.

The PR conversation records the exact final head and current CI status. The
existing workflow must qualify the final code; no CI workflows or dependencies
are added. Do not substitute an earlier passing shard for a later commit.

Local qualification is limited to Go formatting/syntax and an isolated replay of
the actual receipt-reader/evidence logic, with `go test -race -count=20`. That replay
passed, including concurrent snapshots and idempotent closure. Removing finality
validation made its negative control fail as expected. This does not execute the
complete project, Gin handlers or SQL tests: local Go is 1.23.2 and the project
requires Go 1.27.1. Full results must be taken from CI, not inferred from the replay.

## Remaining operational limits

The earlier audit's lack of a durable cross-database settlement ledger/outbox is
not fixed here. Process death or an ambiguous database commit still requires
reconciliation; do not promise exactly-once charging or assume log success is a
substitute for physical balances. Non-Jina provider-specific admission/usage and
pricing paths are not globally certified by these Jina tests. Published Jina
pricing has not been reconciled against a paid account invoice. Unbounded media
inputs remain rejected before inference rather than forwarded at a cheap guess.
