# Tier-priced native Responses continuations

A native `previous_response_id` can cause a provider to process inherited context
that is not present in the new request body. Charging only the incremental input
before dispatch can authorize work beyond the owner's or finite token's balance.
Final settlement still records that cost, but cannot undo the upstream exposure.

## Admission policy

For active tier contracts in a nonzero billing group, a native continuation must
have an owner-scoped, terminal parent record whose channel, API type, actual model
and upstream response handle match the prepared request. Both saved parent input
and output counters must be positive and representable. Admission adds parent
input, parent output and prepared incremental input with checked integer
arithmetic, then uses the existing shared tier estimator and current output cap.
The parent input counter already includes its inherited chain; no ancestor walk,
transcript replay or extra provider call is introduced.

Unknown or incompatible parent context now returns HTTP 400 through
`invalid_response_tier_quote`, before a new quota reservation or provider call.
The message explains that inherited context cannot be safely priced. Start a new
self-contained request without `previous_response_id`, supplying the full input
needed for the application. Resending only the incremental message would lose
conversation context. The gateway does not silently change native affinity or
replay an incomplete transcript on another API.

Operational parent-store read failures retain HTTP 503 and can be retried. Missing
or expired records, missing bindings, nonterminal parents, mismatched pricing
identities, absent/malformed/negative/unrepresentable counters and ambiguous zero
counters do not fall back to incremental-only quoting.

## Compatibility boundary

Native typed decoding and persistence do not retain original wire-field presence.
An omitted usage counter can therefore become a saved zero. A zero cannot safely
be treated as proof that the provider processed no inherited tokens. This policy
also rejects genuinely zero-token parent receipts for paid tier continuations
until reliable counter-presence provenance is available. It does not claim that
all zero receipts are malicious or invalid provider responses.

First requests remain accepted, including responses with incomplete usage receipts;
existing settlement retains the appropriate reservation when usage is absent.
Continuations with matching positive parent counters preserve the existing quote.
Authoritative free groups bypass the extra parent usage lookup. Non-tier contracts
retain their existing path. Provider-visible request bytes, output bounds, final
settlement, refunds, debt recording and the original request's consume log are
unchanged. There is no schema migration, new configuration switch or deployment
change. Reverting this repair restores the former under-reservation exposure.

## Retained regressions

`TestSecurityNativeResponseUnknownContinuationAdmission` creates a real first
response against a loopback TLS provider, persists its incomplete receipt, and
checks that both streaming and nonstreaming continuations reject before a second
provider call. It checks unchanged durable user/token balances, no second hold,
and preservation of the first consume log. No paid upstream or production state
is used.

`TestSecurityResponseContinuationTierCounters` covers receipt validation,
owner/binding identity, bounded store reads, overflow, free groups and non-tier
controls. `TestSecurityNativeResponseOwnedContinuationAdmission` retains fully
funded, underfunded owner, underfunded finite-token and operational-outage controls,
including exact-once settlement of a funded two-turn conversation.

Run the focused acceptance selection under the repository-supported Go toolchain:

```sh
go test -race ./relay/controller -run '^(TestSecurityNativeResponse(Unknown|Owned)ContinuationAdmission|TestSecurityResponseContinuationTierCounters)$' -count=1 -timeout=10m -v
go test -race ./relay/quota ./relay/pricing -count=1
go vet ./relay/controller
```

These are ordinary retained Go tests, not replacements for the existing assertions
or security gates. Follow `docs/ci-testing.md` for manual qualification and the
small automatic test budget; a focused pass is not a full-repository or production
qualification claim.
